#!/bin/zsh

# Requires a port-forward to the prod gRPC server in another terminal:
#   kubectl --context ip-prod -n internal-apps port-forward deploy/codespaces-api 9000:9000

# requires BITRISE_PAT

go run main.go \
    --endpoint localhost:9000 --insecure \
    session create \
    --workspace 26e1bb55524221dd \
    --template  3da102f3-8fd2-444c-8e7f-4c0177049fac \
    --name      "ai-qa-agent-cli-session-$(uuidgen | cut -c1-8)" \
    --upload    /tmp/hello-rde \
    --upload-destination /tmp \
    --ai-prompt 'Wait until {{REMOTE_PATH}} exists (poll every 5 seconds for up to 5 minutes). When it appears, run it once with no arguments and paste its complete stdout back to me. Then exit.'
