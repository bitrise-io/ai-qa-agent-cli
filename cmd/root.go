package cmd

import (
	"os"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultEndpoint = "codespaces-api.services.bitrise.io:443"
	envEndpoint     = "BITRISE_CODESPACES_GRPC_ENDPOINT"
	envPAT          = "BITRISE_PAT"
)

var (
	flagEndpoint string
	flagInsecure bool
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
	rootCmd.PersistentFlags().StringVar(&flagEndpoint, "endpoint", endpointDefault, "Codespaces gRPC endpoint (host:port). Env: "+envEndpoint)
	rootCmd.PersistentFlags().BoolVar(&flagInsecure, "insecure", false, "Disable TLS (for local backend)")
	rootCmd.PersistentFlags().DurationVar(&flagTimeout, "timeout", 15*time.Minute, "Overall timeout for the command (covers create + wait)")

	rootCmd.AddCommand(sessionCmd)
}
