#!/usr/bin/env bash
# Ensure the "Bitrise AI QA Agent" RDE template exists in the workspace, then
# print its template ID on stdout (the only thing written to stdout — all
# progress goes to stderr, so callers can do: TEMPLATE_ID=$(ensure-template.sh <ws>)).
#
# Idempotent: reuses an existing template with the same name; otherwise creates
# one via the RDE API (https://api.bitrise.io/rde). The template clones
# github.com/bitrise-io/bitrise-ai-qa-agent and sources its warmup/startup
# scripts, matching the known-good live template.
set -euo pipefail

API="${BITRISE_RDE_API_BASE_URL:-https://api.bitrise.io/rde}"
TEMPLATE_NAME="${QA_TEMPLATE_NAME:-Bitrise AI QA Agent}"

WORKSPACE="${1:-${WORKSPACE:-}}"
if [ -z "$WORKSPACE" ]; then
  echo "usage: ensure-template.sh <workspace-id>   (or set WORKSPACE)" >&2
  exit 2
fi

PAT="${BITRISE_PAT:-}"
[ -n "$PAT" ] || PAT="$(cat "$HOME/.bitrise/pat" 2>/dev/null || true)"
if [ -z "$PAT" ]; then
  echo "no Bitrise PAT: set \$BITRISE_PAT or write ~/.bitrise/pat" >&2
  exit 2
fi

# 1. Reuse an existing template with the same name, if any.
existing="$(curl -fsS -m 30 -H "Authorization: $PAT" \
  "$API/v1/workspaces/$WORKSPACE/templates" \
  | TEMPLATE_NAME="$TEMPLATE_NAME" python3 -c '
import json, os, sys
name = os.environ["TEMPLATE_NAME"]
for t in json.load(sys.stdin).get("templates", []):
    if t.get("name") == name:
        print(t.get("id")); break
')"

if [ -n "$existing" ]; then
  echo "reusing existing template $existing (\"$TEMPLATE_NAME\")" >&2
  echo "$existing"
  exit 0
fi

# 2. Create it.
echo "no \"$TEMPLATE_NAME\" template found — creating one..." >&2

body="$(python3 - <<'PY'
import json
print(json.dumps({
    "name": "Bitrise AI QA Agent",
    "description": "Autonomous iOS QA agent: boots a macOS Simulator and lets a "
                   "headless Claude Code agent drive the uploaded app. Scripts: "
                   "github.com/bitrise-io/bitrise-ai-qa-agent.",
    "image": "osx-26-edge",
    "machineType": "g2.mac.m2pro.6c-14g",
    "warmupScript": "set -euo pipefail\n\ncd\n\n"
                    "git clone https://github.com/bitrise-io/bitrise-ai-qa-agent.git\n\n"
                    "source $HOME/bitrise-ai-qa-agent/template/warmup.sh\n",
    "startupScript": "set -euo pipefail\n\n"
                     "source $HOME/bitrise-ai-qa-agent/template/startup.sh\n",
    "sessionInputs": [
        {"key": "BITRISE_PAT", "required": True, "exposeAsEnvVar": True,
         "description": "Bitrise PAT exposed to the in-VM agent for result upload and callbacks."},
        {"key": "DEVICE_TYPE", "required": False, "exposeAsEnvVar": True, "defaultValue": "iPhone 15"},
        {"key": "IOS_VERSION", "required": False, "exposeAsEnvVar": True, "defaultValue": "26.2"},
        {"key": "XCODE_VERSION", "required": False, "exposeAsEnvVar": True, "defaultValue": "26.3"},
        {"key": "QA_WATCH_DIR", "required": False, "exposeAsEnvVar": True, "defaultValue": "/tmp/bitrise-ai-qa-agent"},
        {"key": "QA_WATCH_TIMEOUT_SEC", "required": False, "exposeAsEnvVar": True, "defaultValue": "1800"},
        {"key": "QA_WATCH_POLL_SEC", "required": False, "exposeAsEnvVar": True, "defaultValue": "2"},
    ],
}))
PY
)"

id="$(curl -fsS -m 60 -X POST \
  -H "Authorization: $PAT" -H "Content-Type: application/json" \
  -d "$body" "$API/v1/workspaces/$WORKSPACE/templates" \
  | python3 -c 'import json,sys; t=json.load(sys.stdin); print(t.get("template",{}).get("id") or t.get("id",""))')"

if [ -z "$id" ]; then
  echo "template creation failed (no id in response)" >&2
  exit 1
fi
echo "created template $id" >&2
echo "$id"
