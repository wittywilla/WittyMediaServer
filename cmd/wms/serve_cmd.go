package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/server"
	"github.com/wittywilla/wittymediaserver/internal/themes"
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
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	tools, err := ffmpeg.Locate(cfg.FFmpegPath, cfg.FFprobePath)
	if err != nil {
		log.Error("ffmpeg", "err", err)
		return 1
	}
	log.Info("ffmpeg", "path", tools.FFmpeg, "version", tools.Version)
	// Find out which hardware encoders really work here (for Settings).
	go tools.ProbeHardware(context.Background())

	database, err := db.Open(cfg.DataDir)
	if err != nil {
		log.Error("database", "err", err)
		return 1
	}
	defer database.Close()

	store := &library.Store{DB: database}
	tmdbKey := func() string { return database.GetSetting(context.Background(), "tmdbKey", "") }
	scanner := library.NewScanner(store, tools, cfg.CacheDir, tmdbKey, log)
	scanner.TVDBKey = func() (string, string) {
		ctx := context.Background()
		return database.GetSetting(ctx, "tvdbKey", ""), database.GetSetting(ctx, "tvdbPin", "")
	}
	scanner.SaveMeta = func() bool { return database.GetSetting(context.Background(), "saveMetadata", "1") == "1" }
	hlsMgr := hls.NewManager(tools, cfg.TranscodeDir, cfg.MaxTranscode, log)
	dvdMgr := dvdsession.NewManager(hlsMgr, tools, cfg.CacheDir, "/api", log)

	themeMgr := &themes.Manager{Dir: filepath.Join(cfg.DataDir, "themes")}
	if err := themeMgr.Install(builtinthemes.Builtin); err != nil {
		log.Warn("themes", "err", err)
	}

	webFS := web.Dist()
	if cfg.Dev {
		webFS = os.DirFS(filepath.Join("web", "dist"))
	}
	srv := &server.Server{
		Cfg: cfg, DB: database, Log: log, Tools: tools, Store: store, Scanner: scanner,
		Auth: &auth.Service{DB: database}, HLS: hlsMgr, DVD: dvdMgr, Themes: themeMgr, Web: webFS, Version: version,
	}
	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		log.Error("listen", "addr", cfg.Listen, "err", err)
		return 1
	}
	log.Info("WittyMediaServer listening", "url", listenURL(cfg.Listen), "data", cfg.DataDir)

	// Rescan libraries periodically.
	go func() {
		for range time.Tick(6 * time.Hour) {
			libs, _ := store.Libraries(context.Background())
			for _, l := range libs {
				scanner.Start(l.ID)
			}
		}
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()
	if cfg.WatchStdin {
		// The tray launcher holds our stdin open; when it closes the pipe we
		// shut down gracefully (Windows has no SIGTERM to deliver).
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			cancel()
			time.Sleep(10 * time.Second)
			os.Exit(0)
		}()
	}
	go func() {
		<-ctx.Done()
		log.Info("shutting down")
		hlsMgr.StopAll()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(sctx)
	}()
	if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		return 1
	}
	return 0
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
