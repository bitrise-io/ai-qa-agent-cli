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

const (
	remotePathPlaceholder = "{{REMOTE_PATH}}"
	// qaPromptInputKey is the session input the QA Agent template's watcher
	// reads to launch Claude. We do NOT set CreateSessionRequest.AiPrompt
	// because that would trigger the codespaces backend's claudeAIAutoStart,
	// which would start a second Claude session at warmup before the upload
	// has arrived — the watcher inside the template is the only intended
	// launcher.
	qaPromptInputKey = "QA_PROMPT"

	// The QA Agent template needs these two as session inputs so warmup.sh
	// can register the bitrise-dev-environments MCP server with credentials
	// scoped to the same workspace driving the session. We auto-fill them
	// from --workspace and BITRISE_PAT respectively when the caller hasn't
	// already supplied them.
	bitriseTokenInputKey       = "BITRISE_TOKEN"
	bitriseWorkspaceIDInputKey = "BITRISE_WORKSPACE_ID"
)

// defaultQAPrompt is sent when --qa-prompt is omitted. It runs a generic
// smoke test of the uploaded app: install, launch, exercise via the
// bitrise-dev-environments MCP tools, report. Knowledge it relies on is
// produced by the QA Agent template's startup.sh and watcher.sh.
const defaultQAPrompt = `You are an iOS QA tester running inside a Bitrise RDE session.

Environment:
  /tmp/.qa-agent-info.json   { udid, session_id, workspace_id }
  ~/.qa-agent/upload-path    path to the uploaded app directory
  ~/.qa-agent/results/       PRE-CREATED. Save ALL artefacts here, FLAT. Bitrise's JUnit attachment convention requires attachment files to sit next to junit.xml — do NOT create subdirectories.

Smoke-test the uploaded app:
  1. Resolve UDID, SESSION_ID, and the upload directory from the files above.
  2. Find the .app inside the upload directory. If it is an .ipa, unzip it first; the bundle is at Payload/*.app.
  3. xcrun simctl install $UDID <path-to-.app>
  4. Read CFBundleIdentifier from <.app>/Info.plist.
  5. xcrun simctl launch $UDID <bundle-id>
  6. Use the bitrise-dev-environments MCP server (screenshot, click, scroll, type) with session_id=$SESSION_ID to drive the simulator: take an initial screenshot, then tap visible primary buttons, scroll on each screen, and walk through any tab bar or menu. Save each screenshot directly into ~/.qa-agent/results/ as screenshot-NN-<short-tag>.png (zero-padded NN, lowercase tag, no spaces).
  7. Stop after about 10 interactions or sooner if you hit a crash, an unexpected alert, or a stuck loading state.
  8. Write results to ~/.qa-agent/results/ (FLAT — no subdirs):
     a. cp ~/.qa-agent/claude.log ~/.qa-agent/results/claude.log so it can be referenced as a JUnit attachment.
     b. summary.md — short prose: launched yes/no, screens reached, anomalies, overall PASS or FAIL, list of attached screenshots.
     c. junit.xml — Surefire-style JUnit XML with <testsuite name="QA Agent"> containing AT LEAST these <testcase> elements:
        - app_launch (failure if simctl install or launch failed)
        - screen_navigation (failure if you could not get past the first screen)
        - no_crashes (failure if you observed a crash, unexpected alert, or stuck loading state)
        Add more <testcase> elements per major feature you exercised (e.g. login_flow, cart_checkout). For every <testcase>, include a <properties> block listing the screenshots and (on failure) claude.log as Bitrise attachments:

          <testcase name="app_launch" classname="QAAgent" time="3.2">
            <properties>
              <property name="attachment_1" value="screenshot-01-launch.png" />
            </properties>
          </testcase>

        - The property name MUST be attachment_<N> with N a 1-based ordered index per testcase.
        - The value MUST be a bare filename (no path) of a file you wrote into ~/.qa-agent/results/.
        - Bitrise accepts these extensions for attachments: .jpg .jpeg .png .txt .log .mp4 .webm .ogg.
        - On failure, add a <failure message="..." type="..."><![CDATA[ details ]]></failure> element inside the <testcase> AND attach claude.log:

          <testcase name="no_crashes" classname="QAAgent" time="42.0">
            <failure message="Login screen froze" type="StuckLoading"><![CDATA[Spinner remained 30s, no transition. See attachment_2.]]></failure>
            <properties>
              <property name="attachment_1" value="screenshot-04-frozen-login.png" />
              <property name="attachment_2" value="claude.log" />
            </properties>
          </testcase>

        Make junit.xml well-formed XML (declaration optional, but balance every tag). Do not point an attachment at a file you did not write.
  9. Exit when finished.
`

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
	createQAPrompt             string
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
	f.StringVar(&createQAPrompt, "qa-prompt", "", "QA Agent prompt. Sent to the template as the "+qaPromptInputKey+" session input. "+
		"Any "+remotePathPlaceholder+" is substituted with the remote path of --upload before submission. "+
		"The in-VM watcher launches Claude with this prompt once the --upload directory is populated and size-stable. "+
		"When omitted, a built-in smoke-test prompt is used (install + launch the uploaded app and exercise its UI).")
	f.StringVar(&createUpload, "upload", "", "Local file to upload to the session after it reaches RUNNING")
	f.StringVar(&createUploadDestination, "upload-destination", "/tmp/bitrise-ai-qa-agent", "Absolute remote directory the --upload file is extracted into. "+
		"Must match the QA Agent template's QA_WATCH_DIR (default /tmp/bitrise-ai-qa-agent), since that directory becoming non-empty is the watcher's trigger.")
	f.Int32Var(&createAutoTerminateMinutes, "auto-terminate-minutes", 60, "Minutes before auto-termination (0 disables; -1 leaves backend default). Defaults to 60 so a crashed CLI eventually frees the VM; pass 0 explicitly if you want sessions that don't auto-terminate.")
	f.BoolVar(&createMapSavedInputs, "map-saved-inputs", true, "Auto-fill template session inputs from caller's saved inputs")
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

	rawPrompt := createQAPrompt
	usingDefaultPrompt := rawPrompt == ""
	if usingDefaultPrompt {
		rawPrompt = defaultQAPrompt
	}

	qaPrompt, _, err := resolveUploadAndPrompt(createUpload, createUploadDestination, rawPrompt, usingDefaultPrompt)
	if err != nil {
		return err
	}

	inputs, err := buildSessionInputs(createInputs, createSecretInputs, createSavedInputs)
	if err != nil {
		return err
	}
	inputs, err = injectQAPrompt(inputs, qaPrompt)
	if err != nil {
		return err
	}
	inputs = ensureBitriseAuthInputs(inputs, pat, createWorkspace)

	req := &codespacesv1.CreateSessionRequest{
		Name:                    createName,
		Description:             createDescription,
		TemplateId:              createTemplate,
		WorkspaceId:             createWorkspace,
		SessionInputs:           inputs,
		EnabledFeatureFlagNames: createFeatureFlags,
		Cluster:                 createCluster,
		MapSavedToSessionInputs: createMapSavedInputs,
	}
	if createAutoTerminateMinutes >= 0 {
		v := createAutoTerminateMinutes
		req.AutoTerminateMinutes = &v
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), flagTimeout)
	defer cancel()

	client, err := codespaces.NewClient(flagEndpoint, pat)
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
	if createUpload != "" {
		fmt.Fprintf(os.Stderr,
			"\nWhen the QA run finishes, collect results + stop the VM with:\n"+
				"  ai-qa-agent-cli session collect %s --workspace %s\n",
			session.GetId(), createWorkspace)
	}
	return nil
}

// resolveUploadAndPrompt validates the upload flags against the prompt placeholder
// and returns the (possibly substituted) prompt plus the resolved remote path.
// remotePath is empty when --upload is not set.
//
// isDefault tells us the prompt came from defaultQAPrompt (no user input). The
// default deliberately resolves the upload path at runtime via
// `cat ~/.qa-agent/upload-path`, so the "doesn't reference REMOTE_PATH" warning
// is suppressed for it.
func resolveUploadAndPrompt(uploadLocal, uploadDest, prompt string, isDefault bool) (string, string, error) {
	hasPlaceholder := strings.Contains(prompt, remotePathPlaceholder)

	if uploadLocal == "" {
		if hasPlaceholder {
			return "", "", fmt.Errorf("--qa-prompt contains %s but --upload is not set", remotePathPlaceholder)
		}
		return prompt, "", nil
	}

	if !path.IsAbs(uploadDest) {
		return "", "", fmt.Errorf("--upload-destination must be absolute, got %q", uploadDest)
	}

	if _, err := os.Stat(uploadLocal); err != nil {
		return "", "", fmt.Errorf("--upload %s: %w", uploadLocal, err)
	}

	remote := path.Join(uploadDest, filepath.Base(uploadLocal))
	if hasPlaceholder {
		prompt = strings.ReplaceAll(prompt, remotePathPlaceholder, remote)
	} else if !isDefault && prompt != "" {
		fmt.Fprintf(os.Stderr, "warning: --qa-prompt does not reference %s; ensure the prompt knows the file's path (%s)\n", remotePathPlaceholder, remote)
	}
	return prompt, remote, nil
}

// injectQAPrompt appends the resolved QA prompt as a QA_PROMPT session input.
// Errors if the caller already supplied QA_PROMPT via --input / --secret-input
// / --saved-input — the dedicated --qa-prompt flag is the supported entry point.
func injectQAPrompt(inputs []*codespacesv1.SessionInputValue, prompt string) ([]*codespacesv1.SessionInputValue, error) {
	if prompt == "" {
		return inputs, nil
	}
	for _, in := range inputs {
		if in.GetKey() == qaPromptInputKey {
			return nil, fmt.Errorf("%s already supplied via --input/--secret-input/--saved-input; use --qa-prompt only", qaPromptInputKey)
		}
	}
	return append(inputs, &codespacesv1.SessionInputValue{Key: qaPromptInputKey, Value: prompt}), nil
}

// ensureBitriseAuthInputs adds BITRISE_TOKEN and BITRISE_WORKSPACE_ID session
// inputs unless the caller already supplied them via --input / --secret-input
// / --saved-input. The QA Agent template's warmup.sh requires both to register
// the bitrise-dev-environments MCP server inside the VM, scoped to the same
// workspace driving the session — without auto-injection, callers would have
// to repeat their CLI auth on every invocation.
func ensureBitriseAuthInputs(inputs []*codespacesv1.SessionInputValue, pat, workspaceID string) []*codespacesv1.SessionInputValue {
	have := func(key string) bool {
		for _, in := range inputs {
			if in.GetKey() == key {
				return true
			}
		}
		return false
	}
	if !have(bitriseTokenInputKey) {
		inputs = append(inputs, &codespacesv1.SessionInputValue{
			Key:      bitriseTokenInputKey,
			Value:    pat,
			IsSecret: true,
		})
	}
	if !have(bitriseWorkspaceIDInputKey) {
		inputs = append(inputs, &codespacesv1.SessionInputValue{
			Key:   bitriseWorkspaceIDInputKey,
			Value: workspaceID,
		})
	}
	return inputs
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
