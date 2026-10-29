// Package config resolves runtime settings from flags, environment and
// platform conventions. Everything the server needs lives under DataDir so a
// single directory can be backed up or mounted as a volume.
package config

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

type Config struct {
	DataDir      string
	Listen       string
	FFmpegPath   string
	FFprobePath  string
	TranscodeDir string
	CacheDir     string
	LogLevel     string
	PublicURL    string // optional external URL used in generated links
	Dev          bool   // serve the web UI from web/dist on disk instead of the embedded copy
	MaxTranscode int
	WatchStdin   bool // exit when stdin closes (used by the tray launcher)
}

const DefaultPort = 9797

// DefaultDataDir is the per-platform data directory used when WMS_DATA_DIR is unset.
func DefaultDataDir() string { return defaultDataDir() }

func defaultDataDir() string {
	if v := os.Getenv("WMS_DATA_DIR"); v != "" {
		return v
	}
	// Containers: a conventional /config volume.
	if st, err := os.Stat("/config"); err == nil && st.IsDir() && runtime.GOOS != "windows" {
		return "/config"
	}
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "WittyMediaServer")
		}
	case "darwin":
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, "Library", "Application Support", "WittyMediaServer")
		}
	default:
		if v := os.Getenv("XDG_DATA_HOME"); v != "" {
			return filepath.Join(v, "wittymediaserver")
		}
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, ".local", "share", "wittymediaserver")
		}
	}
	return "wms-data"
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Parse builds a Config from command line arguments and the environment.
func Parse(args []string) (*Config, error) {
	fs := flag.NewFlagSet("wms", flag.ContinueOnError)
	c := &Config{}
	fs.StringVar(&c.DataDir, "data", defaultDataDir(), "data directory (WMS_DATA_DIR)")
	fs.StringVar(&c.Listen, "listen", env("WMS_LISTEN", ":"+strconv.Itoa(DefaultPort)), "listen address (WMS_LISTEN)")
	fs.StringVar(&c.FFmpegPath, "ffmpeg", env("WMS_FFMPEG", ""), "path to ffmpeg (WMS_FFMPEG); default: bundled or PATH")
	fs.StringVar(&c.FFprobePath, "ffprobe", env("WMS_FFPROBE", ""), "path to ffprobe (WMS_FFPROBE)")
	fs.StringVar(&c.LogLevel, "log", env("WMS_LOG", "info"), "log level: debug|info|warn|error")
	fs.StringVar(&c.PublicURL, "public-url", env("WMS_PUBLIC_URL", ""), "external base URL")
	fs.BoolVar(&c.Dev, "dev", os.Getenv("WMS_DEV") == "1", "serve web UI from ./web/dist on disk")
	fs.BoolVar(&c.WatchStdin, "watch-stdin", os.Getenv("WMS_WATCH_STDIN") == "1", "shut down when stdin closes (parent process control)")
	maxT, _ := strconv.Atoi(env("WMS_MAX_TRANSCODES", "4"))
	fs.IntVar(&c.MaxTranscode, "max-transcodes", maxT, "maximum concurrent transcodes")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	c.TranscodeDir = env("WMS_TRANSCODE_DIR", filepath.Join(c.DataDir, "transcode"))
	c.CacheDir = filepath.Join(c.DataDir, "cache")
	for _, d := range []string{c.DataDir, c.TranscodeDir, c.CacheDir, filepath.Join(c.CacheDir, "images")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return c, nil
}
