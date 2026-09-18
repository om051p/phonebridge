package rtpmedia

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// sliceAU is one access unit from the Spike 04 capture slice with the
// receiver-recorded metadata from the companion .idx sidecar
// (idx bytes rtp_ts idr marker spsN ppsN).
type sliceAU struct {
	idx    int
	data   []byte
	rtpTS  uint32
	idr    bool
	marker bool
	spsN   int
	ppsN   int
}

// loadSliceAUs reads the golden capture slice and its AU index from the
// package testdata directory.
func loadSliceAUs(base string) ([]sliceAU, error) {
	blob, err := os.ReadFile(base + ".h264")
	if err != nil {
		return nil, fmt.Errorf("read capture: %w", err)
	}
	f, err := os.Open(base + ".idx")
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	defer f.Close()

	var aus []sliceAU
	sc := bufio.NewScanner(f)
	offset := 0
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 7 {
			continue
		}
		var r [7]int
		ok := true
		for i := range r {
			v, err := strconv.Atoi(fields[i])
			if err != nil {
				ok = false
				break
			}
			r[i] = v
		}
		if !ok {
			continue
		}
		size := r[1]
		if offset+size > len(blob) {
			return nil, fmt.Errorf("AU %d: index sizes exceed capture", r[0])
		}
		aus = append(aus, sliceAU{
			idx:    r[0],
			data:   blob[offset : offset+size : offset+size],
			rtpTS:  uint32(uint64(r[2]) & 0xFFFFFFFF),
			idr:    r[3] == 1,
			marker: r[4] == 1,
			spsN:   r[5],
			ppsN:   r[6],
		})
		offset += size
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(aus) == 0 {
		return nil, fmt.Errorf("no AUs parsed from index")
	}
	return aus, nil
}
