#!/usr/bin/env bash

# Copyright 2019 The Knative Authors
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


# There are a couple of dependencies that we need to update manually. go mod
# does not handle them because they are only used in the scripts under the
# "/test" directory.

readonly ROOT_DIR=$(dirname "$0")/..

set -o errexit
set -o nounset
set -o pipefail

SERVING_VERSION=0.12.1
TEST_INFRA_VERSION=master

cd "${ROOT_DIR}"

# Update serving
rm -rf "${ROOT_DIR}"/vendor/knative.dev/serving
wget -qO- github.com/knative/serving/archive/v"${SERVING_VERSION}".zip &> /tmp/serving.zip
unzip /tmp/serving.zip -d "${ROOT_DIR}"/vendor/knative.dev/
mv "${ROOT_DIR}"/vendor/knative.dev/serving-"$SERVING_VERSION" "${ROOT_DIR}"/vendor/knative.dev/serving

# Update test-infra
rm -rf "${ROOT_DIR}"/vendor/knative.dev/test-infra
wget -qO- github.com/knative/test-infra/archive/"${TEST_INFRA_VERSION}".zip &> /tmp/test-infra.zip
unzip /tmp/test-infra.zip -d "${ROOT_DIR}"/vendor/knative.dev/
mv "${ROOT_DIR}"/vendor/knative.dev/test-infra-"$TEST_INFRA_VERSION" "${ROOT_DIR}"/vendor/knative.dev/test-infra
