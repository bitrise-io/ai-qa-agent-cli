// hello is a tiny example binary used to smoke-test
// `ai-qa-agent-cli session create --upload`. It prints a banner plus host info
// and (optionally) sleeps so Claude can observe it running.
package main

import (
	"fmt"
	"os"
	"runtime"
	"time"
)

func main() {
	host, _ := os.Hostname()
	fmt.Printf("hello from ai-qa-agent example binary\n")
	fmt.Printf("  host:      %s\n", host)
	fmt.Printf("  os/arch:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  pid:       %d\n", os.Getpid())
	fmt.Printf("  args:      %v\n", os.Args[1:])
	fmt.Printf("  time:      %s\n", time.Now().Format(time.RFC3339))

	if len(os.Args) > 1 && os.Args[1] == "--sleep" {
		fmt.Println("sleeping 30s so the agent can watch...")
		time.Sleep(30 * time.Second)
	}
	fmt.Println("done.")
}
