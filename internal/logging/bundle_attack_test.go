package logging

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bundleText makes a bundle from a log file holding these lines, and gives back all of its text.
func bundleText(t *testing.T, lines []string, sanitize bool, k Known) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wms-2026-11-01.log"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := WriteBundleDir(&buf, dir, nil, BundleOptions{Sanitize: sanitize, Known: k}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(r)
		r.Close()
		out.WriteString("== " + f.Name + "\n")
		out.Write(b)
	}
	return out.String()
}

// Secrets written to a log by every route a person has seen them written: whichever toggle is
// chosen, none may leave. The lines have the shape they'd have on disk, and older lines from
// before a rule existed are why a bundle redacts them again.
func TestNoSecretLeavesInABundle(t *testing.T) {
	const s = "S3CRETVALUE9876"
	lines := []string{
		`time=2026-11-01T10:00:00Z level=INFO msg="fetch failed" url="https://host.example/a?api_key=` + s + `&x=1"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="fetch failed" url="https://host.example/a?apikey=` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="fetch failed" url="https://host.example/a?access_token=` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="fetch failed" url="https://host.example/a?refresh_token=` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="fetch failed" url="https://host.example/a?auth=` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="fetch failed" url="https://host.example/a?passphrase=` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="fetch failed" url="https://user:` + s + `@host.example/a"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="fetch failed" url="rtsp://` + s + `@cam.example/a"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="fetch failed" url="http://host.example/live/user/` + s + `/1.ts"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" path="/api/s/` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" path="/stream/st1.` + s + `/x.m3u8"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" cookie="wms_token=` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" header="Authorization: Bearer ` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" header="X-Api-Key: ` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" header="X-Wms-Token: ` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" tdarrKey=` + s,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" apiKey=` + s,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" key=` + s,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" password=` + s,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" passphrase=` + s,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" backupPassword=` + s,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" sessionToken=` + s,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" setupCode=` + s,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" recoveryCodes=` + s,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" totpSecret=` + s,
		`time=2026-11-01T10:00:00Z level=INFO msg="config" body="{\"password\":\"` + s + `\"}"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="config" body="{\"apiKey\":\"` + s + `\"}"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="config" body="{\"passphrase\":\"` + s + `\"}"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="config" body="{\"privateKey\":\"` + s + `\"}"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="config" body="{\"secretKey\":\"` + s + `\"}"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="config" body="{\"accessKey\":\"` + s + `\"}"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="config" body="{\"signingKey\":\"` + s + `\"}"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="config" body="{\"auth\":\"` + s + `\"}"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="config" body="{\"credentials\":\"` + s + `\"}"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="config" body="{\"cookie\":\"` + s + `\"}"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="config" body="{\"code\":\"190817\"}"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="ffmpeg" args="-headers Authorization: Basic ` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="ffmpeg" args="-i http://host.example/a?sig=` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="env" line="WMS_API_KEY=` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="env" line="TMDB_API_KEY=` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="helper says" output="password: ` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="helper says" output="token: ` + s + `"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="helper says" output="api key: ` + s + `"`,
		`not a line of ours: password=` + s,
		`not a line of ours: {"token":"` + s + `"}`,
	}
	for _, sanitize := range []bool{false, true} {
		text := bundleText(t, lines, sanitize, Known{})
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(line, s) || strings.Contains(line, "190817") {
				t.Errorf("sanitize=%v: a secret left in: %s", sanitize, line)
			}
		}
	}
}

// One log file on its own goes through the same check as a bundle.
func TestDownloadingOneLogRedactsItAgain(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wms.log")
	if err := os.WriteFile(p, []byte(`time=2026-11-01T10:00:00Z level=INFO msg="old line" tdarrKey=OLDSECRET12345 url="https://h.example/a?passphrase=OLDSECRET12345"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := CopyRedacted(&out, p); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "OLDSECRET12345") || !strings.Contains(out.String(), "old line") {
		t.Errorf("%s", out.String())
	}
}

// An error's code and a port stay readable: a reader is after those.
func TestRedactLeavesWhatADiagnosisNeeds(t *testing.T) {
	for _, keep := range []string{`{"code":"password-needed"}`, `dial tcp 10.0.0.1:443: refused`, `token: expired, sign in again`[:0] + `error: no such file`, `http: TLS handshake error`, `keyboard: disconnected`} {
		if got := Redact(keep); got != keep {
			t.Errorf("Redact(%q) = %q", keep, got)
		}
	}
}

// What a sanitized bundle promises to replace, written every way a log writes it.
func TestSanitizedBundleHidesWhoAndWhere(t *testing.T) {
	k := Known{Users: []string{"marigold"}, Libraries: []string{"Bluebell Films"}, Folders: []string{"/srv/bluebell", `D:\Bluebell\Films`, `\\nas7\bluebell`}, Hosts: []string{"quietbox"}, Titles: []string{"The Glass Orchard"}}
	lines := []string{
		`time=2026-11-01T10:00:00Z level=INFO msg="scan" path=/srv/bluebell/Orchard/one.mkv`,
		`time=2026-11-01T10:00:00Z level=INFO msg="scan" path="D:\\Bluebell\\Films\\Orchard\\one.mkv"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="scan" path="D:/Bluebell/Films/Orchard/one.mkv"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="scan" path="d:\\bluebell\\films\\orchard\\one.mkv"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="scan" path="\\\\nas7\\bluebell\\Orchard\\one.mkv"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="scan" path="//nas7/bluebell/Orchard/one.mkv"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="other" path="/Users/rosalind/Movies/one.mkv"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="other" path="/home/rosalind/Movies/one.mkv"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="other" path="C:\\Users\\rosalind\\Movies\\one.mkv"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="other" file="file:///home/rosalind/Movies/one.mkv"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="open /srv/bluebell/Orchard/one.mkv: no such file"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="exec" args="ffmpeg -i /mnt/rosalind/Movies/one.mkv -f null -"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="exec" err="exit status 1: /mnt/rosalind/Movies/one.mkv: Invalid data"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="login" user=marigold ip=203.0.113.9`,
		`time=2026-11-01T10:00:00Z level=INFO msg="login" user=MARIGOLD ip=2001:db8::77`,
		`time=2026-11-01T10:00:00Z level=INFO msg="login" by=hollyhock`,
		`time=2026-11-01T10:00:00Z level=INFO msg="rename hollyhock to something"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" remote="[2001:db8::77]:5512"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" remote="203.0.113.9:5512" forwarded="198.51.100.3"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" ua="Mozilla/5.0 (Marigold's iPad)" device="Marigold's iPad"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="net" mac=aa:bb:cc:dd:ee:ff host=quietbox`,
		`time=2026-11-01T10:00:00Z level=INFO msg="dial tcp quietbox.fritz.box:8096: refused"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="dial tcp nas7.lan:445: refused"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="mail" to=marigold@example.org`,
		`time=2026-11-01T10:00:00Z level=INFO msg="fetch" url="https://my.private.example/feed.xml?x=1"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="match" title="The Glass Orchard"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="matched The Glass Orchard (2001)"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="request" path="/api/images/the%20glass%20orchard.jpg"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="library" name="Bluebell Films"`,
		`time=2026-11-01T10:00:00Z level=INFO msg="saved" file=The.Glass.Orchard.2001.mkv`,
	}
	text := bundleText(t, lines, true, k)
	for _, leak := range []string{"bluebell", "Bluebell", "rosalind", "marigold", "Marigold", "MARIGOLD", "hollyhock", "203.0.113.9", "2001:db8::77", "198.51.100.3", "aa:bb:cc:dd:ee:ff", "quietbox", "nas7", "my.private.example", "Glass Orchard", "glass%20orchard", "Glass.Orchard", "Orchard"} {
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(line, leak) {
				t.Errorf("%q left in: %s", leak, line)
			}
		}
	}
	// The diagnosis stays.
	if !strings.Contains(text, "Invalid data") || !strings.Contains(text, "refused") {
		t.Error("the sanitizer took out what the bundle is for")
	}
	// Each file's name goes in as well as its contents.
	if strings.Contains(text, "== logs/") && strings.Contains(text, "== logs/..") {
		t.Error("a file name climbed out")
	}
}
