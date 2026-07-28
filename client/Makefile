# Build the mkpk client + provisioning binaries with the version stamped in.
#
# VERSION resolves to the nearest git tag (e.g. v0.1.0), plus a -N-gSHA suffix
# and -dirty marker when the tree is ahead of / dirty against that tag. Untagged
# trees fall back to the short SHA. Override explicitly with `make VERSION=v1.2.3`.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X mikrotik-psk-knock/client/internal/version.Version=$(VERSION)
BIN := bin
PREFIX ?= /usr/local
MANDIR := ../docs/man

.PHONY: all build provision client desktop test version install clean

all: build

build: provision client

provision:
	@mkdir -p $(BIN)
	go build -ldflags "$(LDFLAGS)" -o $(BIN)/mkpk-provision ./cmd/mkpk-provision

client:
	@mkdir -p $(BIN)
	go build -ldflags "$(LDFLAGS)" -o $(BIN)/mkpk ./cmd/mkpk

# Desktop app (.app bundle) via the Wails CLI. Needs `wails` on PATH
# (go install github.com/wailsapp/wails/v2/cmd/wails@latest) and Xcode CLT.
# Output: cmd/mkpk-desktop/build/bin/mkpk-desktop.app
desktop:
	cd cmd/mkpk-desktop && wails build -s -skipbindings -ldflags "$(LDFLAGS)"

# Install the CLI binaries and man pages under PREFIX (default /usr/local).
install: build
	install -d "$(PREFIX)/bin" "$(PREFIX)/share/man/man1"
	install -m 0755 $(BIN)/mkpk $(BIN)/mkpk-provision "$(PREFIX)/bin/"
	install -m 0644 $(MANDIR)/mkpk.1 $(MANDIR)/mkpk-provision.1 "$(PREFIX)/share/man/man1/"

test:
	go test ./...

version:
	@echo $(VERSION)

clean:
	rm -rf $(BIN)
