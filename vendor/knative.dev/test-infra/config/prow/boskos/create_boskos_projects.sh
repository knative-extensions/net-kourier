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

set -e

readonly NUMBER=${1:?"First argument is the number of new projects to create."}
readonly BILLING_ACCOUNT=${2:?"Second argument must be the billing account."}
# All remaining arguments will be passed verbatim to set_boskos_permissions.sh
shift 2

cd "$(dirname $0)"

readonly BOSKOS_RESOURCE_FILE=${BOSKOS_RESOURCE_FILE:-boskos_resources.yaml}
readonly BOSKOS_PROJECT_PREFIX=${BOSKOS_PROJECT_PREFIX:-knative-boskos-}

if [[ ! -f ${BOSKOS_RESOURCE_FILE} || ! -w ${BOSKOS_RESOURCE_FILE} ]]; then
  echo "${BOSKOS_RESOURCE_FILE} does not exist or is not writable"
  exit 1
fi

# Get the index of the last boskos project from the resources file
LAST_INDEX=$(grep "${BOSKOS_PROJECT_PREFIX}" ${BOSKOS_RESOURCE_FILE} | grep -o "[0-9]\+" | sort -nr | head -1)
[[ -z "${LAST_INDEX}" ]] && LAST_INDEX=0
for (( i=1; i<=${NUMBER}; i++ )); do
  PROJECT="$(printf ${BOSKOS_PROJECT_PREFIX}%02d $(( ${LAST_INDEX} + i )))"
  # This Folder ID is google.com/knative
  # If this needs to be changed for any reason, GCP project settings must be updated.
  # Details are available in Google's internal issue 137963841.
  gcloud projects create ${PROJECT} --folder=829970148577
  gcloud beta billing projects link ${PROJECT} --billing-account=${BILLING_ACCOUNT}

  LAST_PROJECT=$(grep "${BOSKOS_PROJECT_PREFIX}" ${BOSKOS_RESOURCE_FILE} | tail -1)
  [[ -z "${LAST_PROJECT}" ]] && LAST_PROJECT="- names:"
  sed "/^${LAST_PROJECT}$/a\ \ -\ ${PROJECT}" -i ${BOSKOS_RESOURCE_FILE}

  # Set permissions for this project
  "./set_boskos_permissions.sh" ${PROJECT} $@
done
