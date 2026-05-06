package codespaces

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"

	codespacesv1 "github.com/bitrise-io/bitrise-codespaces/backend/proto/codespaces/v1"
)

// UploadFile ships a single local file to the session's VM.
// The file is packed into a gzipped tar containing one entry (mode 0755) and
// uploaded via the backend's signed PUT URL flow. Returns the absolute remote
// path the file lands at after extraction.
//
// destFolder must be an absolute path; the server enforces this anyway, but we
// validate locally for a faster failure.
func (c *Client) UploadFile(
	ctx context.Context,
	sessionID, workspaceID string,
	localPath, destFolder string,
) (string, error) {
	if !path.IsAbs(destFolder) {
		return "", fmt.Errorf("destination folder must be absolute, got %q", destFolder)
	}

	f, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", localPath, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", localPath, err)
	}

	archive, err := buildTarGz(filepath.Base(localPath), stat.Size(), f)
	if err != nil {
		return "", fmt.Errorf("build archive: %w", err)
	}

	startReq := &codespacesv1.SessionStartUploadRequest{
		SessionId:         sessionID,
		WorkspaceId:       workspaceID,
		DestinationFolder: destFolder,
	}
	var startResp codespacesv1.SessionStartUploadResponse
	startPath := fmt.Sprintf("/v1/workspaces/%s/sessions/%s/start-upload",
		url.PathEscape(workspaceID), url.PathEscape(sessionID))
	if err := c.do(ctx, http.MethodPost, startPath, startReq, &startResp); err != nil {
		return "", fmt.Errorf("SessionStartUpload: %w", err)
	}

	if err := putToSignedURL(ctx, startResp.GetSignedUrl(), archive); err != nil {
		return "", fmt.Errorf("upload archive: %w", err)
	}

	completeReq := &codespacesv1.SessionCompleteUploadRequest{
		SessionId:         sessionID,
		WorkspaceId:       workspaceID,
		UploadId:          startResp.GetUploadId(),
		DestinationFolder: destFolder,
	}
	completePath := fmt.Sprintf("/v1/workspaces/%s/sessions/%s/complete-upload",
		url.PathEscape(workspaceID), url.PathEscape(sessionID))
	if err := c.do(ctx, http.MethodPost, completePath, completeReq, nil); err != nil {
		return "", fmt.Errorf("SessionCompleteUpload: %w", err)
	}

	return path.Join(destFolder, filepath.Base(localPath)), nil
}

func buildTarGz(name string, size int64, body io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     0o755,
		Size:     size,
		Typeflag: tar.TypeReg,
	}); err != nil {
		return nil, err
	}
	if _, err := io.Copy(tw, body); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func putToSignedURL(ctx context.Context, signedURL string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, signedURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(body))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("PUT signed URL: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
