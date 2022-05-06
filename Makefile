
# Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
# See LICENSE for license information.

# ====================================================================================
# Variables

# App Code location
CONFIG_APP_CODE         += ./cmd/rtcd

## Docker Variables
# Docker executable
DOCKER                  := $(shell which docker)
# Dockerfile's location
DOCKER_FILE             += ./build/Dockerfile
# Docker options to inherit for all docker run commands
DOCKER_OPTS             += --rm -i -u $$(id -u):$$(id -g) --platform "linux/amd64"
# Registry to upload images
DOCKER_REGISTRY         += docker.io
DOCKER_REGISTRY_REPO    += mattermost
# Registry credentials
DOCKER_USER             ?= user
DOCKER_PASSWORD         ?= password
## Docker Images
DOCKER_IMAGE_GOLINT     += "golangci/golangci-lint:v1.45.2@sha256:e84b639c061c8888be91939c78dae9b1525359954e405ab0d9868a46861bd21b"
DOCKER_IMAGE_DOCKERLINT += "hadolint/hadolint:v2.9.2@sha256:d355bd7df747a0f124f3b5e7b21e9dafd0cb19732a276f901f0fdee243ec1f3b"


## Go Variables
# Go executable
GO                           := $(shell which go)
# LDFLAGS
GO_LDFLAGS                   += -X "github.com/mattermost/${APP_NAME}/service.buildHash=$(APP_COMMIT)"
GO_LDFLAGS                   += -X "github.com/mattermost/${APP_NAME}/service.buildVersion=$(APP_VERSION)"
GO_LDFLAGS                   += -X "github.com/mattermost/${APP_NAME}/service.buildDate=$(BUILD_DATE)"
# Architectures to build for
GO_BUILD_PLATFORMS           ?= linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 freebsd-amd64
GO_BUILD_PLATFORMS_ARTIFACTS = $(foreach cmd,$(addprefix go-build/,${APP_NAME}),$(addprefix $(cmd)-,$(GO_BUILD_PLATFORMS)))
# Build options
GO_BUILD_OPTS                += -mod=readonly -trimpath
GO_TEST_OPTS                 += -mod=readonly -failfast -race
# Temporary folder to output compiled binaries artifacts
GO_OUT_BIN_DIR               := ./dist

## General Variables
# Use repository name as application name
APP_NAME    := $(shell basename -s .git `git config --get remote.origin.url`)
# Get current commit
APP_COMMIT  := $(shell git log --pretty=format:'%h' -n 1)
# Check if we are in a release tag, if yes use it as app version.
APP_VERSION := $(shell git describe --abbrev=0 --exact-match --tags > /dev/null 2>&1 || echo dev-$(APP_COMMIT))
# Get current date and format like: 2022-04-27 11:32
BUILD_DATE  := $(shell date +%Y-%m-%d\ %H:%M)

## General Configuration Variables
# We don't need make's built-in rules.
MAKEFLAGS     += --no-builtin-rules
# Be pedantic about undefined variables.
MAKEFLAGS     += --warn-undefined-variables
# Set help as default target
.DEFAULT_GOAL := help

# ====================================================================================
# Colors

BLUE   := $(shell printf "\033[34m")
YELLOW := $(shell printf "\033[33m")
RED    := $(shell printf "\033[31m")
GREEN  := $(shell printf "\033[32m")
CYAN   := $(shell printf "\033[36m")
CNone  := $(shell printf "\033[0m")

# ====================================================================================
# Logger

TIME_LONG	= `date +%Y-%m-%d' '%H:%M:%S`
TIME_SHORT	= `date +%H:%M:%S`
TIME		= $(TIME_SHORT)

INFO = echo ${TIME} ${BLUE}[ .. ]${CNone}
WARN = echo ${TIME} ${YELLOW}[WARN]${CNone}
ERR  = echo ${TIME} ${RED}[FAIL]${CNone}
OK   = echo ${TIME} ${GREEN}[ OK ]${CNone}
FAIL = (echo ${TIME} ${RED}[FAIL]${CNone} && false)

# ====================================================================================
# Verbosity control hack

VERBOSE ?= 0
AT_0 := @
AT_1 :=
AT = $(AT_$(VERBOSE))

# ====================================================================================
# Targets

help: ## to get help
	@echo "Usage:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) |\
	awk 'BEGIN {FS = ":.*?## "}; {printf "make ${CYAN}%-30s${CNone} %s\n", $$1, $$2}'

.PHONY: lint
lint: go-lint go-mod-check docker-lint ## to lint all

.PHONY: test
test: go-test ## to test all

.PHONY: docker-build
docker-build: go-build ## to build the docker image
	@$(INFO) Performing Docker build...
	$(AT)$(DOCKER) build -f ${DOCKER_FILE} . \
	-t ${APP_NAME}:${APP_VERSION} || ${FAIL}
	@$(OK) Performing Docker build ${APP_NAME}:${APP_VERSION}

.PHONY: docker-push
docker-push: ## to push the docker image
	@$(INFO) Pushing to registry...
	$(AT)$(DOCKER) tag ${APP_NAME}:${APP_VERSION} $(DOCKER_REGISTRY)/${DOCKER_REGISTRY_REPO}/${APP_NAME}:${APP_VERSION} || ${FAIL}
	$(AT)$(DOCKER) push $(DOCKER_REGISTRY)/${DOCKER_REGISTRY_REPO}/${APP_NAME}:${APP_VERSION} || ${FAIL}
	@$(OK) Pushing to registry $(DOCKER_REGISTRY)/${DOCKER_REGISTRY_REPO}/${APP_NAME}:${APP_VERSION}

.PHONY: docker-sbom
docker-sbom: ## to print a sbom report
	@$(INFO) Performing Docker sbom report...
	$(AT)$(DOCKER) sbom ${APP_NAME}:${APP_VERSION} || ${FAIL}
	@$(OK) Performing Docker sbom report

.PHONY: docker-scan
docker-scan: ## to print a vulnerability report
	@$(INFO) Performing Docker scan report...
	$(AT)$(DOCKER) scan ${APP_NAME}:${APP_VERSION} || ${FAIL}
	@$(OK) Performing Docker scan report

.PHONY: docker-lint
docker-lint: ## to lint the Dockerfile
	@$(INFO) Dockerfile linting...
	$(AT)$(DOCKER) run ${DOCKER_OPTS} \
	${DOCKER_IMAGE_DOCKERLINT} \
	< ${DOCKER_FILE} || ${FAIL}
	@$(OK) Dockerfile linting

.PHONY: docker-login
docker-login: ## to login to a container registry
	@$(INFO) Dockerd login to container registry ${DOCKER_REGISTRY}...
	@echo -n "${DOCKER_PASSWORD}" | $(DOCKER) login --password-stdin -u ${DOCKER_USER} $(DOCKER_REGISTRY) || ${FAIL}
	@$(OK) Dockerd login to container registry ${DOCKER_REGISTRY}...

go-build: $(GO_BUILD_PLATFORMS_ARTIFACTS) ## to build binaries

.PHONY: go-build
go-build/%:
	@$(INFO) go build $*...
	$(AT)target="$*"; \
	command="$${target%%-*}"; \
	platform_ext="$${target#*-}"; \
	platform="$${platform_ext%.*}"; \
	export GOOS="$${platform%%-*}"; \
	export GOARCH="$${platform#*-}"; \
	echo export GOOS=$${GOOS}; \
	echo export GOARCH=$${GOARCH}; \
	CGO_ENABLED=0 \
	$(GO) build ${GO_BUILD_OPTS} \
	-ldflags '${GO_LDFLAGS}' \
	-o ${GO_OUT_BIN_DIR}/$* \
	${CONFIG_APP_CODE} || ${FAIL}
	@$(OK) go build $*

.PHONY: go-run
go-run: ## to run locally for development
	@$(INFO) running locally...
	$(AT)$(GO) run ${GO_BUILD_OPTS} ${CONFIG_APP_CODE} || ${FAIL}
	@$(OK) running locally

.PHONY: go-test
go-test: ## to run tests
	@$(INFO) testing...
	$(AT)$(GO) test ${GO_TEST_OPTS} ./... || ${FAIL}
	@$(OK) testing

.PHONY: go-mod-check
go-mod-check: ## to check go mod files consistency
	@$(INFO) Checking go mod files consistency...
	$(AT)go mod tidy
	$(AT)git --no-pager diff --exit-code go.mod go.sum || \
	(${WARN} Please run "go mod tidy" and commit the changes in go.mod and go.sum. && ${FAIL} ; exit 128 )
	@$(OK) Checking go mod files consistency

.PHONY: go-update-dependencies
go-update-dependencies: ## to update go dependencies (vendor)
	@$(INFO) updating go dependencies...
	$(AT)$(GO) get -u ./... && \
	$(AT)$(GO) mod vendor && \
	$(AT)$(GO) mod tidy || ${FAIL}
	@$(OK) updating go dependencies

.PHONY: go-lint
go-lint: ## to lint go code
	@$(INFO) App linting...
	$(AT)GOCACHE="/tmp" docker run ${DOCKER_OPTS} \
	-v $(PWD):/app -w /app \
	-e GOCACHE="/tmp" \
	-e GOLANGCI_LINT_CACHE="/tmp" \
	${DOCKER_IMAGE_GOLINT} \
	golangci-lint run ./... || ${FAIL}
	@$(OK) App linting

.PHONY: go-fmt
go-fmt: ## to perform formatting
	@$(INFO) App code formatting...
	$(AT)go fmt ./... || ${FAIL}
	@$(OK) App code formatting...

.PHONY: go-doc
go-doc: ## to generate documentation
	@$(INFO) Generating Documentation...
	$(AT)$(GO) run ./scripts/env_config.go ./docs/env_config.md || ${FAIL}
	@$(OK) Generating Documentation

.PHONY: clean
clean: ## to clean-up
	@$(INFO) cleaning /${GO_OUT_BIN_DIR} folder...
	$(AT)rm -rf ${GO_OUT_BIN_DIR} || ${FAIL}
	@$(OK) cleaning /${GO_OUT_BIN_DIR} folder
