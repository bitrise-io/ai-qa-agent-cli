package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bitrise-io/ai-qa-agent-cli/internal/qaresults"
	"github.com/spf13/cobra"
)

var (
	uploadResultsURL string
)

// uploadResultsCmd is invoked from the QA Agent template's watcher.sh on
// the session VM after Claude exits. The binary is already on the VM
// (warmup.sh runs `go install github.com/bitrise-io/ai-qa-agent-cli@latest`).
//
// It is also useful locally for re-uploading a previously collected folder
// (`session collect` doesn't upload — the VM does).
var uploadResultsCmd = &cobra.Command{
	Use:   "upload-results <results-dir>",
	Short: "Tar a flat result folder and POST it to the QA results visualisation service",
	Long: `Pack the regular files at the top level of <results-dir> into a tar.gz
and POST it to the bitrise-rde-qa-results service. Authenticates with
$BITRISE_PAT (Bearer). Override the endpoint with --upload-url or
$` + qaresults.EnvURL + `.

The path may be either:
  - the result folder itself (junit.xml at the top), or
  - a session-collect output dir (results/ subfolder containing them).

On success the absolute result URL is printed on stdout.`,
	Args: cobra.ExactArgs(1),
	RunE: runUploadResults,
}

func init() {
	rootCmd.AddCommand(uploadResultsCmd)

	f := uploadResultsCmd.Flags()
	f.StringVar(&uploadResultsURL, "upload-url", "",
		"Override the upload endpoint (default "+qaresults.DefaultURL+", env "+qaresults.EnvURL+")")
}

func runUploadResults(cmd *cobra.Command, args []string) error {
	pat := os.Getenv(envPAT)
	if pat == "" {
		return fmt.Errorf("%s not set", envPAT)
	}

	src := args[0]
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}

	resultsRoot := src
	if _, err := os.Stat(filepath.Join(src, "junit.xml")); err == nil {
		// flat results folder
	} else if sub, err := os.Stat(filepath.Join(src, "results")); err == nil && sub.IsDir() {
		resultsRoot = filepath.Join(src, "results")
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), flagTimeout)
	defer cancel()

	uploader := qaresults.New(uploadResultsURL, pat)
	logf("uploading %s -> %s", resultsRoot, uploader.URL)
	r, err := uploader.UploadDir(ctx, resultsRoot)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	logf("  status:  %s (%d/%d passed, %d failed, %d skipped)",
		r.Summary.Status, r.Summary.Passed, r.Summary.Total, r.Summary.Failed, r.Summary.Skipped)
	abs := uploader.AbsoluteResultURL(r.URL)
	logf("  view at: %s", abs)
	fmt.Println(abs)
	return nil
}
