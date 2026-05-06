package codespaces

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	codespacesv1 "github.com/bitrise-io/bitrise-codespaces/backend/proto/codespaces/v1"
)

func (c *Client) CreateSession(ctx context.Context, req *codespacesv1.CreateSessionRequest) (*codespacesv1.Session, error) {
	if req.GetWorkspaceId() == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	var resp codespacesv1.CreateSessionResponse
	p := fmt.Sprintf("/v1/workspaces/%s/sessions", url.PathEscape(req.GetWorkspaceId()))
	if err := c.do(ctx, http.MethodPost, p, req, &resp); err != nil {
		return nil, err
	}
	return resp.GetSession(), nil
}

func (c *Client) getSession(ctx context.Context, sessionID, workspaceID string) (*codespacesv1.Session, error) {
	var resp codespacesv1.GetSessionResponse
	p := fmt.Sprintf("/v1/workspaces/%s/sessions/%s", url.PathEscape(workspaceID), url.PathEscape(sessionID))
	if err := c.do(ctx, http.MethodGet, p, nil, &resp); err != nil {
		return nil, err
	}
	return resp.GetSession(), nil
}

func (c *Client) OpenRemoteAccess(ctx context.Context, sessionID, workspaceID string) (*codespacesv1.Session, error) {
	var resp codespacesv1.OpenRemoteAccessResponse
	body := &codespacesv1.OpenRemoteAccessRequest{
		SessionId:   sessionID,
		WorkspaceId: workspaceID,
	}
	p := fmt.Sprintf("/v1/workspaces/%s/sessions/%s/open-remote-access", url.PathEscape(workspaceID), url.PathEscape(sessionID))
	if err := c.do(ctx, http.MethodPost, p, body, &resp); err != nil {
		return nil, err
	}
	return resp.GetSession(), nil
}

// WaitForRunning polls GetSession until the session reaches RUNNING (success),
// FAILED / ARCHIVED (error), or ctx is cancelled. onStatus is invoked once per
// observed status transition (including the first observation).
func (c *Client) WaitForRunning(
	ctx context.Context,
	sessionID, workspaceID string,
	interval time.Duration,
	onStatus func(codespacesv1.SessionStatus),
) (*codespacesv1.Session, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var last codespacesv1.SessionStatus = codespacesv1.SessionStatus_SESSION_STATUS_UNSPECIFIED
	for {
		s, err := c.getSession(ctx, sessionID, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("get session: %w", err)
		}
		if s.GetStatus() != last {
			last = s.GetStatus()
			if onStatus != nil {
				onStatus(last)
			}
		}
		switch s.GetStatus() {
		case codespacesv1.SessionStatus_SESSION_STATUS_RUNNING:
			return s, nil
		case codespacesv1.SessionStatus_SESSION_STATUS_FAILED:
			return s, fmt.Errorf("session %s entered FAILED state", sessionID)
		case codespacesv1.SessionStatus_SESSION_STATUS_ARCHIVED:
			return s, fmt.Errorf("session %s entered ARCHIVED state", sessionID)
		}

		select {
		case <-ctx.Done():
			return s, ctx.Err()
		case <-ticker.C:
		}
	}
}
