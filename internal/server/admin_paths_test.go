package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/livetv"
)

var fromTheHomeNetwork = []string{"RemoteAddr", "192.168.1.20:1", "Host", "192.168.1.5:9797"}

// A path naming another machine makes the server sign in there with its own account, on Windows
// the machine's. That's for the owner to decide, not for whoever has an administrator's session.
func TestAdminPathsRefuseOtherMachines(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	for _, share := range []string{`\\attacker.example\share\x`, `//attacker.example/share/x`, `\\?\UNC\attacker.example\share`, `\\.\pipe\x`, `\/attacker.example/share`} {
		for _, c := range []struct {
			what, method, path string
			body               any
		}{
			{"library", "POST", "/api/libraries", map[string]any{"name": "L", "type": "movies", "paths": []string{share}}},
			{"certificate", "PUT", "/api/remote", map[string]any{"httpsMode": "cert", "certFile": share, "keyFile": "/a/key.pem"}},
			{"key", "PUT", "/api/remote", map[string]any{"httpsMode": "cert", "certFile": "/a/cert.pem", "keyFile": share}},
			{"log folder", "PUT", "/api/logs", map[string]any{"dir": share}},
			{"recordings", "PUT", "/api/tv/settings", map[string]any{"dvrFolder": share}},
			{"guide", "POST", "/api/epg/sources", map[string]any{"name": "G", "url": share}},
			{"playlist", "POST", "/api/tuners", map[string]any{"driver": "iptv", "name": "P", "options": map[string]string{"playlist": share}}},
			{"browser", "GET", "/api/fs?path=" + strings.ReplaceAll(strings.ReplaceAll(share, `\`, "%5C"), "?", "%3F"), nil},
		} {
			w := ts.do(t, c.method, c.path, tok, c.body, fromTheHomeNetwork...)
			if w.Code < 400 || !strings.Contains(w.Body.String(), "another machine") {
				t.Errorf("%s %s: %d %s", c.what, share, w.Code, w.Body)
			}
		}
	}
	if got, _ := ts.Store.Libraries(context.Background()); len(got) != 0 {
		t.Errorf("%+v", got)
	}
}

// Someone at the server itself may choose a share, as may a server whose owner said so. A library
// already on a share can still be edited.
func TestNetworkPathsWhenTheOwnerSaysSo(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	share := `//attacker.example/share/x`
	body := map[string]any{"name": "L", "type": "movies", "paths": []string{share}}
	// Refused for not existing here, not for being a share.
	if w := ts.do(t, "POST", "/api/libraries", tok, body); strings.Contains(w.Body.String(), "another machine") {
		t.Errorf("at the server itself: %d %s", w.Code, w.Body)
	}
	ts.Cfg.AllowNetworkPaths = true
	if w := ts.do(t, "POST", "/api/libraries", tok, body, fromTheHomeNetwork...); strings.Contains(w.Body.String(), "another machine") {
		t.Errorf("with the setting: %d %s", w.Code, w.Body)
	}
	ts.Cfg.AllowNetworkPaths = false
	r := httptest.NewRequest("PUT", "/api/libraries/1", nil)
	r.RemoteAddr, r.Host = "192.168.1.20:1", "192.168.1.5:9797"
	if err := ts.checkAdminPaths(r, []string{share}, []string{share}); err != nil {
		t.Errorf("a share it already has: %v", err)
	}
	if err := ts.checkAdminPaths(r, []string{share}, nil); err == nil {
		t.Error("a new share")
	}
}

// Playlists and guides named as files come from where media and the server's own things are.
func TestAdminFilesComeFromKnownFolders(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	private := filepath.Join(t.TempDir(), "secrets.conf")
	if err := os.WriteFile(private, []byte("#EXTM3U\nhttps://user:token@host.example/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		what, path string
		body       any
	}{
		{"guide", "/api/epg/sources", map[string]any{"name": "G", "url": private}},
		{"playlist", "/api/tuners", map[string]any{"driver": "iptv", "name": "P", "options": map[string]string{"playlist": private}}},
	} {
		w := ts.do(t, "POST", c.path, tok, c.body, fromTheHomeNetwork...)
		if w.Code < 400 {
			t.Errorf("%s: %d %s", c.what, w.Code, w.Body)
		}
	}
	if runtime.GOOS != "windows" {
		if w := ts.do(t, "POST", "/api/epg/sources", tok, map[string]any{"name": "Z", "url": "/dev/zero"}, fromTheHomeNetwork...); w.Code < 400 {
			t.Errorf("a device: %d %s", w.Code, w.Body)
		}
	}
	// In the data folder, in a library, or chosen at the server itself, it's fine.
	inData := filepath.Join(ts.Cfg.DataDir, "list.m3u")
	if err := os.WriteFile(inData, []byte("#EXTM3U\n#EXTINF:-1,A\nhttp://s.example/a.ts\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "POST", "/api/tuners", tok, map[string]any{"driver": "iptv", "name": "D", "options": map[string]string{"playlist": inData}}, fromTheHomeNetwork...); w.Code != 200 {
		t.Errorf("in the data folder: %d %s", w.Code, w.Body)
	}
	lib := t.TempDir()
	ts.library(t, "movies", lib)
	inLib := filepath.Join(lib, "guide.xml")
	if err := os.WriteFile(inLib, []byte(`<tv></tv>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "POST", "/api/epg/sources", tok, map[string]any{"name": "L", "url": inLib}, fromTheHomeNetwork...); w.Code != 200 {
		t.Errorf("in a library: %d %s", w.Code, w.Body)
	}
	here := filepath.Join(t.TempDir(), "mine.xml")
	if err := os.WriteFile(here, []byte(`<tv></tv>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "POST", "/api/epg/sources", tok, map[string]any{"name": "H", "url": here}); w.Code != 200 {
		t.Errorf("at the server itself: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/epg/sources", tok, map[string]any{"name": "W", "url": "https://guide.example/g.xml"}, fromTheHomeNetwork...); w.Code != 200 {
		t.Errorf("a web address: %d %s", w.Code, w.Body)
	}
}

// A stream's address can hold a login, and what a connection answered says what's on the network.
// Neither is for the page.
func TestTunerErrorsHoldNoAddress(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "viewer", "adult", false, "")
	const secret = "USERNAME/PASSWORD"
	w := ts.do(t, "POST", "/api/tuners", admin, map[string]any{"driver": "stream", "name": "One", "options": map[string]string{"url": "http://127.0.0.1:1/live/" + secret + "/1.ts"}})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var tn livetv.Tuner
	if err := json.Unmarshal(w.Body.Bytes(), &tn); err != nil {
		t.Fatal(err)
	}
	chs, err := ts.TV.Channels(context.Background(), tn.ID)
	if err != nil || len(chs) != 1 {
		t.Fatalf("%v %v", err, chs)
	}
	key := "tun-" + strconv.FormatInt(chs[0].ID, 10)
	w = ts.do(t, "GET", "/api/tv/"+key+"/tune", adult, nil)
	if w.Code < 400 || strings.Contains(w.Body.String(), "PASSWORD") || strings.Contains(w.Body.String(), "127.0.0.1") || strings.Contains(w.Body.String(), "refused") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	// An owner's test: the same, with the detail in the log.
	w = ts.do(t, "POST", "/api/tuners/"+strconv.FormatInt(tn.ID, 10)+"/refresh", admin, nil)
	if strings.Contains(w.Body.String(), "dial tcp") || strings.Contains(w.Body.String(), "connection refused") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}
