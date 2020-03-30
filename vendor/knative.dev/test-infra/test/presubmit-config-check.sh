#!/usr/bin/env bash

# Copyright 2020 The Knative Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

source $(dirname $0)/../scripts/library.sh

set -e

trap 'echo "--- FAIL: Directly changing production Prow config files is not allowed, please move the changes to staging directories."' ERR
header "Checking to make sure Prow productions config files are not modified manually"
go run "${REPO_ROOT_DIR}"/tools/prow-config-updater/presubmit-checker --github-token="/etc/repoview-token/token"

trap 'echo "--- FAIL: Prow config files have errors, please check."' ERR
header "Validating production Prow config files"
bazel run @k8s//prow/cmd/checkconfig -- \
  --config-path="$(realpath "config/prow/core/config.yaml")" \
  --job-config-path="$(realpath "config/prow/jobs/config.yaml")" \
  --plugin-config="$(realpath "config/prow/core/plugins.yaml")"

header "Validating staging Prow config files"
bazel run @k8s//prow/cmd/checkconfig -- \
  --config-path="$(realpath "config/prow-staging/core/config.yaml")" \
  --job-config-path="$(realpath "config/prow-staging/jobs/config.yaml")" \
  --plugin-config="$(realpath "config/prow-staging/core/plugins.yaml")"

trap 'echo "--- FAIL: Testgrid config file has errors, please check."' ERR
header "Validating Testgrid config file"
bazel run @k8s//testgrid/cmd/configurator -- \
  --validate-config-file \
  --yaml="$(realpath "config/prow/testgrid/testgrid.yaml")"
# Ensure TestGrid config can be converted to binary form.
bazel run @k8s//testgrid/cmd/configurator -- \
  --oneshot \
  --output=/dev/null \
  --yaml="$(realpath "config/prow/testgrid/testgrid.yaml")"
