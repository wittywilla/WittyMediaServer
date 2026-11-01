package server

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymediaserver/internal/metafolder"
)

// The editable metadata folder (Settings, Metadata files), for administrators: export
// it, say what editing it would change, apply it. The location is fixed and no route
// takes a path, so none of these can be pointed at anything else on the machine.

func (s *Server) metaFolderRoutes(m *http.ServeMux) {
	m.Handle("GET /api/metadata-folder", s.requireAdmin(s.handleMetaFolder))
	m.Handle("POST /api/metadata-folder/export", s.requireAdmin(s.handleMetaFolderExport))
	m.Handle("GET /api/metadata-folder/preview", s.requireAdmin(s.handleMetaFolderPreview))
	m.Handle("POST /api/metadata-folder/apply", s.requireAdmin(s.handleMetaFolderApply))
}

// metaFolderJob is the export running, or the last one. One thing at a time: a preview
// or an apply reading files an export is writing would be misleading.
type metaFolderJob struct {
	mu sync.Mutex
	metaFolderView
	// What was counted last, for while an export runs: the page asks often, and walking
	// a folder of many thousands of files each time would slow the export it's watching.
	files, pictures, walkProblems int
}

type metaFolderView struct {
	Running  bool                  `json:"running"`
	What     string                `json:"what,omitempty"` // export, preview or apply
	Done     int                   `json:"done"`
	Total    int                   `json:"total"`
	Finished int64                 `json:"finished,omitempty"` // ms
	Error    string                `json:"error,omitempty"`
	Result   *library.ExportResult `json:"result,omitempty"`
}

func (j *metaFolderJob) view() metaFolderView {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.metaFolderView
}

// begin claims the folder, and says no if something else has it.
func (j *metaFolderJob) begin(what string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.Running {
		return false
	}
	j.Running, j.What = true, what
	return true
}

func (j *metaFolderJob) end() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Running, j.What = false, ""
}

func (s *Server) metaFolderDir() string { return filepath.Join(s.Cfg.DataDir, metafolder.DirName) }

// GET /api/metadata-folder: where it is, what's in it, and the export's progress.
func (s *Server) handleMetaFolder(w http.ResponseWriter, r *http.Request) {
	dir := s.metaFolderDir()
	j := &s.metaFolder
	job := j.view()
	if !job.Running {
		files, pictures := 0, 0
		problems := metafolder.Walk(dir, func(e metafolder.Entry) {
			if e.Kind == "picture" {
				pictures++
			} else {
				files++
			}
		})
		j.mu.Lock()
		j.files, j.pictures, j.walkProblems = files, pictures, len(problems)
		j.mu.Unlock()
	}
	j.mu.Lock()
	files, pictures, walkProblems := j.files, j.pictures, j.walkProblems
	j.mu.Unlock()
	writeJSON(w, 200, map[string]any{
		"path": dir, "files": files, "pictures": pictures, "walkProblems": walkProblems,
		"job":    job,
		"layout": []string{"README.txt", "Movies/My Film (2001) [item-123].json", "Movies/My Film (2001) [item-123]-poster.jpg", "Shows/A Show [show-22]/show.json", "Shows/A Show [show-22]/S01E01 Pilot [item-311].json"},
	})
}

// POST /api/metadata-folder/export {library}: starts writing the files, in the background.
func (s *Server) handleMetaFolderExport(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Library int64 `json:"library"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	j := &s.metaFolder
	if !j.begin("export") {
		writeErr(w, 409, "the metadata folder is busy. Try again in a moment")
		return
	}
	j.mu.Lock()
	j.Done, j.Total, j.Error, j.Result, j.Finished = 0, 0, "", nil, 0
	j.mu.Unlock()
	log := s.Log.With("cat", "library", "session", logging.NewID("meta"))
	dir := s.metaFolderDir()
	task := s.Tasks.Start(tasks.Spec{Type: tasks.TypeMetadata, Title: "Writing metadata files", Unit: "titles", By: byUser(userFrom(r)), AdminOnly: true})
	logging.Go("metadata folder export", func() {
		res, err := s.Store.ExportMetadata(context.Background(), dir, b.Library, func(done, total int) {
			j.mu.Lock()
			j.Done, j.Total = done, total
			j.mu.Unlock()
			task.Progress(int64(done), int64(total))
		})
		task.Finish(err)
		j.mu.Lock()
		j.Running, j.What, j.Finished, j.Result = false, "", time.Now().UnixMilli(), res
		if err != nil {
			j.Error = err.Error()
		}
		j.mu.Unlock()
		if err != nil {
			log.Error("could not export the metadata folder", "err", err)
			return
		}
		log.Info("metadata folder exported", "written", res.Written, "unchanged", res.Unchanged, "kept", res.Kept)
	}, "cat", "library")
	writeJSON(w, 202, j.view())
}

// GET /api/metadata-folder/preview: what applying it would change. Nothing is written.
func (s *Server) handleMetaFolderPreview(w http.ResponseWriter, r *http.Request) {
	j := &s.metaFolder
	if !j.begin("preview") {
		writeErr(w, 409, "the metadata folder is busy. Try again in a moment")
		return
	}
	defer j.end()
	p, err := s.Store.PreviewMetadataFolder(r.Context(), s.metaFolderDir())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, p)
}

// POST /api/metadata-folder/apply: makes the changes, in one batch that can be undone.
// It finishes even if the page goes away, so a change isn't left half made.
func (s *Server) handleMetaFolderApply(w http.ResponseWriter, r *http.Request) {
	j := &s.metaFolder
	if !j.begin("apply") {
		writeErr(w, 409, "the metadata folder is busy. Try again in a moment")
		return
	}
	defer j.end()
	opts := library.FolderApply{User: userName(r), ImagesDir: s.Cfg.ImagesDir}
	if s.Scanner != nil {
		opts.Persist = s.Scanner.Persist
	}
	ctx := context.WithoutCancel(r.Context())
	res, err := s.Store.ApplyMetadataFolder(ctx, s.metaFolderDir(), opts)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.InfoContext(ctx, "metadata folder applied", "cat", "library", "applied", res.Applied, "skipped", res.Skipped, "batch", res.Batch, "by", userName(r))
	writeJSON(w, 200, res)
}
