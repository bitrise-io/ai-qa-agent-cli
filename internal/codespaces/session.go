package codespaces

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func (c *Client) CreateSession(ctx context.Context, req *CreateSessionRequest) (*Session, error) {
	if req.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	var resp CreateSessionResponse
	p := fmt.Sprintf("/v1/workspaces/%s/sessions", url.PathEscape(req.WorkspaceID))
	if err := c.do(ctx, http.MethodPost, p, req, &resp); err != nil {
		return nil, err
	}
	return resp.Session, nil
}

func (c *Client) getSession(ctx context.Context, sessionID, workspaceID string) (*Session, error) {
	var resp GetSessionResponse
	p := fmt.Sprintf("/v1/workspaces/%s/sessions/%s", url.PathEscape(workspaceID), url.PathEscape(sessionID))
	if err := c.do(ctx, http.MethodGet, p, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Session, nil
}

func (c *Client) OpenRemoteAccess(ctx context.Context, sessionID, workspaceID string) (*Session, error) {
	var resp OpenRemoteAccessResponse
	body := &OpenRemoteAccessRequest{
		SessionID:   sessionID,
		WorkspaceID: workspaceID,
	}
	p := fmt.Sprintf("/v1/workspaces/%s/sessions/%s/open-remote-access", url.PathEscape(workspaceID), url.PathEscape(sessionID))
	if err := c.do(ctx, http.MethodPost, p, body, &resp); err != nil {
		return nil, err
	}
	return resp.Session, nil
}

func (c *Client) StopSession(ctx context.Context, sessionID, workspaceID string) (*Session, error) {
	var resp StopSessionResponse
	body := &StopSessionRequest{
		SessionID:   sessionID,
		WorkspaceID: workspaceID,
	}
	p := fmt.Sprintf("/v1/workspaces/%s/sessions/%s/stop", url.PathEscape(workspaceID), url.PathEscape(sessionID))
	if err := c.do(ctx, http.MethodPost, p, body, &resp); err != nil {
		return nil, err
	}
	return resp.Session, nil
}

// WaitForAgentIdle polls GetSession until the in-VM agent reaches IDLE (i.e.
// Claude has fired its Stop hook and the codespaces backend has flipped
// agent_session_status to AGENT_SESSION_STATUS_IDLE). Returns on the first
// observed IDLE.
//
// We deliberately gate IDLE acceptance on having seen at least one
// non-UNSPECIFIED status first — a freshly-created session is UNSPECIFIED,
// and our template's watcher fires AGENT_WORKING right after launching
// Claude, so the gate ensures we don't false-positive on the initial zero
// value before Claude has even started.
//
// onStatus is invoked once per observed agent-status transition (including
// the first observation). Errors out if the session itself enters FAILED or
// ARCHIVED, or if ctx is cancelled.
func (c *Client) WaitForAgentIdle(
	ctx context.Context,
	sessionID, workspaceID string,
	interval time.Duration,
	onStatus func(AgentSessionStatus),
) (*Session, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	last := AgentSessionStatusUnspecified
	seenNonUnspecified := false
	for {
		s, err := c.getSession(ctx, sessionID, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("get session: %w", err)
		}

		switch s.Status {
		case SessionStatusFailed:
			return s, fmt.Errorf("session %s entered FAILED state while waiting for agent", sessionID)
		case SessionStatusArchived:
			return s, fmt.Errorf("session %s was archived while waiting for agent", sessionID)
		}

		ag := agentStatusOrUnspecified(s.AgentSessionStatus)
		if ag != last {
			last = ag
			if onStatus != nil {
				onStatus(last)
			}
		}
		if ag != AgentSessionStatusUnspecified {
			seenNonUnspecified = true
		}
		if seenNonUnspecified && ag == AgentSessionStatusIdle {
			return s, nil
		}

		select {
		case <-ctx.Done():
			return s, ctx.Err()
		case <-ticker.C:
		}
	}
}

// WaitForRunning polls GetSession until the session reaches RUNNING (success),
// FAILED / ARCHIVED (error), or ctx is cancelled. onStatus is invoked once per
// observed status transition (including the first observation).
func (c *Client) WaitForRunning(
	ctx context.Context,
	sessionID, workspaceID string,
	interval time.Duration,
	onStatus func(SessionStatus),
) (*Session, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	last := SessionStatusUnspecified
	for {
		s, err := c.getSession(ctx, sessionID, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("get session: %w", err)
		}
		curr := sessionStatusOrUnspecified(s.Status)
		if curr != last {
			last = curr
			if onStatus != nil {
				onStatus(last)
			}
		}
		switch curr {
		case SessionStatusRunning:
			return s, nil
		case SessionStatusFailed:
			return s, fmt.Errorf("session %s entered FAILED state", sessionID)
		case SessionStatusArchived:
			return s, fmt.Errorf("session %s entered ARCHIVED state", sessionID)
		}

		select {
		case <-ctx.Done():
			return s, ctx.Err()
		case <-ticker.C:
		}
	}
}

// protojson with EmitUnpopulated=false omits zero-valued enums entirely, so
// a freshly-created session may return JSON with no `status` / `agentSessionStatus`
// field at all. Normalize the empty string to the explicit UNSPECIFIED constant
// so the wait loops compare cleanly and the onStatus callback prints something
// meaningful on the first observation.

func sessionStatusOrUnspecified(s SessionStatus) SessionStatus {
	if s == "" {
		return SessionStatusUnspecified
	}
	return s
}

func agentStatusOrUnspecified(s AgentSessionStatus) AgentSessionStatus {
	if s == "" {
		return AgentSessionStatusUnspecified
	}
	return s
}
