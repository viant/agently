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

if [[ -z "$OOB_SECRET_REF" ]]; then
  echo "AGENTLY_IOS_OOB_SECRET_REF is required" >&2
  exit 2
fi

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
xcrun simctl launch \
  --terminate-running-process \
  "$SIMULATOR_ID" \
  "$BUNDLE_ID" \
  "--enableDevAuth=1" \
  "--apiBaseURL=$API_BASE_URL" \
  "--oobSecretReference=$OOB_SECRET_REF" \
  "--autoOOBSignIn=1"
