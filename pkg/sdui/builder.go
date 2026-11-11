// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

// DocumentBuilder assembles a Document fluently. Obtain one with NewDocument
// and finish with Build, which validates the result.
type DocumentBuilder struct {
	doc  Document
	root *NodeBuilder
}

// NewDocument starts a builder for the named screen at SchemaVersion.
func NewDocument(screen string) *DocumentBuilder {
	return &DocumentBuilder{doc: Document{Version: SchemaVersion, Screen: screen}}
}

// Theme sets the theme id, creating the ThemeRef if needed.
func (b *DocumentBuilder) Theme(id string) *DocumentBuilder {
	if b.doc.Theme == nil {
		b.doc.Theme = &ThemeRef{}
	}
	b.doc.Theme.ID = id
	return b
}

// ModuleOverride records a per-module token colour override on the theme.
// If no theme has been set, the "default" theme is assumed.
func (b *DocumentBuilder) ModuleOverride(module, token, color string) *DocumentBuilder {
	if b.doc.Theme == nil {
		b.doc.Theme = &ThemeRef{ID: "default"}
	}
	if b.doc.Theme.ModuleOverrides == nil {
		b.doc.Theme.ModuleOverrides = map[string]map[string]string{}
	}
	if b.doc.Theme.ModuleOverrides[module] == nil {
		b.doc.Theme.ModuleOverrides[module] = map[string]string{}
	}
	b.doc.Theme.ModuleOverrides[module][token] = color
	return b
}

// Action registers a named action that nodes may reference with OnPress.
func (b *DocumentBuilder) Action(name string, a Action) *DocumentBuilder {
	if b.doc.Actions == nil {
		b.doc.Actions = map[string]Action{}
	}
	b.doc.Actions[name] = a
	return b
}

// Meta sets the document metadata. GeneratedAt and ETag are overwritten by
// Render.
func (b *DocumentBuilder) Meta(m Meta) *DocumentBuilder {
	b.doc.Meta = m
	return b
}

// Root sets the root node. The builder's node should have Type Screen.
func (b *DocumentBuilder) Root(n *NodeBuilder) *DocumentBuilder {
	b.root = n
	return b
}

// Document returns the assembled document without validating it.
func (b *DocumentBuilder) Document() Document {
	d := b.doc
	if b.root != nil {
		d.Root = b.root.Node()
	}
	return d
}

// Build assembles and validates the document.
func (b *DocumentBuilder) Build() (Document, error) {
	d := b.Document()
	if err := Validate(&d); err != nil {
		return Document{}, err
	}
	return d, nil
}

// NodeBuilder assembles one Node and its subtree fluently.
type NodeBuilder struct {
	n Node
}

// N starts a builder for a node of type t with the given id.
func N(t ComponentType, id string) *NodeBuilder {
	return &NodeBuilder{n: Node{Type: t, ID: id}}
}

// Prop sets one prop.
func (nb *NodeBuilder) Prop(k string, v any) *NodeBuilder {
	if nb.n.Props == nil {
		nb.n.Props = map[string]any{}
	}
	nb.n.Props[k] = v
	return nb
}

// Props merges a map of props.
func (nb *NodeBuilder) Props(m map[string]any) *NodeBuilder {
	for k, v := range m {
		nb.Prop(k, v)
	}
	return nb
}

// Style lets fn edit the node's Style, creating it if needed.
func (nb *NodeBuilder) Style(fn func(*Style)) *NodeBuilder {
	if nb.n.Style == nil {
		nb.n.Style = &Style{}
	}
	fn(nb.n.Style)
	return nb
}

// Module scopes the node's Style to a module id (see Style.Module).
func (nb *NodeBuilder) Module(module string) *NodeBuilder {
	return nb.Style(func(s *Style) { s.Module = module })
}

// Child appends children, in order. Nil builders are skipped.
func (nb *NodeBuilder) Child(children ...*NodeBuilder) *NodeBuilder {
	for _, c := range children {
		if c == nil {
			continue
		}
		nb.n.Children = append(nb.n.Children, c.Node())
	}
	return nb
}

// ChildNodes appends already-built nodes.
func (nb *NodeBuilder) ChildNodes(nodes ...Node) *NodeBuilder {
	nb.n.Children = append(nb.n.Children, nodes...)
	return nb
}

// If sets the node's visibility condition.
func (nb *NodeBuilder) If(c Condition) *NodeBuilder {
	cc := c
	nb.n.Visibility = &cc
	return nb
}

// Bind binds prop to a data path.
func (nb *NodeBuilder) Bind(prop, path string) *NodeBuilder {
	return nb.BindWith(prop, Binding{Path: path})
}

// BindWith binds prop with a full Binding (path, default, format).
func (nb *NodeBuilder) BindWith(prop string, b Binding) *NodeBuilder {
	if nb.n.Bindings == nil {
		nb.n.Bindings = map[string]Binding{}
	}
	nb.n.Bindings[prop] = b
	return nb
}

// OnPress sets the named action dispatched when the node is activated.
func (nb *NodeBuilder) OnPress(actionName string) *NodeBuilder {
	nb.n.Action = actionName
	return nb
}

// Node returns a copy of the assembled node.
func (nb *NodeBuilder) Node() Node {
	return cloneNode(nb.n)
}

// ---- convenience constructors ----

// ScreenNode starts a Screen root node.
func ScreenNode(id string) *NodeBuilder { return N(Screen, id) }

// StackOf starts a vertical Stack.
func StackOf(id string) *NodeBuilder {
	return N(Stack, id).Style(func(s *Style) { s.Direction = "column" })
}

// Row starts a horizontal Stack.
func Row(id string) *NodeBuilder {
	return N(Stack, id).Style(func(s *Style) { s.Direction = "row" })
}

// GridOf starts a Grid with the given column count.
func GridOf(id string, columns int) *NodeBuilder {
	return N(Grid, id).Style(func(s *Style) { s.Columns = columns })
}

// ShelfOf starts a titled Shelf.
func ShelfOf(id, title string) *NodeBuilder { return N(Shelf, id).Prop("title", title) }

// PosterRowOf starts a titled PosterRow.
func PosterRowOf(id, title string) *NodeBuilder { return N(PosterRow, id).Prop("title", title) }

// Hero starts a HeroBanner.
func Hero(id string) *NodeBuilder { return N(HeroBanner, id) }

// TextOf starts a Text node with literal text.
func TextOf(id, text string) *NodeBuilder { return N(Text, id).Prop("text", text) }

// ButtonOf starts a Button with a label that dispatches action.
func ButtonOf(id, label, action string) *NodeBuilder {
	return N(Button, id).Prop("label", label).OnPress(action)
}

// ImageOf starts an Image.
func ImageOf(id, src string) *NodeBuilder { return N(Image, id).Prop("src", src) }

// CardOf starts a Card for an item.
func CardOf(id string, it Item) *NodeBuilder {
	nb := N(Card, id).Prop("title", it.Title)
	if it.Subtitle != "" {
		nb.Prop("subtitle", it.Subtitle)
	}
	if it.ImageURL != "" {
		nb.Prop("imageUrl", it.ImageURL)
	}
	if it.Kind != "" {
		nb.Prop("kind", it.Kind)
	}
	if it.ID != "" {
		nb.Prop("itemId", it.ID)
	}
	if it.Progress > 0 {
		nb.Prop("progress", it.Progress)
	}
	if len(it.Badges) > 0 {
		nb.Prop("badges", append([]string(nil), it.Badges...))
	}
	if it.Module != "" {
		nb.Prop("module", it.Module).Module(it.Module)
	}
	return nb
}

// BadgeOf starts a Badge.
func BadgeOf(id, text string) *NodeBuilder { return N(Badge, id).Prop("text", text) }

// Progress starts a ProgressBar with value 0..1.
func Progress(id string, value float64) *NodeBuilder {
	return N(ProgressBar, id).Prop("value", value)
}

// SpacerOf starts a Spacer of the given size in CSS pixels.
func SpacerOf(id string, size int) *NodeBuilder { return N(Spacer, id).Prop("size", size) }

// DividerOf starts a Divider.
func DividerOf(id string) *NodeBuilder { return N(Divider, id) }

// TabsOf starts a Tabs container.
func TabsOf(id string) *NodeBuilder { return N(Tabs, id) }

// TabOf starts a Tab with a label.
func TabOf(id, label string) *NodeBuilder { return N(Tab, id).Prop("label", label) }

// ListOf starts a List.
func ListOf(id string) *NodeBuilder { return N(List, id) }

// ListItemOf starts a ListItem with a title.
func ListItemOf(id, title string) *NodeBuilder { return N(ListItem, id).Prop("title", title) }

// VideoOf starts a Video.
func VideoOf(id, src string) *NodeBuilder { return N(Video, id).Prop("src", src) }

// SearchBarOf starts a SearchBar.
func SearchBarOf(id, placeholder string) *NodeBuilder {
	return N(SearchBar, id).Prop("placeholder", placeholder)
}

// ToolbarOf starts a Toolbar.
func ToolbarOf(id string) *NodeBuilder { return N(Toolbar, id) }

// ModalOf starts a Modal with a title.
func ModalOf(id, title string) *NodeBuilder { return N(Modal, id).Prop("title", title) }

// CarouselOf starts a Carousel.
func CarouselOf(id string) *NodeBuilder { return N(Carousel, id) }

// ---- action constructors ----

// NavigateTo returns a Navigate action.
func NavigateTo(route string) Action { return Action{Type: Navigate, Target: route} }

// PlayItem returns a Play action.
func PlayItem(itemID string) Action { return Action{Type: Play, Target: itemID} }

// OpenModalAction returns an OpenModal action for the given node id.
func OpenModalAction(modalID string) Action { return Action{Type: OpenModal, Target: modalID} }

// CloseModalAction returns a CloseModal action.
func CloseModalAction() Action { return Action{Type: CloseModal} }

// FetchFrom returns a Fetch action.
func FetchFrom(url string) Action { return Action{Type: Fetch, Target: url} }

// BackAction returns a Back action.
func BackAction() Action { return Action{Type: Back} }

// ---- condition helpers ----

// HasFeature is true when the feature is enabled for the requester.
func HasFeature(f string) Condition { return Condition{Feature: f} }

// IsRole is true when the requester has the role.
func IsRole(r string) Condition { return Condition{Role: r} }

// OnPlatform is true on the named platform.
func OnPlatform(p string) Condition { return Condition{Platform: p} }

// ModuleEnabled is true when the module is installed and enabled.
func ModuleEnabled(m string) Condition { return Condition{Module: m} }

// HasFlag is true when the named flag is set.
func HasFlag(f string) Condition { return Condition{Flag: f} }

// InLocale is true when the requester's locale starts with the prefix.
func InLocale(prefix string) Condition { return Condition{Locale: prefix} }

// MinWidth is true when the viewport is at least n pixels wide.
func MinWidth(n int) Condition { return Condition{MinWidth: n} }

// MaxWidth is true when the viewport is at most n pixels wide.
func MaxWidth(n int) Condition { return Condition{MaxWidth: n} }

// All is true when every condition is true.
func All(cs ...Condition) Condition { return Condition{All: cs} }

// Any is true when at least one condition is true.
func Any(cs ...Condition) Condition { return Condition{Any: cs} }

// Not inverts a condition.
func Not(c Condition) Condition { return Condition{Not: &c} }
