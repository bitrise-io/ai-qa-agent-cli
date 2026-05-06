package codespaces

import (
	"context"
	"crypto/tls"
	"fmt"

	codespacesv1 "github.com/bitrise-io/bitrise-codespaces/backend/proto/codespaces/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn    *grpc.ClientConn
	Service codespacesv1.CodespacesServiceClient
}

func NewClient(endpoint, pat string, useInsecure bool) (*Client, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	if pat == "" {
		return nil, fmt.Errorf("PAT is required")
	}

	var transport credentials.TransportCredentials
	if useInsecure {
		transport = insecure.NewCredentials()
	} else {
		transport = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}

	conn, err := grpc.NewClient(
		endpoint,
		grpc.WithTransportCredentials(transport),
		grpc.WithPerRPCCredentials(bearerCreds{token: pat, requireTLS: !useInsecure}),
	)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", endpoint, err)
	}

	return &Client{
		conn:    conn,
		Service: codespacesv1.NewCodespacesServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

type bearerCreds struct {
	token      string
	requireTLS bool
}

func (b bearerCreds) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + b.token}, nil
}

func (b bearerCreds) RequireTransportSecurity() bool { return b.requireTLS }
