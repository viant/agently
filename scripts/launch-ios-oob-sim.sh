#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IOS_DIR="$ROOT_DIR/ios"
DERIVED_DATA_PATH="${DERIVED_DATA_PATH:-$IOS_DIR/.build/xcode-oob}"
SCHEME="${SCHEME:-AgentlyApp}"
BUNDLE_ID="${BUNDLE_ID:-com.viant.agently.ios}"
API_BASE_URL="${AGENTLY_API_BASE_URL:-http://127.0.0.1:9292}"
OOB_SECRET_REF="${AGENTLY_IOS_OOB_SECRET_REF:-}"
DESTINATION="${DESTINATION:-platform=iOS Simulator,name=iPad Pro 11-inch (M4)}"
ACTIVE_CONVERSATION_ID="${AGENTLY_IOS_ACTIVE_CONVERSATION_ID:-}"

if [[ -z "$OOB_SECRET_REF" ]]; then
  echo "AGENTLY_IOS_OOB_SECRET_REF is required" >&2
  exit 2
fi

# The hosted OOB endpoint runs on Linux and cannot resolve a path on the
# developer's Mac. Inline the encrypted resource for simulator builds so the
# server can decrypt it and return the normal persistent session cookie. The
# plaintext credential is never read or printed by this script.
inline_local_secret_ref() {
  local raw="$1"
  local resource_url="${raw%%|*}"
  local resource_key=""
  if [[ "$raw" == *"|"* ]]; then
    resource_key="${raw#*|}"
  fi
  case "$resource_url" in
    inlined://*|http://*|https://*|gcp://*|aws://*|s3://*|gs://*)
      printf '%s' "$raw"
      return
      ;;
  esac
  local local_path="$resource_url"
  if [[ "$local_path" == "~"* ]]; then
    local_path="$HOME${local_path:1}"
  elif [[ "$local_path" == file://* ]]; then
    local_path="${local_path#file://}"
  fi
  if [[ ! -f "$local_path" ]]; then
    echo "OOB encrypted resource was not found: $resource_url" >&2
    exit 2
  fi
  local encoded
  encoded="$(base64 < "$local_path" | tr -d '\r\n')"
  printf 'inlined://base64/%s' "$encoded"
  if [[ -n "$resource_key" ]]; then
    printf '|%s' "$resource_key"
  fi
}

OOB_SECRET_REF="$(inline_local_secret_ref "$OOB_SECRET_REF")"

xcodebuild \
  -project "$IOS_DIR/AgentlyApp.xcodeproj" \
  -scheme "$SCHEME" \
  -configuration Debug \
  -destination "$DESTINATION" \
  -derivedDataPath "$DERIVED_DATA_PATH" \
  build

APP_PATH="$(find "$DERIVED_DATA_PATH/Build/Products/Debug-iphonesimulator" -maxdepth 1 -name '*.app' -print -quit)"
if [[ -z "$APP_PATH" ]]; then
  echo "Could not find built .app under $DERIVED_DATA_PATH" >&2
  exit 3
fi

SIMULATOR_ID="${SIMULATOR_ID:-$(xcrun simctl list devices booted | awk -F '[()]' '/Booted/ {print $2; exit}')}"
if [[ -z "$SIMULATOR_ID" ]]; then
  echo "No booted iOS simulator found. Boot one in Simulator.app or set SIMULATOR_ID." >&2
  exit 4
fi

xcrun simctl install "$SIMULATOR_ID" "$APP_PATH"
LAUNCH_ARGUMENTS=(
  "--enableDevAuth=1"
  "--apiBaseURL=$API_BASE_URL"
  "--oobSecretReference=$OOB_SECRET_REF"
  "--autoOOBSignIn=1"
)
if [[ -n "$ACTIVE_CONVERSATION_ID" ]]; then
  LAUNCH_ARGUMENTS+=("--activeConversationID=$ACTIVE_CONVERSATION_ID")
fi

xcrun simctl launch \
  --terminate-running-process \
  "$SIMULATOR_ID" \
  "$BUNDLE_ID" \
  "${LAUNCH_ARGUMENTS[@]}"
