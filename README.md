# ai-qa-agent-cli

CLI for driving **Bitrise Remote Development Environment (RDE)** QA agent
sessions. It creates a session from a QA-agent RDE template, uploads an iOS
app, waits while a headless Claude Code agent inside the VM drives the iOS
Simulator, then downloads the results (screenshots, JUnit XML, logs).

The same binary also ships the in-VM pieces:
- `mcp` — an MCP server exposing screenshot/click/type/scroll/drag tools that
  the in-VM agent uses to drive the Simulator locally.
- `upload-results` — packs a results folder and posts it to the
  `bitrise-rde-qa-results` visualisation service.

## How it works

```
ai-qa-agent-cli session create
  └─ Codespaces API → provisions an RDE VM from your QA-agent template
       warmup.sh (one-time setup) → startup.sh (forks the upload watcher)
  └─ uploads the app → QA_WATCH_DIR
       watcher detects the upload → launches Claude in a tmux session
       Claude boots the sim, installs + launches the app, drives the UI
       via the qa-agent MCP tools → writes ~/.qa-agent/results/
ai-qa-agent-cli session collect <session-id>
  └─ waits for the agent to go idle → downloads results → stops the VM
```

The RDE template that the VM boots from lives in a separate repo:
[`bitrise-io/bitrise-ai-qa-agent`](https://github.com/bitrise-io/bitrise-ai-qa-agent).

## Prerequisites

### 1. Set up the RDE QA-agent template (do this first)

The CLI does **not** work on its own — it launches sessions from an RDE
template that must already exist in your Bitrise workspace. Follow the setup
in [`bitrise-io/bitrise-ai-qa-agent`](https://github.com/bitrise-io/bitrise-ai-qa-agent),
which covers the `warmup.sh` / `startup.sh` scripts and the required
configuration. In short, the template needs:

- A **macOS RDE image** with Xcode + an iOS Simulator runtime, Go ≥ 1.25,
  `tmux`, and `python3` available.
- **Template variables** (set once by the template author):
  - `BITRISE_TOKEN` (secret) — PAT the in-VM agent uses to call back to
    codespaces for this session.
  - `BITRISE_WORKSPACE_ID` — workspace slug for the same purpose.
- **Anthropic credentials** — `ANTHROPIC_API_KEY` or
  `CLAUDE_CODE_OAUTH_TOKEN`, so the backend installs and authenticates
  Claude Code.
- **Session inputs** exposed as env vars (`QA_PROMPT`, `DEVICE_TYPE`,
  `IOS_VERSION`, `QA_WATCH_DIR`, …) — see the template repo for the full table.

Once the template exists, note its **template ID** and your **workspace ID** —
you'll pass both to `session create`.

### 2. A Bitrise personal access token

Export a PAT the CLI uses to authenticate against the Bitrise API:

```sh
export BITRISE_PAT="<your-bitrise-pat>"
```

## Installation

### Option A — install script (recommended)

Downloads the latest release binary for your platform, verifies its checksum,
and installs it:

```sh
curl -fsSL https://raw.githubusercontent.com/bitrise-io/ai-qa-agent-cli/main/install.sh | sh
```

Override the version or destination with env vars:

```sh
curl -fsSL https://raw.githubusercontent.com/bitrise-io/ai-qa-agent-cli/main/install.sh | VERSION=v0.1.0 BIN_DIR="$HOME/.local/bin" sh
```

### Option B — download the archive manually

Download the archive for your platform from the
[latest release](https://github.com/bitrise-io/ai-qa-agent-cli/releases/latest),
verify the checksum, and put the binary on your `PATH`:

```sh
VERSION=v0.1.0
OS=$(uname -s | tr 'A-Z' 'a-z')      # darwin or linux
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

curl -sSL -O "https://github.com/bitrise-io/ai-qa-agent-cli/releases/download/${VERSION}/ai-qa-agent-cli_${VERSION}_${OS}_${ARCH}.tar.gz"
curl -sSL -O "https://github.com/bitrise-io/ai-qa-agent-cli/releases/download/${VERSION}/SHA256SUMS"
shasum -a 256 -c SHA256SUMS --ignore-missing

tar -xzf "ai-qa-agent-cli_${VERSION}_${OS}_${ARCH}.tar.gz"
sudo mv ai-qa-agent-cli /usr/local/bin/
```

Prebuilt binaries are published for `darwin/arm64`, `darwin/amd64`,
`linux/amd64`, and `linux/arm64`.

### Option C — `go install`

Requires Go ≥ 1.25:

```sh
go install github.com/bitrise-io/ai-qa-agent-cli@v0.1.0   # or @latest
```

This installs to `$(go env GOPATH)/bin` — make sure that's on your `PATH`.

## Quick start

```sh
export BITRISE_PAT="<your-bitrise-pat>"

# Create a session, upload the app, and wait for the agent to start.
SESSION_ID=$(ai-qa-agent-cli session create \
  --workspace "<workspace-id>" \
  --template  "<template-id>" \
  --name      "smoke-test" \
  --upload    ./MyApp.app)

echo "session: $SESSION_ID"

# Wait for the agent to finish, download results, and stop the VM.
ai-qa-agent-cli session collect "$SESSION_ID" --workspace "<workspace-id>"
# → results land in ./qa-agent-results/<session-id>/
```

Pass a custom test brief with `--qa-prompt` (or `--qa-prompt-file`); omit it to
use the built-in smoke test (install + launch the app and exercise its UI).

## Commands

| Command | Purpose |
|---|---|
| `session create` | Create a session from a template, upload the app, wait for the agent to launch. Prints the session ID. |
| `session collect <session-id>` | Wait for the agent to go idle, download `~/.qa-agent/` results, stop the VM. `--no-stop` keeps the VM for debugging; `--no-wait` downloads immediately. |
| `upload-results <dir>` | Pack a results folder and POST it to the QA results visualisation service. Prints the result URL. |
| `mcp` | Run the in-VM MCP server (stdio). Registered automatically by the template's `warmup.sh`; you normally don't run this by hand. |

Run any command with `--help` for the full flag list. Notable `session create`
flags:

- `--qa-prompt` / `--qa-prompt-file` — the agent's task (a `{{REMOTE_PATH}}`
  placeholder is replaced with the uploaded app's remote path).
- `--device-type` (default `iPhone 15`), `--ios-version`, `--xcode-version`
  (default `26.3`) — simulator/toolchain selection.
- `--upload-destination` (default `/tmp/bitrise-ai-qa-agent`) — must match the
  template's `QA_WATCH_DIR`.
- `--auto-terminate-minutes` (default `60`) — safety net that frees the VM if
  the CLI crashes.

## Configuration (environment variables)

| Variable | Default | Purpose |
|---|---|---|
| `BITRISE_PAT` | — (required) | Bitrise PAT for API auth. |
| `BITRISE_CODESPACES_API_BASE_URL` | `https://codespaces-api.services.bitrise.io` | Codespaces REST API base URL (`--endpoint`). |
| `BITRISE_DEV_ENVIRONMENTS_UI_BASE_URL` | `https://app.bitrise.io/dev-environments` | Used to print a session-detail link after create (`--ui-base-url`). |
| `BITRISE_RDE_QA_RESULTS_URL` | `https://rde-qa-results.services.bitrise.dev/api/results` | `upload-results` endpoint (`--upload-url`). |

## Use as a Bitrise Step

This repo is also packaged as a Bitrise step (`step.yml` / `step.sh`) that runs
`session create` + `session collect` end to end. Key inputs:

| Input | Required | Default | Notes |
|---|---|---|---|
| `workspace` | yes | — | Workspace ID. |
| `template` | yes | — | RDE template ID. |
| `upload` | yes | — | Path to the `.app` / `.ipa` to test. |
| `bitrise_pat` | yes | `$BITRISE_PAT` | Sensitive. |
| `name` | no | `qa-agent-<build>-<rand>` | Session name. |
| `timeout` | no | `30m` | Max wait for the session to finish. |
| `cli_version` | no | `latest` | Module version to `go install` (e.g. `v0.1.0`). |

Outputs: `QA_SESSION_ID`, `QA_RESULTS_DIR` (contains `junit.xml`, screenshots,
`claude.log`).

> The step installs the CLI via `go install`, so Go ≥ 1.25 must be on `PATH`
> (it is, on Bitrise's macOS stacks). For reproducible builds, pin
> `cli_version` to a tagged release instead of `latest`.

## Development

```sh
go build ./...      # build
go vet ./...        # vet
go test ./...       # tests
```

Releases are automated: pushing a `v*` tag triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml), which
cross-compiles the four target platforms and attaches the archives +
`SHA256SUMS` to the GitHub release.
