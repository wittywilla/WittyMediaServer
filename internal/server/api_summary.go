package server

import (
	"context"
	"log/slog"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// The summary a report starts with: what this server is and runs on. It goes in
// the log at every start, on the settings page and in the support bundle, so it
// holds no secrets. A setting is only here if it's on this list, so a new secret
// can't slip in; secrets only ever appear as whether they are set.
var summarySettings = []string{"hwAccel", "hwEncoders", "maxHeight", "audioCodec", "lowPower", "bdjEnabled", "dvdRegion", "blurayRegion", "saveMetadata", "directPlay", "qualityMode", "qualityProfile",
	"x264Preset", "crf", "maxBitrate", "audioBitrate", "maxSpeed", "tvmaze", "wikidata", "updateCheck", "moduleUpdateCheck", "moduleAutoInstall", "uploads", "shareLinks", "lanDiscovery", "remoteAccess", "remoteMode", "httpsEnabled"}

// FolderSpace is one folder the server uses.
type FolderSpace struct {
	Role  string `json:"role"`
	Path  string `json:"path"`
	Free  int64  `json:"free"`
	Total int64  `json:"total"`
	Error string `json:"error,omitempty"`
}

// SystemSummary is what a support report starts with.
type SystemSummary struct {
	Time         time.Time         `json:"time"`
	Version      string            `json:"version"`
	Go           string            `json:"go"`
	Started      time.Time         `json:"started"`
	System       sysinfo.Info      `json:"system"`
	Modules      []moduleInfo      `json:"modules"`
	FFmpeg       string            `json:"ffmpeg"`
	FFmpegInfo   string            `json:"ffmpegInfo"`
	Encoders     []string          `json:"encoders"`
	Hardware     []string          `json:"hardware,omitempty"`
	HardwareDone bool              `json:"hardwareTested"`
	Folders      []FolderSpace     `json:"folders"`
	Settings     map[string]string `json:"settings"`
	SecretsSet   []string          `json:"secretsSet"`
	Libraries    map[string]int    `json:"libraries"`
	MaxTranscode int               `json:"maxTranscode"`
	Log          logging.Health    `json:"log"`
	LogLevels    map[string]string `json:"logLevels"`
}

var serverStart = time.Now()

// SystemSummary gathers the summary. It starts no programs and doesn't go online.
func (s *Server) SystemSummary(ctx context.Context) SystemSummary {
	out := SystemSummary{Time: time.Now(), Version: s.Version, Go: runtime.Version(), Started: serverStart, System: sysinfo.Read(), Modules: builtModules(),
		Settings: map[string]string{}, SecretsSet: []string{}, Libraries: map[string]int{}, Folders: []FolderSpace{}, Encoders: []string{}}
	if s.Tools != nil {
		out.FFmpeg, out.FFmpegInfo = s.Tools.FFmpeg, s.Tools.Version
		for e, ok := range s.Tools.Encoders {
			if ok && (strings.Contains(e, "264") || strings.Contains(e, "hevc") || strings.Contains(e, "265")) {
				out.Encoders = append(out.Encoders, e)
			}
		}
		sort.Strings(out.Encoders)
		hw, done := s.Tools.HardwareStatus()
		out.HardwareDone = done
		for _, h := range hw {
			if !h.Compiled {
				continue
			}
			status := "not tested"
			switch {
			case h.Working:
				status = "works"
			case h.Tested:
				status = "failed: " + h.Error
			}
			out.Hardware = append(out.Hardware, h.Name+": "+status)
		}
	}
	add := func(role, p string) {
		if p == "" {
			return
		}
		f := FolderSpace{Role: role, Path: p}
		if d, err := sysinfo.DiskUsage(p); err != nil {
			f.Error = err.Error()
		} else {
			f.Free, f.Total = d.Free, d.Total
		}
		out.Folders = append(out.Folders, f)
	}
	if s.Cfg != nil {
		out.MaxTranscode = s.Cfg.MaxTranscode
		add("data", s.Cfg.DataDir)
		add("cache", s.Cfg.CacheDir)
		add("transcode", s.Cfg.TranscodeDir)
		add("images", s.Cfg.ImagesDir)
	}
	if s.Logs != nil {
		add("logs", s.Logs.Dir())
		out.Log = s.Logs.Health()
		out.LogLevels = s.Logs.Levels()
	}
	if s.Store != nil {
		if libs, err := s.Store.Libraries(ctx); err == nil {
			for _, l := range libs {
				out.Libraries[l.Type]++
				for _, p := range l.Paths {
					add("library", p)
				}
			}
		}
	}
	if s.DB != nil {
		for _, k := range summarySettings {
			if v := s.DB.GetSetting(ctx, k, settingDefaults[k]); v != "" {
				out.Settings[k] = v
			}
		}
		for _, k := range secretSettings {
			if s.DB.GetSetting(ctx, k, "") != "" {
				out.SecretsSet = append(out.SecretsSet, k)
			}
		}
	}
	return out
}

// GET /api/logs/summary (admin)
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.SystemSummary(r.Context()))
}

// LogStartup writes the summary to the log, so a report always says what it came
// from, and tells the secrets it holds to the redactor.
func (s *Server) LogStartup(ctx context.Context) {
	s.protectSecrets(ctx)
	sum := s.SystemSummary(ctx)
	sys := sum.System
	s.Log.Info("server started", "version", sum.Version, "go", sum.Go, "os", sys.OS, "arch", sys.Arch, "release", sys.Release,
		"cpu", sys.CPU, "cores", sys.Cores, "memoryMB", sys.MemoryMB, "freeMemoryMB", sys.FreeMB, "gpus", strings.Join(sys.GPUs, "; "), "container", sys.Container)
	var mods []string
	for _, m := range sum.Modules {
		mods = append(mods, m.Name+" "+m.Version)
	}
	s.Log.Info("modules", "cat", "modules", "builtin", strings.Join(mods, ", "))
	s.Log.Info("ffmpeg", "cat", "transcode", "path", sum.FFmpeg, "version", sum.FFmpegInfo, "encoders", strings.Join(sum.Encoders, ","), "maxTranscode", sum.MaxTranscode)
	for _, f := range sum.Folders {
		args := []any{"cat", "storage", "role", f.Role, "path", f.Path, "freeMB", f.Free >> 20, "totalMB", f.Total >> 20}
		lv := slog.LevelInfo
		switch {
		case f.Error != "":
			args = append(args, "err", f.Error)
			lv = slog.LevelWarn
		case f.Total > 0 && f.Free < 1<<30:
			args = append(args, "note", "less than 1 GB free")
			lv = slog.LevelWarn
		}
		s.Log.Log(ctx, lv, "folder", args...)
	}
	s.Log.Info("settings", "values", settingsText(sum.Settings), "secretsSet", strings.Join(sum.SecretsSet, ","), "libraries", libraryCounts(sum.Libraries))
}

func settingsText(m map[string]string) string {
	var b strings.Builder
	for _, k := range summarySettings {
		if v, ok := m[k]; ok {
			b.WriteString(k + ":" + v + " ")
		}
	}
	return strings.TrimSpace(b.String())
}

func libraryCounts(m map[string]int) string {
	var parts []string
	for k, n := range m {
		parts = append(parts, k+":"+strconv.Itoa(n))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// protectSecrets tells the redactor the exact values to hide, wherever they turn
// up: an API key inside an odd address, say. Saving settings calls this again.
func (s *Server) protectSecrets(ctx context.Context) {
	if s.DB == nil {
		return
	}
	for _, k := range secretSettings {
		logging.AddSecret(s.DB.GetSetting(ctx, k, ""))
	}
}
