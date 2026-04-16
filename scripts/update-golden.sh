#!/usr/bin/env bash
# update-golden.sh — regenerate golden files for examples/gicel/**/*.gicel.
# Run this after intentional output-format changes.
# Usage: ./scripts/update-golden.sh

set -euo pipefail

cd "$(dirname "$0")/.."

echo "Regenerating golden files for all GICEL examples..."
go test ./tests/e2e/ -update
echo "Done. Review the diff with: git diff examples/gicel/"
