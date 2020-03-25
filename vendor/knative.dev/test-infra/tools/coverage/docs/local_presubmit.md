# Local Pre-submit Diff

A pre-submit check, to see how the coverage of your local repository compare to
the latest successful postsubmit run, can be run locally in the following way

## Steps

1. `go get k8s.io/test-infra/robots/coverage`

2. Go to the target directory (where you want to run code coverage on), call the
   [local_presubmit.sh](../local_presubmit.sh) with the name of the Prow
   post-submit coverage job, e.g.
   `./local_presubmit.sh post-knative-serving-go-coverage`.

## Note

This script is not intended to test the changes made to the tool here in
`knative/test-infra/tools/coverage`. The
[external tool](https://github.com/kubernetes/test-infra/tree/master/robots/coverage)
was derived from this tool but may diverge as it was ported to the Kubernetes
repository. By design, both of them should produce the same result, but that
cannot be ensured unless we converge them.
