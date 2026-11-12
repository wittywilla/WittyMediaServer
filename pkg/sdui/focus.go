// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

// Focusable reports whether a person can act on a rendered node, so that it
// takes a place in the focus order: a card, a tab, a track, a search box, a
// filter bar, a hero with something in it, a title header with buttons, and
// anything else that has an action. A Card inside a Hero doesn't: the Hero
// is one stop and moves between its own titles.
func Focusable(n *Node, inHero bool) bool {
	switch n.Type {
	case "Card":
		return !inHero
	case "Tab", "Track", "SearchBox", "FilterBar":
		return true
	case "Hero":
		return len(n.Children) > 0
	case "DetailHeader":
		_, ok := n.Props["actions"]
		return ok
	}
	return n.Action != nil
}

// focusGroup reports whether a node's children form one focus section.
func focusGroup(n *Node) bool {
	if spec, ok := Component(n.Type); ok && spec.Focus == "group" {
		return true
	}
	return n.Type == "Stack" && n.Props["direction"] == "row"
}

// stampFocus numbers every focusable node in reading order (depth first, in
// display order), names the group each is in, and marks the one focus starts
// on:
//
//  1. a SearchBox with autofocus;
//  2. else the page's DetailHeader;
//  3. else the first focusable node that isn't a Tab, a FilterBar or a
//     SearchBox: the content, not the controls above it;
//  4. else the first focusable node.
func stampFocus(root *Node) {
	order := 0
	var first, content, header, search *Node
	var walk func(n *Node, group string, inHero bool)
	var stamp func(n *Node, group string, inHero bool)
	inDock := false // the Dock's buttons are controls, not content: focus doesn't start there
	walk = func(n *Node, group string, inHero bool) {
		if n.Type == "Dock" {
			inDock = true
			defer func() { inDock = false }()
		}
		if n.Type != "Tab" { // a Tab was stamped with its strip
			stamp(n, group, inHero)
		}
		if n.Type == "Tabs" || n.Type == "CrossBar" {
			// The strip first, whole, then the chosen tab's content: its
			// tabs are neighbours in the order, not split by what they hold.
			for i := range n.Children {
				stamp(&n.Children[i], n.ID, inHero)
			}
			for i := range n.Children {
				for j := range n.Children[i].Children {
					walk(&n.Children[i].Children[j], "", inHero)
				}
			}
			return
		}
		if focusGroup(n) {
			group = n.ID
		}
		for i := range n.Children {
			walk(&n.Children[i], group, inHero || n.Type == "Hero")
		}
	}
	stamp = func(n *Node, group string, inHero bool) {
		n.Focus = nil
		if Focusable(n, inHero) {
			order++
			n.Focus = &Focus{Order: order, Group: group}
			if first == nil {
				first = n
			}
			switch n.Type {
			case "Tab", "FilterBar":
			case "SearchBox":
				if search == nil && n.Props["autofocus"] == true {
					search = n
				}
			case "DetailHeader":
				if header == nil {
					header = n
				}
				fallthrough
			default:
				if content == nil && !inDock {
					content = n
				}
			}
		}
	}
	walk(root, "", false)
	for _, n := range []*Node{search, header, content, first} {
		if n != nil {
			n.Focus.Default = true
			return
		}
	}
}
