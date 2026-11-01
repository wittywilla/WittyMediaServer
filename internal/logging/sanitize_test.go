package logging

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Made-up values planted in every kind of place something can leak from. Each one must be
// gone from every file of a sanitized bundle.
var planted = []struct {
	name  string
	value string // what must not survive
}{
	{"ipv4", "192.168.40.17"},
	{"ipv4 public", "203.0.113.77"},
	{"ipv6 compressed", "2001:db8::77:1"},
	{"ipv6 upper case", "2001:DB8:AA::1"},
	{"ipv6 mapped", "::ffff:198.51.100.9"},
	{"ipv6 link local", "fe80::1234:5678:9abc:def0"},
	{"mac with colons", "aa:bb:cc:11:22:33"},
	{"mac with dashes", "AA-BB-CC-44-55-66"},
	{"email", "plantedperson@mail.example.com"},
	{"email encoded", "encodedperson%40mail.example.com"},
	{"url login", "cameralogin:camerapass77"},
	{"url host", "tuner.planted-home.lan"},
	{"public host", "myserver.example.net"},
	{"lookup name", "planted-nas"},
	{"windows path", `D:\PlantedMedia\Holiday Tapes\Summer.mkv`},
	{"windows folder", "PlantedMedia"},
	{"windows forward", `E:/PlantedForward/film.mkv`},
	{"unc path", `\\PLANTEDNAS\share\Family\clip.mp4`},
	{"unc host", "PLANTEDNAS"},
	{"unix path", "/mnt/planted-disk/Family Films/clip.mkv"},
	{"unix folder", "planted-disk"},
	{"unix folder with space", "Family Films"},
	{"home folder", "/home/plantedhome"},
	{"account", "plantedaccount"},
	{"account mixed case", "PlantedAccount"},
	{"library", "Attic Cinema"},
	{"library folder", "/srv/plantedlibrary"},
	{"title", "A Very Secret Movie Title"},
	{"title in free text", "The Quiet Harbour"},
	{"single word title", "Zanzibarium"},
	{"show", "Lanterns Over Marsh"},
	{"game", "Crystal Burrow Saga"},
	{"album", "Songs For Nobody Else"},
	{"device", "Plantedphone Twelve"},
	{"hostname", "planted-computer"},
	{"token value", "plantedtokenvalue123"},
	{"password value", "plantedpassword456"},
	{"bearer", "plantedbearer789"},
	{"cookie", "plantedcookie321"},
	{"api key", "plantedapikey654"},
	{"percent encoded token", "plantedencoded987"},
	{"encoded title", "A%20Very%20Secret"},
	{"share link", "plantedsharelink01"},
	{"setup code", "plantedsetupcode"},
	{"ffmpeg meta", "Plantedmeta Comment Here"},
	{"ffmpeg arg", "Plantedarg Title Here"},
}

func plantedKnown() Known {
	return Known{
		Users:     []string{"plantedaccount"},
		Libraries: []string{"Attic Cinema", "Movies"},
		Folders:   []string{`D:\PlantedMedia`, "/srv/plantedlibrary", "/home/plantedhome"},
		Hosts:     []string{"planted-computer"},
		Titles:    []string{"A Very Secret Movie Title", "The Quiet Harbour", "Zanzibarium", "Lanterns Over Marsh", "Error", "Server"},
	}
}

// plantEverything writes a log with every value, in every place it can be.
func plantEverything(t *testing.T, l *Logger, log *slog.Logger) {
	t.Helper()
	win := `D:\PlantedMedia\Holiday Tapes\Summer.mkv`
	unix := "/mnt/planted-disk/Family Films/clip.mkv"
	// The message.
	log.Warn("from 192.168.40.17 and 2001:db8::77:1 for plantedaccount in Attic Cinema on planted-computer")
	log.Warn("the file " + win + ": could not be opened")
	log.Warn("the mail is plantedperson@mail.example.com and the card aa:bb:cc:11:22:33")
	// Attributes that say what they hold.
	log.Warn("named", "title", "A Very Secret Movie Title", "show", "Lanterns Over Marsh", "game", "Crystal Burrow Saga", "album", "Songs For Nobody Else",
		"user", "PlantedAccount", "account", "plantedaccount", "library", "Attic Cinema", "device", "Plantedphone Twelve",
		"host", "planted-computer", "hostname", "planted-computer", "ip", "203.0.113.77", "from", "192.168.40.17:4000", "mac", "AA-BB-CC-44-55-66",
		"email", "plantedperson@mail.example.com", "path", unix, "file", win, "dir", "/srv/plantedlibrary/sub", "url", "rtsp://cameralogin:camerapass77@tuner.planted-home.lan/A Very Secret/stream")
	// Attributes that don't, holding free text.
	log.Warn("free text", "err", "open "+win+": access denied", "reason", "dial tcp [2001:DB8:AA::1]:443: refused by myserver.example.net",
		"note", "seen fe80::1234:5678:9abc:def0%eth0 and ::ffff:198.51.100.9", "why", `Get "http://cameralogin:camerapass77@tuner.planted-home.lan/x": EOF`,
		"more", "lookup planted-nas: no such host", "unc", `\\PLANTEDNAS\share\Family\clip.mp4`, "fwd", `E:/PlantedForward/film.mkv`, "other", "played The Quiet Harbour and Zanzibarium today",
		"mail", "encodedperson%40mail.example.com", "search", "q=A%20Very%20Secret")
	// Secrets, in any spelling.
	log.Warn("secrets", "token", "plantedtokenvalue123", "PassWord", "plantedpassword456", "cmd", "x?TOKEN=plantedtokenvalue123&Password=plantedpassword456",
		"hdr", "Authorization: Bearer plantedbearer789", "ck", "Cookie: wms_token=plantedcookie321", "api", `{"apiKey":"plantedapikey654"}`,
		"enc", "u?%74oken=plantedencoded987", "share", "/api/share/plantedsharelink01", "setupCode", "plantedsetupcode")
	// What a program printed.
	log.Warn("ffmpeg stopped", "stderr", "Input #0, matroska, from '"+win+"':\n  Metadata:\n    title           : A Very Secret Movie Title\n    comment         : Plantedmeta Comment Here\n  Duration: 01:00:00, bitrate: 1200 kb/s\n    Stream #0:0: Video: h264 (High), yuv420p, 1920x1080",
		"args", `-i `+unix+` -metadata title="Plantedarg Title Here" -c:v libx264`)
	// A stack, and the fields a line carries.
	log.Error("crashed", "session", "tv-plantedaccount", "stack", "goroutine 1 [running]:\nmain.run(0xc000012345)\n\t/home/plantedhome/src/wms/main.go:12 +0x1d\nopened "+unix)
	l.Flush()
}

func plantedBundle(t *testing.T, sanitize bool) map[string]string {
	t.Helper()
	l, log := newTest(t)
	plantEverything(t, l, log)
	dir := t.TempDir()
	report := "Server stopped unexpectedly\nfile: " + `D:\PlantedMedia\Holiday Tapes\Summer.mkv` + "\nuser plantedaccount from 192.168.40.17\n\t/home/plantedhome/src/wms/server.go:55 +0x2\n" +
		`time=2026-11-01T10:00:00.000Z level=ERROR cat=playback msg="could not play" title="The Quiet Harbour" file="/mnt/planted-disk/Family Films/clip.mkv"` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "crash-20261101T100000.000Z-fatal.txt"), []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	guard := OpenCrashReports(dir)
	if len(guard.List()) != 1 {
		t.Fatalf("the crash report wasn't found: %v", guard.List())
	}
	summary, _ := json.Marshal(map[string]any{
		"version": "0.6.0-test", "hostname": "planted-computer", "ffmpeg": "/home/plantedhome/bin/ffmpeg",
		"folders":  []map[string]any{{"role": "data", "path": `D:\PlantedMedia\data`}, {"role": "library", "path": "/srv/plantedlibrary"}},
		"settings": map[string]string{"remoteHost": "myserver.example.net", "note": "for plantedaccount"},
		"Attic Cinema": 12, "encoders": []string{"libx264"},
	})
	diag, _ := json.Marshal(map[string]any{"subject": "The Quiet Harbour", "findings": []map[string]any{{"title": "Bad sector", "target": "Zanzibarium", "names": []string{"Crystal Burrow Saga"}, "what": "read " + `\\PLANTEDNAS\share\Family\clip.mp4`}}})
	opts := BundleOptions{Days: 1, Sanitize: sanitize, Known: plantedKnown(), Extra: []BundleFile{{Name: "summary.json", Description: "s", Data: summary}, {Name: "diagnostics/d.json", Description: "d", Data: diag}}}
	var out bytes.Buffer
	if err := l.WriteBundle(&out, guard, opts); err != nil {
		t.Fatal(err)
	}
	return unzipFiles(t, out.Bytes())
}

func unzipFiles(t *testing.T, b []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(data)
	}
	return files
}

// Every planted value, searched for in every file of the real zip, in the forms a line
// and a JSON file escape it to.
func TestSanitizedBundleHoldsNoPlantedValue(t *testing.T) {
	files := plantedBundle(t, true)
	for _, want := range []string{"README.txt", "summary.json", "diagnostics/d.json", "crashes/crash-20261101T100000.000Z-fatal.txt"} {
		if _, ok := files[want]; !ok {
			t.Fatalf("no %s in %v", want, files)
		}
	}
	logs := 0
	for name := range files {
		if strings.HasPrefix(name, "logs/") {
			logs++
		}
	}
	if logs == 0 {
		t.Fatal("no log files in the bundle")
	}
	for _, p := range planted {
		forms := []string{p.value, strings.ToLower(p.value), strings.ReplaceAll(p.value, `\`, `\\`)}
		for name, text := range files {
			lower := strings.ToLower(text)
			for _, form := range forms {
				if strings.Contains(lower, strings.ToLower(form)) {
					t.Errorf("%s: %q survived in %s:\n%s", p.name, form, name, excerpt(lower, strings.ToLower(form)))
				}
			}
		}
	}
}

func excerpt(text, at string) string {
	i := strings.Index(text, at)
	from, to := i-120, i+len(at)+120
	if from < 0 {
		from = 0
	}
	if to > len(text) {
		to = len(text)
	}
	return text[from:to]
}

// With it off, it's the owner's own log: names stay, and secrets still don't.
func TestRawBundleKeepsNamesAndNeverSecrets(t *testing.T) {
	files := plantedBundle(t, false)
	all := strings.Join(mapValues(files), "\n")
	for _, keep := range []string{"A Very Secret Movie Title", "plantedaccount", "192.168.40.17"} {
		if !strings.Contains(all, keep) {
			t.Errorf("%q should stay in a bundle that isn't sanitized", keep)
		}
	}
	for _, secret := range []string{"plantedtokenvalue123", "plantedpassword456", "plantedbearer789", "plantedcookie321", "plantedapikey654", "plantedencoded987", "plantedsharelink01", "plantedsetupcode", "camerapass77"} {
		if strings.Contains(all, secret) {
			t.Errorf("%q is a secret, sanitized or not", secret)
		}
	}
	if !strings.Contains(files["README.txt"], "NOT sanitized") {
		t.Error("the readme should say it isn't sanitized")
	}
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// The same value has the same placeholder wherever it appears, and different values differ.
func TestPlaceholdersAreConsistentAcrossFiles(t *testing.T) {
	s := NewSanitizer(plantedKnown())
	a := s.Text("open /mnt/one/a.mkv from 192.168.1.20 as plantedaccount")
	b := s.Attr("file", "/mnt/one/a.mkv")
	c := s.Attr("from", "192.168.1.20:4000")
	d := s.Attr("user", "PLANTEDACCOUNT")
	e := s.Text("open /mnt/one/b.mkv from 192.168.1.21")
	pathA := regexp.MustCompile(`\[path-\d+\]`).FindString(a)
	ipA := regexp.MustCompile(`\[ip-\d+\]`).FindString(a)
	userA := regexp.MustCompile(`\[user-\d+\]`).FindString(a)
	if pathA == "" || ipA == "" || userA == "" {
		t.Fatalf("no placeholders in %q", a)
	}
	if !strings.Contains(b, pathA) {
		t.Errorf("the same file differs: %q and %q", a, b)
	}
	if !strings.Contains(c, ipA) || !strings.HasSuffix(c, ":4000") {
		t.Errorf("the same address differs: %q and %q", a, c)
	}
	if d != userA {
		t.Errorf("the same account differs: %q and %q", a, d)
	}
	if strings.Contains(e, pathA) || strings.Contains(e, ipA) {
		t.Errorf("different values share a placeholder: %q and %q", a, e)
	}
	// The windows spelling of one file is one file.
	f, g := s.Attr("file", `D:\PlantedMedia\x\y.mkv`), s.Text(`opening D:/PlantedMedia/x/y.mkv`)
	if !strings.Contains(g, f) {
		t.Errorf("one file, two placeholders: %q and %q", f, g)
	}

	// Across the files of a real bundle.
	files := plantedBundle(t, true)
	var logText string
	for name, text := range files {
		if strings.HasPrefix(name, "logs/") {
			logText += text
		}
	}
	re := regexp.MustCompile(`\[folder-\d+\]/\[path-\d+\]\.mkv`)
	inLog, inCrash := re.FindString(logText), re.FindString(files["crashes/crash-20261101T100000.000Z-fatal.txt"])
	if inLog == "" || inLog != inCrash {
		t.Errorf("one file differs between a log and a crash report: %q and %q", inLog, inCrash)
	}
}

// What a diagnosis needs stays.
func TestSanitizingKeepsDiagnostics(t *testing.T) {
	s := NewSanitizer(plantedKnown())
	for _, c := range []struct{ in, want string }{
		{"version 0.6.0-alpha built with go1.26.0", "version 0.6.0-alpha built with go1.26.0"},
		{"Stream #0:0: Video: h264 (High), yuv420p, 1920x1080, 23.98 fps", "Stream #0:0: Video: h264 (High), yuv420p, 1920x1080, 23.98 fps"},
		{"Audio: aac (LC), 48000 Hz, stereo, fltp, 192 kb/s", "Audio: aac (LC), 48000 Hz, stereo, fltp, 192 kb/s"},
		{"  Duration: 01:32:10.45, start: 0.000000, bitrate: 8123 kb/s", "  Duration: 01:32:10.45, start: 0.000000, bitrate: 8123 kb/s"},
		{"exit status 137 after 12.5s", "exit status 137 after 12.5s"},
		{"listening on 127.0.0.1:9797 and 0.0.0.0:9797 and [::1]:9797", "listening on 127.0.0.1:9797 and 0.0.0.0:9797 and [::1]:9797"},
		{"at 2026-11-01T10:00:00.123Z and 12:34:56", "at 2026-11-01T10:00:00.123Z and 12:34:56"},
		{"github.com/some-vendor/thing v1.2.3.4", "github.com/some-vendor/thing v1.2.3.4"},
		{"std::string failed in libav::decode", "std::string failed in libav::decode"},
		{"server.go:123 and net/http.(*conn).serve", "server.go:123 and net/http.(*conn).serve"},
		{"100% done, 50%", "100% done, 50%"},
	} {
		if got := s.Text(c.in); got != c.want {
			t.Errorf("%q became %q", c.in, got)
		}
	}
	for _, c := range []struct{ k, v, want string }{
		{"p", "/api/items/5/play", "/api/items/5/play"},
		{"p", "/api/hls/abc-123/seg/7.ts?st=plantedtoken", "/api/hls/abc-123/seg/7.ts?…"},
		{"p", "/api/browse?q=A+Very+Secret", "/api/browse?…"},
		{"p", "/api/share/plantedsharelink01/image/3", "/api/share/[hidden]/image/3"},
		{"s", "500", "500"},
		{"codec", "hevc", "hevc"},
		{"library", "7", "7"},
		{"addr", ":9797", ":9797"},
	} {
		if got := s.Attr(c.k, c.v); got != c.want {
			t.Errorf("%s=%q became %q", c.k, c.v, got)
		}
	}
	// A title that's an ordinary word can't blot out the words of a message, in any part.
	rec := Record{Msg: "Server Error while reading", Attrs: []Attr{{"err", "Server Error"}}}
	s.Record(&rec)
	if rec.Msg != "Server Error while reading" || rec.Attrs[0].V != "Server Error" {
		t.Errorf("%+v", rec)
	}
	// A single-word title is hidden in free text, and left alone in the fixed message.
	s2 := NewSanitizer(Known{Titles: []string{"Zanzibarium"}})
	rec = Record{Msg: "Zanzibarium is the name of a step", Attrs: []Attr{{"err", "opened Zanzibarium"}}}
	s2.Record(&rec)
	if rec.Msg != "Zanzibarium is the name of a step" || strings.Contains(rec.Attrs[0].V, "Zanzibarium") {
		t.Errorf("%+v", rec)
	}
	// A word inside another word isn't a name.
	if got := NewSanitizer(Known{Users: []string{"alice"}}).Text("malice and alice"); got != "malice and [user-1]" {
		t.Error(got)
	}
}

// The preview shows the same thing the zip will hold, and says how much it changed.
func TestPreviewSaysWhatSanitizingDoes(t *testing.T) {
	l, log := newTest(t)
	plantEverything(t, l, log)
	p := l.Preview(nil, BundleOptions{Days: 1, Sanitize: true, Known: plantedKnown()})
	if !p.Sanitize || !p.Redact || p.Scanned == 0 || len(p.Sample) == 0 || len(p.Changed) == 0 {
		t.Fatalf("%+v", p)
	}
	for _, kind := range []string{"ip", "user", "path", "title", "host", "mac", "email"} {
		if p.Replaced[kind] == 0 {
			t.Errorf("no %s counted: %v", kind, p.Replaced)
		}
	}
	for _, line := range p.Sample {
		for _, v := range []string{"192.168.40.17", "plantedaccount", "A Very Secret Movie Title"} {
			if strings.Contains(line, v) {
				t.Errorf("the sample shows %q: %s", v, line)
			}
		}
	}
	for _, c := range p.Changed {
		if c.Before == c.After || strings.Contains(c.After, "plantedaccount") {
			t.Errorf("%+v", c)
		}
	}
	raw := l.Preview(nil, BundleOptions{Days: 1, Known: plantedKnown()})
	if raw.Sanitize || raw.Replaced != nil || len(raw.Changed) != 0 {
		t.Errorf("%+v", raw)
	}
}

// Any text log can be run through it, ours or not.
func TestSanitizeLogFile(t *testing.T) {
	in := `time=2026-11-01T10:00:00.000Z level=WARN cat=playback msg="could not open" file="D:\\PlantedMedia\\x.mkv" user=plantedaccount
a plain line from 192.168.40.17 about /mnt/planted-disk/clip.mkv and A Very Secret Movie Title
` + "\tC:/Users/plantedhome/go/server.go:12 +0x3\n"
	var out bytes.Buffer
	k := plantedKnown()
	k.Folders = append(k.Folders, "C:/Users/plantedhome")
	counts, err := SanitizeLog(&out, strings.NewReader(in), k)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, v := range []string{"PlantedMedia", "plantedaccount", "192.168.40.17", "planted-disk", "A Very Secret", "plantedhome"} {
		if strings.Contains(text, v) {
			t.Errorf("%q survived:\n%s", v, text)
		}
	}
	if !strings.Contains(text, "level=WARN cat=playback") || !strings.Contains(text, "server.go:12") || strings.Count(text, "\n") != 3 {
		t.Errorf("too much went:\n%s", text)
	}
	if counts["ip"] != 1 || counts["user"] == 0 {
		t.Errorf("%v", counts)
	}
}
