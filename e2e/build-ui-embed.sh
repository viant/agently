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

# Vite consumes the linked SDK sources directly. npm does not install a linked
# package's dependencies when installing the UI, so install them separately.
SDK_DIR="$(node -e '
  const path = require("path");
  const uiDir = process.argv[1];
  const dependency = require(path.join(uiDir, "package.json")).dependencies["agently-core-ui-sdk"];
  if (!dependency || !dependency.startsWith("file:")) {
    throw new Error("Expected a local file: dependency for agently-core-ui-sdk");
  }
  process.stdout.write(path.resolve(uiDir, dependency.slice(5)));
' "${UI_DIR}")"

if [ ! -f "${SDK_DIR}/package.json" ]; then
  echo "Error: local SDK missing at ${SDK_DIR}. Check out the repository referenced by ui/package.json." >&2
  exit 1
fi

install_deps() {
  local package_dir="$1"
  echo "[build-ui-embed] Installing deps in ${package_dir}..."
  if [ -f "${package_dir}/package-lock.json" ] || [ -f "${package_dir}/npm-shrinkwrap.json" ]; then
    (cd "${package_dir}" && npm ci --include=dev)
  else
    (cd "${package_dir}" && npm install --include=dev)
  fi
}

# Refresh even when node_modules exists: it may predate new dependencies.
install_deps "${SDK_DIR}"
install_deps "${UI_DIR}"

echo "[build-ui-embed] Building UI (${UI_DIR})..."
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
find "${DEPLOY}/assets" -maxdepth 1 -mindepth 1 -exec rm -rf {} +
find "$DIST" -maxdepth 1 -mindepth 1 ! -name 'assets' -exec cp -R {} "${DEPLOY}/" \;
cp -R "$DIST"/assets/. "${DEPLOY}/assets/"

echo "[build-ui-embed] Done. Rebuild the binary: cd agently && go build -o agently ."
