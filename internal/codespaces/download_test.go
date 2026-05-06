package codespaces

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func writeTarGz(t *testing.T, entries map[string]string, dirs []string, traversal []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, d := range dirs {
		if err := tw.WriteHeader(&tar.Header{Name: d, Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
			t.Fatalf("write dir header: %v", err)
		}
	}
	for name, body := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(body)),
		}); err != nil {
			t.Fatalf("write reg header: %v", err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write reg body: %v", err)
		}
	}
	for _, name := range traversal {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     5,
		}); err != nil {
			t.Fatalf("write traversal header: %v", err)
		}
		if _, err := tw.Write([]byte("evil!")); err != nil {
			t.Fatalf("write traversal body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gz: %v", err)
	}
	return buf.Bytes()
}

func TestExtractTarGz_HappyPath(t *testing.T) {
	t.Parallel()
	dest := t.TempDir()
	archive := writeTarGz(t,
		map[string]string{
			"junit.xml":              `<testsuite/>`,
			"screenshot-01-home.png": "PNG-DATA",
			"results/summary.md":     "# summary",
		},
		[]string{"results/"},
		nil,
	)

	files, err := extractTarGz(bytes.NewReader(archive), dest)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("want 3 files, got %d (%v)", len(files), files)
	}

	for path, want := range map[string]string{
		"junit.xml":              `<testsuite/>`,
		"screenshot-01-home.png": "PNG-DATA",
		"results/summary.md":     "# summary",
	} {
		got, err := os.ReadFile(filepath.Join(dest, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(got) != want {
			t.Errorf("%s: got %q, want %q", path, string(got), want)
		}
	}
}

func TestExtractTarGz_RejectsTraversal(t *testing.T) {
	t.Parallel()
	cases := []string{
		"../escape.txt",
		"results/../../escape.txt",
		"/etc/passwd",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			dest := t.TempDir()
			archive := writeTarGz(t, nil, nil, []string{name})
			_, err := extractTarGz(bytes.NewReader(archive), dest)
			if err == nil {
				t.Fatalf("expected error for traversal entry %q, got nil", name)
			}
		})
	}
}

func TestExtractTarGz_SkipsLinks(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "real.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{
		Name:     "link.txt",
		Typeflag: tar.TypeSymlink,
		Linkname: "/etc/passwd",
		Mode:     0o644,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	files, err := extractTarGz(&buf, dest)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(files) != 1 || files[0] != "real.txt" {
		t.Fatalf("want only real.txt, got %v", files)
	}
	if _, err := os.Lstat(filepath.Join(dest, "link.txt")); !os.IsNotExist(err) {
		t.Errorf("symlink should have been skipped; got Lstat err=%v", err)
	}
}
