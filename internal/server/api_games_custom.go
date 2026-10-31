package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay"
	"github.com/wittywilla/wittyplay/cores"
	"github.com/wittywilla/wittyplay/stream"
)

// Custom cores: an administrator's own libretro cores, and custom consoles to play
// with them. docs/GAMING.md has the reasoning. A core is native code that will run on
// this machine, so:
//
//   - only an adult administrator gets here, and adding one asks for the password again;
//   - it is test-loaded in a runner process, boxed in like a game, before anyone can
//     approve it, and always runs in one afterwards. This process never loads it;
//   - its SHA-256 is approved, and a file that no longer matches doesn't run again until
//     an administrator looks at it;
//   - the whole thing can be off, and is until someone turns it on.

const (
	settingCustomCores = "gamingCustomCores"
	// lockCustomCores keeps custom cores off whatever the settings say, for whoever
	// runs the machine when that isn't the same person as this server's administrator.
	lockCustomCores = "WMS_GAMING_NO_CUSTOM_CORES"

	stageLife      = time.Hour
	stagesAtOnce   = 2
	maxKeptVersion = 3
	maxAuditRows   = 5000
	maxPassword    = 4 << 10
	uploadTime     = 30 * time.Minute
)

// customRoutes registers the routes. Everything here is for administrators, and every
// route that changes anything other than turning this off checks it is turned on.
func (s *Server) customRoutes(m *http.ServeMux) {
	on := s.customOn
	m.Handle("GET /api/gaming/custom", s.requireAdmin(s.handleCustomGet))
	m.Handle("PUT /api/gaming/custom/enabled", s.requireAdmin(s.handleCustomEnable))
	m.Handle("POST /api/gaming/custom/staged/upload", s.requireAdmin(on(s.handleStageUpload)))
	m.Handle("POST /api/gaming/custom/staged/url", s.requireAdmin(on(s.handleStageURL)))
	m.Handle("POST /api/gaming/custom/staged/placed", s.requireAdmin(on(s.handleStagePlaced)))
	m.Handle("POST /api/gaming/custom/staged/{token}/approve", s.requireAdmin(on(s.handleStageApprove)))
	m.Handle("DELETE /api/gaming/custom/staged/{token}", s.requireAdmin(s.handleStageCancel))
	m.Handle("PUT /api/gaming/custom/cores/{id}", s.requireAdmin(on(s.handleCustomCorePut)))
	m.Handle("DELETE /api/gaming/custom/cores/{id}", s.requireAdmin(s.handleCustomCoreDelete))
	m.Handle("POST /api/gaming/custom/cores/{id}/reapprove", s.requireAdmin(on(s.handleCustomReapprove)))
	m.Handle("POST /api/gaming/custom/cores/{id}/rollback", s.requireAdmin(on(s.handleCustomRollback)))
	m.Handle("POST /api/gaming/custom/cores/{id}/files", s.requireAdmin(on(s.handleCustomFiles)))
	m.Handle("POST /api/gaming/custom/consoles", s.requireAdmin(on(s.handleCustomConsolePost)))
	m.Handle("PUT /api/gaming/custom/consoles/{id}", s.requireAdmin(on(s.handleCustomConsolePut)))
	m.Handle("DELETE /api/gaming/custom/consoles/{id}", s.requireAdmin(s.handleCustomConsoleDelete))
	m.Handle("PUT /api/gaming/consoles/{console}/core", s.requireAdmin(s.handleConsoleCore))
	m.Handle("GET /api/gaming/games/{id}/core", s.requireAdmin(s.handleGameCoreGet))
	m.Handle("PUT /api/gaming/games/{id}/core", s.requireAdmin(s.handleGameCorePut))
	s.publishCustomConsoles(context.Background())
}

// bigUploadPath says which routes take a body larger than the server's usual limit.
// They set their own limit once they know who's asking.
func bigUploadPath(p string) bool {
	return strings.HasSuffix(p, "/gaming/custom/staged/upload") || (strings.Contains(p, "/gaming/custom/cores/") && strings.HasSuffix(p, "/files"))
}

// customService is what the routes keep between requests.
type customService struct {
	store *cores.CustomStore

	mu        sync.Mutex
	staged    map[string]*stagedCore
	slots     chan struct{}
	verified  map[string]verifyMemo
	probeTime time.Duration     // 0 for stream.ProbeTimeout; tests shorten it
	noticed   map[string]string // a changed file already in the audit log, by core
}

type stagedCore struct {
	s        *cores.Staged
	report   CoreReport
	owner    int64
	replaces string
}

type verifyMemo struct {
	size  int64
	mod   time.Time
	sum   string
	state cores.CoreFileState
}

func newCustomService(dir, system string, placed []string, client *http.Client) *customService {
	st := &cores.CustomStore{Dir: dir, System: system, Placed: placed, Client: client}
	st.ClearStaging()
	return &customService{store: st, staged: map[string]*stagedCore{}, slots: make(chan struct{}, stagesAtOnce), verified: map[string]verifyMemo{}, noticed: map[string]string{}}
}

// customFetchClient only reaches the internet. It's the server's safe client, which
// checks each address as it connects so a name can't resolve its way somewhere
// private, but with no proxy: through a proxy that check would only see the proxy.
func customFetchClient() *http.Client {
	tr := library.SafeClient.Transport.(*http.Transport).Clone()
	tr.Proxy = nil
	return &http.Client{Transport: tr}
}

// ---- whether this is on ----

func customLocked() bool { return os.Getenv(lockCustomCores) == "1" }

func (s *Server) customCoresOn(ctx context.Context) bool {
	if _, sandboxed := stream.SandboxStrength(); !sandboxed || customLocked() {
		return false
	}
	return s.DB.GetSetting(ctx, settingCustomCores, "0") == "1"
}

// customOn lets a request through while custom cores are on. Kids and guest profiles
// are never administrators; this says so again, so a change elsewhere can't quietly
// open this up.
func (s *Server) customOn(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u := userFrom(r); u.Kind != auth.KindAdult && u.Kind != "" {
			writeErr(w, 403, "admin only")
			return
		}
		if !s.customCoresOn(r.Context()) {
			writeErr(w, 409, "Custom cores are turned off on this server. Turn them on in Settings, Gaming.")
			return
		}
		h(w, r)
	}
}

// customPassword is checkOwnPassword, with a refusal written down.
func (s *Server) customPassword(w http.ResponseWriter, r *http.Request, password, core string) bool {
	u := userFrom(r)
	if s.checkOwnPassword(w, r, u, password) {
		return true
	}
	s.auditCore(r.Context(), u, "password-refused", core, r.Method+" "+r.URL.Path)
	return false
}

// PUT /api/gaming/custom/enabled {enabled, password}. Turning this on takes the
// password; turning it off, which only ever makes things safer, doesn't.
func (s *Server) handleCustomEnable(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Enabled  bool   `json:"enabled"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	if b.Enabled {
		if customLocked() {
			writeErr(w, 403, "Whoever runs this machine has kept custom cores off with "+lockCustomCores+".")
			return
		}
		if why, sandboxed := stream.SandboxStrength(); !sandboxed {
			writeErr(w, 403, "Custom cores can't be used on this system: the only thing boxing a core in here would be "+why+".")
			return
		}
		if !s.customPassword(w, r, b.Password, "") {
			return
		}
	}
	val, action := "0", "turned-off"
	if b.Enabled {
		val, action = "1", "turned-on"
	}
	if err := s.DB.SetSetting(ctx, settingCustomCores, val); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.auditCore(ctx, userFrom(r), action, "", "")
	s.publishCustomConsoles(ctx)
	writeJSON(w, 200, map[string]any{"enabled": b.Enabled})
}

// ---- the audit log ----

// CoreAudit is one thing that happened to a custom core.
type CoreAudit struct {
	At     int64  `json:"at"`
	User   string `json:"user"`
	Action string `json:"action"`
	Core   string `json:"core,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// auditCore writes down something that happened, and also logs it, so it reaches the
// log files and survives someone clearing the database.
func (s *Server) auditCore(ctx context.Context, u *auth.User, action, core, detail string) {
	name := "the server"
	if u != nil {
		name = u.Name
	}
	if len(detail) > 800 {
		detail = detail[:800]
	}
	// Written with its own context: a request that has gone away still happened.
	bg := context.WithoutCancel(ctx)
	if _, err := s.DB.ExecContext(bg, `INSERT INTO game_core_audit(at, user_name, action, core, detail) VALUES(?,?,?,?,?)`, time.Now().Unix(), name, action, core, detail); err != nil {
		s.Log.Warn("custom core audit", "err", err)
	}
	_, _ = s.DB.ExecContext(bg, `DELETE FROM game_core_audit WHERE id <= (SELECT MAX(id) FROM game_core_audit) - ?`, maxAuditRows)
	s.Log.Warn("custom core: "+action, "cat", "gaming", "user", name, "core", core, "detail", detail)
}

func (s *Server) coreAudit(ctx context.Context, limit int) []CoreAudit {
	out := []CoreAudit{}
	rows, err := s.DB.QueryContext(ctx, `SELECT at, user_name, action, core, detail FROM game_core_audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var a CoreAudit
		if rows.Scan(&a.At, &a.User, &a.Action, &a.Core, &a.Detail) == nil {
			out = append(out, a)
		}
	}
	return out
}

// ---- what's stored about a core ----

// CoreReport is what a test-load and the file's own headers said. Everything in it
// came from the file, so it's shown as what the core says, never as fact.
type CoreReport struct {
	ID            string           `json:"id"` // the name this server would use
	File          string           `json:"file"`
	Size          int64            `json:"size"`
	SHA256        string           `json:"sha256"`
	Binary        cores.Binary     `json:"binary"`
	Arch          string           `json:"arch"`
	Name          string           `json:"name"`
	Version       string           `json:"version"`
	Extensions    []string         `json:"extensions"`
	NeedFullPath  bool             `json:"needFullPath,omitempty"`
	BlockExtract  bool             `json:"blockExtract,omitempty"`
	NoGame        bool             `json:"noGame,omitempty"`
	Options       int              `json:"options,omitempty"`
	NeedsGPU      string           `json:"needsGpu"` // yes | maybe | no
	GPUWhy        string           `json:"gpuWhy,omitempty"`
	BIOS          []cores.Firmware `json:"bios"`
	BIOSKnown     bool             `json:"biosKnown"` // an info file said which, so its absence means something
	License       string           `json:"license"`
	LicenseSource string           `json:"licenseSource,omitempty"` // info | table
	NonCommercial bool             `json:"nonCommercial,omitempty"`
	HasInfo       bool             `json:"hasInfo,omitempty"`
	System        string           `json:"system,omitempty"` // the system its info file names
	Notes         string           `json:"notes,omitempty"`
	Assets        int              `json:"assets,omitempty"`
	SystemFiles   int              `json:"systemFiles,omitempty"`
	Ignored       []string         `json:"ignored,omitempty"`
	SandboxNote   string           `json:"sandboxNote,omitempty"`
	Warnings      []string         `json:"warnings"`
	Consoles      []string         `json:"consoles"` // consoles its extensions say it may be for
	Source        string           `json:"source"`
	SourceURL     string           `json:"sourceUrl,omitempty"`
}

// CustomCore is a core somebody approved, as the settings page sees it.
type CustomCore struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Version       string              `json:"version"`
	SHA256        string              `json:"sha256"`
	Size          int64               `json:"size"`
	License       string              `json:"license"`
	LicenseSource string              `json:"licenseSource,omitempty"` // info | table | admin
	NonCommercial bool                `json:"nonCommercial,omitempty"`
	Consoles      []string            `json:"consoles"`
	DefaultFor    []string            `json:"defaultFor"`
	Note          string              `json:"note,omitempty"`
	Source        string              `json:"source"`
	SourceURL     string              `json:"sourceUrl,omitempty"`
	AddedBy       string              `json:"addedBy"`
	AddedAt       int64               `json:"addedAt"`
	UpdatedAt     int64               `json:"updatedAt"`
	Report        CoreReport          `json:"report"`
	State         cores.CoreFileState `json:"state"`
	Versions      []CoreVersion       `json:"versions"`
	Plays         CorePlays           `json:"plays"`
	Running       int                 `json:"running"`

	systemFiles []string
}

// CoreVersion is a version kept for going back to.
type CoreVersion struct {
	SHA256  string `json:"sha256"`
	Version string `json:"version"`
	AddedAt int64  `json:"addedAt"`
	Current bool   `json:"current"`
}

// CorePlays is how a core has really gone, across every game played with it.
type CorePlays struct {
	Starts    int     `json:"starts"`
	Failures  int     `json:"failures"`
	Seconds   float64 `json:"seconds"`
	LastError string  `json:"lastError,omitempty"`
	LastAt    int64   `json:"lastAt,omitempty"`
}

const customCoreCols = `id, name, version, sha256, size, license, license_source, non_commercial, consoles, note, source, source_url, added_by, added_at, updated_at, report, system_files`

func scanCustomCore(row interface{ Scan(...any) error }) (*CustomCore, error) {
	var c CustomCore
	var consoles, report, system string
	var nc int
	if err := row.Scan(&c.ID, &c.Name, &c.Version, &c.SHA256, &c.Size, &c.License, &c.LicenseSource, &nc, &consoles, &c.Note, &c.Source, &c.SourceURL, &c.AddedBy, &c.AddedAt, &c.UpdatedAt, &report, &system); err != nil {
		return nil, err
	}
	c.NonCommercial = nc != 0
	_ = json.Unmarshal([]byte(consoles), &c.Consoles)
	_ = json.Unmarshal([]byte(report), &c.Report)
	_ = json.Unmarshal([]byte(system), &c.systemFiles)
	if c.Consoles == nil {
		c.Consoles = []string{}
	}
	return &c, nil
}

func (s *Server) customCoreRows(ctx context.Context) []*CustomCore {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+customCoreCols+` FROM custom_cores ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*CustomCore
	for rows.Next() {
		if c, err := scanCustomCore(rows); err == nil {
			out = append(out, c)
		}
	}
	return out
}

func (s *Server) customCoreRow(ctx context.Context, id string) (*CustomCore, error) {
	return scanCustomCore(s.DB.QueryRowContext(ctx, `SELECT `+customCoreCols+` FROM custom_cores WHERE id = ?`, id))
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

func (s *Server) saveCustomCore(ctx context.Context, c *CustomCore) error {
	if c.Consoles == nil {
		c.Consoles = []string{}
	}
	nc := 0
	if c.NonCommercial {
		nc = 1
	}
	c.UpdatedAt = time.Now().Unix()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO custom_cores(`+customCoreCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, version=excluded.version, sha256=excluded.sha256, size=excluded.size, license=excluded.license,
		license_source=excluded.license_source, non_commercial=excluded.non_commercial, consoles=excluded.consoles, note=excluded.note, source=excluded.source,
		source_url=excluded.source_url, added_by=excluded.added_by, updated_at=excluded.updated_at, report=excluded.report, system_files=excluded.system_files`,
		c.ID, c.Name, c.Version, c.SHA256, c.Size, c.License, c.LicenseSource, nc, toJSON(c.Consoles), c.Note, c.Source, c.SourceURL, c.AddedBy, c.AddedAt, c.UpdatedAt, toJSON(c.Report), toJSON(c.systemFiles))
	return err
}

// customCoreList is what the core manager adds to its list: approved cores, when
// they're on. A core file that has changed stays listed, so the refusal can say why.
func (s *Server) customCoreList() []cores.Core {
	ctx := context.Background()
	if !s.customCoresOn(ctx) {
		return nil
	}
	store := s.games().custom.store
	var out []cores.Core
	for _, c := range s.customCoreRows(ctx) {
		out = append(out, cores.Core{ID: c.ID, Name: c.Name, License: c.License, LicenseSource: c.LicenseSource, Source: cores.SourceCustom, SHA256: c.SHA256,
			NonCommercial: c.NonCommercial, Version: c.Version, Consoles: c.Consoles, Installed: c.AddedAt,
			Path: store.CorePath(c.ID, c.SHA256), Assets: store.AssetsPath(c.ID, c.SHA256)})
	}
	return out
}

// customState is a core's file against what was approved. The settings page asks
// often, so it only reads the whole file when the file's size or time has moved. Play
// always reads it all (checkCustomCore).
func (s *Server) customState(ctx context.Context, c *CustomCore) cores.CoreFileState {
	cs := s.games().custom
	p := cs.store.CorePath(c.ID, c.SHA256)
	st, err := os.Lstat(p)
	if err != nil {
		return cores.CoreFileState{State: "missing"}
	}
	cs.mu.Lock()
	memo, ok := cs.verified[c.ID]
	cs.mu.Unlock()
	if ok && memo.sum == c.SHA256 && memo.size == st.Size() && memo.mod.Equal(st.ModTime()) {
		return memo.state
	}
	state := cs.store.Verify(c.ID, c.SHA256)
	cs.mu.Lock()
	cs.verified[c.ID] = verifyMemo{size: st.Size(), mod: st.ModTime(), sum: c.SHA256, state: state}
	cs.mu.Unlock()
	s.noticeChange(ctx, c, state)
	return state
}

// noticeChange writes a changed file in the audit log, once for each change.
func (s *Server) noticeChange(ctx context.Context, c *CustomCore, state cores.CoreFileState) {
	if state.State == "ok" {
		return
	}
	cs := s.games().custom
	key := state.State + ":" + state.Found
	cs.mu.Lock()
	seen := cs.noticed[c.ID] == key
	cs.noticed[c.ID] = key
	cs.mu.Unlock()
	if !seen {
		s.auditCore(ctx, nil, "file-"+state.State, c.ID, "approved "+c.SHA256+", found "+state.Found)
	}
}

// checkCustomCore is the last thing between an approved file and a game starting. It
// reads the whole file, every time: on a low-end machine that's a moment against a
// game's load, and it's the promise this feature makes.
func (s *Server) checkCustomCore(ctx context.Context, core *cores.Core) error {
	c, err := s.customCoreRow(ctx, core.ID)
	if err != nil {
		return errors.New("this core isn't approved")
	}
	state := s.games().custom.store.Verify(c.ID, c.SHA256)
	if state.State == "ok" {
		return nil
	}
	s.noticeChange(ctx, c, state)
	return fmt.Errorf("The core %s has changed since it was approved, so it won't start. An administrator can look at it in Settings, Gaming.", c.Name)
}

// ---- which core plays what ----

func (s *Server) gameCoreChoice(ctx context.Context, itemID int64) string {
	var id string
	_ = s.DB.QueryRowContext(ctx, `SELECT core_id FROM game_core_choice WHERE item_id = ?`, itemID).Scan(&id)
	return id
}

// preferredCore decides what to ask the core manager for: this game's own choice, then
// the console's, then, when nothing in the table is here, the first custom core
// assigned to the console. Only cores from the table or approved and assigned for this
// console count, so no setting can reach a file nobody approved.
func (s *Server) preferredCore(ctx context.Context, console *wittyplay.Console, itemID int64) string {
	mgr := s.games().cores
	all := mgr.List()
	usable := func(id string) bool {
		for i := range all {
			if all[i].ID != id {
				continue
			}
			if all[i].Unapproved {
				return false
			}
			return all[i].Source != cores.SourceCustom || slices.Contains(all[i].Consoles, console.ID) || consoleListsCore(console, id)
		}
		return false
	}
	if id := s.gameCoreChoice(ctx, itemID); id != "" && usable(id) {
		return id
	}
	if id := s.preferredCores(ctx)[console.ID]; id != "" && usable(id) {
		return id
	}
	if mgr.ForConsole(console, "") != nil {
		return ""
	}
	for i := range all {
		if all[i].Source == cores.SourceCustom && slices.Contains(all[i].Consoles, console.ID) {
			return all[i].ID
		}
	}
	return ""
}

func consoleListsCore(c *wittyplay.Console, id string) bool {
	for _, x := range c.Cores {
		if x.ID == id {
			return true
		}
	}
	return false
}

// customFor says whether an approved custom core has been assigned to a console. That
// is an administrator's choice to try it, which is what lifts a console's "not playable
// yet": they know more about their core than our table does.
func (s *Server) customFor(ctx context.Context, console string) bool {
	if !s.customCoresOn(ctx) {
		return false
	}
	for _, c := range s.customCoreRows(ctx) {
		if slices.Contains(c.Consoles, console) {
			return true
		}
	}
	return false
}

// ---- how play went ----

// noteCorePlay adds to what's known about a core on a game. Every core is counted, the
// table's too: it's how "works best" can mean something.
func (s *Server) noteCorePlay(ctx context.Context, core string, item int64, starts, failures int, seconds float64, lastErr string) {
	if core == "" {
		return
	}
	if len(lastErr) > 400 {
		lastErr = lastErr[:400]
	}
	_, err := s.DB.ExecContext(context.WithoutCancel(ctx), `INSERT INTO game_core_plays(core_id, item_id, starts, failures, seconds, last_error, last_at) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(core_id, item_id) DO UPDATE SET starts = starts + excluded.starts, failures = failures + excluded.failures, seconds = seconds + excluded.seconds,
		last_error = CASE WHEN excluded.last_error != '' THEN excluded.last_error ELSE last_error END, last_at = excluded.last_at`,
		core, item, starts, failures, seconds, lastErr, time.Now().Unix())
	if err != nil {
		s.Log.Warn("recording how a core went", "err", err)
	}
}

// watchCorePlay waits for a game to end and notes how it did.
func (s *Server) watchCorePlay(sess *stream.Session, core *cores.Core, u *auth.User, item int64) {
	start := time.Now()
	<-sess.Done()
	ctx := context.Background()
	fail := sess.Failure()
	failures := 0
	if fail != "" {
		failures = 1
		if core.Source == cores.SourceCustom {
			s.auditCore(ctx, u, "use-failed", core.ID, fmt.Sprintf("item %d, after %d seconds: %s", item, int(time.Since(start).Seconds()), fail))
		}
	}
	s.noteCorePlay(ctx, core.ID, item, 0, failures, time.Since(start).Seconds(), fail)
}

func (s *Server) corePlayTotals(ctx context.Context) map[string]CorePlays {
	out := map[string]CorePlays{}
	rows, err := s.DB.QueryContext(ctx, `SELECT core_id, SUM(starts), SUM(failures), SUM(seconds), MAX(last_at) FROM game_core_plays GROUP BY core_id`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var p CorePlays
		if rows.Scan(&id, &p.Starts, &p.Failures, &p.Seconds, &p.LastAt) == nil {
			out[id] = p
		}
	}
	return out
}

// ---- GET /api/gaming/custom ----

// CustomConsoleView is a custom console as it can be edited or exported.
type CustomConsoleView struct {
	wittyplay.CustomConsole
	Games     int `json:"games"`
	Libraries int `json:"libraries"`
}

// CustomStatus is everything the settings page needs for this section.
type CustomStatus struct {
	Enabled     bool                `json:"enabled"`
	Locked      bool                `json:"locked,omitempty"`
	CanEnable   bool                `json:"canEnable"`
	Sandbox     string              `json:"sandbox"`
	HaveRunner  bool                `json:"haveRunner"`
	Platform    string              `json:"platform"`
	LibraryExt  string              `json:"libraryExt"`
	MaxUploadMB int64               `json:"maxUploadMB"`
	CoresDir    string              `json:"coresDir"`
	SystemDir   string              `json:"systemDir"`
	Cores       []*CustomCore       `json:"cores"`
	Consoles    []CustomConsoleView `json:"consoles"`
	Placed      []cores.PlacedFile  `json:"placed"`
	Audit       []CoreAudit         `json:"audit"`
	PadChoices  []CorePickOption    `json:"padChoices"`
	OtherCores  []CorePickOption    `json:"otherCores"` // cores already here a custom console could use
}

// CorePickOption is something to choose from a list.
type CorePickOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Server) handleCustomGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g := s.games()
	why, sandboxed := stream.SandboxStrength()
	out := CustomStatus{
		Enabled: s.customCoresOn(ctx), Locked: customLocked(), CanEnable: sandboxed && !customLocked(), Sandbox: why, HaveRunner: s.probeRunner() != "",
		Platform: cores.Binary{OS: runtime.GOOS, Arch: runtime.GOARCH}.Describe(), LibraryExt: cores.LibraryExt(), MaxUploadMB: cores.DefaultLimits.Package >> 20,
		CoresDir: g.cores.Dir, SystemDir: g.systemDir(), Cores: []*CustomCore{}, Consoles: []CustomConsoleView{}, Placed: []cores.PlacedFile{}, Audit: s.coreAudit(ctx, 100),
	}
	totals := s.corePlayTotals(ctx)
	prefs := s.preferredCores(ctx)
	for _, c := range s.customCoreRows(ctx) {
		c.State = s.customState(ctx, c)
		c.Versions = s.coreVersions(ctx, c)
		c.Plays = totals[c.ID]
		c.Running = g.mgr.UsingCore(g.custom.store.CorePath(c.ID, c.SHA256))
		c.DefaultFor = []string{}
		for console, id := range prefs {
			if id == c.ID {
				c.DefaultFor = append(c.DefaultFor, console)
			}
		}
		sort.Strings(c.DefaultFor)
		out.Cores = append(out.Cores, c)
	}
	out.Consoles = s.customConsoleViews(ctx)
	if out.Enabled {
		out.Placed = s.placedFiles()
	}
	for _, c := range wittyplay.Consoles {
		if !c.Hidden {
			out.PadChoices = append(out.PadChoices, CorePickOption{c.ID, c.Name})
			for _, core := range c.Cores {
				if !slices.ContainsFunc(out.OtherCores, func(o CorePickOption) bool { return o.ID == core.ID }) {
					out.OtherCores = append(out.OtherCores, CorePickOption{core.ID, core.Name})
				}
			}
		}
	}
	writeJSON(w, 200, out)
}

// placedFiles is what's waiting in the cores folders. A core this server installed
// itself isn't waiting for anything.
func (s *Server) placedFiles() []cores.PlacedFile {
	g := s.games()
	installed := map[string]bool{}
	for _, c := range g.cores.List() {
		if c.SHA256 != "" && c.Source != cores.SourceCustom {
			installed[filepath.Base(c.Path)] = true
		}
	}
	out := []cores.PlacedFile{}
	for _, p := range g.custom.store.ListPlaced() {
		if !(p.Ours && installed[p.Name]) {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) coreVersions(ctx context.Context, c *CustomCore) []CoreVersion {
	out := []CoreVersion{}
	rows, err := s.DB.QueryContext(ctx, `SELECT sha256, version, added_at FROM custom_core_versions WHERE core_id = ? ORDER BY added_at DESC, rowid DESC`, c.ID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var v CoreVersion
		if rows.Scan(&v.SHA256, &v.Version, &v.AddedAt) == nil {
			v.Current = v.SHA256 == c.SHA256
			out = append(out, v)
		}
	}
	return out
}

// ---- adding a core: staging ----

// probeRunner is always our own runner, whichever one plays games: a test-load is a
// question it knows how to answer, and it needs no graphics card.
func (s *Server) probeRunner() string {
	if p := os.Getenv("WMS_WITTYPLAY_RUNNER"); p != "" {
		return p
	}
	return findProgram("wittyplay-host")
}

func (cs *customService) takeSlot() bool {
	select {
	case cs.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (cs *customService) freeSlot() { <-cs.slots }

const stageBusy = "Another core is being tested right now. Try again when it's finished."

// stageError answers for a file that was refused, and writes it down.
func (s *Server) stageError(w http.ResponseWriter, r *http.Request, err error, name string) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		s.auditCore(r.Context(), userFrom(r), "add-refused", name, "too large")
		writeErr(w, 413, "This file is larger than a core may be.")
		return
	}
	if r.Context().Err() != nil {
		return
	}
	s.auditCore(r.Context(), userFrom(r), "add-refused", name, err.Error())
	writeErr(w, 422, sentence(err.Error()))
}

// sentence makes a message from the file checks read as one: they're written to be
// joined onto others, and this one stands alone on the page.
func sentence(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.ContainsAny(s[len(s)-1:], ".!?") {
		s += "."
	}
	return s
}

// finishStage is what every route ends with: test-load the file, build the report, and
// hold it for approval.
func (s *Server) finishStage(w http.ResponseWriter, r *http.Request, st *cores.Staged, replaces, expect string) {
	ctx := r.Context()
	cs := s.games().custom
	u := userFrom(r)
	if expect = strings.ToLower(strings.TrimSpace(expect)); expect != "" && expect != st.SHA256 && expect != st.DownloadSHA {
		cs.store.Discard(st)
		s.auditCore(ctx, u, "add-refused", st.OrigName, "not the SHA-256 that was expected: "+st.SHA256)
		writeErr(w, 422, "This isn't the file you expected: its SHA-256 is "+st.SHA256+".")
		return
	}
	if replaces != "" {
		if _, err := s.customCoreRow(ctx, replaces); err != nil {
			cs.store.Discard(st)
			writeErr(w, 404, "the core being replaced isn't here")
			return
		}
	}
	res, err := s.testLoad(ctx, st.Library, filepath.Join(st.Dir, "box"))
	if err != nil {
		cs.store.Discard(st)
		var pe *stream.ProbeError
		kind := "refused"
		if errors.As(err, &pe) {
			kind = pe.Kind
		}
		s.auditCore(ctx, u, "test-load-failed", st.OrigName, kind+": "+err.Error()+" sha256 "+st.SHA256)
		writeJSON(w, 422, map[string]any{"error": err.Error(), "kind": kind, "sha256": st.SHA256})
		return
	}
	report := s.buildReport(ctx, st, res, replaces)
	cs.mu.Lock()
	for token, old := range cs.staged {
		if time.Since(old.s.Created) > stageLife {
			cs.store.Discard(old.s)
			delete(cs.staged, token)
		}
	}
	cs.staged[st.Token] = &stagedCore{s: st, report: report, owner: u.ID, replaces: replaces}
	cs.mu.Unlock()
	s.auditCore(ctx, u, "test-load-passed", report.ID, report.Name+" "+report.Version+", sha256 "+st.SHA256)
	writeJSON(w, 201, map[string]any{"token": st.Token, "report": report, "replaces": replaces})
}

// testLoad runs a core file in the sandbox. box is a folder for it to write in, and
// the only one it can.
func (s *Server) testLoad(ctx context.Context, core, box string) (*stream.ProbeResult, error) {
	res, err := stream.Probe(ctx, stream.ProbeOptions{Runner: s.probeRunner(), Core: core, SystemDir: s.games().systemDir(), WorkDir: box, Timeout: s.games().custom.probeTime, Log: s.Log})
	if err != nil {
		return nil, err
	}
	res.Probe = cores.CleanProbe(res.Probe)
	return res, nil
}

// buildReport brings together what the file, its info file and its test-load say.
func (s *Server) buildReport(ctx context.Context, st *cores.Staged, res *stream.ProbeResult, replaces string) CoreReport {
	r := CoreReport{
		File: cores.CleanText(st.OrigName, 200), Size: st.Size, SHA256: st.SHA256, Binary: st.Binary, Arch: st.Binary.Describe(), Name: res.Name, Version: res.Version,
		Extensions: res.Extensions, NeedFullPath: res.NeedFullPath, BlockExtract: res.BlockExtract, NoGame: res.NoGame, Options: res.Options, BIOS: []cores.Firmware{},
		HasInfo: st.HasInfo, Assets: st.Assets, SystemFiles: st.System, Ignored: st.Ignored, SandboxNote: res.SandboxNote, Warnings: []string{}, Source: st.Source, SourceURL: st.SourceURL,
	}
	if r.Name == "" {
		r.Warnings = append(r.Warnings, "It loaded, but doesn't give a name for itself.")
		r.Name = strings.TrimSuffix(r.File, filepath.Ext(r.File))
	}
	r.ID = cores.SuggestID(st.OrigName, res.Name)
	if replaces != "" {
		r.ID = replaces
	}
	r.NeedsGPU = "no"
	switch {
	case res.HWRender:
		r.NeedsGPU, r.GPUWhy = "yes", "It asked for a graphics card while loading."
	case st.Info != nil && st.Info.HWRender:
		r.NeedsGPU, r.GPUWhy = "yes", "Its info file says it wants a graphics card."
	case st.GPUHint:
		r.NeedsGPU, r.GPUWhy = "maybe", "It has graphics card code, though it may only use it when one is there. This can only be known once a game is running."
	}
	if r.NeedsGPU == "yes" {
		r.Warnings = append(r.Warnings, gpuNote)
	}
	if st.Info != nil {
		r.BIOSKnown, r.System, r.Notes = true, st.Info.System, st.Info.Notes
		for _, f := range st.Info.Firmware {
			native := filepath.FromSlash(f.File)
			if filepath.IsLocal(native) {
				if _, err := os.Lstat(filepath.Join(s.games().systemDir(), native)); err == nil {
					f.Present = true
				}
			}
			r.BIOS = append(r.BIOS, f)
		}
		if len(st.Info.Extensions) > 0 && len(res.Extensions) > 0 && len(intersect(st.Info.Extensions, res.Extensions)) == 0 {
			r.Warnings = append(r.Warnings, "Its info file doesn't seem to belong to it: the file types it lists aren't the ones the core reports.")
		}
		if st.Info.CoreName != "" && res.Name != "" && !strings.EqualFold(st.Info.CoreName, res.Name) && !looselySame(st.Info.CoreName, res.Name) {
			r.Warnings = append(r.Warnings, fmt.Sprintf("Its info file says it's for %q, and the core says it's %q.", st.Info.CoreName, res.Name))
		}
	}
	if len(res.Extensions) == 0 {
		r.Warnings = append(r.Warnings, "It doesn't say which files it opens, so it can't be matched to a console for you.")
	}
	if len(st.Ignored) > 0 {
		r.Warnings = append(r.Warnings, "Some files in the package aren't used: only the core, its .info file, and assets/ and system/ folders are unpacked.")
	}
	r.License, r.LicenseSource, r.NonCommercial = coreLicense(r.ID, st.OrigName, res.Name, st.Info, "")
	r.Consoles = suggestConsoles(res.Extensions, s.testingGames())
	return r
}

// gpuNote says what a core asking for a graphics card can expect. It stays in one place
// because what the runner can do is what decides it.
const gpuNote = "It asked for a graphics card. The built-in runner plays software-drawn cores, and refuses that request, so this core may fall back to software, or may not start."

func intersect(a, b []string) []string {
	var out []string
	for _, x := range a {
		if slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

var notWord = regexp.MustCompile(`[^a-z0-9]+`)

func looselySame(a, b string) bool {
	x, y := notWord.ReplaceAllString(strings.ToLower(a), ""), notWord.ReplaceAllString(strings.ToLower(b), "")
	return strings.Contains(x, y) || strings.Contains(y, x)
}

// coreLicense decides what to say about a core's licence: what the administrator
// typed, else what its info file says, else what our table says about the name. It's
// never more than a claim, so the page says where it came from. Non-commercial is said
// if anything at all points that way, because that one matters most when it's wrong.
func coreLicense(id, file, reported string, info *cores.Info, typed string) (license, source string, nonCommercial bool) {
	base := strings.TrimSuffix(id, "-custom")
	stem := strings.TrimSuffix(strings.TrimSuffix(file, filepath.Ext(file)), "_libretro")
	switch {
	case typed != "":
		license, source = typed, "admin"
	case info != nil && info.License != "":
		license, source = info.License, "info"
	default:
		if known, ok := wittyplay.CoreLicense(base); ok {
			license, source = known.License, "table"
		}
	}
	nonCommercial = wittyplay.IsNonCommercial(base) || wittyplay.IsNonCommercial(stem) || wittyplay.NonCommercialName(reported) ||
		wittyplay.NonCommercialName(stem) || wittyplay.NonCommercialLicense(license)
	if nonCommercial && license == "" {
		license, source = "Non-commercial", "table"
	}
	return license, source, nonCommercial
}

// suggestConsoles finds the consoles only this core's file types point to.
func suggestConsoles(exts []string, testing bool) []string {
	out := []string{}
	all := wittyplay.All()
	for i := range all {
		c := &all[i]
		if c.Hidden && !testing {
			continue
		}
		for _, e := range exts {
			if slices.Contains(c.Extensions, "."+e) {
				out = append(out, c.ID)
				break
			}
		}
	}
	return out
}

// readFormValue reads one small field of a multipart form.
func readFormValue(part *multipart.Part) (string, error) {
	b, err := io.ReadAll(io.LimitReader(part, maxPassword+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxPassword {
		return "", errors.New("a form field is too long")
	}
	return string(b), nil
}

// rawFileName is the name a client sent. part.FileName tidies away any folders, and
// someone sending a name with folders in it is something this code wants to see.
func rawFileName(part *multipart.Part) string {
	_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if err != nil {
		return ""
	}
	return params["filename"]
}

// readUpload checks the password, which has to come first so nothing is read from
// anyone who doesn't have it, and returns the fields before the file and the file.
func (s *Server) readUpload(w http.ResponseWriter, r *http.Request, core string) (fields map[string]string, file *multipart.Part, ok bool) {
	limits := cores.DefaultLimits
	if st := s.games().custom.store.Limits; st.Package > 0 {
		limits = st
	}
	r.Body = http.MaxBytesReader(w, r.Body, limits.Package+1<<20)
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadTime))
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, 400, "expected a multipart upload")
		return nil, nil, false
	}
	part, err := mr.NextPart()
	if err != nil || part.FormName() != "password" {
		writeErr(w, 400, "the password has to come first in the form")
		return nil, nil, false
	}
	password, err := readFormValue(part)
	part.Close()
	if err != nil {
		writeErr(w, 400, err.Error())
		return nil, nil, false
	}
	if !s.customPassword(w, r, password, core) {
		return nil, nil, false
	}
	fields = map[string]string{}
	for {
		part, err := mr.NextPart()
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				s.stageError(w, r, err, core)
			} else {
				writeErr(w, 400, "the form has no file in it")
			}
			return nil, nil, false
		}
		if part.FormName() == "file" {
			return fields, part, true
		}
		v, err := readFormValue(part)
		part.Close()
		if err != nil {
			writeErr(w, 400, err.Error())
			return nil, nil, false
		}
		fields[part.FormName()] = v
	}
}

// POST /api/gaming/custom/staged/upload: multipart, with password first, then optional
// replaces and sha256, then file.
func (s *Server) handleStageUpload(w http.ResponseWriter, r *http.Request) {
	cs := s.games().custom
	if !cs.takeSlot() {
		writeErr(w, 429, stageBusy)
		return
	}
	defer cs.freeSlot()
	fields, part, ok := s.readUpload(w, r, "")
	if !ok {
		return
	}
	defer part.Close()
	name := rawFileName(part)
	st, err := cs.store.StageReader(part, name)
	if err != nil {
		s.stageError(w, r, err, cores.CleanText(name, 120))
		return
	}
	s.finishStage(w, r, st, fields["replaces"], fields["sha256"])
}

// POST /api/gaming/custom/staged/url {password, url, sha256?, replaces?}
func (s *Server) handleStageURL(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Password string `json:"password"`
		URL      string `json:"url"`
		SHA256   string `json:"sha256"`
		Replaces string `json:"replaces"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.customPassword(w, r, b.Password, "") {
		return
	}
	cs := s.games().custom
	if !cs.takeSlot() {
		writeErr(w, 429, stageBusy)
		return
	}
	defer cs.freeSlot()
	ctx, cancel := context.WithTimeout(r.Context(), uploadTime)
	defer cancel()
	st, err := cs.store.StageURL(ctx, b.URL)
	if err != nil {
		s.stageError(w, r, err, cores.CleanText(b.URL, 300))
		return
	}
	s.finishStage(w, r, st, b.Replaces, b.SHA256)
}

// POST /api/gaming/custom/staged/placed {password, key, replaces?}: the key is one
// GET /gaming/custom listed, so no request ever names a path.
func (s *Server) handleStagePlaced(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Password string `json:"password"`
		Key      string `json:"key"`
		Replaces string `json:"replaces"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.customPassword(w, r, b.Password, "") {
		return
	}
	cs := s.games().custom
	if !cs.takeSlot() {
		writeErr(w, 429, stageBusy)
		return
	}
	defer cs.freeSlot()
	st, err := cs.store.StagePlaced(b.Key)
	if err != nil {
		s.stageError(w, r, err, cores.CleanText(b.Key, 200))
		return
	}
	s.finishStage(w, r, st, b.Replaces, "")
}

// takeStage finds a stage this person made. To anyone else it doesn't exist.
func (s *Server) takeStage(r *http.Request) *stagedCore {
	cs := s.games().custom
	cs.mu.Lock()
	defer cs.mu.Unlock()
	st := cs.staged[r.PathValue("token")]
	if st == nil || st.owner != userFrom(r).ID {
		return nil
	}
	if time.Since(st.s.Created) > stageLife {
		cs.store.Discard(st.s)
		delete(cs.staged, st.s.Token)
		return nil
	}
	return st
}

// DELETE /api/gaming/custom/staged/{token}
func (s *Server) handleStageCancel(w http.ResponseWriter, r *http.Request) {
	st := s.takeStage(r)
	if st == nil {
		writeErr(w, 404, "not found")
		return
	}
	cs := s.games().custom
	cs.mu.Lock()
	delete(cs.staged, st.s.Token)
	cs.mu.Unlock()
	cs.store.Discard(st.s)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- adding a core: approval ----

type coreSettings struct {
	Name       *string   `json:"name"`
	License    *string   `json:"license"`
	Note       *string   `json:"note"`
	Consoles   *[]string `json:"consoles"`
	DefaultFor *[]string `json:"defaultFor"`
}

// checkConsoles says which consoles can be assigned: any that exists and isn't hidden.
func (s *Server) checkConsoles(list []string) error {
	if len(list) > 200 {
		return errors.New("that's too many consoles")
	}
	for _, id := range list {
		if s.gameConsole(id) == nil {
			return fmt.Errorf("%q isn't a console on this server", cores.CleanText(id, 60))
		}
	}
	return nil
}

// apply puts settings from a request on a core, checking them.
func (s *Server) applyCoreSettings(c *CustomCore, b *coreSettings, info *cores.Info) error {
	if b.Name != nil {
		name := cores.CleanText(*b.Name, 120)
		if name == "" {
			return errors.New("a core needs a name")
		}
		c.Name = name
	}
	if b.Note != nil {
		c.Note = cores.CleanText(*b.Note, 2000)
	}
	if b.Consoles != nil {
		if err := s.checkConsoles(*b.Consoles); err != nil {
			return err
		}
		c.Consoles = slices.Compact(slices.Sorted(slices.Values(*b.Consoles)))
	}
	typed := ""
	switch {
	case b.License != nil:
		typed = cores.CleanText(*b.License, 200)
	case c.LicenseSource == "admin":
		typed = c.License
	}
	c.License, c.LicenseSource, c.NonCommercial = coreLicense(c.ID, c.Report.File, c.Report.Name, info, typed)
	return nil
}

// setDefaults makes a core the default for these consoles, and no others.
func (s *Server) setCoreDefaults(ctx context.Context, c *CustomCore, list []string) error {
	prefs := s.preferredCores(ctx)
	for console, id := range prefs {
		if id == c.ID && !slices.Contains(list, console) {
			delete(prefs, console)
		}
	}
	for _, console := range list {
		if !slices.Contains(c.Consoles, console) {
			return fmt.Errorf("it has to be assigned to %s before it can be the default", cores.CleanText(console, 60))
		}
		prefs[console] = c.ID
	}
	return s.DB.SetSetting(ctx, "gamingCores", toJSON(prefs))
}

// POST /api/gaming/custom/staged/{token}/approve {id?, acknowledged, name?, license?,
// note?, consoles?, defaultFor?}. Whoever got here gave their password to start this;
// the token is what ties the two together.
func (s *Server) handleStageApprove(w http.ResponseWriter, r *http.Request) {
	st := s.takeStage(r)
	if st == nil {
		writeErr(w, 404, "This test has expired, or was for someone else. Add the core again.")
		return
	}
	var b struct {
		coreSettings
		ID           string `json:"id"`
		Acknowledged bool   `json:"acknowledged"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !b.Acknowledged {
		writeErr(w, 400, "Please confirm that you understand this core runs as code on this server.")
		return
	}
	ctx := r.Context()
	u := userFrom(r)
	id := st.report.ID
	if st.replaces != "" {
		id = st.replaces
	} else if b.ID != "" {
		id = b.ID
	}
	if !cores.ValidCustomID(id) {
		writeErr(w, 400, "A core's name can only have small letters, numbers, dashes and underscores.")
		return
	}
	if cores.Offered(id) {
		writeErr(w, 409, fmt.Sprintf("%q is a core in this server's own list. Give this one another name, so nobody takes it for that.", id))
		return
	}
	existing, err := s.customCoreRow(ctx, id)
	replacing := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 500, err.Error())
		return
	}
	if replacing && st.replaces != id {
		writeErr(w, 409, fmt.Sprintf("A core called %q is already here. Replace it from its own card, or give this one another name.", id))
		return
	}
	now := time.Now().Unix()
	c := &CustomCore{ID: id, AddedAt: now, Consoles: []string{}}
	if replacing {
		c = existing
	}
	c.Name, c.Version, c.SHA256, c.Size, c.Report = st.report.Name, st.report.Version, st.s.SHA256, st.s.Size, st.report
	c.Source, c.SourceURL, c.AddedBy = st.s.Source, st.s.SourceURL, u.Name
	c.Report.ID = id
	if err := s.applyCoreSettings(c, &b.coreSettings, st.s.Info); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cs := s.games().custom
	// Nothing past this point can be cancelled by a second approval.
	cs.mu.Lock()
	if cs.staged[st.s.Token] != st {
		cs.mu.Unlock()
		writeErr(w, 404, "This test has already been used.")
		return
	}
	delete(cs.staged, st.s.Token)
	cs.mu.Unlock()
	added, kept, err := cs.store.Commit(st.s, id)
	if err != nil {
		cs.store.Discard(st.s)
		s.auditCore(ctx, u, "add-refused", id, err.Error())
		writeErr(w, 422, err.Error())
		return
	}
	for _, f := range kept {
		c.Report.Warnings = append(c.Report.Warnings, fmt.Sprintf("%s is already in the BIOS folder, so that copy was kept and this package's wasn't used.", f))
	}
	c.systemFiles = slices.Compact(slices.Sorted(slices.Values(append(c.systemFiles, added...))))
	if err := s.saveCustomCore(ctx, c); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO custom_core_versions(core_id, sha256, version, added_at) VALUES(?,?,?,?)
		ON CONFLICT(core_id, sha256) DO UPDATE SET version = excluded.version, added_at = excluded.added_at`, id, c.SHA256, c.Version, now); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if b.DefaultFor != nil {
		if err := s.setCoreDefaults(ctx, c, *b.DefaultFor); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	s.pruneVersions(ctx, c)
	s.publishCustomConsoles(ctx)
	action := "added"
	if replacing {
		action = "replaced"
	}
	s.auditCore(ctx, u, action, id, fmt.Sprintf("%s %s, sha256 %s, from %s %s, consoles %s", c.Name, c.Version, c.SHA256, c.Source, c.SourceURL, strings.Join(c.Consoles, ",")))
	writeJSON(w, 200, c)
}

// pruneVersions keeps the newest few, so going back is possible and disks don't fill.
func (s *Server) pruneVersions(ctx context.Context, c *CustomCore) {
	versions := s.coreVersions(ctx, c)
	kept := 0
	for _, v := range versions {
		if v.Current {
			continue
		}
		kept++
		if kept >= maxKeptVersion {
			store := s.games().custom.store
			store.RemoveVersion(c.ID, v.SHA256)
			if _, err := s.DB.ExecContext(ctx, `DELETE FROM custom_core_versions WHERE core_id = ? AND sha256 = ?`, c.ID, v.SHA256); err != nil {
				s.Log.Warn("pruning core versions", "err", err)
			}
		}
	}
}

// ---- changing a core ----

func (s *Server) customCorePath(w http.ResponseWriter, r *http.Request) *CustomCore {
	id := r.PathValue("id")
	if !cores.ValidCustomID(id) {
		writeErr(w, 404, "not found")
		return nil
	}
	c, err := s.customCoreRow(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "not found")
		return nil
	}
	return c
}

// PUT /api/gaming/custom/cores/{id}: its name, licence, note and consoles. The code
// doesn't change, so this needs no password.
func (s *Server) handleCustomCorePut(w http.ResponseWriter, r *http.Request) {
	c := s.customCorePath(w, r)
	if c == nil {
		return
	}
	var b coreSettings
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	store := s.games().custom.store
	if err := s.applyCoreSettings(c, &b, store.ReadVersionInfo(c.ID, c.SHA256)); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.saveCustomCore(ctx, c); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defaults := b.DefaultFor
	if defaults == nil {
		// A core taken off a console stops being its default.
		kept := []string{}
		for console, id := range s.preferredCores(ctx) {
			if id == c.ID && slices.Contains(c.Consoles, console) {
				kept = append(kept, console)
			}
		}
		defaults = &kept
	}
	if err := s.setCoreDefaults(ctx, c, *defaults); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.publishCustomConsoles(ctx)
	s.auditCore(ctx, userFrom(r), "changed", c.ID, "consoles "+strings.Join(c.Consoles, ",")+", default for "+strings.Join(*defaults, ",")+", license "+c.License)
	writeJSON(w, 200, c)
}

// DELETE /api/gaming/custom/cores/{id}. Saves belong to accounts and games, not cores,
// so nothing of anyone's goes with it. This works with custom cores turned off, because
// getting rid of a core should never be harder than having one.
func (s *Server) handleCustomCoreDelete(w http.ResponseWriter, r *http.Request) {
	c := s.customCorePath(w, r)
	if c == nil {
		return
	}
	ctx := r.Context()
	g := s.games()
	if n := g.mgr.UsingCore(g.custom.store.CorePath(c.ID, c.SHA256)); n > 0 {
		writeErr(w, 409, "A game is playing with this core right now. Stop it first.")
		return
	}
	prefs := s.preferredCores(ctx)
	for console, id := range prefs {
		if id == c.ID {
			delete(prefs, console)
		}
	}
	if err := s.DB.SetSetting(ctx, "gamingCores", toJSON(prefs)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, q := range []string{`DELETE FROM game_core_choice WHERE core_id = ?`, `DELETE FROM custom_core_versions WHERE core_id = ?`, `DELETE FROM game_core_plays WHERE core_id = ?`, `DELETE FROM custom_cores WHERE id = ?`} {
		if _, err := s.DB.ExecContext(ctx, q, c.ID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if err := g.custom.store.RemoveCore(c.ID, c.systemFiles); err != nil {
		s.Log.Warn("removing a core's files", "core", c.ID, "err", err)
	}
	s.publishCustomConsoles(ctx)
	s.auditCore(ctx, userFrom(r), "removed", c.ID, c.Name+" "+c.Version+", sha256 "+c.SHA256)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/gaming/custom/cores/{id}/reapprove {password}: someone has looked at why a
// file changed and wants it. It has to pass a test-load again like a new file.
func (s *Server) handleCustomReapprove(w http.ResponseWriter, r *http.Request) {
	c := s.customCorePath(w, r)
	if c == nil {
		return
	}
	var b struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.customPassword(w, r, b.Password, c.ID) {
		return
	}
	ctx := r.Context()
	cs := s.games().custom
	state := cs.store.Verify(c.ID, c.SHA256)
	switch state.State {
	case "ok":
		writeJSON(w, 200, c)
		return
	case "changed":
	default:
		writeErr(w, 409, "This core's file is gone or has been swapped for a link. Remove the core and add it again.")
		return
	}
	box, err := os.MkdirTemp(filepath.Dir(cs.store.VersionFolder(c.ID, c.SHA256)), ".box-")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer os.RemoveAll(box)
	res, err := s.testLoad(ctx, cs.store.CorePath(c.ID, c.SHA256), box)
	if err != nil {
		s.auditCore(ctx, userFrom(r), "reapprove-failed", c.ID, err.Error())
		writeErr(w, 422, err.Error())
		return
	}
	old := c.SHA256
	if err := os.Rename(cs.store.VersionFolder(c.ID, old), cs.store.VersionFolder(c.ID, state.Found)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	bin, _ := cores.ReadBinary(cs.store.CorePath(c.ID, state.Found))
	c.SHA256, c.Version = state.Found, res.Version
	if res.Name != "" {
		c.Report.Name = res.Name
	}
	c.Report.SHA256, c.Report.Version, c.Report.Extensions, c.Report.Binary, c.Report.Arch = state.Found, res.Version, res.Extensions, bin, bin.Describe()
	if st, err := os.Stat(cs.store.CorePath(c.ID, state.Found)); err == nil {
		c.Size, c.Report.Size = st.Size(), st.Size()
	}
	if err := s.saveCustomCore(ctx, c); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE custom_core_versions SET sha256 = ?, version = ? WHERE core_id = ? AND sha256 = ?`, state.Found, res.Version, c.ID, old); err != nil {
		s.Log.Warn("recording a core's version", "err", err)
	}
	s.auditCore(ctx, userFrom(r), "reapproved", c.ID, "was "+old+", now "+state.Found)
	writeJSON(w, 200, c)
}

var shaRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// POST /api/gaming/custom/cores/{id}/rollback {password, sha256}: back to a version
// kept from before. Nothing is thrown away, so this can be done again the other way.
func (s *Server) handleCustomRollback(w http.ResponseWriter, r *http.Request) {
	c := s.customCorePath(w, r)
	if c == nil {
		return
	}
	var b struct {
		Password string `json:"password"`
		SHA256   string `json:"sha256"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.customPassword(w, r, b.Password, c.ID) {
		return
	}
	ctx := r.Context()
	if !shaRe.MatchString(b.SHA256) {
		writeErr(w, 400, "that isn't a version of this core")
		return
	}
	var version string
	if err := s.DB.QueryRowContext(ctx, `SELECT version FROM custom_core_versions WHERE core_id = ? AND sha256 = ?`, c.ID, b.SHA256).Scan(&version); err != nil {
		writeErr(w, 404, "that version isn't kept any more")
		return
	}
	cs := s.games().custom
	if state := cs.store.Verify(c.ID, b.SHA256); state.State != "ok" {
		writeErr(w, 409, "That version's file has changed or gone, so it can't be used.")
		return
	}
	box, err := os.MkdirTemp(filepath.Dir(cs.store.VersionFolder(c.ID, b.SHA256)), ".box-")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer os.RemoveAll(box)
	if _, err := s.testLoad(ctx, cs.store.CorePath(c.ID, b.SHA256), box); err != nil {
		s.auditCore(ctx, userFrom(r), "rollback-failed", c.ID, err.Error())
		writeErr(w, 422, err.Error())
		return
	}
	old := c.SHA256
	c.SHA256, c.Version = b.SHA256, version
	c.Report.SHA256, c.Report.Version = b.SHA256, version
	if st, err := os.Stat(cs.store.CorePath(c.ID, b.SHA256)); err == nil {
		c.Size, c.Report.Size = st.Size(), st.Size()
	}
	if err := s.saveCustomCore(ctx, c); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.auditCore(ctx, userFrom(r), "rolled-back", c.ID, "from "+old+" to "+b.SHA256)
	writeJSON(w, 200, c)
}

// POST /api/gaming/custom/cores/{id}/files: multipart, password first, then file: a
// core's .info file, or a .zip with assets/ or system/ folders. The core's own file
// can't change this way.
func (s *Server) handleCustomFiles(w http.ResponseWriter, r *http.Request) {
	c := s.customCorePath(w, r)
	if c == nil {
		return
	}
	cs := s.games().custom
	if !cs.takeSlot() {
		writeErr(w, 429, stageBusy)
		return
	}
	defer cs.freeSlot()
	_, part, ok := s.readUpload(w, r, c.ID)
	if !ok {
		return
	}
	defer part.Close()
	ctx := r.Context()
	name := rawFileName(part)
	added, kept, err := cs.store.AddExtras(part, name, c.ID, c.SHA256)
	if err != nil {
		s.stageError(w, r, err, c.ID)
		return
	}
	c.systemFiles = slices.Compact(slices.Sorted(slices.Values(append(c.systemFiles, added...))))
	if info := cs.store.ReadVersionInfo(c.ID, c.SHA256); info != nil {
		s.applyInfoToReport(c, info)
		typed := ""
		if c.LicenseSource == "admin" {
			typed = c.License
		}
		c.License, c.LicenseSource, c.NonCommercial = coreLicense(c.ID, c.Report.File, c.Report.Name, info, typed)
	}
	if cs.store.AssetsPath(c.ID, c.SHA256) != "" {
		c.Report.Assets = max(c.Report.Assets, 1)
	}
	c.Report.SystemFiles += len(added)
	if err := s.saveCustomCore(ctx, c); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.auditCore(ctx, userFrom(r), "files-added", c.ID, cores.CleanText(name, 120))
	writeJSON(w, 200, map[string]any{"core": c, "added": added, "kept": kept})
}

func (s *Server) applyInfoToReport(c *CustomCore, info *cores.Info) {
	r := &c.Report
	r.HasInfo, r.BIOSKnown, r.System, r.Notes = true, true, info.System, info.Notes
	r.BIOS = []cores.Firmware{}
	for _, f := range info.Firmware {
		if native := filepath.FromSlash(f.File); filepath.IsLocal(native) {
			if _, err := os.Lstat(filepath.Join(s.games().systemDir(), native)); err == nil {
				f.Present = true
			}
		}
		r.BIOS = append(r.BIOS, f)
	}
	if info.HWRender && r.NeedsGPU != "yes" {
		r.NeedsGPU, r.GPUWhy = "yes", "Its info file says it wants a graphics card."
	}
}

// ---- choosing a core ----

// PUT /api/gaming/consoles/{console}/core {core}: the core a console plays with unless
// a game says otherwise. An empty core goes back to the first one here.
func (s *Server) handleConsoleCore(w http.ResponseWriter, r *http.Request) {
	console := s.gameConsole(r.PathValue("console"))
	if console == nil {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Core string `json:"core"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	if b.Core != "" && !s.coreFitsConsole(console, b.Core) {
		writeErr(w, 400, "that core isn't available for this console")
		return
	}
	prefs := s.preferredCores(ctx)
	if b.Core == "" {
		delete(prefs, console.ID)
	} else {
		prefs[console.ID] = b.Core
	}
	if err := s.DB.SetSetting(ctx, "gamingCores", toJSON(prefs)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.auditCore(ctx, userFrom(r), "console-default", b.Core, console.ID)
	writeJSON(w, 200, map[string]any{"core": b.Core})
}

// coreFitsConsole: a core from the table for this console that's here, or an approved
// custom core assigned to it.
func (s *Server) coreFitsConsole(console *wittyplay.Console, id string) bool {
	for _, c := range s.games().cores.List() {
		if c.ID != id || c.Unapproved {
			continue
		}
		if c.Source == cores.SourceCustom {
			return slices.Contains(c.Consoles, console.ID) || consoleListsCore(console, id)
		}
		return consoleListsCore(console, id)
	}
	return false
}

// GameCoreOption is a core that could play a game, and how it has gone so far.
type GameCoreOption struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Source        string  `json:"source"`
	License       string  `json:"license"`
	NonCommercial bool    `json:"nonCommercial,omitempty"`
	Starts        int     `json:"starts"`
	Failures      int     `json:"failures"`
	Seconds       float64 `json:"seconds"`
	LastError     string  `json:"lastError,omitempty"`
	Best          bool    `json:"best,omitempty"`
}

// GET /api/gaming/games/{id}/core: what would play this game, and what else could.
func (s *Server) handleGameCoreGet(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	ctx := r.Context()
	options := []GameCoreOption{}
	var effective string
	if console := wittyplay.ByID(it.Console); console != nil {
		g := s.games()
		for _, c := range g.cores.List() {
			if !c.Unapproved && s.coreFitsConsole(console, c.ID) {
				options = append(options, GameCoreOption{ID: c.ID, Name: c.Name, Source: c.Source, License: c.License, NonCommercial: c.NonCommercial})
			}
		}
		if core := g.cores.ForConsole(console, s.preferredCore(ctx, console, it.ID)); core != nil {
			effective = core.ID
		}
		s.markBestCore(ctx, it.ID, options)
	}
	writeJSON(w, 200, map[string]any{"choice": s.gameCoreChoice(ctx, it.ID), "effective": effective, "options": options})
}

// markBestCore fills in how each core has done on this game, and marks the one that has
// done best: nothing has gone wrong with it, and it has been played the longest. A few
// seconds' play says nothing, so nothing is marked until there's a minute of evidence.
func (s *Server) markBestCore(ctx context.Context, item int64, options []GameCoreOption) {
	rows, err := s.DB.QueryContext(ctx, `SELECT core_id, starts, failures, seconds, last_error FROM game_core_plays WHERE item_id = ?`, item)
	if err != nil {
		return
	}
	defer rows.Close()
	stats := map[string]GameCoreOption{}
	for rows.Next() {
		var id string
		var o GameCoreOption
		if rows.Scan(&id, &o.Starts, &o.Failures, &o.Seconds, &o.LastError) == nil {
			stats[id] = o
		}
	}
	best, score := -1, 0.0
	for i := range options {
		o := stats[options[i].ID]
		options[i].Starts, options[i].Failures, options[i].Seconds, options[i].LastError = o.Starts, o.Failures, o.Seconds, o.LastError
		if o.Starts == 0 || o.Seconds < 60 {
			continue
		}
		rate := 1 - float64(o.Failures)/float64(o.Starts)
		if rate < 0 {
			rate = 0
		}
		if v := rate * o.Seconds; v > score {
			best, score = i, v
		}
	}
	if best >= 0 {
		options[best].Best = true
	}
}

// PUT /api/gaming/games/{id}/core {core}: this game's own core; empty for the console's.
func (s *Server) handleGameCorePut(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Core string `json:"core"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	var err error
	if b.Core == "" {
		_, err = s.DB.ExecContext(ctx, `DELETE FROM game_core_choice WHERE item_id = ?`, it.ID)
	} else {
		console := wittyplay.ByID(it.Console)
		if console == nil || !s.coreFitsConsole(console, b.Core) {
			writeErr(w, 400, "that core isn't available for this game's console")
			return
		}
		_, err = s.DB.ExecContext(ctx, `INSERT INTO game_core_choice(item_id, core_id) VALUES(?,?) ON CONFLICT(item_id) DO UPDATE SET core_id = excluded.core_id`, it.ID, b.Core)
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.auditCore(ctx, userFrom(r), "game-core", b.Core, fmt.Sprintf("item %d", it.ID))
	writeJSON(w, 200, map[string]any{"core": b.Core})
}

// ---- custom consoles ----

// publishCustomConsoles makes the stored consoles the ones the scanner and every list
// see. They are data, not code, so they stay even with custom cores turned off.
func (s *Server) publishCustomConsoles(ctx context.Context) {
	bg := context.WithoutCancel(ctx)
	customs := s.customCoreRows(bg)
	rows, err := s.DB.QueryContext(bg, `SELECT id, data, aliases, cores FROM custom_consoles ORDER BY created_at, id`)
	if err != nil {
		s.Log.Warn("reading custom consoles", "err", err)
		return
	}
	defer rows.Close()
	list := []wittyplay.Console{}
	for rows.Next() {
		c, listed, err := scanCustomConsole(rows)
		if err != nil {
			s.Log.Warn("a custom console couldn't be read", "err", err)
			continue
		}
		for _, id := range listed {
			if known, ok := wittyplay.CoreLicense(id); ok && cores.Offered(id) {
				c.Cores = append(c.Cores, known)
			}
		}
		for _, core := range customs {
			if slices.Contains(core.Consoles, c.ID) || slices.Contains(listed, core.ID) {
				c.Cores = append(c.Cores, wittyplay.Core{ID: core.ID, Name: core.Name, License: core.License})
			}
		}
		list = append(list, *c)
	}
	wittyplay.SetCustomConsoles(list)
}

func scanCustomConsole(row interface{ Scan(...any) error }) (*wittyplay.Console, []string, error) {
	var id, data, aliases, listed string
	if err := row.Scan(&id, &data, &aliases, &listed); err != nil {
		return nil, nil, err
	}
	var c wittyplay.Console
	if err := json.Unmarshal([]byte(data), &c); err != nil {
		return nil, nil, err
	}
	// Whatever the database holds, a stored console can't become one from the table.
	if !wittyplay.IsCustomConsole(id) {
		return nil, nil, fmt.Errorf("%q isn't a custom console's name", id)
	}
	c.ID = id
	_ = json.Unmarshal([]byte(aliases), &c.Aliases)
	var cs []string
	_ = json.Unmarshal([]byte(listed), &cs)
	c.Cores = nil
	return &c, cs, nil
}

func (s *Server) customConsoleViews(ctx context.Context) []CustomConsoleView {
	out := []CustomConsoleView{}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, data, aliases, cores FROM custom_consoles ORDER BY created_at, id`)
	if err != nil {
		return out
	}
	defer rows.Close()
	type pending struct {
		c      *wittyplay.Console
		listed []string
	}
	var all []pending
	for rows.Next() {
		if c, listed, err := scanCustomConsole(rows); err == nil {
			all = append(all, pending{c, listed})
		}
	}
	rows.Close()
	for _, p := range all {
		v := CustomConsoleView{CustomConsole: wittyplay.CustomConsoleInput(p.c, p.listed)}
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM items WHERE console = ?`, p.c.ID).Scan(&v.Games)
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM libraries WHERE console = ?`, p.c.ID).Scan(&v.Libraries)
		out = append(out, v)
	}
	return out
}

func (s *Server) saveCustomConsole(ctx context.Context, id string, r *http.Request, w http.ResponseWriter) (*wittyplay.Console, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, wittyplay.MaxCustomConsoleJSON+1))
	if err != nil {
		writeErr(w, 400, "this is too long to be one console")
		return nil, false
	}
	in, err := wittyplay.ParseCustomConsole(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return nil, false
	}
	others := []wittyplay.Console{}
	for _, c := range wittyplay.CustomConsoles() {
		if c.ID != id {
			others = append(others, c)
		}
	}
	if id == "" && len(others) >= wittyplay.MaxCustomConsoles {
		writeErr(w, 409, "That's as many custom consoles as a server can have.")
		return nil, false
	}
	c, err := in.Validate(id, others)
	if err != nil {
		writeErr(w, 400, err.Error())
		return nil, false
	}
	for _, core := range in.Cores {
		_, isCustom := s.customCoreRow(ctx, core)
		if !cores.Offered(core) && isCustom != nil {
			writeErr(w, 400, fmt.Sprintf("%q isn't a core this server has heard of", core))
			return nil, false
		}
	}
	data, _ := json.Marshal(c)
	now := time.Now().Unix()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO custom_consoles(id, data, aliases, cores, created_at, updated_at) VALUES(?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET data = excluded.data, aliases = excluded.aliases, cores = excluded.cores, updated_at = excluded.updated_at`,
		c.ID, string(data), toJSON(c.Aliases), toJSON(in.Cores), now, now); err != nil {
		writeErr(w, 500, err.Error())
		return nil, false
	}
	s.publishCustomConsoles(ctx)
	return c, true
}

// POST /api/gaming/custom/consoles: one console as JSON, from the form or pasted in.
func (s *Server) handleCustomConsolePost(w http.ResponseWriter, r *http.Request) {
	c, ok := s.saveCustomConsole(r.Context(), "", r, w)
	if !ok {
		return
	}
	s.auditCore(r.Context(), userFrom(r), "console-added", "", c.ID+": "+c.Name)
	writeJSON(w, 201, wittyplay.ByID(c.ID))
}

// PUT /api/gaming/custom/consoles/{id}
func (s *Server) handleCustomConsolePut(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !wittyplay.IsCustomConsole(id) || wittyplay.ByID(id) == nil {
		writeErr(w, 404, "not found")
		return
	}
	c, ok := s.saveCustomConsole(r.Context(), id, r, w)
	if !ok {
		return
	}
	s.auditCore(r.Context(), userFrom(r), "console-changed", "", c.ID+": "+c.Name)
	writeJSON(w, 200, wittyplay.ByID(c.ID))
}

// DELETE /api/gaming/custom/consoles/{id}. A library set to a console has to be changed
// first; its games have nowhere sensible to go otherwise.
func (s *Server) handleCustomConsoleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !wittyplay.IsCustomConsole(id) || wittyplay.ByID(id) == nil {
		writeErr(w, 404, "not found")
		return
	}
	ctx := r.Context()
	var libs int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM libraries WHERE console = ?`, id).Scan(&libs)
	if libs > 0 {
		writeErr(w, 409, fmt.Sprintf("%d %s set to this console. Change or remove them first.", libs, plural(libs, "library is", "libraries are")))
		return
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM custom_consoles WHERE id = ?`, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, c := range s.customCoreRows(ctx) {
		if i := slices.Index(c.Consoles, id); i >= 0 {
			c.Consoles = slices.Delete(c.Consoles, i, i+1)
			if err := s.saveCustomCore(ctx, c); err != nil {
				s.Log.Warn("updating a core's consoles", "err", err)
			}
		}
	}
	prefs := s.preferredCores(ctx)
	if _, ok := prefs[id]; ok {
		delete(prefs, id)
		_ = s.DB.SetSetting(ctx, "gamingCores", toJSON(prefs))
	}
	s.publishCustomConsoles(ctx)
	s.auditCore(ctx, userFrom(r), "console-removed", "", id)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
