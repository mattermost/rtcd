DEFAULT_GOARCH := $(shell go env GOARCH)
BUILD_HASH = $(shell git rev-parse HEAD)
LDFLAGS += -X "github.com/mattermost/rtcd/service.buildHash=$(BUILD_HASH)"

## Check go mod files consistency
.PHONY: gomod-check check-style golangci-lint test build clean
gomod-check:
	@echo Checking go mod files consistency
	go mod tidy -v && git --no-pager diff --exit-code go.mod go.sum || (echo "Please run \"go mod tidy\" and commit the changes in go.mod and go.sum." && exit 1)

check-style: golangci-lint gomod-check
	@echo Checking for style guide compliance

golangci-lint: ## Run golangci-lint on codebase
# https://stackoverflow.com/a/677212/1027058 (check if a command exists or not)
	@if ! [ -x "$$(command -v golangci-lint)" ]; then \
		echo "golangci-lint is not installed. Please see https://github.com/golangci/golangci-lint#install for installation instructions."; \
		exit 1; \
	fi; \

	@echo Running golangci-lint
	golangci-lint run ./...

test:
	go test -v -mod=readonly -failfast -race ./...

build:
	mkdir -p dist
	env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/rtcd -ldflags '$(LDFLAGS)' -mod=readonly -trimpath ./cmd/rtcd

clean:
	rm -rf dist
