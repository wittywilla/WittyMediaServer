package diagnose

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnoseAudioCD(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 2352*75*8), 0o600); err != nil {
		t.Fatal(err)
	}
	cue := filepath.Join(dir, "a.cue")
	sheet := "FILE \"a.bin\" BINARY\n  TRACK 01 AUDIO\n    INDEX 01 00:00:00\n  TRACK 02 AUDIO\n    INDEX 01 00:04:00\n"
	if err := os.WriteFile(cue, []byte(sheet), 0o600); err != nil {
		t.Fatal(err)
	}
	j := testJob()
	if !checkCD(context.Background(), j, cue) {
		t.Fatal("not taken for a CD")
	}
	if !hasFinding(j, SevOK, "This is an audio CD image") {
		t.Fatal(listFindings(j))
	}

	// A data disc goes on to the other checks.
	sheet = "FILE \"a.bin\" BINARY\n  TRACK 01 MODE2/2352\n    INDEX 01 00:00:00\n"
	if err := os.WriteFile(cue, []byte(sheet), 0o600); err != nil {
		t.Fatal(err)
	}
	if checkCD(context.Background(), testJob(), cue) {
		t.Fatal("a game's disc was taken for music")
	}
}
