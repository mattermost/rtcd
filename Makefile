DEFAULT_GOARCH := $(shell go env GOARCH)
BUILD_HASH = $(shell git rev-parse HEAD)
LDFLAGS += -X "github.com/mattermost/rtcd/service.buildHash=$(BUILD_HASH)"

## Check go mod files consistency
.PHONY: gomod-check
gomod-check:
	@echo Checking go mod files consistency
	go mod tidy -v && git --no-pager diff --exit-code go.mod go.sum || (echo "Please run \"go mod tidy\" and commit the changes in go.mod and go.sum." && exit 1)

.PHONY: check-style
check-style: golangci-lint gomod-check
	@echo Checking for style guide compliance

.PHONY: golangci-lint
golangci-lint: ## Run golangci-lint on codebase
# https://stackoverflow.com/a/677212/1027058 (check if a command exists or not)
	@if ! [ -x "$$(command -v golangci-lint)" ]; then \
		echo "golangci-lint is not installed. Please see https://github.com/golangci/golangci-lint#install for installation instructions."; \
		exit 1; \
	fi; \

	@echo Running golangci-lint
	golangci-lint run ./...

.PHONY: test
test:
	go test -v -mod=readonly -failfast -race ./...

.PHONY: build
build:
	mkdir -p dist
	env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/rtcd -ldflags '$(LDFLAGS)' -mod=readonly -trimpath ./cmd/rtcd

.PHONY: docker-build
docker-build:
	@if ! [ -x "$$(command -v docker)" ]; then \
		echo "Docker is not installed. Please see https://docs.docker.com/get-docker for installation instructions."; \
		exit 127; \
	fi; \

	@[ "${tag}" ] || ( echo "docker-build requires a tag. Set a tag with: \"make docker-build tag=rtcd:my_tag\""; exit 128 )

	docker build -f build/Dockerfile . -t $(tag)

.PHONY: doc
doc:
	go run ./scripts/env_config.go ./docs/env_config.md

.PHONY: clean
clean:
	rm -rf dist
