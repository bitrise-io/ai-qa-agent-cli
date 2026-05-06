package codespaces

// JSON wire types for the bitrise-codespaces gRPC-gateway. Field names and
// enum values track what protojson serializes by default — lowerCamelCase
// fields, enums as their proto symbolic name (e.g. "SESSION_STATUS_RUNNING").
// We re-declare them as plain Go types here so the CLI doesn't need to
// import the (private) bitrise-codespaces module just to read a few fields
// off a JSON response.
//
// Only the subset of messages the CLI actually exchanges is modelled. If
// the contract grows, mirror the new fields here rather than re-introducing
// the proto dependency.

// SessionStatus mirrors `codespaces.v1.SessionStatus`.
type SessionStatus string

const (
	SessionStatusUnspecified SessionStatus = "SESSION_STATUS_UNSPECIFIED"
	SessionStatusPending     SessionStatus = "SESSION_STATUS_PENDING"
	SessionStatusStarting    SessionStatus = "SESSION_STATUS_STARTING"
	SessionStatusRunning     SessionStatus = "SESSION_STATUS_RUNNING"
	SessionStatusStopping    SessionStatus = "SESSION_STATUS_STOPPING"
	SessionStatusFailed      SessionStatus = "SESSION_STATUS_FAILED"
	SessionStatusArchived    SessionStatus = "SESSION_STATUS_ARCHIVED"
)

// AgentSessionStatus mirrors `codespaces.v1.AgentSessionStatus`.
type AgentSessionStatus string

const (
	AgentSessionStatusUnspecified     AgentSessionStatus = "AGENT_SESSION_STATUS_UNSPECIFIED"
	AgentSessionStatusWorking         AgentSessionStatus = "AGENT_SESSION_STATUS_WORKING"
	AgentSessionStatusWaitingForInput AgentSessionStatus = "AGENT_SESSION_STATUS_WAITING_FOR_INPUT"
	AgentSessionStatusIdle            AgentSessionStatus = "AGENT_SESSION_STATUS_IDLE"
)

// Session is a subset of `codespaces.v1.Session` — just the fields the CLI
// reads back from create / get / open-remote-access / stop responses.
type Session struct {
	ID                 string             `json:"id,omitempty"`
	Status             SessionStatus      `json:"status,omitempty"`
	SSHAddress         string             `json:"sshAddress,omitempty"`
	SSHPassword        string             `json:"sshPassword,omitempty"`
	VNCAddress         string             `json:"vncAddress,omitempty"`
	VNCUsername        string             `json:"vncUsername,omitempty"`
	VNCPassword        string             `json:"vncPassword,omitempty"`
	AgentSessionStatus AgentSessionStatus `json:"agentSessionStatus,omitempty"`
}

// SessionInputValue mirrors `codespaces.v1.SessionInputValue`.
type SessionInputValue struct {
	Key          string `json:"key,omitempty"`
	Value        string `json:"value,omitempty"`
	IsSecret     bool   `json:"isSecret,omitempty"`
	SavedInputID string `json:"savedInputId,omitempty"`
}

// CreateSessionRequest mirrors `codespaces.v1.CreateSessionRequest`. We only
// model the fields the CLI sets — fields we never populate (e.g. ai_prompt,
// which would trigger the backend's claudeAIAutoStart we deliberately bypass)
// are intentionally omitted.
type CreateSessionRequest struct {
	Name                    string               `json:"name,omitempty"`
	Description             string               `json:"description,omitempty"`
	TemplateID              string               `json:"templateId,omitempty"`
	WorkspaceID             string               `json:"workspaceId,omitempty"`
	SessionInputs           []*SessionInputValue `json:"sessionInputs,omitempty"`
	EnabledFeatureFlagNames []string             `json:"enabledFeatureFlagNames,omitempty"`
	Cluster                 string               `json:"cluster,omitempty"`
	// AutoTerminateMinutes is proto-optional; a *int32 lets us send 0
	// explicitly (disable auto-terminate) versus omit the field entirely
	// (use the backend's default).
	AutoTerminateMinutes    *int32 `json:"autoTerminateMinutes,omitempty"`
	MapSavedToSessionInputs bool   `json:"mapSavedToSessionInputs,omitempty"`
}

// CreateSessionResponse mirrors `codespaces.v1.CreateSessionResponse`. We
// only consume `session`; the response also carries `auto_mapped_inputs`
// which we currently ignore.
type CreateSessionResponse struct {
	Session *Session `json:"session,omitempty"`
}

type GetSessionResponse struct {
	Session *Session `json:"session,omitempty"`
}

type OpenRemoteAccessRequest struct {
	SessionID   string `json:"sessionId,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

type OpenRemoteAccessResponse struct {
	Session *Session `json:"session,omitempty"`
}

type StopSessionRequest struct {
	SessionID   string `json:"sessionId,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

type StopSessionResponse struct {
	Session *Session `json:"session,omitempty"`
}

type SessionStartUploadRequest struct {
	SessionID         string `json:"sessionId,omitempty"`
	WorkspaceID       string `json:"workspaceId,omitempty"`
	DestinationFolder string `json:"destinationFolder,omitempty"`
}

type SessionStartUploadResponse struct {
	SignedURL string `json:"signedUrl,omitempty"`
	UploadID  string `json:"uploadId,omitempty"`
}

type SessionCompleteUploadRequest struct {
	SessionID         string `json:"sessionId,omitempty"`
	UploadID          string `json:"uploadId,omitempty"`
	DestinationFolder string `json:"destinationFolder,omitempty"`
	WorkspaceID       string `json:"workspaceId,omitempty"`
}

type SessionDownloadRequest struct {
	SessionID            string `json:"sessionId,omitempty"`
	WorkspaceID          string `json:"workspaceId,omitempty"`
	SourcePath           string `json:"sourcePath,omitempty"`
	OnlyContentsOfFolder bool   `json:"onlyContentsOfFolder,omitempty"`
}

type SessionDownloadResponse struct {
	SignedURL string `json:"signedUrl,omitempty"`
}
