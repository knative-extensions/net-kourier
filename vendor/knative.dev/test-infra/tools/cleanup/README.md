# Resources Clean Up Tool

This tool is designed to clean up stale test resources. For now it deletes GCR
images and GKE clusters created during testing.

It can also be used to delete GCR images and GKE clusters from an arbitrary
project.

## Basic Usage

Run `go run cleanup.go` with one of more of the flags below.

By default the current gcloud credentials are used to delete the images. If
necessary, use the flag `--service-account _key-file.json_` to specify a service
account that will be performing the access to the gcr.

Project(s) to be cleaned up are expected to be either defined in a text file or
passed (once or multiple times) using the `--project` flag.

The following flags are available for the tool:

- `--project-resource-yaml` Points to a resources file containing the names of
  the projects to be cleaned up. Such file can be any form of text, as long as
  the project names can be extracted, one per line, using a regular expression.
- `--project` Project to be cleaned up.
- `--re-project-name` Regular expression for filtering project names from the
  resources file. Optional, defaults to `knative-boskos-[a-zA-Z0-9]+`.
- `--days-to-keep-images` Optional, defaults to 365 days (aka 1 year).
- `--hours-to-keep-clusters` Optional, defaults to 720 hours (aka 30 days).
- `--gcr` Defines the GCR hostname to use (e.g., `us.gcr.io`). Optional,
  defaults to `gcr.io`.
- `--dry-run` Optional, performs a dry run for all gcloud functions, defaults to
  false.

Examples:

This command deletes test images older than 90 days and test clusters created
more than 24 hours ago in all Boskos projects.

```sh
$ go run cleanup.go --project-resource-yaml config/prow/boskos_resources.yaml --days-to-keep-images 90 --hours-to-keep-clusters 24`
```

This command deletes test images older than 1 day and test clusters created more
than 24 hours ago in a personal project called `my-knative-project`.

```sh
$ go run cleanup.go --project my-knative-project --days-to-keep-images 1 --hours-to-keep-clusters 24`
```

## Prow Job

There is a weekly prow job that triggers this tool runs at 11:00/12:00PM(Day
light saving) PST every Monday. This tool scans all projects defined in
[config/prow/boskos_resources.yaml](/config/prow/boskos/boskos_resources.yaml)
and deletes images older than 90 days and clusters older than 24 hours.
