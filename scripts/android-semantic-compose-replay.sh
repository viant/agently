#!/usr/bin/env bash
set -euo pipefail

ADB="${ADB:-adb}"
DEVICE="${ANDROID_SERIAL:-}"
INPUT_ID="${INPUT_ID:-new_conversation_composer_input}"
SEND_ID="${SEND_ID:-send_new_conversation}"
INPUT_ID_FALLBACK="${INPUT_ID_FALLBACK:-reply_composer_input}"
SEND_ID_FALLBACK="${SEND_ID_FALLBACK:-send_reply}"
INPUT_DESC="${INPUT_DESC:-Message}"
SEND_DESC="${SEND_DESC:-Send}"
INPUT_DESC_FALLBACK="${INPUT_DESC_FALLBACK:-Ask anything}"
SEND_DESC_FALLBACK="${SEND_DESC_FALLBACK:-Send}"
PROMPT="${PROMPT:-}"
EXPECT="${EXPECT:-}"
WAIT_SECONDS="${WAIT_SECONDS:-20}"
SELF_TEST=false
CLEAR_INPUT="${CLEAR_INPUT:-true}"
CLEAR_CHARS="${CLEAR_CHARS:-160}"

usage() {
  cat <<'USAGE'
Usage:
  android-semantic-compose-replay.sh --prompt "open report builder" [options]

Options:
  --device SERIAL        adb device serial. Defaults to ANDROID_SERIAL or adb default.
  --input-desc TEXT      Composer input content description/text fallback.
                         Default: Message
  --input-id TEXT        Composer input resource-id/test-tag.
                         Default: new_conversation_composer_input
  --send-desc TEXT       Send button content description/text fallback.
                         Default: Send
  --send-id TEXT         Send button resource-id/test-tag.
                         Default: send_new_conversation
  --input-desc-fallback TEXT
                         Fallback composer input content description/text.
                         Default: Ask anything
  --input-id-fallback TEXT
                         Fallback composer input resource-id/test-tag.
                         Default: reply_composer_input
  --send-desc-fallback TEXT
                         Fallback send button content description/text.
                         Default: Send
  --send-id-fallback TEXT
                         Fallback send button resource-id/test-tag.
                         Default: send_reply
  --expect TEXT          Optional text/content-desc substring to verify after send.
  --wait SECONDS         Seconds to wait after send before verification. Default: 20
  --no-clear             Do not clear the focused composer before typing.
  --clear-chars COUNT    Delete-key count used when clearing. Default: 160
  --self-test            Run parser self-tests without adb.

Environment:
  ADB                    adb executable path. Default: adb
  ANDROID_SERIAL         default device serial

Examples:
  ADB="$HOME/Library/Android/sdk/platform-tools/adb" \
    ./scripts/android-semantic-compose-replay.sh \
    --device emulator-5554 \
    --prompt "open report builder" \
    --expect "Performance Metrics"
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --device)
      DEVICE="${2:-}"
      shift 2
      ;;
    --input-desc)
      INPUT_DESC="${2:-}"
      shift 2
      ;;
    --input-id)
      INPUT_ID="${2:-}"
      shift 2
      ;;
    --send-desc)
      SEND_DESC="${2:-}"
      shift 2
      ;;
    --send-id)
      SEND_ID="${2:-}"
      shift 2
      ;;
    --input-desc-fallback)
      INPUT_DESC_FALLBACK="${2:-}"
      shift 2
      ;;
    --input-id-fallback)
      INPUT_ID_FALLBACK="${2:-}"
      shift 2
      ;;
    --send-desc-fallback)
      SEND_DESC_FALLBACK="${2:-}"
      shift 2
      ;;
    --send-id-fallback)
      SEND_ID_FALLBACK="${2:-}"
      shift 2
      ;;
    --prompt)
      PROMPT="${2:-}"
      shift 2
      ;;
    --expect)
      EXPECT="${2:-}"
      shift 2
      ;;
    --wait)
      WAIT_SECONDS="${2:-}"
      shift 2
      ;;
    --no-clear)
      CLEAR_INPUT=false
      shift
      ;;
    --clear-chars)
      CLEAR_CHARS="${2:-}"
      shift 2
      ;;
    --self-test)
      SELF_TEST=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "$SELF_TEST" != true && -z "$PROMPT" ]]; then
  echo "--prompt is required" >&2
  usage >&2
  exit 2
fi

adb_cmd() {
  if [[ -n "$DEVICE" ]]; then
    "$ADB" -s "$DEVICE" "$@"
  else
    "$ADB" "$@"
  fi
}

adb_shell() {
  adb_cmd shell "$@"
}

dump_ui() {
  adb_cmd exec-out uiautomator dump /dev/tty 2>/dev/null
}

xml_escape() {
  local value="$1"
  value="${value//&/&amp;}"
  value="${value//\"/&quot;}"
  value="${value//</&lt;}"
  value="${value//>/&gt;}"
  printf '%s' "$value"
}

find_semantic_bounds() {
  local desc="$1"
  local escaped
  escaped="$(xml_escape "$desc")"
  AGENTLY_TARGET_TEXT="$escaped" perl -0ne '
      my $text = $ENV{"AGENTLY_TARGET_TEXT"};
      while (/<node\b[^>]*(?:content-desc|text)="\Q$text\E"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"/g) {
        print "$1,$2,$3,$4\n";
        exit 0;
      }
    '
}

find_resource_bounds() {
  local resource_id="$1"
  local escaped
  escaped="$(xml_escape "$resource_id")"
  AGENTLY_TARGET_TEXT="$escaped" perl -0ne '
      my $text = $ENV{"AGENTLY_TARGET_TEXT"};
      while (/<node\b[^>]*resource-id="[^"]*\Q$text\E"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"/g) {
        print "$1,$2,$3,$4\n";
        exit 0;
      }
    '
}

tap_content_desc() {
  local desc="$1"
  local bounds
  bounds="$(dump_ui | find_semantic_bounds "$desc")"
  if [[ -z "$bounds" ]]; then
    echo "semantic label not found: $desc" >&2
    return 1
  fi
  IFS=',' read -r left top right bottom <<<"$bounds"
  local x=$(((left + right) / 2))
  local y=$(((top + bottom) / 2))
  adb_shell input tap "$x" "$y"
}

tap_resource_id() {
  local resource_id="$1"
  local bounds
  bounds="$(dump_ui | find_resource_bounds "$resource_id")"
  if [[ -z "$bounds" ]]; then
    echo "resource-id not found: $resource_id" >&2
    return 1
  fi
  IFS=',' read -r left top right bottom <<<"$bounds"
  local x=$(((left + right) / 2))
  local y=$(((top + bottom) / 2))
  adb_shell input tap "$x" "$y"
}

tap_first_target() {
  local target
  for target in "$@"; do
    if [[ -z "$target" ]]; then
      continue
    fi
    case "$target" in
      id:*)
        if tap_resource_id "${target#id:}" >/dev/null 2>&1; then
          printf '%s\n' "$target"
          return 0
        fi
        ;;
      desc:*)
        if tap_content_desc "${target#desc:}" >/dev/null 2>&1; then
          printf '%s\n' "$target"
          return 0
        fi
        ;;
    esac
  done
  echo "target not found: $*" >&2
  return 1
}

tap_first_content_desc() {
  local desc
  for desc in "$@"; do
    if [[ -n "$desc" ]] && tap_content_desc "$desc" >/dev/null 2>&1; then
      printf '%s\n' "$desc"
      return 0
    fi
  done
  echo "content-desc not found: $*" >&2
  return 1
}

clear_focused_text() {
  if [[ "$CLEAR_INPUT" != true ]]; then
    return 0
  fi
  if ! [[ "$CLEAR_CHARS" =~ ^[0-9]+$ ]]; then
    echo "--clear-chars must be a non-negative integer" >&2
    exit 2
  fi
  adb_shell input keyevent 123
  local i
  for ((i = 0; i < CLEAR_CHARS; i++)); do
    adb_shell input keyevent 67
  done
}

self_test() {
  local sample
  sample='<hierarchy><node content-desc="New conversation composer input" bounds="[10,20][110,220]" /><node content-desc="Send new conversation" bounds="[200,300][260,360]" /><node content-desc="Generic &amp; Builder" bounds="[1,2][3,4]" /></hierarchy>'
  local input_bounds send_bounds escaped_bounds
  local resource_bounds
  input_bounds="$(printf '%s' "$sample" | find_semantic_bounds "New conversation composer input")"
  send_bounds="$(printf '%s' "$sample" | find_semantic_bounds "Send new conversation")"
  escaped_bounds="$(printf '%s' "$sample" | find_semantic_bounds "Generic & Builder")"
  resource_bounds="$(printf '%s' '<hierarchy><node resource-id="com.viant.agently.android:id/new_conversation_composer_input" bounds="[30,40][130,240]" /></hierarchy>' | find_resource_bounds "new_conversation_composer_input")"
  [[ "$input_bounds" == "10,20,110,220" ]] || {
    echo "self-test failed: input bounds were '$input_bounds'" >&2
    exit 1
  }
  [[ "$send_bounds" == "200,300,260,360" ]] || {
    echo "self-test failed: send bounds were '$send_bounds'" >&2
    exit 1
  }
  [[ "$escaped_bounds" == "1,2,3,4" ]] || {
    echo "self-test failed: escaped bounds were '$escaped_bounds'" >&2
    exit 1
  }
  [[ "$resource_bounds" == "30,40,130,240" ]] || {
    echo "self-test failed: resource bounds were '$resource_bounds'" >&2
    exit 1
  }
  echo "self-test passed"
}

type_text() {
  local text="$1"
  local word
  for word in $text; do
    adb_shell input text "$word"
    adb_shell input keyevent 62
  done
}

if [[ "$SELF_TEST" == true ]]; then
  self_test
  exit 0
fi

echo "target device: ${DEVICE:-adb default}"
echo "input: ${INPUT_ID:-$INPUT_DESC}"
echo "send: ${SEND_ID:-$SEND_DESC}"

USED_INPUT_TARGET="$(tap_first_target "id:$INPUT_ID" "id:$INPUT_ID_FALLBACK" "desc:$INPUT_DESC" "desc:$INPUT_DESC_FALLBACK")"
sleep 0.3
clear_focused_text
sleep 0.1
type_text "$PROMPT"
sleep 0.3

# Dismiss the soft keyboard before resolving Send. This prevents keyboard
# suggestions from consuming the tap instead of the composer action.
adb_shell input keyevent 111 >/dev/null 2>&1 || true
sleep 0.8
if [[ "$USED_INPUT_TARGET" == "id:$INPUT_ID_FALLBACK" || "$USED_INPUT_TARGET" == "desc:$INPUT_DESC_FALLBACK" ]]; then
  tap_first_target "id:$SEND_ID_FALLBACK" "desc:$SEND_DESC_FALLBACK" "id:$SEND_ID" "desc:$SEND_DESC" >/dev/null
else
  tap_first_target "id:$SEND_ID" "desc:$SEND_DESC" "id:$SEND_ID_FALLBACK" "desc:$SEND_DESC_FALLBACK" >/dev/null
fi

if [[ "$WAIT_SECONDS" =~ ^[0-9]+$ ]] && [[ "$WAIT_SECONDS" -gt 0 ]]; then
  sleep "$WAIT_SECONDS"
fi

if [[ -n "$EXPECT" ]]; then
  if dump_ui | grep -Fq "$EXPECT"; then
    echo "verified: $EXPECT"
  else
    echo "expected text/content-desc not found after send: $EXPECT" >&2
    exit 1
  fi
fi

echo "done"
