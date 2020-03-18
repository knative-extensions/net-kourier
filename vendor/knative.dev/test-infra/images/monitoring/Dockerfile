# Copyright 2019 The Knative Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang:1.10.2
RUN apt-get update

ENV TEMP_REPO_DIR /go/src/knative.dev/test-infra
ENV TOOL_NAME monitoring

# Temporarily add test-infra to the image to build custom tools
ADD ./ $TEMP_REPO_DIR

RUN make -C $TEMP_REPO_DIR/tools/$TOOL_NAME/
RUN cp $TEMP_REPO_DIR/tools/$TOOL_NAME/$TOOL_NAME /$TOOL_NAME

# Remove test-infra from the container
RUN rm -fr $TEMP_REPO_DIR

ENTRYPOINT ["/monitoring"]
