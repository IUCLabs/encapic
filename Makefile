BINARY  := encapic
DIST    := dist
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"
OS      ?= $(shell go env GOOS)
ARCH    ?= $(shell go env GOARCH)

.PHONY: all build test lint fmt pre-commit clean install rpm rpm-package patch minor major

all: build

# Build the single static binary into dist/.
build:
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build $(GOFLAGS) -o $(DIST)/$(BINARY) .

test:
	go test -v -race ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

pre-commit:
	uvx pre-commit run --all-files

clean:
	rm -rf $(DIST) $(BINARY)

install:
	go install $(GOFLAGS) .

# Build every release artifact locally without publishing: the raw binary the
# puppet compilers fetch, and the RPM the VM masters install.
rpm: rpm-package

# goreleaser drives the build and the nfpm packaging in one pass, so packaging
# an already-built binary is not a separate step.
rpm-package:
	GOTOOLCHAIN=auto go run github.com/goreleaser/goreleaser/v2@v2.20.2 release --snapshot --clean --skip=sign

DEFAULT_BRANCH ?= main

# Draft the release with generated notes, then push the tag; the release
# workflow attaches assets to the draft and publishes it.
patch minor major:
	@git fetch --quiet --tags origin $(DEFAULT_BRANCH)
	@test "$$(git branch --show-current)" = "$(DEFAULT_BRANCH)" || { echo "ERROR: not on $(DEFAULT_BRANCH)"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "ERROR: working tree not clean"; exit 1; }
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/$(DEFAULT_BRANCH))" || { echo "ERROR: HEAD is not origin/$(DEFAULT_BRANCH)"; exit 1; }
	@LATEST=$$(git tag --list 'v*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1); \
	NEW=$$(echo "$${LATEST:-v0.0.0}" | sed 's/^v//' | awk -F. -v b=$@ '{ \
		if (b == "major") print "v" ($$1+1) ".0.0"; \
		else if (b == "minor") print "v" $$1 "." ($$2+1) ".0"; \
		else print "v" $$1 "." $$2 "." ($$3+1) }'); \
	SHA=$$(git rev-parse HEAD); \
	gh release create "$$NEW" --draft --target "$$SHA" --generate-notes --title "$$NEW" || exit 1; \
	git push origin "$$SHA:refs/tags/$$NEW" || { gh release delete "$$NEW" --yes; exit 1; }
