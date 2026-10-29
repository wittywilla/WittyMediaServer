package server

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
)

var settingKeys = []string{"tmdbKey", "tvdbKey", "tvdbPin", "hwAccel", "maxHeight", "menuLanguage", "audioLanguage", "subtitleLanguage",
	"serverName", "audioCodec", "lowPower", "saveMetadata", "dvdStart", "insertFx", "episodeView",
	"hwEncoders", "directPlay", "qualityMode", "qualityProfile", "x264Preset", "crf", "maxBitrate", "audioBitrate",
	"ffmpegVideoArgs", "ffmpegOutputArgs", "theme", "brandName", "maxSpeed",
	"loginStyle", "profilesRemote", "setupDone"}

// settingDefaults are returned when a key has never been saved.
var settingDefaults = map[string]string{"saveMetadata": "1", "dvdStart": "menu", "insertFx": "1", "episodeView": "cards",
	"theme": "default", "directPlay": "1", "qualityMode": "easy", "qualityProfile": "balanced", "x264Preset": "veryfast", "crf": "21", "maxSpeed": "4",
	"loginStyle": "profiles", "profilesRemote": "0"}

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
		if err := s.DB.SetSetting(r.Context(), k, strings.TrimSpace(v)); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	s.handleGetSettings(w, r)
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
