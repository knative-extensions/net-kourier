# Automating releases for a new Knative repository

**Note:** Throughout this document, MODULE is a Knative module name, e.g.
`serving` or `eventing`.

By using the release automation already in place, a new Knative repository can
get nightly and official releases with little effort. All automated releases are
monitored through [TestGrid](http://testgrid.knative.dev).

- **Nightly releases** are built every night between 2AM and 3 AM (PST), from
  HEAD on the master branch. They are referenced by a date/commit label, in the
  form `vYYYYMMDD-<commit_short_hash>`. The job status can be checked in the
  `nightly` tab in the corresponding repository dashboard in TestGrid. Images
  are published to the `gcr.io/knative-nightly/MODULE` registry and manifests to
  the `knative-nightly/MODULE` GCS bucket.

- **Versioned releases** are usually built against a branch in the repository.
  They are referenced by a _vX.Y.Z_ label, and published in the _Releases_ page
  of the repository. Images are published to the `gcr.io/knative-release/MODULE`
  registry and manifests to the `knative-releases/MODULE` GCS bucket.

Versioned releases can be one of two kinds:

- **Major or minor releases** are those with changes to the `X` or `Y` values in
  the version. They are cut only when a new release branch (which must be named
  `release-X.Y`) is created from the master branch of a repository. Within about
  2 to 3 hours the new release will be built and published. The job status can
  be checked in the `auto-release` tab in the corresponding repository dashboard
  in TestGrid. The release notes published to GitHub are empty, so you must
  manually edit it and add the relevant markdown content.

- **Patch or dot releases** are those with changes to the `Z` value in the
  version. They are cut automatically, every Tuesday night between 2AM and 3AM
  (PST). For example, if the latest release on release branch `release-0.2` is
  `v0.2.1`, the next minor release will be named `v0.2.2`. A minor release is
  only created if there are new commits to the latest release branch of a
  repository. The job status can be checked in the `dot-release` tab in the
  corresponding repository dashboard in TestGrid. The release notes published to
  GitHub are a copy of the previous release notes, so you must manually edit it
  and adjust its content.

## Setting up automated releases

1. Have the
   [/test/presubmit-tests.sh](prow_setup.md#setting-up-jobs-for-a-new-repo)
   script added to your repo, as it's used as a release gateway. Alternatively,
   have some sort of validation and set `$VALIDATION_TESTS` in your release
   script (see below).

1. Write your release script, which will publish your artifacts. For details,
   see the
   [helper script documentation](../scripts/README.md#using-the-releasesh-helper-script).

1. Enable `nightly`, `auto-release` and `dot-release` jobs for your repo in the
   [config_knative.yaml](../config/prow/config_knative.yaml) file. For example:

   ```
   knative/MODULE:
    - nightly: true
    - dot-release: true
    - auto-release: true
   ```

1. Run `make config` to regenerate
   [config.yaml](../config/prow/jobs/config.yaml), otherwise the presubmit test
   will fail. Merge such pull request and ask the
   [oncall](https://knative.github.io/test-infra/) to update the Prow cluster
   and TestGrid with the new configs, by running `make update-prow-job-config`
   and `make update-testgrid-config` in `config/prow`. Within two hours the 3
   new jobs (nightly, auto-release and dot-release) will appear on TestGrid.

   The jobs can also be found in the
   [Prow status page](https://prow.knative.dev) under the names
   `ci-knative-MODULE-nightly-release`, `ci-knative-MODULE-auto-release` and
   `ci-knative-MODULE-dot-release`.

## Creating a major version release

### Starting the release from the GitHub UI

1. Click the _Branch_ dropdown.
1. Type the desired `release-X.Y` branch name into the search box.
1. Click the `Create branch: release-X.Y from 'master'` button. _You must have
   write permissions to the repo to create a branch._

### Starting the release from the Git CLI

1.  Fetch the upstream remote.

    ```sh
    git fetch upstream
    ```

1.  Create a `release-X.Y` branch from `upstream/master`.

    ```sh
    git branch --no-track release-X.Y upstream/master
    ```

1.  Push the branch to upstream.

    ```sh
    git push upstream release-X.Y
    ```

    _You must have write permissions to the repo to create a branch._

### Finishing the release

Within 2 hours, Prow will detect the new release branch and run the
`hack/release.sh` script. If the build succeeds, a new tag `vX.Y.0` will be
created and a GitHub release published. If the build fails, logs can be
retrieved from `https://testgrid.knative.dev/MODULE#auto-release`.

Write release notes and add them to the release at
`https://github.com/knative/MODULE/releases`.

## Adding a commit to the next minor version release

1.  Fetch the upstream remote.

    ```sh
    git fetch upstream
    ```

1.  Create a branch based on the desired (usually the latest) `release-X.Y`
    branch.

    ```sh
    git checkout -b my-backport-branch upstream/release-X.Y
    ```

1.  Cherry-pick desired commits from master into the new branch.

    ```sh
    git cherry-pick <commitid>
    ```

1.  Push the branch to your fork.

    ```sh
    git push origin
    ```

1.  Create a PR for your branch based on the `release-X.Y` branch.

1.  Once the PR is merged, it will be included in the next minor release, which
    is usually built Tuesday nights, between 2AM and 3AM.

**Note**: If a minor release is required for a release branch that's not the
latest, the job must be
[started manually](manual_release.md#creating-a-dot-release-on-demand).

### Finishing a minor release

When the minor release job runs, it will detect the new commits in the latest
release branch and run the `release.sh` script. If the build succeeds, a new tag
`vX.Y.Z` will be created (where `Z` is the current minor version number + 1) and
a GitHub release published at `https://github.com/knative/MODULE/releases`. If
the build fails, logs can be retrieved from
`https://testgrid.knative.dev/MODULE#dot-release`.

Write release notes and add them to the release at
`https://github.com/knative/MODULE/releases`.
