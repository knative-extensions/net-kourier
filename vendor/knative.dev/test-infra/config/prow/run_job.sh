#!/usr/bin/env bash

# Copyright 2018 The Knative Authors
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

# Simple script to start a Prow job.

source $(dirname $0)/../../scripts/library.sh

[[ -z "$1" ]] && abort "pass the name of the job to start as argument"

set -e

cd ${REPO_ROOT_DIR}

make -C ./config/prow get-cluster-credentials

run_mkpj="mkpj"
[[ -z "$(which mkpj)" ]] && run_mkpj="bazel run @k8s//prow/cmd/mkpj --"

JOB_YAML=$(mktemp)
CONFIG_YAML=${REPO_ROOT_DIR}/config/prow/core/config.yaml
JOB_CONFIG_YAML=${REPO_ROOT_DIR}/config/prow/jobs/config.yaml
${run_mkpj} --job=$1 --config-path=${CONFIG_YAML} --job-config-path=${JOB_CONFIG_YAML} > ${JOB_YAML}
echo "Job YAML file saved to ${JOB_YAML}"
kubectl apply -f ${JOB_YAML}

make -C ./config/prow unset-cluster-credentials
