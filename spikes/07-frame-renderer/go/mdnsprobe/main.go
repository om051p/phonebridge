// mdnsprobe — Spike 07 gate-3 probe.
//
// Uses the repo's own discovery package (read-only) to answer one question:
// is the POCO phone discoverable on this LAN right now?
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/om051p/phonebridge/core/pkg/discovery"
)

func main() {
	secs := flag.Int("t", 15, "seconds to browse")
	flag.Parse()

	disc, err := discovery.NewDiscovery(discovery.Config{
		DeviceID:        "p7-probe",
		DeviceName:      "p7-probe",
		Port:            0,
		IncludeLoopback: true,
		Version:         "1",
		Capabilities:    []string{"SCREEN"},
	})
	if err != nil {
		fmt.Printf("NewDiscovery error: %v\n", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*secs)*time.Second)
	defer cancel()
	go func() { _ = disc.Start(ctx) }()
	defer disc.Close()

	seen := map[string]bool{}
	deadline := time.Now().Add(time.Duration(*secs) * time.Second)
	for time.Now().Before(deadline) {
		for _, d := range disc.Registry().List() {
			if !seen[d.ID] {
				seen[d.ID] = true
				fmt.Printf("FOUND %+v\n", d)
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(seen) == 0 {
		fmt.Printf("NO DEVICES after %ds\n", *secs)
	}
}
