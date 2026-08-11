#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ANDROID_DIR="$ROOT_DIR/android"
API_BASE_URL="${AGENTLY_ANDROID_BASE_URL:-http://10.0.2.2:9292}"
OOB_SECRET_REF="${AGENTLY_ANDROID_OOB_SECRET_REF:-}"
OAUTH_CONFIG_URL="${AGENTLY_ANDROID_OAUTH_CONFIG_URL:-}"
APP_ID="${APP_ID:-com.viant.agently.android}"
ANDROID_SDK_ROOT="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-$HOME/Library/Android/sdk}}"
ADB="${ADB:-$ANDROID_SDK_ROOT/platform-tools/adb}"
ANDROID_SERIAL="${ANDROID_SERIAL:-}"

if [[ -z "$OOB_SECRET_REF" ]]; then
  echo "AGENTLY_ANDROID_OOB_SECRET_REF is required" >&2
  exit 2
fi
if [[ -z "$OAUTH_CONFIG_URL" ]]; then
  echo "AGENTLY_ANDROID_OAUTH_CONFIG_URL is required" >&2
  exit 2
fi

# A hosted Agently server cannot resolve a file on the developer's Mac. Embed
# only the encrypted resource in debug builds; the server still performs the
# normal decryption and returns the persistent session cookie. This preserves
# OOB auth when the same APK is pointed at either localhost or a hosted server.
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

if [[ ! -x "$ADB" ]]; then
  ADB="$(command -v adb || true)"
fi
if [[ -z "$ADB" || ! -x "$ADB" ]]; then
  echo "adb was not found; set ADB or ANDROID_SDK_ROOT" >&2
  exit 2
fi

"$ANDROID_DIR/gradlew" \
  -p "$ANDROID_DIR" \
  clean \
  :app:assembleDebug \
  "-Pagently.android.baseUrl=$API_BASE_URL" \
  "-Pagently.android.oauthConfigUrl=$OAUTH_CONFIG_URL" \
  "-Pagently.android.oobSecretRef=$OOB_SECRET_REF" \
  "-Pagently.android.autoOobSignIn=true"

ADB_TARGET_ARGS=()
if [[ -n "$ANDROID_SERIAL" ]]; then
  ADB_TARGET_ARGS=(-s "$ANDROID_SERIAL")
fi

APK_PATH="$ANDROID_DIR/app/build/outputs/apk/debug/app-debug.apk"
"$ADB" "${ADB_TARGET_ARGS[@]}" install -r "$APK_PATH"
"$ADB" "${ADB_TARGET_ARGS[@]}" shell am force-stop "$APP_ID" >/dev/null 2>&1 || true
"$ADB" "${ADB_TARGET_ARGS[@]}" shell am start -n "$APP_ID/.MainActivity"
