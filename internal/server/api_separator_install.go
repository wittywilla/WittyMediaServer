// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/wittywilla/wittymediaserver/internal/licenses"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/separator"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Installing Demucs with one press, so karaoke's properly separated version is there by
// default rather than after an evening with Python. internal/separator/install.go says what's
// downloaded and where it goes. Administrators only, one at a time, never by itself.
//
//	GET    /api/karaoke/separator           whether it can be installed here, is, or is being
//	POST   /api/karaoke/separator/install   start installing it (a task)
//	DELETE /api/karaoke/separator           remove it

type separatorInstall struct {
	mu     sync.Mutex
	task   string // the install running now
	cancel context.CancelFunc
	failed string // why the last one failed
	phase  string // where the running one is, for Settings
	pct    int
}

func init() {
	// karaokeDevice: what the installed Demucs runs on. "auto" is what the install chose (the
	// fastest that passed its test); the others are separator.Choices' IDs.
	settingKeys = append(settingKeys, "karaokeDevice")
	settingDefaults["karaokeDevice"] = "auto"
	settingChecks["karaokeDevice"] = oneOf("auto", "cuda", "rocm", "mps", "dml", "cpu")
}

func (s *Server) separatorDir() string { return filepath.Join(s.Cfg.DataDir, "separator") }

// separatorDevice is the administrator's choice of what Demucs runs on, "auto" unless set.
func (s *Server) separatorDevice() string {
	if s.DB == nil {
		return "auto"
	}
	return s.DB.GetSetting(context.Background(), "karaokeDevice", settingDefaults["karaokeDevice"])
}

// installedSeparator is Demucs as the server installed it, running on what Settings chose, or nil.
func (s *Server) installedSeparator() *separator.Found {
	if s.Cfg == nil || s.Cfg.DataDir == "" {
		return nil
	}
	return separator.InstalledWith(s.separatorDir(), s.separatorDevice())
}

// separatorEnv is the environment a separator runs in: the server's own, the installed one's
// additions when that's the one, and FFmpeg's folder first on the PATH (Demucs reads songs with it).
func (s *Server) separatorEnv(prog string) []string {
	env := os.Environ()
	if f := s.installedSeparator(); f != nil && filepath.Clean(f.Path) == filepath.Clean(prog) {
		env = append(env, f.Env...)
	}
	if s.Tools != nil && s.Tools.FFmpeg != "" {
		env = separator.WithPath(env, filepath.Dir(s.Tools.FFmpeg))
	}
	return env
}

func (s *Server) separatorInstallRoutes(m *http.ServeMux) {
	m.Handle("GET /api/karaoke/separator", s.requireAdmin(s.handleSeparatorState))
	m.Handle("POST /api/karaoke/separator/install", s.requireAdmin(s.handleSeparatorInstall))
	m.Handle("DELETE /api/karaoke/separator", s.requireAdmin(s.handleSeparatorRemove))
}

// separatorMinMemoryMB is the least memory Demucs is offered with: separating a song on the processor
// holds about 3 GB at its peak, and the server and the system need theirs.
const separatorMinMemoryMB = 3500

// separatorInstallable says whether Demucs can be installed on this machine, and why not.
func (s *Server) separatorInstallable() (bool, string) {
	if ok, why := separator.Supported(); !ok {
		return false, why
	}
	if pace.Machine() == pace.Tiny {
		return false, "This machine is too small to separate songs: one would take hours. Songs use the quick version."
	}
	if mb := sysinfo.MemoryMB(); mb > 0 && mb < separatorMinMemoryMB {
		return false, fmt.Sprintf("This machine has %.1f GB of memory; separating a song needs about 4 GB. Songs use the quick version.", float64(mb)/1024)
	}
	return true, ""
}

func (s *Server) handleSeparatorState(w http.ResponseWriter, r *http.Request) {
	ok, why := s.separatorInstallable()
	machine := pace.Machine()
	j := &s.sepInstall
	j.mu.Lock()
	task, failed, phase, pct := j.task, j.failed, j.phase, j.pct
	j.mu.Unlock()
	acc := separator.DetectAccel()
	out := map[string]any{
		"supported": ok, "why": why, "machine": machine.String(),
		"task": task, "failed": failed, "phase": phase, "percent": pct,
		"packages": separator.Packages(runtime.GOOS, runtime.GOARCH), "uv": separator.UVVersion, "python": separator.PythonVersion, "model": separator.Model,
		"accel": acc, "downloadMB": acc.DownloadMB, "cpuDownloadMB": separator.AccelByID("cpu").DownloadMB,
		// directml: ONNX Runtime's DirectML build comes with an install for a card here (64-bit Windows).
		"directml": separator.DirectMLOn(runtime.GOOS, runtime.GOARCH), "directmlPackages": separator.DirectMLPackages,
	}
	if in := separator.ReadInfo(s.separatorDir()); in != nil {
		// accel: what songs run on now (the choice in Settings, or what the install chose);
		// auto: what the install chose; choices: what can be chosen.
		using := separator.AccelByID(in.Accel)
		if f := s.installedSeparator(); f != nil {
			using = f.Accel
		}
		out["installed"] = map[string]any{"version": in.Version, "model": in.Model, "at": in.Installed, "sizeBytes": separator.Size(s.separatorDir()),
			"accel": using, "auto": separator.AccelByID(in.Accel), "choices": separator.Choices(in), "device": s.separatorDevice()}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleSeparatorInstall(w http.ResponseWriter, r *http.Request) {
	if ok, why := s.separatorInstallable(); !ok {
		reason := "unsupported"
		if sup, _ := separator.Supported(); sup {
			reason = "too-small"
		}
		writeJSON(w, 409, map[string]any{"error": why, "reason": reason})
		return
	}
	if separator.ReadInfo(s.separatorDir()) != nil {
		writeJSON(w, 409, map[string]any{"error": "Demucs is already installed.", "reason": "installed"})
		return
	}
	// The graphics card when there's one Demucs can use, unless the administrator chose the
	// processor only (?accel=cpu: a smaller download), or DirectML on Windows (?accel=dml: for a
	// card CUDA doesn't reach, and for measuring it on one it does).
	acc := separator.DetectAccel()
	switch r.URL.Query().Get("accel") {
	case "cpu":
		acc = separator.AccelByID("cpu")
	case "dml":
		if separator.DirectMLOn(runtime.GOOS, runtime.GOARCH) {
			acc = separator.AccelByID("dml")
		}
	}
	j := &s.sepInstall
	j.mu.Lock()
	if j.task != "" {
		id := j.task
		j.mu.Unlock()
		writeJSON(w, 202, map[string]any{"task": id})
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := s.Tasks.Start(tasks.Spec{Type: tasks.TypeOther, Title: "Installing Demucs for karaoke", Phase: "Starting", Unit: "%", By: byUser(userFrom(r)), Cancel: cancel, AdminOnly: true})
	j.task, j.cancel, j.failed, j.phase, j.pct = t.ID(), cancel, "", "Starting", 0
	j.mu.Unlock()

	log := s.Log.With("cat", "extras", "session", logging.NewID("separator"))
	log.Info("installing Demucs for karaoke", "dir", s.separatorDir(), "uv", separator.UVVersion, "accel", acc.ID)
	ffmpegDir := ""
	if s.Tools != nil && s.Tools.FFmpeg != "" {
		ffmpegDir = filepath.Dir(s.Tools.FFmpeg)
	}
	logging.Go("separator-install", func() {
		said := &logging.Tail{}
		err := separator.Install(ctx, s.separatorDir(), ffmpegDir, acc, func(phase string, pct int) {
			t.Phase(phase)
			t.Progress(int64(pct), 100)
			j.mu.Lock()
			j.phase, j.pct = phase, pct
			j.mu.Unlock()
		}, said)
		j.mu.Lock()
		j.task, j.cancel = "", nil
		if err != nil && !errors.Is(err, context.Canceled) {
			j.failed = err.Error()
		}
		j.mu.Unlock()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Info("installing Demucs was stopped; nothing was kept")
			} else {
				log.Error("installing Demucs failed", "err", err, "output", strings.TrimSpace(said.String()))
			}
			t.Finish(err)
			return
		}
		log.Info("Demucs is installed for karaoke", "sizeMB", separator.Size(s.separatorDir())>>20)
		t.Result("Demucs is installed. Karaoke makes separated versions from now on.")
		t.Finish(nil)
		s.foundSeparator(context.Background(), true)
	})
	writeJSON(w, 202, map[string]any{"task": t.ID()})
}

func (s *Server) handleSeparatorRemove(w http.ResponseWriter, r *http.Request) {
	j := &s.sepInstall
	j.mu.Lock()
	if j.cancel != nil {
		j.cancel() // an install running: stopping it removes what it had done
		j.mu.Unlock()
		writeJSON(w, 200, map[string]any{"removed": true, "stopped": true})
		return
	}
	j.failed = ""
	j.mu.Unlock()
	if err := separator.Remove(s.separatorDir()); err != nil {
		writeErr(w, 500, "it couldn't be removed: "+err.Error())
		return
	}
	s.Log.Info("Demucs was removed", "cat", "extras", "by", adminName(r))
	s.foundSeparator(r.Context(), true)
	writeJSON(w, 200, map[string]any{"removed": true})
}

// installedLicenseBase is where the installed separator's parts start in the Licenses list's
// numbering, well past the server's own.
const installedLicenseBase = 1_000_000

// installedLicenses are the installed separator's parts, as the Licenses page lists them.
func (s *Server) installedLicenses() ([]licenses.Component, []separator.License) {
	if s.Cfg == nil || s.Cfg.DataDir == "" {
		return nil, nil
	}
	ls := separator.Licenses(s.separatorDir())
	out := make([]licenses.Component, 0, len(ls))
	for i, l := range ls {
		out = append(out, licenses.Component{ID: installedLicenseBase + i, Group: licenses.GroupPrograms, Name: l.Name, Version: l.Version,
			License: l.License, URL: l.URL, HasText: l.Text != "",
			Note: "Installed for karaoke with Demucs, from Settings, Lyrics and music information"})
	}
	return out, ls
}
