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

// UploadFile ships a local file or directory to the session's VM.
// The path is packed into a gzipped tar (preserving file modes and symlinks)
// and uploaded via the backend's signed PUT URL flow. Returns the absolute
// remote path it lands at after extraction.
//
// destFolder must be an absolute path; the server enforces this anyway, but
// we validate locally for a faster failure.
func (c *Client) UploadFile(
	ctx context.Context,
	sessionID, workspaceID string,
	localPath, destFolder string,
) (string, error) {
	if !path.IsAbs(destFolder) {
		return "", fmt.Errorf("destination folder must be absolute, got %q", destFolder)
	}

	if _, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("stat %s: %w", localPath, err)
	}

	archive, err := buildTarGzPath(localPath)
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

// buildTarGzPath packs localPath into a gzipped tar. For a file, the archive
// contains one regular entry named filepath.Base(localPath). For a directory,
// every entry under it is packed with names relative to the directory's parent
// — so after `tar -xzf` into <dest>, the contents land at <dest>/<basename>/...
func buildTarGzPath(localPath string) ([]byte, error) {
	clean := filepath.Clean(localPath)
	parent := filepath.Dir(clean)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	walkErr := filepath.Walk(clean, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		var link string
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(p)
			if err != nil {
				return fmt.Errorf("readlink %s: %w", p, err)
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return fmt.Errorf("tar header %s: %w", p, err)
		}
		rel, err := filepath.Rel(parent, p)
		if err != nil {
			return fmt.Errorf("rel %s: %w", p, err)
		}
		hdr.Name = filepath.ToSlash(rel)

		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write header %s: %w", p, err)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("open %s: %w", p, err)
		}
		_, copyErr := io.Copy(tw, f)
		f.Close()
		if copyErr != nil {
			return fmt.Errorf("copy %s: %w", p, copyErr)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
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
