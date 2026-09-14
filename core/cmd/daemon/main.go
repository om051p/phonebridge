// Package main — phonebridge-daemon entrypoint.
// Status: PLANNED — Phase 0 hello-world only. No feature logic.

package main

import (
	"fmt"
	"os"
)

var version = "0.0.0-phase0"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-V") {
		fmt.Println(version)
		return
	}
	fmt.Printf("phonebridge-daemon %s — PLANNED (Phase 0 scaffold)\n", version)
	fmt.Printf("socket: %s/phonebridge/engine.sock (PLANNED)\n", xdgRuntimeDir())
}

func xdgRuntimeDir() string {
	if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
		return v
	}
	return "/run/user/1000"
}
