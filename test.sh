#!/bin/zsh

# Requires a port-forward to the prod gRPC server in another terminal:
#   kubectl --context ip-prod -n internal-apps port-forward deploy/codespaces-api 9000:9000

# requires BITRISE_PAT

go run main.go \
    --endpoint localhost:9000 --insecure \
    session create \
    --workspace 26e1bb55524221dd \
    --template  f5195338-297a-4671-92bb-77b66726da94 \
    --name      "ai-qa-agent-cli-session-$(uuidgen | cut -c1-8)" \
    --upload    /tmp/hello-rde \
    --map-saved-inputs
