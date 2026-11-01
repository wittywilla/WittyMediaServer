package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/backup"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Settings, Backup: a ZIP of the server's data and metadata, and bringing one back. One
// job at a time. Bringing one back works from first-time setup too, where there's no
// account to ask for, under the same rule as making the first account: on the home
// network, or with the setup code.

func init() { secretSettings = append(secretSettings, backupKeySetting) }

const (
	backupKeySetting      = "backupKey"
	backupScheduleSetting = "backupSchedule"
	backupLastSetting     = "backupLast"
	backupLastErrSetting  = "backupLastError"
	backupLinkLife        = 10 * time.Minute
	inspectionLife        = time.Hour
	uploadPrefix          = "wms-backup-upload-"
)

// backupState is the job going, and what checking an archive found.
type backupState struct {
	mu         sync.Mutex
	job        backupJob
	cancel     context.CancelFunc
	inspection *backup.Inspection
	inspected  string
	checkedAt  time.Time
	links      map[string]backupLink
	poke       chan struct{}
	last       *backup.Result
}

type backupLink struct {
	name  string
	until time.Time
}

// backupJob is what the page shows.
type backupJob struct {
	Kind          string             `json:"kind,omitempty"` // create, inspect or restore
	Running       bool               `json:"running"`
	Phase         string             `json:"phase,omitempty"`
	Done          int64              `json:"done"`
	Total         int64              `json:"total"`
	Item          string             `json:"item,omitempty"`
	Error         string             `json:"error,omitempty"`
	NeedsPassword bool               `json:"needsPassword,omitempty"`
	Refused       bool               `json:"refused,omitempty"`
	File          string             `json:"file,omitempty"`
	Result        string             `json:"result,omitempty"`
	Problems      []string           `json:"problems,omitempty"`
	Inspection    *backup.Inspection `json:"inspection,omitempty"`
	Restarting    bool               `json:"restarting,omitempty"`
}

// backupRoutes adds the routes.
func (s *Server) backupRoutes(m *http.ServeMux) {
	m.Handle("GET /api/backup", s.adminOrFirstRun(s.handleBackupView))
	m.Handle("GET /api/backup/estimate", s.requireAdmin(s.handleBackupEstimate))
	m.Handle("POST /api/backup/create", s.requireAdmin(s.handleBackupCreate))
	m.Handle("POST /api/backup/cancel", s.adminOrFirstRun(s.handleBackupCancel))
	m.Handle("PUT /api/backup/schedule", s.requireAdmin(s.handleBackupSchedule))
	m.Handle("DELETE /api/backup/files/{name}", s.requireAdmin(s.handleBackupDelete))
	m.Handle("POST /api/backup/files/{name}/link", s.requireAdmin(s.handleBackupLink))
	m.HandleFunc("GET /api/backup/download", s.handleBackupDownload)
	m.Handle("POST /api/backup/restore/upload", s.adminOrFirstRun(s.handleBackupUpload))
	m.Handle("POST /api/backup/restore/inspect", s.adminOrFirstRun(s.handleBackupInspect))
	m.Handle("POST /api/backup/restore/start", s.adminOrFirstRun(s.handleBackupRestore))
	m.Handle("DELETE /api/backup/restore/request", s.adminOrFirstRun(s.handleBackupRestoreCancel))
	m.Handle("POST /api/backup/undo", s.requireAdmin(s.handleBackupUndo))
}

// adminOrFirstRun lets an administrator in, or, while the server has no accounts, anyone
// allowed to make the first one. Once there's an account it's administrators only.
func (s *Server) adminOrFirstRun(h http.HandlerFunc) http.Handler {
	admin := s.requireAdmin(h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Auth.UserCount(r.Context()) > 0 {
			admin.ServeHTTP(w, r)
			return
		}
		if !onLocalNetwork(r) {
			if d := s.logins.begin("setup|"+addressKey(r), freeTriesPerName); d > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
				writeErr(w, 429, "Too many wrong setup codes. Try again later.")
				return
			}
			if !s.setupCodeOK(r.Header.Get("X-Setup-Code")) {
				s.Log.WarnContext(r.Context(), "restore refused: wrong setup code", "cat", "auth", "from", addressKey(r))
				writeErr(w, 403, "Restoring from another network takes the setup code. You'll find it on the server's console.")
				return
			}
		}
		h(w, r)
	})
}

func (s *Server) backupFolders() backup.Folders {
	return backup.Folders{Data: s.Cfg.DataDir, Images: s.Cfg.ImagesDir}
}

// BackupSecretSettings are the settings a backup with no password leaves out.
func BackupSecretSettings() []string { return append([]string{}, secretSettings...) }

// NoteRestore is told how the restore at this start went.
func (s *Server) NoteRestore(r *backup.Result) {
	s.backup.mu.Lock()
	s.backup.last = r
	s.backup.mu.Unlock()
}

type backupSchedule struct {
	Frequency string   `json:"frequency"` // off, daily or weekly
	Hour      int      `json:"hour"`
	Weekday   int      `json:"weekday"`
	Keep      int      `json:"keep"`
	Parts     []string `json:"parts"`
	Protected bool     `json:"protected"`
}

func (s *Server) loadBackupSchedule(ctx context.Context) backupSchedule {
	sc := backupSchedule{Frequency: "off", Hour: 3, Keep: 7, Parts: backup.DefaultParts()}
	if v := s.DB.GetSetting(ctx, backupScheduleSetting, ""); v != "" {
		if err := json.Unmarshal([]byte(v), &sc); err != nil {
			s.Log.Warn("the backup schedule can't be read", "cat", "server", "err", err)
		}
	}
	sc.Protected = s.DB.GetSetting(ctx, backupKeySetting, "") != ""
	return sc
}

type backupView struct {
	Parts      []backup.Part       `json:"parts"`
	Files      []backup.Info       `json:"files"`
	Folder     string              `json:"folder"`
	Schedule   backupSchedule      `json:"schedule"`
	LastAuto   int64               `json:"lastAuto,omitempty"`
	LastErr    string              `json:"lastError,omitempty"`
	Job        backupJob           `json:"job"`
	Pending    *backup.Request     `json:"pending,omitempty"`
	Last       *backup.Result      `json:"last,omitempty"`
	Safety     []backup.SafetyCopy `json:"safety"`
	CanRestart bool                `json:"canRestart"`
	TwoFactor  bool                `json:"twoFactor"`
	FirstRun   bool                `json:"firstRun"`
}

// GET /api/backup: what's kept, what's scheduled and what's going on.
func (s *Server) handleBackupView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := s.backupFolders()
	st := &s.backup
	st.mu.Lock()
	job := st.job
	st.mu.Unlock()
	v := backupView{Parts: backup.Parts(), Files: backup.List(f), Folder: f.Dir(), Job: job, Pending: backup.Pending(f), Last: backup.Last(f), Safety: backup.SafetyCopies(f), CanRestart: s.Restart != nil}
	if u := userFrom(r); u != nil {
		v.TwoFactor = u.TwoFactor
		v.Schedule = s.loadBackupSchedule(ctx)
		if n, err := strconv.ParseInt(s.DB.GetSetting(ctx, backupLastSetting, "0"), 10, 64); err == nil {
			v.LastAuto = n
		}
		v.LastErr = s.DB.GetSetting(ctx, backupLastErrSetting, "")
	} else {
		v.FirstRun = true
		v.Parts, v.Folder = backup.Parts(), ""
	}
	writeJSON(w, 200, v)
}

// GET /api/backup/estimate: about how large each part is. It walks folders, so it's
// asked for apart from the page itself.
func (s *Server) handleBackupEstimate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"parts": backup.Estimate(r.Context(), s.backupFolders(), s.DB.DB)})
}

// beginBackup claims the one slot.
func (s *Server) beginBackup(kind string) (context.Context, bool) {
	st := &s.backup
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.job.Running {
		return nil, false
	}
	ctx, cancel := context.WithCancel(context.Background())
	st.cancel = cancel
	st.job = backupJob{Kind: kind, Running: true, Phase: "Starting"}
	return ctx, true
}

func (s *Server) backupProgress(t *tasks.Handle) func(backup.Step) {
	st := &s.backup
	return func(p backup.Step) {
		st.mu.Lock()
		st.job.Phase, st.job.Done, st.job.Total, st.job.Item = p.Phase, p.Done, p.Total, p.Item
		st.mu.Unlock()
		if t != nil {
			t.Phase(p.Phase)
			if p.Total > 0 {
				t.Progress(p.Done, p.Total)
			}
		}
	}
}

func (s *Server) endBackup(change func(j *backupJob)) {
	st := &s.backup
	st.mu.Lock()
	st.job.Running, st.job.Phase, st.job.Item = false, "", ""
	change(&st.job)
	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}
	st.mu.Unlock()
}

// startCreate makes a backup in the background.
func (s *Server) startCreate(parts []string, password string, key *backup.Key, auto bool, by tasks.By) error {
	ctx, ok := s.beginBackup("create")
	if !ok {
		return errBackupBusy
	}
	cancel := s.backup.cancel
	title := "Backing up"
	if auto {
		title = "Scheduled backup"
	}
	t := s.Tasks.Start(tasks.Spec{Type: tasks.TypeBackup, Title: title, Unit: "bytes", By: by, Cancel: cancel, AdminOnly: true})
	log := s.Log.With("cat", "server", "session", logging.NewID("backup"))
	logging.Go("backup", func() {
		var problems []string
		out, man, err := backup.Create(ctx, backup.Options{Folders: s.backupFolders(), Parts: parts, LiveDB: s.DB.DB, Password: password, Key: key,
			SecretSettings: BackupSecretSettings(), ServerVersion: s.Version, Auto: auto, Progress: s.backupProgress(t),
			Problem: func(file, why string) {
				problems = append(problems, file+": "+why)
				t.Problem(file, why)
			}})
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Error("a backup failed", "err", err)
			}
			if auto {
				s.setBackupSetting(backupLastErrSetting, err.Error())
			}
			t.Finish(err)
			s.endBackup(func(j *backupJob) {
				if errors.Is(err, context.Canceled) {
					j.Error = "The backup was stopped. Nothing was kept."
				} else {
					j.Error = err.Error()
				}
			})
			return
		}
		name := filepath.Base(out)
		st, _ := os.Stat(out)
		var size int64
		if st != nil {
			size = st.Size()
		}
		log.Info("a backup was made", "name", name, "bytes", size, "protected", man.Protected, "auto", auto)
		if auto {
			s.setBackupSetting(backupLastSetting, strconv.FormatInt(time.Now().Unix(), 10))
			s.setBackupSetting(backupLastErrSetting, "")
			keep := s.loadBackupSchedule(context.Background()).Keep
			if removed := backup.Prune(s.backupFolders(), keep); len(removed) > 0 {
				log.Info("old scheduled backups were removed", "count", len(removed))
			}
		}
		result := fmt.Sprintf("Backed up %d files.", len(man.Files))
		t.Result(result)
		t.Finish(nil)
		s.endBackup(func(j *backupJob) { j.File, j.Result, j.Problems = name, result, problems })
	}, "cat", "server")
	return nil
}

func (s *Server) setBackupSetting(key, value string) {
	if err := s.DB.SetSetting(context.Background(), key, value); err != nil {
		s.Log.Warn("could not save a backup setting", "cat", "server", "key", key, "err", err)
	}
}

var errBackupBusy = errors.New("a backup or restore is already running")

// backupSoonest is how long a backup that's already due waits after a start or a change.
var backupSoonest = time.Minute

// POST /api/backup/create {parts, password}
func (s *Server) handleBackupCreate(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Parts    []string `json:"parts"`
		Password string   `json:"password"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	parts, err := backup.CleanParts(b.Parts)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Password != "" && len(b.Password) < backup.MinPassword {
		writeErr(w, 400, fmt.Sprintf("use a password of at least %d characters", backup.MinPassword))
		return
	}
	if err := s.startCreate(parts, b.Password, nil, false, byUser(userFrom(r))); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, map[string]any{"started": true})
}

// POST /api/backup/cancel
func (s *Server) handleBackupCancel(w http.ResponseWriter, _ *http.Request) {
	st := &s.backup
	st.mu.Lock()
	if st.cancel != nil {
		st.cancel()
	}
	st.mu.Unlock()
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/backup/files/{name}
func (s *Server) handleBackupDelete(w http.ResponseWriter, r *http.Request) {
	if err := backup.Delete(s.backupFolders(), r.PathValue("name")); err != nil {
		writeErr(w, 404, "no such backup")
		return
	}
	s.Log.InfoContext(r.Context(), "a backup was deleted", "cat", "server", "name", r.PathValue("name"), "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/backup/files/{name}/link: a short-lived address to download it, as a backup can
// be far too large for the app to hold in memory on its way to a file.
func (s *Server) handleBackupLink(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.backupFolders().PathOf(name); err != nil {
		writeErr(w, 404, "no such backup")
		return
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	token := hex.EncodeToString(b)
	st := &s.backup
	st.mu.Lock()
	if st.links == nil {
		st.links = map[string]backupLink{}
	}
	now := time.Now()
	for k, l := range st.links {
		if now.After(l.until) {
			delete(st.links, k)
		}
	}
	st.links[token] = backupLink{name: name, until: now.Add(backupLinkLife)}
	st.mu.Unlock()
	s.Log.InfoContext(r.Context(), "a backup is being downloaded", "cat", "server", "name", name, "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"url": "/api/backup/download?st=" + token})
}

// GET /api/backup/download?st=: the file. The ticket is the permission: one backup, for a few minutes.
func (s *Server) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	st := &s.backup
	st.mu.Lock()
	l, ok := st.links[r.URL.Query().Get("st")]
	st.mu.Unlock()
	if !ok || time.Now().After(l.until) {
		writeErr(w, 401, "this download link has run out. Press Download again")
		return
	}
	p, err := s.backupFolders().PathOf(l.name)
	if err != nil {
		writeErr(w, 404, "no such backup")
		return
	}
	f, err := os.Open(p)
	if err != nil {
		writeErr(w, 404, "no such backup")
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		writeErr(w, 404, "no such backup")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+l.name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, l.name, fi.ModTime(), f)
}

// PUT /api/backup/schedule {frequency, hour, weekday, keep, parts, password, clearPassword}
func (s *Server) handleBackupSchedule(w http.ResponseWriter, r *http.Request) {
	var b struct {
		backupSchedule
		Password      string `json:"password"`
		ClearPassword bool   `json:"clearPassword"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	switch b.Frequency {
	case "off", "daily", "weekly":
	default:
		writeErr(w, 400, "choose off, daily or weekly")
		return
	}
	if b.Hour < 0 || b.Hour > 23 || b.Weekday < 0 || b.Weekday > 6 {
		writeErr(w, 400, "that isn't a time")
		return
	}
	if b.Keep < 1 || b.Keep > 365 {
		writeErr(w, 400, "keep between 1 and 365 backups")
		return
	}
	parts, err := backup.CleanParts(b.Parts)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	sc := backupSchedule{Frequency: b.Frequency, Hour: b.Hour, Weekday: b.Weekday, Keep: b.Keep, Parts: parts}
	js, err := json.Marshal(sc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	switch {
	case b.Password != "":
		key, err := backup.DeriveKey(b.Password)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		kb, err := json.Marshal(key)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if err := s.DB.SetSetting(ctx, backupKeySetting, string(kb)); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	case b.ClearPassword:
		if err := s.DB.SetSetting(ctx, backupKeySetting, ""); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if err := s.DB.SetSetting(ctx, backupScheduleSetting, string(js)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.pokeBackupSchedule()
	s.Log.InfoContext(ctx, "the backup schedule changed", "cat", "server", "frequency", b.Frequency, "by", userFrom(r).Name)
	writeJSON(w, 200, s.loadBackupSchedule(ctx))
}

func (s *Server) pokeBackupSchedule() {
	st := &s.backup
	st.mu.Lock()
	ch := st.poke
	st.mu.Unlock()
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// nextBackup says when a schedule next falls due, after a time.
func nextBackup(sc backupSchedule, last, now time.Time) (time.Time, bool) {
	if sc.Frequency != "daily" && sc.Frequency != "weekly" {
		return time.Time{}, false
	}
	base := last
	if base.IsZero() {
		base = now
	}
	at := time.Date(base.Year(), base.Month(), base.Day(), sc.Hour, 0, 0, 0, base.Location())
	for !at.After(base) || (sc.Frequency == "weekly" && int(at.Weekday()) != sc.Weekday) {
		at = at.AddDate(0, 0, 1)
	}
	return at, true
}

// StartBackupSchedule keeps the schedule. It sleeps until something is due or the
// schedule changes, so a server with none does nothing at all.
func (s *Server) StartBackupSchedule(ctx context.Context) {
	st := &s.backup
	st.mu.Lock()
	st.poke = make(chan struct{}, 1)
	poke := st.poke
	st.mu.Unlock()
	logging.Loop("backup schedule", func() {
		for {
			sc := s.loadBackupSchedule(ctx)
			var last time.Time
			if n, err := strconv.ParseInt(s.DB.GetSetting(ctx, backupLastSetting, "0"), 10, 64); err == nil && n > 0 {
				last = time.Unix(n, 0)
			}
			due, on := nextBackup(sc, last, time.Now())
			var wait <-chan time.Time
			if on {
				d := time.Until(due)
				if d < backupSoonest {
					d = backupSoonest // not all at once as the server starts
				}
				wait = time.After(d)
			}
			select {
			case <-ctx.Done():
				return
			case <-poke:
				continue
			case <-wait:
			}
			var key *backup.Key
			if v := s.DB.GetSetting(ctx, backupKeySetting, ""); v != "" {
				key = &backup.Key{}
				if err := json.Unmarshal([]byte(v), key); err != nil {
					s.Log.Error("the scheduled backup's password can't be read: no backup was made", "cat", "server", "err", err)
					s.setBackupSetting(backupLastErrSetting, "The saved password can't be read. Set it again.")
					s.setBackupSetting(backupLastSetting, strconv.FormatInt(time.Now().Unix(), 10))
					continue
				}
			}
			if err := s.startCreate(sc.Parts, "", key, true, tasks.By{}); err != nil {
				// Something else is running: look again soon.
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Minute):
				}
				continue
			}
			s.setBackupSetting(backupLastSetting, strconv.FormatInt(time.Now().Unix(), 10))
		}
	}, "cat", "server")
}

// ---------------------------------------------------------------- restoring

// POST /api/backup/restore/upload?name=: an archive from this browser, as the body. It's
// kept in the backups folder under a name of its own, and looked at when asked.
func (s *Server) handleBackupUpload(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{}) // a backup can take as long as it takes
	f := s.backupFolders()
	if err := os.MkdirAll(f.Dir(), 0o700); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if free := sysinfo.FreeBytes(f.Dir()); free > 0 && r.ContentLength > free {
		writeErr(w, 507, fmt.Sprintf("there isn't room for this file: it's %d MB, and the drive has %d MB free", r.ContentLength>>20, free>>20))
		return
	}
	name := uploadPrefix + time.Now().Format("20060102-150405") + ".zip"
	dst := filepath.Join(f.Dir(), name)
	tmp, err := os.CreateTemp(f.Dir(), ".backup-upload-*.tmp")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer func() {
		tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	_ = tmp.Chmod(0o600)
	n, err := io.Copy(tmp, r.Body)
	if err != nil {
		writeErr(w, 400, "the upload stopped part way: "+err.Error())
		return
	}
	if err := tmp.Close(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "a backup was uploaded", "cat", "server", "name", name, "bytes", n)
	writeJSON(w, 200, map[string]any{"file": name, "size": n})
}

// POST /api/backup/restore/inspect {file, password}: checks an archive from end to end,
// in the background. Nothing changes.
func (s *Server) handleBackupInspect(w http.ResponseWriter, r *http.Request) {
	var b struct {
		File     string `json:"file"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	f := s.backupFolders()
	p, err := f.PathOf(b.File)
	if err != nil {
		writeErr(w, 404, "no such backup")
		return
	}
	ctx, ok := s.beginBackup("inspect")
	if !ok {
		writeErr(w, 409, errBackupBusy.Error())
		return
	}
	st := &s.backup
	st.mu.Lock()
	cancel, old := st.cancel, st.inspection
	st.inspection, st.inspected, st.job.File = nil, "", b.File
	st.mu.Unlock()
	old.Close()
	t := s.Tasks.Start(tasks.Spec{Type: tasks.TypeBackup, Title: "Checking a backup", Unit: "bytes", By: byUser(userFrom(r)), Cancel: cancel, AdminOnly: true})
	logging.Go("backup check", func() {
		in, err := backup.Inspect(ctx, p, b.Password, f, s.backupProgress(t))
		if err != nil {
			var refusal *backup.Refusal
			refused := errors.As(err, &refusal)
			needs := backup.IsWrongPassword(err)
			if refused {
				s.Log.Warn("a backup was refused", "cat", "server", "file", b.File, "why", err.Error())
				// What was only just uploaded to be checked doesn't stay on this server.
				if strings.HasPrefix(b.File, uploadPrefix) {
					if rerr := backup.Delete(f, b.File); rerr != nil {
						s.Log.Warn("could not remove a refused upload", "cat", "server", "file", b.File, "err", rerr)
					}
				}
			}
			t.Finish(err)
			s.endBackup(func(j *backupJob) {
				j.Error, j.Refused, j.NeedsPassword = err.Error(), refused, needs
				j.Inspection = nil
			})
			return
		}
		t.Result("The backup checked out.")
		t.Finish(nil)
		st.mu.Lock()
		st.inspection, st.inspected, st.checkedAt = in, b.File, time.Now()
		st.mu.Unlock()
		s.endBackup(func(j *backupJob) {
			j.Inspection = in
			j.File = b.File
		})
		time.AfterFunc(inspectionLife, func() { s.forgetInspection(in) })
	}, "cat", "server")
	writeJSON(w, 202, map[string]any{"started": true})
}

// forgetInspection clears up one nobody went on with. An inspection holds a decrypted copy.
func (s *Server) forgetInspection(in *backup.Inspection) {
	st := &s.backup
	st.mu.Lock()
	stale := st.inspection == in && !st.job.Running
	if stale {
		st.inspection, st.inspected = nil, ""
		st.job.Inspection = nil
	}
	st.mu.Unlock()
	if stale {
		in.Close()
	}
}

// POST /api/backup/restore/start {parts, confirm: "RESTORE", password, code}. An
// administrator's password is asked for again. It's unpacked in the background, and the
// server restarts to swap it in.
func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	var b struct {
		resetBody
		Parts []string `json:"parts"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Confirm != "RESTORE" {
		writeErr(w, 400, "type RESTORE to confirm")
		return
	}
	if userFrom(r) != nil && !s.resetAllowed(w, r, b.resetBody) {
		return
	}
	st := &s.backup
	st.mu.Lock()
	in := st.inspection
	st.mu.Unlock()
	if in == nil {
		writeErr(w, 409, "check the backup first")
		return
	}
	if _, err := backup.CleanParts(b.Parts); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, ok := s.beginBackup("restore")
	if !ok {
		writeErr(w, 409, errBackupBusy.Error())
		return
	}
	st.mu.Lock()
	cancel := st.cancel
	st.mu.Unlock()
	by := userFrom(r)
	t := s.Tasks.Start(tasks.Spec{Type: tasks.TypeBackup, Title: "Restoring a backup", Unit: "bytes", By: byUser(by), Cancel: cancel, AdminOnly: true})
	logging.Go("backup restore", func() {
		_, err := in.Stage(ctx, s.backupFolders(), backup.StageOptions{Parts: b.Parts, LiveDB: s.DB.DB, Progress: s.backupProgress(t)})
		if err != nil {
			t.Finish(err)
			s.endBackup(func(j *backupJob) { j.Error = err.Error() })
			return
		}
		who := ""
		if by != nil {
			who = by.Name
		}
		s.Log.Warn("a backup is about to be restored", "cat", "server", "parts", strings.Join(b.Parts, ","), "by", who)
		t.Result("Ready. The server restarts to bring it in.")
		t.Finish(nil)
		st.mu.Lock()
		st.inspection, st.inspected = nil, ""
		st.mu.Unlock()
		in.Close()
		restart := s.Restart
		s.endBackup(func(j *backupJob) { j.Restarting = restart != nil; j.Inspection = nil })
		if restart != nil {
			time.AfterFunc(800*time.Millisecond, restart) // so the page sees this first
		}
	}, "cat", "server")
	writeJSON(w, 202, map[string]any{"started": true})
}

// DELETE /api/backup/restore/request: takes back a restore waiting for the next start.
func (s *Server) handleBackupRestoreCancel(w http.ResponseWriter, _ *http.Request) {
	found, err := backup.Cancel(s.backupFolders())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"cancelled": found})
}

// POST /api/backup/undo {copy, password, code}: puts back what a restore replaced.
func (s *Server) handleBackupUndo(w http.ResponseWriter, r *http.Request) {
	var b resetBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.resetAllowed(w, r, b) {
		return
	}
	if err := backup.AskUndo(s.backupFolders(), b.Copy); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	s.Log.WarnContext(r.Context(), "a restore is being undone", "cat", "server", "copy", b.Copy, "by", userFrom(r).Name)
	writeJSON(w, 202, map[string]any{"restarting": s.Restart != nil})
	if s.Restart != nil {
		time.AfterFunc(300*time.Millisecond, s.Restart)
	}
}
