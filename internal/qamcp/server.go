// Package qamcp implements an in-VM MCP server that exposes screenshot,
// click, type, scroll, and mouse_drag tools for driving the local macOS
// display. It replaces the round-trip path through bitrise-mcp-dev-environments
// → codespaces backend → guest-agent for QA Agent sessions: every action
// runs on the same VM the MCP is registered against, so there is no network
// hop and no session_id parameter.
//
// TCC permissions: the kTCCService{Accessibility,PostEvent,ScreenCapture}
// grants needed by screencapture and the embedded Swift binaries are pre-
// installed against guest-agent at warmup time by bitrise-codespaces'
// instancemanager.tccSetup. Because this MCP runs in a process tree rooted
// at guest-agent (warmup → startup → watcher → tmux → claude → mcp), the
// responsible-process attribution chain matches those grants.
package qamcp

import (
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	serverName    = "qa-agent"
	serverVersion = "0.1.0"
)

// NewServer constructs the MCP server with the QA Agent tool belt.
func NewServer() *server.MCPServer {
	s := server.NewMCPServer(
		serverName, serverVersion,
		server.WithRecovery(),
		server.WithLogging(),
	)

	s.AddTool(mcplib.NewTool("qa_screenshot",
		mcplib.WithDescription(`Take a screenshot of the macOS display this MCP server is running on.

Use this to verify the current state of the GUI, identify coordinates for click/drag operations, and debug visual issues. Always call this before qa_click or qa_mouse_drag so the server can capture the real screen resolution and rescale your coordinates correctly.

Returns a JPEG image of the entire display.`),
	), screenshotHandler)

	s.AddTool(mcplib.NewTool("qa_click",
		mcplib.WithDescription(`Click at specific coordinates on the macOS display.

Call qa_screenshot first so the server captures the real screen resolution. Then provide x, y in the coordinate space of the screenshot you are looking at, and pass max_x, max_y — the width and height of that same view. The server rescales (x, y) to real screen coordinates using the cached resolution. If no screenshot has been taken yet, the server falls back to a 1920×1080 screen, so passing max_x=1920 and max_y=1080 with raw screen coordinates also works.`),
		mcplib.WithNumber("x", mcplib.Description("X coordinate in the screenshot view's coordinate space"), mcplib.Required()),
		mcplib.WithNumber("y", mcplib.Description("Y coordinate in the screenshot view's coordinate space"), mcplib.Required()),
		mcplib.WithNumber("max_x", mcplib.Description("Width of the screenshot view you reasoned about when picking x"), mcplib.Required()),
		mcplib.WithNumber("max_y", mcplib.Description("Height of the screenshot view you reasoned about when picking y"), mcplib.Required()),
		mcplib.WithString("button", mcplib.Description("Mouse button: left (default), right, or middle"), mcplib.Enum("left", "right", "middle"), mcplib.DefaultString("left")),
		mcplib.WithBoolean("double_click", mcplib.Description("Whether to perform a double-click (default: false)")),
	), clickHandler)

	s.AddTool(mcplib.NewTool("qa_type",
		mcplib.WithDescription(`Type text on the macOS display via synthetic keyboard events.

The text is typed character by character. Special characters and control sequences are supported.`),
		mcplib.WithString("text", mcplib.Description("The text to type"), mcplib.Required()),
	), typeHandler)

	s.AddTool(mcplib.NewTool("qa_scroll",
		mcplib.WithDescription(`Scroll at the current mouse position on the macOS display.`),
		mcplib.WithString("direction", mcplib.Description("Scroll direction"), mcplib.Enum("up", "down"), mcplib.Required()),
		mcplib.WithNumber("amount", mcplib.Description("Number of lines to scroll (default: 3)"), mcplib.DefaultNumber(3)),
	), scrollHandler)

	s.AddTool(mcplib.NewTool("qa_mouse_drag",
		mcplib.WithDescription(`Drag the mouse between two points on the macOS display.

Call qa_screenshot first so the server captures the real screen resolution. Then provide both endpoints (start_x/start_y, end_x/end_y) in the coordinate space of the screenshot you are looking at, and pass max_x, max_y — the width and height of that same view. The server rescales both endpoints to real screen coordinates using the cached resolution. If no screenshot has been taken yet, the server falls back to a 1920×1080 screen.`),
		mcplib.WithNumber("start_x", mcplib.Description("Starting X coordinate in the screenshot view's coordinate space"), mcplib.Required()),
		mcplib.WithNumber("start_y", mcplib.Description("Starting Y coordinate in the screenshot view's coordinate space"), mcplib.Required()),
		mcplib.WithNumber("end_x", mcplib.Description("Ending X coordinate in the screenshot view's coordinate space"), mcplib.Required()),
		mcplib.WithNumber("end_y", mcplib.Description("Ending Y coordinate in the screenshot view's coordinate space"), mcplib.Required()),
		mcplib.WithNumber("max_x", mcplib.Description("Width of the screenshot view"), mcplib.Required()),
		mcplib.WithNumber("max_y", mcplib.Description("Height of the screenshot view"), mcplib.Required()),
	), mouseDragHandler)

	return s
}
