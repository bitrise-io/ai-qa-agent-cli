// Package qaresults uploads QA agent result folders to the
// bitrise-rde-qa-results service and returns the resulting detail URL.
package qaresults

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultURL is the staging endpoint of the bitrise-rde-qa-results service.
const DefaultURL = "https://rde-qa-results.services.bitrise.dev/api/results"

// EnvURL is the env var that overrides DefaultURL.
const EnvURL = "BITRISE_RDE_QA_RESULTS_URL"

// Response is the shape of a successful POST /api/results response.
type Response struct {
	ID      string  `json:"id"`
	URL     string  `json:"url"`
	Summary Summary `json:"summary"`
}

// Summary mirrors parser.Summary on the server side.
type Summary struct {
	Total   int    `json:"total"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Skipped int    `json:"skipped"`
	Errored int    `json:"errored"`
	Status  string `json:"status"`
}

// Client uploads result folders.
type Client struct {
	URL    string
	Token  string
	HTTP   *http.Client
}

// New constructs a Client. If url is empty, EnvURL or DefaultURL is used in
// that order. token must be a Bitrise PAT.
func New(url, token string) *Client {
	if url == "" {
		if v := os.Getenv(EnvURL); v != "" {
			url = v
		} else {
			url = DefaultURL
		}
	}
	return &Client{
		URL:   url,
		Token: token,
		HTTP:  &http.Client{Timeout: 10 * time.Minute},
	}
}

// UploadDir packs the contents of dir as a flat tar.gz (no wrapper directory)
// and POSTs it to c.URL. Files at the top level of dir are placed at the
// archive root so the server's flat-folder convention is preserved.
//
// Files matching skipNames are skipped entirely. This is used to drop the
// agent's own scripts and logs (watcher.sh, sim-create.log, etc.) when the
// caller is uploading a full ~/.qa-agent/ tree. Pass nil to upload everything.
func (c *Client) UploadDir(ctx context.Context, dir string, skipNames map[string]bool) (*Response, error) {
	if c.Token == "" {
		return nil, errors.New("bitrise PAT is empty")
	}

	var buf bytes.Buffer
	if err := writeTarGz(&buf, dir, skipNames); err != nil {
		return nil, fmt.Errorf("pack %s: %w", dir, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, &buf)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("Accept", "application/json")
	req.ContentLength = int64(buf.Len())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", c.URL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("upload rejected (status %d): %s — check that BITRISE_PAT is valid", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var r Response
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("parse response: %w (body=%q)", err, string(body))
	}
	return &r, nil
}

// AbsoluteResultURL combines the upload URL's host with the relative URL the
// service returns (e.g. /results/<uuid>) into a clickable absolute URL.
func (c *Client) AbsoluteResultURL(rel string) string {
	if !strings.HasPrefix(rel, "/") {
		return rel
	}
	// c.URL points at .../api/results — strip the path to get the host root.
	if i := strings.Index(c.URL, "://"); i >= 0 {
		rest := c.URL[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			return c.URL[:i+3] + rest[:j] + rel
		}
		return c.URL + rel
	}
	return c.URL + rel
}

// writeTarGz packs the regular files at the top level of dir into a gzipped
// tar stream, with archive entries at the root (no wrapper dir). Symlinks
// and subdirectories are skipped.
func writeTarGz(w io.Writer, dir string, skipNames map[string]bool) error {
	gz := gzip.NewWriter(w)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}
	wrote := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if skipNames != nil && skipNames[name] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}

		hdr := &tar.Header{
			Name:    name,
			Mode:    int64(info.Mode().Perm()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("tar header %s: %w", name, err)
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("open %s: %w", name, err)
		}
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			return fmt.Errorf("write %s: %w", name, err)
		}
		f.Close()
		wrote++
	}
	if wrote == 0 {
		return fmt.Errorf("no regular files to upload from %s", dir)
	}
	return nil
}
