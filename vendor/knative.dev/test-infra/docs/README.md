# This is not what you're looking for...

This directory doesn't host documentation for the knative/test-infra repository.

Instead, this is the source of the GitHub Pages for knative/test-infra. See
https://pages.github.com/ for details.

Documentation can be found in other directories of the repository. For example:

- [Main README](../README.md)
- [Documentation on configuration of the CI/CD system](../guides/prow_setup.md)
- [Documentation on the helper scripts](../scripts/README.md)

## Updating the test-infra GitHub page

Main contents are rendered from [index.html](index.html).

In order to allow [index.html](index.html) to read data from the oncall GCS
bucket, proper permissions must be granted:

```shell
$ gsutil cors set cors-json-file.json gs://knative-infra-oncall
```

For more details, see https://cloud.google.com/storage/docs/configuring-cors
