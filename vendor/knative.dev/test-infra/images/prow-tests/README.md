# Prow Test Job Image

This directory contains the custom Docker image used by our Prow test jobs. A
fork of this image with go1.12 installed is defined in
[`prow-tests-go112`](../prow-tests-go112)

## Building and publishing a new image

To build and push a new image, just run `make push`.

For testing purposes you can build an image but not push it; to do so, run
`make build`.

`make build_no_cache` is similar to `make build` except it does not use the
cache when building images with docker. This ensures the image is built with the
latest version of each tool.

The Prow jobs are configured to use the `prow-tests` image tagged with `stable`.
This tag must be manually set in GCR using the Cloud Console.

Note that you must have proper permission in the `knative-tests` project to push
new images to the GCR.

The `prow-tests` image is pinned on a specific `kubekins` image; update
`Dockerfile` if you need to use a newer/different image. This will basically
define the versions of `bazel`, `go`, `kubectl` and other build tools.
