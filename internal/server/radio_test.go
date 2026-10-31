package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

func TestRadioRelay(t *testing.T) {
	ts := newGalleryServer(t)
	ctx := context.Background()
	lib, err := ts.s.Store.CreateLibrary(ctx, "Radio", "radio", nil)
	if err != nil {
		t.Fatal(err)
	}
	station := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/audio":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("mp3 bytes"))
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<script>alert(1)</script>"))
		default:
			w.WriteHeader(404)
		}
	}))
	defer station.Close()
	add := func(path string) string {
		t.Helper()
		it, err := ts.s.Store.AddRadioStation(ctx, lib.ID, library.RadioStation{Name: path, URL: station.URL + path})
		if err != nil {
			t.Fatal(err)
		}
		return itoa(it.ID)
	}
	audio, page, gone := add("/audio"), add("/page"), add("/gone")

	// The real client refuses this machine and its network.
	if w := ts.do("GET", "/api/radio/"+audio+"/stream", ts.adult, nil); w.Code != 502 {
		t.Errorf("a station on this machine was reached: %d %s", w.Code, w.Body)
	}
	real := library.SafeClient
	library.SafeClient = station.Client() // from here on, the test's station is reachable
	defer func() { library.SafeClient = real }()

	w := ts.do("GET", "/api/radio/"+audio+"/stream", ts.adult, nil)
	if w.Code != 200 || w.Body.String() != "mp3 bytes" || w.Header().Get("Content-Type") != "audio/mpeg" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("relay: %d %q %v", w.Code, w.Body, w.Header())
	}
	if w := ts.do("GET", "/api/radio/"+page+"/stream", ts.adult, nil); w.Code != 502 || strings.Contains(w.Body.String(), "<script>") {
		t.Errorf("a web page came through: %d %s", w.Code, w.Body)
	}
	if w := ts.do("GET", "/api/radio/"+gone+"/stream", ts.adult, nil); w.Code != 502 {
		t.Errorf("a station that answers 404: %d", w.Code)
	}
	// Who may do what.
	if w := ts.do("GET", "/api/radio/"+audio+"/stream", "", nil); w.Code != 401 {
		t.Errorf("no account: %d", w.Code)
	}
	if w := ts.do("GET", "/api/radio/"+audio+"/stream", ts.outside, nil); w.Code != 404 {
		t.Errorf("an account without this library: %d", w.Code)
	}
	if w := ts.do("POST", "/api/radio", ts.adult, map[string]any{"libraryId": lib.ID, "url": "https://x.example/"}); w.Code != 403 {
		t.Errorf("a non-admin added a station: %d", w.Code)
	}
	if w := ts.do("DELETE", "/api/radio/"+audio, ts.adult, nil); w.Code != 403 {
		t.Errorf("a non-admin removed a station: %d", w.Code)
	}
	// Listening counts for Your year.
	var plays int
	_ = ts.s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM item_stats WHERE item_id = ?`, audio).Scan(&plays)
	if plays == 0 {
		t.Error("listening wasn't recorded")
	}
}

// radioFixture is a library and a test station that's reachable.
func radioFixture(t *testing.T, h http.HandlerFunc) (*galleryServer, *library.Library, *httptest.Server) {
	t.Helper()
	ts := newGalleryServer(t)
	lib, err := ts.s.Store.CreateLibrary(context.Background(), "Radio", "radio", nil)
	if err != nil {
		t.Fatal(err)
	}
	st := httptest.NewServer(h)
	t.Cleanup(st.Close)
	real := library.SafeClient
	library.SafeClient = st.Client()
	t.Cleanup(func() { library.SafeClient = real })
	return ts, lib, st
}

func addStation(t *testing.T, ts *galleryServer, lib *library.Library, name, addr string) *library.Item {
	t.Helper()
	it, err := ts.s.Store.AddRadioStation(context.Background(), lib.ID, library.RadioStation{Name: name, URL: addr})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func TestRadioIcyTitlesLeaveTheAudio(t *testing.T) {
	ts, lib, st := radioFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Icy-MetaData") != "1" {
			t.Error("metadata wasn't asked for")
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("icy-metaint", "4")
		w.Header().Set("icy-name", "Fake <b>FM</b>")
		w.Header().Set("icy-genre", "Jazz")
		w.Header().Set("icy-br", "128,128")
		w.Header().Set("icy-url", "javascript:alert(1)")
		text := "StreamTitle='Miles - So What';"
		n := (len(text) + 15) / 16
		block := append([]byte{byte(n)}, text...)
		block = append(block, make([]byte, n*16-len(text))...)
		_, _ = w.Write(append(append([]byte("aaaa"), block...), "bbbb"...))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	it := addStation(t, ts, lib, "", st.URL+"/s")
	id := itoa(it.ID)
	w := ts.do("GET", "/api/radio/"+id+"/stream", ts.adult, nil)
	if w.Code != 200 || w.Body.String() != "aaaabbbb" {
		t.Fatalf("metadata left in the audio: %d %q", w.Code, w.Body)
	}
	// What the station said about itself, cleaned, fills the gaps.
	w = ts.do("GET", "/api/radio/"+id, ts.admin, nil)
	var got library.Item
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != "Fake FM" || got.Radio == nil || got.Radio.Codec != "MP3" || got.Radio.Bitrate != 128 || got.Radio.Homepage != "" || got.Radio.Status != "ok" || got.Genres[0] != "Jazz" {
		t.Errorf("%+v %+v", got, got.Radio)
	}

	// While someone listens, the song is known; afterwards there's nothing to say.
	ts.s.radio.begin(it.ID)
	ts.s.radio.setTitle(it.ID, "Miles - So What")
	var now library.NowPlaying
	w = ts.do("GET", "/api/radio/"+id+"/now", ts.adult, nil)
	_ = json.Unmarshal(w.Body.Bytes(), &now)
	if !now.Live || now.Artist != "Miles" || now.Track != "So What" {
		t.Errorf("%s", w.Body)
	}
	ts.s.radio.end(it.ID)
	w = ts.do("GET", "/api/radio/"+id+"/now", ts.adult, nil)
	now = library.NowPlaying{}
	_ = json.Unmarshal(w.Body.Bytes(), &now)
	if now.Live || now.Title != "" {
		t.Errorf("after: %s", w.Body)
	}
	if w := ts.do("GET", "/api/radio/"+id+"/now", ts.outside, nil); w.Code != 404 {
		t.Errorf("an account without this library: %d", w.Code)
	}
	if w := ts.do("GET", "/api/radio/"+id+"/now", "", nil); w.Code != 401 {
		t.Errorf("no account: %d", w.Code)
	}
}

func TestRadioAddressesStayWithAdmins(t *testing.T) {
	ts, lib, _ := radioFixture(t, func(w http.ResponseWriter, r *http.Request) {})
	it := addStation(t, ts, lib, "Private", "https://x.example/s?key=SECRET")
	for _, path := range []string{"/api/radio?library=" + itoa(lib.ID), "/api/radio/" + itoa(it.ID)} {
		if w := ts.do("GET", path, ts.adult, nil); w.Code != 200 || strings.Contains(w.Body.String(), "SECRET") {
			t.Errorf("%s: %d %s", path, w.Code, w.Body)
		}
		if w := ts.do("GET", path, ts.admin, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "SECRET") {
			t.Errorf("%s (admin): %d", path, w.Code)
		}
		if w := ts.do("GET", path, ts.outside, nil); w.Code == 200 && strings.Contains(w.Body.String(), "Private") {
			t.Errorf("%s: an account without this library saw it: %s", path, w.Body)
		}
	}
}

// Everywhere else an item can be read, too.
func TestRadioAddressesStayHiddenOnOtherRoutes(t *testing.T) {
	ts, lib, _ := radioFixture(t, func(w http.ResponseWriter, r *http.Request) {})
	it := addStation(t, ts, lib, "Findable", "https://x.example/s?key=SECRET")
	for _, path := range []string{"/api/items/" + itoa(it.ID), "/api/items?library=" + itoa(lib.ID), "/api/items?q=Findable", "/api/items?search=Findable"} {
		if w := ts.do("GET", path, ts.adult, nil); strings.Contains(w.Body.String(), "SECRET") {
			t.Errorf("%s: %s", path, w.Body)
		}
	}
}

// A logo goes through the same checks as a stream.
func TestRadioLogos(t *testing.T) {
	var private atomic.Int32
	ts, lib, st := radioFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/logo.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`))
		case "/page.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("<html><script>alert(1)</script>"))
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/vnd.microsoft.icon")
			_, _ = w.Write(append([]byte{0, 0, 1, 0, 1, 0}, make([]byte, 64)...))
		case "/private.png":
			private.Add(1)
		}
	})
	ctx := context.Background()
	setLogo := func(addr string) string {
		t.Helper()
		it := addStation(t, ts, lib, addr, "https://x.example/"+url.PathEscape(addr))
		if _, err := ts.s.DB.ExecContext(ctx, `UPDATE items SET poster = ? WHERE id = ?`, addr, it.ID); err != nil {
			t.Fatal(err)
		}
		return "/api/items/" + itoa(it.ID) + "/poster"
	}
	for _, bad := range []string{st.URL + "/logo.svg", st.URL + "/page.png", "javascript:alert(1)", "file:///etc/passwd"} {
		w := ts.do("GET", setLogo(bad), ts.adult, nil)
		if w.Code == 200 || strings.Contains(w.Body.String(), "script") {
			t.Errorf("%s: %d %s", bad, w.Code, w.Body)
		}
	}
	if w := ts.do("GET", setLogo(st.URL+"/favicon.ico"), ts.adult, nil); w.Code != 200 || w.Header().Get("Content-Type") != "image/x-icon" {
		t.Errorf("an icon: %d %v", w.Code, w.Header())
	}
	// With the real client, a logo can't lead onto this machine.
	test := library.SafeClient
	library.SafeClient = safeClientAtStart
	defer func() { library.SafeClient = test }()
	if w := ts.do("GET", setLogo(st.URL+"/private.png"), ts.adult, nil); w.Code == 200 || private.Load() != 0 {
		t.Errorf("reached this machine: %d", w.Code)
	}
}

func TestRadioFavouritesAndRecent(t *testing.T) {
	ts, lib, st := radioFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/aac")
		_, _ = w.Write([]byte("aac"))
	})
	it := addStation(t, ts, lib, "One", st.URL+"/a")
	id := itoa(it.ID)
	if w := ts.do("PUT", "/api/items/"+id+"/reaction", ts.adult, map[string]any{"value": 1}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	_ = ts.do("GET", "/api/radio/"+id+"/stream", ts.adult, nil)
	get := func(tok string) *library.RadioMeta {
		var out []library.Item
		w := ts.do("GET", "/api/radio?library="+itoa(lib.ID), tok, nil)
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out) != 1 {
			t.Fatalf("%s %v", w.Body, err)
		}
		return out[0].Radio
	}
	if m := get(ts.adult); !m.Favorite || m.LastPlayed == 0 {
		t.Errorf("%+v", m)
	}
	if m := get(ts.admin); m.Favorite || m.LastPlayed != 0 {
		t.Errorf("another account's: %+v", m)
	}
}

func TestRadioHLSRelay(t *testing.T) {
	var keyAsked, privateAsked atomic.Int32
	ts, lib, st := radioFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/live.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,Queen - Bohemian\nseg1.aac\n#EXTINF:6,\nsub/seg.ts\n"))
		case "/nested.m3u8":
			w.Header().Set("Content-Type", "application/x-mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=64000,CODECS=\"mp4a.40.2\"\nlive.m3u8\n"))
		case "/seg1.aac":
			w.Header().Set("Content-Type", "audio/aac")
			_, _ = w.Write([]byte("ID3\x03\x00\x00\x00\x00\x00\x15TIT2\x00\x00\x00\x0b\x00\x00\x00Track Name\x00AUDIO"))
		case "/sub/seg.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("TS"))
		case "/page.aac":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<script>x</script>"))
		case "/locked.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key\"\n#EXTINF:6,\nseg1.aac\n"))
		case "/key":
			keyAsked.Add(1)
		case "/private":
			privateAsked.Add(1)
		}
	})
	it := addStation(t, ts, lib, "HLS", st.URL+"/live.m3u8")
	id := itoa(it.ID)
	w := ts.do("GET", "/api/radio/"+id+"/stream", ts.adult, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/vnd.apple.mpegurl" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	body := w.Body.String()
	if strings.Contains(body, st.URL) || strings.Contains(body, "seg1.aac") {
		t.Errorf("station addresses reached the player:\n%s", body)
	}
	var links []string
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, "hls?") {
			links = append(links, l)
		}
	}
	if len(links) != 2 {
		t.Fatalf("%s", body)
	}
	seg := ts.do("GET", "/api/radio/"+id+"/"+links[0], ts.adult, nil)
	if seg.Code != 200 || !strings.Contains(seg.Body.String(), "AUDIO") || !strings.Contains(seg.Header().Get("Content-Security-Policy"), "sandbox") || seg.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("segment: %d %q %v", seg.Code, seg.Body, seg.Header())
	}
	if ts.do("GET", "/api/radio/"+id+"/"+links[1], ts.adult, nil).Code != 200 {
		t.Error("second segment")
	}
	var now library.NowPlaying
	_ = json.Unmarshal(ts.do("GET", "/api/radio/"+id+"/now", ts.adult, nil).Body.Bytes(), &now)
	if now.Title != "Track Name" || !now.Live {
		t.Errorf("%+v", now)
	}
	var hls bool
	_ = ts.s.DB.QueryRowContext(context.Background(), `SELECT hls FROM radio_meta WHERE item_id = ?`, it.ID).Scan(&hls)
	if !hls {
		t.Error("not noted as HLS")
	}

	// A signed address belongs to its station, and nothing unsigned is fetched:
	// the relay can't be used to reach anywhere else, on the internet or off it.
	other := itoa(addStation(t, ts, lib, "Other", st.URL+"/other").ID)
	if w := ts.do("GET", "/api/radio/"+other+"/"+links[0], ts.adult, nil); w.Code != 403 {
		t.Errorf("another station's address: %d", w.Code)
	}
	forge := func(addr, sig string) string {
		return "/api/radio/" + id + "/hls?u=" + base64.RawURLEncoding.EncodeToString([]byte(addr)) + "&s=" + url.QueryEscape(sig)
	}
	for _, path := range []string{forge(st.URL+"/private", ""), forge(st.URL+"/private", "AAAA"), "/api/radio/" + id + "/hls", "/api/radio/" + id + "/hls?u=!!!&s=x"} {
		if w := ts.do("GET", path, ts.adult, nil); w.Code != 403 {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
	if privateAsked.Load() != 0 {
		t.Error("an unsigned address was fetched")
	}
	if w := ts.do("GET", "/api/radio/"+id+"/"+links[0], "", nil); w.Code != 401 {
		t.Errorf("no account: %d", w.Code)
	}
	if w := ts.do("GET", "/api/radio/"+id+"/"+links[0], ts.outside, nil); w.Code != 404 {
		t.Errorf("an account without this library: %d", w.Code)
	}
	// A signed address that serves a page later is still refused, and with the real
	// client a signed address can't reach this machine at all.
	page := forge(st.URL+"/page.aac", ts.s.radio.sign(it.ID, st.URL+"/page.aac"))
	if w := ts.do("GET", page, ts.adult, nil); w.Code != 502 || strings.Contains(w.Body.String(), "<script>") {
		t.Errorf("a page came through: %d %s", w.Code, w.Body)
	}
	test := library.SafeClient
	library.SafeClient = realSafeClient(t)
	inner := forge(st.URL+"/private", ts.s.radio.sign(it.ID, st.URL+"/private"))
	if w := ts.do("GET", inner, ts.adult, nil); w.Code != 502 || privateAsked.Load() != 0 {
		t.Errorf("reached this machine: %d", w.Code)
	}
	library.SafeClient = test

	// Playlists within playlists lead back here too.
	nested := addStation(t, ts, lib, "Nested", st.URL+"/nested.m3u8")
	w = ts.do("GET", "/api/radio/"+itoa(nested.ID)+"/stream", ts.adult, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), st.URL) || !strings.Contains(w.Body.String(), "hls?") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	// Encrypted: refused and marked, and the key is never asked for.
	locked := addStation(t, ts, lib, "Locked", st.URL+"/locked.m3u8")
	if w := ts.do("GET", "/api/radio/"+itoa(locked.ID)+"/stream", ts.adult, nil); w.Code != 502 || !strings.Contains(w.Body.String(), "encrypted") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	if keyAsked.Load() != 0 {
		t.Error("a key was fetched")
	}
}

func realSafeClient(t *testing.T) *http.Client {
	t.Helper()
	return safeClientAtStart
}

var safeClientAtStart = library.SafeClient

func TestRadioPlaybackForNativeApps(t *testing.T) {
	ts, lib, st := radioFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("mp3"))
	})
	it := addStation(t, ts, lib, "App", st.URL+"/a")
	id := itoa(it.ID)
	w := ts.do("POST", "/api/items/"+id+"/playback", ts.adult, map[string]any{"profile": "appletv"})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var p playbackResponse
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Decision != "radio" || p.Protocol != "stream" || p.Seek != "none" || !strings.Contains(p.URL, "/api/radio/"+id+"/stream") || !strings.Contains(p.URL, "st=") || p.NowPlayingURL == "" {
		t.Errorf("%+v", p)
	}
	// A stream token opens this stream, without any other sign-in, and nothing else.
	if w := ts.do("GET", p.URL, "", nil); w.Code != 200 || w.Body.String() != "mp3" {
		t.Errorf("stream token: %d %q", w.Code, w.Body)
	}
	if w := ts.do("GET", "/api/radio/"+id+"?st="+p.StreamToken, "", nil); w.Code != 401 {
		t.Errorf("stream token on another route: %d", w.Code)
	}
	if !auth.IsStreamToken("st1x") && p.StreamToken == "" {
		t.Error("no token")
	}
}

func TestRadioDirectoryRoutes(t *testing.T) {
	const good = "11111111-1111-1111-1111-111111111111"
	var hits atomic.Int32
	ts, lib, dir := radioFixture(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		station := map[string]any{"stationuuid": good, "name": "Alpha", "url": "http://a.example/s", "url_resolved": "https://a.example/s.mp3", "favicon": "https://a.example/l.png", "tags": "jazz", "countrycode": "NL", "country": "Netherlands", "lastcheckok": 1, "codec": "AAC", "bitrate": 64}
		switch {
		case strings.HasPrefix(r.URL.Path, "/json/stations"):
			_ = json.NewEncoder(w).Encode([]any{station, map[string]any{"stationuuid": "22222222-2222-2222-2222-222222222222", "name": "Local", "url": "http://10.0.0.5/s"}})
		case r.URL.Path == "/json/countries", r.URL.Path == "/json/tags", r.URL.Path == "/json/languages":
			_, _ = w.Write([]byte(`[{"name":"X","iso_3166_1":"XX","stationcount":1}]`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	})
	realServers := library.RadioDirectoryServers
	library.RadioDirectoryServers = func(context.Context) []string { return []string{dir.URL} }
	library.ResetRadioDirectoryCache()
	t.Cleanup(func() { library.RadioDirectoryServers = realServers; library.ResetRadioDirectoryCache() })

	// Only administrators reach the directory.
	before := hits.Load()
	for _, tok := range []string{ts.adult, ts.outside} {
		if w := ts.do("GET", "/api/radio/directory/search?q=al", tok, nil); w.Code != 403 {
			t.Errorf("search: %d", w.Code)
		}
		if w := ts.do("GET", "/api/radio/directory/browse", tok, nil); w.Code != 403 {
			t.Errorf("browse: %d", w.Code)
		}
		if w := ts.do("POST", "/api/radio/directory/add", tok, map[string]any{"libraryId": lib.ID, "id": good}); w.Code != 403 {
			t.Errorf("add: %d", w.Code)
		}
	}
	if w := ts.do("GET", "/api/radio/directory/search?q=al", "", nil); w.Code != 401 {
		t.Errorf("no account: %d", w.Code)
	}
	if hits.Load() != before {
		t.Error("the directory was contacted for someone without the right")
	}

	var found []library.DirectoryStation
	search := "/api/radio/directory/search?q=alpha&library=" + itoa(lib.ID)
	w := ts.do("GET", search, ts.admin, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &found); err != nil || len(found) != 1 || found[0].ID != good || found[0].Added {
		t.Fatalf("%s %v", w.Body, err)
	}
	if w := ts.do("GET", "/api/radio/directory/browse", ts.admin, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"code":"XX"`) {
		t.Errorf("%d %s", w.Code, w.Body)
	}

	// Details come from the directory, whatever the request says.
	w = ts.do("POST", "/api/radio/directory/add", ts.admin, map[string]any{"libraryId": lib.ID, "id": good, "name": "Forged", "url": "http://192.168.0.1/"})
	var added library.Item
	if err := json.Unmarshal(w.Body.Bytes(), &added); w.Code != 201 || err != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if added.Title != "Alpha" || added.Path != "https://a.example/s.mp3" || added.Radio == nil || added.Radio.SourceID != good || added.Radio.Country != "Netherlands" {
		t.Errorf("%+v %+v", added, added.Radio)
	}
	w = ts.do("GET", search, ts.admin, nil)
	found = nil
	_ = json.Unmarshal(w.Body.Bytes(), &found)
	if len(found) != 1 || !found[0].Added {
		t.Errorf("not marked as added: %s", w.Body)
	}
	if w := ts.do("POST", "/api/radio/directory/add", ts.admin, map[string]any{"libraryId": lib.ID, "id": "../etc"}); w.Code != 400 {
		t.Errorf("bad ID: %d", w.Code)
	}
	// Into a library of another kind: no.
	if w := ts.do("POST", "/api/radio/directory/add", ts.admin, map[string]any{"libraryId": ts.lib.ID, "id": good}); w.Code == 201 {
		t.Error("added to a photos library")
	}

	// Turned off: nothing is sent.
	if err := ts.s.DB.SetSetting(context.Background(), "radioDirectory", "0"); err != nil {
		t.Fatal(err)
	}
	before = hits.Load()
	if w := ts.do("GET", "/api/radio/directory/search?q=other", ts.admin, nil); w.Code != 403 || !strings.Contains(w.Body.String(), "turned off") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	if hits.Load() != before {
		t.Error("the directory was contacted while turned off")
	}
}

func TestRadioChecksAndLookup(t *testing.T) {
	ts, lib, st := radioFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gone" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(make([]byte, 8192))
	})
	good, gone := addStation(t, ts, lib, "G", st.URL+"/good"), addStation(t, ts, lib, "D", st.URL+"/gone")
	for _, path := range []string{"/api/radio/" + itoa(good.ID) + "/check", "/api/radio/" + itoa(good.ID) + "/lookup"} {
		if w := ts.do("POST", path, ts.adult, nil); w.Code != 403 {
			t.Errorf("%s by a non-admin: %d", path, w.Code)
		}
	}
	check := func(it *library.Item) *library.RadioMeta {
		var got library.Item
		w := ts.do("POST", "/api/radio/"+itoa(it.ID)+"/check", ts.admin, nil)
		if err := json.Unmarshal(w.Body.Bytes(), &got); w.Code != 200 || err != nil {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		return got.Radio
	}
	if m := check(good); m.Status != "ok" {
		t.Errorf("%+v", m)
	}
	if m := check(gone); m.Status != "trouble" || !strings.Contains(m.Reason, "404") {
		t.Errorf("%+v", m)
	}
	if w := ts.do("POST", "/api/radio/check", ts.admin, map[string]any{"libraryId": lib.ID}); w.Code != 202 {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	// It runs on its own; it finishes before the test's data goes.
	ts.s.radioChecks.Wait()
}
