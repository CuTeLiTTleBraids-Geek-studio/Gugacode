#!/usr/bin/env bash
# Install Go / Node / nfpm / wails3 inside WSL for offline packaging builds.
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
LOG="${HOME}/guga-toolchain.log"
exec > >(tee "$LOG") 2>&1

echo "=== install Go 1.25 ==="
if [ ! -x /usr/local/go/bin/go ]; then
  curl -fsSL "https://go.dev/dl/go1.25.0.linux-amd64.tar.gz" -o /tmp/go.tgz
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf /tmp/go.tgz
fi
export PATH="/usr/local/go/bin:${HOME}/go/bin:${PATH}"
go version

echo "=== install Node 20 ==="
if [ ! -x /usr/local/lib/nodejs/bin/node ]; then
  curl -fsSL "https://nodejs.org/dist/v20.19.2/node-v20.19.2-linux-x64.tar.xz" -o /tmp/node.tar.xz
  sudo mkdir -p /usr/local/lib/nodejs
  sudo tar -xJf /tmp/node.tar.xz -C /usr/local/lib/nodejs --strip-components=1
fi
export PATH="/usr/local/lib/nodejs/bin:${PATH}"
node -v
npm -v

echo "=== install nfpm ==="
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
nfpm --version

echo "=== install wails3 ==="
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha2.111 || true
command -v wails3 && wails3 version || echo "wails3 optional"

echo "TOOLCHAIN_OK"
