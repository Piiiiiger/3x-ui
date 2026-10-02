#!/usr/bin/env bash
set -euo pipefail
VERSION=${1:?panel version}
DEST=${2:?output directory}
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
mkdir -p "$DEST/$VERSION"
DEST=$(cd "$DEST/$VERSION" && pwd)
cd "$ROOT"
STAGE=$(mktemp -d)
cleanup() {
  rm -f "$STAGE/pigger-agent" "$STAGE/install.sh" "$STAGE/pigger-agent.service" "$STAGE/pigger-agent.openrc"
  rmdir "$STAGE"
}
trap cleanup EXIT
cp deploy/pigger-agent/install.sh deploy/pigger-agent/pigger-agent.service deploy/pigger-agent/pigger-agent.openrc "$STAGE/"
for ARCH in amd64 arm64; do
  CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$STAGE/pigger-agent" ./cmd/pigger-agent
  tar -czf "$DEST/linux-$ARCH.tar.gz" -C "$STAGE" pigger-agent install.sh pigger-agent.service pigger-agent.openrc
  SUM=$(sha256sum "$DEST/linux-$ARCH.tar.gz" | cut -d ' ' -f 1)
  printf '%s  archive.tar.gz\n' "$SUM" > "$DEST/linux-$ARCH.sha256"
done
