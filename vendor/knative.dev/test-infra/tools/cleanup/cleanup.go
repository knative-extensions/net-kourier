/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// The cleanup tool deletes old images and test clusters from test
// projects.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/pkg/errors"
	"google.golang.org/api/option"
	"knative.dev/pkg/test/cmd"
	"knative.dev/pkg/test/gke"
	"knative.dev/pkg/test/helpers"
)

var (
	// Authentication method for using Google Cloud Registry APIs.
	defaultKeychain = authn.DefaultKeychain

	// Alias of remote.Delete for testing purposes.
	remoteDelete = remote.Delete
)

// ResourceDeleter deletes a specific kind of resource in a GCP project.
type ResourceDeleter interface {
	Projects() []string
	Delete(hoursToKeepResource int, concurrentOperations int, dryRun bool) (int, []string)
	DeleteResources(project string, hoursToKeepResource int, dryRun bool) (int, error)
	ShowStats(count int, errors []string)
}

// BaseResourceDeleter implements the base operations of a ResourceDeleter.
type BaseResourceDeleter struct {
	ResourceDeleter
	projects           []string
	deleteResourceFunc func(string, int, bool) (int, error)
}

// ImageDeleter deletes old images in a given registry.
type ImageDeleter struct {
	BaseResourceDeleter
	registry string
}

// GkeClusterDeleter deletes old GKE cluster in a given project.
type GkeClusterDeleter struct {
	BaseResourceDeleter
	gkeClient gke.SDKOperations
}

// NewBaseResourceDeleter returns a brand new BaseResourceDeleter.
func NewBaseResourceDeleter(projects []string) *BaseResourceDeleter {
	deleter := BaseResourceDeleter{projects: projects}
	deleter.deleteResourceFunc = deleter.DeleteResources
	return &deleter
}

// NewImageDeleter returns a brand new ImageDeleter.
func NewImageDeleter(projects []string, registry string, serviceAccount string) (*ImageDeleter, error) {
	var err error
	deleter := ImageDeleter{*NewBaseResourceDeleter(projects), registry}
	deleter.deleteResourceFunc = deleter.DeleteResources
	if serviceAccount != "" {
		// Activate the service account.
		_, err = cmd.RunCommand("gcloud auth activate-service-account --key-file=" + serviceAccount)
		if err != nil {
			if cmdErr, ok := err.(*cmd.CommandLineError); ok {
				err = fmt.Errorf("cannot activate service account:\n%s", cmdErr.ErrorOutput)
			}
		}
	}
	return &deleter, err
}

// NewGkeClusterDeleter returns a brand new GkeClusterDeleter.
func NewGkeClusterDeleter(projects []string, serviceAccount string) (*GkeClusterDeleter, error) {
	opts := make([]option.ClientOption, 0)
	if serviceAccount != "" {
		// Create GKE client with specific credentials.
		opts = append(opts, option.WithCredentialsFile(serviceAccount))
	}
	gkeClient, err := gke.NewSDKClient(opts...)
	if err != nil {
		err = errors.Wrapf(err, "cannot create GKE SDK client")
	}
	deleter := GkeClusterDeleter{*NewBaseResourceDeleter(projects), gkeClient}
	deleter.deleteResourceFunc = deleter.DeleteResources
	return &deleter, err
}

// selectProjects returns the list of projects to iterate over.
func selectProjects(project, resourceFile, regex string) ([]string, error) {
	// Sanity check flags
	if project == "" && resourceFile == "" {
		return nil, errors.New("neither project nor resource file provided")
	}

	if project != "" && resourceFile != "" {
		return nil, errors.New("provided both project and resource file")
	}
	// --project used, just return it.
	if project != "" {
		log.Printf("Iterating over projects [%s]", project)
		return []string{project}, nil
	}
	// Otherwise, read the resource file and extract the project names.
	projectRegex, err := regexp.Compile(regex)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid regular expression %q", regex)
	}
	content, err := ioutil.ReadFile(resourceFile)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read file %q", resourceFile)
	}
	projects := make([]string, 0)
	for _, line := range strings.Split(string(content), "\n") {
		if len(line) > 0 {
			if p := projectRegex.Find([]byte(line)); p != nil {
				projects = append(projects, string(p))
			}
		}
	}
	if len(projects) == 0 {
		return nil, fmt.Errorf("no project found in %q matching %q", resourceFile, regex)
	}
	log.Printf("Iterating over %d projects defined in %q, matching %q", len(projects), resourceFile, regex)
	return projects, nil
}

// deleteImage deletes a single image pointed by the given reference.
func (d *ImageDeleter) deleteImage(ref string) error {
	image, err := name.ParseReference(ref)
	if err != nil {
		return errors.Wrapf(err, "failed to parse reference %q", ref)
	}
	if err := remoteDelete(image, remote.WithAuthFromKeychain(defaultKeychain)); err != nil {
		return errors.Wrapf(err, "failed to delete %q", image)
	}
	return nil
}

// DeleteResources deletes old clusters from a given project.
func (d *GkeClusterDeleter) DeleteResources(project string, hoursToKeepResource int, dryRun bool) (int, error) {
	before := time.Now().Add(-time.Hour * time.Duration(hoursToKeepResource))
	// TODO(adrcunha): Consider exposing https://github.com/knative/pkg/blob/6d806b998379948bd0107d77bcd831e2bdb4f3cb/testutils/clustermanager/e2e-tests/gke.go#L281
	if project == "knative-tests" {
		return 0, fmt.Errorf("cleaning up %q is forbidden", project)
	}
	// List clusters, delete those created before the given timestamp.
	clusters, err := d.gkeClient.ListClustersInProject(project)
	if err != nil {
		return 0, errors.Wrapf(err, "error listing clusters in %q, maybe try 'gcloud auth application-default login'", project)
	}
	count := 0
	for _, cluster := range clusters {
		creation, err := time.Parse(time.RFC3339, cluster.CreateTime)
		if err != nil {
			return count, errors.Wrapf(err, "error getting creation time for cluster %q", cluster.Name)
		}
		age := int(time.Since(creation).Hours())
		fullClusterName := project + "/" + cluster.Name
		log.Printf("%s is %d hours old", fullClusterName, age)
		if creation.Before(before) {
			if err := helpers.Run(fmt.Sprintf("Deleting %q", fullClusterName), func() error {
				region, zone := gke.RegionZoneFromLoc(cluster.Location)
				if err := d.gkeClient.DeleteCluster(project, region, zone, cluster.Name); err != nil {
					return errors.Wrapf(err, "error deleting cluster %q in project %q", cluster.Name, project)
				}
				count++
				return nil
			}, dryRun); err != nil {
				return count, err
			}
		}
	}
	return count, nil
}

// DeleteResources deletes old docker images from a given project.
func (d *ImageDeleter) DeleteResources(project string, hoursToKeepResource int, dryRun bool) (int, error) {
	before := time.Now().Add(-time.Hour * time.Duration(hoursToKeepResource))
	repoRoot := d.registry + "/" + project
	// TODO(adrcunha): This should be a helper function, like https://github.com/knative/pkg/blob/6d806b998379948bd0107d77bcd831e2bdb4f3cb/testutils/clustermanager/e2e-tests/gke.go#L281
	if repoRoot == "gcr.io/knative-releases" || repoRoot == "gcr.io/knative-nightly" {
		return 0, fmt.Errorf("cleaning up %q is forbidden", repoRoot)
	}
	gcrrepo, err := name.NewRepository(repoRoot)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot open registry %q", repoRoot)
	}
	count := 0
	// Walk down the registry, checking all images and deleting the old ones.
	return count, google.Walk(gcrrepo, func(repo name.Repository, tags *google.Tags, err error) error {
		// If we got an error, just return it, there's nothing to do here.
		if tags == nil || err != nil {
			if err == nil {
				return fmt.Errorf("unexpected nil tags for %q", repo)
			}
			return errors.Wrapf(err, "cannot walk down GCR %q", repo.String())
		}
		for k, m := range tags.Manifests {
			ref := repo.String() + "@" + k
			age := int(time.Since(m.Uploaded).Hours() / 24)
			log.Printf("%q is %d days old (uploaded on %s)", ref, age, m.Uploaded)
			if m.Uploaded.Before(before) {
				if err := helpers.Run(fmt.Sprintf("Deleting %q", ref), func() error {
					// Delete all tags first, otherwise the image can't be deleted.
					for _, tag := range m.Tags {
						if err := d.deleteImage(repo.String() + ":" + tag); err != nil {
							return err
						}
					}
					if err := d.deleteImage(ref); err != nil {
						return err
					}
					count++
					return nil
				}, dryRun); err != nil {
					return err
				}
			}
		}
		return nil
	}, google.WithAuthFromKeychain(defaultKeychain))
}

// Projects returns the projects that should be cleaned up by a ResourceDeleter.
func (d *BaseResourceDeleter) Projects() []string {
	return d.projects
}

// DeleteResources base method that does nothing, as it must be overridden.
func (d *BaseResourceDeleter) DeleteResources(project string, hoursToKeepResource int, dryRun bool) (int, error) {
	return 0, fmt.Errorf("not implemented")
}

// Delete call DeleteResource in parallel, one for each given project.
func (d *BaseResourceDeleter) Delete(hoursToKeepResource int, concurrentOperations int, dryRun bool) (int, []string) {
	// Locks for the concurrent tasks.
	var wg sync.WaitGroup

	// Blocking channel to keep concurrency under control.
	concurrencyChan := make(chan struct{}, concurrentOperations)

	projects := d.Projects()

	// Channel to hold errors.
	errorChan := make(chan error, len(projects))

	var count int32
	for i := range projects {
		wg.Add(1)
		go func(project string) {
			defer wg.Done()
			// Block processing if concurrency reached the limit.
			concurrencyChan <- struct{}{}
			// Do not process if previous invocations failed. This prevents a large
			// build-up of failed requests and rate limit exceeding (e.g. bad auth).
			if len(errorChan) == 0 {
				c, err := d.deleteResourceFunc(project, hoursToKeepResource, dryRun)
				// Update counter and errors list.
				atomic.AddInt32(&count, int32(c))
				if err != nil {
					errorChan <- err
				}
			}
			<-concurrencyChan
		}(projects[i])
	}
	wg.Wait()
	close(errorChan)
	close(concurrencyChan)
	// Extract the error strings from the map of errors.
	// For testing purposes, sort them to keep order constant.
	errStrings := make([]string, 0)
	for e := range errorChan {
		unique := true
		for _, s := range errStrings {
			if s == e.Error() {
				unique = false
				break
			}
		}
		if unique {
			errStrings = append(errStrings, e.Error())
		}
	}
	sort.Strings(errStrings)
	return int(count), errStrings
}

// ShowStats simply shows the number of resources deleted, and any errors.
func (d *BaseResourceDeleter) ShowStats(count int, errors []string) {
	log.Printf("%d resources deleted", count)
	if len(errors) > 0 {
		log.Printf("%d errors occurred: %s", len(errors), strings.Join(errors, ", "))
	}
}

// cleanup parses flags, run the operations and returns the status.
func cleanup() error {
	// Command-line flags.
	projectResourceYaml := flag.String("project-resource-yaml", "", "Resources file containing the names of the projects to be cleaned up.")
	project := flag.String("project", "", "Project to be cleaned up.")
	reProjectName := flag.String("re-project-name", "knative-boskos-[a-zA-Z0-9]+", "Regular expression for filtering project names from the resources file.")
	daysToKeepImages := flag.Int("days-to-keep-images", 365, "Images older than this amount of days will be deleted (defaults to 1 year, -1 means 'forever').")
	hoursToKeepClusters := flag.Int("hours-to-keep-clusters", 720, "Clusters older than this amount of hours will be deleted (defaults to 1 month, -1 means 'forever').")
	registry := flag.String("gcr", "gcr.io", "The registry hostname to use (defaults to gcr.io; currently only GCR is supported).")
	serviceAccount := flag.String("service-account", "", "Specify the key file of the service account to use.")
	concurrentOperations := flag.Int("concurrent-operations", 10, "How many deletion operations to run concurrently (defaults to 10).")
	dryRun := flag.Bool("dry-run", false, "Performs a dry run for all deletion functions.")
	flag.Parse()

	if *dryRun {
		log.Println("-- Running in dry-run mode, no resource deletion --")
	}

	if !strings.HasSuffix(*registry, "gcr.io") {
		return fmt.Errorf("currently only GCR is supported")
	}

	var projects []string
	var err error
	if projects, err = selectProjects(*project, *projectResourceYaml, *reProjectName); err != nil {
		return err
	}

	start := time.Now()

	var deleter ResourceDeleter
	if *daysToKeepImages >= 0 {
		if deleter, err = NewImageDeleter(projects, *registry, *serviceAccount); err != nil {
			return err
		}
		log.Println("Removing images that are:")
		log.Printf("- older than %d days", *daysToKeepImages)
		deleter.ShowStats(deleter.Delete(*daysToKeepImages*24, *concurrentOperations, *dryRun))
	}

	if *hoursToKeepClusters >= 0 {
		if deleter, err = NewGkeClusterDeleter(projects, *serviceAccount); err != nil {
			return err
		}
		log.Println("Removing clusters that are:")
		log.Printf("- older than %d hours", *hoursToKeepClusters)
		deleter.ShowStats(deleter.Delete(*hoursToKeepClusters, *concurrentOperations, *dryRun))
	}

	log.Printf("All operations finished in %s", time.Now().Sub(start))

	return nil
}

// main is the script entry point.
func main() {
	if err := cleanup(); err != nil {
		log.Fatalf("ERROR: %v", err)
	}
}
