APP = peekd
ROOT ?= .
ADDR ?= :8090
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o $(APP) .

test:
	go test -v ./...

run:
	go run . -root "$(ROOT)" -addr "$(ADDR)"

fmt:
	gofmt -w $$(git ls-files '*.go')

fmt-check:
	@if [ -n "$$(git status --porcelain -s)" ]; then \
		echo "Working directory is not clean. Please commit or stash changes before running fmt-check."; \
		exit 1; \
	fi
	@$(MAKE) fmt
	@if [ -z "$$(git status --porcelain)" ]; then \
		echo "All files are formatted correctly."; \
	else \
		echo "The following files were not formatted. Please run 'make fmt' and commit the changes:"; \
		git status --porcelain; \
		exit 1; \
	fi

vet:
	go vet ./...

check: fmt-check vet test

.PHONY: build test run fmt fmt-check vet check
