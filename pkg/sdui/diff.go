// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// PatchOp is the kind of a Patch.
type PatchOp string

// Patch operations.
const (
	// PatchAdd inserts Node at Path ("#parent/children/i").
	PatchAdd PatchOp = "add"
	// PatchRemove deletes the node at Path ("#id").
	PatchRemove PatchOp = "remove"
	// PatchReplace replaces the node at Path ("#id") with Node — all fields
	// except Children, which are patched separately — or, for document-level
	// paths ("/page", "/layout", "/theme", "/personal", "/screenTime", "/catalog", "/meta",
	// "/root"), sets Value (or Node for "/root"); a null Value clears the field.
	PatchReplace PatchOp = "replace"
	// PatchMove moves the node From ("#id") to Path ("#parent/children/i").
	PatchMove PatchOp = "move"
)

// Patch is one step in transforming a document into another. Paths are ID
// based: "#<id>" names a node, "#<id>/children/<n>" a position among its
// children, and "/root", "/page", "/layout", "/theme", "/personal", "/catalog"
// and "/meta" the document-level fields.
type Patch struct {
	Op PatchOp `json:"op"`
	// Path is the target location.
	Path string `json:"path"`
	// Node is the payload for add and node-level replace.
	Node *Node `json:"node,omitempty"`
	// Value is the payload for document-level replace.
	Value any `json:"value,omitempty"`
	// From is the source node ("#id") for move.
	From string `json:"from,omitempty"`
}

// Diff computes the patches that turn old into new. Children are matched by
// ID; a node whose own fields (type, props, action, focus, state, scope)
// changed yields a replace, a new ID an add, a missing ID a remove, and a
// change of order a move. Meta.GeneratedAt and Meta.ETag are ignored. Diff is
// deterministic.
func Diff(old, new Document) []Patch {
	var ps []Patch
	oh, nh := head(old), head(new)
	for _, f := range headFields {
		if !jsonEqual(oh[f], nh[f]) {
			ps = append(ps, Patch{Op: PatchReplace, Path: "/" + f, Value: nh[f]})
		}
	}
	if old.Root.ID != new.Root.ID {
		n := cloneNode(new.Root)
		ps = append(ps, Patch{Op: PatchReplace, Path: "/root", Node: &n})
		return ps
	}
	ps = diffNode(ps, old.Root, new.Root)
	return ps
}

// headFields are the document-level fields a patch can replace.
var headFields = []string{"catalog", "page", "layout", "theme", "personal", "screenTime", "meta"}

// head is a document's fields other than its tree, as JSON values.
func head(d Document) map[string]any {
	d.Meta = stripVolatileMeta(d.Meta)
	d.Root = Node{}
	b, _ := json.Marshal(d)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

func stripVolatileMeta(m Meta) Meta {
	m.GeneratedAt = time.Time{}
	m.ETag = ""
	return m
}

func diffNode(ps []Patch, o, n Node) []Patch {
	if !jsonEqual(withoutChildren(o), withoutChildren(n)) {
		nn := withoutChildren(n)
		ps = append(ps, Patch{Op: PatchReplace, Path: "#" + n.ID, Node: &nn})
	}
	return diffChildren(ps, n.ID, o.Children, n.Children)
}

func withoutChildren(n Node) Node {
	c := cloneNode(n)
	c.Children = nil
	return c
}

func diffChildren(ps []Patch, parentID string, old, new []Node) []Patch {
	newByID := make(map[string]int, len(new))
	for i, c := range new {
		newByID[c.ID] = i
	}
	oldByID := make(map[string]Node, len(old))
	for _, c := range old {
		oldByID[c.ID] = c
	}

	// 1. removals, then the surviving old order.
	work := make([]string, 0, len(old))
	for _, c := range old {
		if _, ok := newByID[c.ID]; !ok {
			ps = append(ps, Patch{Op: PatchRemove, Path: "#" + c.ID})
			continue
		}
		work = append(work, c.ID)
	}

	// 2. walk the new order, inserting or moving so that work[:i+1] == new[:i+1].
	for i, c := range new {
		pos := indexOf(work, c.ID)
		switch {
		case pos < 0:
			nn := cloneNode(c)
			ps = append(ps, Patch{Op: PatchAdd, Path: childPath(parentID, i), Node: &nn})
			work = insertAt(work, i, c.ID)
		case pos != i:
			ps = append(ps, Patch{Op: PatchMove, Path: childPath(parentID, i), From: "#" + c.ID})
			work = append(work[:pos], work[pos+1:]...)
			work = insertAt(work, i, c.ID)
			ps = diffNode(ps, oldByID[c.ID], c)
		default:
			ps = diffNode(ps, oldByID[c.ID], c)
		}
	}
	return ps
}

func childPath(parentID string, i int) string {
	return "#" + parentID + "/children/" + strconv.Itoa(i)
}

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return -1
}

func insertAt(list []string, i int, s string) []string {
	if i >= len(list) {
		return append(list, s)
	}
	list = append(list, "")
	copy(list[i+1:], list[i:])
	list[i] = s
	return list
}

// Apply returns a copy of doc with patches applied in order. It fails on a
// malformed path, an unknown node ID, an out-of-range index or an operation
// missing its payload; doc is never modified.
func Apply(doc Document, patches []Patch) (Document, error) {
	out := cloneDocument(doc)
	for i, p := range patches {
		if err := applyOne(&out, p); err != nil {
			return Document{}, fmt.Errorf("sdui: patch %d (%s %s): %w", i, p.Op, p.Path, err)
		}
	}
	return out, nil
}

func applyOne(d *Document, p Patch) error {
	if strings.HasPrefix(p.Path, "/") {
		if p.Op != PatchReplace {
			return fmt.Errorf("document-level paths only support replace")
		}
		return applyDocField(d, p)
	}
	id, idx, hasIdx, err := parseNodePath(p.Path)
	if err != nil {
		return err
	}
	switch p.Op {
	case PatchAdd:
		if !hasIdx {
			return fmt.Errorf("add needs a #parent/children/<n> path")
		}
		if p.Node == nil {
			return fmt.Errorf("add needs a node")
		}
		parent := findNode(&d.Root, id)
		if parent == nil {
			return fmt.Errorf("unknown parent %q", id)
		}
		if idx > len(parent.Children) {
			return fmt.Errorf("index %d out of range (have %d children)", idx, len(parent.Children))
		}
		parent.Children = insertNode(parent.Children, idx, cloneNode(*p.Node))
	case PatchRemove:
		if hasIdx {
			return fmt.Errorf("remove takes a #id path")
		}
		if id == d.Root.ID {
			return fmt.Errorf("cannot remove the root")
		}
		parent, pos := findParent(&d.Root, id)
		if parent == nil {
			return fmt.Errorf("unknown node %q", id)
		}
		parent.Children = removeNode(parent.Children, pos)
	case PatchReplace:
		if hasIdx {
			return fmt.Errorf("replace takes a #id path")
		}
		if p.Node == nil {
			return fmt.Errorf("replace needs a node")
		}
		target := findNode(&d.Root, id)
		if target == nil {
			return fmt.Errorf("unknown node %q", id)
		}
		children := target.Children
		*target = cloneNode(*p.Node)
		target.Children = children
	case PatchMove:
		if !hasIdx {
			return fmt.Errorf("move needs a #parent/children/<n> path")
		}
		fromID, _, fromIdx, err := parseNodePath(p.From)
		if err != nil || fromIdx {
			return fmt.Errorf("move needs a #id from")
		}
		parent := findNode(&d.Root, id)
		if parent == nil {
			return fmt.Errorf("unknown parent %q", id)
		}
		srcParent, pos := findParent(&d.Root, fromID)
		if srcParent == nil {
			return fmt.Errorf("unknown node %q", fromID)
		}
		if srcParent != parent {
			return fmt.Errorf("move must stay within one parent")
		}
		moved := parent.Children[pos]
		parent.Children = removeNode(parent.Children, pos)
		if idx > len(parent.Children) {
			return fmt.Errorf("index %d out of range", idx)
		}
		parent.Children = insertNode(parent.Children, idx, moved)
	default:
		return fmt.Errorf("unknown op %q", p.Op)
	}
	return nil
}

func applyDocField(d *Document, p Patch) error {
	if p.Path == "/root" {
		if p.Node == nil {
			return fmt.Errorf("replace /root needs a node")
		}
		d.Root = cloneNode(*p.Node)
		return nil
	}
	field := strings.TrimPrefix(p.Path, "/")
	known := false
	for _, f := range headFields {
		known = known || f == field
	}
	if !known {
		return fmt.Errorf("unknown document path %q", p.Path)
	}
	// Set the one field by way of JSON, so a patch that arrived as JSON and
	// one made here apply alike.
	h := head(*d)
	if isJSONNull(p.Value) {
		delete(h, field)
	} else {
		h[field] = p.Value
	}
	root, etag, at := d.Root, d.Meta.ETag, d.Meta.GeneratedAt
	var next Document
	if err := reencode(h, &next); err != nil {
		return err
	}
	next.Root = root
	if field != "meta" || next.Meta.ETag == "" {
		next.Meta.ETag, next.Meta.GeneratedAt = etag, at
	}
	*d = next
	return nil
}

// isJSONNull reports whether v is nil or a typed nil pointer/map that
// encodes as JSON null.
func isJSONNull(v any) bool {
	if v == nil {
		return true
	}
	b, err := json.Marshal(v)
	return err == nil && bytes.Equal(b, []byte("null"))
}

// reencode copies v into dst through JSON so that Value may be either the
// typed Go value or a decoded map.
func reencode(v any, dst any) error {
	if isJSONNull(v) {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// parseNodePath splits "#id" or "#id/children/n".
func parseNodePath(p string) (id string, idx int, hasIdx bool, err error) {
	if !strings.HasPrefix(p, "#") || len(p) < 2 {
		return "", 0, false, fmt.Errorf("path %q must start with #<id>", p)
	}
	rest := p[1:]
	if i := strings.Index(rest, "/"); i >= 0 {
		id = rest[:i]
		tail := rest[i+1:]
		if !strings.HasPrefix(tail, "children/") {
			return "", 0, false, fmt.Errorf("path %q: expected /children/<n>", p)
		}
		n, convErr := strconv.Atoi(tail[len("children/"):])
		if convErr != nil || n < 0 {
			return "", 0, false, fmt.Errorf("path %q: bad child index", p)
		}
		return id, n, true, nil
	}
	return rest, 0, false, nil
}

func findNode(root *Node, id string) *Node {
	if root.ID == id {
		return root
	}
	for i := range root.Children {
		if n := findNode(&root.Children[i], id); n != nil {
			return n
		}
	}
	return nil
}

func findParent(root *Node, id string) (*Node, int) {
	for i := range root.Children {
		if root.Children[i].ID == id {
			return root, i
		}
		if p, j := findParent(&root.Children[i], id); p != nil {
			return p, j
		}
	}
	return nil, -1
}

func insertNode(list []Node, i int, n Node) []Node {
	list = append(list, Node{})
	copy(list[i+1:], list[i:])
	list[i] = n
	return list
}

func removeNode(list []Node, i int) []Node {
	out := append(list[:i:i], list[i+1:]...)
	if len(out) == 0 {
		return nil
	}
	return out
}

// Canonical returns the canonical JSON encoding of doc: object keys sorted,
// no insignificant whitespace, Meta.GeneratedAt and Meta.ETag omitted. Two
// documents with equal Canonical bytes are the same screen.
func Canonical(doc Document) ([]byte, error) {
	doc.Meta = stripVolatileMeta(doc.Meta)
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var generic any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(generic); err != nil { // encoding/json sorts map keys
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// ETag returns a weak entity tag for doc derived from Canonical:
// W/"sha256-<first 32 hex digits>". It is stable across renders that
// produce the same screen.
func ETag(doc Document) string {
	b, err := Canonical(doc)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return `W/"sha256-` + hex.EncodeToString(sum[:])[:32] + `"`
}

// jsonEqual compares two values by their JSON encoding.
func jsonEqual(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(ab, bb)
}
