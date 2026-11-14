// A WittyTuner service for the tests (module API 5). It reads its manifest from beside itself.
// Its channels say how they behave when the server opens them:
//
//	1 clear MPEG-TS, 2 scrambled MPEG-TS, 3 an address FFmpeg mustn't open, 4 an HLS address to
//	look at again, 5 busy, 6 encrypted, 7 one stream at a time until it's let go.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittymodule/service"
)

func packets(n int, scrambled bool) []byte {
	p := bytes.Repeat([]byte{0x47}, 188)
	p[1], p[2], p[3] = 0x01, 0x00, 0x10
	if scrambled {
		p[3] |= 0x80
	}
	return bytes.Repeat(p, n)
}

var (
	mu    sync.Mutex
	seven int
	slots = map[string]int{}
	n     int
)

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /tv/channels", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"channels": []map[string]any{
			{"id": 1, "tunerId": 1, "number": "1", "name": "Clear", "enabled": true, "tunerEnabled": true, "tunerName": "Box", "driver": "network", "security": "unknown", "logoUrl": "https://logos.test/1.png"},
			{"id": 2, "tunerId": 1, "number": "2", "name": strings.Repeat("n", 5000), "enabled": true, "tunerEnabled": true, "security": "unknown", "logoUrl": "file:///etc/passwd"},
			{"id": 0, "tunerId": 1, "number": "0", "name": "No id"},
		}})
	})
	mux.HandleFunc("POST /tv/open", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Channel int64 `json:"channel"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		n++
		slot := string(rune('a'+n%26)) + strings.Repeat("x", n%5)
		switch in.Channel {
		case 1, 2:
			slots[slot] = int(in.Channel)
			write(w, 200, map[string]any{"slot": slot, "channel": in.Channel, "home": true})
		case 3:
			write(w, 200, map[string]any{"slot": slot, "channel": 3, "input": []string{"-i", "file:///etc/passwd"}, "verified": true})
		case 4:
			write(w, 200, map[string]any{"slot": slot, "channel": 4, "input": []string{"-protocol_whitelist", "http,https,tcp,tls", "-i", "http://stream.test/live.m3u8"}, "recheck": true})
		case 5:
			write(w, 503, map[string]string{"error": "all of this device's tuners are in use", "code": "busy"})
		case 6:
			write(w, 403, map[string]string{"error": "its playlist uses DRM", "code": "encrypted"})
		case 7:
			if seven > 0 {
				write(w, 503, map[string]string{"error": "busy", "code": "busy"})
				return
			}
			seven++
			slots[slot] = 7
			write(w, 200, map[string]any{"slot": slot, "channel": 7, "input": []string{"-protocol_whitelist", "http,https,tcp,tls", "-i", "http://stream.test/seven.m3u8"}})
		default:
			write(w, 404, map[string]string{"error": "not found", "code": "not-found"})
		}
	})
	mux.HandleFunc("GET /tv/stream/{slot}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ch := slots[r.PathValue("slot")]
		mu.Unlock()
		if ch == 0 {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write(packets(2000, ch == 2))
	})
	mux.HandleFunc("POST /tv/release", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Slot string `json:"slot"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		if slots[in.Slot] == 7 {
			seven--
		}
		delete(slots, in.Slot)
		mu.Unlock()
		write(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /tv/recheck", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"why": "its HLS playlist uses DRM", "next": 1})
	})
	mux.HandleFunc("POST /tv/note", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		write(w, 200, map[string]bool{"ok": true})
	})
}

func main() {
	exe, _ := os.Executable()
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(exe), wittymodule.ManifestFile))
	if err != nil {
		os.Exit(9)
	}
	service.Main(service.Config{Manifest: raw, Routes: routes})
}
