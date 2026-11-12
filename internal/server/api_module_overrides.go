// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
)

// Overrides (docs/MODULE-STORE.md, "Overriding a refusal"): an administrator's go-ahead past what
// the server would refuse, a module installed from a file, and the record of both. Every route
// here asks for the administrator's password again before it changes which code runs, and writes
// what it did to the module audit log.

// overrideBody is an administrator's go-ahead as a request carries it.
type overrideBody struct {
	Accept []string `json:"accept"`
	Typed  string   `json:"typed"`
	Grants []string `json:"grants"`
}

// kinds are what the override goes past, for the log ("" when there is none).
func (b *overrideBody) kinds() []string {
	if b == nil {
		return nil
	}
	return b.Accept
}

// overrideWords are the override's kinds for an audit row's detail.
func (b *overrideBody) words() string {
	if b == nil || len(b.Accept) == 0 {
		return ""
	}
	return ", overriding " + strings.Join(b.Accept, ", ") + grantWords(b.Grants)
}

// auditOverride writes a log record about a manual module override that no level or
// flood limit silences, beside the audit rows auditModule keeps.
func (s *Server) auditOverride(r *http.Request, msg, id string, args ...any) {
	logging.Audit(r.Context(), s.Log, msg, append([]any{"cat", "modules-store", "by", adminName(r), "module", id}, args...)...)
}

func (b *overrideBody) accept(by string) *modmgr.OverrideAccept {
	if b == nil {
		return nil
	}
	return &modmgr.OverrideAccept{Accept: b.Accept, Typed: b.Typed, Grants: b.Grants, By: by}
}

func (s *Server) moduleOverrideRoutes(m *http.ServeMux) {
	m.Handle("POST /api/module-uploads", s.requireAdmin(s.handleModuleUpload))
	m.Handle("POST /api/module-uploads/{token}/install", s.requireAdmin(s.handleModuleUploadInstall))
	m.Handle("DELETE /api/module-uploads/{token}", s.requireAdmin(s.handleModuleUploadDiscard))
	m.Handle("DELETE /api/modules/{id}/pending", s.requireAdmin(s.handleModulePendingDrop))
	m.Handle("DELETE /api/modules/{id}/override", s.requireAdmin(s.handleModuleOverrideRemove))
	m.Handle("GET /api/modules/audit", s.requireAdmin(s.handleModuleAudit))
}

// The pieces of an upload beside the archive, at most.
const (
	maxUploadSums = 1 << 20
	maxUploadSig  = 4 << 10
	maxUploadKey  = 256
)

// POST /api/module-uploads (multipart: archive, and sums, sig, key when they came with it): unpack
// and check a module release someone chose, and hold it for a go-ahead. Nothing runs.
func (s *Server) handleModuleUpload(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, modmgr.MaxUpload+maxUploadSums+maxUploadSig+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, 400, "send the archive as multipart/form-data, in a field called archive")
		return
	}
	var up modmgr.Upload
	up.By = userFrom(r).ID
	defer func() {
		if up.Archive != "" {
			_ = os.Remove(up.Archive)
		}
	}()
	small := func(p io.Reader, limit int64) ([]byte, error) {
		b, err := io.ReadAll(io.LimitReader(p, limit+1))
		if err == nil && int64(len(b)) > limit {
			err = errors.New("too large")
		}
		return b, err
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeErr(w, 400, "the upload was cut short or is too large: at most 512 MiB")
			return
		}
		switch part.FormName() {
		case "archive":
			name := filepath.Base(part.FileName())
			ext := ".tar.gz"
			if strings.HasSuffix(strings.ToLower(name), ".zip") {
				ext = ".zip"
			}
			f, err := os.CreateTemp("", "wms-module-upload-*"+ext)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			up.Archive, up.ArchiveName = f.Name(), name
			n, err := io.Copy(f, io.LimitReader(part, modmgr.MaxUpload+1))
			_ = f.Close()
			if err != nil || n > modmgr.MaxUpload {
				writeErr(w, 400, "the archive is too large: at most 512 MiB")
				return
			}
		case "sums":
			if up.Sums, err = small(part, maxUploadSums); err != nil {
				writeErr(w, 400, "the checksum list is too large")
				return
			}
		case "sig":
			if up.Sig, err = small(part, maxUploadSig); err != nil {
				writeErr(w, 400, "the signature is too large")
				return
			}
		case "key":
			b, err := small(part, maxUploadKey)
			if err != nil {
				writeErr(w, 400, "that isn't a public key")
				return
			}
			up.Key = strings.TrimSpace(string(b))
		}
		_ = part.Close()
	}
	if up.Archive == "" {
		writeErr(w, 400, "choose the module's release archive, in a field called archive")
		return
	}
	info, err := s.Modules.StageUpload(up)
	if err != nil {
		s.auditOverride(r, "module upload could not be checked", "", "file", up.ArchiveName, "err", err)
		writeErr(w, 500, err.Error())
		return
	}
	detail := info.ID + " " + info.Version
	if info.Refusal != "" {
		detail += ", refused: " + info.Refusal
	}
	s.auditModule(r, "module-upload", info.ID, detail)
	writeJSON(w, 200, info)
}

// POST /api/module-uploads/{token}/install {password, code, override}: install an upload with the
// go-ahead it needs.
func (s *Server) handleModuleUploadInstall(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	var b struct {
		Override *overrideBody `json:"override"`
		moduleSecrets
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil {
		writeErr(w, 400, "unreadable request")
		return
	}
	if b.Override == nil {
		writeErr(w, 400, "say what you're overriding: {\"override\": {\"accept\": [...]}}")
		return
	}
	if !s.moduleGate(w, r, b.moduleSecrets, "upload") {
		return
	}
	id, version := s.Modules.UploadOf(r.PathValue("token"), userFrom(r).ID)
	if err := s.Modules.InstallUpload(r.PathValue("token"), userFrom(r).ID, b.Override.accept(userFrom(r).Name)); err != nil {
		s.auditOverride(r, "module override install failed", id, "version", version, "from", "upload", "overrides", b.Override.kinds(), "grants", b.Override.Grants, "err", err)
		writeModuleErr(w, err)
		return
	}
	s.auditModule(r, "module-override", id, "version "+version+" installed from a file, overriding "+strings.Join(b.Override.Accept, ", ")+grantWords(b.Override.Grants))
	writeJSON(w, 202, s.Modules.Report())
}

func grantWords(g []string) string {
	if len(g) == 0 {
		return ""
	}
	return ", granting " + strings.Join(g, ", ")
}

// DELETE /api/module-uploads/{token}: forget an upload.
func (s *Server) handleModuleUploadDiscard(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	id, version := s.Modules.UploadOf(r.PathValue("token"), userFrom(r).ID)
	if err := s.Modules.DiscardUpload(r.PathValue("token"), userFrom(r).ID); err != nil {
		writeModuleErr(w, err)
		return
	}
	s.auditOverride(r, "module upload discarded", id, "version", version)
	w.WriteHeader(204)
}

// DELETE /api/modules/{id}/pending: forget a download held for a go-ahead.
func (s *Server) handleModulePendingDrop(w http.ResponseWriter, r *http.Request) {
	s.moduleAction(w, r, "drop-pending", 200, func(id string) error { return s.Modules.DropPending(id) })
}

// DELETE /api/modules/{id}/override {password, code}: take an override back.
func (s *Server) handleModuleOverrideRemove(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	if !s.moduleGateFromBody(w, r) {
		return
	}
	id := r.PathValue("id")
	libs, err := s.librariesByType(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.Modules.RemoveOverride(id, libs); err != nil {
		s.auditOverride(r, "module override could not be taken back", id, "err", err)
		writeModuleErr(w, err)
		return
	}
	s.auditModule(r, "module-override-removed", id, "")
	writeJSON(w, 200, s.Modules.Report())
}

// ModuleAudit is one change to which code runs.
type ModuleAudit struct {
	At     int64  `json:"at"`
	User   string `json:"user"`
	Action string `json:"action"`
	Module string `json:"module"`
	Detail string `json:"detail,omitempty"`
}

// GET /api/modules/audit?limit=: what was done to which modules, newest first.
func (s *Server) handleModuleAudit(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 1000 {
		limit = n
	}
	out := []ModuleAudit{}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT at, user_name, action, core, detail FROM game_core_audit WHERE core LIKE 'module %' ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	for rows.Next() {
		var a ModuleAudit
		var core string
		if err := rows.Scan(&a.At, &a.User, &a.Action, &core, &a.Detail); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		a.Module = strings.TrimPrefix(core, "module ")
		out = append(out, a)
	}
	writeJSON(w, 200, out)
}
