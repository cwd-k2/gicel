#!/usr/bin/env bash
# Build the gicel CLI binary with version metadata from git.
# Output: bin/gicel
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"

go build -o bin/gicel \
  -ldflags "-X main.version=${VERSION}" \
  ./cmd/gicel/
