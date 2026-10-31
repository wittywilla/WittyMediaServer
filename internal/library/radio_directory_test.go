package library

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

const (
	dirA = "11111111-1111-1111-1111-111111111111"
	dirB = "22222222-2222-2222-2222-222222222222"
	dirC = "33333333-3333-3333-3333-333333333333"
)

// fakeDirectory answers as the directory does, with hostile entries among the good.
type fakeDirectory struct {
	*httptest.Server
	searches, byUUID, clicks atomic.Int32
	stations                 map[string]map[string]any
	lastQuery                atomic.Value
}

func goodStation(id, name string) map[string]any {
	return map[string]any{"stationuuid": id, "name": name, "url": "http://stream.example/" + name, "url_resolved": "https://cdn.example/" + name + ".mp3", "homepage": "https://example.com/" + name,
		"favicon": "https://example.com/logo.png", "tags": "jazz,  smooth ,jazz,<b>funk</b>", "country": "Netherlands", "countrycode": "nl", "language": "dutch,english",
		"codec": "mp3", "bitrate": 128, "hls": 0, "lastcheckok": 1, "votes": 5}
}

func newFakeDirectory(t *testing.T) *fakeDirectory {
	t.Helper()
	f := &fakeDirectory{stations: map[string]map[string]any{dirA: goodStation(dirA, "Alpha"), dirB: goodStation(dirB, "Bravo")}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.UserAgent(), "WittyMediaServer") {
			t.Errorf("no descriptive User-Agent: %q", r.UserAgent())
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/json/stations/search":
			f.searches.Add(1)
			f.lastQuery.Store(r.URL.RawQuery)
			if r.URL.Query().Get("hidebroken") != "true" {
				t.Error("search without hidebroken")
			}
			if r.URL.Query().Get("name") == "none" {
				_, _ = w.Write([]byte("[]"))
				return
			}
			_ = json.NewEncoder(w).Encode([]any{
				f.stations[dirA],
				map[string]any{"stationuuid": dirB, "name": "<script>alert(1)</script>Bad‮", "url": "http://cdn.example/b", "favicon": "javascript:alert(1)", "homepage": "http://192.168.1.1/admin", "tags": "x", "codec": "UNKNOWN", "bitrate": -5, "countrycode": "<>"},
				map[string]any{"stationuuid": "33333333-3333-3333-3333-333333333330", "name": "Private", "url": "http://192.168.1.10:8000/stream"},
				map[string]any{"stationuuid": "33333333-3333-3333-3333-333333333331", "name": "Loopback", "url": "http://localhost/x"},
				map[string]any{"stationuuid": "33333333-3333-3333-3333-333333333332", "name": "File", "url": "file:///etc/passwd"},
				map[string]any{"stationuuid": "33333333-3333-3333-3333-333333333333", "name": "Password", "url": "http://me:secret@cdn.example/x"},
				map[string]any{"stationuuid": "../../etc/passwd", "name": "Bad ID", "url": "http://cdn.example/y"},
			})
		case r.URL.Path == "/json/stations/byuuid":
			f.byUUID.Add(1)
			out := []any{}
			for _, id := range strings.Split(r.URL.Query().Get("uuids"), ",") {
				if st, ok := f.stations[id]; ok {
					out = append(out, st)
				}
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.URL.Path == "/json/stations/byurl":
			if r.URL.Query().Get("url") == "http://by.example/stream" {
				_ = json.NewEncoder(w).Encode([]any{goodStation(dirC, "Charlie")})
				return
			}
			_, _ = w.Write([]byte("[]"))
		case strings.HasPrefix(r.URL.Path, "/json/url/"):
			f.clicks.Add(1)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.URL.Path == "/json/countries":
			_, _ = w.Write([]byte(`[{"name":"Netherlands","iso_3166_1":"NL","stationcount":10},{"name":"<b>Bad</b>","iso_3166_1":"<>","stationcount":3},{"name":"Austria","iso_3166_1":"AT","stationcount":4}]`))
		case r.URL.Path == "/json/tags":
			_, _ = w.Write([]byte(`[{"name":"jazz","stationcount":10},{"name":"","stationcount":3}]`))
		case r.URL.Path == "/json/languages":
			_, _ = w.Write([]byte(`[{"name":"dutch","iso_639":"nl","stationcount":10}]`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.Close)
	realServers, realClient := RadioDirectoryServers, SafeClient
	RadioDirectoryServers = func(context.Context) []string { return []string{f.URL} }
	SafeClient = f.Client()
	ResetRadioDirectoryCache()
	t.Cleanup(func() { RadioDirectoryServers, SafeClient = realServers, realClient; ResetRadioDirectoryCache() })
	return f
}

func TestDirectoryAnswersAreCleaned(t *testing.T) {
	f := newFakeDirectory(t)
	got, err := DirectorySearch(context.Background(), DirectoryQuery{Text: "al"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("private, loopback, file, password and bad-ID entries should be dropped: %+v", got)
	}
	a := got[0]
	if a.ID != dirA || a.Name != "Alpha" || a.URL != "https://cdn.example/Alpha.mp3" || a.Country != "Netherlands" || a.CountryCode != "NL" || a.Language != "dutch" || a.Codec != "MP3" || a.Bitrate != 128 || !a.Working {
		t.Errorf("%+v", a)
	}
	if strings.Join(a.Tags, ",") != "jazz,smooth,funk" {
		t.Errorf("tags %q", a.Tags)
	}
	b := got[1]
	if strings.ContainsAny(b.Name, "<>‮") || b.Logo != "" || b.Homepage != "" || b.Codec != "" || b.Bitrate != 0 || b.CountryCode != "" {
		t.Errorf("hostile entry: %+v", b)
	}
	// The same question isn't asked twice.
	n := f.searches.Load()
	if _, err := DirectorySearch(context.Background(), DirectoryQuery{Text: "al"}); err != nil || f.searches.Load() != n {
		t.Errorf("not kept: %d to %d, %v", n, f.searches.Load(), err)
	}
	// Few by name: also looks for it as a genre.
	if _, err := DirectorySearch(context.Background(), DirectoryQuery{Text: "none"}); err != nil {
		t.Fatal(err)
	}
	if q, _ := f.lastQuery.Load().(string); !strings.Contains(q, "tag=none") {
		t.Errorf("no genre search: %s", q)
	}
	if _, err := DirectorySearch(context.Background(), DirectoryQuery{Country: "../.."}); err == nil {
		t.Error("a bad country code was sent on")
	}
	if _, err := DirectorySearch(context.Background(), DirectoryQuery{Tag: "jazz", Country: "nl", Language: "dutch"}); err != nil {
		t.Fatal(err)
	}
	if q, _ := f.lastQuery.Load().(string); !strings.Contains(q, "countrycode=NL") || !strings.Contains(q, "language=dutch") || !strings.Contains(q, "tag=jazz") {
		t.Errorf("filters: %s", q)
	}
	fc, err := DirectoryBrowse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(fc.Countries) != 2 || fc.Countries[0].Name != "Austria" || len(fc.Tags) != 1 || len(fc.Languages) != 1 {
		t.Errorf("%+v", fc)
	}
}

func TestDirectoryLimits(t *testing.T) {
	var calls atomic.Int32
	huge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("["))
		for i := 0; i < 1<<20; i++ {
			if _, err := w.Write([]byte(`{"name":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},`)); err != nil {
				return
			}
		}
	}))
	defer huge.Close()
	garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("<html>not json"))
	}))
	defer garbage.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer down.Close()
	realServers, realClient := RadioDirectoryServers, SafeClient
	defer func() { RadioDirectoryServers, SafeClient = realServers, realClient; ResetRadioDirectoryCache() }()
	ResetRadioDirectoryCache()
	SafeClient = huge.Client()
	RadioDirectoryServers = func(context.Context) []string { return []string{huge.URL, garbage.URL, down.URL, down.URL} }
	_, err := DirectorySearch(context.Background(), DirectoryQuery{Text: "x"})
	if !errors.Is(err, ErrDirectoryUnavailable) {
		t.Errorf("%v", err)
	}
	if calls.Load() != directoryTries {
		t.Errorf("asked %d servers, not %d", calls.Load(), directoryTries)
	}
	// A slow server doesn't hold things up for ever.
	stop := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-stop }))
	defer slow.Close()
	defer close(stop)
	RadioDirectoryServers = func(context.Context) []string { return []string{slow.URL} }
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := DirectorySearch(ctx, DirectoryQuery{Text: "y"}); err == nil {
		t.Error("no error from a silent server")
	}
	if time.Since(start) > 5*time.Second {
		t.Error("waited too long")
	}
}

func TestServersComeFromItsOwnDomainOnly(t *testing.T) {
	if !publicHost("radio.example") || publicHost("localhost") || publicHost("router") || publicHost("nas.local") || publicHost("10.1.2.3") || publicHost("[::1]") || !publicHost("93.184.216.34") {
		t.Error("publicHost")
	}
	if cleanStreamAddress("http://a.example/x y") != "" || cleanStreamAddress("http://u:p@a.example/") != "" || cleanStreamAddress("ftp://a.example/") != "" {
		t.Error("cleanStreamAddress")
	}
}

func openRadioStore(t *testing.T) (*Store, *Library) {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s := &Store{DB: d}
	lib, err := s.CreateLibrary(context.Background(), "Radio", "radio", nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, lib
}

func radioMetaOf(t *testing.T, s *Store, it *Item) *RadioMeta {
	t.Helper()
	got, err := s.Item(context.Background(), it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AttachRadio(context.Background(), 0, []*Item{got}); err != nil {
		t.Fatal(err)
	}
	*it = *got
	return got.Radio
}

func TestAddFromDirectoryAndRefresh(t *testing.T) {
	f := newFakeDirectory(t)
	s, lib := openRadioStore(t)
	ctx := context.Background()
	it, err := s.AddFromDirectory(ctx, lib.ID, dirA)
	if err != nil {
		t.Fatal(err)
	}
	m := radioMetaOf(t, s, it)
	if it.Title != "Alpha" || it.Poster != "https://example.com/logo.png" || m.Source != RadioDirectorySource || m.SourceID != dirA || m.Country != "Netherlands" || m.Status != "ok" || m.Bitrate != 128 {
		t.Errorf("%+v %+v", it, m)
	}
	for i := 0; i < 100 && f.clicks.Load() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if f.clicks.Load() != 1 {
		t.Errorf("clicks: %d", f.clicks.Load())
	}
	if _, err := s.AddFromDirectory(ctx, lib.ID, "not-an-id"); err == nil {
		t.Error("a bad ID was sent on")
	}
	if _, err := s.AddFromDirectory(ctx, lib.ID, dirC); err == nil {
		t.Error("a station the directory doesn't have was added")
	}
	have, _ := s.RadioAddresses(ctx, lib.ID)
	if !have[dirA] || !have[it.Path] {
		t.Errorf("%v", have)
	}

	// Refreshed only once it's due, and quietly: one request for the batch.
	n := f.byUUID.Load()
	if err := s.refreshFromDirectory(ctx); err != nil || f.byUUID.Load() != n {
		t.Errorf("refreshed early: %v %d", err, f.byUUID.Load()-n)
	}
	due := func() { _, _ = s.DB.ExecContext(ctx, `UPDATE radio_meta SET dir_checked = 0`) }
	due()
	f.stations[dirA]["favicon"] = "https://example.com/new.png"
	f.stations[dirA]["lastcheckok"] = 0
	f.stations[dirA]["url_resolved"] = "https://cdn.example/moved.mp3"
	if err := s.refreshFromDirectory(ctx); err != nil {
		t.Fatal(err)
	}
	m = radioMetaOf(t, s, it)
	if it.Poster != "https://example.com/new.png" || m.Status != "dead" {
		t.Errorf("after refresh: %q %+v", it.Poster, m)
	}
	if it.Path == "https://cdn.example/moved.mp3" {
		t.Error("a working station's address changed by itself")
	}
	// It stops answering here and has moved: follows it.
	s.RadioResult(ctx, it.ID, errors.New("gone"))
	due()
	if err := s.refreshFromDirectory(ctx); err != nil {
		t.Fatal(err)
	}
	if radioMetaOf(t, s, it); it.Path != "https://cdn.example/moved.mp3" {
		t.Errorf("didn't follow: %s", it.Path)
	}
	// Gone from the directory: noticed, not silently kept.
	delete(f.stations, dirA)
	due()
	if err := s.refreshFromDirectory(ctx); err != nil {
		t.Fatal(err)
	}
	if m = radioMetaOf(t, s, it); m.Status == "ok" || m.Reason == "" {
		t.Errorf("gone: %+v", m)
	}
}

func TestStationStatus(t *testing.T) {
	s, lib := openRadioStore(t)
	ctx := context.Background()
	old := radioFailGap
	defer func() { radioFailGap = old }()
	it, err := s.AddRadioStation(ctx, lib.ID, RadioStation{Name: "A", URL: "https://a.example/s"})
	if err != nil {
		t.Fatal(err)
	}
	if m := radioMetaOf(t, s, it); m.Status != "unknown" {
		t.Errorf("new: %+v", m)
	}
	// Failures close together count once.
	radioFailGap = 600
	for range 5 {
		s.RadioResult(ctx, it.ID, errors.New("the station answered 503"))
	}
	m := radioMetaOf(t, s, it)
	if m.Status != "trouble" || !strings.Contains(m.Reason, "503") {
		t.Errorf("after one outage: %+v", m)
	}
	// Spread out, they mark it dead.
	radioFailGap = -1
	s.RadioResult(ctx, it.ID, ErrNotAudio)
	s.RadioResult(ctx, it.ID, ErrNotAudio)
	if m = radioMetaOf(t, s, it); m.Status != "dead" || !strings.Contains(m.Reason, "audio") {
		t.Errorf("after three: %+v", m)
	}
	// One good answer brings it back.
	s.RadioResult(ctx, it.ID, nil)
	if m = radioMetaOf(t, s, it); m.Status != "ok" || m.Reason != "" || m.LastOK == 0 {
		t.Errorf("recovered: %+v", m)
	}
	// Facts fill gaps only, and are kept when re-adding.
	s.ApplyStationFacts(ctx, it, StationFacts{Name: "Ignored", Genre: "Rock", Description: "About", Homepage: "https://a.example/", Codec: "MP3", Bitrate: 64})
	m = radioMetaOf(t, s, it)
	if it.Title != "A" || it.Genres[0] != "Rock" || it.Overview != "About" || m.Homepage != "https://a.example/" || m.Bitrate != 64 {
		t.Errorf("%+v %+v", it, m)
	}
	s.ApplyStationFacts(ctx, it, StationFacts{Genre: "Pop", Bitrate: 128, Codec: "AAC"})
	if m = radioMetaOf(t, s, it); it.Genres[0] != "Rock" || m.Bitrate != 64 || m.Codec != "MP3" {
		t.Errorf("overwrote: %+v %+v", it, m)
	}
	again, err := s.AddRadioStation(ctx, lib.ID, RadioStation{Name: "B", URL: "https://a.example/s"})
	if err != nil || again.ID != it.ID || again.Overview != "About" || again.Genres[0] != "Rock" {
		t.Errorf("re-add: %+v %v", again, err)
	}
	// A station added by address alone takes the name it gives.
	bare, _ := s.AddRadioStation(ctx, lib.ID, RadioStation{URL: "https://bare.example:8000/s"})
	s.ApplyStationFacts(ctx, bare, StationFacts{Name: "Named"})
	if radioMetaOf(t, s, bare); bare.Title != "Named" {
		t.Errorf("%q", bare.Title)
	}
	// Removing a station removes its details.
	if err := s.RemoveRadioStation(ctx, it.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM radio_meta WHERE item_id = ?`, it.ID).Scan(&n)
	if n != 0 {
		t.Error("details left behind")
	}
}

func TestTypedHLSAddressIsKnownAtOnce(t *testing.T) {
	s, lib := openRadioStore(t)
	ctx := context.Background()
	it, err := s.AddRadioStation(ctx, lib.ID, RadioStation{Name: "H", URL: "https://x.example/live/Index.M3U8?a=1"})
	if err != nil {
		t.Fatal(err)
	}
	if m := radioMetaOf(t, s, it); !m.HLS {
		t.Errorf("%+v", m)
	}
	// What the stream turns out to be wins.
	s.ApplyStationFacts(ctx, it, StationFacts{HLS: false, Codec: "MP3"})
	if m := radioMetaOf(t, s, it); m.HLS {
		t.Errorf("not corrected: %+v", m)
	}
	plain, _ := s.AddRadioStation(ctx, lib.ID, RadioStation{Name: "P", URL: "https://x.example/stream.mp3"})
	if m := radioMetaOf(t, s, plain); m.HLS {
		t.Errorf("%+v", m)
	}
}

func TestLookUpKeepsPrivateAddressesPrivate(t *testing.T) {
	f := newFakeDirectory(t)
	_ = f
	s, lib := openRadioStore(t)
	ctx := context.Background()
	for _, u := range []string{"https://x.example/s?key=SECRET", "https://me:pw@x.example/s"} {
		it, err := s.AddRadioStation(ctx, lib.ID, RadioStation{URL: u, Name: "P"})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.LookUpRadio(ctx, it); err == nil {
			t.Errorf("%s was sent to the directory", u)
		}
	}
	it, _ := s.AddRadioStation(ctx, lib.ID, RadioStation{URL: "http://by.example/stream", Name: "Mine"})
	if err := s.LookUpRadio(ctx, it); err != nil {
		t.Fatal(err)
	}
	if m := radioMetaOf(t, s, it); it.Title != "Mine" || it.Poster == "" || m.Country != "Netherlands" {
		t.Errorf("%+v %+v", it, m)
	}
	no, _ := s.AddRadioStation(ctx, lib.ID, RadioStation{URL: "http://unknown.example/s", Name: "U"})
	if err := s.LookUpRadio(ctx, no); !errors.Is(err, ErrNotFound) {
		t.Errorf("%v", err)
	}
}

func TestChecksDontBlameStationsWhenThisServerIsOffline(t *testing.T) {
	s, lib := openRadioStore(t)
	ctx := context.Background()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dead" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("icy-name", "Good")
		_, _ = w.Write(make([]byte, 8192))
	}))
	defer ok.Close()
	real := SafeClient
	SafeClient = ok.Client()
	defer func() { SafeClient = real }()
	logf := func(string, ...any) {}

	var offline []*Item
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		it, err := s.AddRadioStation(ctx, lib.ID, RadioStation{Name: n, URL: "http://127.0.0.1:1/" + n})
		if err != nil {
			t.Fatal(err)
		}
		offline = append(offline, it)
	}
	old := radioFailGap
	radioFailGap = -1
	defer func() { radioFailGap = old }()
	// Nothing at all answers: nobody is blamed.
	SafeClient = &http.Client{Transport: roundTripFail{}}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	checkFast(t, s, cctx, offline, logf)
	for _, it := range offline {
		if m := radioMetaOf(t, s, it); m.Status != "unknown" {
			t.Errorf("%s blamed for this server's trouble: %+v", it.Title, m)
		}
	}
	SafeClient = ok.Client()

	// One answers, one doesn't: it's the station.
	good, _ := s.AddRadioStation(ctx, lib.ID, RadioStation{URL: ok.URL + "/good"})
	dead, _ := s.AddRadioStation(ctx, lib.ID, RadioStation{Name: "Dead", URL: ok.URL + "/dead"})
	checkFast(t, s, cctx, []*Item{dead, good}, logf)
	if m := radioMetaOf(t, s, good); m.Status != "ok" {
		t.Errorf("%+v", m)
	}
	if m := radioMetaOf(t, s, dead); m.Status != "trouble" || !strings.Contains(m.Reason, "404") {
		t.Errorf("%+v", m)
	}
	// Stations not tried lately are due; the rest aren't.
	due, err := s.stationsDueForCheck(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range due {
		if it.ID == good.ID || it.ID == dead.ID {
			t.Errorf("%s was just checked", it.Title)
		}
	}
}

// checkFast runs a round without the pause between stations.
func checkFast(t *testing.T, s *Store, ctx context.Context, items []*Item, logf func(string, ...any)) {
	t.Helper()
	old := radioPauseFor
	radioPauseFor = time.Millisecond
	defer func() { radioPauseFor = old }()
	if !s.CheckStations(ctx, items, logf) {
		t.Fatal("a round was already running")
	}
}

type roundTripFail struct{}

func (roundTripFail) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no network")
}

func TestRefreshRadioRespectsItsSettings(t *testing.T) {
	f := newFakeDirectory(t)
	s, lib := openRadioStore(t)
	ctx := context.Background()
	if _, err := s.AddFromDirectory(ctx, lib.ID, dirA); err != nil {
		t.Fatal(err)
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE radio_meta SET dir_checked = 0`)
	scanner := &Scanner{Store: s, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	_ = s.DB.SetSetting(ctx, "radioDirectory", "0")
	n := f.byUUID.Load()
	scanner.RefreshRadio(ctx)
	if f.byUUID.Load() != n {
		t.Error("the directory was contacted with it turned off")
	}
	_ = s.DB.SetSetting(ctx, "radioDirectory", "1")
	scanner.RefreshRadio(ctx)
	if f.byUUID.Load() != n+1 {
		t.Error("no refresh with it turned on")
	}
}

func TestParsePlaylistIsHostileProof(t *testing.T) {
	got := ParsePlaylist("#EXTM3U\n#EXTINF:-1 group-title=\"<b>Rock</b>\",<script>x</script>Name‮\nhttps://a.example/s\njavascript:alert(1)\nfile:///etc/passwd\nhttp://\n")
	if len(got) != 1 || got[0].Name != "xName" || got[0].Genre != "Rock" {
		t.Errorf("%+v", got)
	}
	if got := ParsePlaylist("#EXTM3U\n#EXT-X-TARGETDURATION:6\nhttps://a.example/seg1.ts\n"); len(got) != 0 {
		t.Errorf("an HLS playlist's segments count as stations: %+v", got)
	}
	var b strings.Builder
	b.WriteString("[playlist]\n")
	for i := 0; i < maxPlaylistStations*2; i++ {
		b.WriteString("File" + itoaT(i) + "=https://a.example/" + itoaT(i) + "\n")
	}
	if got := ParsePlaylist(b.String()); len(got) != maxPlaylistStations {
		t.Errorf("%d stations", len(got))
	}
}

func itoaT(i int) string {
	var d []byte
	for {
		d = append([]byte{byte('0' + i%10)}, d...)
		if i /= 10; i == 0 {
			return string(d)
		}
	}
}
