#!/usr/bin/env bash
# Build the Agently UI and copy output into the deployment bundle.
# Run from the agently repo root (github.com/viant/agently).
# After this, rebuild the Go binary: cd agently && go build -o agently .

set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if ! command -v npm >/dev/null 2>&1; then
  echo "Error: npm not found. Install Node/npm and try again." >&2
  exit 1
fi

UI_DIR="${ROOT}/ui"
DEPLOY="${ROOT}/deployment/ui"

echo "[build-ui-embed] Building UI (${UI_DIR})..."
if [ ! -d "${UI_DIR}/node_modules" ]; then
  echo "[build-ui-embed] Installing UI deps in ${UI_DIR}..."
  (cd "${UI_DIR}" && npm ci || npm install)
fi

(cd "${UI_DIR}" && npm run build)

DIST="${UI_DIR}/dist"
if [ ! -d "$DIST" ]; then
  echo "Error: ${DIST} not found after build." >&2
  exit 1
fi

echo "[build-ui-embed] Copying ${DIST}/* to ${DEPLOY}/..."
mkdir -p "${DEPLOY}"
find "${DEPLOY}" -maxdepth 1 \
  -not -name 'init.go' \
  -not -name 'assets' \
  -not -path "${DEPLOY}" \
  -exec rm -rf {} +
mkdir -p "${DEPLOY}/assets"
find "$DIST" -maxdepth 1 -mindepth 1 ! -name 'assets' -exec cp -R {} "${DEPLOY}/" \;
cp -R "$DIST"/assets/. "${DEPLOY}/assets/"

echo "[build-ui-embed] Done. Rebuild the binary: cd agently && go build -o agently ."
