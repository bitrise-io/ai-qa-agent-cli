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
	// bitrisePATInputKey is the env var name the in-VM upload helper reads.
	// Sent as a secret session input so the codespaces backend exports it
	// into warmup.sh / startup.sh / watcher.sh on the VM. Used by the
	// `ai-qa-agent-cli upload-results` command from watcher.sh after Claude
	// exits.
	bitrisePATInputKey = "BITRISE_PAT"
)

// defaultQAPrompt is the SCAFFOLDING that wraps every QA run. Anything
// passed via --qa-prompt / --qa-prompt-file is appended after this and
// becomes the run's "USER TASK" — the specific scenario Claude must
// validate. With no user-supplied text, the prompt below falls back to a
// generic smoke test (app launch + screen navigation + crash watch).
const defaultQAPrompt = `You are an iOS QA tester running inside a Bitrise RDE session.

# ENVIRONMENT (already set up by the template; do NOT re-create any of these)
  ~/.qa-agent/wait-for-deps.sh  Run it FIRST. Blocks until the simulator is created+booted and the app upload has stabilised, then writes /tmp/.qa-agent-info.json. Can take a few minutes on a cold Xcode — that's expected.
  /tmp/.qa-agent-info.json     written by wait-for-deps.sh: { udid, session_id }
  ~/.qa-agent/upload-path      written by wait-for-deps.sh: path to the uploaded app directory
  ~/.qa-agent/results/         PRE-CREATED. Save ALL artefacts here, FLAT (no subdirs) — Bitrise's JUnit attachment convention requires attachments to sit next to junit.xml.
  qa-agent MCP server          Pre-registered. Tools: qa_screenshot, qa_click, qa_scroll, qa_type, qa_mouse_drag. ALWAYS call qa_screenshot first so the server learns the real display resolution; pass max_x / max_y matching the screenshot you'll click against.

# SETUP (always, regardless of task)
  1. Run ~/.qa-agent/wait-for-deps.sh. Block until it exits 0; if it errors, abort the run (write a single junit.xml testcase "infrastructure" with a <failure> noting the wait-for-deps failure, copy claude.log into results/, exit).
  2. UDID="$(jq -r .udid /tmp/.qa-agent-info.json)".
  3. UPLOAD_DIR="$(cat ~/.qa-agent/upload-path)". Find the .app inside it (unzip the .ipa first if needed; the bundle is at Payload/*.app).
  4. xcrun simctl install "$UDID" <path-to-.app>.
  5. BUNDLE_ID from <.app>/Info.plist (CFBundleIdentifier).
  6. xcrun simctl launch "$UDID" "$BUNDLE_ID".

# USER TASK
The text APPENDED to this prompt is the user's specific scenario for this run. Treat it as the source of truth for what to verify.

If no extra text was appended, fall back to a generic smoke test: navigate the app like a first-time user — tap obvious primary buttons, scroll on each screen, walk through any tab bar / menu — for ~10 interactions or until you hit a crash, an unexpected alert, or a stuck loading state.

While exercising the app:
  - Call qa_screenshot before every interaction so coordinates rescale correctly.
  - Save each screenshot into ~/.qa-agent/results/ as screenshot-NN-<short-tag>.png (zero-padded NN, lowercase tag, no spaces).
  - Note in your reasoning what you saw on each screen and whether the app behaved as expected for the user task. Be specific (matching text content, button states, error toasts).

# REPORTING (always, FLAT layout under ~/.qa-agent/results/)
  a. cp ~/.qa-agent/claude.log ~/.qa-agent/results/claude.log so failing testcases can attach it.

  b. summary.md — short prose: app launch result, screens reached, what you actually verified vs. the user task, anomalies. END the file with EXACTLY one of these two lines on its own line, with no trailing punctuation:
       OVERALL: PASS
       OVERALL: FAIL
     Use FAIL if any required testcase failed OR was skipped because of an upstream defect (e.g. you couldn't reach a screen because the app crashed).

  c. junit.xml — Surefire-style JUnit. ONE <testsuite name="QA Agent"> containing the testcases below.

# JUNIT TESTCASES — these REQUIRED cases always appear:
    - app_launch:        FAIL if simctl install or simctl launch errored, or the app process exited within 5s of launch.
    - screen_navigation: FAIL if you could not get past the first interactive screen (no tap registered, no transition).
    - no_crashes:        FAIL if you observed a crash, unexpected modal alert, or a load that never resolved.

  Add ADDITIONAL <testcase> elements for every distinct behaviour from the USER TASK that you actually verified or attempted (e.g. login_flow, cart_checkout, search_returns_results). One testcase per behaviour, named in snake_case.

# JUNIT PASS/FAIL DISCIPLINE — strict; do not soften:
  PASS  → <testcase> with NO <failure> and NO <skipped> child. Only emit a PASS if you ACTUALLY verified the behaviour with a concrete observation (a screenshot showing the expected state, a network request you watched succeed, etc.). If you didn't verify it, do NOT mark it PASS.
  FAIL  → <testcase> with a <failure message="<short>" type="<category>"><![CDATA[<details, what you saw vs expected>]]></failure> child. ALWAYS attach claude.log as one of the attachments when emitting a failure.
  SKIPPED → <testcase> with a <skipped message="<reason>"/> child. Use this when an upstream defect prevented you from exercising the case (e.g. login_flow skipped because app_launch failed). For the OVERALL verdict, a skipped REQUIRED case counts as a failure.

# JUNIT ATTACHMENT FORMAT (Bitrise convention — https://docs.bitrise.io/en/bitrise-ci/testing/deploying-and-viewing-test-results.html):
    <testcase name="app_launch" classname="QAAgent" time="3.2">
      <properties>
        <property name="attachment_1" value="screenshot-01-launch.png" />
      </properties>
    </testcase>

    <testcase name="login_flow" classname="QAAgent" time="12.4">
      <failure message="Login button disabled with valid credentials" type="UIBlocked"><![CDATA[Filled email + password matching the spec, the Sign In button stayed greyed out. See attachment_1 (form filled) and attachment_2 (full transcript).]]></failure>
      <properties>
        <property name="attachment_1" value="screenshot-04-login-disabled.png" />
        <property name="attachment_2" value="claude.log" />
      </properties>
    </testcase>

  - property name MUST be attachment_<N>, 1-based, contiguous per testcase.
  - value MUST be a bare filename (no path) you actually wrote into ~/.qa-agent/results/.
  - Allowed extensions: .jpg .jpeg .png .txt .log .mp4 .webm .ogg.
  - junit.xml MUST be well-formed XML (every tag balanced; CDATA properly closed). Don't reference attachments you didn't write.

Exit when junit.xml + summary.md are written.
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
	createQAPromptFile         string
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
	f.StringVar(&createQAPromptFile, "qa-prompt-file", "", "Path to a file whose contents are appended to the QA Agent prompt (after --qa-prompt if also set).")
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

	usingDefaultPrompt := createQAPrompt == "" && createQAPromptFile == ""
	var rawPrompt string
	if usingDefaultPrompt {
		rawPrompt = defaultQAPrompt
	} else {
		rawPrompt = defaultQAPrompt
		if createQAPrompt != "" {
			rawPrompt += "\n\n" + createQAPrompt
		}
		if createQAPromptFile != "" {
			fileBytes, err := os.ReadFile(createQAPromptFile)
			if err != nil {
				return fmt.Errorf("--qa-prompt-file: %w", err)
			}
			rawPrompt += "\n\n" + string(fileBytes)
		}
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
	// Forward the user's PAT to the VM so watcher.sh can upload the results
	// to bitrise-rde-qa-results after Claude exits. Marked secret so the
	// codespaces backend stores it encrypted and never logs the value.
	// Caller-supplied --secret-input BITRISE_PAT=… wins over this default.
	inputs = ensurePATInput(inputs, pat)

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
		if session.Status == codespaces.SessionStatusRunning {
			logf("UI: %s/%s#/sessions/%s", strings.TrimRight(flagUIBaseURL, "/"), createWorkspace, session.ID)
		}
	}

	if createUpload != "" && session.Status == codespaces.SessionStatusRunning {
		actualPath, err := client.UploadFile(ctx, session.ID, createWorkspace, createUpload, createUploadDestination)
		if err != nil {
			return fmt.Errorf("upload %s: %w", createUpload, err)
		}
		logf("uploaded %s -> %s", createUpload, actualPath)

		if createWaitForAgent {
			if _, err := client.WaitForAgentLaunch(ctx, session.ID, createWorkspace, createPollInterval, nil); err != nil {
				return fmt.Errorf("waiting for agent launch: %w", err)
			}
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

// ensurePATInput appends BITRISE_PAT as a secret session input unless the
// caller already supplied one (via --input / --secret-input / --saved-input)
// or pat is empty. The watcher on the VM uses this to authenticate the
// upload to the QA results visualisation service.
func ensurePATInput(inputs []*codespaces.SessionInputValue, pat string) []*codespaces.SessionInputValue {
	if pat == "" {
		return inputs
	}
	for _, in := range inputs {
		if in.Key == bitrisePATInputKey {
			return inputs
		}
	}
	return append(inputs, &codespaces.SessionInputValue{
		Key:      bitrisePATInputKey,
		Value:    pat,
		IsSecret: true,
	})
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
