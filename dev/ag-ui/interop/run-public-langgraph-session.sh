#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
sdk_dir="$(cd -- "$script_dir/../../../../agently-core-ag-ui/sdk/ts" && pwd)"
cd -- "$sdk_dir"
exec node ./node_modules/vite-node/vite-node.mjs "$script_dir/public-langgraph-session.ts"
