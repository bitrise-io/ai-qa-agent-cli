#!/bin/zsh

# requires BITRISE_PAT

GOOS=darwin GOARCH=arm64 go build -o /tmp/hello-rde ./examples/hello

go run main.go \
    session create \
    --workspace 26e1bb55524221dd \
    --template  f5195338-297a-4671-92bb-77b66726da94 \
    --name      "ai-qa-agent-cli-session-$(uuidgen | cut -c1-8)" \
    --upload    /tmp/hello-rde \
    --map-saved-inputs
