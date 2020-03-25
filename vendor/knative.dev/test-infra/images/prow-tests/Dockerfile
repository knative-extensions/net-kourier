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

# This is the base Dockerfile for knative-tests images, please remember to also
# do `make push` in `../prow-tests-go112` directory to make sure these 2 images are in
# sync

# TODO(chizhg): probably build the image from scratch?
FROM gcr.io/k8s-testimages/kubekins-e2e:v20200302-c921a88-master
LABEL maintainer "Adriano Cunha <adrcunha@google.com>"

# Install extras on top of base image

ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update
RUN gcloud components update

# Docker
RUN gcloud components install docker-credential-gcr
RUN docker-credential-gcr configure-docker

# Extra tools through apt-get
RUN apt-get install -y uuid-runtime  # for uuidgen
RUN apt-get install -y rubygems  # for mdl

# Install go at 1.13 same as how it's installed in kubekins-e2e image, reference code here:
# https://github.com/kubernetes/test-infra/blob/1e9b5dc3de4b268aab57c9c48120aa3dcf096bc6/images/kubekins-e2e/Dockerfile#L64
ENV GO_TARBALL "go1.13.linux-amd64.tar.gz"
RUN rm -rf /usr/local/go && \
    wget -q "https://storage.googleapis.com/golang/${GO_TARBALL}" && \
    tar xzf "${GO_TARBALL}" -C /usr/local && \
    rm "${GO_TARBALL}"

# Extra tools through go get
RUN GO111MODULE=on go get github.com/google/ko/cmd/ko@v0.3.0
RUN GO111MODULE=on go get github.com/boz/kail/cmd/kail
RUN go get -u github.com/golang/dep/cmd/dep
RUN go get -u github.com/jstemmer/go-junit-report
RUN GO111MODULE=on go get -u github.com/raviqqe/liche@v0.0.0-20200229003944-f57a5d1c5be4  # stable liche version for checking md links

# Extract bazel version
RUN bazel version > /bazel_version
RUN bazel shutdown

# Extract ko version
RUN ko version > /ko_version

# Extra tools through gem
RUN gem install mixlib-config -v 2.2.4  # required because ruby is 2.1
RUN gem install mdl -v 0.5  # required because later version of mdl requires ruby >= 2.4

# Temporarily add test-infra to the image to build custom tools
ADD . /go/src/knative.dev/test-infra

# Build custom tools in the container
RUN make -C /go/src/knative.dev/test-infra/tools/githubhelper
RUN cp /go/src/knative.dev/test-infra/tools/githubhelper/githubhelper .

# Remove test-infra from the container
RUN rm -fr /go/src/knative.dev/test-infra
