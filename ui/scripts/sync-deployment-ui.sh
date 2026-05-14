#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ui_dir="$(cd "${script_dir}/.." && pwd)"
repo_dir="$(cd "${ui_dir}/.." && pwd)"
dist_dir="${ui_dir}/dist"
deploy_dir="${repo_dir}/deployment/ui"

if [[ ! -d "${dist_dir}" ]]; then
  echo "missing dist directory: ${dist_dir}" >&2
  exit 1
fi

mkdir -p "${deploy_dir}"

# Keep Go embed sources in deployment/ui while refreshing the built bundle.
rsync -a --delete --exclude 'init.go' "${dist_dir}/" "${deploy_dir}/"

# Invalidate the Go package that embeds deployment/ui so the next `go build`
# cannot reuse a stale cached embed action after the bundle content changes.
touch "${deploy_dir}/init.go"
