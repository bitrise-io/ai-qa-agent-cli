#!/usr/bin/env bash
set -euo pipefail

: "${workspace:?workspace input required}"
: "${template:?template input required}"
: "${upload:?upload input required}"
: "${bitrise_pat:?bitrise_pat input required}"

CLI_VERSION="${cli_version:-latest}"
TIMEOUT="${timeout:-30m}"
NAME="${name:-qa-agent-${BITRISE_BUILD_NUMBER:-local}-$(uuidgen | cut -c1-8)}"

if [ ! -e "$upload" ]; then
  echo "upload path does not exist: $upload" >&2
  exit 1
fi

export BITRISE_PAT="$bitrise_pat"

GOBIN="$(mktemp -d)"
export GOBIN
echo "installing github.com/bitrise-io/ai-qa-agent-cli@${CLI_VERSION}..."
go install "github.com/bitrise-io/ai-qa-agent-cli@${CLI_VERSION}"
CLI="$GOBIN/ai-qa-agent-cli"
if [ ! -x "$CLI" ]; then
  echo "go install completed but binary not found at $CLI" >&2
  ls -la "$GOBIN" >&2 || true
  exit 1
fi

WORKDIR="$(pwd)"
echo "uploading: $upload"

SESSION_ID=$("$CLI" session create \
  --workspace "$workspace" \
  --template  "$template" \
  --name      "$NAME" \
  --upload    "$upload" \
  --map-saved-inputs)

echo "session: $SESSION_ID"
envman add --key QA_SESSION_ID --value "$SESSION_ID"

"$CLI" --timeout "$TIMEOUT" session collect "$SESSION_ID" \
  --workspace "$workspace"

RESULTS="$WORKDIR/qa-agent-results/$SESSION_ID/results"
if [ ! -d "$RESULTS" ]; then
  echo "expected results dir not found: $RESULTS" >&2
  exit 1
fi

envman add --key QA_RESULTS_DIR --value "$RESULTS"
echo "results: $RESULTS"
