package cmd

import (
	"fmt"
	"os"
	"time"
)

// logf writes a timestamped progress line to stderr. Used by `session create`
// and `session collect` so a long-running command (e.g. waiting for IDLE)
// shows timing information when its output is piped to a file or scrollback.
//
// Command results that scripts may want to consume (session id, extracted
// dest dir) still go to stdout via fmt.Println — those stay untimestamped.
func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}
