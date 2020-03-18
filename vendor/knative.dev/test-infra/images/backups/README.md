# Knative Backups Image

This directory contains the custom Docker image used by our backups job.

## Building and publishing a new image

To build and push a new image, just run `make push`.

For testing purposes you can build an image but not push it; to do so, run
`make build`.

Note that you must have proper permission in the `knative-tests` project to push
new images to the GCR.
