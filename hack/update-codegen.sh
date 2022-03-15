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
set -o pipefail

source $(dirname $0)/../vendor/knative.dev/hack/codegen-library.sh

group "Deepcopy Gen"

# Broken out of code-generator scripts, since we don't need generate-groups.sh
(
    cd "${CODEGEN_PKG}"
    go install ./cmd/deepcopy-gen
)

${GOPATH}/bin/deepcopy-gen \
  -O zz_generated.deepcopy \
  --go-header-file "${REPO_ROOT_DIR}/hack/boilerplate/boilerplate.go.txt" \
  -i knative.dev/net-kourier/pkg/config \
  -i knative.dev/net-kourier/pkg/reconciler/ingress/config

# Make sure our dependencies are up-to-date
${REPO_ROOT_DIR}/hack/update-deps.sh
