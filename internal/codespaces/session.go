package codespaces

import (
	"context"
	"fmt"
	"time"

	codespacesv1 "github.com/bitrise-io/bitrise-codespaces/backend/proto/codespaces/v1"
)

func (c *Client) CreateSession(ctx context.Context, req *codespacesv1.CreateSessionRequest) (*codespacesv1.Session, error) {
	resp, err := c.Service.CreateSession(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetSession(), nil
}

func (c *Client) OpenRemoteAccess(ctx context.Context, sessionID, workspaceID string) (*codespacesv1.Session, error) {
	resp, err := c.Service.OpenRemoteAccess(ctx, &codespacesv1.OpenRemoteAccessRequest{
		SessionId:   sessionID,
		WorkspaceId: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetSession(), nil
}

// WaitForRunning polls GetSession until the session reaches RUNNING (success),
// FAILED (error), or ctx is cancelled. onStatus is invoked once per observed
// status transition (including the first observation).
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
		resp, err := c.Service.GetSession(ctx, &codespacesv1.GetSessionRequest{
			SessionId:   sessionID,
			WorkspaceId: workspaceID,
		})
		if err != nil {
			return nil, fmt.Errorf("get session: %w", err)
		}
		s := resp.GetSession()
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
