APP = peekd
ROOT ?= .
ADDR ?= :8090
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

run:
	go run . "$(ROOT)" -addr "$(ADDR)"

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o $(APP) .

wasm-build:
	sh wasm/build.sh

pages-build: wasm-build
	mkdir -p public/wasm
	cp -R docs/. public/
	cp wasm/index.html wasm/app.js wasm/worker.js wasm/styles.css wasm/peekd.wasm wasm/wasm_exec.js public/wasm/

test:
	go test -v ./...

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

EXCLUDE = --exclude "*webp" --exclude "*svg" --exclude "*gif" --exclude "saved-imgs"

webp:
	fd -t f $(EXCLUDE) --full-path './docs/images' --exec convert {} {.}.webp \;
	fd -t f $(EXCLUDE) --full-path './docs/images' --exec rm {} \;

.PHONY: build wasm-build pages-build test run fmt fmt-check vet check webp
