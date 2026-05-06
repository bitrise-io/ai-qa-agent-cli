package main

import (
	"fmt"
	"os"

	"github.com/bitrise-io/ai-qa-agent-cli/cmd"
	"github.com/bitrise-io/ai-qa-agent-cli/internal/codespaces"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", codespaces.FormatError(err))
		os.Exit(1)
	}
}
