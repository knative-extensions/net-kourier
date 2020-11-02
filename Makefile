.PHONY:  help
.DEFAULT_GOAL := help
SHELL = /bin/bash
PROJECT_PATH := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

local-setup: ## Builds and deploys kourier locally in a k3s cluster with knative, forwards the local 8080 to kourier/envoy
	./utils/setup.sh

.PHONY: test
test: test-unit test-integration ## Runs all the tests

test-unit: ## Runs unit tests
	mkdir -p "$(PROJECT_PATH)/tests_output"
	go test -mod vendor -race $(shell go list ./... | grep -v kourier/test) -coverprofile="$(PROJECT_PATH)/tests_output/unit.cov"

test-integration: local-setup ## Runs integration tests
	go test -mod vendor -race test/*.go

test-unit-coverage: test-unit ## Runs unit tests and generates a coverage report
	go tool cover -html="$(PROJECT_PATH)/tests_output/unit.cov"

.PHONY: fmt
fmt: ## Runs code formatting
	goimports -w $$(find . -type f -name '*.go' -not -path './vendor/*' -not -path './utils/extauthz_test_image/vendor/*')

help: ## Print this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-39s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
