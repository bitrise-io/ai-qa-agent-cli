package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/ai-qa-agent-cli/internal/codespaces"
	codespacesv1 "github.com/bitrise-io/bitrise-codespaces/backend/proto/codespaces/v1"
	"github.com/spf13/cobra"
)

const remotePathPlaceholder = "{{REMOTE_PATH}}"

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage RDE sessions",
}

var (
	createWorkspace            string
	createTemplate             string
	createName                 string
	createDescription          string
	createInputs               []string
	createSecretInputs         []string
	createSavedInputs          []string
	createFeatureFlags         []string
	createCluster              string
	createAIPrompt             string
	createAutoTerminateMinutes int32
	createMapSavedInputs       bool
	createWait                 bool
	createPollInterval         time.Duration
	createOpenRemoteAccess     bool
	createUpload               string
	createUploadDestination    string
)

var sessionCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new RDE session and (optionally) wait for it to be running",
	RunE:  runSessionCreate,
}

func init() {
	sessionCmd.AddCommand(sessionCreateCmd)

	f := sessionCreateCmd.Flags()
	f.StringVarP(&createWorkspace, "workspace", "w", "", "Workspace ID (required)")
	f.StringVarP(&createTemplate, "template", "t", "", "Template ID (required)")
	f.StringVar(&createName, "name", "", "Session name (required)")
	f.StringVar(&createDescription, "description", "", "Session description")
	f.StringArrayVar(&createInputs, "input", nil, "Session input as key=value (repeatable)")
	f.StringArrayVar(&createSecretInputs, "secret-input", nil, "Secret session input as key=value (repeatable)")
	f.StringArrayVar(&createSavedInputs, "saved-input", nil, "Saved input reference as key=savedInputID (repeatable)")
	f.StringArrayVar(&createFeatureFlags, "feature-flag", nil, "Feature flag name to enable (repeatable)")
	f.StringVar(&createCluster, "cluster", "", "Target cluster name (only required when image+machine-type matches multiple clusters)")
	f.StringVar(&createAIPrompt, "ai-prompt", "", "AI prompt to pass to Claude Code on session start. "+
		"Any "+remotePathPlaceholder+" is substituted with the remote path of --upload. "+
		"Note: the binary is uploaded AFTER the session reaches RUNNING, so phrase the prompt to wait for the file "+
		"(e.g. 'Wait until "+remotePathPlaceholder+" exists, then run it and report output').")
	f.StringVar(&createUpload, "upload", "", "Local file to upload to the session after it reaches RUNNING")
	f.StringVar(&createUploadDestination, "upload-destination", "/tmp", "Absolute remote directory the --upload file is extracted into")
	f.Int32Var(&createAutoTerminateMinutes, "auto-terminate-minutes", -1, "Minutes before auto-termination (0 disables; -1 leaves backend default)")
	f.BoolVar(&createMapSavedInputs, "map-saved-inputs", false, "Auto-fill template session inputs from caller's saved inputs")
	f.BoolVar(&createWait, "wait", true, "Poll until session reaches RUNNING")
	f.DurationVar(&createPollInterval, "poll-interval", 5*time.Second, "Status poll interval when --wait is set")
	f.BoolVar(&createOpenRemoteAccess, "open-remote-access", false, "After RUNNING, call OpenRemoteAccess and print SSH/VNC details")

	_ = sessionCreateCmd.MarkFlagRequired("workspace")
	_ = sessionCreateCmd.MarkFlagRequired("template")
	_ = sessionCreateCmd.MarkFlagRequired("name")
}

func runSessionCreate(cmd *cobra.Command, _ []string) error {
	pat := os.Getenv(envPAT)
	if pat == "" {
		return fmt.Errorf("%s not set", envPAT)
	}

	aiPrompt, remotePath, err := resolveUploadAndPrompt(createUpload, createUploadDestination, createAIPrompt)
	if err != nil {
		return err
	}

	inputs, err := buildSessionInputs(createInputs, createSecretInputs, createSavedInputs)
	if err != nil {
		return err
	}

	req := &codespacesv1.CreateSessionRequest{
		Name:                    createName,
		Description:             createDescription,
		TemplateId:              createTemplate,
		WorkspaceId:             createWorkspace,
		SessionInputs:           inputs,
		EnabledFeatureFlagNames: createFeatureFlags,
		Cluster:                 createCluster,
		AiPrompt:                aiPrompt,
		MapSavedToSessionInputs: createMapSavedInputs,
	}
	if createAutoTerminateMinutes >= 0 {
		v := createAutoTerminateMinutes
		req.AutoTerminateMinutes = &v
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), flagTimeout)
	defer cancel()

	client, err := codespaces.NewClient(flagEndpoint, pat, flagInsecure)
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.CreateSession(ctx, req)
	if err != nil {
		return fmt.Errorf("CreateSession: %w", err)
	}
	fmt.Fprintf(os.Stderr, "created session %s (status: %s)\n", session.GetId(), session.GetStatus())

	if createWait {
		session, err = client.WaitForRunning(ctx, session.GetId(), createWorkspace, createPollInterval, func(s codespacesv1.SessionStatus) {
			fmt.Fprintf(os.Stderr, "  status: %s\n", s)
		})
		if err != nil {
			return err
		}
	}

	if createUpload != "" && session.GetStatus() == codespacesv1.SessionStatus_SESSION_STATUS_RUNNING {
		actualPath, err := client.UploadFile(ctx, session.GetId(), createWorkspace, createUpload, createUploadDestination)
		if err != nil {
			return fmt.Errorf("upload %s: %w", createUpload, err)
		}
		fmt.Fprintf(os.Stderr, "uploaded %s -> %s\n", createUpload, actualPath)
		_ = remotePath // resolved earlier for the prompt; logged here from the server's confirmed path
	}

	if createOpenRemoteAccess && session.GetStatus() == codespacesv1.SessionStatus_SESSION_STATUS_RUNNING {
		session, err = client.OpenRemoteAccess(ctx, session.GetId(), createWorkspace)
		if err != nil {
			return fmt.Errorf("OpenRemoteAccess: %w", err)
		}
		fmt.Fprintf(os.Stderr, "ssh: %s (password: %s)\n", session.GetSshAddress(), session.GetSshPassword())
		fmt.Fprintf(os.Stderr, "vnc: %s (user: %s, password: %s)\n", session.GetVncAddress(), session.GetVncUsername(), session.GetVncPassword())
	}

	fmt.Println(session.GetId())
	return nil
}

// resolveUploadAndPrompt validates the upload flags against the prompt placeholder
// and returns the (possibly substituted) prompt plus the resolved remote path.
// remotePath is empty when --upload is not set.
func resolveUploadAndPrompt(uploadLocal, uploadDest, prompt string) (string, string, error) {
	hasPlaceholder := strings.Contains(prompt, remotePathPlaceholder)

	if uploadLocal == "" {
		if hasPlaceholder {
			return "", "", fmt.Errorf("--ai-prompt contains %s but --upload is not set", remotePathPlaceholder)
		}
		return prompt, "", nil
	}

	if !path.IsAbs(uploadDest) {
		return "", "", fmt.Errorf("--upload-destination must be absolute, got %q", uploadDest)
	}

	stat, err := os.Stat(uploadLocal)
	if err != nil {
		return "", "", fmt.Errorf("--upload %s: %w", uploadLocal, err)
	}
	if stat.IsDir() {
		return "", "", fmt.Errorf("--upload %s: must be a file, not a directory", uploadLocal)
	}

	remote := path.Join(uploadDest, filepath.Base(uploadLocal))
	if hasPlaceholder {
		prompt = strings.ReplaceAll(prompt, remotePathPlaceholder, remote)
	} else if prompt != "" {
		fmt.Fprintf(os.Stderr, "warning: --ai-prompt does not reference %s; ensure the prompt knows the file's path (%s)\n", remotePathPlaceholder, remote)
	}
	return prompt, remote, nil
}

func buildSessionInputs(plain, secret, saved []string) ([]*codespacesv1.SessionInputValue, error) {
	out := make([]*codespacesv1.SessionInputValue, 0, len(plain)+len(secret)+len(saved))

	for _, kv := range plain {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("--input %q: expected key=value", kv)
		}
		out = append(out, &codespacesv1.SessionInputValue{Key: k, Value: v})
	}
	for _, kv := range secret {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("--secret-input %q: expected key=value", kv)
		}
		out = append(out, &codespacesv1.SessionInputValue{Key: k, Value: v, IsSecret: true})
	}
	for _, kv := range saved {
		k, id, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("--saved-input %q: expected key=savedInputID", kv)
		}
		out = append(out, &codespacesv1.SessionInputValue{Key: k, SavedInputId: id})
	}
	return out, nil
}
