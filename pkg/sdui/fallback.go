// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

// What a client that can't draw a component is sent instead. Every optional
// component has one fallback, made of required components only, so a page
// written for today's catalogue still shows on an app that knows less of it.

// fallback draws a section as its component's fallback.
func (r *renderer) fallback(in instance, depth int) ([]Node, bool) {
	s := in.sec
	alt := *s
	alt.Props = map[string]any{}
	switch s.Component {
	case "Hero":
		// The same titles as a row of large landscape cards.
		alt.Component = "Shelf"
		alt.Props["title"] = propStr(s.Props, "title", "Featured")
		alt.Props["cardStyle"], alt.Props["cardSize"] = "landscape", "l"
		alt.Props["maxItems"] = propInt(s.Props, "maxItems", 5)
		alt.Props["seeAll"] = "never"
	case "SeasonList":
		// One season's episodes as a row.
		alt.Component = "Shelf"
		alt.Props["title"] = "Episodes"
		alt.Props["cardStyle"] = "landscape"
	case "TrackList":
		alt.Component = "List"
	case "Banner":
		n := Node{Type: "Banner", ID: in.id, Props: map[string]any{}}
		if !r.banner(&n, in) {
			return nil, false
		}
		out := Node{Type: "Notice", ID: r.uniqueID(in.id), Props: map[string]any{"tone": "info", "text": firstNonEmpty(propStr(n.Props, "text", ""), propStr(n.Props, "title", ""))}, Action: n.Action}
		if propStr(n.Props, "text", "") != "" && propStr(n.Props, "title", "") != "" {
			out.Props["title"] = n.Props["title"]
		}
		if out.Action != nil {
			out.Props["buttonLabel"] = propStr(n.Props, "buttonLabel", "Open")
		}
		r.scope(&out, s.Scope)
		return []Node{out}, true
	case "DetailHeader":
		return r.headerFallback(in)
	case "Tabs":
		// The chosen tab's content alone.
		n := Node{Type: "Tabs", ID: in.id, Props: map[string]any{}}
		if !r.tabs(&n, in, depth) {
			return nil, false
		}
		for _, t := range n.Children {
			if t.Props["selected"] == true && len(t.Children) > 0 {
				out := Node{Type: "Stack", ID: n.ID, Children: t.Children}
				return []Node{out}, true
			}
		}
		return nil, false
	case "FilterBar":
		return nil, false
	case "SearchBox":
		out := Node{Type: "Button", ID: r.uniqueID(in.id), Props: map[string]any{"label": "Search", "icon": "search", "variant": "secondary"}, Action: &Action{Type: ActNavigate, To: "/search"}}
		return []Node{out}, true
	case "Split", "Tab", "NativeView":
		alt.Component = "Stack"
	default:
		return nil, false
	}
	if !r.ctx.supports(alt.Component) {
		return nil, false
	}
	return r.build(instance{sec: &alt, id: in.id, each: in.each}, depth)
}

// headerFallback is a DetailHeader from required components: the backdrop,
// the title, the facts on one line, the synopsis and a row of buttons.
func (r *renderer) headerFallback(in instance) ([]Node, bool) {
	h := Node{Type: "DetailHeader", ID: in.id, Props: map[string]any{}}
	if !r.header(&h, in) {
		return nil, false
	}
	id := r.uniqueID(in.id)
	out := Node{Type: "Stack", ID: id, Props: map[string]any{"direction": "column", "gap": "m"}}
	sub := func(suffix, typ string, props map[string]any) Node {
		return Node{Type: typ, ID: r.uniqueID(id + ":" + suffix), Props: props}
	}
	if images, _ := h.Props["images"].(map[string]any); images != nil {
		if b, _ := images["backdrop"].(map[string]any); b != nil {
			out.Children = append(out.Children, sub("art", "Image", map[string]any{"image": b, "aspect": "16/9", "alt": ""}))
		}
	}
	out.Children = append(out.Children, sub("title", "Text", map[string]any{"text": h.Props["title"], "role": "pageTitle"}))
	if facts, _ := h.Props["facts"].([]any); len(facts) > 0 {
		line := ""
		for i, f := range facts {
			if i > 0 {
				line += " · "
			}
			line += f.(map[string]any)["value"].(string)
		}
		out.Children = append(out.Children, sub("facts", "Text", map[string]any{"text": line, "role": "caption"}))
	}
	if o, _ := h.Props["overview"].(string); o != "" {
		out.Children = append(out.Children, sub("overview", "Text", map[string]any{"text": o, "role": "body"}))
	}
	if acts, _ := h.Props["actions"].([]any); len(acts) > 0 {
		row := sub("actions", "Stack", map[string]any{"direction": "row", "gap": "s"})
		for _, a := range acts {
			m := a.(map[string]any)
			variant := "secondary"
			if m["primary"] == true {
				variant = "primary"
			}
			b := sub("actions:"+m["id"].(string), "Button", map[string]any{"label": m["label"], "icon": m["icon"], "variant": variant})
			b.Action = m["action"].(*Action)
			row.Children = append(row.Children, b)
		}
		out.Children = append(out.Children, row)
	}
	return []Node{out}, true
}
