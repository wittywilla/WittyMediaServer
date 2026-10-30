// Package config resolves the server's launch settings. They come, from
// weakest to strongest, from built-in defaults, a JSON config file
// (wms.json), environment variables and command-line flags.
//
// The config file is looked for in this order:
//
//  1. -config <file> or WMS_CONFIG
//  2. wms.json next to the program: portable mode. Relative paths and the
//     default data folder are then inside the program's folder, so the
//     whole server can live on a USB stick or in one folder.
//  3. the platform's config folder (see UserConfigFile), and on Linux/BSD
//     /etc/wittymediaserver/wms.json for system services.
//
// Relative paths inside a config file are relative to the file itself.
package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type Config struct {
	DataDir      string
	Listen       string
	FFmpegPath   string
	FFprobePath  string
	TranscodeDir string
	CacheDir     string
	ImagesDir    string // downloaded and generated artwork ("metadata media")
	LogDir       string
	LogLevel     string
	PublicURL    string // optional external URL used in generated links
	Dev          bool   // serve the web UI from web/dist on disk instead of the embedded copy
	MaxTranscode int
	WatchStdin   bool // exit when stdin closes (used by the tray launcher)
	OpenBrowser  bool // open the setup page on first start (desktops only)

	ConfigFile string // the file that was loaded ("" = none)
	Portable   bool
}

// File is the JSON config file. Every field is optional.
type File struct {
	Portable      bool   `json:"portable,omitempty"`
	Listen        string `json:"listen,omitempty"`
	DataDir       string `json:"dataDir,omitempty"`
	CacheDir      string `json:"cacheDir,omitempty"`
	MetadataDir   string `json:"metadataDir,omitempty"`
	TranscodeDir  string `json:"transcodeDir,omitempty"`
	LogDir        string `json:"logDir,omitempty"`
	LogLevel      string `json:"logLevel,omitempty"`
	FFmpeg        string `json:"ffmpeg,omitempty"`
	FFprobe       string `json:"ffprobe,omitempty"`
	MaxTranscodes int    `json:"maxTranscodes,omitempty"`
	PublicURL     string `json:"publicUrl,omitempty"`
	OpenBrowser   *bool  `json:"openBrowser,omitempty"`
}

const (
	DefaultPort = 9797
	FileName    = "wms.json"
)

// DefaultDataDir is the per-platform data directory used when nothing else is set.
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

// UserConfigFile is where the config file lives when it is not portable:
// %APPDATA%\WittyMediaServer\wms.json on Windows, ~/Library/Application
// Support/WittyMediaServer/wms.json on macOS, $XDG_CONFIG_HOME (or
// ~/.config)/wittymediaserver/wms.json elsewhere.
func UserConfigFile() string {
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("APPDATA"); v != "" {
			return filepath.Join(v, "WittyMediaServer", FileName)
		}
	case "darwin":
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, "Library", "Application Support", "WittyMediaServer", FileName)
		}
	default:
		if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
			return filepath.Join(v, "wittymediaserver", FileName)
		}
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, ".config", "wittymediaserver", FileName)
		}
	}
	return FileName
}

// ProgramDir is the folder of the running executable.
func ProgramDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return filepath.Dir(exe)
}

// findFile returns the config file to load and whether it makes the server
// portable.
func findFile(explicit string) (string, bool) {
	if explicit != "" {
		return explicit, false
	}
	if v := os.Getenv("WMS_CONFIG"); v != "" {
		return v, false
	}
	if p := filepath.Join(ProgramDir(), FileName); exists(p) {
		return p, true
	}
	if p := UserConfigFile(); exists(p) {
		return p, false
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		if p := "/etc/wittymediaserver/" + FileName; exists(p) {
			return p, false
		}
	}
	return "", false
}

func exists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// LoadFile reads and checks a config file.
func LoadFile(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields() // a typo in a setting name should not be silently ignored
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Parse builds a Config from the config file, the environment and the
// command line arguments.
func Parse(args []string) (*Config, error) {
	// The config file is found first: its values become the flag defaults.
	explicit := ""
	for i, a := range args {
		switch {
		case a == "-config" || a == "--config":
			if i+1 < len(args) {
				explicit = args[i+1]
			}
		case strings.HasPrefix(a, "-config=") || strings.HasPrefix(a, "--config="):
			explicit = a[strings.Index(a, "=")+1:]
		}
	}
	path, portable := findFile(explicit)
	f := &File{}
	if path != "" {
		var err error
		if f, err = LoadFile(path); err != nil {
			return nil, err
		}
		portable = portable || f.Portable
	}
	base := ProgramDir()
	if path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		base = filepath.Dir(path)
	}
	rel := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(base, p)
	}
	or := func(a, b string) string {
		if a != "" {
			return a
		}
		return b
	}

	dataDef := defaultDataDir()
	if portable && os.Getenv("WMS_DATA_DIR") == "" {
		dataDef = filepath.Join(base, "data")
	}
	maxDef := 4
	if f.MaxTranscodes > 0 {
		maxDef = f.MaxTranscodes
	}
	if v, err := strconv.Atoi(os.Getenv("WMS_MAX_TRANSCODES")); err == nil && v > 0 {
		maxDef = v
	}
	openDef := f.OpenBrowser == nil || *f.OpenBrowser

	c := &Config{ConfigFile: path, Portable: portable}
	fs := flag.NewFlagSet("wms", flag.ContinueOnError)
	var configFlag string
	fs.StringVar(&configFlag, "config", "", "config file (WMS_CONFIG); see docs/CONFIG.md")
	fs.StringVar(&c.DataDir, "data", env("WMS_DATA_DIR", or(rel(f.DataDir), dataDef)), "data directory (WMS_DATA_DIR)")
	fs.StringVar(&c.Listen, "listen", env("WMS_LISTEN", or(f.Listen, ":"+strconv.Itoa(DefaultPort))), "listen address (WMS_LISTEN)")
	fs.StringVar(&c.FFmpegPath, "ffmpeg", env("WMS_FFMPEG", rel(f.FFmpeg)), "path to ffmpeg (WMS_FFMPEG); default: bundled or PATH")
	fs.StringVar(&c.FFprobePath, "ffprobe", env("WMS_FFPROBE", rel(f.FFprobe)), "path to ffprobe (WMS_FFPROBE)")
	fs.StringVar(&c.LogLevel, "log", env("WMS_LOG", or(f.LogLevel, "info")), "log level: debug|info|warn|error")
	fs.StringVar(&c.LogDir, "log-dir", env("WMS_LOG_DIR", rel(f.LogDir)), "folder for log files (WMS_LOG_DIR); default <data>/logs")
	fs.StringVar(&c.PublicURL, "public-url", env("WMS_PUBLIC_URL", f.PublicURL), "external base URL")
	fs.BoolVar(&c.Dev, "dev", os.Getenv("WMS_DEV") == "1", "serve web UI from ./web/dist on disk")
	fs.BoolVar(&c.WatchStdin, "watch-stdin", os.Getenv("WMS_WATCH_STDIN") == "1", "shut down when stdin closes (parent process control)")
	fs.BoolVar(&c.OpenBrowser, "open-browser", openDef && os.Getenv("WMS_NO_BROWSER") != "1", "open the setup page in the browser on first start")
	fs.IntVar(&c.MaxTranscode, "max-transcodes", maxDef, "maximum concurrent transcodes")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if c.MaxTranscode < 1 {
		return nil, errors.New("max-transcodes must be at least 1")
	}
	c.TranscodeDir = env("WMS_TRANSCODE_DIR", or(rel(f.TranscodeDir), filepath.Join(c.DataDir, "transcode")))
	c.CacheDir = env("WMS_CACHE_DIR", or(rel(f.CacheDir), filepath.Join(c.DataDir, "cache")))
	c.ImagesDir = env("WMS_METADATA_DIR", or(rel(f.MetadataDir), filepath.Join(c.CacheDir, "images")))
	c.LogDir = or(c.LogDir, filepath.Join(c.DataDir, "logs"))
	for _, d := range []string{c.DataDir, c.TranscodeDir, c.CacheDir, c.ImagesDir, c.LogDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Example is a commented starting point written by `wms config init`.
const Example = `{
  "portable": false,
  "listen": ":9797",
  "dataDir": "",
  "metadataDir": "",
  "cacheDir": "",
  "transcodeDir": "",
  "logDir": "",
  "logLevel": "info",
  "ffmpeg": "",
  "ffprobe": "",
  "maxTranscodes": 4,
  "publicUrl": "",
  "openBrowser": true
}
`
