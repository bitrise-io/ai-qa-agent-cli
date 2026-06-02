#!/bin/zsh

# requires BITRISE_PAT
set -euo pipefail

WORKSPACE=26e1bb55524221dd
TEMPLATE=f5195338-297a-4671-92bb-77b66726da94

GOOS=darwin GOARCH=arm64 go build -o /tmp/hello-rde ./examples/hello

# `session create` prints the session id on stdout and the progress log
# (including the UI link) to stderr — capture stdout into SESSION_ID, tee
# stderr to a log so it both streams through and can be parsed for the link.
CREATE_LOG=$(mktemp)
SESSION_ID=$(go run main.go \
    session create \
    --workspace "$WORKSPACE" \
    --template  "$TEMPLATE" \
    --name      "ai-qa-agent-cli-session-$(uuidgen | cut -c1-8)" \
    --upload    /tmp/hello-rde \
    --map-saved-inputs \
    2> >(tee "$CREATE_LOG" >&2))

echo "session: $SESSION_ID"

# Surface the live session UI link parsed from the create log.
UI_LINK=$(grep -oE 'https://[^[:space:]]*dev-environments[^[:space:]]*' "$CREATE_LOG" | head -1)
[ -n "$UI_LINK" ] && echo "watch live: $UI_LINK"
rm -f "$CREATE_LOG"

# Wait for the QA run to finish, download results into ./qa-agent-results/<id>/,
# then stop the VM. --timeout 30m is plenty for a smoke test.
go run main.go \
    --timeout 30m \
    session collect "$SESSION_ID" \
    --workspace "$WORKSPACE"
