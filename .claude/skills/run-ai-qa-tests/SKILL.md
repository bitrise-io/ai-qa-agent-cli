---
name: run-ai-qa-tests
description: >-
  Run AI QA agent tests against an iOS app on Bitrise RDE. Ensures the
  "Bitrise AI QA Agent" RDE template exists (creates it via the RDE API at
  api.bitrise.io/rde if missing), then drives ai-qa-agent-cli to create a
  session, upload the app, wait for the in-VM Claude agent to test it, and
  download the results. Use when the user wants to QA-test an iOS .app/.ipa,
  run the AI QA agent, or set up the QA-agent RDE template.
---

# Run AI QA Agent tests

End-to-end: make sure the RDE template exists, then run a QA session with
`ai-qa-agent-cli`.

```
ensure-template.sh  ──▶  TEMPLATE_ID
        │
        ▼
ai-qa-agent-cli session create --upload <app>   ──▶  SESSION_ID
        │
        ▼
ai-qa-agent-cli session collect <SESSION_ID>     ──▶  ./qa-agent-results/<id>/
```

## Inputs to collect from the user

Before starting, make sure you have:

- **Workspace ID** — the Bitrise workspace UUID (e.g. `26e1bb55524221dd`). Ask
  if not provided.
- **App to test** — local path to a `.app` or `.ipa`. Ask if not provided.
- **Bitrise PAT** — read from `$BITRISE_PAT`, else `~/.bitrise/pat`. If neither
  is set, ask the user to export `BITRISE_PAT`.
- *(optional)* **QA prompt** — what the agent should test. Omit to use the
  CLI's built-in smoke test (install + launch + exercise the UI).
- *(optional)* device / iOS / Xcode versions.

## Step 1 — Ensure the template exists

Run the helper next to this file. It is idempotent: it reuses an existing
"Bitrise AI QA Agent" template, or creates one (image `osx-26-edge`, machine
`g2.mac.m2pro.6c-14g`, scripts sourced from
`github.com/bitrise-io/bitrise-ai-qa-agent`) via `POST /v1/workspaces/{ws}/templates`.

```sh
TEMPLATE_ID=$(bash "<skill-dir>/ensure-template.sh" "<workspace-id>")
```

`<skill-dir>` is the directory containing this SKILL.md. The script writes only
the template ID to stdout (progress goes to stderr), so the command substitution
above captures the ID cleanly. If it exits non-zero, surface the stderr message
(usually a missing PAT or workspace ID) and stop.

## Step 2 — Make sure the CLI is installed

If `ai-qa-agent-cli` is already on `PATH`, skip this. Otherwise install the
latest release binary with the install script — it detects the OS/arch,
verifies the checksum, and needs no Go toolchain:

```sh
command -v ai-qa-agent-cli >/dev/null || \
  curl -fsSL https://raw.githubusercontent.com/bitrise-io/ai-qa-agent-cli/main/install.sh | sh
```

The script installs to `/usr/local/bin` (or `~/.local/bin` as a fallback); if it
lands in `~/.local/bin`, make sure that's on `PATH`. Override with env vars:
`VERSION=v0.1.0` pins a release, `BIN_DIR=…` sets the destination.

Fallback (only if `curl`/the release isn't usable) — build from source with
Go ≥ 1.25: `go install github.com/bitrise-io/ai-qa-agent-cli@latest`.

## Step 3 — Run the test session

```sh
export BITRISE_PAT="<pat>"   # if not already exported

SESSION_ID=$(ai-qa-agent-cli session create \
  --workspace "<workspace-id>" \
  --template  "$TEMPLATE_ID" \
  --name      "qa-$(date +%s)" \
  --upload    "<path-to-app>")
  # optional: --qa-prompt "Log in with test/test and verify the home screen"
  # optional: --device-type "iPhone 15" --ios-version 26.2 --xcode-version 26.3

echo "session: $SESSION_ID"

ai-qa-agent-cli session collect "$SESSION_ID" --workspace "<workspace-id>"
# results land in ./qa-agent-results/<session-id>/ (junit.xml, screenshots, claude.log)
```

Report the results directory path and a short summary of `junit.xml` to the user.

## Step 4 (optional) — Publish results to the visualisation service

```sh
ai-qa-agent-cli upload-results ./qa-agent-results/<session-id>/
# prints a result URL
```

## Notes

- **`--upload-destination` must equal the template's `QA_WATCH_DIR`** (default
  `/tmp/bitrise-ai-qa-agent`). Leave both at the default unless you change one
  on purpose.
- Use **`session collect --no-stop`** to keep the VM alive for debugging
  (`tmux attach -t claude-auto` inside the VM). Otherwise the VM is stopped
  after collection; the template also auto-terminates after 60 min as a safety net.
- The API base URL defaults to `https://api.bitrise.io/rde`; override with
  `$BITRISE_RDE_API_BASE_URL`. Auth is the raw PAT in the `Authorization`
  header (no `Bearer` prefix).
- To force a differently-named template, set `$QA_TEMPLATE_NAME` before
  running the helper.
