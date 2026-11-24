// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/appcontrol"
	"github.com/wittywilla/wittymediaserver/internal/brandname"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// uiLanguagePattern: the language of the app's own pages, "de" or "pt-PT".
var uiLanguagePattern = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)

var settingKeys = []string{"tmdbKey", "tvdbKey", "tvdbPin", "guideKey", "hwAccel", "maxHeight", "menuLanguage", "audioLanguage", "subtitleLanguage", "uiLanguage",
	"serverName", "audioCodec", "lowPower", "bdjEnabled", "dvdRegion", "blurayRegion", "dvdParental", "blurayAge", "discCountry", "saveMetadata", "saveWittyinfo", "dvdStart", "insertFx", "episodeView",
	"hwEncoders", "gpuPathsOff", "directPlay", "qualityMode", "qualityProfile", "x264Preset", "crf", "maxBitrate", "audioBitrate",
	"ffmpegVideoArgs", "ffmpegOutputArgs", "timeZone", "theme", "brandName", "brandColouring", "brandSpans", "brandMark", "maxSpeed",
	"loginStyle", "profilesRemote", "setupDone", "kidsManagedBy", "insertSound", "tvmaze", "wikidata", "musicbrainz", "animeIn", "animeTitles",
	"libraryGrouping", "updateCheck", "moduleUpdateCheck", "moduleAutoInstall", "uploads", "uploadMaxMb", "uploadAccountMb", "shareLinks", "lanDiscovery", "radioDirectory", "radioChecks",
	"legalOwnerNotice", "legalOwnerContact", "noPoster", "pairRequirePin", settingSecondOpinion}

// settingDefaults are returned when a key has never been saved.
var settingDefaults = map[string]string{"saveMetadata": "1", "saveWittyinfo": "1", "brandMark": brandname.DefaultMark, "noPoster": library.NoPosterFrame, "dvdStart": "menu", "insertFx": "1", "episodeView": "cards",
	"theme": "default", "brandColouring": brandname.Auto, "directPlay": "1", "hwEncoders": "auto", "qualityMode": "easy", "qualityProfile": "balanced", "x264Preset": "veryfast", "crf": "21", "maxSpeed": "4",
	"loginStyle": "profiles", "profilesRemote": "0", "kidsManagedBy": "adults", "insertSound": "1", "tvmaze": "1", "wikidata": "1", "musicbrainz": "1", "animeIn": "both", "animeTitles": "english",
	// libraryGrouping: another folder of a kind joins that kind's library
	// ("combine"), or becomes a library of its own ("separate").
	"libraryGrouping": "combine",
	// updateCheck: look for new releases on GitHub twice a day.
	"updateCheck": "1",
	// moduleAutoInstall: install compatible module updates without asking. Off
	// unless the owner turns it on. moduleUpdateCheck has no default on purpose:
	// until it is set, it follows updateCheck.
	"moduleAutoInstall": "0",
	// uploads and shareLinks: adults may add photos from their devices, and make
	// links for people without an account. (The photo map's online map is WittyPhotos'
	// setting.)
	"uploads": "1", "uploadMaxMb": "4096", "uploadAccountMb": "102400", "shareLinks": "1",
	// lanDiscovery: apps on the home network can find this server.
	"lanDiscovery": "1",
	// radioDirectory: search the free station directory, and refresh details of
	// stations that came from it. radioChecks: try stations in the background, which
	// contacts them when nobody is listening, so it's off unless the owner wants it.
	"radioDirectory": "1", "radioChecks": "0"}

// settingChecks: settings handed to FFmpeg or used as numbers. A wrong value
// would otherwise only show up as every transcode failing.
var settingChecks = map[string]func(string) bool{
	"timeZone":          validTimeZone,
	"crf":               intBetween(0, 51),
	"maxHeight":         intBetween(0, 4320),
	"maxSpeed":          floatBetween(1, 16),
	"gpuPathsOff":       gpuPathList,
	"x264Preset":        oneOf("ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow"),
	"qualityMode":       oneOf("easy", "advanced", "expert"),
	"qualityProfile":    oneOf("speed", "balanced", "quality"),
	"audioCodec":        oneOf("aac", "mp3"),
	"maxBitrate":        bitrateRe.MatchString,
	"audioBitrate":      bitrateRe.MatchString,
	"moduleUpdateCheck": oneOf("", "0", "1"),
	"moduleAutoInstall": oneOf("0", "1"),
	"uploadAccountMb":   intBetween(0, 1<<30),
}

var bitrateRe = regexp.MustCompile(`^[0-9]{1,6}(\.[0-9]{1,3})?[kKmM]?$`)

func intBetween(lo, hi int) func(string) bool {
	return func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= lo && n <= hi
	}
}

func floatBetween(lo, hi float64) func(string) bool {
	return func(v string) bool {
		f, err := strconv.ParseFloat(v, 64)
		return err == nil && f >= lo && f <= hi
	}
}

func oneOf(list ...string) func(string) bool {
	return func(v string) bool { return slices.Contains(list, v) }
}

// secretSettings are masked when read back.
var secretSettings = []string{"tmdbKey", "tvdbKey", "tvdbPin", "guideKey"}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	out, status := s.serverSettingsView(r)
	if status != 0 {
		writeErr(w, status, "could not read the settings")
		return
	}
	writeJSON(w, 200, out)
}

// serverSettingsView is every setting, saved or default, with secrets masked; status is non-zero when
// the settings can't be read.
func (s *Server) serverSettingsView(r *http.Request) (map[string]string, int) {
	saved, err := s.DB.Settings(r.Context()) // one statement, not one for each of the hundred
	if err != nil {
		return nil, 500
	}
	out := make(map[string]string, len(settingKeys))
	for _, k := range settingKeys {
		v, ok := saved[k]
		if !ok {
			v = settingDefaults[k]
		}
		out[k] = v
	}
	out["theme"] = s.serverTheme(r.Context()) // the look's own until one is chosen
	out["timeZoneNow"] = time.Local.String()  // the zone the server runs in, which is timeZone's, else the computer's
	out["timeZoneComputer"] = computerZone
	if len(s.wittyTVLookPending(r.Context())) > 0 {
		out[settingWittyTVLookAsk] = "1" // WittyTuner was just installed: the guide's look is to be asked (wittytv_look.go)
	}
	if s.timeZoneWaiting(r.Context()) {
		out["timeZoneRestart"] = "1" // saved, in use once the server starts again
	}
	for _, k := range secretSettings {
		if out[k] != "" {
			out[k+"Set"] = "1"
			out[k] = strings.Repeat("•", 8)
		}
	}
	return out, 0
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	for k, v := range in {
		if check := settingChecks[k]; check != nil && v != "" && !check(strings.TrimSpace(v)) {
			writeErr(w, 400, "that value is not valid for "+k)
			return
		}
	}
	// Words chosen for one name don't fit another: a new name sent without its
	// colouring (by an older app) goes back to the automatic split.
	if n, ok := in["brandName"]; ok && in["brandColouring"] == "" && strings.TrimSpace(n) != s.DB.GetSetting(r.Context(), "brandName", "") &&
		strings.HasPrefix(s.DB.GetSetting(r.Context(), "brandColouring", ""), brandname.Pick) {
		in["brandColouring"] = brandname.Auto
	}
	// Everything is checked before anything is saved: a refused value mustn't leave
	// half a change behind (a new name kept with words chosen for the old one).
	for _, k := range settingKeys {
		if v, ok := in[k]; ok && !strings.Contains(v, "•") {
			if err := checkSetting(k, strings.TrimSpace(v)); err != nil {
				writeErr(w, 400, err.Error())
				return
			}
		}
	}
	// Choosing the theme the server already wears (the look's own) doesn't pin it: the server's
	// theme follows the look until the administrator picks a different one. "" goes back to
	// following the look. PUT /api/theme/global {id} pins any theme, this one included.
	if v, ok := in["theme"]; ok && !s.themeChosen(r.Context()) && strings.TrimSpace(v) == s.serverTheme(r.Context()) {
		delete(in, "theme")
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
		if err := s.DB.SetSetting(r.Context(), k, v); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if v, ok := in["gpuPathsOff"]; ok {
		ffmpeg.SetGPUPathsOff(strings.Split(v, ","))
	}
	if _, ok := in["timeZone"]; ok {
		s.restartForTimeZone(r.Context(), adminName(r)) // time.Local isn't changed while the server runs
	}
	s.protectSecrets(r.Context()) // a new API key is hidden from the log from now on
	if s.Discovery != nil {
		s.Discovery.Refresh() // it may have been switched off, or the server renamed
	}
	s.writeSettings(w, r, brandChanged(in))
}

// brandChanged: the request set the server's name, how it is coloured or its badge.
func brandChanged(in map[string]string) bool {
	for _, k := range []string{"brandName", "brandColouring", "brandSpans", "brandMark"} {
		if _, ok := in[k]; ok {
			return true
		}
	}
	return false
}

// writeSettings answers a settings change. After a change to the server's name it also says
// where a fixed colour of the name is under 4.5:1 on the surfaces it sits on, in the server's
// theme: brandWarnings is [{text, surface, where, ratio, need, variant?}], empty when it reads
// everywhere, for the name editor and apps to show as "Check:" notes.
func (s *Server) writeSettings(w http.ResponseWriter, r *http.Request, brand bool) {
	if !brand {
		s.handleGetSettings(w, r)
		return
	}
	out, status := s.serverSettingsView(r)
	if status != 0 {
		writeErr(w, status, "could not read the settings")
		return
	}
	res := map[string]any{}
	for k, v := range out {
		res[k] = v
	}
	warn := []BrandWarning{}
	if st, err := s.uiStore(); err == nil {
		t := s.resolveThemeWith(r, st.Current(), s.moduleSet(nil), themeChoices{})
		warn = t.Brand.Warnings
	}
	res["brandWarnings"] = warn
	writeJSON(w, 200, res)
}

// checkSetting refuses values that would reach FFmpeg's command line or build
// file names unless they are what the setting is meant to hold. An account
// that can change settings must not get the machine's file system with them.
func checkSetting(k, v string) error {
	switch k {
	case "legalOwnerNotice", "legalOwnerContact":
		return checkLegalSetting(k, v)
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
	case "noPoster":
		if v != library.NoPosterFrame && v != library.NoPosterTitle && v != library.NoPosterNone {
			return errors.New("no poster must be frame, title or none")
		}
	case "audioCodec":
		if v != "" && v != "aac" && v != "mp3" {
			return errors.New("the audio codec must be aac or mp3")
		}
	case "serverName", "brandName":
		if len([]rune(v)) > brandname.MaxRunes {
			return errors.New("the name must be at most 80 characters")
		}
	case "brandSpans":
		if len(v) > 4096 {
			return errors.New("the name's colours are too long")
		}
		if _, err := brandname.ParseSpans(v, brandname.MaxRunes); err != nil {
			return err
		}
	case "brandMark":
		if _, err := brandname.ParseMark(v); err != nil {
			return err
		}
	case "uiLanguage":
		// The language of the app's own pages: a two-letter code, with a region where the app has one (pt-BR,
		// pt-PT), or empty for each browser's own.
		if v != "" && !uiLanguagePattern.MatchString(v) {
			return errors.New("the language must be a code such as en, de or pt-PT")
		}
	case "brandColouring":
		if !brandname.Valid(v) {
			return errors.New(`the colouring is "auto", "one", or "pick:" and ranges such as 5-10`)
		}
	}
	return nil
}

// gpuPathList checks gpuPathsOff: GPU path IDs, separated by commas.
func gpuPathList(v string) bool {
	for _, id := range strings.Split(v, ",") {
		if id = strings.TrimSpace(id); id != "" && !slices.ContainsFunc(ffmpeg.GPUPathList, func(p ffmpeg.GPUPath) bool { return p.ID == id }) {
			return false
		}
	}
	return true
}

// gpuInUse says what a new stream would use of the graphics chip with the settings as they are:
// the encoder, and the paths that work for it. A stream uses less when its file needs it (a
// codec the chip doesn't decode) or after a failure, which the log says.
func (s *Server) gpuInUse(r *http.Request) map[string]string {
	get := func(k, def string) string { return s.DB.GetSetting(r.Context(), k, def) }
	enc := s.Tools.Plan(ffmpeg.Quality{HW: hwAllowed(get)}).Name
	out := map[string]string{"encoder": enc}
	if p, ok := ffmpeg.GPUPipeFor(enc, "h264", "yuv420p"); ok && ffmpeg.GPUPathWorks(p.ID()) {
		out["pipeline"] = p.ID()
	}
	if ffmpeg.GPUPathWorks("tonemap:libplacebo") && enc != "h264_vaapi" {
		out["toneMap"] = "tonemap:libplacebo"
	}
	return out
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	s.Tools.StartHardwareProbe() // Settings is asking: it shows which ones work
	hwList, hwDone := s.Tools.HardwareStatus()
	var encoders []string
	for _, e := range []string{"libx264", "libopenh264", "h264_nvenc", "h264_qsv", "h264_amf", "h264_vaapi", "h264_videotoolbox"} {
		if s.Tools.Encoders[e] {
			encoders = append(encoders, e)
		}
	}
	lanUrls, lanGuess, localOnly := s.homeAddresses(r)
	writeJSON(w, 200, map[string]any{
		"lanUrls":     lanUrls,
		"lanGuess":    lanGuess,  // the home-network address, even while it only listens on this computer
		"localOnly":   localOnly, // listens on this computer only
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
		"gpuPaths":    s.Tools.GPUPaths(),
		"gpuInUse":    s.gpuInUse(r),
		"gpus":        ffmpeg.GPUs(),
		"container":   ffmpeg.InContainer(),
		"dvdSessions": s.DVD.Count(),
		"blurays":     s.BD.Count(),
		// Windows only: whether Smart App Control is on, in words.
		"smartAppControl": appcontrol.Words(appcontrol.State()),
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
		// Scripts and stylesheets are kept compressed only (web/vite.config.ts).
		if strings.HasPrefix(p, "assets/") && s.serveCompressedOnly(w, r, p) {
			return
		}
		if f, err := s.Web.Open(p); err == nil {
			st, _ := f.Stat()
			f.Close()
			if st != nil && !st.IsDir() {
				if ct := webTypes[path.Ext(p)]; ct != "" {
					w.Header().Set("Content-Type", ct)
				}
				if !strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "no-cache") // the page names the assets of this version
					// The embedded files have no modification time, so without a tag the browser
					// had nothing to ask again with, and fetched each font (350 KB) on every
					// visit (#643). With one, a visit after the first gets 304 Not Modified.
					if tag := s.webTag(p, st); tag != "" {
						w.Header().Set("ETag", tag)
					}
				} else {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					if s.serveGzipped(w, r, p) {
						return
					}
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
			// A build without the client (web/dist holds only .gitkeep) still runs.
			http.Error(w, "Web UI not built — run npm run build in web/", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(b)
	})
}

// webTypes are the types of the client's files, whatever this machine says:
// a Windows registry entry can make .js "text/plain", and browsers then refuse
// to run it (the server sends nosniff) and show a blank page.
var webTypes = map[string]string{
	".js": "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8",
	".html": "text/html; charset=utf-8", ".svg": "image/svg+xml", ".json": "application/json", ".webmanifest": "application/manifest+json",
	".png": "image/png", ".wasm": "application/wasm",
}

// serveGzipped sends the compressed copy made at build time (web/vite.config.ts),
// to browsers that take it. The main script goes from 1.2 MB to 380 KB, which a
// phone away from home notices, at no cost to the server's processor.
func (s *Server) serveGzipped(w http.ResponseWriter, r *http.Request, name string) bool {
	if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
		return false
	}
	f, err := s.Web.Open(name + ".gz")
	if err != nil {
		return false
	}
	defer f.Close()
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		return false
	}
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Vary", "Accept-Encoding")
	http.ServeContent(w, r, "", time.Time{}, rs)
	return true
}

// serveCompressedOnly answers for a file that has only its compressed copy, unpacking it for
// the rare browser that doesn't take that. False when the file isn't one of those.
func (s *Server) serveCompressedOnly(w http.ResponseWriter, r *http.Request, name string) bool {
	if f, err := s.Web.Open(name); err == nil {
		f.Close()
		return false
	}
	f, err := s.Web.Open(name + ".gz")
	if err != nil {
		return false
	}
	defer f.Close()
	if ct := webTypes[path.Ext(name)]; ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if s.serveGzipped(w, r, name) {
		return true
	}
	w.Header().Set("Vary", "Accept-Encoding")
	zr, err := gzip.NewReader(f)
	if err != nil {
		http.Error(w, "this file is damaged", 500)
		return true
	}
	defer zr.Close()
	_, _ = io.Copy(w, zr)
	return true
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		enc, q, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(enc), "gzip") {
			return strings.ReplaceAll(strings.TrimSpace(q), " ", "") != "q=0"
		}
	}
	return false
}

// lanURLs are the addresses other devices on the home network can open (this machine's private IPv4 addresses, on the
// port this request used), the one its network traffic goes out through first; none when the server only listens on
// this machine.
func (s *Server) lanURLs(r *http.Request) []string {
	urls, _, _ := s.homeAddresses(r)
	return urls
}

// homeAddresses is lanURLs, with guess, the address the server has on the home network (its main one), which is
// there even when it only listens on this computer, so first-run setup can say where it will be once it's opened up;
// and localOnly, true when it listens on loopback only.
func (s *Server) homeAddresses(r *http.Request) (urls []string, guess string, localOnly bool) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	_, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		_, port, _ = net.SplitHostPort(s.Cfg.Listen)
	}
	var ips []net.IP
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				ips = append(ips, ipn.IP)
			}
		}
	}
	return homeURLs(s.Cfg.Listen, scheme, port, ips, mainLANIP())
}

// homeURLs works out homeAddresses from the listen address, the machine's addresses and its main one.
func homeURLs(listen, scheme, port string, ips []net.IP, main net.IP) (urls []string, guess string, localOnly bool) {
	url := func(ip net.IP) string {
		host := ip.String()
		if port != "" {
			host = net.JoinHostPort(host, port)
		}
		return scheme + "://" + host
	}
	var home []net.IP // private IPv4 addresses, the main one first
	if main != nil && main.To4() != nil && main.IsPrivate() {
		home = append(home, main)
	}
	for _, ip := range ips {
		if ip.To4() != nil && ip.IsPrivate() && !slices.ContainsFunc(home, ip.Equal) {
			home = append(home, ip)
		}
	}
	if len(home) > 0 {
		guess = url(home[0])
	}
	urls = []string{}
	if h, _, err := net.SplitHostPort(listen); err == nil && h != "" && h != "0.0.0.0" && h != "::" {
		ip := net.ParseIP(h)
		if ip == nil || ip.IsLoopback() {
			return urls, guess, true
		}
		// One address of this machine: only that one answers.
		if ip.To4() != nil && ip.IsPrivate() {
			return []string{url(ip)}, url(ip), false
		}
		return urls, guess, false
	}
	for _, ip := range home {
		urls = append(urls, url(ip))
	}
	return urls, guess, false
}

// mainLANIP is the address this computer's traffic to the rest of the network goes out from: what its default route
// uses. A UDP socket is pointed at a documentation address (TEST-NET-1) to ask the system which; nothing is sent.
func mainLANIP() net.IP {
	c, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return nil
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok && a.IP.IsPrivate() {
		return a.IP
	}
	return nil
}

// webTags: each client file's tag, from its content, worked out once per file as it is.
var webTags sync.Map // "path size modtime" -> `"sha256 prefix"`

// webTag is a strong ETag for one of the client's files: the first 16 bytes of its SHA-256.
// Kept by path, size and time, so a development build reading web/dist from disk gets a new
// tag when a file changes.
func (s *Server) webTag(p string, st fs.FileInfo) string {
	key := fmt.Sprintf("%s %d %d", p, st.Size(), st.ModTime().UnixNano())
	if v, ok := webTags.Load(key); ok {
		return v.(string)
	}
	b, err := fs.ReadFile(s.Web, p)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	tag := `"` + hex.EncodeToString(sum[:16]) + `"`
	webTags.Store(key, tag)
	return tag
}
