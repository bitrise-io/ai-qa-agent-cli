package cmd

import (
	"os"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultEndpoint = "https://codespaces-api.services.bitrise.io"
	envEndpoint     = "BITRISE_CODESPACES_API_BASE_URL"
	envPAT          = "BITRISE_PAT"
)

var (
	flagEndpoint string
	flagTimeout  time.Duration
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
	rootCmd.PersistentFlags().StringVar(&flagEndpoint, "endpoint", endpointDefault, "Codespaces REST API base URL (scheme://host[:port]). For local dev use http://localhost:8081. Env: "+envEndpoint)
	rootCmd.PersistentFlags().DurationVar(&flagTimeout, "timeout", 15*time.Minute, "Overall timeout for the command (covers create + wait)")

	rootCmd.AddCommand(sessionCmd)
}
