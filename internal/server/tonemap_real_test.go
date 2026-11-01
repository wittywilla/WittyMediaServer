package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// HDR whose frames don't say what they are used to stop the tone mapping filter with "no path
// between colorspaces". A file like that is known to be HDR from somewhere else: here, a Dolby
// Vision record naming the kind of its base layer, with nothing in the stream. These play real
// files through the live path, from the playback answer to the first segment, with the real
// FFmpeg.

func TestLiveToneMapOfUntaggedHDR(t *testing.T) {
	tools, err := ffmpeg.Locate(os.Getenv("WMS_FFMPEG"), os.Getenv("WMS_FFPROBE"))
	if err != nil || !tools.Encoders["libx265"] || !tools.Encoders["libx264"] {
		t.Skip("needs FFmpeg with libx264 and libx265")
	}
	if tools.ToneMapFilter("hdr10") == "" {
		t.Skip("this FFmpeg has no tone mapping filters")
	}
	make := func(t *testing.T, name string, compat int, x265 string) string {
		t.Helper()
		out := filepath.Join(t.TempDir(), name)
		args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25:duration=4",
			"-f", "lavfi", "-i", "sine=frequency=440:duration=4:sample_rate=48000",
			"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-x265-params", x265,
			"-c:a", "aac", "-shortest"}
		if strings.HasSuffix(name, ".mp4") {
			args = append(args, "-tag:v", "hvc1")
		}
		cmd := exec.Command(tools.FFmpeg, append(args, out)...)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("making %s: %v\n%s", name, err, b)
		}
		if compat != 0 {
			addDolbyVisionRecord(t, out, compat)
		}
		return out
	}
	cases := []struct {
		name   string
		file   string
		rng    string
		compat int
		x265   string
	}{
		// The stream itself says nothing.
		{"PQ, untagged", "untagged-pq.mp4", "hdr10", 1, "log-level=error"},
		{"HLG, untagged", "untagged-hlg.mp4", "hlg", 4, "log-level=error"},
		// And the ordinary kind still has to work.
		{"PQ, marked in the stream", "tagged-pq.mkv", "hdr10", 0, "log-level=error:hdr10=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc"},
		{"HLG, marked in the stream", "tagged-hlg.mkv", "hlg", 0, "log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := make(t, c.file, c.compat, c.x265)
			p, err := tools.ProbeFile(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			v0 := p.Video()
			if v0 == nil || v0.Range() != c.rng {
				t.Fatalf("the sample should read as %s: %+v", c.rng, v0)
			}
			if c.compat != 0 && v0.ColorTransfer != "" {
				t.Fatalf("the sample's stream should say nothing: %+v", v0)
			}
			ts := newTestServerWith(t, tools)
			_, admin := ts.user(t, "owner", "adult", true, "")
			lib := ts.library(t, "videos", filepath.Dir(path))
			it := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: path, Title: "Bright Sky", RuntimeSec: 4, Width: 640, Height: 360, VideoCodec: "hevc"})
			w := ts.do(t, http.MethodPost, fmt.Sprintf("/api/items/%d/playback", it.ID), admin, map[string]any{"profile": "web"})
			if w.Code != 200 {
				t.Fatalf("playback: %d %s", w.Code, w.Body)
			}
			var ans struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &ans); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(w.Body.String(), `"video-range"`) {
				t.Fatalf("this should be converted for its range: %s", w.Body)
			}
			if ans.URL == "" {
				t.Fatalf("no address: %s", w.Body)
			}
			id := strings.Split(strings.TrimPrefix(ans.URL, "/api/hls/"), "/")[0]
			t.Cleanup(func() { ts.do(t, http.MethodDelete, "/api/hls/"+id, admin, nil) })
			var seg []byte
			deadline := time.Now().Add(90 * time.Second)
			for {
				w := ts.do(t, http.MethodGet, "/api/hls/"+id+"/seg/0", admin, nil)
				if w.Code == 200 && w.Body.Len() > 0 {
					seg = w.Body.Bytes()
					break
				}
				if w.Code >= 500 || time.Now().After(deadline) {
					t.Fatalf("no first segment: %d %s", w.Code, w.Body)
				}
				time.Sleep(200 * time.Millisecond)
			}
			out := filepath.Join(t.TempDir(), "seg0.ts")
			if err := os.WriteFile(out, seg, 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := tools.ProbeFile(context.Background(), out)
			if err != nil {
				t.Fatalf("the segment doesn't open: %v", err)
			}
			v := got.Video()
			if v == nil || v.CodecName != "h264" || v.PixFmt != "yuv420p" {
				t.Fatalf("%+v", v)
			}
			if v.ColorTransfer != "bt709" {
				t.Fatalf("the picture should say it's an ordinary one now: %+v", v)
			}
			// It really decodes, to a picture that isn't all one colour.
			check := exec.Command(tools.FFmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-i", out, "-frames:v", "5", "-vf", "signalstats,metadata=print:file=-", "-f", "null", "-")
			stats, err := check.CombinedOutput()
			if err != nil || !strings.Contains(string(stats), "lavfi.signalstats.YDIF") {
				t.Fatalf("%v %.300s", err, stats)
			}
		})
	}
}

// addDolbyVisionRecord puts a configuration record in the MP4's track, naming the kind of base
// layer the stream is. The stream's own frames say nothing about their colours.
func addDolbyVisionRecord(t *testing.T, path string, compat int) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	box := make([]byte, 32)
	binary.BigEndian.PutUint32(box, 32)
	copy(box[4:], "dvcC")
	box[8] = 1
	binary.BigEndian.PutUint32(box[10:], uint32(8)<<25|uint32(6)<<19|1<<18|1<<16|uint32(compat)<<12)
	hvcC := bytes.Index(b, []byte("hvcC")) - 4
	if hvcC < 0 || bytes.Index(b, []byte("mdat")) > hvcC {
		t.Fatal("expected an HEVC track with its movie header after the data")
	}
	at := hvcC + int(binary.BigEndian.Uint32(b[hvcC:]))
	// Every box around the sample entry grows, walking down from the movie header.
	pos := bytes.Index(b, []byte("moov")) - 4
	binary.BigEndian.PutUint32(b[pos:], binary.BigEndian.Uint32(b[pos:])+uint32(len(box)))
	pos += 8
	for _, kind := range []string{"trak", "mdia", "minf", "stbl", "stsd", "hvc1"} {
		for {
			n := int(binary.BigEndian.Uint32(b[pos:]))
			if string(b[pos+4:pos+8]) == kind && pos < at && at <= pos+n {
				binary.BigEndian.PutUint32(b[pos:], uint32(n+len(box)))
				if kind == "stsd" {
					pos += 16 // its version, flags and count come before the entry
				} else {
					pos += 8
				}
				break
			}
			if n < 8 {
				t.Fatalf("no %s around the track", kind)
			}
			pos += n
		}
	}
	out := append(append(append([]byte{}, b[:at]...), box...), b[at:]...)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestToneMapChainSaysWhatThePictureIs(t *testing.T) {
	if ffmpeg.ToneMapChain("hlg") == ffmpeg.ToneMapChain("hdr10") {
		t.Fatal("HLG and PQ are different pictures")
	}
	for _, c := range []struct{ in, want string }{{"hdr10", "color_trc=smpte2084"}, {"dv5", "color_trc=smpte2084"}, {"", "color_trc=smpte2084"}, {"hlg", "color_trc=arib-std-b67"}} {
		chain := ffmpeg.ToneMapChain(c.in)
		if !strings.HasPrefix(chain, "setparams=") || !strings.Contains(chain, c.want) || !strings.HasSuffix(chain, "color_trc=bt709:colorspace=bt709") {
			t.Errorf("%q: %s", c.in, chain)
		}
	}
}
