# Knative Test Infrastructure

The `test-infra` repository contains a collection of tools for testing Knative,
collecting metrics and displaying test results.

## High level architecture

Knative uses [Prow](https://github.com/kubernetes/test-infra/tree/master/prow)
to schedule testing and update issues.

### Gubernator

Knative uses
[gubernator](https://github.com/kubernetes/test-infra/tree/master/gubernator) to
provide a [PR dashboard](https://gubernator.knative.dev/pr) for contributions in
the Knative github organization, and
[Spyglass](https://github.com/kubernetes/test-infra/tree/master/prow/spyglass)
to visualize test details.

### TestGrid

Knative provides a [health dashboard](https://testgrid.knative.dev/) to show
test, code and release health for each repo. It covers key areas such as
continuous integration, code coverage, nightly release, conformance and etc.

### E2E Testing

Our E2E testing uses
[kubetest](https://github.com/kubernetes/test-infra/blob/master/kubetest) to
build/deploy/test Knative clusters.
