#!/usr/bin/env bash

go test -coverprofile new_profile.txt
coverage download knative-prow $1 -o base_profile.txt
coverage diff base_profile.txt new_profile.txt