.PHONY:  help
.DEFAULT_GOAL := help
SHELL = /bin/bash
PROJECT_PATH := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

run: ## runs kourier locally with "go run"
	@echo "[i] Remember to have a valid kubeconfig in $(HOME)/.kube/config"
	@go run ./cmd/kourier/main.go

docker-run-envoy: ## Runs envoy in a docker
	docker run --rm  -p 19000:19000 -p 10000:10000 --link kourier --name kourier_envoy -v $(PWD)/conf/:/tmp/conf --entrypoint=/usr/local/bin/envoy -ti docker.io/maistra/proxyv2-ubi8:1.0.8 -c /tmp/conf/envoy-bootstrap.yaml

docker-run: docker-build ## Runs kourier in a docker
	@echo "[i] Remember to have a valid kubeconfig in $(HOME)/.kube/config"
	docker run --rm  --name kourier -v $(HOME)/.kube:/tmp/.kube -ti 3scale-kourier:test -kubeconfig /tmp/.kube/config

build: ## Builds kourier binary, outputs binary to ./build
	mkdir -p ./build
	go build -mod vendor -o build/kourier cmd/kourier/main.go 

docker-build: ## Builds kourier docker, tagged by default as 3scale-kourier:test
	docker build -t 3scale-kourier:test ./

docker-build-extauthzutil: ## Builds kourier docker, tagged by default as test_externalauthz:latest
	docker build -f ./utils/extauthz_test_image/Dockerfile -t test_externalauthz:latest ./utils/extauthz_test_image/

local-setup: ## Builds and deploys kourier locally in a k3s cluster with knative, forwards the local 8080 to kourier/envoy
	./utils/setup.sh

test: test-unit test-integration ## Runs all the tests

test-unit: ## Runs unit tests
	mkdir -p "$(PROJECT_PATH)/tests_output"
	go test -mod vendor -race $(shell go list ./... | grep -v kourier/test) -coverprofile="$(PROJECT_PATH)/tests_output/unit.cov"

test-integration: local-setup ## Runs integration tests
	go test -mod vendor -race test/* -args -kubeconfig="$(shell k3d get-kubeconfig --name='kourier-integration')"

test-unit-coverage: test-unit ## Runs unit tests and generates a coverage report
	go tool cover -html="$(PROJECT_PATH)/tests_output/unit.cov"

.PHONY: fmt
fmt: ## Runs code formatting
	goimports -w $$(find . -type f -name '*.go' -not -path './vendor/*' -not -path './utils/extauthz_test_image/vendor/*')

help: ## Print this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-39s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
