# Build the mkpk client + provisioning binaries with the version stamped in.
#
# VERSION resolves to the nearest git tag (e.g. v0.1.0), plus a -N-gSHA suffix
# and -dirty marker when the tree is ahead of / dirty against that tag. Untagged
# trees fall back to the short SHA. Override explicitly with `make VERSION=v1.2.3`.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X mikrotik-psk-knock/client/internal/version.Version=$(VERSION)
BIN := bin

.PHONY: all build provision client test version clean

all: build

build: provision client

provision:
	@mkdir -p $(BIN)
	go build -ldflags "$(LDFLAGS)" -o $(BIN)/mkpk-provision ./cmd/mkpk-provision

client:
	@mkdir -p $(BIN)
	go build -ldflags "$(LDFLAGS)" -o $(BIN)/mkpk ./cmd/mkpk

test:
	go test ./...

version:
	@echo $(VERSION)

clean:
	rm -rf $(BIN)
