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

set -e

# This scripts takes no command line parameters. It reads from BOSKOS_RESOURCE_FILE
# and run set_up_boskos_project.sh on projects that matches BOSKOS_PROJECT_PREFIX.
# All additional arguments will be passed verbatim to set_up_boskos_project.sh

cd "$(dirname $0)"

readonly BOSKOS_RESOURCE_FILE=${BOSKOS_RESOURCE_FILE:-boskos_resources.yaml}
readonly BOSKOS_PROJECT_PREFIX=${BOSKOS_PROJECT_PREFIX:-knative-boskos-}

if [[ ! -f ${BOSKOS_RESOURCE_FILE} ]]; then
  echo "${BOSKOS_RESOURCE_FILE} does not exist"
  exit 1
fi

# Get the all boskos project names from the resource file. Each project separated by new line
BOSKOS_PROJECTS=$(grep "${BOSKOS_PROJECT_PREFIX}" ${BOSKOS_RESOURCE_FILE} | grep -o "${BOSKOS_PROJECT_PREFIX}[0-9]\+")
if [[ -z "${BOSKOS_PROJECTS}" ]]; then
  echo "There's no Boskos project with prefix ${BOSKOS_PROJECT_PREFIX} to update."
  exit 0
fi

for boskos_project in ${BOSKOS_PROJECTS}; do
  # Set up this project
  ./set_up_boskos_project.sh ${boskos_project} $@
done
