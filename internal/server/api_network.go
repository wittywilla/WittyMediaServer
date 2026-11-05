package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/discovery"
)

// Settings → Network and the setup wizard: the port the server answers on. It's saved in
// settings.ini (config.SettingsSaveFile) and used from the next start, so changing it is: check,
// save, restart. Where something stronger sets the port (-listen, WMS_LISTEN, wms.json, Docker),
// the page says where instead of saving somewhere that wouldn't win.

func (s *Server) networkRoutes(m *http.ServeMux) {
	m.Handle("GET /api/network", s.requireAdmin(s.handleNetwork))
	m.Handle("POST /api/network/check", s.requireAdmin(s.handleNetworkCheck))
	m.Handle("PUT /api/network", s.requireAdmin(s.handleNetworkPut))
	m.Handle("POST /api/network/restart", s.requireAdmin(s.handleNetworkRestart))
}

// networkView is the port, where it comes from, and what changing it does.
type networkView struct {
	Port int    `json:"port"` // the one it answers on now
	Host string `json:"host"` // "" for every network card
	// From is where the port came from: flag, env, config, settings or default.
	From string `json:"from"`
	// SettingsFile set the port, when From is settings. SaveFile is where a change goes.
	SettingsFile string `json:"settingsFile,omitempty"`
	SaveFile     string `json:"saveFile"`
	// Locked says why the port can't be changed here, and where it can. "" when it can.
	Locked string `json:"locked,omitempty"`
	Docker bool   `json:"docker"`
	// Next is the port the next start answers on: Port, unless a change is waiting for a restart.
	Next       int  `json:"next"`
	CanRestart bool `json:"canRestart"`
	// Problems are lines of the settings files that were skipped.
	Problems  []string `json:"problems"`
	Addresses []string `json:"addresses"` // on the home network, now
	// DiscoveryPort is the UDP port apps ask on, which stays put whatever the port is.
	DiscoveryPort int `json:"discoveryPort"`
	// HTTPSPort is Remote Access's port, which this one can't be, while it's on.
	HTTPSPort int `json:"httpsPort,omitempty"`
}

// portCheck is what checking a port found.
type portCheck struct {
	Port int  `json:"port"`
	OK   bool `json:"ok"` // it can be saved
	// Problems say why it can't, Warnings what to know first.
	Problems []string `json:"problems"`
	Warnings []string `json:"warnings"`
	Current  bool     `json:"current"` // it's the one in use now
}

func inDocker() bool {
	if os.Getenv("WMS_DOCKER") == "1" {
		return true
	}
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

func (s *Server) runningPort() (string, int) {
	host, p, err := net.SplitHostPort(s.Cfg.Listen)
	if err != nil {
		return "", 0
	}
	n, _ := strconv.Atoi(p)
	return host, n
}

// nextPort is the port the next start uses, read again from the files.
func (s *Server) nextPort(running int) int {
	switch s.Cfg.ListenFrom {
	case config.FromSettings, config.FromDefault, "":
	default:
		return running
	}
	for _, f := range config.SettingsFiles(s.Cfg.DataDir) {
		if st, err := config.ReadSettings(f); err == nil && st.Port != 0 {
			return st.Port
		}
	}
	return config.DefaultPort
}

// lockedWhy says where the port is set when it's somewhere this page can't change.
func (s *Server) lockedWhy() string {
	if inDocker() {
		return "In Docker, the port is the one published in docker-compose.yml, under ports: change the first number of \"9797:9797\" and run docker compose up -d. The port inside the container stays 9797."
	}
	switch s.Cfg.ListenFrom {
	case config.FromFlag:
		return "The server was started with -listen " + s.Cfg.Listen + ", which wins over this setting. Remove -listen where it's started to choose the port here."
	case config.FromEnv:
		return "WMS_LISTEN=" + s.Cfg.Listen + " is set where the server starts, which wins over this setting. Remove it to choose the port here: on Linux with sudo systemctl edit wittymediaserver, on FreeBSD with sysrc -x wittymediaserver_listen."
	case config.FromConfig:
		return "\"listen\" is set to " + s.Cfg.Listen + " in " + s.Cfg.ConfigFile + ", which wins over this setting. Remove that line to choose the port here, or change it there."
	}
	return ""
}

func (s *Server) networkView() networkView {
	host, port := s.runningPort()
	v := networkView{Port: port, Host: host, From: s.Cfg.ListenFrom, SettingsFile: s.Cfg.SettingsFile, SaveFile: config.SettingsSaveFile(s.Cfg.DataDir),
		Locked: s.lockedWhy(), Docker: inDocker(), Next: s.nextPort(port), CanRestart: s.Restart != nil, Problems: s.Cfg.SettingsProblems,
		Addresses: s.homeURLs(), DiscoveryPort: discovery.UDPPort}
	if v.From == "" {
		v.From = config.FromDefault
	}
	if v.Problems == nil {
		v.Problems = []string{}
	}
	if st := s.state(); st.on && st.cfg.HTTPSMode != "off" {
		v.HTTPSPort = st.cfg.HTTPSPort
	}
	return v
}

func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.networkView())
}

// browserBlocked are the ports browsers refuse to open, whatever answers there: the Fetch
// standard's list (https://fetch.spec.whatwg.org/#port-blocking). A server there would lose its
// own web app.
var browserBlocked = []int{1, 7, 9, 11, 13, 15, 17, 19, 20, 21, 22, 23, 25, 37, 42, 43, 53, 69, 77, 79, 87, 95, 101, 102, 103, 104,
	109, 110, 111, 113, 115, 117, 119, 123, 135, 137, 139, 143, 161, 179, 389, 427, 465, 512, 513, 514, 515, 526, 530, 531, 532,
	540, 548, 554, 556, 563, 587, 601, 636, 989, 990, 993, 995, 1719, 1720, 1723, 2049, 3659, 4045, 4190, 5060, 5061, 6000, 6566,
	6665, 6666, 6667, 6668, 6669, 6679, 6697, 10080}

// checkPort says whether the server could move to a port: in range, not one browsers refuse,
// not Remote Access's, and free on this computer, tried the way the server would listen.
func (s *Server) checkPort(port int) portCheck {
	c := portCheck{Port: port, Problems: []string{}, Warnings: []string{}}
	host, running := s.runningPort()
	switch {
	case port < config.MinPort || port > config.MaxPort:
		c.Problems = append(c.Problems, fmt.Sprintf("A port is a number from %d to %d.", config.MinPort, config.MaxPort))
		return c
	case port == running:
		c.OK, c.Current = true, true
		return c
	}
	if slices.Contains(browserBlocked, port) {
		c.Problems = append(c.Problems, fmt.Sprintf("Browsers refuse to open port %d, as it belongs to another kind of service, so this page couldn't open any more. Choose another.", port))
	}
	if st := s.state(); st.cfg.HTTPSPort == port {
		if st.on && st.cfg.HTTPSMode != "off" {
			c.Problems = append(c.Problems, fmt.Sprintf("Port %d is Remote Access's HTTPS port. Choose another, or change that one in Settings → Remote Access first.", port))
		} else {
			c.Warnings = append(c.Warnings, fmt.Sprintf("Port %d is the HTTPS port Remote Access would use. If you turn its HTTPS on later, choose another there.", port))
		}
	}
	if port < 1024 {
		c.Warnings = append(c.Warnings, "Ports below 1024 are usually kept for the system's own services, and on Linux and BSD a server needs extra rights to use them. 8096 to 9999 is a good range for a media server.")
	}
	if len(c.Problems) == 0 {
		if why := portBusy(host, port); why != "" {
			c.Problems = append(c.Problems, why)
		}
	}
	c.OK = len(c.Problems) == 0
	return c
}

// portBusy tries listening on a port, the way the server would, and says why it can't. On
// Windows a socket on every network card can share a port with one on 127.0.0.1, so for every
// network card both are tried.
func portBusy(host string, port int) string {
	addrs := []string{net.JoinHostPort(host, strconv.Itoa(port))}
	if host == "" || host == "0.0.0.0" || host == "::" {
		addrs = append(addrs, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	}
	for _, a := range addrs {
		ln, err := net.Listen("tcp", a)
		if err != nil {
			switch {
			case errors.Is(err, syscall.EADDRINUSE) || isAddrInUse(err):
				return fmt.Sprintf("Something else on this computer is using port %d. Choose another, or stop that program first.", port)
			case errors.Is(err, syscall.EACCES) || isAccessDenied(err):
				if port < 1024 {
					return fmt.Sprintf("This computer doesn't let the server use port %d: ports below 1024 need extra rights. Choose one from 1024 up.", port)
				}
				return fmt.Sprintf("This computer doesn't let the server use port %d. Windows keeps some ranges for itself; choose another.", port)
			}
			return fmt.Sprintf("The server couldn't use port %d: %v", port, err)
		}
		_ = ln.Close()
	}
	return ""
}

func readPort(r *http.Request) (int, error) {
	var b struct {
		Port int `json:"port"`
	}
	if err := readJSON(r, &b); err != nil {
		return 0, errors.New("expected {\"port\": <number>}")
	}
	return b.Port, nil
}

// POST /api/network/check {port}: whether the server could move there. Nothing is saved.
func (s *Server) handleNetworkCheck(w http.ResponseWriter, r *http.Request) {
	port, err := readPort(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, s.checkPort(port))
}

// PUT /api/network {port}: saves the port for the next start, after the same checks.
func (s *Server) handleNetworkPut(w http.ResponseWriter, r *http.Request) {
	port, err := readPort(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if why := s.lockedWhy(); why != "" {
		writeErr(w, 409, why)
		return
	}
	c := s.checkPort(port)
	if !c.OK {
		writeErr(w, 400, c.Problems[0])
		return
	}
	file := config.SettingsSaveFile(s.Cfg.DataDir)
	if err := config.SavePort(file, port); err != nil {
		writeErr(w, 500, "couldn't save "+file+": "+err.Error())
		return
	}
	s.Log.WarnContext(r.Context(), "the port was changed, for the next start", "cat", "server", "port", port, "file", file, "by", userFrom(r).Name)
	writeJSON(w, 200, s.networkView())
}

// POST /api/network/restart: starts the server again, on the saved port.
func (s *Server) handleNetworkRestart(w http.ResponseWriter, r *http.Request) {
	if s.Restart == nil {
		writeErr(w, 409, "this server can't restart itself: restart it the way it was started")
		return
	}
	s.Log.InfoContext(r.Context(), "restarting, for the port", "cat", "server", "by", userFrom(r).Name)
	writeJSON(w, 202, map[string]any{"restarting": true})
	time.AfterFunc(300*time.Millisecond, s.Restart) // so the page has its answer first
}

// Windows' own codes for the same two errors: WSAEADDRINUSE and WSAEACCES.
func isAddrInUse(err error) bool    { return isErrno(err, 10048) }
func isAccessDenied(err error) bool { return isErrno(err, 10013) }

func isErrno(err error, code syscall.Errno) bool {
	var en syscall.Errno
	return runtime.GOOS == "windows" && errors.As(err, &en) && en == code
}

// plainHost is a host name or IPv4 address that can go in a policy as it is.
var plainHost = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

// appCSPFor is appCSP, also letting the page ask whether the server answers on another port of
// the same host: how Settings → Network follows it to its new port after a restart. Only that
// one address, /api/health, and only for a plain host name or IPv4 address.
func appCSPFor(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if !plainHost.MatchString(host) {
		return appCSP
	}
	return strings.Replace(appCSP, "connect-src 'self'", "connect-src 'self' http://"+host+":*/api/health", 1)
}
