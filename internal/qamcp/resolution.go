package qamcp

import "sync"

// Resolution is the pixel dimensions of the captured display.
type Resolution struct {
	Width  int
	Height int
}

// defaultResolution is what coordinate rescaling falls back to before the
// first screenshot has populated the cache. 1920×1080 matches the standard
// macOS RDE session image.
var defaultResolution = Resolution{Width: 1920, Height: 1080}

var (
	resMu      sync.RWMutex
	resCurrent = defaultResolution
)

// SetScreenResolution stores the most recently observed display resolution
// for use by click/mouse_drag rescaling.
func SetScreenResolution(r Resolution) {
	resMu.Lock()
	defer resMu.Unlock()
	resCurrent = r
}

// GetScreenResolution returns the most recently observed display resolution,
// or defaultResolution if no screenshot has been captured yet in this
// process.
func GetScreenResolution() Resolution {
	resMu.RLock()
	defer resMu.RUnlock()
	return resCurrent
}
