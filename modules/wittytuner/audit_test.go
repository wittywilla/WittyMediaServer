package wittytuner

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// FFmpeg may pick any variant of a stream, so a clear first one says nothing.
func TestEveryHLSVariantIsChecked(t *testing.T) {
	const clear = "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nseg.ts\n"
	const locked = "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"https://k/k\"\n#EXTINF:6,\nseg.ts\n"
	mux := http.NewServeMux()
	serve := func(path, body string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
	}
	serve("/clear.m3u8", clear)
	serve("/locked.m3u8", locked)
	mux.HandleFunc("/gone.m3u8", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	serve("/both-clear.m3u8", "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nclear.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2\nclear.m3u8\n")
	serve("/second-locked.m3u8", "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nclear.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2\nlocked.m3u8\n")
	serve("/audio-locked.m3u8", "#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"a\",NAME=\"a\",URI=\"locked.m3u8\"\n#EXT-X-STREAM-INF:BANDWIDTH=1,AUDIO=\"a\"\nclear.m3u8\n")
	serve("/one-unread.m3u8", "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nclear.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2\ngone.m3u8\n")
	var many strings.Builder
	many.WriteString("#EXTM3U\n")
	for range maxHLSVariants + 1 {
		many.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=1\nclear.m3u8\n")
	}
	serve("/many.m3u8", many.String())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	st, err := openStream(t, srv.URL+"/both-clear.m3u8")
	if err != nil || !st.Verified {
		t.Errorf("all clear: %v %+v", err, st)
	}
	for _, p := range []string{"/second-locked.m3u8", "/audio-locked.m3u8"} {
		_, err := openStream(t, srv.URL+p)
		var e *EncryptedError
		if !errors.As(err, &e) {
			t.Errorf("%s should be refused as encrypted, got %v", p, err)
		}
	}
	// It may still play, but nobody can say it's clear.
	for _, p := range []string{"/one-unread.m3u8", "/many.m3u8"} {
		st, err := openStream(t, srv.URL+p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
		} else if st.Verified {
			t.Errorf("%s was not read all the way through, and was called clear", p)
		}
	}
}

func withLookup(t *testing.T, ips map[string]string) {
	t.Helper()
	old := lookupHost
	lookupHost = func(host string) ([]net.IP, error) {
		if s, ok := ips[host]; ok {
			return []net.IP{net.ParseIP(s)}, nil
		}
		return nil, errors.New("no such host")
	}
	t.Cleanup(func() { lookupHost = old })
}

// A playlist from elsewhere names this machine by a name as easily as by its address.
func TestPlaylistFromElsewhereCantLeadHomeByName(t *testing.T) {
	withLookup(t, map[string]string{"sneaky.example": "127.0.0.1", "wide.example": "0.0.0.0", "meta.example": "169.254.169.254", "fine.example": "93.184.216.34"})
	for _, u := range []string{"http://sneaky.example/x", "https://sneaky.example./x", "http://wide.example/x", "http://meta.example/x"} {
		if checkOpening(u, false) == nil {
			t.Errorf("%s went through", u)
		}
	}
	for _, u := range []string{"http://fine.example/x", "http://unknown.example/x"} {
		if err := checkOpening(u, false); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	// One the owner's own network or playlist names is theirs to choose.
	if err := checkOpening("http://sneaky.example/x", true); err != nil {
		t.Errorf("a local playlist: %v", err)
	}
	// Parsing a playlist of thousands of channels doesn't ask about each.
	withLookup(t, nil)
	asked := false
	lookupHost = func(string) ([]net.IP, error) { asked = true; return nil, errors.New("no") }
	if err := checkStreamURL("http://any.example/x", false); err != nil || asked {
		t.Errorf("%v, asked %v", err, asked)
	}
}

// A redirect is a way home too.
func TestRedirectsCantLeadHome(t *testing.T) {
	withLookup(t, map[string]string{"sneaky.example": "127.0.0.1"})
	home := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "#EXTM3U\n") }))
	defer home.Close()
	for _, to := range []string{home.URL + "/x.m3u8", "http://sneaky.example/x.m3u8", "http://[::1]/x.m3u8"} {
		c := noWayHome(&http.Client{Transport: rewrite{to: to}})
		res, err := c.Get("http://start.example/")
		if err == nil {
			res.Body.Close()
			t.Errorf("followed to %s", to)
		}
	}
}

// rewrite answers everything with a redirect, with no network.
type rewrite struct{ to string }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	http.Redirect(w, req, r.to, http.StatusFound)
	return w.Result(), nil
}
