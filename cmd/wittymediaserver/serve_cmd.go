// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/licenses"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/backup"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/cd"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/desktop"
	"github.com/wittywilla/wittymediaserver/internal/discovery"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/instance"
	"github.com/wittywilla/wittymediaserver/internal/layout"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/livetv"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/modules/bdremote"
	"github.com/wittywilla/wittymediaserver/internal/modules/dvdremote"
	"github.com/wittywilla/wittymediaserver/internal/modules/photoengine"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/remote"
	"github.com/wittywilla/wittymediaserver/internal/reset"
	"github.com/wittywilla/wittymediaserver/internal/server"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/internal/update"
	builtinthemes "github.com/wittywilla/wittymediaserver/themes"
	"github.com/wittywilla/wittymediaserver/web"
)

func cmdServe(args []string) int {
	startedAt := time.Now()
	cfg, err := config.Parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if hint := unwritableData(err, os.Getenv("WMS_DATA_DIR")); hint != "" {
			fmt.Fprintln(os.Stderr, hint)
		}
		return 2
	}
	// One server per data folder, before a reset, a restore or the database touches it.
	lock, cfg, code, ok, lockWarn := claimDataFolder(cfg, args)
	if !ok {
		return code
	}
	defer lock.Release()
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

	// A reset waiting from last time happens first, before anything opens these files.
	resetDone := reset.Apply(reset.Folders{Data: cfg.DataDir, Program: config.ProgramDir(), Config: cfg.ConfigFile})
	repairDone := reset.ApplyRepair(cfg.DataDir)
	restoreDone := backup.Apply(backup.Folders{Data: cfg.DataDir, Images: cfg.ImagesDir})
	// A reset or restore has just moved folders away that Parse made: make them again,
	// or every thumbnail fails until the next start.
	if err := cfg.MakeFolders(); err != nil {
		fmt.Fprintln(os.Stderr, "data folders:", err)
		return 1
	}
	// Armed before anything else can go wrong: a crash from here on leaves a report. It
	// reads what the last run left before this one's standard error can replace it.
	crashDir := filepath.Join(cfg.DataDir, "crash")
	guard, lastRun, guardErr := logging.StartCrashGuard(crashDir, cfg.LogDir, version)
	console := io.Writer(os.Stderr)
	stderrFile := logging.CaptureStderr(crashDir)
	if stderrFile != "" {
		console = io.Discard // nothing reads it, and the log has its own files
	}
	logs, err := logging.New(cfg.LogDir, level, console)
	if err != nil {
		fmt.Fprintln(os.Stderr, "log folder:", err)
		logging.NoteStop(logging.StopFatal, "the log folder can't be used: "+err.Error(), "")
		guard.Close()
		return 1
	}
	defer logs.Close()
	log := slog.New(logs.Handler())
	slog.SetDefault(log)
	logs.SetFlightRecorder(true) // from the first line; Settings may turn it off once they're read
	if h := logs.Health(); h.Error != "" {
		log.Error("the log folder can't be used: messages stay in memory until it can", "dir", cfg.LogDir, "err", h.Error)
	}
	if guardErr != nil {
		log.Warn("crash reports can't be kept", "err", guardErr)
	}
	defer guard.Close() // a clean stop: how it ended goes in last-run.json
	logStart(log, guard, lastRun, stderrFile)
	if cfg.ConfigFile != "" {
		log.Info("config", "file", cfg.ConfigFile, "portable", cfg.Portable)
	}
	if cfg.SettingsFile != "" {
		log.Info("the port is set in a settings file", "file", cfg.SettingsFile, "listen", cfg.Listen)
	}
	for _, p := range cfg.SettingsProblems {
		log.Warn("a line of the settings file was skipped: "+p, "cat", "server")
	}
	if lockWarn != "" {
		log.Warn(lockWarn, "cat", "server", "data", cfg.DataDir)
	}

	tools, err := ffmpeg.Locate(cfg.FFmpegPath, cfg.FFprobePath)
	if err != nil {
		log.Error("ffmpeg", "err", err)
		return stopOnError(log, "FFmpeg wasn't found", err)
	}
	log.Info("ffmpeg", "path", tools.FFmpeg, "version", tools.Version)
	if repairDone != nil {
		if repairDone.OK {
			log.Warn("a repaired database took over", "cat", "server", "kept", repairDone.Kept)
		} else {
			log.Error("a repaired database couldn't take over, and the old one is still in use", "cat", "server", "err", repairDone.Error)
		}
	}
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		log.Error("database", "err", err)
		fmt.Fprintln(os.Stderr, "The database couldn't be opened. To find out why and repair it, run: wittymediaserver doctor")
		return stopOnError(log, "the database couldn't be opened", err)
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

	if ffmpeg.InContainer() {
		if msg := puidMismatch(os.Getenv("PUID"), os.Getenv("PGID"), os.Getuid(), os.Getgid()); msg != "" {
			log.Warn(msg)
		}
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
	scanner.NoPoster = func() string { return database.GetSetting(context.Background(), "noPoster", library.NoPosterFrame) }
	gamemeta.UserAgent = "WittyMediaServer/" + version + " (https://github.com/wittywilla/WittyMediaServer)"
	gameMeta := library.NewGameMeta(store, cfg.ImagesDir, cfg.CacheDir, log)
	gameMeta.SaveMeta, gameMeta.Save = scanner.SaveMeta, scanner.SaveItemMetadata
	scanner.GameMeta = gameMeta
	hlsMgr := hls.NewManager(tools, cfg.TranscodeDir, cfg.MaxTranscode, log)
	// A small machine: Go keeps its memory inside what there is, and scans give way
	// while a stream is being converted (internal/pace).
	pace.SetBusy(func() bool { return hlsMgr.ActiveCount()+hlsMgr.LiveCount() > 0 })
	if t := pace.Tune(); t.Class != pace.Roomy {
		log.Info("this is a small machine, so the server holds itself back", "cat", "server", "class", t.Class.String(), "cores", t.Cores, "memoryMB", t.MemoryMB,
			"goMemoryLimitMB", t.MemoryLimit>>20, "gcPercent", t.GCPercent, "probesAtOnce", pace.Slots(pace.Probes), "thumbnailsAtOnce", pace.Slots(pace.Thumbs))
	}
	dvdMgr := dvdsession.NewManager(hlsMgr, tools, cfg.CacheDir, "/api", log)
	bdMgr := bdsession.NewManager(hlsMgr, tools, cfg.CacheDir, "/api", log)
	bdMgr.BDJ = func() (bool, string) {
		return database.GetSetting(context.Background(), "bdjEnabled", "0") == "1", cfg.JavaPath
	}
	if old := database.GetSetting(context.Background(), "javaPath", ""); old != "" && cfg.JavaPath == "" {
		log.Warn("The Java location saved in Settings is no longer used: a web account can't choose what the server runs. If Java isn't found by itself, name it in the config file (javaPath) or with WMS_JAVA.", "cat", "discs")
	}

	themeMgr := &themes.Manager{Dir: filepath.Join(cfg.DataDir, "themes")}
	if err := themeMgr.Install(builtinthemes.Builtin); err != nil {
		log.Warn("themes", "err", err)
	}

	// Updates: remove what the last update set aside, and get ready to
	// install the next one. Nothing in the data folders is ever replaced.
	progDir := config.ProgramDir()
	update.Cleanup(progDir)
	layout.Current().ClearObsolete()
	protect := []string{cfg.DataDir, cfg.CacheDir, cfg.TranscodeDir, cfg.ImagesDir, cfg.LogDir, cfg.ConfigFile, logs.Dir()}
	updater := update.New(version, progDir, protect, logging.Calls(library.SafeClient.Transport, "update", "updates"), log)
	updater.LoadWithdrawals(filepath.Join(cfg.DataDir, "update-withdrawn.json"))
	// An installer for a copy that can't replace its own files waits here for the admin to run it.
	updater.DownloadDir = filepath.Join(cfg.CacheDir, "updates")
	updater.CleanDownloads()
	restartReq := make(chan struct{}, 1)
	updater.Restart = func() {
		// What asked for the restart says so first (a reset, a restore); otherwise it's an update.
		logging.NoteStop(logging.StopUpdate, "", "")
		select {
		case restartReq <- struct{}{}:
		default:
		}
	}

	// Modules: which version of each to use. Nothing here goes online; only Run does,
	// and only while the settings allow it.
	modules.ProtectServer()
	modMgr := modules.New(modules.Config{
		Dir: filepath.Join(cfg.DataDir, "modules"), Server: version, Builtins: modules.ServerManifests(), Known: true, CatalogURL: update.ModuleCatalog(), Starter: layout.Current().Modules(),
		WriteFolders: func(id string) []string {
			switch id {
			case "wittyplay": // saves, cores and what's unpacked stay where a server keeps them
				return []string{filepath.Join(cfg.DataDir, "gaming")}
			case "wittycouch": // so do the keys that say this server is paired with a computer
				return []string{filepath.Join(cfg.DataDir, "wittycouch"), filepath.Join(cfg.DataDir, "gamerigs")}
			}
			return nil
		},
		Transport: logging.Calls(library.SafeClient.Transport, "modules", "modules"), Log: log, Launcher: modulesLauncher(), LogEnv: logs.ModuleEnv,
		CheckEnabled: func() bool {
			ctx := context.Background()
			v := database.GetSetting(ctx, "moduleUpdateCheck", "")
			if v == "" {
				v = database.GetSetting(ctx, "updateCheck", "1")
			}
			return v == "1"
		},
		AutoInstall: func() bool { return database.GetSetting(context.Background(), "moduleAutoInstall", "0") == "1" },
		OnReady: func(id string) {
			licenses.Reload()
			scanFor(context.Background(), store, scanner, id, log)
		},
	})
	defer modMgr.Close()
	modStores := modules.NewStores(modules.StoresConfig{Manager: modMgr, Dir: filepath.Join(cfg.DataDir, "modules"), Transport: logging.Calls(library.SafeClient.Transport, "modules", "module store"), Log: log, CheckEnabled: modMgr.CheckEnabled})
	library.SetPhotoEngine(photoengine.PhotoEngine{M: modMgr})
	livetv.SetEngine(modules.Tuners{M: modMgr})
	cd.Open = modules.CDs{M: modMgr}.Open
	library.EngineThere = modMgr.Installed
	modMgr.UseServices()
	dvdsession.SetEngine(dvdremote.Opener{M: modMgr, Log: log}.Open)
	bdremote.Use(modMgr)

	webFS := web.Dist()
	if cfg.Dev {
		webFS = os.DirFS(filepath.Join("web", "dist"))
	}
	srv := &server.Server{
		Cfg: cfg, DB: database, Log: log, Tools: tools, Store: store, Scanner: scanner, GameMeta: gameMeta,
		Auth: &auth.Service{DB: database}, HLS: hlsMgr, DVD: dvdMgr, BD: bdMgr, Themes: themeMgr, Web: webFS, Version: version, Logs: logs,
		Update: updater, Modules: modMgr, ModuleStores: modStores, Crashes: guard, Restart: updater.Restart,
		Instance: lock.ID(),
	}
	if restoreDone != nil {
		if restoreDone.OK {
			log.Warn("a backup was restored", "cat", "server", "mode", restoreDone.Mode, "file", restoreDone.Name, "parts", strings.Join(restoreDone.Parts, ","), "safety", restoreDone.Safety)
		} else {
			log.Error("a backup couldn't be restored", "cat", "server", "err", restoreDone.Error)
		}
		srv.NoteRestore(restoreDone)
	}
	if resetDone != nil {
		if resetDone.OK {
			log.Warn("this server was reset", "cat", "server", "mode", resetDone.Mode, "copy", resetDone.Copy, "notes", resetDone.Notes)
		} else {
			log.Error("a reset couldn't be carried out, and nothing was changed", "cat", "server", "err", resetDone.Error)
		}
	}
	handler := srv.Handler()
	srv.ApplyEvidenceSettings(context.Background())
	srv.StartMetadata()
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
		why := portTaken(cfg.Listen, err)
		log.Error("listen", "addr", cfg.Listen, "err", err, "why", why)
		fmt.Fprintln(os.Stderr, why)
		return stopOnError(log, "it couldn't listen on "+cfg.Listen, err)
	}
	srv.Conns = &remote.ConnLimit{Total: cfg.MaxConnections, PerAddress: min(remote.DefaultPerAddress, cfg.MaxConnections), Policy: srv.NetPolicy, Log: log}
	ln = srv.Conns.Wrap(ln)
	log.Info("WittyMediaServer listening", "url", listenURL(cfg.Listen), "data", cfg.DataDir)
	if err := lock.Publish(instance.Info{PID: os.Getpid(), Started: time.Now(), Version: version, URLs: []string{listenURL(cfg.Listen)},
		Service: asService || os.Getenv("INVOCATION_ID") != ""}); err != nil {
		log.Warn("could not say where this server is, for a second start to find it", "cat", "server", "err", err)
	}
	srv.LogStartup(context.Background())
	if err := srv.Auth.HashStoredTokens(context.Background()); err != nil {
		log.Warn("session tokens", "err", err)
	}
	if srv.Auth.NoAccounts(context.Background()) {
		// On the console only, on purpose: the person at the server is meant to see
		// it, and the log files and viewer never hold it.
		logging.AddSecret(srv.SetupCode())
		fmt.Fprintln(os.Stderr, "This server has no accounts yet. Set it up in your browser. From another device it asks for this code:", srv.SetupCode())
		if err := srv.WriteSetupCode(); err != nil {
			log.Warn("could not write down the setup code", "err", err)
		}
		log.Warn("This server has no accounts yet. Set it up in your browser. From another device, the setup code is on the server's console and in setup-code.txt in its data folder.")
	}
	// Remote access: nothing opens unless the owner turned it on.
	srv.StartRemote(context.Background(), handler)
	// First start on a desktop: open the setup wizard in the browser.
	if cfg.OpenBrowser && srv.Auth.NoAccounts(context.Background()) && !desktop.Headless() {
		if err := desktop.OpenURL(listenURL(cfg.Listen)); err != nil {
			log.Info("open the setup page in your browser", "url", listenURL(cfg.Listen))
		}
	}

	// Check podcast feeds for new episodes.
	logging.Loop("podcast refresh", func() {
		time.Sleep(time.Minute)
		for {
			scanner.RefreshPodcasts(pace.Background(context.Background()))
			time.Sleep(2 * time.Hour)
		}
	}, "cat", "library")

	// Radio: details from the directory, and station checks if turned on. Each step
	// has its own limits, and does nothing for a server with no stations.
	go func() {
		time.Sleep(10 * time.Minute)
		for {
			scanner.RefreshRadio(pace.Background(context.Background()))
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watchStopEvents()
	sigs := make(chan os.Signal, 1)
	if asService {
		// The service manager says when to stop. Go turns a sign-out into SIGTERM, and a
		// service given a console somehow must not stop for someone else signing out.
		signal.Notify(sigs, os.Interrupt)
	} else {
		signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	}
	defer signal.Stop(sigs)
	go func() {
		select {
		case sig := <-sigs:
			noteSignal(sig)
			signal.Stop(sigs) // a second Ctrl+C ends it at once, as before
			cancel()
		case <-ctx.Done():
		}
	}()
	go gameMeta.Run(ctx)
	// When the server stops answering, write down what it was doing: see internal/logging/hang.go.
	logging.StartWatchdog(ctx, log, srv.WatchChecks(selfURL(ln.Addr()))...)
	go func() {
		// Windows asks a service to stop this way: it has no signals to send.
		select {
		case <-serviceStop:
			cancel()
		case <-ctx.Done():
		}
	}()
	if cfg.WatchStdin {
		// The tray launcher holds our stdin open; when it closes the pipe we
		// shut down gracefully (Windows has no SIGTERM to deliver).
		go func() {
			readTrayStop(os.Stdin)
			logging.NoteStopIfNothingElse(logging.StopTrayGone, "its pipe to the server closed")
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
	srv.StartBackupSchedule(ctx)
	logging.Loop("update check", func() {
		updater.Run(ctx, func() bool { return database.GetSetting(context.Background(), "updateCheck", "1") == "1" })
	}, "cat", "update")
	logging.Loop("module update check", func() { modMgr.Run(ctx) }, "cat", "modules")
	logging.Loop("module store check", func() { modStores.Run(ctx) }, "cat", "modules")
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
		st := logging.StopNoted()
		stopLine := []any{"cat", "server", "reason", st.Reason, "why", st.Words(), "uptime", logging.Uptime(time.Since(startedAt))}
		if st.Detail != "" {
			stopLine = append(stopLine, "detail", st.Detail)
		}
		if st.By != "" {
			stopLine = append(stopLine, "by", st.By)
		}
		log.Warn("stopping", append(stopLine, srv.RunningNow()...)...)
		logging.NoteStopping() // a stop while it wasn't answering keeps what it was doing
		hlsMgr.StopAllLive()
		hlsMgr.StopAll()
		if srv.TV != nil {
			srv.TV.Close()
		}
		srv.StopGames() // saves every running game
		srv.StopRigs()
		srv.StopCouch()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.StopRemote(sctx)
		srv.EndStreams()
		if err := httpServer.Shutdown(sctx); err != nil {
			log.Info("streams still open at shutdown are being closed", "err", err)
		}
		_ = httpServer.Close() // long streams: their contexts end, and their FFmpeg with them
		hlsMgr.StopAll()
		srv.Close() // what requests started on their own, and the activity on its way to the database
	}()
	if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		return stopOnError(log, "it stopped serving", err)
	}
	<-stopped // Serve returns as soon as shutdown begins: don't close the database under it
	if restarting.Load() {
		return relaunch(cfg, log)
	}
	return 0
}

// relaunch starts the server again, after an update or for a reset. The tray app,
// systemd and a container's restart policy do that themselves when it exits with
// update.ExitRestart; otherwise the program is started here, with the same
// arguments, before this one exits.
func relaunch(cfg *config.Config, log *slog.Logger) int {
	_, dockerErr := os.Stat("/.dockerenv")
	// The Windows service, and with WMS_SUPERVISED launchd and FreeBSD's daemon, start it again
	// themselves: a copy started here as well would fight them for the port.
	if cfg.WatchStdin || asService || os.Getenv("INVOCATION_ID") != "" || os.Getenv("WMS_SUPERVISED") == "1" || dockerErr == nil || os.Getenv("WMS_DOCKER") == "1" {
		log.Info("restarting")
		return update.ExitRestart
	}
	cmd := exec.Command(startExe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), "WMS_RESTARTED=1")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	detach(cmd)
	if err := cmd.Start(); err != nil {
		log.Error("could not start the server again; start it by hand", "err", err)
		return update.ExitRestart
	}
	log.Info("started the server again", "pid", cmd.Process.Pid)
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

// selfURL is how this machine reaches the server's own port, for the watchdog.
func selfURL(a net.Addr) string {
	t, ok := a.(*net.TCPAddr)
	if !ok {
		return "http://127.0.0.1"
	}
	ip := t.IP
	if ip == nil || ip.IsUnspecified() {
		ip = net.IPv4(127, 0, 0, 1)
	}
	return "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(t.Port))
}

func parseLevelName(v string) (slog.Level, bool) {
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(strings.ToUpper(v))); err != nil {
		return 0, false
	}
	return lv, true
}

// scanFor scans the libraries that have been waiting for a module: discs and games stay listed
// without theirs, with nothing known about what's in them.
func scanFor(ctx context.Context, store *library.Store, scanner *library.Scanner, id string, log *slog.Logger) {
	var kinds map[string]bool
	switch id {
	case "wittyplay":
		kinds = map[string]bool{"games": true}
	case "wittydvd", "wittybluray", "wittycd":
		kinds = map[string]bool{"discs": true, "movies": true, "shows": true}
	default:
		return
	}
	libs, err := store.Libraries(ctx)
	if err != nil {
		log.WarnContext(ctx, "looking for libraries to scan after a module arrived", "module", id, "err", err)
		return
	}
	for _, l := range libs {
		if kinds[l.Type] {
			scanner.Start(l.ID)
		}
	}
}
