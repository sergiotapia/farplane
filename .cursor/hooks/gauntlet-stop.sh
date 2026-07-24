#!/usr/bin/env bash
# Cursor `stop` hook: run `make gauntlet` after a completed agent turn.
# On failure, emit followup_message so the agent fixes until green.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

SKIP_FILE="$ROOT/.cursor/skip-gauntlet"
STAMP_FILE="$ROOT/.cursor/.gauntlet-green"
LOG_FILE="$ROOT/.cursor/.gauntlet-last.log"
MAX_LOG_CHARS=12000

emit_json() {
  python3 -c 'import json,sys; print(json.dumps(json.loads(sys.stdin.read())))'
}

emit_empty() {
  printf '%s\n' '{}'
}

emit_followup() {
  local body="$1"
  LOOP_COUNT="${LOOP_COUNT:-0}" python3 -c '
import json, os, sys
body = sys.stdin.read()
loop = os.environ.get("LOOP_COUNT", "0")
msg = (
  "Stop hook: `make gauntlet` failed (auto-loop "
  + loop
  + "/8). Fix the failures below until `make gauntlet` exits 0, then stop. "
  "Do not claim done while the gauntlet is red. "
  "Escape hatch for one turn: `touch .cursor/skip-gauntlet`.\n\n"
  "----- make gauntlet output -----\n"
  + body
)
print(json.dumps({"followup_message": msg}))
' <<<"$body"
}

# stdin from Cursor
INPUT="$(cat || true)"
STATUS="$(
  printf '%s' "$INPUT" | python3 -c '
import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    data = {}
print(data.get("status") or "")
' 2>/dev/null || true
)"
LOOP_COUNT="$(
  printf '%s' "$INPUT" | python3 -c '
import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    data = {}
print(data.get("loop_count", 0))
' 2>/dev/null || echo 0
)"
export LOOP_COUNT

# Only continue a cleanly finished turn.
if [[ "$STATUS" != "completed" ]]; then
  emit_empty
  exit 0
fi

# One-shot escape hatch (self-clearing).
if [[ -f "$SKIP_FILE" ]]; then
  rm -f "$SKIP_FILE"
  emit_empty
  exit 0
fi

fingerprint() {
  git status --porcelain 2>/dev/null | sha256sum | awk '{print $1}'
}

FP="$(fingerprint)"

# No local changes → nothing to verify.
if [[ -z "$(git status --porcelain 2>/dev/null)" ]]; then
  emit_empty
  exit 0
fi

# Already green for this exact working-tree fingerprint.
if [[ -f "$STAMP_FILE" ]] && [[ "$(cat "$STAMP_FILE")" == "$FP" ]]; then
  emit_empty
  exit 0
fi

# Ensure mise-pinned tools are on PATH when available.
if command -v mise >/dev/null 2>&1; then
  # shellcheck disable=SC1090
  eval "$(mise activate bash)" 2>/dev/null || true
fi

set +e
make gauntlet >"$LOG_FILE" 2>&1
RC=$?
set -e

if [[ "$RC" -eq 0 ]]; then
  # Re-fingerprint after formatters may have rewritten files.
  fingerprint >"$STAMP_FILE"
  emit_empty
  exit 0
fi

# Prefer FAIL / error lines so Gin route dumps do not hide the real failure.
BODY="$(python3 -c '
import pathlib, re, sys
path = pathlib.Path(sys.argv[1])
text = path.read_text(errors="replace") if path.exists() else ""
max_chars = int(sys.argv[2])
lines = text.splitlines()
interesting = [
    ln for ln in lines
    if re.search(r"(?i)(--- FAIL|FAIL\t|Error:|panic:|migrate |FAIL\s*$|make: \*\*\*)", ln)
]
if interesting:
    body = "\n".join(interesting[-200:])
else:
    body = text
if len(body) > max_chars:
    body = "[... truncated ...]\n" + body[-max_chars:]
print(body, end="")
' "$LOG_FILE" "$MAX_LOG_CHARS")"

emit_followup "$BODY"
exit 0
