#!/bin/zsh

# Talks to the production codespaces REST API by default. For local dev,
# port-forward the API HTTP port and point --endpoint at it:
#   kubectl --context ip-prod -n internal-apps port-forward deploy/codespaces-api 8081:8080
#   ./test.sh --endpoint http://localhost:8081

# requires BITRISE_PAT

GOOS=darwin GOARCH=arm64 go build -o /tmp/hello-rde ./examples/hello

go run main.go \
    session create \
    --workspace 26e1bb55524221dd \
    --template  f5195338-297a-4671-92bb-77b66726da94 \
    --name      "ai-qa-agent-cli-session-$(uuidgen | cut -c1-8)" \
    --upload    /tmp/hello-rde \
    --map-saved-inputs
