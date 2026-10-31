package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// How each module is logged. Every module the server knows about is listed in
// Settings → WittyLog with a level of its own and its advanced options, and
// a module that turns up later (installed, updated, or part of a new version of
// the server) is noticed and listed without anyone asking. See docs/LOGGING.md.

// LogTopic is one advanced option of a module: a kind of detail that can be turned
// on by itself, without making everything else in that module detailed too.
type LogTopic struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	On          bool   `json:"on"`
}

// LogModule is what the settings page shows for one module.
type LogModule struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Version   string     `json:"version"`
	Delivery  string     `json:"delivery,omitempty"`
	New       bool       `json:"new,omitempty"`     // appeared since an admin last looked
	Missing   bool       `json:"missing,omitempty"` // was here once and isn't now
	FirstSeen time.Time  `json:"firstSeen"`
	Level     string     `json:"level"` // "" follows its area
	Topics    []LogTopic `json:"topics"`
	Lines     int        `json:"lines"` // of its messages in memory just now
	Warnings  int        `json:"warnings"`
	Errors    int        `json:"errors"`
}

// What a helper process can offer, whichever module it is: all three are done by
// the server around the helper, so they work without the module's help.
var helperTopics = []LogTopic{
	{ID: "calls", Label: "Every call", Description: "Each thing asked of this helper and how long it took."},
	{ID: "output", Label: "Everything it prints", Description: "What the helper writes, line by line, not just when it fails. May include file names."},
	{ID: "lifecycle", Label: "Starting and stopping", Description: "Each time the helper process starts or stops, and why."},
}

// What the built-in engines log, for those with more to say than their area's
// level gives. A module from elsewhere lists its own in its manifest, under
// capabilities.logTopics; see moduleTopics.
var engineTopics = map[string][]LogTopic{
	"wittydvd": {
		{ID: "nav", Label: "Navigation", Description: "Every step the DVD navigator takes: program chains, cells and commands."},
		{ID: "menus", Label: "Menus and buttons", Description: "Each menu read from the disc, and each button pressed."},
	},
	"wittybluray": {
		{ID: "nav", Label: "Navigation", Description: "Every step the Blu-ray navigator takes: titles, playlists and commands."},
		{ID: "menus", Label: "Menus and buttons", Description: "Each menu read from the disc, and each button pressed."},
		{ID: "bdj", Label: "Java menus", Description: "What a disc's Java program prints, and what it asks the player to do."},
	},
	"wittyplay": {
		{ID: "core", Label: "Emulator messages", Description: "What the emulator core says as it runs. Often a lot."},
		{ID: "runner", Label: "Game runner", Description: "What the separate process that runs the game prints."},
	},
	"wittytuner": {
		{ID: "tuner", Label: "Tuning", Description: "Each tune, scan and signal report from tuners."},
	},
}

type logModuleState struct {
	Version   string          `json:"version"`
	Name      string          `json:"name"`
	New       bool            `json:"new,omitempty"`
	Missing   bool            `json:"missing,omitempty"`
	FirstSeen time.Time       `json:"firstSeen"`
	Level     string          `json:"level,omitempty"`
	Topics    map[string]bool `json:"topics,omitempty"`
}

// logModules is the record of every module seen, kept in the settings table.
type logModules struct {
	mu     sync.Mutex
	loaded bool
	state  map[string]*logModuleState
}

const logModulesKey = "logModules"

// moduleSeen is one module as the server has it now.
type moduleSeen struct {
	id, name, version, delivery string
}

func (s *Server) currentModules() []moduleSeen {
	var out []moduleSeen
	if s.Modules != nil {
		for _, m := range s.Modules.Report().Modules {
			out = append(out, moduleSeen{m.ID, m.Name, m.Version, m.Delivery})
		}
		return out
	}
	for _, m := range builtModules() {
		id := m.Module[strings.LastIndexByte(m.Module, '/')+1:]
		out = append(out, moduleSeen{id, m.Name, m.Version, "builtin"})
	}
	return out
}

// syncLogModules compares what's installed with what was last seen, says what has
// changed, and makes sure the logger is using the owner's choices. The very first
// time, everything simply becomes the starting point.
func (s *Server) syncLogModules(ctx context.Context) {
	if s.DB == nil || s.Logs == nil {
		return
	}
	lm := &s.logMods
	lm.mu.Lock()
	defer lm.mu.Unlock()
	first := false
	changed := false
	if !lm.loaded {
		lm.state = map[string]*logModuleState{}
		raw := s.DB.GetSetting(ctx, logModulesKey, "")
		first = raw == ""
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &lm.state); err != nil {
				s.Log.Warn("saved module log settings can't be read", "cat", "modules", "err", err)
				lm.state = map[string]*logModuleState{}
			}
		}
		lm.loaded = true
		s.applyModuleLogs()
	}
	present := map[string]bool{}
	for _, m := range s.currentModules() {
		present[m.id] = true
		st := lm.state[m.id]
		log := s.Log.With("cat", "modules", "module", m.id, "version", m.version)
		switch {
		case st == nil:
			lm.state[m.id] = &logModuleState{Version: m.version, Name: m.name, New: !first, FirstSeen: time.Now()}
			changed = true
			if !first {
				log.Info("a new module was found: its logging is ready in Settings, WittyLog", "name", m.name)
			}
		case st.Missing:
			st.Missing, st.Version, st.Name, st.New = false, m.version, m.name, true
			changed = true
			log.Info("a module has come back", "name", m.name)
		case st.Version != m.version:
			log.Info("a module's version changed", "name", m.name, "from", st.Version)
			st.Version, st.Name = m.version, m.name
			changed = true
		}
	}
	for id, st := range lm.state {
		if !present[id] && !st.Missing {
			st.Missing = true
			changed = true
			s.Log.Info("a module is no longer here", "cat", "modules", "module", id, "version", st.Version)
		}
	}
	if changed {
		s.saveLogModules(ctx)
	}
}

// Call with the lock held.
func (s *Server) applyModuleLogs() {
	want := make(map[string]logging.ModuleLog, len(s.logMods.state))
	for id, st := range s.logMods.state {
		if st.Level != "" || len(st.Topics) > 0 {
			want[id] = logging.ModuleLog{Level: st.Level, Topics: st.Topics}
		}
	}
	s.Logs.SetModules(want)
}

// Call with the lock held.
func (s *Server) saveLogModules(ctx context.Context) {
	b, err := json.Marshal(s.logMods.state)
	if err != nil {
		return
	}
	if err := s.DB.SetSetting(ctx, logModulesKey, string(b)); err != nil {
		s.Log.ErrorContext(ctx, "could not save the module log settings", "cat", "modules", "err", err)
	}
}

// moduleTopics is the list of advanced options for a module: what any helper has,
// what the built-in engines list here, and what the module's own manifest declares
// under capabilities.logTopics, as "id|label|description".
func (s *Server) moduleTopics(id, delivery string) []LogTopic {
	var out []LogTopic
	if delivery == "helper" {
		out = append(out, helperTopics...)
	}
	out = append(out, engineTopics[id]...)
	if s.Modules != nil {
		seen := map[string]bool{}
		for _, t := range out {
			seen[t.ID] = true
		}
		for _, raw := range s.Modules.LogTopics(id) {
			parts := strings.SplitN(raw, "|", 3)
			tid := logging.ValidSession(parts[0])
			if tid == "" || seen[tid] || len(out) >= 32 {
				continue
			}
			seen[tid] = true
			t := LogTopic{ID: tid, Label: tid}
			if len(parts) > 1 {
				t.Label = cutRunes(parts[1], 60)
			}
			if len(parts) > 2 {
				t.Description = cutRunes(parts[2], 300)
			}
			out = append(out, t)
		}
	}
	return out
}

// logModuleViews is the list the settings page shows: new ones first, then by name.
func (s *Server) logModuleViews(ctx context.Context) []LogModule {
	s.syncLogModules(ctx)
	out := []LogModule{}
	if s.Logs == nil {
		return out
	}
	activity := s.Logs.ModuleActivity()
	current := map[string]moduleSeen{}
	for _, m := range s.currentModules() {
		current[m.id] = m
	}
	lm := &s.logMods
	lm.mu.Lock()
	defer lm.mu.Unlock()
	for id, st := range lm.state {
		v := LogModule{ID: id, Name: st.Name, Version: st.Version, New: st.New, Missing: st.Missing, FirstSeen: st.FirstSeen, Level: st.Level, Topics: []LogTopic{}}
		v.Delivery = current[id].delivery
		for _, t := range s.moduleTopics(id, v.Delivery) {
			t.On = st.Topics[t.ID]
			v.Topics = append(v.Topics, t)
		}
		a := activity[id]
		v.Lines, v.Warnings, v.Errors = a.Lines, a.Warnings, a.Errors
		out = append(out, v)
	}
	sortLogModules(out)
	return out
}

func sortLogModules(list []LogModule) {
	less := func(a, b *LogModule) bool {
		if a.New != b.New {
			return a.New
		}
		if a.Missing != b.Missing {
			return b.Missing
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && less(&list[j], &list[j-1]); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// moduleLogChange is what PUT /api/logs takes for one module. A nil Level leaves
// it as it is, and "" follows its area again. Topics not named are left as they are.
type moduleLogChange struct {
	Level  *string         `json:"level"`
	Topics map[string]bool `json:"topics"`
}

// changeLogModules applies what the settings page sent. It returns a plain reason
// if something can't be done, having changed nothing.
func (s *Server) changeLogModules(ctx context.Context, changes map[string]moduleLogChange, seen bool) string {
	s.syncLogModules(ctx)
	lm := &s.logMods
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if lm.state == nil {
		return "module logging isn't available"
	}
	for id, c := range changes {
		if lm.state[id] == nil {
			return "there is no module called " + cutRunes(id, 60)
		}
		if c.Level != nil && *c.Level != "" {
			if _, ok := logging.ParseLevel(*c.Level); !ok {
				return "level must be debug, info, warn, error or off, or empty to follow its area"
			}
		}
	}
	for id, c := range changes {
		st := lm.state[id]
		if c.Level != nil {
			st.Level = strings.ToLower(*c.Level)
		}
		if len(c.Topics) > 0 {
			known := map[string]bool{}
			for _, t := range s.moduleTopics(id, s.deliveryOf(id)) {
				known[t.ID] = true
			}
			for t, on := range c.Topics {
				switch {
				case !known[t]:
				case on:
					if st.Topics == nil {
						st.Topics = map[string]bool{}
					}
					st.Topics[t] = true
				default:
					delete(st.Topics, t)
				}
			}
		}
		s.Log.InfoContext(ctx, "log settings changed for a module", "cat", "modules", "module", id, "moduleLevel", st.Level, "options", topicList(st.Topics))
	}
	if seen {
		for _, st := range lm.state {
			st.New = false
		}
	}
	s.applyModuleLogs()
	s.saveLogModules(ctx)
	return ""
}

func (s *Server) deliveryOf(id string) string {
	for _, m := range s.currentModules() {
		if m.id == id {
			return m.delivery
		}
	}
	return ""
}

func topicList(m map[string]bool) string {
	var b strings.Builder
	for t, on := range m {
		if on {
			if b.Len() > 0 {
				b.WriteByte(',')
			}
			b.WriteString(t)
		}
	}
	return b.String()
}

// WatchModules notices modules that arrive by themselves, such as an automatic
// update. It only reads the module list, so this is cheap.
func (s *Server) WatchModules(ctx context.Context) {
	logging.Loop("module watch", func() {
		s.syncLogModules(ctx)
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.syncLogModules(ctx)
			}
		}
	}, "cat", "modules")
}
