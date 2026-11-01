package disc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittydisc/isofs"
	"github.com/wittywilla/wittydvd"
)

// The tests that need a real disc read its path from an environment variable,
// and only ever read it:
//
//	WITTY_TEST_DVD=".../MY_DVD.ISO" WITTY_TEST_BD=".../MY_BLURAY.iso" go test ./...

func needDisc(t *testing.T, env string) string {
	t.Helper()
	p := os.Getenv(env)
	if p == "" {
		t.Skipf("%s is not set", env)
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("%s: %v", env, err)
	}
	return p
}

func TestClassifyPlainFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "clip.mkv")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if k, err := Classify(file); err != nil || k != KindFile {
		t.Errorf("a plain file is %q, %v", k, err)
	}
	if k, _ := Classify(dir); k != KindData {
		t.Errorf("an empty folder is %q", k)
	}
	if err := os.Mkdir(filepath.Join(dir, "VIDEO_TS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if k, _ := Classify(dir); k != KindDVD {
		t.Errorf("a folder with VIDEO_TS is %q", k)
	}
	if k, _ := Classify(filepath.Join(dir, "VIDEO_TS")); k != KindDVD {
		t.Errorf("VIDEO_TS itself is %q", k)
	}
	// A junk file named like an image isn't a crash, just not a video disc.
	junk := filepath.Join(dir, "junk.iso")
	if err := os.WriteFile(junk, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	if k, err := Classify(junk); err != nil || k != KindData {
		t.Errorf("a junk image is %q, %v", k, err)
	}
	if _, err := Open(junk, KindData, false, Prefs{}); err == nil || !strings.Contains(err.Error(), "isn't a DVD or Blu-ray") {
		t.Errorf("opening a junk image says %v", err)
	}
}

// presentations walks a disc from insertion, pressing the selected button of each
// menu and ending each clip, and checks each screen is one a player can use.
func checkPresentation(t *testing.T, s Session, p *Presentation) {
	t.Helper()
	if p == nil {
		t.Fatal("no presentation")
	}
	if p.Kind == "stop" {
		return
	}
	m := p.Media
	if m == nil || m.Type != "clip" || m.Token == "" {
		t.Fatalf("%s has unusable media: %+v", p.Kind, m)
	}
	if m.Video == 0 && m.Audio == 0 && !m.AudioFirst {
		t.Errorf("a clip with no streams: %+v", m)
	}
	var bytes, seg int64
	for _, e := range m.Extents {
		if e[0] < 0 || int(e[0]) >= len(m.Files) || e[1] < 0 || e[2] <= 0 {
			t.Fatalf("bad extent %v of %d files", e, len(m.Files))
		}
		bytes += e[2]
	}
	for _, g := range m.Segments {
		seg += g.Bytes
	}
	if bytes == 0 || bytes != seg {
		t.Errorf("%d bytes of extents, %d of segments", bytes, seg)
	}
	if p.Menu != nil && p.Menu.HasOverlay {
		if b, ok := s.Art(p.Menu.Art, "overlay", 0, 0); !ok || len(b) < 8 || string(b[1:4]) != "PNG" {
			t.Errorf("menu %d has no overlay picture", p.Menu.Art)
		}
	}
	if p.Menu != nil && p.Menu.HasHighlights && len(p.Menu.Buttons) > 0 {
		if b, ok := s.Art(p.Menu.Art, "highlight", p.Menu.Buttons[0].N, 0); !ok || string(b[1:4]) != "PNG" {
			t.Errorf("menu %d has no highlight picture", p.Menu.Art)
		}
	}
	if p.Title != nil && p.Title.Duration <= 0 {
		t.Errorf("a title with no length: %+v", p.Title)
	}
}

func walk(t *testing.T, s Session) *Presentation {
	t.Helper()
	p := s.Start()
	for i := 0; i < 30 && p != nil && p.Kind != "stop"; i++ {
		checkPresentation(t, s, p)
		if p.Kind == "title" {
			return p
		}
		if p.Menu != nil && len(p.Menu.Buttons) > 0 {
			return p
		}
		p = s.Do(Action{Type: "ended"})
	}
	return p
}

func TestRealDVD(t *testing.T) {
	path := needDisc(t, "WITTY_TEST_DVD")
	if k, err := Classify(path); err != nil || k != KindDVD {
		t.Fatalf("classified as %q, %v", k, err)
	}
	s, err := Open(path, KindDVD, false, Prefs{MenuLanguage: "en", AudioLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	in := s.Info()
	if in.Main == 0 || len(in.Titles) == 0 {
		t.Fatalf("info %+v", in)
	}
	if p := walk(t, s); p == nil || (p.Kind != "menu" && p.Kind != "title") {
		t.Fatalf("never reached a menu or a title: %+v", p)
	} else if p.Kind == "menu" {
		// Arrow keys move the selection along the disc's own links, and stay on
		// the same screen.
		before := p.Serial
		q := s.Do(Action{Type: "key", Key: "down"})
		checkPresentation(t, s, q)
		if q.Kind == "menu" && q.Serial == before && len(q.Menu.Buttons) > 1 && q.Menu.Selected == p.Menu.Selected {
			// Some menus have no neighbour below: try the other way.
			q = s.Do(Action{Type: "key", Key: "right"})
		}
		// Enter runs the selected button, whatever it leads to.
		r := s.Do(Action{Type: "key", Key: "enter"})
		checkPresentation(t, s, r)
	}
	// The main title plays as one stream, with its tracks and chapters.
	p := s.PlayTitle(in.Main, 0)
	for i := 0; i < 5 && p.Kind == "menu"; i++ {
		p = s.Do(Action{Type: "ended"})
	}
	checkPresentation(t, s, p)
	if p.Kind != "title" {
		t.Skipf("title %d plays through interactive cells here: %s", in.Main, p.Kind)
	}
	if p.Media.Container != "mpeg" || p.Media.Video != wittydvd.VideoStreamID || p.Media.Audio == 0 {
		t.Errorf("title media %+v", p.Media)
	}
	if len(p.Title.Audio) == 0 || len(p.Title.Chapters) == 0 || len(p.Title.Palette) != 16 {
		t.Errorf("title %+v", p.Title)
	}
	for _, tr := range p.Title.Subtitles {
		if tr.ID < 0x20 || tr.ID > 0x3f {
			t.Errorf("subtitle stream id %#x", tr.ID)
		}
	}
	// Seeking finds a later place in the stream, close to the time asked for.
	var last int64 = -1
	for _, sec := range []float64{60, 600, 1800} {
		if sec > p.Title.Duration {
			break
		}
		loc, err := s.Locate(sec)
		if err != nil {
			t.Fatal(err)
		}
		if loc.Offset <= last || loc.Sec > sec+0.5 || sec-loc.Sec > 30 {
			t.Errorf("locate %v: %+v (after %d)", sec, loc, last)
		}
		last = loc.Offset
	}
	// Changing audio restarts the same title at the same moment.
	if len(p.Title.Audio) > 1 {
		q := s.Do(Action{Type: "audio", Stream: p.Title.Audio[1].N, PositionSec: 600})
		checkPresentation(t, s, q)
		if q.Kind != "title" || q.Media.StartSec != 600 || q.Media.StartOffset <= 0 || q.Media.Token == p.Media.Token && p.Title.Audio[1].N != 0 {
			t.Errorf("audio change: %s %+v", q.Kind, q.Media.Token)
		}
	}
	// The menu key leaves the title; resume comes back to where it was.
	m := s.Do(Action{Type: "menu", Menu: "root", PositionSec: 700})
	checkPresentation(t, s, m)
	if m.Kind == "menu" {
		r := s.Do(Action{Type: "resume"})
		checkPresentation(t, s, r)
		if r.Kind == "title" && (r.Title.StartSec < 650 || r.Title.StartSec > 700.5) {
			t.Errorf("resume at %v, left at 700", r.Title.StartSec)
		}
	}
}

func TestRealBluray(t *testing.T) {
	path := needDisc(t, "WITTY_TEST_BD")
	if k, err := Classify(path); err != nil || k != KindBluray {
		t.Fatalf("classified as %q, %v", k, err)
	}
	s, err := Open(path, KindBluray, false, Prefs{MenuLanguage: "en", AudioLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	in := s.Info()
	if in.Main == 0 || len(in.Titles) == 0 {
		t.Fatalf("info %+v", in)
	}
	p := walk(t, s)
	if p == nil || p.Kind == "stop" {
		t.Fatalf("stopped: %+v", p)
	}
	q := s.PlayTitle(in.Main, 0)
	checkPresentation(t, s, q)
	if q.Kind != "title" {
		t.Fatalf("main title is %s: %s", q.Kind, q.Reason)
	}
	if q.Media.Container != "mpegts" || q.Media.Video == 0 || q.Media.Audio == 0 || q.Media.Segments[0].BasePTS < 0 {
		t.Errorf("media %+v", q.Media.Segments)
	}
	var last int64 = -1
	for _, sec := range []float64{60, 600, 1800} {
		loc, err := s.Locate(sec)
		if err != nil {
			t.Fatal(err)
		}
		if loc.Offset <= last || loc.Offset%192 != 0 || loc.Sec > sec+0.5 || sec-loc.Sec > 30 || loc.PTS < 0 {
			t.Errorf("locate %v: %+v (after %d)", sec, loc, last)
		}
		last = loc.Offset
	}
	if len(q.Title.Chapters) > 2 {
		r := s.PlayTitle(in.Main, 3)
		if r.Kind != "title" || r.Media.StartSec <= 0 || r.Title.Chapter != 3 {
			t.Errorf("chapter 3: start %v chapter %d", r.Media.StartSec, r.Title.Chapter)
		}
	}
}

// extract copies the small structure files of an image into a folder disc,
// leaving the big streams to the test.
func extract(t *testing.T, image, dir string, keep func(string) bool) []string {
	t.Helper()
	d, err := isofs.Open(image)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var rest []string
	for _, f := range d.Files() {
		if f.IsDir {
			continue
		}
		if !keep(f.Path) {
			rest = append(rest, f.Path)
			continue
		}
		b, err := d.ReadFile(f.Path, 64<<20)
		if err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return rest
}

// scrambledPack is a pack whose first PES says it is scrambled.
func scrambledPack() []byte {
	p := make([]byte, wittydvd.Sector)
	copy(p, []byte{0, 0, 1, 0xba, 0x44})
	copy(p[14:], []byte{0, 0, 1, 0xe0, 0x07, 0xec, 0x30})
	return p
}

// A DVD whose film is scrambled is refused, in plain words, even though its
// structure is fine.
func TestScrambledDVDRefused(t *testing.T) {
	path := needDisc(t, "WITTY_TEST_DVD")
	dir := t.TempDir()
	extract(t, path, dir, func(p string) bool {
		up := strings.ToUpper(p)
		return strings.HasSuffix(up, ".IFO") || strings.HasSuffix(up, ".BUP")
	})
	o, err := wittydvd.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	main := o.Disc.MainTitle()
	o.Close()
	if main == nil || len(main.Cells) == 0 {
		t.Fatal("no main title")
	}
	vob, err := os.Create(filepath.Join(dir, "VIDEO_TS", "VTS_"+pad(main.VTS)+"_1.VOB"))
	if err != nil {
		t.Fatal(err)
	}
	last := main.Cells[0].Cell
	for _, c := range main.Cells {
		if c.Cell.LastSector > last.LastSector {
			last = c.Cell
		}
	}
	if err := vob.Truncate(int64(last.LastSector+1) * wittydvd.Sector); err != nil {
		t.Fatal(err)
	}
	// Scramble where the check looks: the middle of the first cell.
	first := main.Cells[0].Cell
	at := int64(first.FirstSector+(first.LastSector-first.FirstSector)/2) * wittydvd.Sector
	if _, err := vob.WriteAt(scrambledPack(), at); err != nil {
		t.Fatal(err)
	}
	vob.Close()
	_, err = Open(dir, KindDVD, false, Prefs{})
	if !IsProtected(err) || err.Error() != MsgDVDProtected {
		t.Fatalf("a scrambled disc opened with: %v", err)
	}
	if err := CheckProtected(dir, KindDVD, false); !IsProtected(err) {
		t.Fatalf("the imaging check said: %v", err)
	}
}

func pad(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// A Blu-ray whose streams are AACS-encrypted is refused likewise.
func TestEncryptedBlurayRefused(t *testing.T) {
	path := needDisc(t, "WITTY_TEST_BD")
	dir := t.TempDir()
	rest := extract(t, path, dir, func(p string) bool {
		up := strings.ToUpper(p)
		return strings.HasPrefix(up, "BDMV/") && !strings.Contains(up, "/STREAM/") && !strings.Contains(up, "/JAR/") && !strings.Contains(up, "/AUXDATA/")
	})
	// Encrypted units keep their first packet's header, with the copy-permission
	// bits set, and every other packet is noise.
	const unit = 32 * 192
	enc := make([]byte, unit*16)
	for u := 0; u+unit <= len(enc); u += unit {
		enc[u] = 0xc0
		enc[u+4] = 0x47
	}
	if err := os.MkdirAll(filepath.Join(dir, "BDMV", "STREAM"), 0o755); err != nil {
		t.Fatal(err)
	}
	wrote := 0
	for _, p := range rest {
		if strings.HasSuffix(strings.ToUpper(p), ".M2TS") {
			if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), enc, 0o644); err != nil {
				t.Fatal(err)
			}
			wrote++
		}
	}
	if wrote == 0 {
		t.Fatal("no streams found")
	}
	if d, err := wittybluray.Open(dir); err != nil || !d.Encrypted {
		t.Fatalf("the engine says %v, %v", d, err)
	} else {
		d.Close()
	}
	_, err := Open(dir, KindBluray, false, Prefs{})
	if !IsProtected(err) || err.Error() != MsgBlurayProtected {
		t.Fatalf("an encrypted disc opened with: %v", err)
	}
}
