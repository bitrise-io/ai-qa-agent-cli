package main

import (
	"fmt"
	"os"

	"github.com/bitrise-io/ai-qa-agent-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
