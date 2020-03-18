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

# This is a script to backup knative released images and yaml to the
# knative-backups project.

echo "Activating service account"
gcloud auth activate-service-account --key-file=$1

echo "Copying images"
gcrane cp -r gcr.io/knative-releases gcr.io/knative-backups

echo "Copying kubernetes manifests"
gsutil -m cp -r gs://knative-releases gs://knative-backups
