// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Limits on what is authored, so one edit can't make a page no small device
// can hold.
const (
	// MaxPageBytes bounds one page as JSON.
	MaxPageBytes = 256 << 10
	// MaxSections bounds the sections of one page, nested ones and every
	// variant's included.
	MaxSections = 200
	// MaxDepth bounds nesting: a top-level section is depth 1.
	MaxDepth = 6
	// MaxChildren bounds the sections directly inside one container, and the
	// top-level sections of a page or variant.
	MaxChildren = 60
	// MaxVariants bounds a page's variants.
	MaxVariants = 8
	// MaxNavItems bounds the navigation.
	MaxNavItems = 40
	// MaxPages bounds the pages of one configuration.
	MaxPages = 200
	// MaxIDLength bounds a page's or section's ID; MaxNodeIDLength a rendered
	// node's, which is a section's ID and an item's joined.
	MaxIDLength     = 64
	MaxNodeIDLength = 128
	// MaxPersonalBytes bounds what one profile stores about one page.
	MaxPersonalBytes = 8 << 10
)

// Issue is one thing wrong with something authored: where, what, and how to
// put it right, in words an administrator can act on.
type Issue struct {
	// Path locates it: /sections/2/props/cardStyle.
	Path string `json:"path"`
	// Severity is "error" (it can't be saved) or "warning" (it can, but
	// probably isn't what was meant).
	Severity string `json:"severity"`
	Message  string `json:"message"`
	// Fix suggests what to do about it.
	Fix string `json:"fix,omitempty"`
}

func (i Issue) String() string { return i.Path + ": " + i.Message }

// ValidationError carries every error found.
type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "sdui: invalid"
	}
	s := "sdui: " + e.Issues[0].String()
	if n := len(e.Issues) - 1; n > 0 {
		s += fmt.Sprintf(" (and %d more)", n)
	}
	return s
}

// HasErrors reports whether any issue is an error.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity != "warning" {
			return true
		}
	}
	return false
}

// AsError wraps the errors among issues, nil when there are none.
func AsError(issues []Issue) error {
	var errs []Issue
	for _, i := range issues {
		if i.Severity != "warning" {
			errs = append(errs, i)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return &ValidationError{Issues: errs}
}

var (
	idPattern    = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	scopePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	clockPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	wordPattern  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]{0,63}$`)
	localeTag    = regexp.MustCompile(`^[A-Za-z]{2,8}([-_][A-Za-z0-9]{1,8}){0,3}$`)
)

// ValidID reports whether s can be a page's or section's ID.
func ValidID(s string) bool { return idPattern.MatchString(s) }

type validator struct {
	issues []Issue
	page   *Page
	count  int
	// ui are the modules in use, whose components, sources and views a page
	// may use. nil: none.
	ui *ModuleSet
	// own is set while a module's own file is checked: then another module's
	// component or source is an error, since a module must work alone.
	own bool
}

// missingModule reports a component or source of a module that isn't in use.
func (v *validator) missingModule(path, fix, what, name, mod string) {
	if v.own {
		v.err(path, "Use the catalogue's, or this module's own.", "%q is another module's %s: a module uses only its own, so it works wherever it is installed", name, what)
		return
	}
	v.warn(path, fix, "%q is a %s of the %s module, which isn't in use: the section is left out until it is", name, what, mod)
}

// component is a component's spec: the catalogue's, or a module's as the
// editor sees it. NativeView's views include the modules'.
func (v *validator) component(t string) (*ComponentSpec, bool) {
	if mc := v.ui.Component(t); mc != nil {
		spec := moduleComponentSpec(mc)
		return &spec, true
	}
	spec, ok := Component(t)
	if ok && t == "NativeView" && v.ui != nil && len(v.ui.views) > 0 {
		c := *spec
		c.Props = append([]PropSpec(nil), spec.Props...)
		for i := range c.Props {
			if c.Props[i].Name == "view" {
				c.Props[i].Options = append(append([]EnumOption(nil), c.Props[i].Options...), v.ui.viewOptions()...)
			}
		}
		return &c, true
	}
	return spec, ok
}

// sourceSpec is a data source's spec: the catalogue's or a module's.
func (v *validator) sourceSpec(t string) (*SourceSpec, bool) {
	if s := v.ui.Source(t); s != nil {
		return s, true
	}
	return SourceType(t)
}

func (v *validator) err(path, fix, format string, args ...any) {
	v.issues = append(v.issues, Issue{Path: path, Severity: "error", Message: fmt.Sprintf(format, args...), Fix: fix})
}

func (v *validator) warn(path, fix, format string, args ...any) {
	v.issues = append(v.issues, Issue{Path: path, Severity: "warning", Message: fmt.Sprintf(format, args...), Fix: fix})
}

// ValidatePage checks a page against the catalogue and answers everything
// wrong with it: errors, which keep it from being saved, and warnings.
func ValidatePage(p *Page) []Issue { return ValidatePageWith(p, nil) }

// ValidatePageWith checks a page that may use the components, sources and
// views of the modules in ui. A section of a module that isn't in use is a
// warning, not an error: it is left out until the module is back.
func ValidatePageWith(p *Page, ui *ModuleSet) []Issue {
	return validatePage(p, ui, false)
}

func validatePage(p *Page, ui *ModuleSet, own bool) []Issue {
	v := &validator{page: p, ui: ui, own: own}
	if !ValidID(p.ID) {
		v.err("/id", "Use a short name such as family-night.", "a page's ID is lower-case letters, digits, - and _, starting with a letter, at most %d long", MaxIDLength)
	}
	kind := pageKind(p.Kind)
	if kind == nil {
		fix := orFix(suggest(p.Kind, pageKindNames()), "The kinds are "+strings.Join(pageKindNames(), ", ")+".")
		if p.Kind == "settings" || p.Kind == "player" {
			fix = "Settings and the player are the app's own screens, not pages. Link to Settings with a Button or a navigation entry: \"native\": \"settings\"."
		}
		v.err("/kind", fix, "%q is not a kind of page", p.Kind)
	}
	if strings.TrimSpace(p.Title) == "" {
		v.err("/title", "Give the page a title; it is what the navigation and the window show.", "a page needs a title")
	} else if len([]rune(p.Title)) > 120 {
		v.err("/title", "Shorten it.", "a title is at most 120 characters")
	}
	if p.Icon != "" && !contains(Icons, p.Icon) {
		v.err("/icon", suggest(p.Icon, Icons), "%q is not one of the built-in icons", p.Icon)
	}
	if len(p.Description) > 500 {
		v.err("/description", "Shorten it.", "a description is at most 500 characters")
	}
	if p.Scope != "" && !scopePattern.MatchString(p.Scope) {
		v.err("/scope", "Use a module's ID such as wittymusic, or a name of your own in lower case.", "%q is not a theme scope", p.Scope)
	}
	if p.TTLSeconds < 0 || p.TTLSeconds > 86400 {
		v.err("/ttlSeconds", "Use 0 for the default.", "ttlSeconds is between 0 and 86400")
	}
	if m := p.Match; m != nil && kind != nil {
		for field, set := range map[string]bool{"library": m.Library != 0, "libraryKind": m.LibraryKind != "", "mediaKind": m.MediaKind != ""} {
			if set && !contains(kind.Match, field) {
				v.err("/match/"+field, "Remove it, or change the page's kind.", "a %s page can't be matched by %s", p.Kind, field)
			}
		}
		if m.Library < 0 {
			v.err("/match/library", "", "a library's ID is a positive number")
		}
		if m.LibraryKind != "" && !contains(LibraryKinds, m.LibraryKind) {
			v.err("/match/libraryKind", suggest(m.LibraryKind, LibraryKinds), "%q is not a kind of library", m.LibraryKind)
		}
		if m.MediaKind != "" && !contains(MediaKinds, m.MediaKind) {
			v.err("/match/mediaKind", suggest(m.MediaKind, MediaKinds), "%q is not a kind of title", m.MediaKind)
		}
	}
	if p.Personal != nil {
		for i, a := range p.Personal.Allow {
			if !contains(PersonalKinds, a) {
				v.err("/personal/allow/"+strconv.Itoa(i), suggest(a, PersonalKinds), "%q is not something a profile can change", a)
			}
		}
	}
	if len(p.Sections) == 0 && len(p.Variants) == 0 {
		v.warn("/sections", "Add a section, or apply a preset.", "the page has no sections, so it will be empty")
	}
	v.sectionList(p.Sections, "/sections")
	if len(p.Variants) > MaxVariants {
		v.err("/variants", "Combine variants with any/all conditions.", "a page has at most %d variants", MaxVariants)
	}
	seen := map[string]bool{}
	for i := range p.Variants {
		va := &p.Variants[i]
		path := "/variants/" + strconv.Itoa(i)
		if !ValidID(va.ID) {
			v.err(path+"/id", "Use a short name such as kids.", "a variant's ID is lower-case letters, digits, - and _")
		} else if seen[va.ID] {
			v.err(path+"/id", "Give each variant its own ID.", "two variants are called %q", va.ID)
		}
		seen[va.ID] = true
		if va.When == nil || isZeroCondition(va.When) {
			v.err(path+"/when", "Say who or what the variant is for, such as {\"profileKind\": [\"kids\"]}.", "a variant needs a condition, or it would always replace the page")
		}
		v.condition(va.When, path+"/when", 0)
		v.sectionList(va.Sections, path+"/sections")
	}
	if v.count > MaxSections {
		v.err("/sections", "Split it into more than one page.", "the page has %d sections; the most is %d", v.count, MaxSections)
	}
	if b, err := json.Marshal(p); err == nil && len(b) > MaxPageBytes {
		v.err("", "Split it into more than one page.", "the page is %d bytes; the most is %d", len(b), MaxPageBytes)
	}
	return v.issues
}

func isZeroCondition(c *Condition) bool {
	b, _ := json.Marshal(c)
	return string(b) == "{}"
}

func pageKind(k string) *PageKindSpec {
	kinds := Catalog().PageKinds
	for i := range kinds {
		if kinds[i].Kind == k {
			return &kinds[i]
		}
	}
	return nil
}

func pageKindNames() []string {
	var out []string
	for _, k := range Catalog().PageKinds {
		out = append(out, k.Kind)
	}
	return out
}

// sectionList checks one list of sections: the page's own, or a variant's.
func (v *validator) sectionList(list []Section, path string) {
	if len(list) > MaxChildren {
		v.err(path, "Group some under Tabs, or split the page.", "a page has at most %d top-level sections", MaxChildren)
	}
	ids := map[string]string{}
	grids := map[string]*Section{}
	v.sections(list, path, 1, "", ids, grids)
	// A FilterBar's target must be a Grid in the same list that follows filters.
	var bars func(list []Section, path string)
	bars = func(list []Section, path string) {
		for i := range list {
			s := &list[i]
			sp := path + "/" + strconv.Itoa(i)
			if s.Component == "FilterBar" {
				t := propStr(s.Props, "target", "")
				g := grids[t]
				var names []string
				for id := range grids {
					names = append(names, id)
				}
				sort.Strings(names)
				switch {
				case t == "":
				case g == nil:
					v.err(sp+"/props/target", orFix(suggest(t, names), "Set target to the ID of a Grid section on this page."), "the filter bar's target %q is not a Grid on this page", t)
				case !g.Source.Bool("filterable"):
					v.err(sp+"/props/target", "Set \"filterable\": true on the grid's source (the browse source has it).", "the grid %q doesn't follow filters", t)
				}
			}
			if s.Component == "Dock" {
				t := propStr(s.Props, "target", "")
				if t != "" && grids[t] == nil {
					v.err(sp+"/props/target", "Set target to the ID of a Grid section on this page.", "the dock's target %q is not a Grid on this page", t)
				}
				if t == "" && len(propList(s.Props, "controls", nil)) > 0 {
					v.err(sp+"/props/target", "Name the Grid the controls act on, or remove the controls.", "a dock's generated controls need a target")
				}
				if i != len(list)-1 {
					v.warn(sp, "Move the dock to the end of the page: it is drawn at the foot of the screen.", "a Dock is the last thing on a page")
				}
			}
			bars(s.Children, sp+"/children")
		}
	}
	bars(list, path)
}

func (v *validator) sections(list []Section, path string, depth int, parent string, ids map[string]string, grids map[string]*Section) {
	for i := range list {
		s := &list[i]
		sp := path + "/" + strconv.Itoa(i)
		v.count++
		if depth > MaxDepth {
			v.err(sp, "Flatten the page.", "sections are nested at most %d deep", MaxDepth)
			return
		}
		if !ValidID(s.ID) {
			v.err(sp+"/id", "Use a short name such as continue or recent.", "a section's ID is lower-case letters, digits, - and _, starting with a letter, at most %d long", MaxIDLength)
		} else if prev, dup := ids[s.ID]; dup {
			v.err(sp+"/id", "Give each section its own ID.", "the ID %q is already used at %s", s.ID, prev)
		}
		ids[s.ID] = sp
		if mod := ModuleOf(s.Component); mod != "" && v.ui.Component(s.Component) == nil {
			v.missingModule(sp+"/component", "Install or turn on the "+mod+" module, or remove the section.", "component", s.Component, mod)
			continue
		}
		if mc := v.ui.Component(s.Component); mc != nil {
			spec, _ := v.component(s.Component)
			v.props(spec.Props, s.Props, sp+"/props", spec.Label)
			if len(s.Children) > 0 {
				v.err(sp+"/children", "Remove them: the component brings its own.", "a %s can't hold other sections", spec.Label)
			}
			if len(s.Source) > 0 {
				v.err(sp+"/source", "Remove the source: the component brings its own.", "a %s doesn't take a data source", spec.Label)
			}
			if len(mc.PageKinds) > 0 && !contains(mc.PageKinds, v.page.Kind) {
				v.err(sp+"/component", "Use it on a "+strings.Join(mc.PageKinds, " or ")+" page.", "a %s only works on a %s page", spec.Label, strings.Join(mc.PageKinds, " or "))
			}
			// What it stands for is checked as if written out, on its own.
			ev := &validator{page: v.page, ui: v.ui, own: v.own}
			ev.sections([]Section{expandComponent(mc, *s)}, sp, depth, parent, map[string]string{}, map[string]*Section{})
			for _, is := range ev.issues {
				is.Message = spec.Label + ": " + is.Message
				v.issues = append(v.issues, is)
			}
			continue
		}
		spec, ok := v.component(s.Component)
		if !ok || !spec.Authored {
			v.err(sp+"/component", orFix(suggest(s.Component, authoredComponents()), "Choose a component from the catalogue: "+strings.Join(authoredComponents(), ", ")+"."), "%q is not a component", s.Component)
			continue
		}
		if s.Component == "Grid" {
			grids[s.ID] = s
		}
		if (s.Component == "Tab") != (parent == "Tabs") {
			if s.Component == "Tab" {
				v.err(sp+"/component", "Put the Tab inside a Tabs section.", "a Tab belongs inside Tabs")
			} else {
				v.err(sp+"/component", "Wrap it in a Tab.", "Tabs holds only Tab sections, not %s", s.Component)
			}
		}
		if parent == "List" && s.Component != "ListItem" {
			v.err(sp+"/component", "Use a ListItem.", "a List holds only ListItem sections, not %s", s.Component)
		}
		if parent == "Stack:row" && !contains([]string{"Button", "Text", "Image", "Spacer", "Divider", "SearchBox"}, s.Component) {
			v.err(sp+"/component", "Put it in a Stack that goes down, or directly on the page.", "a Stack that goes across holds only Button, Text, Image, Spacer, Divider and SearchBox, not %s", s.Component)
		}
		v.props(spec.Props, s.Props, sp+"/props", spec.Label)
		v.source(s, spec, sp, parent != "")
		v.condition(s.When, sp+"/when", 0)
		if s.Scope != "" && !scopePattern.MatchString(s.Scope) {
			v.err(sp+"/scope", "Use a module's ID such as wittytv, or a name of your own in lower case.", "%q is not a theme scope", s.Scope)
		}
		if s.Repeat != nil {
			v.repeat(s, sp, depth, parent)
		}
		// Things that open something name exactly one place.
		if contains([]string{"Button", "ListItem", "Notice", "Banner", "Image"}, s.Component) {
			n := 0
			for _, k := range []string{"to", "native", "url"} {
				if propStr(s.Props, k, "") != "" {
					n++
				}
			}
			if n > 1 {
				v.err(sp+"/props", "Keep one of to, native and url.", "a %s opens one thing, not %d", spec.Label, n)
			}
			if n == 0 && s.Component == "Button" {
				v.err(sp+"/props", "Set to (a place in the app), native (a screen of the app) or url (a web page).", "the button doesn't open anything")
			}
			if s.Component == "Notice" && (n > 0) != (propStr(s.Props, "buttonLabel", "") != "") {
				v.err(sp+"/props/buttonLabel", "Set both a buttonLabel and where it leads, or neither.", "a notice's button needs a label and somewhere to go")
			}
		}
		if len(s.Children) > 0 && !spec.Container {
			v.err(sp+"/children", "Move them out, or into a Stack.", "a %s can't hold other sections", spec.Label)
			continue
		}
		if len(s.Children) > MaxChildren {
			v.err(sp+"/children", "Split it.", "a section holds at most %d others", MaxChildren)
		}
		if s.Component == "Split" && len(s.Children) != 2 {
			v.err(sp+"/children", "Give it exactly two sections; use a Stack inside one for more.", "Two columns needs exactly two sections, not %d", len(s.Children))
		}
		if s.Component == "Tabs" && len(s.Children) == 0 {
			v.err(sp+"/children", "Add a Tab.", "Tabs needs at least one Tab")
		}
		if s.Component == "Tabs" {
			if d := propStr(s.Props, "default", ""); d != "" {
				found := false
				for _, c := range s.Children {
					found = found || c.ID == d
				}
				if !found {
					v.err(sp+"/props/default", "Use the ID of one of its tabs.", "%q is not one of this section's tabs", d)
				}
			}
		}
		if s.Component == "List" && len(s.Children) > 0 && s.Source.Type() != "" {
			v.err(sp+"/source", "Keep the rows or the source.", "a List is written by hand or filled from a source, not both")
		}
		if s.Component == "NativeView" && i != len(list)-1 {
			v.err(sp, "Move it to the end.", "the app's own view fills the rest of the page, so nothing can follow it")
		}
		if s.Component == "Grid" && i != len(list)-1 && s.Source.Type() != "" {
			v.warn(sp, "Move the grid to the end, or use a Row for the sections above it.", "a Grid loads more as it is scrolled, so the sections after it are only reached at its end")
		}
		child := s.Component
		if s.Component == "Stack" && propStr(s.Props, "direction", "column") == "row" {
			child = "Stack:row"
		}
		if len(spec.Children) > 0 {
			for j, c := range s.Children {
				if !contains(spec.Children, c.Component) {
					v.err(sp+"/children/"+strconv.Itoa(j)+"/component", "Use "+strings.Join(spec.Children, " or ")+".", "a %s holds only %s, not %s", spec.Label, strings.Join(spec.Children, " and "), c.Component)
				}
			}
		}
		v.sections(s.Children, sp+"/children", depth+1, child, ids, grids)
	}
}

func authoredComponents() []string {
	var out []string
	for _, c := range Catalog().Components {
		if c.Authored {
			out = append(out, c.Type)
		}
	}
	return out
}

func (v *validator) repeat(s *Section, sp string, depth int, parent string) {
	rp := s.Repeat
	if !contains(RepeatKinds, rp.Over) {
		v.err(sp+"/repeat/over", suggest(rp.Over, RepeatKinds), "%q is not something a section can be repeated over", rp.Over)
	}
	if s.Source.Type() == "" || len(s.Children) > 0 {
		v.err(sp+"/repeat", "Repeat a Row, Grid or Banner that has a source.", "only a section with a data source can be repeated")
	}
	if depth > 2 && parent != "Tab" {
		v.err(sp+"/repeat", "Move it to the top of the page or directly inside a Tab.", "a repeated section sits at the top of a page or directly inside a Tab")
	}
	for i, k := range rp.Kinds {
		if !contains(LibraryKinds, k) {
			v.err(sp+"/repeat/kinds/"+strconv.Itoa(i), suggest(k, LibraryKinds), "%q is not a kind of library", k)
		}
	}
	if len(rp.Kinds) > 0 && rp.Over != "libraries" {
		v.err(sp+"/repeat/kinds", "Remove kinds.", "kinds only applies when repeating over libraries")
	}
	if (rp.Over == "bookSeries" || rp.Over == "bookAuthors") && v.page != nil && v.page.Kind != "library" {
		v.err(sp+"/repeat/over", "Use it on a library page, or repeat over libraries.", "%s goes over the page's books library, so it works only on a library page", rp.Over)
	}
	if rp.Over == "personParts" && v.page != nil && v.page.Kind != "detail" {
		v.err(sp+"/repeat/over", "Use it on a person's page (a detail page matching mediaKind person).", "personParts goes over the page's person, so it works only on a detail page")
	}
	if rp.Limit < 0 || rp.Limit > 50 {
		v.err(sp+"/repeat/limit", "", "a repeat's limit is between 0 and 50")
	}
	uses := false
	for _, val := range s.Source {
		uses = uses || val == "$each"
	}
	if !uses {
		v.warn(sp+"/source", "Use \"$each\" for the library, view, collection, series or author.", "the section is repeated but its source never uses $each, so every copy would be the same")
	}
}

// props checks a section's props, or a source's parameters, against specs.
func (v *validator) props(specs []PropSpec, props map[string]any, path, what string) {
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var spec *PropSpec
		for i := range specs {
			if specs[i].Name == k {
				spec = &specs[i]
			}
		}
		if spec == nil {
			v.err(path+"/"+k, orFix(suggest(k, names), "Remove it. The properties are "+strings.Join(names, ", ")+"."), "%s has no property %q", what, k)
			continue
		}
		v.value(spec, props[k], path+"/"+k)
	}
	for i := range specs {
		if s := &specs[i]; s.Required {
			if val, ok := props[s.Name]; !ok || val == "" || val == nil {
				v.err(path+"/"+s.Name, "Set "+s.Name+": "+strings.ToLower(s.Label)+".", "%s needs %q", what, s.Name)
			}
		}
	}
}

func (v *validator) value(spec *PropSpec, val any, path string) {
	str, isStr := val.(string)
	isVar := isStr && strings.HasPrefix(str, "$")
	wantStr := func() bool {
		if !isStr {
			v.err(path, "Write it in quotes.", "%s is text", spec.Name)
		}
		return isStr
	}
	switch spec.Type {
	case PropString, PropText, PropMarkdown:
		if !wantStr() {
			return
		}
		if isVar && spec.Type == PropString {
			v.variable(str, path)
			return
		}
		limit := spec.MaxLength
		if limit == 0 {
			limit = 4000
		}
		if len([]rune(str)) > limit {
			v.err(path, "Shorten it.", "%s is at most %d characters", spec.Name, limit)
		}
		if strings.ContainsAny(str, "\x00\x1b") {
			v.err(path, "Remove the control characters.", "%s holds characters that can't be shown", spec.Name)
		}
		if spec.Type == PropMarkdown {
			v.markdown(str, path)
		}
	case PropInt:
		n, ok := toInt(val)
		if !ok {
			if isVar {
				v.variable(str, path)
				return
			}
			v.err(path, "Write a whole number without quotes.", "%s is a whole number", spec.Name)
			return
		}
		if (spec.Min != nil && n < *spec.Min) || (spec.Max != nil && n > *spec.Max) {
			v.err(path, "", "%s is between %d and %d", spec.Name, *spec.Min, *spec.Max)
		}
	case PropBool:
		if _, ok := val.(bool); !ok {
			v.err(path, "Write true or false without quotes.", "%s is true or false", spec.Name)
		}
	case PropEnum:
		if !wantStr() {
			return
		}
		if !contains(values(spec.Options), str) {
			v.err(path, suggest(str, values(spec.Options)), "%q is not a choice for %s; the choices are %s", str, spec.Name, strings.Join(values(spec.Options), ", "))
		}
	case PropEnumList:
		list, ok := val.([]any)
		if !ok {
			if sl, isSl := val.([]string); isSl {
				for _, s := range sl {
					list = append(list, s)
				}
				ok = true
			}
		}
		if !ok {
			v.err(path, "Write a list: [\"a\", \"b\"].", "%s is a list", spec.Name)
			return
		}
		seen := map[string]bool{}
		for i, x := range list {
			s, _ := x.(string)
			if !contains(values(spec.Options), s) {
				v.err(path+"/"+strconv.Itoa(i), suggest(s, values(spec.Options)), "%q is not a choice for %s; the choices are %s", s, spec.Name, strings.Join(values(spec.Options), ", "))
			} else if seen[s] {
				v.err(path+"/"+strconv.Itoa(i), "Remove the repeat.", "%q is listed twice", s)
			}
			seen[s] = true
		}
	case PropIcon:
		if wantStr() && !contains(Icons, str) {
			v.err(path, suggest(str, Icons), "%q is not one of the built-in icons", str)
		}
	case PropRoute:
		if wantStr() && !ValidRoute(str) {
			v.err(path, "Use a place in the app beginning with one /, such as /library/3. For a web page use url.", "%q is not a place in the app", str)
		}
	case PropImage:
		if wantStr() && (!ValidRoute(str) || !strings.HasPrefix(str, "/api/")) {
			v.err(path, "Use the address of a picture on this server, beginning /api/.", "%q is not a picture on this server", str)
		}
	case PropURL:
		if wantStr() && !ValidURL(str) {
			v.err(path, "Use a full address beginning http:// or https://.", "%q is not a web address", str)
		}
	case PropSection, PropScope:
		if wantStr() && !idPattern.MatchString(str) {
			v.err(path, "", "%q is not an ID", str)
		}
	case PropLibrary, PropRef:
		if isVar {
			v.variable(str, path)
			return
		}
		if n, ok := toInt(val); ok {
			if n <= 0 {
				v.err(path, "", "%s is a positive number", spec.Name)
			}
			return
		}
		if isStr {
			if _, err := strconv.ParseInt(str, 10, 64); err == nil && spec.Type == PropLibrary {
				return
			}
			if spec.Type == PropRef && len(str) <= 300 && str != "" {
				return
			}
		}
		v.err(path, "Use the ID number, or a variable such as $library or $each.", "%s is an ID or a $variable", spec.Name)
	}
}

// variable checks a "$name" against what the page's kind provides.
func (v *validator) variable(name, path string) {
	if name == "$each" {
		return
	}
	var allowed []string
	if k := pageKind(v.page.Kind); k != nil {
		allowed = k.Variables
	}
	if !contains(allowed, name) {
		fix := "A " + v.page.Kind + " page has no variables; write the value itself."
		if len(allowed) > 0 {
			fix = "On a " + v.page.Kind + " page the variables are " + strings.Join(allowed, ", ") + ", and $each in a repeated section."
		}
		v.err(path, fix, "%s means nothing on a %s page", name, v.page.Kind)
	}
}

// markdown checks the links of Markdown text: a place in the app or a web
// address, nothing else. Raw HTML is never interpreted, so it needs no check.
var mdLink = regexp.MustCompile(`\]\(([^)\s]*)[^)]*\)`)

func (v *validator) markdown(s, path string) {
	for _, m := range mdLink.FindAllStringSubmatch(s, -1) {
		if !ValidRoute(m[1]) && !ValidURL(m[1]) {
			v.err(path, "Link to a place in the app (/library/3) or a web address (https://…).", "the link %q doesn't lead anywhere the app can open", m[1])
		}
	}
}

func (v *validator) source(s *Section, spec *ComponentSpec, sp string, nested bool) {
	t := s.Source.Type()
	if len(s.Source) > 0 && t == "" {
		v.err(sp+"/source/type", "Name the source: \"type\": \"recentlyAdded\".", "the source has no type")
		return
	}
	switch {
	case t == "" && spec.Data == "required":
		v.err(sp+"/source", "Choose where its items come from, such as {\"type\": \"recentlyAdded\"}.", "a %s needs a data source", spec.Label)
		return
	case t != "" && spec.Data == "none":
		v.err(sp+"/source", "Remove the source.", "a %s doesn't take a data source", spec.Label)
		return
	case t == "":
		return
	}
	if mod := ModuleOf(t); mod != "" && v.ui.Source(t) == nil {
		v.missingModule(sp+"/source/type", "Install or turn on the "+mod+" module, or choose another source.", "source", t, mod)
		return
	}
	src, ok := v.sourceSpec(t)
	if !ok {
		var names []string
		for _, x := range Catalog().Sources {
			names = append(names, x.Type)
		}
		v.err(sp+"/source/type", orFix(suggest(t, names), "Choose a source from the catalogue: "+strings.Join(names, ", ")+"."), "%q is not a data source", t)
		return
	}
	if len(spec.Accepts) > 0 && !contains(spec.Accepts, t) {
		v.err(sp+"/source/type", "Use the "+strings.Join(spec.Accepts, " or ")+" source.", "a %s shows %s, not %s", spec.Label, strings.Join(spec.Accepts, " or "), t)
	}
	if len(src.PageKinds) > 0 && !contains(src.PageKinds, v.page.Kind) {
		v.err(sp+"/source/type", "Use it on a "+strings.Join(src.PageKinds, " or ")+" page.", "the %s source only works on a %s page", t, strings.Join(src.PageKinds, " or "))
	}
	params := map[string]any{}
	for k, val := range s.Source {
		if k != "type" && k != "map" {
			params[k] = val
		}
	}
	v.props(src.Params, params, sp+"/source", "the "+t+" source")
	if m, has := s.Source["map"]; has {
		mm, ok := m.(map[string]any)
		if !ok {
			v.err(sp+"/source/map", "Write {\"subtitle\": \"studio\"}.", "map pairs a card's title, subtitle or synopsis with an item field")
		}
		for prop, f := range mm {
			field, _ := f.(string)
			if !contains(MapProps, prop) {
				v.err(sp+"/source/map/"+prop, suggest(prop, MapProps), "map can set title, subtitle or synopsis, not %q", prop)
			}
			if !contains(MapFields, field) {
				v.err(sp+"/source/map/"+prop, orFix(suggest(field, MapFields), "Use one of the fields listed."), "%q is not an item field; the fields are %s", field, strings.Join(MapFields, ", "))
			}
		}
	}
	if style := propStr(s.Props, "cardStyle", "auto"); style != "auto" && s.Component != "List" {
		if cs, ok := CardStyle(style); ok && len(cs.Kinds) > 0 {
			match := false
			for _, k := range src.Yields {
				match = match || contains(cs.Kinds, k)
			}
			if !match && contains([]string{"channel", "episode", "chip"}, style) {
				v.warn(sp+"/props/cardStyle", "Use auto, or "+src.CardStyle+".", "the %s card style is made for %s, which the %s source doesn't give", style, strings.Join(cs.Kinds, " and "), t)
			}
		}
	}
}

func (v *validator) condition(c *Condition, path string, depth int) {
	if c == nil {
		return
	}
	if depth > 8 {
		v.err(path, "Flatten it.", "conditions are nested at most 8 deep")
		return
	}
	for i := range c.All {
		v.condition(&c.All[i], path+"/all/"+strconv.Itoa(i), depth+1)
	}
	for i := range c.Any {
		v.condition(&c.Any[i], path+"/any/"+strconv.Itoa(i), depth+1)
	}
	v.condition(c.Not, path+"/not", depth+1)
	if len(c.All) > 16 || len(c.Any) > 16 {
		v.err(path, "", "all and any hold at most 16 conditions")
	}
	list := func(field string, vals, allowed []string) {
		for i, x := range vals {
			if !contains(allowed, x) {
				v.err(path+"/"+field+"/"+strconv.Itoa(i), suggest(x, allowed), "%q is not a %s; the choices are %s", x, field, strings.Join(allowed, ", "))
			}
		}
	}
	list("formFactor", c.FormFactor, FormFactors)
	list("platform", c.Platform, Platforms)
	list("profileKind", c.ProfileKind, ProfileKinds)
	list("weekday", c.Weekday, Weekdays)
	one := func(field, val string, allowed []string) {
		if val != "" && !contains(allowed, val) {
			v.err(path+"/"+field, suggest(val, allowed), "%q is not a choice for %s; the choices are %s", val, field, strings.Join(allowed, ", "))
		}
	}
	one("can", c.Can, CanFeatures)
	one("hasLibrary", c.HasLibrary, LibraryKinds)
	one("libraryKind", c.LibraryKind, LibraryKinds)
	one("flag", c.Flag, Flags)
	for field, val := range map[string]string{"feature": c.Feature, "module": c.Module, "capability": c.Capability} {
		if val != "" && !wordPattern.MatchString(val) {
			v.err(path+"/"+field, "", "%q is not a name", val)
		}
	}
	for field, val := range map[string]string{"timeFrom": c.TimeFrom, "timeTo": c.TimeTo} {
		if val != "" && !clockPattern.MatchString(val) {
			v.err(path+"/"+field, "Write the time as HH:MM on a 24-hour clock, such as 18:30.", "%q is not a time of day", val)
		}
	}
	if c.MinWidth < 0 || c.MaxWidth < 0 || (c.MinWidth > 0 && c.MaxWidth > 0 && c.MinWidth > c.MaxWidth) {
		v.err(path+"/minWidth", "", "minWidth and maxWidth are positive, and minWidth is no more than maxWidth")
	}
	if c.Locale != "" && !localeTag.MatchString(c.Locale) {
		v.err(path+"/locale", "Use a language tag such as en or pt-BR.", "%q is not a language", c.Locale)
	}
	if c.LibraryKind != "" && v.page != nil && v.page.Kind != "library" && v.page.Kind != "detail" {
		v.warn(path+"/libraryKind", "Use hasLibrary to ask whether the profile has such a library.", "libraryKind asks about the page's own library, and a %s page has none", v.page.Kind)
	}
}

// ValidateNavigation checks the navigation. pageExists says whether a page ID
// is one the configuration or the built-in pages have.
func ValidateNavigation(n *Navigation, pageExists func(id string) bool) []Issue {
	v := &validator{page: &Page{Kind: "custom"}}
	if len(n.Items) > MaxNavItems {
		v.err("/items", "Put less used entries behind a page of links.", "the navigation has at most %d entries", MaxNavItems)
	}
	ids := map[string]bool{}
	for i := range n.Items {
		it := &n.Items[i]
		p := "/items/" + strconv.Itoa(i)
		if !ValidID(it.ID) {
			v.err(p+"/id", "Use a short name such as home.", "an entry's ID is lower-case letters, digits, - and _")
		} else if ids[it.ID] {
			v.err(p+"/id", "Give each entry its own ID.", "two entries are called %q", it.ID)
		}
		ids[it.ID] = true
		if len([]rune(it.Label)) > 40 {
			v.err(p+"/label", "Shorten it.", "a label is at most 40 characters")
		}
		if it.Icon != "" && !contains(Icons, it.Icon) {
			v.err(p+"/icon", suggest(it.Icon, Icons), "%q is not one of the built-in icons", it.Icon)
		}
		switch it.Type {
		case "page":
			if !ValidID(it.Page) {
				v.err(p+"/page", "Name the page by its ID.", "the entry doesn't say which page it opens")
			} else if pageExists != nil && !pageExists(it.Page) {
				v.err(p+"/page", "Create the page first, or remove the entry.", "there is no page called %q", it.Page)
			}
		case "libraries":
		case "library":
			if it.Library <= 0 {
				v.err(p+"/library", "Give the library's ID.", "the entry doesn't say which library it opens")
			}
		case "native":
			if menu := menuScreens(); !contains(menu, it.Screen) {
				v.err(p+"/screen", suggest(it.Screen, menu), "%q is not a screen an entry can open; the choices are %s", it.Screen, strings.Join(menu, ", "))
			}
			if it.Screen == "settings" && it.When != nil {
				v.err(p+"/when", "Remove the condition.", "Settings is always in the navigation; it can't have a condition")
			}
			if it.Screen == "settings" {
				for ff, where := range it.Placement {
					if where == "hidden" {
						v.err(p+"/placement/"+ff, "Use more or end to keep it out of the way.", "Settings can't be hidden")
					}
				}
			}
		case "route":
			if !ValidRoute(it.Route) {
				v.err(p+"/route", "Use a place in the app beginning with one /.", "%q is not a place in the app", it.Route)
			}
			if it.Label == "" {
				v.err(p+"/label", "Give it a label.", "an entry that opens a route needs a label")
			}
		default:
			v.err(p+"/type", suggest(it.Type, []string{"page", "libraries", "library", "native", "route"}), "%q is not a kind of entry; the kinds are page, libraries, library, native and route", it.Type)
		}
		v.condition(it.When, p+"/when", 0)
		for ff, where := range it.Placement {
			if !contains(FormFactors, ff) {
				v.err(p+"/placement/"+ff, suggest(ff, FormFactors), "%q is not a form factor", ff)
			}
			if !contains(Placements, where) {
				v.err(p+"/placement/"+ff, suggest(where, Placements), "%q is not a placement; the choices are %s", where, strings.Join(Placements, ", "))
			}
		}
	}
	return v.issues
}

// ValidatePersonal checks what a profile sent about its own arrangement.
func ValidatePersonal(p *Personal) []Issue {
	v := &validator{}
	ids := func(field string, list []string) {
		if len(list) > MaxChildren*2 {
			v.err("/"+field, "", "%s names at most %d sections", field, MaxChildren*2)
		}
		for i, id := range list {
			if len(id) == 0 || len(id) > MaxNodeIDLength || safeID(id) != id {
				v.err("/"+field+"/"+strconv.Itoa(i), "", "%q is not a section's ID", id)
			}
		}
	}
	ids("order", p.Order)
	ids("hidden", p.Hidden)
	ids("pinned", p.Pinned)
	if p.Density != "" && !contains(Densities, p.Density) {
		v.err("/density", suggest(p.Density, Densities), "%q is not a density; the choices are %s", p.Density, strings.Join(Densities, ", "))
	}
	if len(p.CardSize) > MaxChildren || len(p.DefaultTab) > MaxChildren {
		v.err("/cardSize", "", "too many sections")
	}
	for id, size := range p.CardSize {
		if safeID(id) != id || !contains(CardSizes, size) {
			v.err("/cardSize/"+id, "", "a card size is s, m or l, for a section's ID")
		}
	}
	for id, tab := range p.DefaultTab {
		if safeID(id) != id || safeID(tab) != tab {
			v.err("/defaultTab/"+id, "", "a default tab is a tab's ID, for a Tabs section's ID")
		}
	}
	return v.issues
}

// ValidRoute reports whether s is a place in the app: a path on this server
// beginning with exactly one slash, with nothing that could lead elsewhere.
func ValidRoute(s string) bool {
	if len(s) == 0 || len(s) > 600 || s[0] != '/' || strings.HasPrefix(s, "//") || strings.ContainsAny(s, "\\\x00\r\n\t ") {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "" && u.Host == ""
}

// ValidURL reports whether s is an http or https address.
func ValidURL(s string) bool {
	if len(s) > 2000 || strings.ContainsAny(s, "\x00\r\n\t ") {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		if x == float64(int(x)) {
			return int(x), true
		}
	case int:
		return x, true
	case int64:
		return int(x), true
	case json.Number:
		n, err := x.Int64()
		return int(n), err == nil
	}
	return 0, false
}

// orFix is the suggestion, or the fallback when nothing was near enough.
func orFix(suggestion, fallback string) string {
	if suggestion != "" {
		return suggestion
	}
	return fallback
}

// suggest answers "Did you mean …?" for the name in list nearest to s, or ""
// when nothing is near.
func suggest(s string, list []string) string {
	best, bestD := "", 4
	ls := strings.ToLower(s)
	for _, c := range list {
		lc := strings.ToLower(c)
		d := editDistance(ls, lc)
		if strings.HasPrefix(lc, ls) && ls != "" {
			d = min(d, 1)
		}
		if d < bestD {
			best, bestD = c, d
		}
	}
	if best == "" || bestD > max(1, len(s)/2) {
		return ""
	}
	return "Did you mean \"" + best + "\"?"
}

func editDistance(a, b string) int {
	if len(a) > 64 || len(b) > 64 {
		return 99
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
