## Check go mod files consistency
.PHONY: gomod-check
gomod-check:
	@echo Checking go mod files consistency
	go mod tidy -v && git --no-pager diff --exit-code go.mod go.sum || (echo "Please run \"go mod tidy\" and commit the changes in go.mod and go.sum." && exit 1)

.PHONY: check-style
check-style: gomod-check golangci-lint
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
	env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/rtcd -mod=readonly -trimpath ./cmd/rtcd

clean:
	rm -rf dist
