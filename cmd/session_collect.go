package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bitrise-io/ai-qa-agent-cli/internal/codespaces"
	"github.com/spf13/cobra"
)

// remoteQAAgentDir is the absolute path of the QA Agent template's working
// directory on the session VM. The template's warmup.sh / startup.sh / watcher
// all use $HOME/.qa-agent, and macOS RDE sessions run as the `vagrant` user —
// so the absolute is /Users/vagrant/.qa-agent. SessionDownload requires an
// absolute path, hence the hardcode. If the session-user convention ever
// changes, the template should publish its absolute path to a known location
// the CLI can read instead.
const remoteQAAgentDir = "/Users/vagrant/.qa-agent"

var (
	collectWorkspace    string
	collectResultsDir   string
	collectNoWait       bool
	collectNoStop       bool
	collectPollInterval time.Duration
)

var sessionCollectCmd = &cobra.Command{
	Use:   "collect <session-id>",
	Short: "Wait for the QA run to finish, download results, then stop the VM",
	Long: `Wait for the in-VM agent to reach IDLE (Claude has fired its Stop hook),
download the contents of ~/.qa-agent on the VM into --results-dir, and stop
the session.

The three steps are on by default. Use --no-wait to skip polling and
download immediately. Use --no-stop to keep the VM running after collection
(handy for debugging via tmux attach -t qa-agent).`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionCollect,
}

func init() {
	sessionCmd.AddCommand(sessionCollectCmd)

	f := sessionCollectCmd.Flags()
	f.StringVarP(&collectWorkspace, "workspace", "w", "", "Workspace ID (required)")
	f.StringVar(&collectResultsDir, "results-dir", "",
		"Local directory to extract results into. Defaults to ./qa-agent-results/<session-id>/")
	f.BoolVar(&collectNoWait, "no-wait", false,
		"Skip waiting for the agent to reach IDLE; download whatever is currently in ~/.qa-agent on the VM")
	f.BoolVar(&collectNoStop, "no-stop", false,
		"Skip stopping the session after collection (keeps the VM around for inspection)")
	f.DurationVar(&collectPollInterval, "poll-interval", 10*time.Second,
		"How often to poll GetSession while waiting for the agent to reach IDLE")

	_ = sessionCollectCmd.MarkFlagRequired("workspace")
}

func runSessionCollect(cmd *cobra.Command, args []string) error {
	pat := os.Getenv(envPAT)
	if pat == "" {
		return fmt.Errorf("%s not set", envPAT)
	}
	sessionID := args[0]

	destDir := collectResultsDir
	if destDir == "" {
		destDir = filepath.Join("qa-agent-results", sessionID)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), flagTimeout)
	defer cancel()

	client, err := codespaces.NewClient(flagEndpoint, pat)
	if err != nil {
		return err
	}
	defer client.Close()

	if !collectNoWait {
		fmt.Fprintln(os.Stderr, "waiting for QA agent to reach IDLE...")
		_, err := client.WaitForAgentIdle(ctx, sessionID, collectWorkspace, collectPollInterval, func(s codespaces.AgentSessionStatus) {
			fmt.Fprintf(os.Stderr, "  agent_session_status: %s\n", s)
		})
		if err != nil {
			return fmt.Errorf("waiting for agent: %w", err)
		}
		// Give tmux pipe-pane a beat to flush claude.log before tar runs on the VM.
		time.Sleep(3 * time.Second)
	}

	fmt.Fprintf(os.Stderr, "downloading %s -> %s\n", remoteQAAgentDir, destDir)
	files, err := client.DownloadDir(ctx, sessionID, collectWorkspace, remoteQAAgentDir, destDir, true)
	if err != nil {
		return fmt.Errorf("download results: %w", err)
	}
	fmt.Fprintf(os.Stderr, "extracted %d file(s):\n", len(files))
	for _, f := range files {
		fmt.Fprintf(os.Stderr, "  %s\n", f)
	}

	if !collectNoStop {
		fmt.Fprintln(os.Stderr, "stopping session...")
		stopped, err := client.StopSession(ctx, sessionID, collectWorkspace)
		if err != nil {
			return fmt.Errorf("stop session: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  status: %s\n", stopped.Status)
	}

	fmt.Println(destDir)
	return nil
}
