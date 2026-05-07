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
	uploadURL string
)

var sessionUploadCmd = &cobra.Command{
	Use:   "upload <results-dir>",
	Short: "Upload an already-collected result folder to the QA results visualisation service",
	Long: `Upload a flat result folder (junit.xml + summary.md + claude.log + screenshot-*.png)
to the bitrise-rde-qa-results service. Use this to re-upload after a previous
` + "`session collect`" + ` finished, or to push results from any local directory.

The given path may be either:
  - the result folder itself (contains junit.xml at the top), or
  - a session-collect output (contains a results/ subfolder).

Authenticates with $BITRISE_PAT (Bearer). Override the endpoint with
--upload-url or $` + qaresults.EnvURL + `.`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionUpload,
}

func init() {
	sessionCmd.AddCommand(sessionUploadCmd)

	f := sessionUploadCmd.Flags()
	f.StringVar(&uploadURL, "upload-url", "",
		"Override the upload endpoint (default "+qaresults.DefaultURL+", env "+qaresults.EnvURL+")")
}

func runSessionUpload(cmd *cobra.Command, args []string) error {
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

	// Auto-detect: prefer a results/ subfolder, otherwise use the dir itself.
	resultsRoot := src
	if _, err := os.Stat(filepath.Join(src, "junit.xml")); err == nil {
		// looks like a flat results folder already
	} else if sub, err := os.Stat(filepath.Join(src, "results")); err == nil && sub.IsDir() {
		resultsRoot = filepath.Join(src, "results")
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), flagTimeout)
	defer cancel()

	uploader := qaresults.New(uploadURL, pat)
	logf("uploading %s -> %s", resultsRoot, uploader.URL)
	r, err := uploader.UploadDir(ctx, resultsRoot, nil)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	logf("  status:  %s (%d/%d passed, %d failed, %d skipped)",
		r.Summary.Status, r.Summary.Passed, r.Summary.Total, r.Summary.Failed, r.Summary.Skipped)
	logf("  view at: %s", uploader.AbsoluteResultURL(r.URL))

	fmt.Println(uploader.AbsoluteResultURL(r.URL))
	return nil
}
