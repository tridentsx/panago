package cramfs

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

// TestZlibDeflateGroundTruth compares zlibDeflate's output against real
// firmware-captured cramfs blocks. Set PANAGO_BLOCK_DIRS to a
// colon-separated list of directories each containing blockN.raw/blockN.comp
// pairs (see project notes for how these were captured from a real
// PANAEUSB.FRM). Skipped if unset.
func TestZlibDeflateGroundTruth(t *testing.T) {
	dirsEnv := os.Getenv("PANAGO_BLOCK_DIRS")
	if dirsEnv == "" {
		t.Skip("PANAGO_BLOCK_DIRS not set")
	}

	var dirs []string
	start := 0
	for i := 0; i <= len(dirsEnv); i++ {
		if i == len(dirsEnv) || dirsEnv[i] == ':' {
			dirs = append(dirs, dirsEnv[start:i])
			start = i + 1
		}
	}

	total, matched := 0, 0
	var firstFails []string
	for _, dir := range dirs {
		for i := 0; ; i++ {
			rawPath := fmt.Sprintf("%s/block%d.raw", dir, i)
			compPath := fmt.Sprintf("%s/block%d.comp", dir, i)
			raw, err := os.ReadFile(rawPath)
			if err != nil {
				break
			}
			comp, err := os.ReadFile(compPath)
			if err != nil {
				break
			}
			total++
			got := zlibDeflate(raw)
			if bytes.Equal(got, comp) {
				matched++
			} else if len(firstFails) < 10 {
				n := len(got)
				if len(comp) < n {
					n = len(comp)
				}
				diffAt := -1
				for j := 0; j < n; j++ {
					if got[j] != comp[j] {
						diffAt = j
						break
					}
				}
				firstFails = append(firstFails, fmt.Sprintf("%s/block%d: got %d bytes, want %d bytes, firstDiff=%d", dir, i, len(got), len(comp), diffAt))
			}
		}
	}

	t.Logf("Matched %d / %d blocks", matched, total)
	for _, f := range firstFails {
		t.Logf("FAIL: %s", f)
	}
	if total == 0 {
		t.Fatal("no block pairs found")
	}
	if matched != total {
		t.Fatalf("%d / %d blocks did not match byte-for-byte", total-matched, total)
	}
}
