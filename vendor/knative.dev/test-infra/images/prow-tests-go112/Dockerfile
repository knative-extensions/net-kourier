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

FROM gcr.io/knative-tests/test-infra/prow-tests:stable
LABEL maintainer "Chao Dai <chaodai@google.com>"

# Install go at 1.12 same as how it's installed in kubekins-e2e image, reference code here:
# https://github.com/kubernetes/test-infra/blob/1e9b5dc3de4b268aab57c9c48120aa3dcf096bc6/images/kubekins-e2e/Dockerfile#L64
ENV GO_TARBALL "go1.12.linux-amd64.tar.gz"
RUN rm -rf /usr/local/go && \
    wget -q "https://storage.googleapis.com/golang/${GO_TARBALL}" && \
    tar xzf "${GO_TARBALL}" -C /usr/local && \
    rm "${GO_TARBALL}"
