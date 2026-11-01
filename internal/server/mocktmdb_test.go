package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/themes"
)

// A whole server against a made-up TMDB, to try in a browser what needs a key: search with
// posters, artwork from a service, and streaming listings. It answers for any title, and
// says each is listed on services that don't exist. Nothing here is in the product. Start it
// with:
//
//	WMS_MOCK_SERVE=127.0.0.1:18461 go test ./internal/server -run TestServeWithMockTMDB -timeout 0
//
// It serves the web app from web/dist, so build that first. WMS_MOCK_DATA chooses its data folder.
func TestServeWithMockTMDB(t *testing.T) {
	addr := os.Getenv("WMS_MOCK_SERVE")
	if addr == "" {
		t.Skip("set WMS_MOCK_SERVE to run a server against a made-up TMDB")
	}
	dir := os.Getenv("WMS_MOCK_DATA")
	if dir == "" {
		dir = t.TempDir()
	}
	mock := httptest.NewServer(mockTMDB())
	defer mock.Close()
	// What would go to the real image host goes to the mock too, and a mock on this machine
	// is allowed, as it never is for a real server.
	safe := *library.SafeClient
	defer func() { *library.SafeClient = safe }()
	*library.SafeClient = http.Client{Transport: toMock{mock.URL}}

	d, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	tools, err := ffmpeg.Locate("", "")
	if err != nil {
		tools = &ffmpeg.Tools{Encoders: map[string]bool{}}
	}
	store := &library.Store{DB: d}
	cache := filepath.Join(dir, "cache")
	images := filepath.Join(cache, "images")
	h := hls.NewManager(tools, filepath.Join(dir, "transcode"), 2, log)
	scanner := library.NewScanner(store, tools, cache, func() string { return "made-up-key" }, log)
	scanner.ImagesDir = images
	scanner.TMDBBase = mock.URL
	scanner.SaveMeta = func() bool { return false }
	s := &Server{
		Cfg: &config.Config{DataDir: dir, ImagesDir: images, CacheDir: cache, MaxTranscode: 2}, DB: d, Log: log, Tools: tools, Store: store, Scanner: scanner,
		Auth: &auth.Service{DB: d}, HLS: h, DVD: dvdsession.NewManager(h, tools, dir, "/api", log), BD: bdsession.NewManager(h, tools, dir, "/api", log),
		Themes: &themes.Manager{Dir: filepath.Join(dir, "themes")}, Web: os.DirFS("../../web/dist"), Version: "mock",
	}
	if err := d.SetSetting(t.Context(), "saveMetadata", "0"); err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	s.StartMetadata()
	t.Logf("serving on %s, with a made-up TMDB on %s", addr, mock.URL)
	if err := http.ListenAndServe(addr, handler); err != nil {
		t.Fatal(err)
	}
}

// toMock sends everything to the mock.
type toMock struct{ base string }

func (m toMock) RoundTrip(r *http.Request) (*http.Response, error) {
	u, err := url.Parse(m.base + r.URL.Path)
	if err != nil {
		return nil, err
	}
	r2 := r.Clone(r.Context())
	r2.URL, r2.Host = u, ""
	return http.DefaultTransport.RoundTrip(r2)
}

func mockTMDB() http.Handler {
	var mu sync.Mutex
	titles := map[string]string{} // id to title
	idFor := func(title string) int {
		id := int(crc32.ChecksumIEEE([]byte(strings.ToLower(title))) % 1000000)
		mu.Lock()
		titles[strconv.Itoa(id)] = title
		mu.Unlock()
		return id
	}
	send := func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		q := r.URL.Query().Get("query")
		switch {
		case strings.HasSuffix(r.URL.Path, ".jpg"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(mockPicture(r.URL.Path))
		case r.URL.Path == "/search/movie":
			id := idFor(q)
			send(w, map[string]any{"results": []map[string]any{
				{"id": id, "title": q, "release_date": "2001-05-01", "overview": "What the made-up TMDB says about " + q + ".", "poster_path": fmt.Sprintf("/p%d.jpg", id), "vote_average": 7.5},
				{"id": idFor(q + " Returns"), "title": q + " Returns", "release_date": "2011-05-01", "overview": "A made-up sequel.", "poster_path": fmt.Sprintf("/s%d.jpg", id), "vote_average": 5.5},
			}})
		case r.URL.Path == "/search/tv":
			id := idFor(q)
			send(w, map[string]any{"results": []map[string]any{{"id": id, "name": q, "first_air_date": "2002-01-01", "overview": "A made-up show.", "poster_path": fmt.Sprintf("/t%d.jpg", id), "vote_average": 6.5}}})
		case len(parts) == 4 && parts[2] == "watch" && parts[3] == "providers":
			n, _ := strconv.Atoi(parts[1])
			us := []map[string]any{{"provider_name": "Streamly"}}
			if n%2 == 0 {
				us = append(us, map[string]any{"provider_name": "Cinema Box Plus"})
			}
			send(w, map[string]any{"results": map[string]any{
				"US": map[string]any{"flatrate": us, "ads": []map[string]any{{"provider_name": "Free With Adverts"}}, "rent": []map[string]any{{"provider_name": "Rentals Only"}}},
				"GB": map[string]any{"flatrate": []map[string]any{{"provider_name": "Island Stream"}}},
			}})
		case len(parts) == 2 && parts[0] == "movie":
			mu.Lock()
			title := titles[parts[1]]
			mu.Unlock()
			if title == "" {
				w.WriteHeader(404)
				return
			}
			first, _, _ := strings.Cut(title, " ")
			send(w, map[string]any{"id": mustInt(parts[1]), "title": title, "release_date": "2001-05-01", "overview": "What the made-up TMDB says about " + title + ".", "tagline": "A made-up tagline.",
				"poster_path": "/p" + parts[1] + ".jpg", "backdrop_path": "/b" + parts[1] + ".jpg", "vote_average": 7.5, "runtime": 90,
				"genres":                 []map[string]string{{"name": "Comedy"}, {"name": "Family"}},
				"production_companies":   []map[string]string{{"name": "Lantern Works"}, {"name": "Small Pictures"}},
				"belongs_to_collection":  map[string]string{"name": first + " Collection"},
				"credits":                map[string]any{"crew": []map[string]string{{"name": "Dee Rector", "job": "Director"}}},
				"release_dates":          map[string]any{"results": []map[string]any{{"iso_3166_1": "US", "release_dates": []map[string]string{{"certification": "PG"}}}}},
				"images":                 map[string]any{"posters": []map[string]string{{"file_path": "/p" + parts[1] + ".jpg"}, {"file_path": "/q" + parts[1] + ".jpg"}}, "backdrops": []map[string]string{{"file_path": "/b" + parts[1] + ".jpg"}}, "logos": []map[string]string{}},
			})
		case len(parts) == 2 && parts[0] == "tv":
			mu.Lock()
			title := titles[parts[1]]
			mu.Unlock()
			send(w, map[string]any{"id": mustInt(parts[1]), "name": title, "first_air_date": "2002-01-01", "overview": "A made-up show.", "poster_path": "/t" + parts[1] + ".jpg", "vote_average": 6.5,
				"genres": []map[string]string{{"name": "Comedy"}}, "networks": []map[string]string{{"name": "Made Up TV"}}, "seasons": []map[string]any{}})
		default:
			w.WriteHeader(404)
			_, _ = io.WriteString(w, "{}")
		}
	})
}

func mustInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// mockPicture is a plain picture, a colour of its own for each address.
func mockPicture(path string) []byte {
	sum := crc32.ChecksumIEEE([]byte(path))
	m := image.NewRGBA(image.Rect(0, 0, 200, 300))
	c := color.RGBA{uint8(sum), uint8(sum >> 8), uint8(sum >> 16), 255}
	for y := 0; y < 300; y++ {
		for x := 0; x < 200; x++ {
			m.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, m)
	return b.Bytes()
}
