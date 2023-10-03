#!/bin/bash

# Copyright 2021 The Knative Authors
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

KOURIER_CONTROL_NAMESPACE=knative-serving
out_dir="$(mktemp -d /tmp/certs-XXX)"
subdomain="example.com"

openssl req -x509 -sha256 -nodes -days 365 -newkey rsa:2048 \
  -subj "/O=Example Inc./CN=Example" \
  -keyout "${out_dir}"/root.key \
  -out "${out_dir}"/root.crt

openssl req -nodes -newkey rsa:2048 \
    -subj "/O=Example Inc./CN=Example" \
    -reqexts san \
    -config <(printf "[req]\ndistinguished_name=req\n[san]\nsubjectAltName=DNS:*.%s" "$subdomain") \
    -keyout "${out_dir}"/wildcard.key \
    -out "${out_dir}"/wildcard.csr

openssl x509 -req -days 365 -set_serial 0 \
    -extfile <(printf "subjectAltName=DNS:*.%s" "$subdomain") \
    -CA "${out_dir}"/root.crt \
    -CAkey "${out_dir}"/root.key \
    -in "${out_dir}"/wildcard.csr \
    -out "${out_dir}"/wildcard.crt

kubectl create -n ${KOURIER_CONTROL_NAMESPACE} secret tls wildcard-certs \
    --key="${out_dir}"/wildcard.key \
    --cert="${out_dir}"/wildcard.crt --dry-run=client -o yaml | kubectl apply -f -
