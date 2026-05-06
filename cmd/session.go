package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bitrise-io/ai-qa-agent-cli/internal/codespaces"
	"github.com/spf13/cobra"
)

// xcodeVersionRE matches Apple-style Xcode versions: digits with up to two
// dot-separated components (e.g. 26, 26.3, 16.4.1). The template resolves
// /Applications/Xcode-<version>.app, so anything outside this shape is
// guaranteed to fail on the VM — better to fail fast on the client.
var xcodeVersionRE = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)

const (
	remotePathPlaceholder = "{{REMOTE_PATH}}"

	// Optional QA Agent template inputs. Each is forwarded as a session
	// input only when the caller explicitly sets the corresponding flag —
	// empty values let the template apply its own defaults. QA_WATCH_DIR
	// is the exception: we always send it, tied to --upload-destination,
	// so the watcher trigger and the upload destination cannot drift.
	deviceTypeInputKey        = "DEVICE_TYPE"
	iosVersionInputKey        = "IOS_VERSION"
	xcodeVersionInputKey      = "XCODE_VERSION"
	qaWatchDirInputKey        = "QA_WATCH_DIR"
	qaWatchTimeoutSecInputKey = "QA_WATCH_TIMEOUT_SEC"
	qaWatchPollSecInputKey    = "QA_WATCH_POLL_SEC"
)

// defaultQAPrompt is sent when --qa-prompt is omitted. It runs a generic
// smoke test of the uploaded app: install, launch, exercise via the
// in-VM qa-agent MCP tools, report. Knowledge it relies on is produced by
// the QA Agent template's startup.sh and watcher.sh.
const defaultQAPrompt = `You are an iOS QA tester running inside a Bitrise RDE session.

Environment:
  ~/.qa-agent/wait-for-deps.sh  PRE-INSTALLED. Run it FIRST. It blocks until the simulator is created and booted and the app upload has stabilised, then writes /tmp/.qa-agent-info.json. The wait can take a few minutes on a cold Xcode — that's expected.
  /tmp/.qa-agent-info.json     written by wait-for-deps.sh: { udid, session_id }
  ~/.qa-agent/upload-path      written by wait-for-deps.sh: path to the uploaded app directory
  ~/.qa-agent/results/         PRE-CREATED. Save ALL artefacts here, FLAT. Bitrise's JUnit attachment convention requires attachment files to sit next to junit.xml — do NOT create subdirectories.

Smoke-test the uploaded app:
  0. Run ~/.qa-agent/wait-for-deps.sh and wait for it to exit 0. Don't begin any other work until it returns.
  1. Resolve UDID and the upload directory from the files above (jq -r .udid /tmp/.qa-agent-info.json, cat ~/.qa-agent/upload-path).
  2. Find the .app inside the upload directory. If it is an .ipa, unzip it first; the bundle is at Payload/*.app.
  3. xcrun simctl install $UDID <path-to-.app>
  4. Read CFBundleIdentifier from <.app>/Info.plist.
  5. xcrun simctl launch $UDID <bundle-id>
  6. Use the qa-agent MCP server (qa_screenshot, qa_click, qa_scroll, qa_type, qa_mouse_drag) to drive the simulator. Always call qa_screenshot first so the server knows the real display resolution; then tap visible primary buttons, scroll on each screen, and walk through any tab bar or menu. Save each screenshot directly into ~/.qa-agent/results/ as screenshot-NN-<short-tag>.png (zero-padded NN, lowercase tag, no spaces).
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
	createDeviceType           string
	createIOSVersion           string
	createXcodeVersion         string
	createWatchTimeout         time.Duration
	createWatchPoll            time.Duration
	createWaitForAgent         bool
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
	f.StringVar(&createQAPrompt, "qa-prompt", "", "QA Agent prompt. Sent on the session's ai_prompt field, which the codespaces backend exports as $AI_PROMPT to the inner script. "+
		"Any "+remotePathPlaceholder+" is substituted with the remote path of --upload before submission. "+
		"When omitted, a built-in smoke-test prompt is used (install + launch the uploaded app and exercise its UI).")
	f.StringVar(&createUpload, "upload", "", "Local file to upload to the session after it reaches RUNNING")
	f.StringVar(&createUploadDestination, "upload-destination", "/tmp/bitrise-ai-qa-agent", "Absolute remote directory the --upload file is extracted into. "+
		"Sent to the template as "+qaWatchDirInputKey+" so the watcher trigger and the upload destination stay in sync.")
	f.StringVar(&createDeviceType, "device-type", "", "DEVICE_TYPE session input. Simulator device passed to xcrun simctl create. Template default: \"iPhone 15\".")
	f.StringVar(&createIOSVersion, "ios-version", "", "IOS_VERSION session input. iOS runtime for the simulator. Template default: highest available.")
	f.StringVar(&createXcodeVersion, "xcode-version", "26.3", "XCODE_VERSION session input. Xcode version selected via DEVELOPER_DIR on the VM (e.g. \"26.3\" → /Applications/Xcode-26.3.app). Default matches the bitrise-ai-qa-agent template's pre-warmed Xcode; pinning to a beta version costs ~4min of CoreSimulator first-launch on session creation.")
	f.DurationVar(&createWatchTimeout, "watch-timeout", 0, "QA_WATCH_TIMEOUT_SEC session input. How long the in-VM watcher waits for the upload before giving up. Template default: 30m.")
	f.DurationVar(&createWatchPoll, "watch-poll", 0, "QA_WATCH_POLL_SEC session input. Watcher poll interval. Template default: 2s.")
	f.Int32Var(&createAutoTerminateMinutes, "auto-terminate-minutes", 60, "Minutes before auto-termination (0 disables; -1 leaves backend default). Defaults to 60 so a crashed CLI eventually frees the VM; pass 0 explicitly if you want sessions that don't auto-terminate.")
	f.BoolVar(&createMapSavedInputs, "map-saved-inputs", true, "Auto-fill template session inputs from caller's saved inputs")
	f.BoolVar(&createWait, "wait", true, "Poll until session reaches RUNNING")
	f.BoolVar(&createWaitForAgent, "wait-for-agent", true, "After --upload, also wait until the in-VM agent (Claude) has been launched (agent_session_status leaves UNSPECIFIED). Only meaningful when --upload is set.")
	f.DurationVar(&createPollInterval, "poll-interval", 5*time.Second, "Status poll interval for --wait and --wait-for-agent")
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

	if createXcodeVersion != "" && !xcodeVersionRE.MatchString(createXcodeVersion) {
		return fmt.Errorf("--xcode-version %q: must be MAJOR[.MINOR[.PATCH]] digits only (e.g. 26.3 or 16.4.1)", createXcodeVersion)
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
	inputs = ensureQAAgentInputs(inputs, createDeviceType, createIOSVersion, createXcodeVersion, createUploadDestination, createWatchTimeout, createWatchPoll)

	req := &codespaces.CreateSessionRequest{
		Name:                    createName,
		Description:             createDescription,
		TemplateID:              createTemplate,
		WorkspaceID:             createWorkspace,
		SessionInputs:           inputs,
		EnabledFeatureFlagNames: createFeatureFlags,
		Cluster:                 createCluster,
		AiPrompt:                qaPrompt,
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
	logf("created session %s (status: %s)", session.ID, session.Status)

	if createWait {
		session, err = client.WaitForRunning(ctx, session.ID, createWorkspace, createPollInterval, func(s codespaces.SessionStatus) {
			logf("  status: %s", s)
		})
		if err != nil {
			return err
		}
	}

	if createUpload != "" && session.Status == codespaces.SessionStatusRunning {
		actualPath, err := client.UploadFile(ctx, session.ID, createWorkspace, createUpload, createUploadDestination)
		if err != nil {
			return fmt.Errorf("upload %s: %w", createUpload, err)
		}
		logf("uploaded %s -> %s", createUpload, actualPath)

		if createWaitForAgent {
			logf("waiting for the in-VM launcher to start Claude in tmux (Claude will then run wait-for-deps.sh itself)...")
			if _, err := client.WaitForAgentLaunch(ctx, session.ID, createWorkspace, createPollInterval, func(s codespaces.AgentSessionStatus) {
				logf("  agent_session_status: %s", s)
			}); err != nil {
				return fmt.Errorf("waiting for agent launch: %w", err)
			}
			logf("Claude launched; the in-VM tmux session 'claude-auto' is live (the codespaces UI's 'open Claude' button picks it up; over SSH use `tmux attach -t claude-auto`)")
		}
	}

	if createOpenRemoteAccess && session.Status == codespaces.SessionStatusRunning {
		session, err = client.OpenRemoteAccess(ctx, session.ID, createWorkspace)
		if err != nil {
			return fmt.Errorf("OpenRemoteAccess: %w", err)
		}
		logf("ssh: %s (password: %s)", session.SSHAddress, session.SSHPassword)
		logf("vnc: %s (user: %s, password: %s)", session.VNCAddress, session.VNCUsername, session.VNCPassword)
	}

	fmt.Println(session.ID)
	if createUpload != "" {
		logf("when the QA run finishes, collect results + stop the VM with:")
		logf("  ai-qa-agent-cli session collect %s --workspace %s", session.ID, createWorkspace)
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
		logf("warning: --qa-prompt does not reference %s; ensure the prompt knows the file's path (%s)", remotePathPlaceholder, remote)
	}
	return prompt, remote, nil
}

// ensureQAAgentInputs forwards the QA Agent template's optional session
// inputs (DEVICE_TYPE, IOS_VERSION, watcher knobs) and always sends
// QA_WATCH_DIR — set to whatever --upload-destination is — so the watcher
// dir cannot drift from where the upload actually lands. Empty / zero values
// are skipped so the template's own defaults apply. Already-supplied inputs
// (via --input / --secret-input / --saved-input) win.
func ensureQAAgentInputs(
	inputs []*codespaces.SessionInputValue,
	deviceType, iosVersion, xcodeVersion, watchDir string,
	watchTimeout, watchPoll time.Duration,
) []*codespaces.SessionInputValue {
	have := func(key string) bool {
		for _, in := range inputs {
			if in.Key == key {
				return true
			}
		}
		return false
	}
	add := func(key, val string) {
		if val == "" || have(key) {
			return
		}
		inputs = append(inputs, &codespaces.SessionInputValue{Key: key, Value: val})
	}
	add(deviceTypeInputKey, deviceType)
	add(iosVersionInputKey, iosVersion)
	add(xcodeVersionInputKey, xcodeVersion)
	add(qaWatchDirInputKey, watchDir)
	if watchTimeout > 0 {
		add(qaWatchTimeoutSecInputKey, strconv.Itoa(int(watchTimeout.Seconds())))
	}
	if watchPoll > 0 {
		add(qaWatchPollSecInputKey, strconv.Itoa(int(watchPoll.Seconds())))
	}
	return inputs
}

func buildSessionInputs(plain, secret, saved []string) ([]*codespaces.SessionInputValue, error) {
	out := make([]*codespaces.SessionInputValue, 0, len(plain)+len(secret)+len(saved))

	for _, kv := range plain {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("--input %q: expected key=value", kv)
		}
		out = append(out, &codespaces.SessionInputValue{Key: k, Value: v})
	}
	for _, kv := range secret {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("--secret-input %q: expected key=value", kv)
		}
		out = append(out, &codespaces.SessionInputValue{Key: k, Value: v, IsSecret: true})
	}
	for _, kv := range saved {
		k, id, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("--saved-input %q: expected key=savedInputID", kv)
		}
		out = append(out, &codespaces.SessionInputValue{Key: k, SavedInputID: id})
	}
	return out, nil
}
