package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/desktop"
	"github.com/wittywilla/wittymediaserver/internal/discovery"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/livetv"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/modules/bdremote"
	"github.com/wittywilla/wittymediaserver/internal/modules/dvdremote"
	"github.com/wittywilla/wittymediaserver/internal/server"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/internal/update"
	builtinthemes "github.com/wittywilla/wittymediaserver/themes"
	"github.com/wittywilla/wittymediaserver/web"
)

func cmdServe(args []string) int {
	cfg, err := config.Parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logging.FreeSpace = sysinfo.FreeBytes
	logs, err := logging.New(cfg.LogDir, level, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "log folder:", err)
		return 1
	}
	defer logs.Close()
	log := slog.New(logs.Handler())
	slog.SetDefault(log)
	if h := logs.Health(); h.Error != "" {
		log.Error("the log folder can't be used: messages stay in memory until it can", "dir", cfg.LogDir, "err", h.Error)
	}
	// Armed before anything else can go wrong: a crash from here on leaves a report.
	guard, lastRun, err := logging.StartCrashGuard(filepath.Join(cfg.DataDir, "crash"), cfg.LogDir, version)
	if err != nil {
		log.Warn("crash reports can't be kept", "err", err)
	}
	defer guard.Close()
	for _, c := range lastRun {
		log.Error("the server stopped unexpectedly last time", "report", c.ID, "kind", c.Kind, "version", c.Version, "summary", c.Summary)
	}
	if cfg.ConfigFile != "" {
		log.Info("config", "file", cfg.ConfigFile, "portable", cfg.Portable)
	}

	tools, err := ffmpeg.Locate(cfg.FFmpegPath, cfg.FFprobePath)
	if err != nil {
		log.Error("ffmpeg", "err", err)
		return 1
	}
	log.Info("ffmpeg", "path", tools.FFmpeg, "version", tools.Version)
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		log.Error("database", "err", err)
		return 1
	}
	defer database.Close()

	// Log choices made in Settings → Logs (folder, level, categories).
	{
		ctx := context.Background()
		if d := database.GetSetting(ctx, "logDir", ""); d != "" {
			if err := logs.SetDir(d); err != nil {
				log.Warn("log folder from settings is not usable, using the default", "dir", d, "err", err)
			}
		}
		if v := database.GetSetting(ctx, "logLevel", ""); v != "" {
			if lv, ok := parseLevelName(v); ok {
				logs.SetLevel(lv)
			}
		}
		if v := database.GetSetting(ctx, "logCategories", ""); v != "" {
			logs.SetCategories(strings.Split(v, ","))
		}
		if v := database.GetSetting(ctx, "logLevels", ""); v != "" {
			var saved map[string]string
			if err := json.Unmarshal([]byte(v), &saved); err != nil {
				log.Warn("saved log levels can't be read", "err", err)
			}
			want := map[string]slog.Level{}
			for k, name := range saved {
				if lv, ok := logging.ParseLevel(name); ok {
					want[k] = lv
				}
			}
			logs.SetLevels(want)
		}
		if level == slog.LevelDebug {
			logs.SetFloor(slog.LevelDebug) // -log debug means everything, whatever the settings say
		}
		guard.SetLogDir(logs.Dir())
	}

	// Find out which hardware encoders really work here, if any are to be used.
	{
		ctx := context.Background()
		if database.GetSetting(ctx, "hwEncoders", "") != "" || database.GetSetting(ctx, "hwAccel", "0") == "1" {
			tools.StartHardwareProbe()
		}
	}

	store := &library.Store{DB: database}
	// Parental controls need the minimum age of titles saved before they existed.
	if err := store.BackfillAges(context.Background()); err != nil {
		log.Warn("rating ages", "err", err)
	}
	if err := store.ResetScanning(context.Background()); err != nil {
		log.Warn("scan flags", "err", err)
	}
	tmdbKey := func() string { return database.GetSetting(context.Background(), "tmdbKey", "") }
	scanner := library.NewScanner(store, tools, cfg.CacheDir, tmdbKey, log)
	scanner.ImagesDir = cfg.ImagesDir
	scanner.TVDBKey = func() (string, string) {
		ctx := context.Background()
		return database.GetSetting(ctx, "tvdbKey", ""), database.GetSetting(ctx, "tvdbPin", "")
	}
	scanner.TVMazeOn = func() bool { return database.GetSetting(context.Background(), "tvmaze", "1") == "1" }
	scanner.AnimeTitles = func() string { return database.GetSetting(context.Background(), "animeTitles", "english") }
	scanner.WikidataOn = func() bool { return database.GetSetting(context.Background(), "wikidata", "1") == "1" }
	scanner.MusicBrainzOn = func() bool { return database.GetSetting(context.Background(), "musicbrainz", "1") == "1" }
	scanner.Agents = func() []library.Agent {
		return library.ParseAgents(database.GetSetting(context.Background(), "agents", ""))
	}
	scanner.SaveMeta = func() bool { return database.GetSetting(context.Background(), "saveMetadata", "1") == "1" }
	gamemeta.UserAgent = "WittyMediaServer/" + version + " (https://github.com/wittywilla/WittyMediaServer)"
	gameMeta := library.NewGameMeta(store, cfg.ImagesDir, cfg.CacheDir, log)
	gameMeta.SaveMeta, gameMeta.Save = scanner.SaveMeta, scanner.SaveItemMetadata
	scanner.GameMeta = gameMeta
	hlsMgr := hls.NewManager(tools, cfg.TranscodeDir, cfg.MaxTranscode, log)
	dvdMgr := dvdsession.NewManager(hlsMgr, tools, cfg.CacheDir, "/api", log)
	bdMgr := bdsession.NewManager(hlsMgr, tools, cfg.CacheDir, "/api", log)
	bdMgr.BDJ = func() (bool, string) {
		ctx := context.Background()
		return database.GetSetting(ctx, "bdjEnabled", "0") == "1", database.GetSetting(ctx, "javaPath", "")
	}

	themeMgr := &themes.Manager{Dir: filepath.Join(cfg.DataDir, "themes")}
	if err := themeMgr.Install(builtinthemes.Builtin); err != nil {
		log.Warn("themes", "err", err)
	}

	// Updates: remove what the last update set aside, and get ready to
	// install the next one. Nothing in the data folders is ever replaced.
	progDir := config.ProgramDir()
	update.Cleanup(progDir)
	protect := []string{cfg.DataDir, cfg.CacheDir, cfg.TranscodeDir, cfg.ImagesDir, cfg.LogDir, cfg.ConfigFile, logs.Dir()}
	updater := update.New(version, progDir, protect, library.SafeClient.Transport, log)
	restartReq := make(chan struct{}, 1)
	updater.Restart = func() {
		select {
		case restartReq <- struct{}{}:
		default:
		}
	}

	// Modules: which version of each to use. Nothing here goes online; only Run does,
	// and only while the settings allow it.
	modMgr := modules.New(modules.Config{
		Dir: filepath.Join(cfg.DataDir, "modules"), Server: version, Builtins: append(append(modules.GoManifests(), web.ModuleManifests()...), modules.ServerManifests()...),
		Transport: library.SafeClient.Transport, Log: log, Launcher: modulesLauncher(), LogEnv: logs.ModuleEnv,
		CheckEnabled: func() bool {
			ctx := context.Background()
			v := database.GetSetting(ctx, "moduleUpdateCheck", "")
			if v == "" {
				v = database.GetSetting(ctx, "updateCheck", "1")
			}
			return v == "1"
		},
		AutoInstall: func() bool { return database.GetSetting(context.Background(), "moduleAutoInstall", "0") == "1" },
	})
	defer modMgr.Close()
	library.SetPhotoEngine(modules.PhotoEngine{M: modMgr})
	livetv.SetEngine(modules.Tuners{M: modMgr}.New, modules.Tuners{M: modMgr}.Discover)
	dvdsession.SetEngine(dvdremote.Opener{M: modMgr, Log: log}.Open)
	bdremote.Use(modMgr)

	webFS := web.Dist()
	if cfg.Dev {
		webFS = os.DirFS(filepath.Join("web", "dist"))
	}
	srv := &server.Server{
		Cfg: cfg, DB: database, Log: log, Tools: tools, Store: store, Scanner: scanner, GameMeta: gameMeta,
		Auth: &auth.Service{DB: database}, HLS: hlsMgr, DVD: dvdMgr, BD: bdMgr, Themes: themeMgr, Web: webFS, Version: version, Logs: logs,
		Update: updater, Modules: modMgr, Crashes: guard,
	}
	handler := srv.Handler()
	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ErrorLog:          logs.StdLogger("http", slog.LevelInfo),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	ln, err := listen(cfg.Listen)
	if err != nil {
		log.Error("listen", "addr", cfg.Listen, "err", err)
		return 1
	}
	log.Info("WittyMediaServer listening", "url", listenURL(cfg.Listen), "data", cfg.DataDir)
	srv.LogStartup(context.Background())
	if err := srv.Auth.HashStoredTokens(context.Background()); err != nil {
		log.Warn("session tokens", "err", err)
	}
	if srv.Auth.UserCount(context.Background()) == 0 {
		// On the console only, on purpose: the person at the server is meant to see
		// it, and the log files and viewer never hold it.
		logging.AddSecret(srv.SetupCode())
		fmt.Fprintln(os.Stderr, "This server has no accounts yet. Set it up in your browser. From another network it asks for this code:", srv.SetupCode())
		log.Warn("This server has no accounts yet. Set it up in your browser; the setup code is on the server's console.")
	}
	// Remote access: nothing opens unless the owner turned it on.
	srv.StartRemote(context.Background(), handler)
	// First start on a desktop: open the setup wizard in the browser.
	if cfg.OpenBrowser && srv.Auth.UserCount(context.Background()) == 0 && !desktop.Headless() {
		if err := desktop.OpenURL(listenURL(cfg.Listen)); err != nil {
			log.Info("open the setup page in your browser", "url", listenURL(cfg.Listen))
		}
	}

	// Check podcast feeds for new episodes.
	logging.Loop("podcast refresh", func() {
		time.Sleep(time.Minute)
		for {
			scanner.RefreshPodcasts(context.Background())
			time.Sleep(2 * time.Hour)
		}
	}, "cat", "library")

	// Radio: details from the directory, and station checks if turned on. Each step
	// has its own limits, and does nothing for a server with no stations.
	go func() {
		time.Sleep(10 * time.Minute)
		for {
			scanner.RefreshRadio(context.Background())
			time.Sleep(6 * time.Hour)
		}
	}()

	// Rescan libraries periodically.
	logging.Loop("periodic library scan", func() {
		for range time.Tick(6 * time.Hour) {
			libs, err := store.Libraries(context.Background())
			if err != nil {
				log.Error("periodic scan: can't list the libraries", "cat", "library", "err", err)
			}
			for _, l := range libs {
				scanner.Start(l.ID)
			}
		}
	}, "cat", "library")

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()
	go gameMeta.Run(ctx)
	if cfg.WatchStdin {
		// The tray launcher holds our stdin open; when it closes the pipe we
		// shut down gracefully (Windows has no SIGTERM to deliver).
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			cancel()
			time.Sleep(10 * time.Second)
			log.Warn("shutdown took too long: stopping now")
			logs.Close()
			guard.Close() // asked to stop, so not a crash
			os.Exit(0)
		}()
	}
	// Let apps on the home network find this server (not when it only listens on
	// this machine, where nobody else could connect anyway).
	if reachable, bind := discovery.Reachable(cfg.Listen); reachable {
		port := 0
		if a, ok := ln.Addr().(*net.TCPAddr); ok {
			port = a.Port
		}
		d := &discovery.Service{
			Info:    func() discovery.Info { return srv.DiscoveryInfo(port) },
			Enabled: func() bool { return database.GetSetting(context.Background(), "lanDiscovery", "1") == "1" },
			Bind:    bind,
			Log:     log.With("cat", "remote"),
		}
		srv.Discovery = d
		logging.Go("discovery", func() { d.Run(ctx) }, "cat", "remote")
	}
	srv.WatchModules(ctx)
	logging.Loop("update check", func() {
		updater.Run(ctx, func() bool { return database.GetSetting(context.Background(), "updateCheck", "1") == "1" })
	}, "cat", "update")
	logging.Loop("module update check", func() { modMgr.Run(ctx) }, "cat", "modules")
	var restarting atomic.Bool
	go func() {
		select {
		case <-restartReq:
			restarting.Store(true)
			cancel()
		case <-ctx.Done():
		}
	}()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		log.Info("shutting down")
		hlsMgr.StopAllLive()
		hlsMgr.StopAll()
		if srv.TV != nil {
			srv.TV.Close()
		}
		srv.StopGames() // saves every running game
		srv.StopRigs()
		srv.StopMoonlight()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.StopRemote(sctx)
		if err := httpServer.Shutdown(sctx); err != nil {
			log.Info("streams still open at shutdown are being closed", "err", err)
		}
		_ = httpServer.Close() // long streams: their contexts end, and their FFmpeg with them
		hlsMgr.StopAll()
	}()
	if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		return 1
	}
	<-stopped // Serve returns as soon as shutdown begins: don't close the database under it
	if restarting.Load() {
		return relaunch(cfg, log)
	}
	return 0
}

// relaunch starts the newly installed version after an update. The tray
// app and systemd start the server again themselves when it exits with
// update.ExitRestart; otherwise the new program is started here, with the
// same arguments, before this one exits.
func relaunch(cfg *config.Config, log *slog.Logger) int {
	if cfg.WatchStdin || os.Getenv("INVOCATION_ID") != "" {
		log.Info("restarting for the update")
		return update.ExitRestart
	}
	cmd := exec.Command(startExe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), "WMS_RESTARTED=1")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	detach(cmd)
	if err := cmd.Start(); err != nil {
		log.Error("could not start the updated server; start it again by hand", "err", err)
		return update.ExitRestart
	}
	log.Info("started the updated server", "pid", cmd.Process.Pid)
	_ = cmd.Process.Release()
	return 0
}

// listen opens the server's port. Right after an update the previous
// process may still be letting go of it, so keep trying for a while.
func listen(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil || os.Getenv("WMS_RESTARTED") != "1" {
		return ln, err
	}
	for i := 0; i < 40 && err != nil; i++ {
		time.Sleep(500 * time.Millisecond)
		ln, err = net.Listen("tcp", addr)
	}
	return ln, err
}

func listenURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func parseLevelName(v string) (slog.Level, bool) {
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(strings.ToUpper(v))); err != nil {
		return 0, false
	}
	return lv, true
}
