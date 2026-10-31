package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaplayer/engine/internal/cdrip"
)

// makeCue writes a two-track CD image of made-up samples.
func makeCue(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	pcm := make([]byte, 300*cdrip.SectorBytes)
	for i := range pcm {
		pcm[i] = byte(i * 7)
	}
	if err := os.WriteFile(filepath.Join(dir, "d.bin"), pcm, 0o644); err != nil {
		t.Fatal(err)
	}
	cue := "TITLE \"Album\"\nFILE \"d.bin\" BINARY\n  TRACK 01 AUDIO\n    TITLE \"One\"\n    INDEX 01 00:00:00\n  TRACK 02 AUDIO\n    TITLE \"Two\"\n    INDEX 01 00:02:00\n"
	path := filepath.Join(dir, "d.cue")
	if err := os.WriteFile(path, []byte(cue), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, pcm
}

func TestOpenSaysItsACD(t *testing.T) {
	cue, _ := makeCue(t)
	c := startHelper(t)
	var reply openReply
	if _, err := c.Call("open", map[string]any{"path": cue}, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Kind != "audiocd" || reply.CD == nil || len(reply.CD.Tracks) != 2 || reply.CD.Title != "Album" || reply.CD.Tracks[1].Title != "Two" || reply.CD.DiscID == "" {
		t.Errorf("%+v", reply)
	}
	// A cue sheet that's nothing of the kind gets a plain answer.
	bad := filepath.Join(t.TempDir(), "x.cue")
	os.WriteFile(bad, []byte("nonsense"), 0o644)
	if _, err := c.Call("open", map[string]any{"path": bad}, &reply); err == nil {
		t.Error("that isn't a CD")
	}
}

func TestRipProtocol(t *testing.T) {
	cue, pcm := makeCue(t)
	var out bytes.Buffer
	if code := runCDRip([]string{cue, "2,1", "secure", "0"}, nil, &out); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	r := bufio.NewReader(&out)
	got := map[int][]byte{}
	var order []int
	var dones int
	finished := false
	for {
		line, err := r.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		var ev struct {
			Event  string        `json:"event"`
			Track  int           `json:"track"`
			Bytes  int           `json:"bytes"`
			Report *cdrip.Report `json:"report"`
		}
		if err := json.Unmarshal(line, &ev); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		switch ev.Event {
		case "track":
			order = append(order, ev.Track)
		case "pcm":
			p := make([]byte, ev.Bytes)
			io.ReadFull(r, p)
			got[ev.Track] = append(got[ev.Track], p...)
		case "done":
			dones++
			if ev.Report == nil || ev.Report.CRC == "" {
				t.Errorf("no report: %s", line)
			}
		case "finished":
			finished = true
		}
	}
	if fmt.Sprint(order) != "[2 1]" || dones != 2 || !finished {
		t.Errorf("%v %d %v", order, dones, finished)
	}
	if !bytes.Equal(got[1], pcm[:150*cdrip.SectorBytes]) || !bytes.Equal(got[2], pcm[150*cdrip.SectorBytes:]) {
		t.Error("the samples differ")
	}
}

func TestRipRefusals(t *testing.T) {
	cue, _ := makeCue(t)
	for _, args := range [][]string{{cue, "9", "secure", "0"}, {cue, "x", "secure", "0"}, {cue, "1", "secure", "99999"}, {filepath.Join(t.TempDir(), "gone.cue"), "1", "secure", "0"}, {"too few"}} {
		var out bytes.Buffer
		if code := runCDRip(args, nil, &out); code == 0 || !strings.Contains(out.String(), `"event":"error"`) {
			t.Errorf("%v: %d %s", args, code, out.String())
		}
	}
}

func TestImageCommand(t *testing.T) {
	cue, pcm := makeCue(t)
	dst := filepath.Join(t.TempDir(), "copy.cue")
	var out bytes.Buffer
	if code := runCDImage(cue, dst, true, 0, nil, &out); code != 0 {
		t.Fatalf("%d %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"verified":true`) {
		t.Error(out.String())
	}
	bin, err := os.ReadFile(strings.TrimSuffix(dst, ".cue") + ".bin")
	if err != nil || !bytes.Equal(bin, pcm) {
		t.Errorf("%v", err)
	}
}

func TestStreamCommand(t *testing.T) {
	cue, pcm := makeCue(t)
	in := strings.NewReader("R 0 44\nR 44 100\nR 1000000000 10\n")
	var out bytes.Buffer
	if code := runCDStream(cue, in, &out); code != 0 {
		t.Fatalf("%d", code)
	}
	r := bufio.NewReader(&out)
	line, _ := r.ReadString('\n')
	if want := fmt.Sprintf("S %d\n", 44+len(pcm)); line != want {
		t.Errorf("%q, not %q", line, want)
	}
	read := func() []byte {
		line, _ := r.ReadString('\n')
		var n int
		fmt.Sscanf(line, "D %d", &n)
		p := make([]byte, n)
		io.ReadFull(r, p)
		return p
	}
	if h := read(); len(h) != 44 || string(h[:4]) != "RIFF" {
		t.Errorf("%q", h)
	}
	if p := read(); !bytes.Equal(p, pcm[:100]) {
		t.Error("the samples differ")
	}
	if p := read(); len(p) != 0 {
		t.Errorf("past the end: %d", len(p))
	}
}
