BINARY  := encapic
DIST    := dist
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"
OS      ?= $(shell go env GOOS)
ARCH    ?= $(shell go env GOARCH)

.PHONY: all build test lint fmt pre-commit clean install rpm rpm-package patch minor major _tag

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
	goreleaser release --snapshot --clean

# Bump helpers — read the latest semver tag and create the next one.
# If no tag exists yet, start from v0.0.0.
_LATEST := $(shell git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1)
_BASE   := $(if $(_LATEST),$(_LATEST),v0.0.0)
_MAJ    := $(shell echo $(_BASE) | sed 's/^v//' | cut -d. -f1)
_MIN    := $(shell echo $(_BASE) | sed 's/^v//' | cut -d. -f2)
_PAT    := $(shell echo $(_BASE) | sed 's/^v//' | cut -d. -f3)

patch:
	@NEW=v$(_MAJ).$(_MIN).$(shell expr $(_PAT) + 1); \
	git tag $$NEW && echo "Tagged $$NEW" && $(MAKE) _tag TAG=$$NEW

minor:
	@NEW=v$(_MAJ).$(shell expr $(_MIN) + 1).0; \
	git tag $$NEW && echo "Tagged $$NEW" && $(MAKE) _tag TAG=$$NEW

major:
	@NEW=v$(shell expr $(_MAJ) + 1).0.0; \
	git tag $$NEW && echo "Tagged $$NEW" && $(MAKE) _tag TAG=$$NEW

_tag:
	git push origin $(TAG)
