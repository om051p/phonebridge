// missingffmpeg — Spike 07 P4 probe.
//
// Demonstrates what the production sinks return when the ffmpeg family is
// absent from PATH: typed errors from exec.LookPath, which the session
// manager's sink-selection chain converts into a NullSink fallback (session
// still starts; UI reports SINK_KIND_NULL). No production code is modified.
package main

import (
	"fmt"
	"os"

	"github.com/om051p/phonebridge/core/pkg/receiver"
)

func main() {
	// Simulate a host without the ffmpeg package.
	_ = os.Setenv("PATH", "/nonexistent-bin")

	_, err := receiver.NewDisplaySink("P4 probe", true)
	fmt.Printf("NewDisplaySink (ffplay) err: %v\n", err)

	_, err = receiver.NewPipeSink("ffmpeg", "-version")
	fmt.Printf("NewPipeSink (ffmpeg) err: %v\n", err)

	_, err = receiver.NewFFmpegVerifySink()
	fmt.Printf("NewFFmpegVerifySink (ffmpeg) err: %v\n", err)

	// The FileSink/NullSink paths do not need ffmpeg at all.
	fs := receiver.NewNullSink()
	fmt.Printf("NullSink usable without ffmpeg: %v\n", fs != nil)
}
