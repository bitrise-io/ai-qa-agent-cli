#!/bin/zsh

# requires BITRISE_PAT
set -euo pipefail

WORKSPACE=26e1bb55524221dd
TEMPLATE=f5195338-297a-4671-92bb-77b66726da94

GOOS=darwin GOARCH=arm64 go build -o /tmp/hello-rde ./examples/hello

# `session create` prints the session id on stdout and a follow-up hint to
# stderr — capture stdout into SESSION_ID and let stderr stream through.
SESSION_ID=$(go run main.go \
    session create \
    --workspace "$WORKSPACE" \
    --template  "$TEMPLATE" \
    --name      "ai-qa-agent-cli-session-$(uuidgen | cut -c1-8)" \
    --upload    /tmp/hello-rde \
    --map-saved-inputs)

echo "session: $SESSION_ID"

# Wait for the QA run to finish, download results into ./qa-agent-results/<id>/,
# then stop the VM. --timeout 30m is plenty for a smoke test.
go run main.go \
    --timeout 30m \
    session collect "$SESSION_ID" \
    --workspace "$WORKSPACE"
