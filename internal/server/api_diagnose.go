package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	fb "github.com/wittywilla/wittyintegratedfilebrowser"

	"github.com/wittywilla/wittymediaserver/internal/diagnose"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// WittyDiagnoser, the disk diagnoser (Settings, WittyDiagnoser): checks of a DVD or Blu-ray, and
// of storage folders, for administrators. It only reads. Disc paths must be
// inside a library folder or the data folder; storage is chosen by ID only, so no
// route here can be pointed at somewhere else on the machine.

func (s *Server) diagnoseRoutes(m *http.ServeMux) {
	m.Handle("GET /api/diagnose/targets", s.requireAdmin(s.handleDiagnoseTargets))
	m.Handle("GET /api/diagnose/browse", s.requireAdmin(s.handleDiagnoseBrowse))
	m.Handle("POST /api/diagnose/disc", s.requireAdmin(s.handleDiagnoseDisc))
	m.Handle("POST /api/diagnose/storage", s.requireAdmin(s.handleDiagnoseStorage))
	m.Handle("GET /api/diagnose/jobs", s.requireAdmin(s.handleDiagnoseJobs))
	m.Handle("GET /api/diagnose/jobs/{id}", s.requireAdmin(s.handleDiagnoseJob))
	m.Handle("DELETE /api/diagnose/jobs/{id}", s.requireAdmin(s.handleDiagnoseCancel))
	m.Handle("GET /api/diagnose/jobs/{id}/report", s.requireAdmin(s.handleDiagnoseReport))
	if s.BundleExtras == nil {
		s.BundleExtras = func() []logging.BundleFile { return s.diagnoser().BundleFiles(3) }
	}
}

// diagnoser starts WittyDiagnoser's manager the first time it's wanted. It costs
// nothing until a check is asked for.
func (s *Server) diagnoser() *diagnose.Manager {
	s.diagOnce.Do(func() {
		s.diag = diagnose.NewManager(diagnose.Env{
			DataDir: s.Cfg.DataDir,
			Log:     s.Log,
			Version: s.Version,
			Busy:    s.streamsPlaying,
			Setting: func(key, def string) string { return s.DB.GetSetting(context.Background(), key, def) },
			ItemPaths: func(ctx context.Context, lib int64, limit int) ([]string, error) {
				rows, err := s.DB.QueryContext(ctx, `SELECT path FROM items WHERE library_id = ? AND missing = 0 LIMIT ?`, lib, limit)
				if err != nil {
					return nil, err
				}
				defer rows.Close()
				var out []string
				for rows.Next() {
					var p string
					if err := rows.Scan(&p); err != nil {
						return nil, err
					}
					out = append(out, p)
				}
				return out, rows.Err()
			},
		})
	})
	return s.diag
}

// streamsPlaying says someone may be watching: a stream being converted, or a
// disc or a channel open.
func (s *Server) streamsPlaying() bool {
	if s.HLS != nil && (s.HLS.ActiveCount() > 0 || s.HLS.LiveCount() > 0) {
		return true
	}
	return s.DVD != nil && s.DVD.Count() > 0 || s.BD != nil && s.BD.Count() > 0
}

// diagnoseFolder is a folder the storage check can be pointed at.
type diagnoseFolder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Role string `json:"role"` // library, data, cache, transcode, images, logs
}

type diagnoseLibrary struct {
	ID      int64            `json:"id"`
	Name    string           `json:"name"`
	Type    string           `json:"type"`
	Folders []diagnoseFolder `json:"folders"`
}

type diagnoseDisc struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Library int64  `json:"library"`
}

type diagnoseTargets struct {
	Libraries []diagnoseLibrary `json:"libraries"`
	Data      []diagnoseFolder  `json:"data"`
	Discs     []diagnoseDisc    `json:"discs"`
}

// diagnoseFolders lists every storage target, by ID.
func (s *Server) diagnoseFolders(ctx context.Context) (diagnoseTargets, error) {
	out := diagnoseTargets{Libraries: []diagnoseLibrary{}, Data: []diagnoseFolder{}, Discs: []diagnoseDisc{}}
	libs, err := s.Store.Libraries(ctx)
	if err != nil {
		return out, err
	}
	for _, l := range libs {
		d := diagnoseLibrary{ID: l.ID, Name: l.Name, Type: l.Type, Folders: []diagnoseFolder{}}
		for i, p := range l.Paths {
			d.Folders = append(d.Folders, diagnoseFolder{ID: "lib:" + strconv.FormatInt(l.ID, 10) + ":" + strconv.Itoa(i), Name: l.Name + ": " + filepath.Base(p), Path: p, Role: "library"})
		}
		out.Libraries = append(out.Libraries, d)
	}
	logDir := s.Cfg.LogDir
	if s.Logs != nil {
		logDir = s.Logs.Dir()
	}
	for _, f := range []diagnoseFolder{
		{ID: "data:data", Name: "Data folder", Path: s.Cfg.DataDir, Role: "data"},
		{ID: "data:transcode", Name: "Transcode folder", Path: s.Cfg.TranscodeDir, Role: "transcode"},
		{ID: "data:cache", Name: "Cache folder", Path: s.Cfg.CacheDir, Role: "cache"},
		{ID: "data:images", Name: "Artwork folder", Path: s.Cfg.ImagesDir, Role: "images"},
		{ID: "data:logs", Name: "Log folder", Path: logDir, Role: "logs"},
	} {
		if f.Path != "" {
			out.Data = append(out.Data, f)
		}
	}
	if items, _, err := s.Store.Items(ctx, library.ItemQuery{Kind: "disc", Limit: 500, Sort: "title", NoTotal: true, Brief: true}); err == nil {
		for _, it := range items {
			out.Discs = append(out.Discs, diagnoseDisc{ID: it.ID, Title: it.Title, Kind: it.Kind, Path: it.Path, Library: it.LibraryID})
		}
	}
	return out, nil
}

// GET /api/diagnose/targets (admin): the folders storage checks can be pointed
// at, and the discs the libraries know.
func (s *Server) handleDiagnoseTargets(w http.ResponseWriter, r *http.Request) {
	t, err := s.diagnoseFolders(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, t)
}

// diagnoseRoots are the places a disc may be: library folders and the data folder.
func (s *Server) diagnoseRoots(ctx context.Context) (*diagnose.Roots, []diagnoseRoot, error) {
	libs, err := s.Store.Libraries(ctx)
	if err != nil {
		return nil, nil, err
	}
	var folders []string
	roots := []diagnoseRoot{}
	for _, l := range libs {
		for _, p := range l.Paths {
			folders = append(folders, p)
			roots = append(roots, diagnoseRoot{Name: l.Name + ": " + filepath.Base(p), Path: p, Kind: "library"})
		}
	}
	if s.Cfg.DataDir != "" {
		folders = append(folders, s.Cfg.DataDir)
		roots = append(roots, diagnoseRoot{Name: "Data folder", Path: s.Cfg.DataDir, Kind: "data"})
	}
	return diagnose.NewRoots(folders), roots, nil
}

type diagnoseRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"` // library or data
}

type diagnoseEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
	Disc string `json:"disc,omitempty"` // a folder holding a disc: dvd or bluray
	Kind string `json:"kind,omitempty"` // disc, for a disc image
	Size int64  `json:"size,omitempty"`
}

type diagnoseListing struct {
	Path      string          `json:"path"`
	Parent    *string         `json:"parent"` // null at the top of an allowed place
	Roots     []diagnoseRoot  `json:"roots,omitempty"`
	Entries   []diagnoseEntry `json:"entries"`
	Truncated bool            `json:"truncated,omitempty"`
}

// diagnoseImageExt: what might be a disc image. .bin and .mdf may turn out to be
// formats the engines don't read, and the check says so.
var diagnoseImageExt = map[string]bool{".iso": true, ".img": true, ".bin": true, ".udf": true, ".mdf": true, ".nrg": true}

// GET /api/diagnose/browse?path=: folders, disc folders and disc images inside
// the allowed places. With no path, the places themselves.
func (s *Server) handleDiagnoseBrowse(w http.ResponseWriter, r *http.Request) {
	roots, tops, err := s.diagnoseRoots(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		writeJSON(w, 200, diagnoseListing{Roots: tops, Entries: []diagnoseEntry{}})
		return
	}
	real, err := roots.Resolve(p)
	if err != nil {
		writeDiagnosePathErr(w, err)
		return
	}
	st, err := os.Stat(real)
	if err != nil {
		writeDiagnosePathErr(w, diagnose.ErrMissing)
		return
	}
	if !st.IsDir() {
		writeErr(w, 400, "That's a file, not a folder.")
		return
	}
	l, err := fb.List(real, fb.Options{MaxEntries: 3000})
	switch {
	case errors.Is(err, fb.ErrNotFound):
		writeDiagnosePathErr(w, diagnose.ErrMissing)
		return
	case err != nil:
		writeErr(w, 403, "The server isn't allowed to open this folder.")
		return
	}
	out := diagnoseListing{Path: l.Path, Entries: []diagnoseEntry{}, Truncated: l.Truncated}
	// Up goes as far as the top of an allowed place, and no further.
	if l.Parent != nil {
		if _, err := roots.Resolve(*l.Parent); err == nil {
			out.Parent = l.Parent
		}
	}
	for _, e := range l.Entries {
		switch {
		case e.Dir:
			out.Entries = append(out.Entries, diagnoseEntry{Name: e.Name, Path: e.Path, Dir: true, Disc: e.Disc})
		case diagnoseImageExt[strings.ToLower(filepath.Ext(e.Name))]:
			out.Entries = append(out.Entries, diagnoseEntry{Name: e.Name, Path: e.Path, Kind: "disc", Size: e.Size})
		}
	}
	writeJSON(w, 200, out)
}

func writeDiagnosePathErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, diagnose.ErrOutside), errors.Is(err, diagnose.ErrLinkOut):
		writeErr(w, 403, err.Error())
	default:
		writeErr(w, 404, err.Error())
	}
}

func writeDiagnoseQueued(w http.ResponseWriter, j *diagnose.Job, err error) {
	if err != nil {
		w.Header().Set("Retry-After", "60")
		writeErr(w, 429, err.Error())
		return
	}
	writeJSON(w, 202, j.View())
}

// POST /api/diagnose/disc {path, crawl, damage}: starts a check of a disc.
func (s *Server) handleDiagnoseDisc(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Path   string `json:"path"`
		Crawl  bool   `json:"crawl"`
		Damage bool   `json:"damage"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Path == "" {
		writeErr(w, 400, "Choose a disc first.")
		return
	}
	roots, _, err := s.diagnoseRoots(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	real, err := roots.Resolve(b.Path)
	if err != nil {
		writeDiagnosePathErr(w, err)
		return
	}
	if st, err := os.Stat(real); err == nil && st.IsDir() {
		if err := roots.CheckInnerLinks(real); err != nil {
			writeDiagnosePathErr(w, err)
			return
		}
	}
	j, err := s.diagnoser().SubmitDisc(real, diagnose.DiscOptions{Crawl: b.Crawl, Damage: b.Damage})
	writeDiagnoseQueued(w, j, err)
}

// POST /api/diagnose/storage {targets, readTest, writeTest, walk}: starts a check
// of some folders, chosen by the IDs GET /api/diagnose/targets lists.
func (s *Server) handleDiagnoseStorage(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Targets   []string `json:"targets"`
		ReadTest  bool     `json:"readTest"`
		WriteTest bool     `json:"writeTest"`
		Walk      bool     `json:"walk"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(b.Targets) == 0 {
		writeErr(w, 400, "Choose at least one folder.")
		return
	}
	all, err := s.diagnoseFolders(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	known := map[string]diagnose.StorageTarget{}
	var libraries []string
	libNo := 0
	for _, l := range all.Libraries {
		libNo++
		for i, f := range l.Folders {
			libraries = append(libraries, f.Path)
			known[f.ID] = diagnose.StorageTarget{ID: f.ID, Role: "library", Label: f.Name, Plain: "Library " + strconv.Itoa(libNo) + ", folder " + strconv.Itoa(i+1), Path: f.Path, LibraryID: l.ID}
		}
	}
	for _, f := range all.Data {
		known[f.ID] = diagnose.StorageTarget{ID: f.ID, Role: f.Role, Label: f.Name, Plain: f.Name, Path: f.Path}
	}
	targets := make([]diagnose.StorageTarget, 0, len(b.Targets))
	seen := map[string]bool{}
	for _, id := range b.Targets {
		t, ok := known[id]
		if !ok {
			writeErr(w, 400, "One of those folders isn't on the list any more. Reload the page and choose again.")
			return
		}
		if !seen[id] {
			seen[id] = true
			targets = append(targets, t)
		}
	}
	j, err := s.diagnoser().SubmitStorage(targets, diagnose.StorageOptions{ReadTest: b.ReadTest, WriteTest: b.WriteTest, Walk: b.Walk, Libraries: libraries})
	writeDiagnoseQueued(w, j, err)
}

// GET /api/diagnose/jobs: what's running and waiting, then past reports.
func (s *Server) handleDiagnoseJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"jobs": s.diagnoser().Jobs()})
}

// GET /api/diagnose/jobs/{id}: one job, with its findings.
func (s *Server) handleDiagnoseJob(w http.ResponseWriter, r *http.Request) {
	v, ok := s.diagnoser().JobView(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "No such check.")
		return
	}
	writeJSON(w, 200, v)
}

// DELETE /api/diagnose/jobs/{id}: cancels a job.
func (s *Server) handleDiagnoseCancel(w http.ResponseWriter, r *http.Request) {
	j, ok := s.diagnoser().Cancel(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "No such check is running.")
		return
	}
	writeJSON(w, 200, j.View())
}

// GET /api/diagnose/jobs/{id}/report?format=json|txt&redact=1: the report as a
// file to keep or send.
func (s *Server) handleDiagnoseReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rep, ok := s.diagnoser().ReportFor(id)
	if !ok {
		writeErr(w, 404, "No such report.")
		return
	}
	if qBool(r, "redact") {
		rep = rep.Redact()
	}
	safe := logging.ValidSession(id) // only letters, digits and a little punctuation
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Query().Get("format") == "txt" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="wittydiagnoser-`+safe+`.txt"`)
		_, _ = w.Write([]byte(rep.Text()))
		return
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="wittydiagnoser-`+safe+`.json"`)
	_, _ = w.Write(b)
}
