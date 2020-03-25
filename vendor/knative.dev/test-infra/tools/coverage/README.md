# Overview

The code coverage tool has two major features:

1. As a pre-submit tool, it runs code coverage on every single commit to Github
   and reports coverage changes back to the PR as a comment by a robot account.
   If made a required job on the repository, it can be used to block a PR from
   merging if coverage falls below a certain threshold.
1. As a periodic running job, it outputs a Junit XML that can be read from other
   tools like [TestGrid](http://testgrid.knative.dev/serving#coverage) to get
   overall coverage metrics.

## Design

See the [design document](design.md).

## Build and Release

In the `/images/coverage` directory, run `make IMAGE_NAME=coverage-dev push` to
build and upload a staging version, intended for testing and debugging. The
staging version can be triggered on a PR through the comment
`/test pull-knative-serving-go-coverage-dev`. Note that staging version can only
be tested against the serving repository because the staging jobs only exist in
the serving repository.

### Validating the staging version

- To run the pre-submit workflow, add the comment
  `/test pull-knative-serving-go-coverage-dev` to a PR.
- To run the periodic workflow, (re)run a `post-knative-serving-go-coverage-dev`
  job.

To publish a new version of the code coverage tool, run `make push`.
