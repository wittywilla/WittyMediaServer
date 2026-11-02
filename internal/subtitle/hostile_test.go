package subtitle

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

// A subtitle file sits beside a video for anyone who can drop files in a library, and it's converted
// when someone opens the film. Lines in reverse order are the worst case for a sort that expects them
// in order, and one stamp can stand for many cues.
func TestLyricsFileCantTieUpTheServer(t *testing.T) {
	var b strings.Builder
	const lines = 200000
	for i := lines; i > 0; i-- {
		fmt.Fprintf(&b, "[%d:%02d.00]x\n", i/60%999, i%60)
	}
	start := time.Now()
	out, err := Convert("lrc", []byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 5*timeScale*time.Second {
		t.Errorf("%d lines in reverse took %v", lines, took)
	}
	if !bytes.Contains(out, []byte("-->")) {
		t.Error("nothing came out")
	}
}

func TestLyricsFileCantMakeAHugeAnswer(t *testing.T) {
	stamps := strings.Repeat("[00:01.00]", 64)
	in := stamps + strings.Repeat("word ", 4<<20) // 20 MB of words, said 64 times over
	out, err := Convert("lrc", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > 96<<20 {
		t.Errorf("%d MB from %d MB", len(out)>>20, len(in)>>20)
	}
}
