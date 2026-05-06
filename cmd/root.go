package cmd

import (
	"os"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultEndpoint  = "https://codespaces-api.services.bitrise.io"
	defaultUIBaseURL = "https://app.bitrise.io/dev-environments"
	envEndpoint      = "BITRISE_CODESPACES_API_BASE_URL"
	envUIBaseURL     = "BITRISE_DEV_ENVIRONMENTS_UI_BASE_URL"
	envPAT           = "BITRISE_PAT"
)

var (
	flagEndpoint  string
	flagUIBaseURL string
	flagTimeout   time.Duration
)

var rootCmd = &cobra.Command{
	Use:           "ai-qa-agent-cli",
	Short:         "CLI for driving Bitrise Remote Development Environment (RDE) sessions",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	endpointDefault := defaultEndpoint
	if v := os.Getenv(envEndpoint); v != "" {
		endpointDefault = v
	}
	uiBaseDefault := defaultUIBaseURL
	if v := os.Getenv(envUIBaseURL); v != "" {
		uiBaseDefault = v
	}
	rootCmd.PersistentFlags().StringVar(&flagEndpoint, "endpoint", endpointDefault, "Codespaces REST API base URL (scheme://host[:port]). For local dev use http://localhost:8081. Env: "+envEndpoint)
	rootCmd.PersistentFlags().StringVar(&flagUIBaseURL, "ui-base-url", uiBaseDefault, "Base URL for the Bitrise dev-environments UI; used to print a session-detail link after create. Env: "+envUIBaseURL)
	rootCmd.PersistentFlags().DurationVar(&flagTimeout, "timeout", 15*time.Minute, "Overall timeout for the command (covers create + wait)")

	rootCmd.AddCommand(sessionCmd)
	rootCmd.AddCommand(mcpCmd)
}
