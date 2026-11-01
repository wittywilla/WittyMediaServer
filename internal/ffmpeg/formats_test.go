package ffmpeg

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// realTools finds a real FFmpeg, by default the one on this machine's path. WMS_TEST_FFMPEG_DIR
// tries another build, such as the one a release carries.
func realTools(t *testing.T) *Tools {
	t.Helper()
	dir := os.Getenv("WMS_TEST_FFMPEG_DIR")
	ff, fp := filepath.Join(dir, "ffmpeg"), filepath.Join(dir, "ffprobe")
	if dir == "" {
		var err error
		if ff, err = exec.LookPath("ffmpeg"); err != nil {
			t.Skip("needs FFmpeg")
		}
		if fp, err = exec.LookPath("ffprobe"); err != nil {
			t.Skip("needs ffprobe")
		}
	}
	return &Tools{FFmpeg: ff, FFprobe: fp}
}

// sample makes a file of one kind, or says this build can't.
func sample(t *testing.T, tools *Tools, name string, args ...string) (string, bool) {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	all := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)
	if out, err := exec.Command(tools.FFmpeg, append(all, p)...).CombinedOutput(); err != nil {
		t.Logf("%s: this build can't make one: %v %s", name, err, out)
		return "", false
	}
	return p, true
}

// A real file of every common kind goes on opening. What's listed has to cover what people have.
func TestRealFilesStillOpen(t *testing.T) {
	tools := realTools(t)
	video := []string{"-f", "lavfi", "-i", "testsrc=s=64x64:r=5:d=1", "-f", "lavfi", "-i", "sine=d=1"}
	still := []string{"-f", "lavfi", "-i", "testsrc=s=64x64:r=5:d=1", "-frames:v", "1"}
	sound := []string{"-f", "lavfi", "-i", "sine=d=1"}
	for _, c := range []struct {
		name string
		args []string
		pic  bool // a frame can be taken from it
	}{
		{"a.mkv", video, true},
		{"a.mp4", video, true},
		{"a.mov", video, true},
		{"a.m4v", video, true},
		{"a.avi", video, true},
		{"a.ts", video, true},
		{"a.m2ts", video, true},
		{"a.mpg", append(append([]string{}, video...), "-c:v", "mpeg2video"), true},
		{"a.vob", append(append([]string{}, video...), "-c:v", "mpeg2video", "-f", "vob"), true},
		{"a.flv", video, true},
		{"a.webm", video, true},
		{"a.wmv", video, true},
		{"a.asf", video, true},
		{"a.ogv", video, true},
		{"a.3gp", append(append([]string{}, video...), "-s", "176x144", "-ar", "8000", "-ac", "1", "-c:a", "libopencore_amrnb"), true},
		{"a.mp3", sound, false},
		{"a.flac", sound, false},
		{"a.wav", sound, false},
		{"a.ogg", sound, false},
		{"a.opus", sound, false},
		{"a.m4a", sound, false},
		{"a.aac", sound, false},
		{"a.ac3", sound, false},
		{"a.wma", sound, false},
		{"a.mka", sound, false},
		{"a.aiff", sound, false},
		{"a.jpg", still, true},
		{"a.png", still, true},
		{"a.gif", still, true},
		{"a.webp", still, true},
		{"a.bmp", still, true},
		{"a.tif", still, true},
		{"a.avif", still, true},
		{"a.jxl", still, true},
	} {
		p, ok := sample(t, tools, c.name, c.args...)
		if !ok {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		if _, err := tools.ProbeFile(ctx, p); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if c.pic {
			out := filepath.Join(t.TempDir(), "out.jpg")
			if err := tools.Still(ctx, FileInput(p), 0, 32, out); err != nil {
				t.Errorf("%s still: %v", c.name, err)
			}
		}
		cancel()
	}
}

// A picture with an odd name: every kind that names no number still opens.
func TestPicturesWithOddNames(t *testing.T) {
	tools := realTools(t)
	p, ok := sample(t, tools, "plain.jpg", "-f", "lavfi", "-i", "testsrc=s=64x64:r=5:d=1", "-frames:v", "1")
	if !ok {
		t.Skip("no JPEG here")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, name := range []string{"100% holiday.jpg", "a (1).jpg", "a, b.jpg", "x=y.jpg", "with'quote.jpg", "a%d.jpg", "b*.jpg", "c{1}.jpg", "d[1].jpg", "e?.jpg"} {
		pic := filepath.Join(dir, name)
		if err := os.WriteFile(pic, b, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := tools.ProbeFile(context.Background(), pic); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if err := tools.Still(context.Background(), FileInput(pic), 0, 32, filepath.Join(t.TempDir(), "out.jpg")); err != nil {
			t.Errorf("%s still: %v", name, err)
		}
	}
}

// The attack: a "film" whose bytes are FFmpeg's own list format, naming a link that leads out of
// the library. Everything else that a demuxer can follow is the same kind of thing.
func TestAListDressedAsAFilmOpensNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a link")
	}
	tools := realTools(t)
	out := t.TempDir()
	secret := filepath.Join(out, "private.txt")
	text := bytes.Repeat([]byte("TOPSECRET-MARKER a line of a private file\n"), 200)
	if err := os.WriteFile(secret, text, 0o600); err != nil {
		t.Fatal(err)
	}
	real, ok := sample(t, tools, "outside.ts", "-f", "lavfi", "-i", "testsrc=s=64x64:r=5:d=1", "-c:v", "mpeg2video")
	if !ok {
		t.Skip("can't make a transport stream")
	}
	lib := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(lib, "s.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(lib, "v.ts")); err != nil {
		t.Fatal(err)
	}
	pad := strings.Repeat("# padding past what a probe reads\n", 200)
	for name, body := range map[string]string{
		"text.mkv":     "ffconcat version 1.0\nfile s.txt\n" + pad,
		"video.mkv":    "ffconcat version 1.0\nfile v.ts\n" + pad,
		"hls.mp4":      "#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:1,\nv.ts\n#EXT-X-ENDLIST\n" + pad,
		"mpd.mkv":      `<?xml version="1.0"?><MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT1S"><Period><AdaptationSet><Representation id="1" bandwidth="1"><BaseURL>v.ts</BaseURL></Representation></AdaptationSet></Period></MPD>`,
		"meta.mkv":     ";FFMETADATA1\ntitle=x\n" + pad,
		"as.txt":       string(text),
		"as.nfo":       string(text),
		"sdp.mkv":      "v=0\no=- 0 0 IN IP4 127.0.0.1\ns=x\nc=IN IP4 127.0.0.1\nt=0 0\nm=video 5004 RTP/AVP 96\na=rtpmap:96 H264/90000\n",
		"vector.svg":   `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><image href="s.txt"/></svg>`,
		"playlist.m3u": "#EXTM3U\n#EXTINF:1,\nv.ts\n",
	} {
		p := filepath.Join(lib, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if pr, err := tools.ProbeFile(ctx, p); err == nil {
			t.Errorf("%s opened, as %s", name, pr.Format.FormatName)
		}
		pic := filepath.Join(t.TempDir(), "out.jpg")
		if err := tools.Still(ctx, FileInput(p), 0, 32, pic); err == nil {
			t.Errorf("%s gave a picture", name)
		}
		cancel()
	}
}

// A file named like a numbered pattern is that one file or nothing, never its neighbours.
func TestAPatternNameReadsNoNeighbour(t *testing.T) {
	tools := realTools(t)
	dir := t.TempDir()
	neighbour := filepath.Join(dir, "a0.jpg")
	gen := exec.Command(tools.FFmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=red:s=64x64", "-frames:v", "1", neighbour)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("can't make a picture: %v %s", err, out)
	}
	pattern := filepath.Join(dir, "a%d.jpg")
	if runtime.GOOS == "windows" {
		t.Skip("the name works differently")
	}
	if err := os.WriteFile(pattern, []byte("not a picture at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.jpg")
	if err := tools.Still(context.Background(), FileInput(pattern), 0, 32, out); err == nil {
		t.Errorf("a pattern name made a picture out of a neighbouring file")
	}
	// A real picture with such a name shows itself: the neighbour is another size.
	gen = exec.Command(tools.FFmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=32x32", "-frames:v", "1", "-update", "1", pattern)
	gen.Args = append(gen.Args[:len(gen.Args)-1], "file:"+pattern)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("can't make a picture: %v %s", err, out)
	}
	pr, err := tools.ProbeFile(context.Background(), pattern)
	if err != nil || pr.Video() == nil || pr.Video().Width != 32 {
		t.Errorf("not the file it names: %+v %v", pr, err)
	}
}

// What follows addresses, or turns text into pictures, can't come back to the list by accident.
func TestLibraryFormatsLeaveOut(t *testing.T) {
	list := strings.Split(libraryFormats+",image2", ",")
	for _, bad := range []string{"concat", "hls", "dash", "imf", "sdp", "rtp", "rtsp", "sap", "lavfi", "tty", "bin", "xbin", "adf", "idf", "data", "ffmetadata", "webm_dash_manifest", "avisynth", "vapoursynth", "dvdvideo", "svg_pipe", "image2pipe"} {
		if slices.Contains(list, bad) {
			t.Errorf("%s is on the list", bad)
		}
	}
	for _, need := range []string{"matroska,webm", "mov,mp4,m4a,3gp,3g2,mj2", "mpegts", "mpeg", "avi", "asf", "flv", "ogg", "mp3", "flac", "wav", "aac", "jpeg_pipe", "png_pipe", "webp_pipe", "gif", "image2"} {
		if !strings.Contains(","+libraryFormats+",image2,", ","+need+",") {
			t.Errorf("%s isn't on the list", need)
		}
	}
	for _, p := range []string{"/a/b%d.jpg", "/a/b*.jpg", "/a/b?.jpg", "/a/{b}.jpg", "/a/[b].jpg", "/a%03d/b.jpg"} {
		if strings.Contains(libraryFormatsFor(p), "image2") {
			t.Errorf("%s can be read as a pattern", p)
		}
		if in := strings.Join(FileInput(p), " "); !strings.Contains(in, "-pattern_type none") {
			t.Errorf("%s: %s", p, in)
		}
	}
	if in := strings.Join(FileInput("/a%d/b.mkv"), " "); strings.Contains(in, "pattern_type") || strings.Contains(in, "image2") {
		t.Errorf("a film in an odd folder: %s", in)
	}
	if !strings.Contains(libraryFormatsFor("/a/b.jpg"), "image2") {
		t.Error("a plain name needs image2 for some picture formats")
	}
}

// A disc's parts are what discs hold, whatever a part turns out to be.
func TestConcatInputHasAWhitelist(t *testing.T) {
	in := strings.Join(ConcatInputArgs("/x/list.ffconcat"), " ")
	if !strings.Contains(in, "-format_whitelist "+discFormats) {
		t.Errorf("%s", in)
	}
}
