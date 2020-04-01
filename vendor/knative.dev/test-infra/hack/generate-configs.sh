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

set -o errexit
set -o nounset

source $(dirname $0)/../scripts/library.sh

# Generate Prow configs since we are using generator
readonly CONFIG_GENERATOR_DIR="${REPO_ROOT_DIR}/tools/config-generator"
readonly CONFIG_DIR="${REPO_ROOT_DIR}/config"

# Generate config for production Prow
go run "${CONFIG_GENERATOR_DIR}" \
    --env="prow" \
    --gcs-bucket="knative-prow" \
    --generate-testgrid-config=true \
    --generate-maintenance-jobs=true \
    --image-docker=gcr.io/knative-tests/test-infra \
    --prow-host=https://prow.knative.dev \
    --testgrid-gcs-bucket="knative-testgrid" \
    --plugins-config-output="${CONFIG_DIR}/prow/core/plugins.yaml" \
    --prow-config-output="${CONFIG_DIR}/prow/core/config.yaml" \
    --prow-jobs-config-output="${CONFIG_DIR}/prow/jobs/config.yaml" \
    --testgrid-config-output="${CONFIG_DIR}/prow/testgrid/testgrid.yaml" \
    "${CONFIG_DIR}/prow/config_knative.yaml"

# Generate config for staging Prow
go run "${CONFIG_GENERATOR_DIR}" \
    --env="prow-staging" \
    --gcs-bucket="knative-prow-staging" \
    --generate-testgrid-config=false \
    --generate-maintenance-jobs=false \
    --image-docker=gcr.io/knative-tests-staging/test-infra \
    --prow-host=https://prow-staging.knative.dev \
    --testgrid-gcs-bucket="knative-testgrid-staging" \
    --plugins-config-output="${CONFIG_DIR}/prow-staging/core/plugins.yaml" \
    --prow-config-output="${CONFIG_DIR}/prow-staging/core/config.yaml" \
    --prow-jobs-config-output="${CONFIG_DIR}/prow-staging/jobs/config.yaml" \
    --testgrid-config-output="${CONFIG_DIR}/prow-staging/testgrid/testgrid.yaml" \
    "${CONFIG_DIR}/prow-staging/config_staging.yaml"
