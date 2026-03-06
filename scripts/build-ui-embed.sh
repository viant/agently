#!/usr/bin/env bash
# Build the Agently UI and copy output into deployment/ui for embedding.
# Run from the agently repo root (github.com/viant/agently).
# After this, rebuild the Go binary: cd agently && go build -o agently .

set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if ! command -v npm >/dev/null 2>&1; then
  echo "Error: npm not found. Install Node/npm and try again." >&2
  exit 1
fi

echo "[build-ui-embed] Building UI (ui/)..."
(cd ui && npm run build)

DIST="${ROOT}/ui/dist"
DEPLOY="${ROOT}/deployment/ui"
if [ ! -d "$DIST" ]; then
  echo "Error: ui/dist not found after build." >&2
  exit 1
fi

echo "[build-ui-embed] Copying ui/dist/* to deployment/ui/..."
# Preserve init.go (Go embed directive) while replacing the web assets
find "${DEPLOY}" -maxdepth 1 -not -name 'init.go' -not -path "${DEPLOY}" -exec rm -rf {} +
rm -rf "${DEPLOY}/assets"
cp -R "$DIST"/* "$DEPLOY/"

echo "[build-ui-embed] Done. Rebuild the binary: cd agently && go build -o agently ."
