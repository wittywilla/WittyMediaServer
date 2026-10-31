package modules

import (
	"errors"
	"fmt"
	"sort"
)

// What someone can add to their server and take away again. Every module's code is
// compiled in, so adding one works straight away and offline, and costs nothing until
// it is used; what it decides is which libraries can be made and what shows in the
// app. A server that has never chosen has everything, so nothing changes for it.

// ServerManifests describe the kinds of media that are part of the server rather than
// projects of their own. They are listed in the store with the rest, so someone who
// only wants music can leave films out, and they update with the server.
func ServerManifests() [][]byte {
	const common = `"repo":"wittywilla/WittyMediaServer","delivery":"builtin","version":"0.1.0","api":1,"minServer":"0.1.0","maxServer":"0.x","stays":"All of it: this is part of the server, so a new server brings its new versions."`
	return [][]byte{
		[]byte(`{"id":"wittyvideo","name":"WittyVideo","group":"watch","summary":"Films, TV shows, anime and home videos, with posters, plots and continue watching.","libraries":["movies","shows","anime","videos","commercials"],` + common + `}`),
		[]byte(`{"id":"wittychannels","name":"WittyChannels","group":"watch","summary":"Creator videos, music videos and concerts, grouped by who made them.","libraries":["channels","musicvideos"],` + common + `}`),
		[]byte(`{"id":"wittyshorts","name":"WittyShorts","group":"watch","summary":"Vertical videos in a feed you swipe through.","libraries":["shorts"],` + common + `}`),
		[]byte(`{"id":"wittyaudio","name":"WittyAudio","group":"listen","summary":"Music, audiobooks that remember your place, and internet radio.","libraries":["music","audiobooks","radio"],` + common + `}`),
		[]byte(`{"id":"wittypodcasts","name":"WittyPodcasts","group":"listen","summary":"Podcasts you follow, downloaded for you.","libraries":["podcasts"],` + common + `}`),
	}
}

// LibraryType says whether a kind of library can be made here, and what it takes.
type LibraryType struct {
	Type      string      `json:"type"`
	Available bool        `json:"available"`
	Modules   []ModuleRef `json:"modules"` // any one of these makes it possible
}

// ModuleRef names a module for the store's links.
type ModuleRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
}

// ErrInUse: libraries depend on the module being removed.
type ErrInUse struct {
	Module    string
	Libraries []string
}

func (e *ErrInUse) Error() string {
	if len(e.Libraries) == 1 {
		return fmt.Sprintf("%s can't be removed while your library %s needs it. Remove that library first.", e.Module, e.Libraries[0])
	}
	return fmt.Sprintf("%s can't be removed while %d of your libraries need it (%s). Remove those libraries first.", e.Module, len(e.Libraries), joinNames(e.Libraries))
}

func joinNames(s []string) string {
	out := ""
	for i, n := range s {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

// LibraryTypes lists every kind of library a module declares, with whether it can be
// made. A type no module declares isn't listed, and is always available.
func (m *Manager) LibraryTypes() []LibraryType {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.libraryTypes()
}

func (m *Manager) libraryTypes() []LibraryType {
	byType := map[string]*LibraryType{}
	for _, id := range m.order {
		e := m.mods[id]
		for _, t := range e.builtin.Libraries {
			lt := byType[t]
			if lt == nil {
				lt = &LibraryType{Type: t, Modules: []ModuleRef{}}
				byType[t] = lt
			}
			on := !e.state.Removed
			lt.Modules = append(lt.Modules, ModuleRef{ID: id, Name: e.builtin.Name, Installed: on})
			lt.Available = lt.Available || on
		}
	}
	out := make([]LibraryType, 0, len(byType))
	for _, lt := range byType {
		out = append(out, *lt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// ErrModuleMissing: a library of this type needs a module that isn't installed.
type ErrModuleMissing struct {
	Type    string
	Modules []ModuleRef
}

func (e *ErrModuleMissing) Error() string {
	if len(e.Modules) == 0 {
		return "this kind of library isn't available on this server"
	}
	names := make([]string, len(e.Modules))
	for i, r := range e.Modules {
		names[i] = r.Name
	}
	return "this kind of library needs " + joinOr(names) + ". Add it in Settings, Modules."
}

func joinOr(s []string) string {
	switch len(s) {
	case 1:
		return s[0]
	case 2:
		return s[0] + " or " + s[1]
	}
	return joinNames(s[:len(s)-1]) + " or " + s[len(s)-1]
}

// LibraryAllowed says whether a library of this type can be made now; nil if it can.
func (m *Manager) LibraryAllowed(libType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, lt := range m.libraryTypes() {
		if lt.Type == libType && !lt.Available {
			return &ErrModuleMissing{Type: libType, Modules: lt.Modules}
		}
	}
	return nil
}

// Installed says whether a module is on this server. An unknown one counts as on: the
// server can't be told to do without something it has never heard of.
func (m *Manager) Installed(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	return e == nil || !e.state.Removed
}

// AddModule puts a module on this server. It works straight away, offline.
func (m *Manager) AddModule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	if !e.state.Removed {
		return nil
	}
	old := e.state
	e.state.Removed = false
	e.note("added", e.man.Version, "")
	if err := m.saveState(); err != nil {
		e.state = old
		return err
	}
	return nil
}

// RemoveModule takes a module off this server. libraries names the libraries there are,
// by type; if any would be left with no module, nothing changes and *ErrInUse says
// which. Its downloaded versions go, since the built-in copy is there if it comes back,
// and no data is touched.
func (m *Manager) RemoveModule(id string, libraries map[string][]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeModule(id, libraries)
}

func (m *Manager) removeModule(id string, libraries map[string][]string) error {
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	if e.builtin.Core {
		return errors.New(e.builtin.Name + " is what other modules are built on, so it stays")
	}
	if e.busy {
		return errors.New("an update of this module is being installed")
	}
	if e.state.Removed {
		return nil
	}
	if using := m.wouldStrand(id, e, libraries); len(using) > 0 {
		return &ErrInUse{Module: e.builtin.Name, Libraries: using}
	}
	old := e.state
	s := moduleState{Removed: true, DataHigh: old.DataHigh, Withdrawn: old.Withdrawn, Events: old.Events}
	e.state = s
	e.note("removed", e.man.Version, "")
	if err := m.saveState(); err != nil {
		e.state = old
		return err
	}
	m.resolve(id, e)
	e.latest, e.tooNew, e.rel = nil, nil, nil
	m.prune(id, e)
	return nil
}

// wouldStrand lists the libraries left with no module if this one goes. Call with
// m.mu held.
func (m *Manager) wouldStrand(id string, e *entry, libraries map[string][]string) []string {
	var using []string
	for _, t := range e.builtin.Libraries {
		other := false
		for _, oid := range m.order {
			o := m.mods[oid]
			if oid == id || o.state.Removed {
				continue
			}
			for _, ot := range o.builtin.Libraries {
				other = other || ot == t
			}
		}
		if !other {
			using = append(using, libraries[t]...)
		}
	}
	sort.Strings(using)
	return using
}

// SetInstalled makes the set of modules on this server what someone chose, as during
// first-run setup. It adds first, so a choice that moves libraries from one module to
// another isn't refused along the way. Core modules stay whatever the list says.
func (m *Manager) SetInstalled(ids []string, libraries map[string][]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[string]bool{}
	for _, id := range ids {
		if m.mods[id] == nil {
			return fmt.Errorf("%w: %q", ErrUnknown, id)
		}
		want[id] = true
	}
	// All of it or none of it: look at the end result before changing anything.
	for _, id := range m.order {
		e := m.mods[id]
		if want[id] || e.state.Removed || e.builtin.Core {
			continue
		}
		if e.busy {
			return errors.New("an update of " + e.builtin.Name + " is being installed")
		}
		var using []string
		for _, t := range e.builtin.Libraries {
			kept := false
			for _, oid := range m.order {
				o := m.mods[oid]
				if !o.state.Removed && (want[oid] || o.builtin.Core) || want[oid] {
					for _, ot := range o.builtin.Libraries {
						kept = kept || ot == t
					}
				}
			}
			if !kept {
				using = append(using, libraries[t]...)
			}
		}
		if len(using) > 0 {
			sort.Strings(using)
			return &ErrInUse{Module: e.builtin.Name, Libraries: using}
		}
	}
	for _, id := range m.order {
		e := m.mods[id]
		if want[id] && e.state.Removed {
			e.state.Removed = false
			e.note("added", e.man.Version, "")
		}
	}
	for _, id := range m.order {
		e := m.mods[id]
		if want[id] || e.state.Removed || e.builtin.Core {
			continue
		}
		if err := m.removeModule(id, libraries); err != nil {
			return err
		}
	}
	return m.saveState()
}

// requiredBy names installed modules that are built on this one.
func (m *Manager) requiredBy(id string) []string {
	var out []string
	for _, oid := range m.order {
		o := m.mods[oid]
		if o.state.Removed {
			continue
		}
		for _, r := range o.builtin.Requires {
			if r == id {
				out = append(out, o.builtin.Name)
			}
		}
	}
	return out
}
