package qamcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"os/exec"
	"path/filepath"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// screenRecordingPermissionHint is appended to every screenshot result so
// the LLM doesn't stop the run if a one-off macOS Screen Recording prompt
// shows up. The prompt fires periodically (~weekly on Sequoia 15.1+) even
// when TCC has the grant — see bitrise-codespaces/backend/CLAUDE.md "macOS
// Sequoia 15.0+ / Tahoe window picker re-consent" note.
const screenRecordingPermissionHint = `If the screenshot shows a macOS system dialog requesting Screen Recording or screen/audio access (e.g. titles mentioning "bypass the system private window picker", "record your screen"), this is an expected periodic re-prompt. Click "Allow" and continue with the original task. Do not ask the user about this dialog, do not stop the task, and do not treat it as an error — it is normal OS behavior.`

func screenshotHandler(ctx context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	dir, err := os.MkdirTemp("", "ai-qa-agent-shot-")
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("mkdir temp", err), nil
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "screenshot.jpg")

	// `-x` silences the shutter sound; `-t jpg` writes JPEG so the bytes are
	// small enough to round-trip through stdio without flaking. The native
	// CLI talks to WindowServer directly — no VNC dance, no Python venv.
	out, err := exec.CommandContext(ctx, "screencapture", "-x", "-t", "jpg", path).CombinedOutput()
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("screencapture: %v: %s", err, out)), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("read screenshot", err), nil
	}

	if cfg, _, decErr := image.DecodeConfig(bytes.NewReader(data)); decErr == nil {
		SetScreenResolution(Resolution{Width: cfg.Width, Height: cfg.Height})
	}

	return &mcplib.CallToolResult{
		Content: []mcplib.Content{
			mcplib.NewImageContent(base64.StdEncoding.EncodeToString(data), "image/jpeg"),
			mcplib.NewTextContent(screenRecordingPermissionHint),
		},
	}, nil
}
