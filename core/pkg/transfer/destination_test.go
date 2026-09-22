package transfer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

func newTestDestination(t *testing.T) *FileDestination {
	t.Helper()
	dest, err := NewFileDestination(FileDestinationConfig{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	return dest
}

func TestFileDestinationStagesThenCommits(t *testing.T) {
	dest := newTestDestination(t)

	committer, err := dest.Begin(Meta{TransferID: "1111", Filename: "notes.txt", SizeBytes: 4})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := committer.Write([]byte("da")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := committer.Write([]byte("ta")); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Nothing may be visible at the destination before Commit (DEC-024).
	if entries := destEntries(t, dest.Dir()); len(entries) != 0 {
		t.Fatalf("staged bytes are visible before commit: %v", entries)
	}
	staged, err := os.ReadDir(dest.PartialDir())
	if err != nil || len(staged) != 1 {
		t.Fatalf("staging directory should hold exactly one partial: %v (%v)", staged, err)
	}

	name, err := committer.Commit()
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if name != "notes.txt" {
		t.Fatalf("saved name = %q, want notes.txt", name)
	}
	if got := readFile(t, filepath.Join(dest.Dir(), "notes.txt")); string(got) != "data" {
		t.Fatalf("committed content = %q, want data", got)
	}
	assertNoStagedPartials(t, dest)

	// A committer is single-use.
	if _, err := committer.Commit(); err == nil {
		t.Fatalf("a second commit must fail")
	}
}

func TestFileDestinationAbortRemovesEverything(t *testing.T) {
	dest := newTestDestination(t)

	committer, err := dest.Begin(Meta{TransferID: "2222", Filename: "partial.bin", SizeBytes: 100})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := committer.Write([]byte("garbage")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := committer.Abort(); err != nil {
		t.Fatalf("abort: %v", err)
	}
	if entries := destEntries(t, dest.Dir()); len(entries) != 0 {
		t.Fatalf("abort left destination entries: %v", entries)
	}
	assertNoStagedPartials(t, dest)

	// Abort is idempotent.
	if err := committer.Abort(); err != nil {
		t.Fatalf("second abort: %v", err)
	}
	// Commit after abort is refused.
	if _, err := committer.Commit(); err == nil {
		t.Fatalf("commit after abort must fail")
	}
}

func TestFileDestinationCollisionRename(t *testing.T) {
	dest := newTestDestination(t)

	if err := os.WriteFile(filepath.Join(dest.Dir(), "report.pdf"), []byte("first"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for i, want := range []string{"report (1).pdf", "report (2).pdf"} {
		committer, err := dest.Begin(Meta{TransferID: string(rune('a' + i)), Filename: "report.pdf", SizeBytes: 1})
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if _, err := committer.Write([]byte("x")); err != nil {
			t.Fatalf("write: %v", err)
		}
		name, err := committer.Commit()
		if err != nil {
			t.Fatalf("commit: %v", err)
		}
		if name != want {
			t.Fatalf("collision rename = %q, want %q", name, want)
		}
	}
	if got := readFile(t, filepath.Join(dest.Dir(), "report.pdf")); string(got) != "first" {
		t.Fatalf("collision rename must not overwrite the existing file: %q", got)
	}
}

func TestFileDestinationSweepRemovesStalePartials(t *testing.T) {
	dest := newTestDestination(t)

	committer, err := dest.Begin(Meta{TransferID: "3333", Filename: "stale.bin", SizeBytes: 10})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := committer.Write([]byte("stale")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := dest.SweepPartial(); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	assertNoStagedPartials(t, dest)

	// The swept committer must not resurrect the file.
	_ = committer.Abort()
	if entries := destEntries(t, dest.Dir()); len(entries) != 0 {
		t.Fatalf("sweep left destination entries: %v", entries)
	}
}

func TestFileDestinationRefusesAbsurdSize(t *testing.T) {
	dest := newTestDestination(t)

	// 4 EiB cannot fit on any real filesystem: the preflight must refuse with a
	// typed storage failure instead of filling the disk and failing mid-transfer.
	_, err := dest.Begin(Meta{TransferID: "4444", Filename: "huge.bin", SizeBytes: 1 << 62})
	if err == nil {
		t.Fatalf("absurd size must be refused")
	}
	f, ok := IsFailure(err)
	if !ok || f.Code != phonebridgev1.Code_CODE_STORAGE_FAILED {
		t.Fatalf("error = %v, want CODE_STORAGE_FAILED", err)
	}
	if entries := destEntries(t, dest.Dir()); len(entries) != 0 {
		t.Fatalf("a refused transfer must write nothing: %v", entries)
	}
}

func TestFileDestinationRefusesUnsafeName(t *testing.T) {
	dest := newTestDestination(t)
	if _, err := dest.Begin(Meta{TransferID: "5555", Filename: "../escape.bin", SizeBytes: 1}); err == nil {
		t.Fatalf("unsafe filename must be refused")
	}
	outside := filepath.Join(filepath.Dir(dest.Dir()), "escape.bin")
	if _, err := os.Stat(outside); err == nil {
		t.Fatalf("path traversal wrote outside the destination: %s", outside)
	}
}

func TestFileDestinationKeepsStagingInsideDestinationFilesystem(t *testing.T) {
	dest := newTestDestination(t)
	if filepath.Dir(dest.PartialDir()) != dest.Dir() {
		t.Fatalf("staging dir %q must live inside %q for an atomic rename", dest.PartialDir(), dest.Dir())
	}
	fi, err := os.Stat(dest.PartialDir())
	if err != nil {
		t.Fatalf("stat staging dir: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Fatalf("staging dir permissions = %o, want 700", perm)
	}
}
