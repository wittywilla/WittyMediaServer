package dvdsession

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/seamcheck"
)

// TestRealDiscChapterSeams plays a title of a real disc through the real stream
// and compares what comes out with a straight read of the disc: pictures and
// sound, at every chapter and cell boundary. It needs a decrypted DVD image
// (WMS_SEAM_DVD) and FFmpeg (WMS_FFMPEG = its full path, or on PATH); it takes
// minutes, not seconds. WMS_SEAM_TITLE picks a title (default: the main one) and
// WMS_SEAM_OUT receives the table.
func TestRealDiscChapterSeams(t *testing.T) {
	path := os.Getenv("WMS_SEAM_DVD")
	if path == "" {
		t.Skip("set WMS_SEAM_DVD to a decrypted DVD image")
	}
	ff, err := ffmpeg.Locate(os.Getenv("WMS_FFMPEG"), "")
	if err != nil {
		t.Skip(err)
	}
	ctx := context.Background()
	tmp := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hls.NewManager(ff, filepath.Join(tmp, "hls"), 2, log)
	m := NewManager(h, ff, filepath.Join(tmp, "cache"), "/api", log)
	s, err := m.Open(1, path, Prefs{Wide: true, MaxHeight: 144})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ttn := s.opened.Disc.MainTitle().Number
	if v, _ := strconv.Atoi(os.Getenv("WMS_SEAM_TITLE")); v > 0 {
		ttn = v
	}
	ts := s.opened.Disc.Title(ttn)
	fps := 25.0
	if ts.Video.Standard != "pal" {
		fps = 30000.0 / 1001
	}
	v := s.opened.Disc.VTS[ts.VTS]
	audio := s.titleAudio(v, v.PGCs[ts.Cells[0].PGCN-1].PGC)
	audioMap := "0:a:0"
	if audio == 0 {
		audioMap = ""
	}

	// The straight read: runs of cells that follow each other on the disc in
	// one VOB, whose timestamps carry on from cell to cell. Interleaved cells
	// (angles, branching) are read on their own, through only their own VOBUs.
	sp := wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: ts.VTS}
	var runs [][]string
	var run []wittydvd.FileExtent
	flush := func(name string) {
		if len(run) == 0 {
			return
		}
		ids := []int{wittydvd.VideoStreamID}
		if audio != 0 {
			ids = append(ids, audio)
		}
		list, err := ffmpeg.WriteConcat(tmp, name, []ffmpeg.Segment{{Extents: run}}, ids...)
		if err != nil {
			t.Fatal(err)
		}
		runs = append(runs, ffmpeg.ConcatInputArgs(list))
		run = nil
	}
	for i, c := range ts.Cells {
		ext, err := s.opened.CellExtents(sp, c.Cell)
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			p := ts.Cells[i-1].Cell
			if c.Cell.VOBID != p.VOBID || c.Cell.FirstSector != p.LastSector+1 || c.Cell.Interleaved || p.Interleaved || c.Cell.BlockType == 1 || p.BlockType == 1 {
				flush(fmt.Sprintf("ref%d", i))
			}
		}
		run = append(run, ext...)
	}
	flush("ref-last")
	t.Logf("title %d: %.1f s, %d cells, %d chapters, %d straight runs, %.3f fps", ttn, ts.Duration, len(ts.Cells), len(ts.Chapters), len(runs), fps)
	ref, refSound, err := seamcheck.Reference(ctx, ff, runs, fps, dvdDeinterlace, audioMap, tmp)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("reference: %d frames, %d s of sound", len(ref.Frames), len(refSound)/seamcheck.AudioRate)

	// The real stream.
	p := s.PlayTitle(ctx, ttn, 1)
	if p.Media == nil || p.Media.Type != "hls" {
		t.Fatalf("not a stream: %+v", p)
	}
	id := strings.TrimSuffix(strings.TrimPrefix(p.Media.URL, "/api/hls/"), "/playlist.m3u8")
	joined := filepath.Join(tmp, "out.ts")
	if err := seamcheck.Join(ctx, h.Get(id), joined, os.Getenv("WMS_SEAM_PACED") != ""); err != nil {
		t.Fatal(err)
	}
	out, outSound, err := seamcheck.Decode(ctx, ff, []string{"-i", joined}, fps, "", "0:a:0", tmp)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("output: %d frames, %d s of sound", len(out.Frames), len(outSound)/seamcheck.AudioRate)

	var at []float64
	var labels []string
	for i, c := range ts.Cells {
		if i == 0 {
			continue
		}
		label := fmt.Sprintf("cell %d", i+1)
		for k, ch := range ts.Chapters {
			if ch == c.Start {
				label = fmt.Sprintf("chapter %d", k+1)
			}
		}
		at, labels = append(at, c.Start), append(labels, label)
	}
	rows := seamcheck.Measure(seamcheck.Input{Ref: ref, Out: out, RefSound: refSound, OutSound: outSound, Boundaries: at, Labels: labels})
	table := seamcheck.Table(rows) + seamcheck.Whole(ref, out, refSound, outSound)
	t.Log("\n" + table)
	if f := os.Getenv("WMS_SEAM_OUT"); f != "" {
		_ = os.WriteFile(f, []byte(table), 0o644)
	}
	// A title's first picture may lie a little after its start, which moves
	// everything alike. What can't happen is a boundary moving the picture. A dark
	// or still scene can fool the comparison, so one or two oddities are let pass.
	off := 0
	for _, r := range rows {
		if math.Abs(r.ShiftMs) > 1500/fps {
			off++
			t.Logf("%s: the picture moves %.0f ms", r.Label, r.ShiftMs)
		}
	}
	if off > 2 {
		t.Errorf("%d boundaries move the picture", off)
	}
}
