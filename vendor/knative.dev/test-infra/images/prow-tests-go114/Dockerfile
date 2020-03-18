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

FROM gcr.io/knative-tests/test-infra/prow-tests:stable
LABEL maintainer "Chao Dai <chaodai@google.com>"

ENV GO_TARBALL "go1.14beta1.tar.gz"

# Required variables to install go from source
# Reference: https://golang.org/doc/install/source#environment
ENV GOROOT_BOOTSTRAP "/usr/local/go-old"
ENV GOROOT "/usr/local/go"
ENV CGO_ENABLED 1
ENV GOOS "linux"
ENV GOARCH "amd64"

# install netbase as it's required for Go linux amd64 install (OS specific)
RUN apt-get install netbase

# go1.14 is not released yet. Downloading the beta version directly from github and building from source
# Reference: https://golang.org/doc/install/source
RUN mv /usr/local/go /usr/local/go-old && \
    wget -q "https://github.com/golang/go/archive/${GO_TARBALL}" && \
    tar xzf "${GO_TARBALL}" -C /usr/local && \
    mv /usr/local/go-go1.14beta1 /usr/local/go && \
    cd ${GOROOT}/src && \
    echo go1.14beta1 > ${GOROOT}/VERSION && \
    ./all.bash && \
    rm -rf ${GOROOT_BOOTSTRAP}
