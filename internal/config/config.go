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
//     /etc/wittymediaserver/wms.json for system services, or on FreeBSD
//     /usr/local/etc/wittymediaserver/wms.json.
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

	"github.com/wittywilla/wittymediaserver/internal/layout"
)

// How many connections can be open at once. A household is nowhere near this: a browser opens
// 6 per server at most, plus a socket per tab, and that many is the fewest that makes sense.
const (
	DefaultConnections = 2048
	minConnections     = 16
)

type Config struct {
	DataDir      string
	Listen       string
	FFmpegPath   string
	FFprobePath  string
	JavaPath     string // the Java for Blu-ray menus; "" finds it. Never a web setting: see handleBDJStatus
	TranscodeDir string
	CacheDir     string
	ImagesDir    string // downloaded and generated artwork ("metadata media")
	LogDir       string
	LogLevel     string
	PublicURL    string // optional external URL used in generated links
	// RemoteAccess turns remote access on regardless of Settings, for servers
	// nobody can reach from a home network first (a rented server).
	RemoteAccess bool
	// SetupFromHome lets the first account be made from the home network without the
	// setup code. Off, only the server's own desktop goes without it.
	SetupFromHome bool
	// AllowNetworkPaths lets settings from another device name network shares and device paths.
	// Reaching one signs in there with the server's own account.
	AllowNetworkPaths bool
	// GamingCustomCores lets custom game cores be turned on in Settings. It is set only on the
	// machine: a core is a program, and a web session must not be able to decide to run one.
	GamingCustomCores bool
	TrustedProxies    string // reverse proxies whose forwarded headers are believed, added to those in Settings
	Dev               bool   // serve the web UI from web/dist on disk instead of the embedded copy
	MaxTranscode      int
	// MaxConnections is how many connections can be open at once, on both ports together.
	MaxConnections int
	WatchStdin     bool // exit when stdin closes (used by the tray launcher)
	OpenBrowser    bool // open the setup page on first start (desktops only)

	ConfigFile string // the file that was loaded ("" = none)
	Portable   bool
}

// File is the JSON config file. Every field is optional.
type File struct {
	Portable          bool   `json:"portable,omitempty"`
	Listen            string `json:"listen,omitempty"`
	DataDir           string `json:"dataDir,omitempty"`
	CacheDir          string `json:"cacheDir,omitempty"`
	MetadataDir       string `json:"metadataDir,omitempty"`
	TranscodeDir      string `json:"transcodeDir,omitempty"`
	LogDir            string `json:"logDir,omitempty"`
	LogLevel          string `json:"logLevel,omitempty"`
	FFmpeg            string `json:"ffmpeg,omitempty"`
	FFprobe           string `json:"ffprobe,omitempty"`
	JavaPath          string `json:"javaPath,omitempty"`
	MaxTranscodes     int    `json:"maxTranscodes,omitempty"`
	MaxConnections    int    `json:"maxConnections,omitempty"`
	PublicURL         string `json:"publicUrl,omitempty"`
	RemoteAccess      bool   `json:"remoteAccess,omitempty"`
	SetupFromHome     bool   `json:"setupFromHome,omitempty"`
	AllowNetworkPaths bool   `json:"allowNetworkPaths,omitempty"`
	GamingCustomCores bool   `json:"gamingCustomCores,omitempty"`
	TrustedProxies    string `json:"trustedProxies,omitempty"`
	OpenBrowser       *bool  `json:"openBrowser,omitempty"`
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
	// Root using the command line of a packaged server means the service's library, not one of its own.
	if d := serviceDataDir(); d != "" {
		return d
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

// serviceDataDir is where a package's service keeps its data, for root's command line. "" when
// this isn't root, or the service's folder isn't there.
func serviceDataDir() string {
	if os.Geteuid() != 0 {
		return ""
	}
	d := ""
	switch layout.Current().Kind {
	case layout.KindDeb, layout.KindRPM:
		d = "/var/lib/wittymediaserver"
	case layout.KindFreeBSD:
		d = "/var/db/wittymediaserver"
	}
	if d == "" {
		return ""
	}
	if st, err := os.Stat(d); err != nil || !st.IsDir() {
		return ""
	}
	return d
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

// ProgramDir is the program's top folder: where wms.json and a portable data folder
// are, whichever program of the package is asking.
func ProgramDir() string { return layout.Current().Root }

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
	if runtime.GOOS == "freebsd" {
		if p := "/usr/local/etc/wittymediaserver/" + FileName; exists(p) {
			return p, false
		}
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
	c, err := ParseOnly(args)
	if err != nil {
		return nil, err
	}
	if err := c.MakeFolders(); err != nil {
		return nil, err
	}
	return c, nil
}

// MakeFolders makes the folders the server keeps things in, those that are missing kept
// private. Parse does it; so does the start of a server after a reset or a restore has
// moved them all away (issue #66), and nothing it writes then should need a restart.
func (c *Config) MakeFolders() error {
	for _, d := range []string{c.DataDir, c.TranscodeDir, c.CacheDir, c.ImagesDir, c.LogDir} {
		_, missing := os.Stat(d)
		if err := os.MkdirAll(d, 0o750); err != nil {
			return err
		}
		if os.IsNotExist(missing) {
			if err := makePrivate(d); err != nil {
				return err
			}
		}
	}
	return nil
}

// ParseOnly is Parse with nothing made on disk, for commands that only look, or that have
// something to check first.
func ParseOnly(args []string) (*Config, error) {
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
	connDef := DefaultConnections
	if f.MaxConnections != 0 {
		connDef = f.MaxConnections
	}
	if v, err := strconv.Atoi(os.Getenv("WMS_MAX_CONNECTIONS")); err == nil && v != 0 {
		connDef = v
	}
	openDef := f.OpenBrowser == nil || *f.OpenBrowser

	c := &Config{ConfigFile: path, Portable: portable}
	fs := flag.NewFlagSet(layout.ServerCommand, flag.ContinueOnError)
	var configFlag string
	fs.StringVar(&configFlag, "config", "", "config file (WMS_CONFIG); see Help in the app, Config file and folders")
	fs.StringVar(&c.DataDir, "data", env("WMS_DATA_DIR", or(rel(f.DataDir), dataDef)), "data directory (WMS_DATA_DIR)")
	fs.StringVar(&c.Listen, "listen", env("WMS_LISTEN", or(f.Listen, ":"+strconv.Itoa(DefaultPort))), "listen address (WMS_LISTEN)")
	fs.StringVar(&c.FFmpegPath, "ffmpeg", env("WMS_FFMPEG", rel(f.FFmpeg)), "path to ffmpeg (WMS_FFMPEG); default: bundled or PATH")
	fs.StringVar(&c.FFprobePath, "ffprobe", env("WMS_FFPROBE", rel(f.FFprobe)), "path to ffprobe (WMS_FFPROBE)")
	fs.StringVar(&c.JavaPath, "java", env("WMS_JAVA", rel(f.JavaPath)), "Java folder or program for Blu-ray menus (WMS_JAVA); default: found by itself")
	fs.StringVar(&c.LogLevel, "log", env("WMS_LOG", or(f.LogLevel, "info")), "log level: debug|info|warn|error")
	fs.StringVar(&c.LogDir, "log-dir", env("WMS_LOG_DIR", rel(f.LogDir)), "folder for log files (WMS_LOG_DIR); default <data>/logs")
	fs.StringVar(&c.PublicURL, "public-url", env("WMS_PUBLIC_URL", f.PublicURL), "external base URL")
	fs.BoolVar(&c.GamingCustomCores, "gaming-custom-cores", f.GamingCustomCores || os.Getenv("WMS_GAMING_CUSTOM_CORES") == "1", "let custom game cores be turned on in Settings (WMS_GAMING_CUSTOM_CORES=1)")
	fs.BoolVar(&c.SetupFromHome, "setup-from-home", f.SetupFromHome || os.Getenv("WMS_SETUP_FROM_HOME") == "1", "let anyone on the home network make the first account, without the setup code (WMS_SETUP_FROM_HOME=1)")
	fs.BoolVar(&c.AllowNetworkPaths, "allow-network-paths", f.AllowNetworkPaths || os.Getenv("WMS_ALLOW_NETWORK_PATHS") == "1", "let settings changed from another device name network shares, such as \\\\nas\\media (WMS_ALLOW_NETWORK_PATHS=1)")
	fs.BoolVar(&c.RemoteAccess, "remote-access", f.RemoteAccess || os.Getenv("WMS_REMOTE_ACCESS") == "1", "allow connections from outside the home network (WMS_REMOTE_ACCESS=1); normally set in Settings")
	fs.StringVar(&c.TrustedProxies, "trusted-proxies", env("WMS_TRUSTED_PROXIES", f.TrustedProxies), "reverse proxies whose X-Forwarded-For is believed: addresses or networks, comma separated (WMS_TRUSTED_PROXIES)")
	fs.BoolVar(&c.Dev, "dev", os.Getenv("WMS_DEV") == "1", "serve web UI from ./web/dist on disk")
	fs.BoolVar(&c.WatchStdin, "watch-stdin", os.Getenv("WMS_WATCH_STDIN") == "1", "shut down when stdin closes (parent process control)")
	fs.BoolVar(&c.OpenBrowser, "open-browser", openDef && os.Getenv("WMS_NO_BROWSER") != "1", "open the setup page in the browser on first start")
	fs.IntVar(&c.MaxTranscode, "max-transcodes", maxDef, "maximum concurrent transcodes")
	fs.IntVar(&c.MaxConnections, "max-connections", connDef, "maximum connections open at once, for the whole server (WMS_MAX_CONNECTIONS)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if c.MaxTranscode < 1 {
		return nil, errors.New("max-transcodes must be at least 1")
	}
	if c.MaxConnections < minConnections {
		return nil, fmt.Errorf("max-connections must be at least %d: a single browser can use that many", minConnections)
	}
	c.TranscodeDir = env("WMS_TRANSCODE_DIR", or(rel(f.TranscodeDir), filepath.Join(c.DataDir, "transcode")))
	c.CacheDir = env("WMS_CACHE_DIR", or(rel(f.CacheDir), filepath.Join(c.DataDir, "cache")))
	c.ImagesDir = env("WMS_METADATA_DIR", or(rel(f.MetadataDir), filepath.Join(c.CacheDir, "images")))
	c.LogDir = or(c.LogDir, filepath.Join(c.DataDir, "logs"))
	return c, nil
}

// Example is a starting point written by `wittymediaserver config init`.
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
  "remoteAccess": false,
  "trustedProxies": "",
  "openBrowser": true
}
`
