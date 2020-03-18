# Prow config

This directory contains the config for our
[Prow](https://github.com/kubernetes/test-infra/tree/master/prow) instance.

- `Makefile` Commands to interact with the Prow instance regarding configs and
  updates. Run `make help` for assistance.
- `cluster/*.yaml` Deployments of the Prow cluster.
- `core/*.yaml` Generated core configuration for Prow.
- `jobs/config.yaml` Generated configuration of the Prow jobs.
- `testgrid/testgrid.yaml` Generated Testgrid configuration.
- `config_knative.yaml` Input configuration for `config-generator` tool to
  generate `core/config.yaml`, `core/plugins.yaml`, `jobs/config.yaml` and
  `testgrid/testgrid.yaml`.
- `run_job.sh` Convenience script to start a Prow job from command-line.
- `pj-on-kind.sh` Convenience script to start a Prow job on kind from
  command-line.
- `boskos` Just Boskos resource definition and helper scripts; deployments in
  `cluster/*`.
