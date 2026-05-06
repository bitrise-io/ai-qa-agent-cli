package qamcp

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

//go:embed scripts/click.swift
var clickSwift string

//go:embed scripts/type.swift
var typeSwift string

//go:embed scripts/scroll.swift
var scrollSwift string

//go:embed scripts/mouse_drag.swift
var mouseDragSwift string

// compiledScript wraps a Swift source string and lazily compiles it on first
// use. The compiled binary path is cached for the lifetime of the process.
//
// On compile failure the cache stays empty so a subsequent call retries —
// helpful when the first call races a transient swiftc / DEVELOPER_DIR
// hiccup.
type compiledScript struct {
	name   string
	source string

	mu   sync.Mutex
	path string
}

var (
	clickBin     = &compiledScript{name: "click", source: clickSwift}
	typeBin      = &compiledScript{name: "type", source: typeSwift}
	scrollBin    = &compiledScript{name: "scroll", source: scrollSwift}
	mouseDragBin = &compiledScript{name: "mouse_drag", source: mouseDragSwift}
)

func (c *compiledScript) ensureCompiled(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.path != "" {
		if _, err := os.Stat(c.path); err == nil {
			return c.path, nil
		}
		c.path = ""
	}

	dir, err := os.MkdirTemp("", "ai-qa-agent-mcp-")
	if err != nil {
		return "", fmt.Errorf("mkdir temp: %w", err)
	}
	srcPath := filepath.Join(dir, c.name+".swift")
	if err := os.WriteFile(srcPath, []byte(c.source), 0o644); err != nil {
		return "", fmt.Errorf("write %s source: %w", c.name, err)
	}
	binPath := filepath.Join(dir, c.name)
	cmd := exec.CommandContext(ctx, "swiftc", srcPath, "-o", binPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("swiftc %s: %w: %s", c.name, err, out)
	}
	c.path = binPath
	return c.path, nil
}

// run compiles (if needed) and executes the script binary with args,
// returning its combined stdout+stderr.
func (c *compiledScript) run(ctx context.Context, args ...string) (string, error) {
	bin, err := c.ensureCompiled(ctx)
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w: %s", c.name, err, out)
	}
	return string(out), nil
}
