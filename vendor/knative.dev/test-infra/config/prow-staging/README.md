# Staging Prow config

This directory contains the config for our staging
[Prow](https://github.com/kubernetes/test-infra/tree/master/prow) instance.

- `Makefile` Commands to interact with the staging Prow instance regarding
  configs and updates. Run `make help` for assistance.
- `boskos/boskos_resources.yaml` Pool of projects used by Boskos.
- `core/config.yaml` Generated configuration for Prow.
- `jobs/config.yaml` Generated configuration for Prow jobs.
- `config_staging.yaml` Input configuration for `make_config.go` to generate
  `jobs/config.yaml`.
- `core/plugins.yaml` Generated configuration of the Prow plugins.
