package codespaces

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildTarGzPath_File(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hello.bin")
	if err := os.WriteFile(p, []byte("hi"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := readArchive(t, mustBuild(t, p))
	if len(got) != 1 || got[0].name != "hello.bin" || got[0].typ != tar.TypeReg {
		t.Fatalf("unexpected entries: %+v", got)
	}
}

func TestBuildTarGzPath_Directory(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "Hello.app")
	if err := os.MkdirAll(filepath.Join(app, "Sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Hello"), []byte("\x7fELF"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Info.plist"), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Sub", "leaf.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := readArchive(t, mustBuild(t, app))

	want := map[string]byte{
		"Hello.app":              tar.TypeDir,
		"Hello.app/Hello":        tar.TypeReg,
		"Hello.app/Info.plist":   tar.TypeReg,
		"Hello.app/Sub":          tar.TypeDir,
		"Hello.app/Sub/leaf.txt": tar.TypeReg,
	}
	got := map[string]byte{}
	for _, e := range entries {
		got[e.name] = e.typ
	}
	for n, w := range want {
		if got[n] != w {
			t.Errorf("entry %s: got typ %c, want %c", n, got[n], w)
		}
	}
}

type entry struct {
	name string
	typ  byte
	mode int64
}

func mustBuild(t *testing.T, p string) []byte {
	t.Helper()
	b, err := buildTarGzPath(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func readArchive(t *testing.T, raw []byte) []entry {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var out []entry
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, entry{name: h.Name, typ: h.Typeflag, mode: h.Mode})
	}
	return out
}
