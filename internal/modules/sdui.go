// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
	"github.com/wittywilla/wittymodule"
)

// The module API this server speaks. The numbers are the server's own: it decides what it
// takes, and the published wittymodule a server is built with may be older than the
// contract here (a test keeps wittymodule's from getting ahead of these).
//
// API 4 makes a module's SDUI part of the module (docs/SDUI-MODULES.md): a version built
// for it carries sdui.json, checked against the catalogue before it is used, and a version
// without one is refused. Versions built for API 3 keep working as they did; one that
// carries sdui.json anyway has it used.
//
// API 5 makes a module own its whole feature (docs/MODULES-API5.md): a version built for it
// carries integration.json, read by integration.go, saying which routes it answers, what its
// web bundle adds, its data and its lifecycle. The server keeps its own copy of a module's
// feature for versions built for 3 and 4 (legacyFor).
const (
	API            = 5
	MinAPI         = 3
	SDUIAPI        = 4
	IntegrationAPI = 5
)

// Methods a helper answers for its SDUI.
const (
	// MethodSDUIItems answers one of the module's data sources: ItemsRequest in,
	// ItemsAnswer out.
	MethodSDUIItems = "sdui.items"
	// MethodSDUIImage answers a picture the module's cards show: {"name"} in, the
	// picture as the call's blob.
	MethodSDUIImage = "sdui.image"
	// MethodSettingsChanged tells a running helper its settings changed: {"values"}.
	MethodSettingsChanged = "settings.changed"
)

// SettingsEnv names the environment variable a helper is started with: its server
// settings, as a JSON object.
const SettingsEnv = "WITTYMODULE_SETTINGS"

// needsUI reports whether a version must carry sdui.json: one built for API 4 or newer
// that the server runs or loads. A library reaches the server inside others.
func needsUI(man *wittymodule.Manifest) bool {
	if man.API < SDUIAPI {
		return false
	}
	switch man.Delivery {
	case wittymodule.DeliveryHelper, wittymodule.DeliveryProgram, wittymodule.DeliveryWeb:
		return true
	}
	return false
}

// readUI reads and checks a version's sdui.json from its folder: nil and no error when it
// has none and doesn't need one.
func readUI(dir string, man *wittymodule.Manifest, checks sdui.ModuleChecks) (*sdui.ModuleUI, error) {
	b, err := os.ReadFile(filepath.Join(dir, sdui.ModuleFile))
	if errors.Is(err, fs.ErrNotExist) {
		if needsUI(man) {
			return nil, fmt.Errorf("it is built for module API %d, where a module carries its own SDUI, and it has no %s", man.API, sdui.ModuleFile)
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ui, err := sdui.ParseModuleUI(b)
	if err != nil {
		return nil, err
	}
	if len(ui.Sources) > 0 && man.Delivery != wittymodule.DeliveryHelper {
		return nil, fmt.Errorf("its %s has data sources, and only a helper can answer them; it is delivered as %q", sdui.ModuleFile, man.Delivery)
	}
	if issues := sdui.ValidateModuleUI(ui, man.ID, checks); sdui.HasErrors(issues) {
		var words []string
		for _, is := range issues {
			if is.Severity == "error" && len(words) < 3 {
				words = append(words, is.Path+": "+is.Message)
			}
		}
		return nil, fmt.Errorf("its %s doesn't fit this server's catalogue: %s", sdui.ModuleFile, strings.Join(words, "; "))
	}
	return ui, nil
}

// UIs are the SDUI files of the modules in use, by module, and a number that changes
// whenever which versions are in use does, so a caller can keep what it makes of them.
func (m *Manager) UIs() ([]sdui.ModuleUI, uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []sdui.ModuleUI
	for _, id := range m.order {
		e := m.mods[id]
		if e.ui != nil && e.dir != "" && !e.state.Disabled {
			out = append(out, *e.ui)
		}
	}
	return out, m.uiGen
}

// UI is one module's SDUI in use, nil when it has none.
func (m *Manager) UI(id string) *sdui.ModuleUI {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.mods[id]; e != nil && e.dir != "" && !e.state.Disabled {
		return e.ui
	}
	return nil
}

// ItemsRequest is what a helper is asked for one of its data sources.
type ItemsRequest struct {
	// Source is the section's source, with every $variable filled in.
	Source map[string]any `json:"source"`
	// Filter holds the page's filter and sort parameters, for a filterable source.
	Filter map[string]string `json:"filter,omitempty"`
	Offset int               `json:"offset"`
	Limit  int               `json:"limit"`
	// Viewer is who is asking, so the module can leave out what isn't for them.
	Viewer Viewer `json:"viewer"`
	// Settings are the module's settings for this profile: the server's, then the
	// profile's own.
	Settings map[string]any `json:"settings,omitempty"`
}

// Viewer describes the profile a page is drawn for, as much as a module needs.
type Viewer struct {
	// User is the account the page is for, for a module's own data about that person.
	User        int64  `json:"user,omitempty"`
	ProfileKind string `json:"profileKind"`
	Admin       bool   `json:"admin,omitempty"`
	// Cannot are the parental feature switches that are off for it.
	Cannot     []string `json:"cannot,omitempty"`
	FormFactor string   `json:"formFactor,omitempty"`
	Platform   string   `json:"platform,omitempty"`
	Locale     string   `json:"locale,omitempty"`
}

// ItemsAnswer is a helper's answer.
type ItemsAnswer struct {
	Items []sdui.Item `json:"items"`
	More  bool        `json:"more,omitempty"`
	Total int         `json:"total,omitempty"`
}

// ErrNotRunning is a module's SDUI call when the module has no helper running here.
var ErrNotRunning = errors.New("the module isn't running on this server")

// UIItems asks a module's helper for one of its sources.
func (m *Manager) UIItems(id string, req ItemsRequest) (ItemsAnswer, error) {
	var out ItemsAnswer
	_, err, handled := m.call(id, MethodSDUIItems, req, &out)
	if !handled {
		return out, ErrNotRunning
	}
	return out, err
}

// MaxUIImage bounds a picture a module answers.
const MaxUIImage = 4 << 20

// UIImage asks a module's helper for a picture its cards show.
func (m *Manager) UIImage(id, name string) ([]byte, error) {
	blob, err, handled := m.call(id, MethodSDUIImage, map[string]string{"name": name}, nil)
	if !handled {
		return nil, ErrNotRunning
	}
	if err != nil {
		return nil, err
	}
	if len(blob) == 0 || len(blob) > MaxUIImage {
		return nil, errors.New("the module answered no picture, or one too large")
	}
	return blob, nil
}

// SettingsChanged tells a module's running helper its settings changed. A helper that
// isn't running reads them when it starts; one that doesn't know the call is fine.
func (m *Manager) SettingsChanged(id string, values map[string]any) {
	m.mu.Lock()
	e := m.mods[id]
	running := e != nil && e.proc != nil
	m.mu.Unlock()
	if running {
		_, _, _ = m.call(id, MethodSettingsChanged, map[string]any{"values": values}, nil)
	}
}

// Name is a module's name, for an app to show; its ID when it has none.
func (m *Manager) Name(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.mods[id]; e != nil && e.man != nil && e.man.Name != "" {
		return e.man.Name
	}
	return id
}
