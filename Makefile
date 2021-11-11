.PHONY:  help
.DEFAULT_GOAL := help
SHELL = /bin/bash
PROJECT_PATH := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

test-unit-coverage: ## Runs unit tests and generates a coverage report
	mkdir -p "$(PROJECT_PATH)/tests_output"
	go test -race ./... -coverprofile="$(PROJECT_PATH)/tests_output/unit.cov"
	go tool cover -html="$(PROJECT_PATH)/tests_output/unit.cov"

.PHONY: fmt
fmt: ## Runs code formatting
	goimports -w $$(find . -type f -name '*.go' -not -path './vendor/*' -not -path './utils/extauthz_test_image/vendor/*')

help: ## Print this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-39s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
