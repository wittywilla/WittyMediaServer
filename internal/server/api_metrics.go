package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/diagnose"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/metrics"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// The numbers behind the statistics screen, for the WittyStats module and any native app.
// These are the server's own and work with or without that module. docs/STATS.md has it all.
//
// Only an administrator gets the machine. Anyone else, if the owner allows it, gets the
// server's name for itself and how long it's been up, their own streams, and the libraries
// they can see. Every route checks for itself: a page being hidden protects nothing.

func init() {
	settingKeys = append(settingKeys, "statsViewers")
	settingDefaults["statsViewers"] = "admins"
	settingChecks["statsViewers"] = oneOf("admins", "adults", "everyone")
}

func (s *Server) metricsRoutes(m *http.ServeMux) {
	s.Metrics = metrics.New(metricStore{s}, func() metrics.Counts {
		a, b, c := s.streams.counts()
		return metrics.Counts{Streams: a, Transcodes: b, Games: c}
	})
	a := s.requireUser
	m.Handle("GET /api/system/metrics", a(s.handleMetricsCatalog))
	m.Handle("GET /api/system/metrics/now", a(s.handleMetricsNow))
	m.Handle("GET /api/system/metrics/stream", a(s.handleMetricsStream))
	m.Handle("GET /api/system/metrics/history", a(s.handleMetricsHistory))
	m.Handle("GET /api/system/metrics/report", a(s.handleMetricsReport))
	m.Handle("GET /api/system/metrics/drives", s.requireAdmin(s.handleMetricsDrives))
}

// metricsAccess says what this account gets.
type metricsAccess struct {
	Visible bool `json:"visible"`
	Full    bool `json:"full"` // the whole machine, not only one's own
}

func (s *Server) metricsAccessFor(ctx context.Context, u *auth.User) metricsAccess {
	if u.IsAdmin {
		return metricsAccess{Visible: true, Full: true}
	}
	switch s.DB.GetSetting(ctx, "statsViewers", "admins") {
	case "everyone":
		return metricsAccess{Visible: true}
	case "adults":
		return metricsAccess{Visible: u.Kind == auth.KindAdult}
	}
	return metricsAccess{}
}

// MetricSection says whether a part of the screen can show anything on this server, and if
// not, why, so the screen can say so rather than draw a zero.
type MetricSection struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Available bool   `json:"available"`
	Why       string `json:"why,omitempty"`
	Full      bool   `json:"full,omitempty"` // only with the whole machine
}

// MetricsCatalog is what this server and this account can show.
type MetricsCatalog struct {
	Access   metricsAccess   `json:"access"`
	Sections []MetricSection `json:"sections"`
	Series   []string        `json:"series"`
	Ranges   []string        `json:"ranges"`
	EverySec int             `json:"everySec"`
}

var metricRanges = map[string]time.Duration{"15m": 15 * time.Minute, "1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour, "90d": 90 * 24 * time.Hour}
var metricRangeOrder = []string{"15m", "1h", "6h", "24h", "7d", "30d", "90d"}

// Sections an account without the whole machine can have.
var ownSections = map[string]bool{"server": true, "streams": true, "games": true, "libraries": true}

// ownMetricSections are what an account without the whole machine can have. Saying so
// reads nothing from the machine.
func ownMetricSections() []MetricSection {
	return []MetricSection{{ID: "server", Title: "Server", Available: true}, {ID: "streams", Title: "Streams", Available: true},
		{ID: "games", Title: "Game sessions", Available: true}, {ID: "libraries", Title: "Libraries", Available: true}}
}

func (s *Server) metricSections() []MetricSection {
	notOn := func(what string) string { return what + " isn't available on " + runtime.GOOS + "." }
	_, _, cpu := sysinfo.ReadCPU()
	nets := sysinfo.ReadNetwork()
	counted := false
	for _, n := range nets {
		counted = counted || n.RxBytes > 0 || n.TxBytes > 0
	}
	sec := func(id, title string, ok bool, why string, full bool) MetricSection {
		m := MetricSection{ID: id, Title: title, Available: ok, Full: full}
		if !ok {
			m.Why = why
		}
		return m
	}
	gpus := s.gpus.Read(context.Background())
	return []MetricSection{
		sec("server", "Server", true, "", false),
		sec("cpu", "Processor", cpu, notOn("Processor use"), true),
		sec("temperature", "Temperature", len(sysinfo.ReadTemperatures()) > 0, "This system doesn't report temperatures to the server.", true),
		sec("memory", "Memory and swap", true, "", true),
		sec("gpu", "Graphics", len(gpus) > 0, "No graphics card here reports its use. NVIDIA needs nvidia-smi, and AMD needs Linux.", true),
		sec("disks", "Disks", true, "", true),
		sec("diskio", "Disk speed", len(sysinfo.ReadDiskIO()) > 0, notOn("Disk speed"), true),
		sec("network", "Network", counted || runtime.GOOS == "linux" || runtime.GOOS == "windows", notOn("Network traffic"), true),
		sec("process", "This server", true, "", true),
		sec("modules", "Modules", s.Modules != nil, "This build can't update modules.", true),
		sec("streams", "Streams", true, "", false),
		sec("games", "Game sessions", true, "", false),
		sec("libraries", "Libraries", true, "", false),
		sec("logs", "Log", s.Logs != nil, "This server isn't keeping a log.", true),
	}
}

// GET /api/system/metrics: what can be shown here. Always answered, and an account that
// may not see any of this is told so.
func (s *Server) handleMetricsCatalog(w http.ResponseWriter, r *http.Request) {
	acc := s.metricsAccessFor(r.Context(), userFrom(r))
	out := MetricsCatalog{Access: acc, Sections: []MetricSection{}, Series: []string{}, Ranges: []string{}, EverySec: 2}
	if acc.Visible {
		out.Ranges = metricRangeOrder
		if acc.Full {
			out.Sections, out.Series = s.metricSections(), metrics.SeriesNames
		} else {
			out.Sections = ownMetricSections()
		}
	}
	writeJSON(w, 200, out)
}

// MetricsNow is the state of things at one moment. A section that wasn't asked for, or that
// this account or this system doesn't have, is left out.
type MetricsNow struct {
	Time    int64                `json:"time"`
	Server  *ServerNow           `json:"server,omitempty"`
	CPU     *metrics.CPUView     `json:"cpu,omitempty"`
	Memory  *metrics.MemoryView  `json:"memory,omitempty"`
	GPU     []metrics.GPU        `json:"gpu,omitempty"`
	Disks   []metrics.DiskView   `json:"disks,omitempty"`
	Volumes []metrics.VolumeView `json:"volumes,omitempty"`
	Net     []metrics.NetView    `json:"net,omitempty"`
	Process *metrics.ProcessView `json:"process,omitempty"`
	Modules []ModuleNow          `json:"modules,omitempty"`
	Streams []StreamView         `json:"streams,omitempty"`
	Games   []GameView           `json:"games,omitempty"`
	// Transcodes is how many are going, and how many the server allows at once.
	Transcodes *TranscodeNow  `json:"transcodes,omitempty"`
	Tasks      *tasks.Summary `json:"tasks,omitempty"`
}

// ServerNow is the server itself.
type ServerNow struct {
	Name      string `json:"name,omitempty"`
	Version   string `json:"version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	UptimeSec int64  `json:"uptimeSec"`
	Update    string `json:"updateAvailable,omitempty"` // admins only
}

// TranscodeNow is how busy the converting is.
type TranscodeNow struct {
	Active int `json:"active"`
	Max    int `json:"max"`
}

// ModuleNow is a module and how it stands.
type ModuleNow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Latest    string `json:"latest,omitempty"`
	Phase     string `json:"phase,omitempty"`
	Error     string `json:"error,omitempty"`
	Installed bool   `json:"installed"`
}

func wantedSections(r *http.Request, full bool) func(string) bool {
	asked := r.URL.Query().Get("sections")
	set := map[string]bool{}
	for _, x := range strings.Split(asked, ",") {
		if x != "" {
			set[x] = true
		}
	}
	return func(id string) bool {
		if !full && !ownSections[id] {
			return false
		}
		return len(set) == 0 || set[id]
	}
}

// buildNow fills in everything but the machine's own sample.
func (s *Server) buildNow(ctx context.Context, u *auth.User, full bool, want func(string) bool, sample metrics.Sample) MetricsNow {
	out := MetricsNow{Time: time.Now().Unix()}
	if want("server") {
		v := &ServerNow{Version: s.Version, OS: runtime.GOOS, Arch: runtime.GOARCH, UptimeSec: int64(time.Since(serverStart).Seconds())}
		if full {
			v.Name = s.DB.GetSetting(ctx, "serverName", "")
			if s.Update != nil {
				v.Update = s.Update.Available()
			}
		}
		out.Server = v
	}
	only := u.ID
	if full {
		only = 0
	}
	if want("streams") {
		out.Streams = s.streams.streams(only)
	}
	if want("games") {
		out.Games = s.streams.gameViews(only)
	}
	if !full {
		return out
	}
	if want("cpu") || want("temperature") {
		out.CPU = sample.CPU
	}
	if want("memory") {
		out.Memory = sample.Memory
	}
	if want("disks") || want("diskio") {
		out.Disks, out.Volumes = sample.Disks, sample.Volumes
	}
	if want("network") {
		out.Net = sample.Net
	}
	if want("process") {
		out.Process = sample.Process
	}
	if want("gpu") {
		out.GPU = s.gpus.Read(ctx)
	}
	if want("modules") {
		out.Modules = s.modulesNow()
	}
	if want("streams") {
		_, t, _ := s.streams.counts()
		out.Transcodes = &TranscodeNow{Active: t, Max: s.Cfg.MaxTranscode}
		sum := tasks.Summarize(s.Tasks.Live())
		out.Tasks = &sum
	}
	return out
}

// modulesNow is how the modules stand. It hardly changes, and asking takes the manager's
// lock and a look at some files, so the answer is kept for a while whoever asks.
func (s *Server) modulesNow() []ModuleNow {
	if s.Modules == nil {
		return nil
	}
	s.reports.Lock()
	defer s.reports.Unlock()
	if time.Since(s.reports.modulesAt) < modulesKeep {
		return s.reports.modules
	}
	out := []ModuleNow{}
	for _, m := range s.Modules.Report().Modules {
		v := ModuleNow{ID: m.ID, Name: m.Name, Version: m.Version, Phase: m.Phase, Error: m.Error, Installed: m.Installed}
		if m.Latest != nil {
			v.Latest = m.Latest.Version
		}
		out = append(out, v)
	}
	s.reports.modules, s.reports.modulesAt = out, time.Now()
	return out
}

// GET /api/system/metrics/now: one reading. With sections= to choose. For an app that wants
// a number now and then. Speeds need two readings, so it says what the sampler last saw when
// someone is watching, and otherwise takes two a moment apart.
func (s *Server) handleMetricsNow(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	acc := s.metricsAccessFor(r.Context(), u)
	if !acc.Visible {
		writeErr(w, 403, "statistics aren't available to this account")
		return
	}
	var sample metrics.Sample
	if acc.Full {
		sample = s.Metrics.OneReading(r.Context())
	}
	writeJSON(w, 200, s.buildNow(r.Context(), u, acc.Full, wantedSections(r, acc.Full), sample))
}

// GET /api/system/metrics/stream: a reading every two seconds, for as long as this stays open.
// The sampler only runs while there's one of these, so nobody watching costs nothing.
func (s *Server) handleMetricsStream(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	acc := s.metricsAccessFor(r.Context(), u)
	if !acc.Visible {
		writeErr(w, 403, "statistics aren't available to this account")
		return
	}
	if !s.metricStreams.take(u.ID) {
		writeErr(w, 429, "too many statistics streams are open")
		return
	}
	defer s.metricStreams.give(u.ID)
	tok := tokenFrom(r)
	es := newEventStream(w)
	want := wantedSections(r, acc.Full)
	release := s.Metrics.Watch()
	defer release()
	sample, changed := s.Metrics.Latest()
	if !es.send("sample", s.buildNow(r.Context(), u, acc.Full, want, sample)) {
		return
	}
	check := time.NewTicker(taskStreamBeat)
	defer check.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.streamsEnding():
			return
		case <-check.C:
			if !s.metricStreamAllowed(r, tok, u) {
				return
			}
		case <-changed:
			sample, changed = s.Metrics.Latest()
			if !es.send("sample", s.buildNow(r.Context(), u, acc.Full, want, sample)) {
				return
			}
		}
	}
}

func (s *Server) metricStreamAllowed(r *http.Request, tok string, was *auth.User) bool {
	if tok == "" {
		return true
	}
	if auth.IsStreamToken(tok) {
		return false
	}
	now, err := s.Auth.Resolve(r.Context(), tok)
	if err != nil || now == nil || now.IsAdmin != was.IsAdmin || now.Kind != was.Kind {
		return false
	}
	return s.metricsAccessFor(r.Context(), now).Visible
}

// GET /api/system/metrics/history?range=1h&series=cpu,memory: charts' worth of numbers.
func (s *Server) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	acc := s.metricsAccessFor(r.Context(), userFrom(r))
	if !acc.Full {
		writeErr(w, 403, "only an administrator can see this")
		return
	}
	span, ok := metricRanges[r.URL.Query().Get("range")]
	if !ok {
		writeErr(w, 400, "range must be one of "+strings.Join(metricRangeOrder, ", "))
		return
	}
	var names []string
	for _, n := range strings.Split(r.URL.Query().Get("series"), ",") {
		if n == "" {
			continue
		}
		if !slices.Contains(metrics.SeriesNames, n) {
			writeErr(w, 400, "unknown series: GET /api/system/metrics lists them")
			return
		}
		names = append(names, n)
	}
	h, err := s.Metrics.Read(r.Context(), span, names)
	if err != nil {
		writeErr(w, 500, "the history could not be read")
		return
	}
	writeJSON(w, 200, h)
}

// metricStore keeps the hourly numbers in the database.
type metricStore struct{ s *Server }

func (m metricStore) SaveHour(ctx context.Context, hour int64, name string, avg, peak float64) error {
	_, err := m.s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO metrics_hourly(hour, name, avg, peak) VALUES (?, ?, ?, ?)`, hour, name, avg, peak)
	return err
}

func (m metricStore) Hours(ctx context.Context, from, to int64, names []string) ([]metrics.HourRow, error) {
	rows, err := m.s.DB.QueryContext(ctx, `SELECT hour, name, avg, peak FROM metrics_hourly WHERE hour >= ? AND hour <= ? ORDER BY hour`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metrics.HourRow
	for rows.Next() {
		var h metrics.HourRow
		if err := rows.Scan(&h.Hour, &h.Name, &h.Avg, &h.Peak); err != nil {
			return nil, err
		}
		if slices.Contains(names, h.Name) {
			out = append(out, h)
		}
	}
	return out, rows.Err()
}

func (m metricStore) Prune(ctx context.Context, before int64) error {
	_, err := m.s.DB.ExecContext(ctx, `DELETE FROM metrics_hourly WHERE hour < ?`, before)
	return err
}

// ---------------------------------------------------------------- the report

// MetricsReport is everything that takes a look through the database: slow enough that
// it's kept for a minute, and only made when someone asks.
type MetricsReport struct {
	GeneratedAt int64             `json:"generatedAt"`
	Libraries   []ReportLibrary   `json:"libraries"`
	ByType      []ReportCount     `json:"byType"`
	Codecs      []ReportCount     `json:"codecs"`
	Resolutions []ReportCount     `json:"resolutions"`
	Duplicates  *ReportDuplicates `json:"duplicates,omitempty"`
	Savings     *ReportSavings    `json:"savings,omitempty"`
	Scans       []ReportScan      `json:"scans,omitempty"`
	Database    *ReportDatabase   `json:"database,omitempty"`
	Caches      []ReportCache     `json:"caches,omitempty"`
	Logs        []ReportLogHour   `json:"logs,omitempty"`
	MostWatched []ReportWatched   `json:"mostWatched"`
	Bandwidth   []ReportBandwidth `json:"bandwidth,omitempty"`
	Peak        *ReportPeak       `json:"peak,omitempty"`
}

// ReportLibrary is one library's size.
type ReportLibrary struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Items    int64  `json:"items"`
	Bytes    int64  `json:"bytes"`
	Missing  int64  `json:"missing"`
	LastScan *int64 `json:"lastScanAt,omitempty"`
}

// ReportCount is how many of something, and how big they are.
type ReportCount struct {
	Name  string `json:"name"`
	Items int64  `json:"items"`
	Bytes int64  `json:"bytes"`
}

// ReportDuplicates is the same title held more than once.
type ReportDuplicates struct {
	Groups      int64        `json:"groups"`
	WastedBytes int64        `json:"wastedBytes"` // what's left if only the largest of each stays
	Top         []ReportDupe `json:"top"`
}

// ReportDupe is one title held more than once.
type ReportDupe struct {
	Title  string `json:"title"`
	Year   *int64 `json:"year,omitempty"`
	Copies int64  `json:"copies"`
	Bytes  int64  `json:"bytes"`
	Wasted int64  `json:"wasted"`
}

// ReportSavings is a rough idea of what converting older video could save. An estimate,
// and the screen says so.
type ReportSavings struct {
	Items          int64 `json:"items"`
	Bytes          int64 `json:"bytes"`
	EstimatedSaved int64 `json:"estimatedSaved"`
}

// ReportScan is how a scan went.
type ReportScan struct {
	Library    string `json:"library"`
	State      string `json:"state"`
	Files      int64  `json:"files"`
	TookSec    int64  `json:"tookSec"`
	FinishedAt int64  `json:"finishedAt"`
}

// ReportDatabase is the size of the server's database.
type ReportDatabase struct {
	Bytes    int64 `json:"bytes"`
	WALBytes int64 `json:"walBytes"`
}

// ReportCache is one of the server's folders of things it can make again.
type ReportCache struct {
	Name    string `json:"name"`
	Bytes   int64  `json:"bytes"`
	Files   int64  `json:"files"`
	Partial bool   `json:"partial,omitempty"` // too many to finish counting
}

// ReportLogHour is how much went wrong in one hour.
type ReportLogHour struct {
	Hour     int64 `json:"hour"`
	Warnings int   `json:"warnings"`
	Errors   int   `json:"errors"`
}

// ReportWatched is one title and how long it's been played.
type ReportWatched struct {
	ItemID  int64   `json:"itemId"`
	Title   string  `json:"title"`
	Seconds float64 `json:"seconds"`
	People  int64   `json:"people"`
}

// ReportBandwidth is how much one person has been sent.
type ReportBandwidth struct {
	UserID int64  `json:"userId"`
	User   string `json:"user"`
	Bytes  int64  `json:"bytes"`
}

// ReportPeak is the most people at once.
type ReportPeak struct {
	Streams int   `json:"streams"`
	At      int64 `json:"at,omitempty"`
}

type reportCache struct {
	sync.Mutex
	entries   map[int64]reportEntry
	caches    []ReportCache
	modules   []ModuleNow
	modulesAt time.Time
	cachesAt  time.Time
}

type reportEntry struct {
	at  time.Time
	rep MetricsReport
}

const (
	reportKeep      = time.Minute
	reportCacheSize = 32
	cacheCountKeep  = 5 * time.Minute
	cacheCountTime  = 2 * time.Second
	modulesKeep     = 30 * time.Second
)

// GET /api/system/metrics/report: libraries, what's in them, and what's gone on. Someone
// without the whole machine gets the libraries they can see and their own record.
func (s *Server) handleMetricsReport(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	acc := s.metricsAccessFor(r.Context(), u)
	if !acc.Visible {
		writeErr(w, 403, "statistics aren't available to this account")
		return
	}
	s.reports.Lock()
	if e, ok := s.reports.entries[u.ID]; ok && time.Since(e.at) < reportKeep && !qBool(r, "fresh") {
		s.reports.Unlock()
		writeJSON(w, 200, e.rep)
		return
	}
	s.reports.Unlock()
	rep, err := s.makeReport(r, u, acc)
	if err != nil {
		writeErr(w, 500, "the report could not be made")
		return
	}
	s.reports.Lock()
	if s.reports.entries == nil || len(s.reports.entries) >= reportCacheSize {
		s.reports.entries = map[int64]reportEntry{}
	}
	s.reports.entries[u.ID] = reportEntry{at: time.Now(), rep: rep}
	s.reports.Unlock()
	writeJSON(w, 200, rep)
}

// DriveHealthView is what one disk says about itself.
type DriveHealthView struct {
	Mount   string   `json:"mount"`
	Checked bool     `json:"checked"`
	Bad     bool     `json:"bad,omitempty"`
	Lines   []string `json:"lines"`
	Note    string   `json:"note,omitempty"` // why it wasn't checked
}

// How long what the drives said is kept. Asking can start a program and wake a disk.
const drivesKeep = 10 * time.Minute

// GET /api/system/metrics/drives: each disk's health, where the system will say.
func (s *Server) handleMetricsDrives(w http.ResponseWriter, r *http.Request) {
	s.driveHealth.Lock()
	defer s.driveHealth.Unlock()
	if s.driveHealth.list == nil || time.Since(s.driveHealth.at) >= drivesKeep {
		list := []DriveHealthView{}
		for _, v := range sysinfo.ReadVolumes() {
			h := diagnose.ReadDriveHealth(r.Context(), s.Log, v.Mount)
			list = append(list, DriveHealthView{Mount: v.Mount, Checked: h.Checked, Bad: h.Bad, Lines: append([]string{}, h.Lines...), Note: h.Note})
		}
		if r.Context().Err() != nil {
			writeErr(w, 503, "that was cut short")
			return
		}
		s.driveHealth.list, s.driveHealth.at = list, time.Now()
	}
	writeJSON(w, 200, map[string]any{"drives": s.driveHealth.list})
}

// resolutionCase sorts a picture's height into the names people use.
const resolutionCase = `CASE WHEN IFNULL(height, 0) <= 0 THEN 'Unknown' WHEN height >= 2000 THEN '4K' WHEN height >= 1000 THEN '1080p' WHEN height >= 700 THEN '720p' ELSE 'SD' END`

func (s *Server) makeReport(r *http.Request, u *auth.User, acc metricsAccess) (MetricsReport, error) {
	ctx := r.Context()
	rep := MetricsReport{GeneratedAt: time.Now().Unix(), Libraries: []ReportLibrary{}, ByType: []ReportCount{}, Codecs: []ReportCount{}, Resolutions: []ReportCount{}, MostWatched: []ReportWatched{}}
	access := s.access(r)
	libs, err := s.Store.Libraries(ctx)
	if err != nil {
		return rep, err
	}
	var allowed []any
	marks := []string{}
	for _, l := range libs {
		if acc.Full || access.AllowsLibrary(l.ID) {
			allowed = append(allowed, l.ID)
			marks = append(marks, "?")
		}
	}
	if len(allowed) == 0 {
		return rep, nil
	}
	in := "library_id IN (" + strings.Join(marks, ",") + ")"
	rows, err := s.DB.QueryContext(ctx, `SELECT library_id, COUNT(*), IFNULL(SUM(size_bytes), 0), SUM(missing) FROM items WHERE `+in+` GROUP BY library_id`, allowed...)
	if err != nil {
		return rep, err
	}
	sizes := map[int64]ReportLibrary{}
	for rows.Next() {
		var l ReportLibrary
		var missing sql.NullInt64
		if err := rows.Scan(&l.ID, &l.Items, &l.Bytes, &missing); err != nil {
			rows.Close()
			return rep, err
		}
		l.Missing = missing.Int64
		sizes[l.ID] = l
	}
	rows.Close()
	byType := map[string]*ReportCount{}
	for _, l := range libs {
		if !acc.Full && !access.AllowsLibrary(l.ID) {
			continue
		}
		v := sizes[l.ID]
		v.ID, v.Name, v.Type, v.LastScan = l.ID, l.Name, l.Type, l.LastScanAt
		rep.Libraries = append(rep.Libraries, v)
		c := byType[l.Type]
		if c == nil {
			c = &ReportCount{Name: l.Type}
			byType[l.Type] = c
		}
		c.Items += v.Items
		c.Bytes += v.Bytes
	}
	for _, c := range byType {
		rep.ByType = append(rep.ByType, *c)
	}
	sort.Slice(rep.ByType, func(i, j int) bool { return rep.ByType[i].Bytes > rep.ByType[j].Bytes })
	group := func(expr, where string) ([]ReportCount, error) {
		rows, err := s.DB.QueryContext(ctx, `SELECT `+expr+`, COUNT(*), IFNULL(SUM(size_bytes), 0) FROM items WHERE missing = 0 AND `+in+where+` GROUP BY 1 ORDER BY 3 DESC LIMIT 30`, allowed...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []ReportCount{}
		for rows.Next() {
			var c ReportCount
			if err := rows.Scan(&c.Name, &c.Items, &c.Bytes); err != nil {
				return nil, err
			}
			out = append(out, c)
		}
		return out, rows.Err()
	}
	if rep.Codecs, err = group(`IFNULL(NULLIF(LOWER(video_codec), ''), 'unknown')`, ` AND kind = 'video'`); err != nil {
		return rep, err
	}
	if rep.Resolutions, err = group(resolutionCase, ` AND kind = 'video'`); err != nil {
		return rep, err
	}
	s.reportWatched(ctx, u, acc, in, allowed, &rep)
	if !acc.Full {
		return rep, nil
	}
	s.reportDuplicates(ctx, &rep)
	var sv ReportSavings
	if s.DB.QueryRowContext(ctx, `SELECT COUNT(*), IFNULL(SUM(size_bytes), 0) FROM items WHERE missing = 0 AND kind = 'video' AND LOWER(video_codec) IN ('h264', 'avc', 'mpeg2video', 'mpeg4', 'vc1', 'msmpeg4v3') AND height >= 480`).Scan(&sv.Items, &sv.Bytes) == nil {
		sv.EstimatedSaved = sv.Bytes * 2 / 5 // newer formats manage about the same picture in 40% less
		rep.Savings = &sv
	}
	if list, err := s.taskStore.List(ctx, tasks.Query{Limit: 30, Type: tasks.TypeScan}); err == nil {
		for _, t := range list {
			rep.Scans = append(rep.Scans, ReportScan{Library: t.LibraryName, State: string(t.State), Files: t.Done, TookSec: t.FinishedAt - t.StartedAt, FinishedAt: t.FinishedAt})
		}
	}
	var db ReportDatabase
	if st, err := os.Stat(filepath.Join(s.Cfg.DataDir, "wms.db")); err == nil {
		db.Bytes = st.Size()
	}
	if st, err := os.Stat(filepath.Join(s.Cfg.DataDir, "wms.db-wal")); err == nil {
		db.WALBytes = st.Size()
	}
	rep.Database = &db
	rep.Caches = s.reportCaches()
	rep.Logs = s.reportLogs()
	s.reportBandwidth(ctx, &rep)
	peak, at := s.streams.peakNow()
	if rows, err := s.DB.QueryContext(ctx, `SELECT hour, peak FROM metrics_hourly WHERE name = ? ORDER BY peak DESC, hour DESC LIMIT 1`, metrics.Streams); err == nil {
		for rows.Next() {
			var h int64
			var p float64
			if rows.Scan(&h, &p) == nil && int(p) > peak {
				peak, at = int(p), h
			}
		}
		rows.Close()
	}
	rep.Peak = &ReportPeak{Streams: peak, At: at}
	return rep, nil
}

func (s *Server) reportWatched(ctx context.Context, u *auth.User, acc metricsAccess, in string, allowed []any, rep *MetricsReport) {
	q := `SELECT p.item_id, i.title, SUM(p.seconds), COUNT(DISTINCT p.user_id) FROM play_log p JOIN items i ON i.id = p.item_id WHERE i.` + in
	args := append([]any{}, allowed...)
	if !acc.Full {
		q += ` AND p.user_id = ?` // what they watched themselves, and nobody else's
		args = append(args, u.ID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` GROUP BY p.item_id ORDER BY 3 DESC LIMIT 20`, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var w ReportWatched
		if rows.Scan(&w.ItemID, &w.Title, &w.Seconds, &w.People) == nil {
			rep.MostWatched = append(rep.MostWatched, w)
		}
	}
}

func (s *Server) reportDuplicates(ctx context.Context, rep *MetricsReport) {
	rows, err := s.DB.QueryContext(ctx, `SELECT title, year, COUNT(*), IFNULL(SUM(size_bytes), 0), IFNULL(SUM(size_bytes) - MAX(size_bytes), 0)
		FROM items WHERE missing = 0 AND show_id IS NULL AND kind IN ('video', 'dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder')
		GROUP BY LOWER(title), year HAVING COUNT(*) > 1 ORDER BY 5 DESC LIMIT 500`)
	if err != nil {
		return
	}
	defer rows.Close()
	d := &ReportDuplicates{Top: []ReportDupe{}}
	for rows.Next() {
		var x ReportDupe
		var year sql.NullInt64
		if rows.Scan(&x.Title, &year, &x.Copies, &x.Bytes, &x.Wasted) != nil {
			continue
		}
		if year.Valid {
			x.Year = &year.Int64
		}
		d.Groups++
		d.WastedBytes += x.Wasted
		if len(d.Top) < 50 {
			d.Top = append(d.Top, x)
		}
	}
	rep.Duplicates = d
}

// reportCaches counts up the folders the server can make again. On a large one this
// stops after a moment, and says so.
func (s *Server) reportCaches() []ReportCache {
	s.reports.Lock()
	defer s.reports.Unlock()
	if time.Since(s.reports.cachesAt) < cacheCountKeep {
		return s.reports.caches
	}
	out := []ReportCache{}
	for _, c := range []struct{ name, dir string }{{"Artwork", s.Cfg.ImagesDir}, {"Thumbnails and cache", s.Cfg.CacheDir}, {"Transcodes", s.Cfg.TranscodeDir}} {
		if c.dir == "" {
			continue
		}
		v := ReportCache{Name: c.name}
		deadline := time.Now().Add(cacheCountTime)
		_ = filepath.WalkDir(c.dir, func(_ string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if time.Now().After(deadline) {
				v.Partial = true
				return filepath.SkipAll
			}
			if !d.IsDir() {
				if st, err := d.Info(); err == nil {
					v.Bytes += st.Size()
					v.Files++
				}
			}
			return nil
		})
		out = append(out, v)
	}
	if s.Logs != nil {
		v := ReportCache{Name: "Logs"}
		if list, err := os.ReadDir(s.Logs.Dir()); err == nil {
			for _, e := range list {
				if st, err := e.Info(); err == nil && !e.IsDir() {
					v.Bytes += st.Size()
					v.Files++
				}
			}
		}
		out = append(out, v)
	}
	s.reports.caches, s.reports.cachesAt = out, time.Now()
	return out
}

// reportLogs counts warnings and errors in the last day, by hour.
func (s *Server) reportLogs() []ReportLogHour {
	if s.Logs == nil {
		return nil
	}
	page, err := s.Logs.Query(logging.Filter{MinLevel: slog.LevelWarn, Limit: 2000, History: true})
	if err != nil {
		return nil
	}
	cut := time.Now().Add(-24 * time.Hour)
	byHour := map[int64]*ReportLogHour{}
	for _, e := range page.Entries {
		if e.Time.Before(cut) {
			continue
		}
		h := e.Time.Unix() - e.Time.Unix()%3600
		v := byHour[h]
		if v == nil {
			v = &ReportLogHour{Hour: h}
			byHour[h] = v
		}
		if e.Lv >= slog.LevelError {
			v.Errors++
		} else {
			v.Warnings++
		}
	}
	out := make([]ReportLogHour, 0, len(byHour))
	for _, v := range byHour {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hour < out[j].Hour })
	return out
}

func (s *Server) reportBandwidth(ctx context.Context, rep *MetricsReport) {
	totals := s.streams.liveBytes()
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id, SUM(bytes) FROM stream_bytes WHERE day >= ? GROUP BY user_id`, time.Now().Unix()/86400-30)
	if err == nil {
		for rows.Next() {
			var id, n int64
			if rows.Scan(&id, &n) == nil {
				totals[id] += n
			}
		}
		rows.Close()
	}
	names := map[int64]string{}
	if rows, err := s.DB.QueryContext(ctx, `SELECT id, name FROM users`); err == nil {
		for rows.Next() {
			var id int64
			var n string
			if rows.Scan(&id, &n) == nil {
				names[id] = n
			}
		}
		rows.Close()
	}
	for id, n := range totals {
		if name, ok := names[id]; ok {
			rep.Bandwidth = append(rep.Bandwidth, ReportBandwidth{UserID: id, User: name, Bytes: n})
		}
	}
	sort.Slice(rep.Bandwidth, func(i, j int) bool { return rep.Bandwidth[i].Bytes > rep.Bandwidth[j].Bytes })
}

// ---------------------------------------------------------------- server-sent events

// eventStream writes server-sent events, with a time limit on each so a client that has
// gone can't hold a goroutine.
type eventStream struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func newEventStream(w http.ResponseWriter) *eventStream {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // a proxy mustn't hold it back
	w.WriteHeader(200)
	return &eventStream{w: w, rc: http.NewResponseController(w)}
}

func (e *eventStream) send(event string, v any) bool {
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	return e.raw(append(append(append(append([]byte("event: "), event...), "\ndata: "...), b...), "\n\n"...))
}

func (e *eventStream) raw(b []byte) bool {
	_ = e.rc.SetWriteDeadline(time.Now().Add(taskStreamWrite))
	if _, err := e.w.Write(b); err != nil {
		return false
	}
	return e.rc.Flush() == nil
}
