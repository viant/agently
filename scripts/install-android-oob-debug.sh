#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ANDROID_DIR="$ROOT_DIR/android"
API_BASE_URL="${AGENTLY_ANDROID_BASE_URL:-http://10.0.2.2:9292}"
OOB_SECRET_REF="${AGENTLY_ANDROID_OOB_SECRET_REF:-}"
APP_ID="${APP_ID:-com.viant.agently.android}"
ANDROID_SDK_ROOT="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-$HOME/Library/Android/sdk}}"
ADB="${ADB:-$ANDROID_SDK_ROOT/platform-tools/adb}"

if [[ -z "$OOB_SECRET_REF" ]]; then
  echo "AGENTLY_ANDROID_OOB_SECRET_REF is required" >&2
  exit 2
fi

if [[ ! -x "$ADB" ]]; then
  ADB="$(command -v adb || true)"
fi
if [[ -z "$ADB" || ! -x "$ADB" ]]; then
  echo "adb was not found; set ADB or ANDROID_SDK_ROOT" >&2
  exit 2
fi

"$ANDROID_DIR/gradlew" \
  -p "$ANDROID_DIR" \
  :app:installDebug \
  "-Pagently.android.baseUrl=$API_BASE_URL" \
  "-Pagently.android.oobSecretRef=$OOB_SECRET_REF" \
  "-Pagently.android.autoOobSignIn=true"

"$ADB" shell am force-stop "$APP_ID" >/dev/null 2>&1 || true
"$ADB" shell am start -n "$APP_ID/.MainActivity"
