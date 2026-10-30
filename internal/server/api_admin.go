package server

import (
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

var settingKeys = []string{"tmdbKey", "tvdbKey", "tvdbPin", "hwAccel", "maxHeight", "menuLanguage", "audioLanguage", "subtitleLanguage",
	"serverName", "audioCodec", "lowPower", "bdjEnabled", "javaPath", "dvdRegion", "blurayRegion", "dvdParental", "blurayAge", "discCountry", "saveMetadata", "dvdStart", "insertFx", "episodeView",
	"hwEncoders", "directPlay", "qualityMode", "qualityProfile", "x264Preset", "crf", "maxBitrate", "audioBitrate",
	"ffmpegVideoArgs", "ffmpegOutputArgs", "theme", "brandName", "maxSpeed",
	"loginStyle", "profilesRemote", "setupDone", "kidsManagedBy", "insertSound", "tvmaze", "wikidata", "animeIn", "animeTitles",
	"libraryGrouping", "updateCheck", "uploads", "uploadMaxMb", "shareLinks", "mapTiles"}

// settingDefaults are returned when a key has never been saved.
var settingDefaults = map[string]string{"saveMetadata": "1", "dvdStart": "menu", "insertFx": "1", "episodeView": "cards",
	"theme": "default", "directPlay": "1", "qualityMode": "easy", "qualityProfile": "balanced", "x264Preset": "veryfast", "crf": "21", "maxSpeed": "4",
	"loginStyle": "profiles", "profilesRemote": "0", "kidsManagedBy": "adults", "insertSound": "1", "tvmaze": "1", "wikidata": "1", "animeIn": "both", "animeTitles": "english",
	// libraryGrouping: another folder of a kind joins that kind's library
	// ("combine"), or becomes a library of its own ("separate").
	"libraryGrouping": "combine",
	// updateCheck: look for new releases on GitHub twice a day.
	"updateCheck": "1",
	// uploads and shareLinks: adults may add photos from their devices, and make
	// links for people without an account. mapTiles (off when empty) is an online
	// map provider's address for the photo map; nothing else ever contacts one.
	"uploads": "1", "uploadMaxMb": "4096", "shareLinks": "1", "mapTiles": ""}

// secretSettings are masked when read back.
var secretSettings = []string{"tmdbKey", "tvdbKey", "tvdbPin"}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{}
	for _, k := range settingKeys {
		out[k] = s.DB.GetSetting(r.Context(), k, settingDefaults[k])
	}
	for _, k := range secretSettings {
		if out[k] != "" {
			out[k+"Set"] = "1"
			out[k] = strings.Repeat("•", 8)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	for _, k := range settingKeys {
		v, ok := in[k]
		if !ok {
			continue
		}
		if strings.Contains(v, "•") { // masked secret sent back unchanged
			continue
		}
		v = strings.TrimSpace(v)
		if err := checkSetting(k, v); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if err := s.DB.SetSetting(r.Context(), k, v); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	s.handleGetSettings(w, r)
}

// validMapTiles: empty (no online map), or an https address of map squares.
func validMapTiles(v string) bool {
	if v == "" {
		return true
	}
	return strings.HasPrefix(v, "https://") && strings.Contains(v, "{z}") && strings.Contains(v, "{x}") && strings.Contains(v, "{y}") && len(v) < 400 && !strings.ContainsAny(v, "\"'<> ")
}

// checkSetting refuses values that would reach FFmpeg's command line or build
// file names unless they are what the setting is meant to hold. An account
// that can change settings must not get the machine's file system with them.
func checkSetting(k, v string) error {
	switch k {
	case "mapTiles":
		if !validMapTiles(v) {
			return errors.New("the map address must start with https:// and contain {z}, {x} and {y}")
		}
	case "ffmpegVideoArgs", "ffmpegOutputArgs":
		return ffmpeg.CheckExpertArgs(ffmpeg.SplitArgs(v))
	case "x264Preset":
		if v != "" && !ffmpeg.ValidPreset(v) {
			return errors.New("unknown encoder preset")
		}
	case "maxBitrate", "audioBitrate":
		if !ffmpeg.ValidRate(v) {
			return errors.New(`a bitrate looks like "8M" or "192k"`)
		}
	case "crf", "maxHeight", "maxSpeed":
		if v != "" {
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				return errors.New(k + " must be a number")
			}
		}
	case "audioCodec":
		if v != "" && v != "aac" && v != "mp3" {
			return errors.New("the audio codec must be aac or mp3")
		}
	case "serverName", "brandName":
		if len([]rune(v)) > 80 {
			return errors.New("the name must be at most 80 characters")
		}
	}
	return nil
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	hwList, hwDone := s.Tools.HardwareStatus()
	var encoders []string
	for _, e := range []string{"libx264", "libopenh264", "h264_nvenc", "h264_qsv", "h264_amf", "h264_vaapi", "h264_videotoolbox"} {
		if s.Tools.Encoders[e] {
			encoders = append(encoders, e)
		}
	}
	writeJSON(w, 200, map[string]any{
		"lanUrls":     s.lanURLs(r),
		"version":     s.Version,
		"go":          runtime.Version(),
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"hostname":    host,
		"dataDir":     s.Cfg.DataDir,
		"ffmpeg":      s.Tools.FFmpeg,
		"ffmpegInfo":  s.Tools.Version,
		"encoders":    encoders,
		"hardware":    hwList,
		"hwTested":    hwDone,
		"dvdSessions": s.DVD.Count(),
		"blurays":     s.BD.Count(),
	})
}

// spa serves the embedded web client with index.html fallback.
func (s *Server) spa() http.Handler {
	fileServer := http.FileServer(http.FS(s.Web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := s.Web.Open(p); err == nil {
			st, _ := f.Stat()
			f.Close()
			if st != nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, 404, "not found")
			return
		}
		b, err := fs.ReadFile(s.Web, "index.html")
		if err != nil {
			http.Error(w, "web UI not built", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(b)
	})
}

// lanURLs are the addresses other devices on the home network can open
// (this machine's private IPv4 addresses, on the port this request used);
// none when the server only listens on this machine.
func (s *Server) lanURLs(r *http.Request) []string {
	if h, _, err := net.SplitHostPort(s.Cfg.Listen); err == nil && h != "" && h != "0.0.0.0" && h != "::" {
		if ip := net.ParseIP(h); ip == nil || ip.IsLoopback() {
			return []string{}
		}
	}
	_, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		port = ""
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	out := []string{}
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.To4() == nil || !ipn.IP.IsPrivate() {
			continue
		}
		host := ipn.IP.String()
		if port != "" {
			host = net.JoinHostPort(host, port)
		}
		out = append(out, scheme+"://"+host)
	}
	return out
}
