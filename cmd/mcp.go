package cmd

import (
	"fmt"

	"github.com/bitrise-io/ai-qa-agent-cli/internal/qamcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run the QA Agent MCP server (stdio)",
	Long: `Run the QA Agent MCP server over stdio.

Registered with the in-VM Claude Code instance by the QA Agent template's
warmup.sh and exposes screenshot/click/type/scroll/mouse_drag tools that
drive the local macOS display directly. Unlike bitrise-mcp-dev-environments,
this server runs *inside* the same VM it operates on, so it bypasses the
public Codespaces backend and does not need a Bitrise PAT.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		s := qamcp.NewServer()
		if err := server.ServeStdio(s); err != nil {
			return fmt.Errorf("serve stdio: %w", err)
		}
		return nil
	},
}
