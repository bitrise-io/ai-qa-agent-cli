package codespaces

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	codespacesv1 "github.com/bitrise-io/bitrise-codespaces/backend/proto/codespaces/v1"
)

// DownloadDir asks the codespaces backend to tar+gzip sourcePath on the VM
// and upload it to GCS, then HTTP-GETs the returned signed URL and extracts
// the archive into destDir.
//
// onlyContents maps to the SessionDownload `only_contents_of_folder` flag.
// When true and sourcePath is a directory, the tar entries are relative to
// sourcePath itself rather than including sourcePath as the top-level dir —
// which is what we want when extracting into a session-specific destDir.
//
// destDir is created if missing. Tar entries are sanitised against absolute
// paths and `..` traversal.
func (c *Client) DownloadDir(
	ctx context.Context,
	sessionID, workspaceID, sourcePath, destDir string,
	onlyContents bool,
) ([]string, error) {
	if sourcePath == "" {
		return nil, fmt.Errorf("source path is required")
	}
	if destDir == "" {
		return nil, fmt.Errorf("destination directory is required")
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", destDir, err)
	}

	req := &codespacesv1.SessionDownloadRequest{
		SessionId:            sessionID,
		WorkspaceId:          workspaceID,
		SourcePath:           sourcePath,
		OnlyContentsOfFolder: onlyContents,
	}
	var resp codespacesv1.SessionDownloadResponse
	p := fmt.Sprintf("/v1/workspaces/%s/sessions/%s/download",
		url.PathEscape(workspaceID), url.PathEscape(sessionID))
	if err := c.do(ctx, http.MethodPost, p, req, &resp); err != nil {
		return nil, fmt.Errorf("SessionDownload: %w", err)
	}

	signedURL := resp.GetSignedUrl()
	if signedURL == "" {
		return nil, fmt.Errorf("SessionDownload returned an empty signed URL")
	}

	body, err := getSignedURL(ctx, signedURL)
	if err != nil {
		return nil, fmt.Errorf("fetch signed URL: %w", err)
	}
	defer body.Close()

	files, err := extractTarGz(body, destDir)
	if err != nil {
		return nil, fmt.Errorf("extract archive: %w", err)
	}
	return files, nil
}

func getSignedURL(ctx context.Context, signedURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("GET signed URL: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return resp.Body, nil
}

// extractTarGz extracts a tar.gz stream into destDir. Returns the relative
// paths of the regular files written. Rejects entries with absolute paths
// or `..` segments (Zip Slip).
func extractTarGz(r io.Reader, destDir string) ([]string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return files, fmt.Errorf("read tar: %w", err)
		}

		name, err := safeJoin(absDest, hdr.Name)
		if err != nil {
			return files, err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(name, fileMode(hdr.Mode, 0o755)); err != nil {
				return files, fmt.Errorf("mkdir %s: %w", name, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
				return files, fmt.Errorf("mkdir parent %s: %w", filepath.Dir(name), err)
			}
			f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode(hdr.Mode, 0o644))
			if err != nil {
				return files, fmt.Errorf("create %s: %w", name, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return files, fmt.Errorf("write %s: %w", name, err)
			}
			if err := f.Close(); err != nil {
				return files, fmt.Errorf("close %s: %w", name, err)
			}
			rel, _ := filepath.Rel(absDest, name)
			files = append(files, rel)
		case tar.TypeSymlink, tar.TypeLink:
			// Skip links — uncommon in result archives, and a link out of
			// destDir is the easiest way to break the safety invariant.
			continue
		default:
			// xattr / fifo / etc. — silently skip.
			continue
		}
	}
	return files, nil
}

// safeJoin resolves entry against base and rejects entries that are absolute
// or contain any `..` segment. We reject suspicious archives loudly rather
// than silently rewriting them onto disk under a sanitised name.
func safeJoin(base, entry string) (string, error) {
	if entry == "" {
		return "", fmt.Errorf("tar entry has empty name")
	}
	if filepath.IsAbs(entry) || path.IsAbs(entry) {
		return "", fmt.Errorf("tar entry %q is absolute", entry)
	}
	if slices.Contains(strings.Split(entry, "/"), "..") {
		return "", fmt.Errorf("tar entry %q contains traversal", entry)
	}
	full := filepath.Join(base, filepath.FromSlash(entry))
	rel, err := filepath.Rel(base, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("tar entry %q escapes destination", entry)
	}
	return full, nil
}

func fileMode(headerMode int64, fallback os.FileMode) os.FileMode {
	if headerMode <= 0 {
		return fallback
	}
	return os.FileMode(headerMode) & os.ModePerm
}

