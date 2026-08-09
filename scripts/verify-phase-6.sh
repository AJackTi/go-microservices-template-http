#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"

echo "==> validating workflow YAML"
ruby -e 'require "yaml"; %w[.github/workflows/ci.yml .github/workflows/release.yml .github/dependabot.yml].each { |path| YAML.load_file(path) }'

echo "==> checking documentation files"
for path in \
  CONTRIBUTING.md \
  SECURITY.md \
  docs/observability-and-protocols.md \
  docs/release-process.md \
  docs/template-usage.md
do
  [[ -s "$path" ]]
done

echo "==> validating local source"
make verify-local

echo "==> Phase 6 verification passed"
