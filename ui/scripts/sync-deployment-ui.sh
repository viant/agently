#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UI_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_DIR="$(cd "${UI_DIR}/.." && pwd)"
DIST_DIR="${UI_DIR}/dist"
DEPLOY_DIR="${REPO_DIR}/deployment/ui"

if [[ ! -f "${DIST_DIR}/index.html" || ! -d "${DIST_DIR}/assets" ]]; then
  echo "UI dist is missing; run the Vite build before syncing." >&2
  exit 1
fi

mkdir -p "${DEPLOY_DIR}/assets"
rsync -a --delete "${DIST_DIR}/assets/" "${DEPLOY_DIR}/assets/"
cp "${DIST_DIR}/index.html" "${DEPLOY_DIR}/index.html"
if [[ -f "${DIST_DIR}/favicon.ico" ]]; then
  cp "${DIST_DIR}/favicon.ico" "${DEPLOY_DIR}/favicon.ico"
fi
