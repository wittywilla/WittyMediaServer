// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// A module's SDUI: what one module adds to the app, carried in the module's own
// release as sdui.json (docs/SDUI-MODULES.md). Everything a module adds is named
// after it, is checked against the catalogue before the module is used, and is
// made of the catalogue's components, so every app draws it with nothing of the
// module's own: a module is portable because its UI is data.

// ModuleFile is the name of a module's SDUI file, beside module.json in a release.
const ModuleFile = "sdui.json"

// ModuleFormat and ModuleFormatVersion mark a module's SDUI file.
const (
	ModuleFormat        = "wittymodule-sdui"
	ModuleFormatVersion = 1
)

// MaxModuleBytes bounds a module's SDUI file.
const MaxModuleBytes = 1 << 20

// Limits on what one module may add.
const (
	maxModulePages      = 32
	maxModuleSources    = 64
	maxModuleComponents = 64
	maxModuleNav        = 16
	maxModuleSettings   = 16
	maxModuleRows       = 64
	maxModuleViews      = 32
	maxModuleInserts    = 32
	maxModulePresets    = 64
)

// ItemKinds are the kinds of item a data source may give.
var ItemKinds = []string{"movie", "episode", "show", "season", "video", "disc", "album", "artist", "track", "book", "audiobook", "podcast", "game", "photo", "channel", "recording", "collection", "library", "genre", "view"}

// ModuleUI is one module's SDUI file.
type ModuleUI struct {
	// Format is ModuleFormat, FormatVersion ModuleFormatVersion.
	Format        string `json:"format"`
	FormatVersion int    `json:"formatVersion"`
	// Module is the module's ID, the same as its manifest's.
	Module string `json:"module"`
	// Catalog is the catalogue revision the module was written against. A
	// server refuses a file newer than its own catalogue.
	Catalog int `json:"catalog"`
	// Theme is the module's theme scope. Required.
	Theme ModuleTheme `json:"theme"`
	// Pages are whole pages the module adds: its own (kind custom, opened at
	// /page/{id}), or a page for a kind of library or title (a games library
	// page, the detail page of a game), or the page of a kind the module is
	// about (games, photos, books, music, livetv).
	Pages []Page `json:"pages,omitempty"`
	// Inserts put one of the module's sections into a built-in page, such as a
	// row on Home. An administrator's own page is never changed by a module.
	Inserts []ModuleInsert `json:"inserts,omitempty"`
	// Presets are sections offered in the page editor's gallery.
	Presets []ModulePreset `json:"presets,omitempty"`
	// Components are the module's named sections: a section of catalogue
	// components with properties of its own, offered in the editor's palette
	// and expanded by the server, so no app needs to know them.
	Components []ModuleComponent `json:"components,omitempty"`
	// Sources are the data sources the module answers, through its helper.
	Sources []ModuleSource `json:"sources,omitempty"`
	// Views are views of the module's own that an app may draw in a
	// NativeView (the module's web bundle, for one); every other app draws the
	// NativeView's sections instead.
	Views []ModuleView `json:"views,omitempty"`
	// Navigation entries the module adds to the main menu.
	Navigation []NavItem `json:"navigation,omitempty"`
	// Settings describe the module's settings, which every app's own Settings
	// draws in its own style under Settings, Modules. They are never SDUI pages.
	Settings []SettingsPage `json:"settings,omitempty"`
}

// ModuleTheme is a module's theme scope: its ID, a name to show in the theme
// editor, and the colours the module would like, which an administrator's own
// overrides for the scope replace token by token.
type ModuleTheme struct {
	Scope  string            `json:"scope"`
	Label  string            `json:"label,omitempty"`
	Tokens map[string]string `json:"tokens,omitempty"`
}

// ModuleInsert puts a section into a built-in page.
type ModuleInsert struct {
	// Page is the built-in page's ID: home, music, games, ...
	Page string `json:"page"`
	// After or Before name a section of that page; neither: at the end.
	After   string  `json:"after,omitempty"`
	Before  string  `json:"before,omitempty"`
	Section Section `json:"section"`
}

// ModulePreset is a section for the editor's gallery.
type ModulePreset struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Description string  `json:"description,omitempty"`
	Section     Section `json:"section"`
}

// ModuleComponent is a named section with properties of its own. Its Section
// is a template: a value written "{{name}}" becomes the property's value, and
// "{{name}}" inside text is replaced by it.
type ModuleComponent struct {
	// Type is "<module>.<Name>": wittyplay.ConsoleShelf.
	Type        string     `json:"type"`
	Label       string     `json:"label"`
	Description string     `json:"description,omitempty"`
	Icon        string     `json:"icon,omitempty"`
	Props       []PropSpec `json:"props,omitempty"`
	// PageKinds are the kinds of page it may be used on; empty: any.
	PageKinds []string `json:"pageKinds,omitempty"`
	Section   Section  `json:"section"`
}

// ModuleSource is a data source a module answers: the catalogue's description
// of a source, with Type "<module>.<name>".
type ModuleSource struct {
	SourceSpec
}

// ModuleView is a view of the module's own for a NativeView.
type ModuleView struct {
	// ID is "<module>.<name>"; it is what a NativeView's view prop names and
	// what an app that can draw it sends in views=.
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	// Platforms that can draw it, as a hint for the editor; empty: unknown.
	Platforms []string `json:"platforms,omitempty"`
}

// SettingsPage is one page of a module's settings, as every app's own
// Settings draws it: a title, and groups of rows.
type SettingsPage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"`
	// Admin pages are shown only to administrators.
	Admin  bool            `json:"admin,omitempty"`
	Groups []SettingsGroup `json:"groups"`
}

// SettingsGroup is a titled group of rows.
type SettingsGroup struct {
	Title  string       `json:"title,omitempty"`
	Footer string       `json:"footer,omitempty"`
	Rows   []SettingRow `json:"rows"`
}

// Kinds of setting.
const (
	SettingBool   = "bool"   // a switch
	SettingEnum   = "enum"   // one of Options
	SettingInt    = "int"    // a whole number between Min and Max
	SettingString = "string" // a line of text
	SettingSecret = "secret" // a password or key: never shown again once saved
	SettingInfo   = "info"   // a line of text to read, with no value
)

// SettingKinds are the kinds of setting row.
var SettingKinds = []string{SettingBool, SettingEnum, SettingInt, SettingString, SettingSecret, SettingInfo}

// Where a setting's value is kept.
const (
	SettingScopeServer  = "server"  // one value for the server; only administrators change it
	SettingScopeProfile = "profile" // each profile its own
)

// SettingRow is one setting.
type SettingRow struct {
	// Key names the value: lower-case first letter, then letters, digits, . _ -.
	Key   string `json:"key"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Help  string `json:"help,omitempty"`
	// Scope is server (the default) or profile.
	Scope   string       `json:"scope,omitempty"`
	Default any          `json:"default,omitempty"`
	Options []EnumOption `json:"options,omitempty"`
	Min     *int         `json:"min,omitempty"`
	Max     *int         `json:"max,omitempty"`
	// Unit follows a number: "MB", "seconds".
	Unit      string `json:"unit,omitempty"`
	MaxLength int    `json:"maxLength,omitempty"`
}

var (
	moduleIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9]{1,63}$`)
	sourceNamePattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,47}$`)
	compNamePattern   = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,47}$`)
	settingKeyPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9_.-]{0,63}$`)
	placeholderRe     = regexp.MustCompile(`\{\{([A-Za-z][A-Za-z0-9_]{0,47})\}\}`)
)

// ValidModuleID reports whether s can be a module's ID.
func ValidModuleID(s string) bool { return moduleIDPattern.MatchString(s) }

// ModuleOf is the module a namespaced name belongs to: "wittyplay" for
// wittyplay.ConsoleShelf or wittyplay.recent, "" for a name of the catalogue.
func ModuleOf(name string) string {
	mod, _, ok := strings.Cut(name, ".")
	if !ok || !ValidModuleID(mod) {
		return ""
	}
	return mod
}

// ownedID reports whether a page, section, entry or settings page ID belongs to
// module: the module's own ID, or it followed by - and more.
func ownedID(module, id string) bool {
	return id == module || strings.HasPrefix(id, module+"-")
}

// ModulePageKinds are the kinds of page a module may add. Home, search, the
// libraries and collections pages stay the server's; a module adds to Home
// with an insert.
var ModulePageKinds = []string{"custom", "library", "detail", "livetv", "music", "photos", "books", "games"}

// ParseModuleUI reads a module's SDUI file strictly: unknown fields, trailing
// data and a file over MaxModuleBytes are refused. It doesn't check the
// content; ValidateModuleUI does.
func ParseModuleUI(b []byte) (*ModuleUI, error) {
	if len(b) > MaxModuleBytes {
		return nil, fmt.Errorf("%s is %d bytes; the most is %d", ModuleFile, len(b), MaxModuleBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var ui ModuleUI
	if err := dec.Decode(&ui); err != nil {
		return nil, fmt.Errorf("%s can't be read: %w", ModuleFile, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%s has something after its end", ModuleFile)
	}
	return &ui, nil
}

// ModuleChecks are the server's own checks a module's file is held to that
// this package can't make: the theme engine's colour tokens.
type ModuleChecks struct {
	// ColorToken reports whether a name is a colour token themes have; Color
	// whether a value is a colour. nil: not checked.
	ColorToken func(string) bool
	Color      func(string) bool
}

// ValidateModuleUI checks one module's SDUI file on its own: the names are the
// module's, everything is from the catalogue or the module itself, and every
// page, insert, preset and component validates. A module may use only its own
// components and sources, never another module's, so it works wherever it is
// installed.
func ValidateModuleUI(ui *ModuleUI, module string, checks ModuleChecks) []Issue {
	v := &validator{page: &Page{Kind: "custom"}}
	if ui.Format != ModuleFormat {
		v.err("/format", "Write \"format\": \""+ModuleFormat+"\".", "%q is not the format of a module's SDUI file", ui.Format)
	}
	if ui.FormatVersion != ModuleFormatVersion {
		v.err("/formatVersion", fmt.Sprintf("Write \"formatVersion\": %d.", ModuleFormatVersion), "this server reads format version %d, not %d", ModuleFormatVersion, ui.FormatVersion)
	}
	if !ValidModuleID(ui.Module) {
		v.err("/module", "Use the module's ID from module.json.", "%q is not a module ID", ui.Module)
	} else if module != "" && ui.Module != module {
		v.err("/module", "Use the module's ID from module.json.", "the file is for %q, and the module is %q", ui.Module, module)
	}
	mod := ui.Module
	if ui.Catalog < 1 || ui.Catalog > CatalogRevision {
		v.err("/catalog", fmt.Sprintf("Write the catalogue revision it was made for, 1 to %d.", CatalogRevision), "catalog %d is not a revision this server knows; it knows up to %d", ui.Catalog, CatalogRevision)
	}
	// The theme scope.
	if ui.Theme.Scope != mod {
		v.err("/theme/scope", "Use the module's ID as its scope.", "a module's theme scope is its own ID, %q", mod)
	}
	if len([]rune(ui.Theme.Label)) > 60 {
		v.err("/theme/label", "Shorten it.", "a label is at most 60 characters")
	}
	for _, k := range sortedTokenKeys(ui.Theme.Tokens) {
		val := ui.Theme.Tokens[k]
		if checks.ColorToken != nil && !checks.ColorToken(k) {
			v.err("/theme/tokens/"+k, "Use a colour token themes have, such as accent.", "%q is not a colour token", k)
		}
		if checks.Color != nil && !checks.Color(val) {
			v.err("/theme/tokens/"+k, "Use #rrggbb, #rrggbbaa or rgb()/rgba().", "%q is not a colour", val)
		}
	}
	// Limits.
	for _, l := range []struct {
		path    string
		n, most int
	}{{"/pages", len(ui.Pages), maxModulePages}, {"/sources", len(ui.Sources), maxModuleSources}, {"/components", len(ui.Components), maxModuleComponents},
		{"/navigation", len(ui.Navigation), maxModuleNav}, {"/settings", len(ui.Settings), maxModuleSettings}, {"/views", len(ui.Views), maxModuleViews},
		{"/inserts", len(ui.Inserts), maxModuleInserts}, {"/presets", len(ui.Presets), maxModulePresets}} {
		if l.n > l.most {
			v.err(l.path, "Split the module, or leave some out.", "a module adds at most %d of these, not %d", l.most, l.n)
		}
	}
	// The module's own set: what its pages may use.
	self := NewModuleSet([]ModuleUI{*ui})
	seen := map[string]string{}
	name := func(path, kind, n string) {
		if prev, dup := seen[kind+" "+n]; dup {
			v.err(path, "Give each its own name.", "%q is already used at %s", n, prev)
		}
		seen[kind+" "+n] = path
	}
	for i := range ui.Sources {
		s := &ui.Sources[i].SourceSpec
		p := fmt.Sprintf("/sources/%d", i)
		if ModuleOf(s.Type) != mod || !sourceNamePattern.MatchString(strings.TrimPrefix(s.Type, mod+".")) {
			v.err(p+"/type", "Name it "+mod+".something, in camelCase.", "%q is not a name of this module's: a source is %s.<name>", s.Type, mod)
		}
		name(p+"/type", "source", s.Type)
		v.specText(p, s.Label, s.Description)
		if s.Icon != "" && !contains(Icons, s.Icon) {
			v.err(p+"/icon", suggest(s.Icon, Icons), "%q is not one of the built-in icons", s.Icon)
		}
		if len(s.Yields) == 0 {
			v.err(p+"/yields", "List the kinds of item it gives, such as [\"game\"].", "a source says what it gives")
		}
		for j, k := range s.Yields {
			if !contains(ItemKinds, k) {
				v.err(fmt.Sprintf("%s/yields/%d", p, j), suggest(k, ItemKinds), "%q is not a kind of item", k)
			}
		}
		if _, ok := CardStyle(s.CardStyle); !ok {
			v.err(p+"/cardStyle", suggest(s.CardStyle, cardStyleOptions), "%q is not a card style", s.CardStyle)
		}
		for j, k := range s.PageKinds {
			if pageKind(k) == nil {
				v.err(fmt.Sprintf("%s/pageKinds/%d", p, j), suggest(k, pageKindNames()), "%q is not a kind of page", k)
			}
		}
		if s.SeeAll != "" && !ValidRoute(strings.NewReplacer("{", "", "}", "").Replace(s.SeeAll)) {
			v.err(p+"/seeAll", "Use a place in the app such as /page/"+mod+".", "%q is not a place in the app", s.SeeAll)
		}
		v.paramSpecs(s.Params, p+"/params")
	}
	for i := range ui.Components {
		c := &ui.Components[i]
		p := fmt.Sprintf("/components/%d", i)
		if ModuleOf(c.Type) != mod || !compNamePattern.MatchString(strings.TrimPrefix(c.Type, mod+".")) {
			v.err(p+"/type", "Name it "+mod+".Something, in PascalCase.", "%q is not a name of this module's: a component is %s.<Name>", c.Type, mod)
		}
		name(p+"/type", "component", c.Type)
		v.specText(p, c.Label, c.Description)
		if c.Icon != "" && !contains(Icons, c.Icon) {
			v.err(p+"/icon", suggest(c.Icon, Icons), "%q is not one of the built-in icons", c.Icon)
		}
		v.paramSpecs(c.Props, p+"/props")
		for j, k := range c.PageKinds {
			if pageKind(k) == nil {
				v.err(fmt.Sprintf("%s/pageKinds/%d", p, j), suggest(k, pageKindNames()), "%q is not a kind of page", k)
			}
		}
		if self.Component(c.Section.Component) != nil {
			v.err(p+"/section/component", "Build it from the catalogue's components.", "a component is made of the catalogue's components, not of another of the module's")
			continue
		}
		for _, ph := range placeholders(c.Section) {
			found := false
			for _, ps := range c.Props {
				found = found || ps.Name == ph
			}
			if !found {
				v.err(p+"/section", "Add a property called "+ph+", or remove {{"+ph+"}}.", "{{%s}} names no property of the component", ph)
			}
		}
		// The template, filled with each property's default or an example,
		// must make a section of the page kinds it is for.
		kind := "custom"
		if len(c.PageKinds) > 0 {
			kind = c.PageKinds[0]
		}
		sec := expandComponent(c, Section{ID: "example", Component: c.Type, Props: exampleProps(c.Props)})
		sv := &validator{page: &Page{Kind: kind}, ui: self, own: true}
		sv.sectionList([]Section{sec}, p+"/section")
		v.issues = append(v.issues, sv.issues...)
	}
	for i := range ui.Views {
		w := &ui.Views[i]
		p := fmt.Sprintf("/views/%d", i)
		if ModuleOf(w.ID) != mod || !sourceNamePattern.MatchString(strings.TrimPrefix(w.ID, mod+".")) {
			v.err(p+"/id", "Name it "+mod+".something, in camelCase.", "%q is not a name of this module's: a view is %s.<name>", w.ID, mod)
		}
		name(p+"/id", "view", w.ID)
		v.specText(p, w.Label, w.Description)
		for j, pl := range w.Platforms {
			if !contains(Platforms, pl) {
				v.err(fmt.Sprintf("%s/platforms/%d", p, j), suggest(pl, Platforms), "%q is not a platform", pl)
			}
		}
	}
	pageIDs := map[string]bool{}
	for i := range ui.Pages {
		pg := &ui.Pages[i]
		p := fmt.Sprintf("/pages/%d", i)
		if !ownedID(mod, pg.ID) {
			v.err(p+"/id", "Call it "+mod+" or "+mod+"-something.", "%q is not a page ID of this module's", pg.ID)
		}
		if _, stock := StockPage(pg.ID); stock {
			v.err(p+"/id", "Choose another ID.", "%q is a built-in page's ID", pg.ID)
		}
		name(p+"/id", "page", pg.ID)
		pageIDs[pg.ID] = true
		if !contains(ModulePageKinds, pg.Kind) {
			v.err(p+"/kind", "A module adds a page of kind "+strings.Join(ModulePageKinds, ", ")+"; it adds to Home with an insert.", "a module can't add a %s page", pg.Kind)
		}
		if (pg.Kind == "library" || pg.Kind == "detail") && (pg.Match == nil || (pg.Match.LibraryKind == "" && pg.Match.MediaKind == "")) {
			v.err(p+"/match", "Narrow it with match.libraryKind or match.mediaKind.", "a module's %s page is for a kind of library or title, never all of them", pg.Kind)
		}
		if pg.Match != nil && pg.Match.Library != 0 {
			v.err(p+"/match/library", "Use match.libraryKind.", "a module doesn't know this server's libraries")
		}
		for _, is := range validatePage(pg, self, true) {
			is.Path = p + is.Path
			v.issues = append(v.issues, is)
		}
	}
	for i := range ui.Inserts {
		in := &ui.Inserts[i]
		p := fmt.Sprintf("/inserts/%d", i)
		target, ok := StockPage(in.Page)
		if !ok {
			v.err(p+"/page", "Name a built-in page, such as home.", "%q is not a built-in page", in.Page)
			continue
		}
		if in.After != "" && in.Before != "" {
			v.err(p, "Keep one of after and before.", "an insert goes after a section or before one, not both")
		}
		for _, ref := range []string{in.After, in.Before} {
			if ref != "" && !hasSection(target.Sections, ref) {
				v.err(p, "Name a top-level section of the "+in.Page+" page.", "the %s page has no section %q", in.Page, ref)
			}
		}
		if !ownedID(mod, in.Section.ID) {
			v.err(p+"/section/id", "Call it "+mod+"-something.", "%q is not a section ID of this module's", in.Section.ID)
		}
		name(p+"/section/id", "insert "+in.Page, in.Section.ID)
		sv := &validator{page: &Page{ID: target.ID, Kind: target.Kind}, ui: self, own: true}
		sv.sectionList([]Section{in.Section}, p+"/section")
		v.issues = append(v.issues, sv.issues...)
	}
	for i := range ui.Presets {
		ps := &ui.Presets[i]
		p := fmt.Sprintf("/presets/%d", i)
		if !ownedID(mod, ps.ID) {
			v.err(p+"/id", "Call it "+mod+"-something.", "%q is not a preset ID of this module's", ps.ID)
		}
		name(p+"/id", "preset", ps.ID)
		v.specText(p, ps.Label, ps.Description)
		sv := &validator{page: &Page{Kind: "custom"}, ui: self, own: true}
		sv.sectionList([]Section{ps.Section}, p+"/section")
		v.issues = append(v.issues, sv.issues...)
	}
	nav := Navigation{Items: append(append([]NavItem(nil), ui.Navigation...), NavItem{ID: "settings", Type: "native", Screen: "settings"})}
	for _, is := range ValidateNavigation(&nav, func(id string) bool { _, stock := StockPage(id); return pageIDs[id] || stock }) {
		v.issues = append(v.issues, Issue{Path: is.Path, Severity: is.Severity, Message: is.Message, Fix: is.Fix})
	}
	for i := range ui.Navigation {
		it := &ui.Navigation[i]
		p := fmt.Sprintf("/navigation/%d", i)
		if !ownedID(mod, it.ID) {
			v.err(p+"/id", "Call it "+mod+" or "+mod+"-something.", "%q is not an entry ID of this module's", it.ID)
		}
		if it.Type == "native" && it.Screen == "settings" {
			v.err(p+"/screen", "Describe the module's settings under settings instead.", "Settings is the server's entry")
		}
		if it.Type == "libraries" || it.Type == "library" {
			v.err(p+"/type", "Use a page or a route.", "a module doesn't know this server's libraries")
		}
	}
	keys := map[string]string{}
	for i := range ui.Settings {
		sp := &ui.Settings[i]
		p := fmt.Sprintf("/settings/%d", i)
		if !ownedID(mod, sp.ID) {
			v.err(p+"/id", "Call it "+mod+" or "+mod+"-something.", "%q is not a settings page ID of this module's", sp.ID)
		}
		name(p+"/id", "settings", sp.ID)
		v.specText(p, sp.Title, "")
		if sp.Icon != "" && !contains(Icons, sp.Icon) {
			v.err(p+"/icon", suggest(sp.Icon, Icons), "%q is not one of the built-in icons", sp.Icon)
		}
		rows := 0
		for g := range sp.Groups {
			grp := &sp.Groups[g]
			gp := fmt.Sprintf("%s/groups/%d", p, g)
			if len([]rune(grp.Title)) > 80 || len([]rune(grp.Footer)) > 500 {
				v.err(gp, "Shorten it.", "a group's title is at most 80 characters and its footer 500")
			}
			for r := range grp.Rows {
				rows++
				v.settingRow(&grp.Rows[r], fmt.Sprintf("%s/rows/%d", gp, r), keys, sp.Admin)
			}
		}
		if rows == 0 {
			v.err(p+"/groups", "Add a row, or leave the page out.", "a settings page needs at least one row")
		}
		if rows > maxModuleRows {
			v.err(p+"/groups", "Split it into more pages.", "a settings page has at most %d rows", maxModuleRows)
		}
	}
	return v.issues
}

func sortedTokenKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func hasSection(list []Section, id string) bool {
	for i := range list {
		if list[i].ID == id {
			return true
		}
	}
	return false
}

func (v *validator) specText(p, label, desc string) {
	if strings.TrimSpace(label) == "" {
		v.err(p+"/label", "Give it a label for the editor.", "it needs a label")
	} else if len([]rune(label)) > 80 {
		v.err(p+"/label", "Shorten it.", "a label is at most 80 characters")
	}
	if len([]rune(desc)) > 500 {
		v.err(p+"/description", "Shorten it.", "a description is at most 500 characters")
	}
}

var authoredPropTypes = []string{PropString, PropText, PropMarkdown, PropInt, PropBool, PropEnum, PropEnumList, PropIcon, PropRoute, PropImage, PropURL, PropSection, PropLibrary, PropRef, PropScope}

// paramSpecs checks the property descriptions a module wrote.
func (v *validator) paramSpecs(specs []PropSpec, path string) {
	seen := map[string]bool{}
	for i := range specs {
		s := &specs[i]
		p := fmt.Sprintf("%s/%d", path, i)
		if !sourceNamePattern.MatchString(s.Name) || s.Name == "type" || s.Name == "map" {
			v.err(p+"/name", "Use a camelCase name.", "%q is not a property name", s.Name)
		}
		if seen[s.Name] {
			v.err(p+"/name", "Give each its own name.", "%q is described twice", s.Name)
		}
		seen[s.Name] = true
		if !contains(authoredPropTypes, s.Type) {
			v.err(p+"/type", suggest(s.Type, authoredPropTypes), "%q is not a property type", s.Type)
		}
		if strings.TrimSpace(s.Label) == "" {
			v.err(p+"/label", "Give it a label for the editor.", "a property needs a label")
		}
		if (s.Type == PropEnum || s.Type == PropEnumList) && len(s.Options) == 0 {
			v.err(p+"/options", "List the choices.", "a choice needs options")
		}
		if s.Default != nil {
			dv := &validator{page: &Page{Kind: "custom"}}
			dv.value(s, s.Default, p+"/default")
			for _, is := range dv.issues {
				if !strings.Contains(is.Message, "means nothing") {
					v.issues = append(v.issues, is)
				}
			}
		}
	}
}

func (v *validator) settingRow(r *SettingRow, p string, keys map[string]string, adminPage bool) {
	if !contains(SettingKinds, r.Kind) {
		v.err(p+"/kind", suggest(r.Kind, SettingKinds), "%q is not a kind of setting; the kinds are %s", r.Kind, strings.Join(SettingKinds, ", "))
	}
	if strings.TrimSpace(r.Label) == "" || len([]rune(r.Label)) > 80 {
		v.err(p+"/label", "Give it a short label.", "a setting needs a label of at most 80 characters")
	}
	if len([]rune(r.Help)) > 500 || len([]rune(r.Unit)) > 20 {
		v.err(p+"/help", "Shorten it.", "help is at most 500 characters and a unit 20")
	}
	if r.Kind == SettingInfo {
		if r.Key != "" || r.Default != nil {
			v.err(p, "Leave out key and default.", "an info row has no value")
		}
		return
	}
	if !settingKeyPattern.MatchString(r.Key) {
		v.err(p+"/key", "Use a short camelCase name.", "%q is not a setting's key", r.Key)
	} else if prev, dup := keys[r.Key]; dup {
		v.err(p+"/key", "Give each setting its own key.", "%q is already used at %s", r.Key, prev)
	}
	keys[r.Key] = p
	switch r.Scope {
	case "", SettingScopeServer:
	case SettingScopeProfile:
		if adminPage {
			v.err(p+"/scope", "Move it to a page that isn't for administrators only.", "a profile's own setting is on a page every profile sees")
		}
		if r.Kind == SettingSecret {
			v.err(p+"/scope", "Keep secrets for the server.", "a secret is the server's, not a profile's")
		}
	default:
		v.err(p+"/scope", "Use server or profile.", "%q is not where a setting is kept", r.Scope)
	}
	if r.Kind == SettingEnum && len(r.Options) == 0 {
		v.err(p+"/options", "List the choices.", "a choice needs options")
	}
	if r.Kind != SettingEnum && len(r.Options) > 0 {
		v.err(p+"/options", "Remove them, or make it an enum.", "only an enum has options")
	}
	if r.Default != nil {
		if r.Kind == SettingSecret {
			v.err(p+"/default", "Leave it out.", "a secret has no default")
		} else if err := CheckSetting(r, r.Default); err != nil {
			v.err(p+"/default", "", "the default: %s", err)
		}
	}
	if r.MaxLength < 0 || r.MaxLength > 4000 {
		v.err(p+"/maxLength", "", "maxLength is between 0 and 4000")
	}
}

// CheckSetting checks one value for a setting row.
func CheckSetting(r *SettingRow, val any) error {
	switch r.Kind {
	case SettingBool:
		if _, ok := val.(bool); !ok {
			return errors.New(r.Key + " is true or false")
		}
	case SettingEnum:
		s, _ := val.(string)
		if !contains(values(r.Options), s) {
			return fmt.Errorf("%s is one of %s", r.Key, strings.Join(values(r.Options), ", "))
		}
	case SettingInt:
		n, ok := toInt(val)
		if !ok {
			return errors.New(r.Key + " is a whole number")
		}
		if (r.Min != nil && n < *r.Min) || (r.Max != nil && n > *r.Max) {
			lo, hi := "any", "any"
			if r.Min != nil {
				lo = fmt.Sprint(*r.Min)
			}
			if r.Max != nil {
				hi = fmt.Sprint(*r.Max)
			}
			return fmt.Errorf("%s is between %s and %s", r.Key, lo, hi)
		}
	case SettingString, SettingSecret:
		s, ok := val.(string)
		if !ok {
			return errors.New(r.Key + " is text")
		}
		limit := r.MaxLength
		if limit == 0 {
			limit = 1000
		}
		if len([]rune(s)) > limit {
			return fmt.Errorf("%s is at most %d characters", r.Key, limit)
		}
		if strings.ContainsAny(s, "\x00\x1b\n\r") {
			return errors.New(r.Key + " is one line of plain text")
		}
	default:
		return errors.New(r.Key + " takes no value")
	}
	return nil
}

// ---------------------------------------------------------------------------
// The modules in use, together.
// ---------------------------------------------------------------------------

// ModuleSet is the SDUI of every module in use, indexed. A nil *ModuleSet is
// an empty one. It is shared and never changed once made.
type ModuleSet struct {
	uis        []ModuleUI
	components map[string]*ModuleComponent
	sources    map[string]*SourceSpec
	views      map[string]*ModuleView
	pages      map[string]*Page
	pageOwner  map[string]string
}

// NewModuleSet indexes modules' SDUI. Files should have passed
// ValidateModuleUI; a name two files claim goes to the first.
func NewModuleSet(uis []ModuleUI) *ModuleSet {
	s := &ModuleSet{components: map[string]*ModuleComponent{}, sources: map[string]*SourceSpec{}, views: map[string]*ModuleView{}, pages: map[string]*Page{}, pageOwner: map[string]string{}}
	// A copy of its own, so nothing the caller does later changes it.
	if b, err := json.Marshal(uis); err == nil {
		_ = json.Unmarshal(b, &s.uis)
	}
	sort.SliceStable(s.uis, func(i, j int) bool { return s.uis[i].Module < s.uis[j].Module })
	for i := range s.uis {
		ui := &s.uis[i]
		for j := range ui.Components {
			c := &ui.Components[j]
			if ModuleOf(c.Type) == ui.Module && s.components[c.Type] == nil {
				s.components[c.Type] = c
			}
		}
		for j := range ui.Sources {
			src := &ui.Sources[j].SourceSpec
			if ModuleOf(src.Type) == ui.Module && s.sources[src.Type] == nil {
				src.Category = "module"
				if src.Since == 0 {
					src.Since = CatalogRevision
				}
				s.sources[src.Type] = src
			}
		}
		for j := range ui.Views {
			w := &ui.Views[j]
			if ModuleOf(w.ID) == ui.Module && s.views[w.ID] == nil {
				s.views[w.ID] = w
			}
		}
		for j := range ui.Pages {
			p := &ui.Pages[j]
			if ownedID(ui.Module, p.ID) && s.pages[p.ID] == nil {
				if p.Scope == "" {
					p.Scope = ui.Module
				}
				s.pages[p.ID] = p
				s.pageOwner[p.ID] = ui.Module
			}
		}
	}
	return s
}

// Modules are the files in the set, by module ID.
func (s *ModuleSet) Modules() []ModuleUI {
	if s == nil {
		return nil
	}
	return s.uis
}

// Component is a module's component, nil when no module in the set has it.
func (s *ModuleSet) Component(t string) *ModuleComponent {
	if s == nil {
		return nil
	}
	return s.components[t]
}

// Source is a module's data source, nil when no module in the set answers it.
func (s *ModuleSet) Source(t string) *SourceSpec {
	if s == nil {
		return nil
	}
	return s.sources[t]
}

// View reports whether a module in the set has the view.
func (s *ModuleSet) View(id string) bool { return s != nil && s.views[id] != nil }

// Page is a module's page by ID: a copy.
func (s *ModuleSet) Page(id string) (Page, bool) {
	if s == nil || s.pages[id] == nil {
		return Page{}, false
	}
	return ClonePage(*s.pages[id]), true
}

// Pages are every module's pages, copies, by module and then in each file's
// order.
func (s *ModuleSet) Pages() []Page {
	if s == nil {
		return nil
	}
	var out []Page
	for i := range s.uis {
		for _, p := range s.uis[i].Pages {
			if q := s.pages[p.ID]; q != nil && s.pageOwner[p.ID] == s.uis[i].Module {
				out = append(out, ClonePage(*q))
			}
		}
	}
	return out
}

// Navigation are the modules' entries, by module.
func (s *ModuleSet) Navigation() []NavItem {
	if s == nil {
		return nil
	}
	var out []NavItem
	for i := range s.uis {
		out = append(out, s.uis[i].Navigation...)
	}
	return out
}

// Scopes are the modules' colours by scope.
func (s *ModuleSet) Scopes() map[string]map[string]string {
	if s == nil {
		return nil
	}
	out := map[string]map[string]string{}
	for i := range s.uis {
		if t := s.uis[i].Theme; len(t.Tokens) > 0 {
			out[t.Scope] = copyTokens(t.Tokens)
		}
	}
	return out
}

// WithInserts is a built-in page with the modules' sections put into it.
// Sections whose IDs are already on the page are left out.
func (s *ModuleSet) WithInserts(p Page) Page {
	if s == nil {
		return p
	}
	for i := range s.uis {
		ui := &s.uis[i]
		for _, in := range ui.Inserts {
			if in.Page != p.ID || hasSection(p.Sections, in.Section.ID) {
				continue
			}
			sec := in.Section
			if sec.Scope == "" {
				sec.Scope = ui.Module
			}
			at := len(p.Sections)
			for j := range p.Sections {
				if in.After != "" && p.Sections[j].ID == in.After {
					at = j + 1
				}
				if in.Before != "" && p.Sections[j].ID == in.Before {
					at = j
				}
			}
			p.Sections = append(p.Sections[:at], append([]Section{sec}, p.Sections[at:]...)...)
		}
	}
	return p
}

// Presets are the modules' section presets, as the editor's gallery lists them.
func (s *ModuleSet) Presets() []Preset {
	if s == nil {
		return nil
	}
	var out []Preset
	for i := range s.uis {
		ui := &s.uis[i]
		for _, ps := range ui.Presets {
			sec := ps.Section
			out = append(out, Preset{ID: ps.ID, Kind: "section", Label: ps.Label, Description: ps.Description, Module: ui.Module, Section: &sec})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Components: templates the server fills in.
// ---------------------------------------------------------------------------

// ExpandPage replaces every module component on a page with the sections it
// stands for. Sections of a component no module in the set has stay as they
// are; Render leaves them out.
func (s *ModuleSet) ExpandPage(p Page) Page {
	if s == nil || len(s.components) == 0 {
		return p
	}
	p = ClonePage(p)
	p.Sections = s.expandList(p.Sections)
	for i := range p.Variants {
		p.Variants[i].Sections = s.expandList(p.Variants[i].Sections)
	}
	return p
}

func (s *ModuleSet) expandList(list []Section) []Section {
	for i := range list {
		if c := s.components[list[i].Component]; c != nil {
			list[i] = expandComponent(c, list[i])
			continue
		}
		list[i].Children = s.expandList(list[i].Children)
	}
	return list
}

// expandComponent makes the section a use of component c stands for: its
// template with the use's properties filled in, the use's ID, condition,
// repeat, scope and lock, and the template's own sections' IDs put after the
// use's so that two uses on a page never clash.
func expandComponent(c *ModuleComponent, use Section) Section {
	vals := map[string]any{}
	for _, ps := range c.Props {
		if ps.Default != nil {
			vals[ps.Name] = ps.Default
		}
	}
	for k, v := range use.Props {
		vals[k] = v
	}
	out := fillSection(c.Section, vals, use.ID)
	out.ID = use.ID
	switch {
	case use.When != nil && out.When != nil:
		out.When = &Condition{All: []Condition{*use.When, *out.When}}
	case use.When != nil:
		w := *use.When
		out.When = &w
	}
	if use.Repeat != nil {
		r := *use.Repeat
		out.Repeat = &r
	}
	if use.Scope != "" {
		out.Scope = use.Scope
	} else if out.Scope == "" {
		out.Scope = ModuleOf(c.Type)
	}
	out.Locked = out.Locked || use.Locked
	return out
}

func fillSection(t Section, vals map[string]any, prefix string) Section {
	out := t
	out.Props = fillMap(t.Props, vals)
	if t.Source != nil {
		out.Source = Source(fillMap(t.Source, vals))
	}
	if t.When != nil {
		w := *t.When
		out.When = &w
	}
	out.Children = nil
	for _, c := range t.Children {
		fc := fillSection(c, vals, prefix)
		fc.ID = clipSectionID(prefix + "-" + c.ID)
		out.Children = append(out.Children, fc)
	}
	return out
}

func clipSectionID(s string) string {
	if len(s) > MaxIDLength {
		return s[:MaxIDLength]
	}
	return s
}

func fillMap(m map[string]any, vals map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		fv, keep := fillValue(v, vals)
		if keep {
			out[k] = fv
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// fillValue fills a template value: "{{name}}" alone is the property's value
// (left out when the property has none), and inside text is replaced by it.
func fillValue(v any, vals map[string]any) (any, bool) {
	switch x := v.(type) {
	case string:
		if m := placeholderRe.FindStringSubmatch(x); m != nil && m[0] == x {
			val, ok := vals[m[1]]
			return val, ok && val != nil
		}
		return placeholderRe.ReplaceAllStringFunc(x, func(ph string) string {
			val, ok := vals[ph[2:len(ph)-2]]
			if !ok || val == nil {
				return ""
			}
			return Source{"v": val}.Str("v")
		}), true
	case map[string]any:
		return fillMap(x, vals), true
	case []any:
		out := make([]any, 0, len(x))
		for _, e := range x {
			if fe, ok := fillValue(e, vals); ok {
				out = append(out, fe)
			}
		}
		return out, true
	}
	return v, true
}

// placeholders lists the {{names}} a template uses.
func placeholders(t Section) []string {
	seen := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			for _, m := range placeholderRe.FindAllStringSubmatch(x, -1) {
				seen[m[1]] = true
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		case Source:
			for _, e := range x {
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	var sec func(s Section)
	sec = func(s Section) {
		walk(s.Props)
		walk(map[string]any(s.Source))
		for _, c := range s.Children {
			sec(c)
		}
	}
	sec(t)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// exampleProps gives every property of a component a value: its default, or
// an example of its type, to check that the template makes a section.
func exampleProps(specs []PropSpec) map[string]any {
	out := map[string]any{}
	for _, s := range specs {
		if s.Default != nil {
			out[s.Name] = s.Default
			continue
		}
		switch s.Type {
		case PropInt:
			n := 1
			if s.Min != nil {
				n = *s.Min
			}
			out[s.Name] = n
		case PropBool:
			out[s.Name] = false
		case PropEnum:
			if len(s.Options) > 0 {
				out[s.Name] = s.Options[0].Value
			}
		case PropEnumList:
			if len(s.Options) > 0 {
				out[s.Name] = []any{s.Options[0].Value}
			}
		case PropIcon:
			out[s.Name] = "star"
		case PropRoute:
			out[s.Name] = "/"
		case PropImage:
			out[s.Name] = "/api/example.png"
		case PropURL:
			out[s.Name] = "https://example.org/"
		case PropLibrary, PropRef:
			out[s.Name] = "1"
		case PropSection, PropScope:
			out[s.Name] = "example"
		default:
			out[s.Name] = "Example"
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The catalogue with the modules in it.
// ---------------------------------------------------------------------------

// CatalogWith is the catalogue with the modules' components, sources and views
// added, as an editor needs it. With an empty set it is Catalog() itself.
func CatalogWith(s *ModuleSet) *Catalogue {
	base := Catalog()
	if s == nil || (len(s.components) == 0 && len(s.sources) == 0 && len(s.views) == 0) {
		return base
	}
	c := *base
	c.Components = append([]ComponentSpec(nil), base.Components...)
	c.Sources = append([]SourceSpec(nil), base.Sources...)
	c.NativeViews = append([]EnumOption(nil), base.NativeViews...)
	for _, ui := range s.uis {
		for _, mc := range ui.Components {
			if s.components[mc.Type] == nil || ModuleOf(mc.Type) != ui.Module {
				continue
			}
			c.Components = append(c.Components, moduleComponentSpec(s.components[mc.Type]))
		}
		for _, ms := range ui.Sources {
			if src := s.sources[ms.Type]; src != nil && ModuleOf(ms.Type) == ui.Module {
				sc := *src
				sc.Module = ui.Module
				c.Sources = append(c.Sources, sc)
			}
		}
		for _, w := range ui.Views {
			if s.views[w.ID] != nil {
				c.NativeViews = append(c.NativeViews, EnumOption{Value: w.ID, Label: w.Label})
			}
		}
	}
	// NativeView's view property offers the modules' views too.
	for i := range c.Components {
		if c.Components[i].Type != "NativeView" {
			continue
		}
		nv := c.Components[i]
		nv.Props = append([]PropSpec(nil), nv.Props...)
		for j := range nv.Props {
			if nv.Props[j].Name == "view" {
				nv.Props[j].Options = append([]EnumOption(nil), c.NativeViews...)
			}
		}
		c.Components[i] = nv
	}
	return &c
}

func moduleComponentSpec(mc *ModuleComponent) ComponentSpec {
	base, _ := Component(mc.Section.Component)
	spec := ComponentSpec{Type: mc.Type, Label: mc.Label, Description: mc.Description, Icon: mc.Icon, Category: "module",
		Module: ModuleOf(mc.Type), Since: CatalogRevision, Authored: true, Required: true, Data: "none", Focus: "none", Props: mc.Props, PageKinds: mc.PageKinds}
	if spec.Icon == "" {
		spec.Icon = "plug"
	}
	if base != nil {
		spec.Focus = base.Focus
		spec.FormFactors = base.FormFactors
		spec.Expands = base.Type
		spec.Tokens = base.Tokens
	}
	if ex, err := json.Marshal(Section{ID: "example", Component: mc.Type, Props: exampleProps(requiredOnly(mc.Props))}); err == nil {
		spec.Example = ex
	}
	return spec
}

func requiredOnly(specs []PropSpec) []PropSpec {
	var out []PropSpec
	for _, s := range specs {
		if s.Required {
			out = append(out, s)
		}
	}
	return out
}

// viewOptions are the modules' views as choices for NativeView's view.
func (s *ModuleSet) viewOptions() []EnumOption {
	if s == nil {
		return nil
	}
	var out []EnumOption
	for _, ui := range s.uis {
		for _, w := range ui.Views {
			if s.views[w.ID] != nil {
				out = append(out, EnumOption{Value: w.ID, Label: w.Label})
			}
		}
	}
	return out
}

// PageModule is the module a page of the set comes from, "" for none.
func (s *ModuleSet) PageModule(id string) string {
	if s == nil {
		return ""
	}
	return s.pageOwner[id]
}
