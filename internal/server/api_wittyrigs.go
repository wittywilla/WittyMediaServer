package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/wittywilla/wittycouch/gamestream"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittyrigs"
)

// WittyRigs: the advanced option of hosting demanding games on this server's own hardware.
// It's off by default, and only ever looks at this machine. See docs/GAME-RIGS.md.

const wittyRigsSetting = "wittyRigsEnabled"

func init() {
	settingKeys = append(settingKeys, wittyRigsSetting)
	settingDefaults[wittyRigsSetting] = "0"
}

func (s *Server) wittyRigsRoutes(m *http.ServeMux) {
	m.Handle("GET /api/wittyrigs", s.requireAdmin(s.handleWittyRigs))
	m.Handle("PUT /api/wittyrigs", s.requireAdmin(s.handleWittyRigsSet))
}

// sunshineRig is the rig that stands for this machine, if there is one.
func (s *Server) sunshineRig(ctx context.Context) (*rigRow, error) {
	r, err := scanRig(s.DB.QueryRowContext(ctx, `SELECT `+rigCols+` FROM wittyrigs_rigs WHERE address IN ('127.0.0.1', '::1', 'localhost') LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

func (s *Server) wittyRigsStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body := map[string]any{"enabled": s.DB.GetSetting(ctx, wittyRigsSetting, "0") == "1", "installed": wittyrigs.Find(), "license": wittyrigs.License, "source": wittyrigs.Source, "running": false}
	if body["enabled"] == true {
		hctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, err := (&gamestream.Rig{Address: "127.0.0.1"}).Hello(hctx)
		body["running"] = err == nil
		// Added as soon as it can be, so the library is there once it's paired.
		if err == nil {
			if err := s.ensureSunshineRig(ctx); err != nil {
				s.Log.Warn("wittyrigs", "err", err)
			}
		}
		if rig, err := s.sunshineRig(ctx); err == nil && rig != nil {
			body["rig"] = s.rigViewFor(ctx, r, rig)
		}
	}
	writeJSON(w, 200, body)
}

// GET /api/wittyrigs
func (s *Server) handleWittyRigs(w http.ResponseWriter, r *http.Request) { s.wittyRigsStatus(w, r) }

func (s *Server) ensureSunshineRig(ctx context.Context) error {
	if rig, err := s.sunshineRig(ctx); err != nil || rig != nil {
		return err
	}
	info, err := (&gamestream.Rig{Address: "127.0.0.1"}).Hello(ctx)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(s.ServerName(ctx))
	if name == "" {
		name = "This server's games"
	}
	lib, err := s.Store.CreateRigLibrary(ctx, name)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO wittyrigs_rigs(library_id, name, address, port, uuid, created_at) VALUES(?,?,?,?,?,?)`,
		lib.ID, name, "127.0.0.1", 0, info.UUID, db.Now()); err != nil {
		_ = s.Store.DeleteLibrary(ctx, lib.ID)
		return err
	}
	return nil
}

// PUT /api/wittyrigs {enabled}: turning it off takes this machine's library away too, as nothing
// should be left offering games from a host that's been turned off.
func (s *Server) handleWittyRigsSet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var b struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	v := "0"
	if b.Enabled {
		v = "1"
	}
	if err := s.DB.SetSetting(ctx, wittyRigsSetting, v); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !b.Enabled {
		if rig, err := s.sunshineRig(ctx); err == nil && rig != nil {
			s.forgetRig(rig)
			if err := s.Store.DeleteLibrary(ctx, rig.LibraryID); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
	}
	s.Log.Info("wittyrigs", "enabled", b.Enabled, "by", userFrom(r).Name)
	s.wittyRigsStatus(w, r)
}
