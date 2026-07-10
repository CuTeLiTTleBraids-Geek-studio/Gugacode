#!/usr/bin/env bash
# prompt-10 10-N: optional SBOM generation for release artifacts.
# Requires: https://github.com/anchore/syft (or docker image below)
set -euo pipefail
OUT="${1:-sbom.spdx.json}"
if command -v syft >/dev/null 2>&1; then
  syft dir:. -o spdx-json >"$OUT"
  echo "Wrote $OUT via local syft"
elif command -v docker >/dev/null 2>&1; then
  docker run --rm -v "$PWD:/src" anchore/syft:latest dir:/src -o spdx-json >"$OUT"
  echo "Wrote $OUT via docker syft"
else
  echo "syft not found. Install: https://github.com/anchore/syft#installation" >&2
  exit 1
fi
sha256sum "$OUT" 2>/dev/null || shasum -a 256 "$OUT" 2>/dev/null || true
