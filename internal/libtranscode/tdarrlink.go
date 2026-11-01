package libtranscode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/libtranscode/tdarr"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// The connection to someone's own Tdarr server: settings, what's going on there, and
// handing it files. It's off until someone connects it, and with it off nothing here
// runs or contacts anything.

// TdarrTaskType is how Tdarr's work appears in the activity panel.
const TdarrTaskType = "tdarr"

const (
	tdarrKey       = "libtranscode.tdarr"
	tdarrSecretKey = TdarrKeySetting
)

// TdarrKeySetting is where the API key is kept.
const TdarrKeySetting = "libtranscode.tdarrKey"

// TdarrMapping says which of Tdarr's libraries one of this server's libraries goes to.
type TdarrMapping struct {
	LibraryID int64  `json:"libraryId"`
	TdarrID   string `json:"tdarrId"`
	// Folder as it's called here, and as Tdarr calls it, for when they differ.
	From string `json:"from"`
	To   string `json:"to"`
}

// TdarrSettings is the connection. The key is kept apart and never sent back.
type TdarrSettings struct {
	Enabled  bool           `json:"enabled"`
	URL      string         `json:"url"`
	KeySet   bool           `json:"keySet"`
	Mappings []TdarrMapping `json:"mappings"`
}

// LoadTdarr reads the connection's settings.
func (e *Engine) LoadTdarr(ctx context.Context) TdarrSettings {
	s := TdarrSettings{Mappings: []TdarrMapping{}}
	if raw := e.DB.GetSetting(ctx, tdarrKey, ""); raw != "" {
		_ = json.Unmarshal([]byte(raw), &s)
	}
	if s.Mappings == nil {
		s.Mappings = []TdarrMapping{}
	}
	s.KeySet = e.DB.GetSetting(ctx, tdarrSecretKey, "") != ""
	return s
}

// SaveTdarr stores the connection. An empty key leaves the old one, and clear removes it.
func (e *Engine) SaveTdarr(ctx context.Context, s TdarrSettings, key string, clear bool) error {
	if s.URL != "" {
		base, err := tdarr.CleanAddress(s.URL)
		if err != nil {
			return err
		}
		s.URL = base
	}
	if s.Enabled && s.URL == "" {
		return errors.New("enter Tdarr's address first")
	}
	if len(s.Mappings) > 256 {
		return errors.New("too many libraries")
	}
	for _, m := range s.Mappings {
		if len(m.TdarrID) > 256 || len(m.From) > 2048 || len(m.To) > 2048 {
			return errors.New("that library setting is too long")
		}
	}
	if len(key) > 512 {
		return errors.New("that API key is too long")
	}
	// A key belongs to the address it was typed for: at a new address, it's asked for again.
	if key == "" && s.URL != e.LoadTdarr(ctx).URL {
		clear = true
	}
	s.KeySet = false
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := e.DB.SetSetting(ctx, tdarrKey, string(b)); err != nil {
		return err
	}
	switch {
	case clear:
		if err := e.DB.SetSetting(ctx, tdarrSecretKey, ""); err != nil {
			return err
		}
	case key != "":
		if err := e.DB.SetSetting(ctx, tdarrSecretKey, key); err != nil {
			return err
		}
	}
	e.WatchTdarr()
	return nil
}

func (e *Engine) tdarrClient(ctx context.Context, address, key string) (*tdarr.Client, error) {
	s := e.LoadTdarr(ctx)
	if address == "" {
		address = s.URL
	}
	// The saved key goes to the saved address and no other, whatever is typed. It's a secret of
	// someone else's program, and this page never shows it.
	if key == "" {
		if base, err := tdarr.CleanAddress(address); err == nil && base == s.URL {
			key = e.DB.GetSetting(ctx, tdarrSecretKey, "")
		}
	}
	return tdarr.New(address, key)
}

// TdarrReport is a look at a Tdarr server.
type TdarrReport struct {
	OK        bool            `json:"ok"`
	Error     string          `json:"error,omitempty"`
	Version   string          `json:"version,omitempty"`
	Status    string          `json:"status,omitempty"`
	Nodes     []tdarr.Node    `json:"nodes"`
	Libraries []tdarr.Library `json:"libraries"`
}

// TdarrLook asks a Tdarr server about itself. An address or key passed here is tried
// without being saved, so someone can test before they save.
func (e *Engine) TdarrLook(ctx context.Context, address, key string) TdarrReport {
	r := TdarrReport{Nodes: []tdarr.Node{}, Libraries: []tdarr.Library{}}
	c, err := e.tdarrClient(ctx, address, key)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	st, err := c.GetStatus(ctx)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.OK, r.Version, r.Status = true, st.Version, st.Status
	// A server that answers its status may still refuse these, and that's worth saying.
	if nodes, err := c.GetNodes(ctx); err == nil {
		r.Nodes = nodes
	} else {
		r.Error = err.Error()
	}
	if libs, err := c.GetLibraries(ctx); err == nil {
		r.Libraries = libs
	} else if r.Error == "" {
		r.Error = err.Error()
	}
	return r
}

// SendToTdarr hands a library's files to Tdarr, by the names Tdarr knows them by. Tdarr
// then does what its own plugins and flows say: nothing here chooses that.
func (e *Engine) SendToTdarr(ctx context.Context, libraryID int64, items []int64) (string, error) {
	s := e.LoadTdarr(ctx)
	if !s.Enabled {
		return "", errors.New("connect Tdarr first")
	}
	var m *TdarrMapping
	for i := range s.Mappings {
		if s.Mappings[i].LibraryID == libraryID && s.Mappings[i].TdarrID != "" {
			m = &s.Mappings[i]
		}
	}
	if m == nil {
		return "", errors.New("choose which of Tdarr's libraries this one goes to first")
	}
	c, err := e.tdarrClient(ctx, "", "")
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		lib, err := e.Store.Library(ctx, libraryID)
		if err != nil {
			return "", err
		}
		folder := ""
		if len(lib.Paths) > 0 {
			folder, _ = tdarr.Translate(lib.Paths[0], m.From, m.To)
		}
		if err := c.ScanLibrary(ctx, m.TdarrID, folder); err != nil {
			return "", err
		}
		return "Tdarr has been asked to look through this library for files it hasn't seen.", nil
	}
	all, err := e.Store.ConversionItems(ctx, libraryID)
	if err != nil {
		return "", err
	}
	var paths []string
	skipped := 0
	for _, it := range all {
		for _, want := range items {
			if it.ID == want {
				if p, ok := tdarr.Translate(it.Path, m.From, m.To); ok && it.Kind == "video" {
					paths = append(paths, p)
				} else {
					skipped++
				}
			}
		}
	}
	if len(paths) == 0 {
		return "", errors.New("none of those files could be named in Tdarr's terms. Check the folder names for this library")
	}
	if err := c.SendFiles(ctx, m.TdarrID, paths); err != nil {
		return "", err
	}
	if skipped > 0 {
		return fmt.Sprintf("Sent %d files to Tdarr. %d couldn't be named in Tdarr's terms and were left out.", len(paths), skipped), nil
	}
	return fmt.Sprintf("Sent %d files to Tdarr.", len(paths)), nil
}

// tdarrWatch follows what a connected Tdarr is doing, for the activity panel. There's no
// way to be told, so it asks: every 20 seconds with someone watching the panel, and every
// 2 minutes otherwise. With no connection it doesn't exist.
type tdarrWatch struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

// WatchTdarr starts following Tdarr, or stops, according to the settings.
func (e *Engine) WatchTdarr() {
	e.init()
	w := &e.tdarrW
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	if !e.LoadTdarr(context.Background()).Enabled {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	logging.Go("tdarr watch", func() { e.followTdarr(ctx) }, "cat", "transcode")
}

// StopTdarr stops following Tdarr.
func (e *Engine) StopTdarr() {
	e.init()
	w := &e.tdarrW
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}

func (e *Engine) followTdarr(ctx context.Context) {
	var t *tasks.Handle
	finish := func(result string, err error) {
		if t == nil {
			return
		}
		t.Result(result)
		t.Finish(err)
		t = nil
	}
	defer func() { finish("", nil) }()
	misses := 0
	for {
		nodes, err := e.tdarrNodes(ctx)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			misses++
			// A restart of Tdarr isn't worth a word. Being gone for good is.
			if misses == 3 && t != nil {
				finish("Tdarr stopped answering, so there's no telling what it's doing now.", nil)
			}
		default:
			misses = 0
			active, percent, item, path := tdarrActivity(nodes)
			switch {
			case active == 0:
				finish("Tdarr finished what it was doing.", nil)
			default:
				if t == nil {
					t = e.Tasks.Start(tasks.Spec{Type: TdarrTaskType, Title: "Tdarr is working", Unit: "percent", AdminOnly: true, Quiet: true})
				}
				t.Phase(tdarrPhase(active, len(nodes)))
				t.Progress(int64(percent), 100)
				t.Item(item, path)
			}
		}
		wait := 2 * time.Minute
		if e.Tasks.Watchers() > 0 || t != nil {
			wait = 20 * time.Second
		}
		if e.TdarrPoll > 0 {
			wait = e.TdarrPoll
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) tdarrNodes(ctx context.Context) ([]tdarr.Node, error) {
	c, err := e.tdarrClient(ctx, "", "")
	if err != nil {
		return nil, err
	}
	return c.GetNodes(ctx)
}

// tdarrActivity sums up the nodes: how many workers have a file, how far along on average,
// and one file to name.
func tdarrActivity(nodes []tdarr.Node) (active int, percent float64, item, path string) {
	var sum float64
	for _, n := range nodes {
		for _, w := range n.Workers {
			if w.File == "" {
				continue
			}
			active++
			sum += w.Percent
			if item == "" {
				path = w.File
				item = filepath.Base(strings.ReplaceAll(w.File, `\`, `/`))
			}
		}
	}
	if active > 0 {
		percent = sum / float64(active)
	}
	return
}

func tdarrPhase(active, nodes int) string {
	files := "files"
	if active == 1 {
		files = "file"
	}
	on := "nodes"
	if nodes == 1 {
		on = "node"
	}
	return fmt.Sprintf("%d %s on %d %s", active, files, nodes, on)
}
