# Design of the Test Coverage Tool

![design.svg](design.svg)

We pack the test coverage tool in a container, that is triggered by Prow. It
generates a test coverage profiling for the target repository. Afterward, it
calculates the coverage change or summarizes the coverage data, depending on the
workflow type, explained below.

## Post-submit workflow

Produces and stores a coverage profile for the master branch, for later
presubmit jobs to compare against.

1. A PR is merged.
1. Post-submit Prow job started.
1. Test coverage profile generated.

## Pre-submit workflow

The pre-submit workflow is triggered when a pull request is created or updated.
It runs the code coverage tool to report coverage changes through a comment by a
robot GitHub account.

1. Developer submits a new commit to an open PR on GitHub.
1. Matching pre-submit Prow job is started.
1. Test coverage profile generated.
1. Calculate coverage change against master branch. Compare the coverage file
   generated in this cycle against the coverage file in the master branch.
1. Using the PR data from GitHub, `.gitattributes` file, as well as coverage
   change data calculated above, produce a list of files that we care about in
   the line-by-line coverage report. Produce line by line coverage HTML files
   and add links to them in the PR coverage report.
1. The robot account posts the code coverage report on GitHub, as a comment in
   the PR.

## Periodic workflow

Produces periodic code coverage results as input for TestGrid.

1. Periodic Prow job starts. The frequency and start time can be configured in
   [the config file](../config-generator/testgrid_config.go)
1. Test coverage profile and metadata generated.
1. Generate and store per-file coverage data.

- Stores in the XML format that is used by TestGrid.
