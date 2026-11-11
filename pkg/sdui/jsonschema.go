// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"encoding/json"
)

// JSONSchema returns the JSON Schema (draft 2020-12) of what a client is
// sent: a rendered Document, a diff, and Top Shelf content. docs/sdui.schema.json
// is this, and a test keeps the two the same.
//
// The schema is deliberately open: no object forbids extra properties, and a
// node's type is any string, with the known ones listed beside it. That is
// the forward-compatibility rule in machine-readable form. A client built
// today must accept a screen from a newer server, drawing an unknown type as
// a plain container of its children and ignoring props it doesn't know.
func JSONSchema() []byte {
	str := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer"}
	boolean := map[string]any{"type": "boolean"}
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	obj := func(desc string, required []string, props map[string]any) map[string]any {
		o := map[string]any{"type": "object", "description": desc, "properties": props}
		if len(required) > 0 {
			o["required"] = required
		}
		return o
	}
	strMap := map[string]any{"type": "object", "additionalProperties": str}

	var types, actions []string
	components := map[string]any{}
	for _, c := range catalog {
		types = append(types, string(c.Type))
		components[string(c.Type)] = map[string]any{
			"description": c.Description, "allowsChildren": c.AllowsChildren, "allowedChildren": c.AllowedChildren,
			"requiredProps": c.RequiredProps, "optionalProps": c.OptionalProps, "since": c.Since, "fallback": c.Fallback,
		}
	}
	for _, a := range []ActionType{Navigate, Play, OpenModal, CloseModal, Fetch, Toggle, Submit, External, Back} {
		actions = append(actions, string(a))
	}
	imageSize := obj("A picture size in pixels.", []string{"width", "height", "aspect"}, map[string]any{"width": integer, "height": integer, "aspect": str})
	insets := obj("Distances from each edge, in points.", nil, map[string]any{"top": integer, "right": integer, "bottom": integer, "left": integer})

	schema := map[string]any{
		"$schema":           "https://json-schema.org/draft/2020-12/schema",
		"$id":               "urn:wittymediaserver:sdui:schema:1",
		"title":             "WittyMediaServer server-driven UI, schema version 1",
		"description":       "What GET /api/v1/sdui/screens/{screen}, its /diff and /api/v1/sdui/topshelf answer. Unknown properties must be ignored, and an unknown node type drawn as a plain container of its children.",
		"x-schemaVersion":   SchemaVersion,
		"x-catalogRevision": CatalogRevision,
		"x-components":      components,
		"x-limits": map[string]any{"maxDocumentBytes": MaxDocumentBytes, "maxDepth": MaxDepth, "maxNodes": MaxNodes, "maxIdLength": MaxIDLength,
			"maxChildren": MaxChildren, "maxProps": MaxProps, "maxPropBytes": MaxPropBytes, "maxActions": MaxActions,
			"maxTopShelfSections": MaxTopShelfSections, "maxTopShelfItems": MaxTopShelfItems},
		"oneOf": []any{ref("Document"), ref("Diff"), ref("TopShelf")},
		"$defs": map[string]any{
			"Document": obj("One rendered screen.", []string{"version", "screen", "root"}, map[string]any{
				"version": map[string]any{"type": "integer", "const": SchemaVersion},
				"screen":  str,
				"theme":   obj("The theme, and colours for parts that belong to a module.", []string{"id"}, map[string]any{"id": str, "moduleOverrides": map[string]any{"type": "object", "additionalProperties": strMap}}),
				"root":    ref("Node"),
				"actions": map[string]any{"type": "object", "additionalProperties": ref("Action")},
				"layout":  ref("Layout"),
				"meta": obj("About the document.", nil, map[string]any{"title": str, "description": str, "generatedAt": map[string]any{"type": "string", "format": "date-time"},
					"etag": str, "source": str, "ttlSeconds": integer}),
			}),
			"Node": obj("One component. Draw an unknown type as a plain container of its children.", []string{"type", "id"}, map[string]any{
				"type":     map[string]any{"type": "string", "description": "A component type. Known in this revision: see x-knownTypes. Others must be drawn as a plain container.", "x-knownTypes": types},
				"id":       map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_.:-]+$", "maxLength": MaxIDLength},
				"props":    map[string]any{"type": "object"},
				"style":    ref("Style"),
				"children": map[string]any{"type": "array", "items": ref("Node")},
				"action":   map[string]any{"type": "string", "description": "A key of the document's actions."},
				"focus":    ref("Focus"),
			}),
			"Focus": obj("The node's place in the focus engine.", nil, map[string]any{
				"focusable": boolean, "order": map[string]any{"type": "integer", "minimum": 1}, "default": boolean, "group": str,
				"up": str, "down": str, "left": str, "right": str,
			}),
			"Style": obj("Layout and colour hints. Colours are #hex, rgb()/hsl(), or token:<name>.", nil, map[string]any{
				"direction": map[string]any{"enum": []string{"row", "column"}}, "gap": integer, "padding": str, "margin": str, "align": str, "justify": str,
				"width": str, "height": str, "columns": integer, "aspectRatio": str, "background": str, "foreground": str, "accent": str, "border": str,
				"radius": integer, "elevation": integer, "opacity": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "hidden": boolean,
				"module": str, "class": str, "tokens": strMap,
			}),
			"Action": obj("What activating a node does. \"$name\" in target stands for the activated node's prop of that name.", []string{"type"}, map[string]any{
				"type": map[string]any{"type": "string", "description": "Known: see x-knownTypes. Ignore a node's action when its type is unknown.", "x-knownTypes": actions}, "target": str, "params": map[string]any{"type": "object"},
			}),
			"Layout": obj("The device profile the screen was fitted to.", []string{"formFactor", "scale", "safeArea", "scroll", "focusEngine", "maxSections", "maxItems", "maxGridItems", "externalLinks"}, map[string]any{
				"formFactor": map[string]any{"type": "string", "x-knownValues": []string{FormTV, FormPhone, FormTablet, FormDesktop, FormWeb}}, "class": str,
				"canvas": obj("The screen in points.", nil, map[string]any{"width": integer, "height": integer}), "scale": integer, "safeArea": insets,
				"scroll": str, "focusEngine": boolean, "text": map[string]any{"type": "object", "additionalProperties": integer},
				"images": map[string]any{"type": "object", "additionalProperties": imageSize}, "gridColumns": integer,
				"maxSections": integer, "maxItems": integer, "maxGridItems": integer, "externalLinks": boolean, "truncated": boolean,
			}),
			"Patch": obj("One step from the screen a client has to the current one.", []string{"op", "path"}, map[string]any{
				"op": map[string]any{"enum": []string{string(PatchAdd), string(PatchRemove), string(PatchReplace), string(PatchMove)}}, "path": str, "node": ref("Node"), "value": map[string]any{}, "from": str,
			}),
			"Diff": obj("The answer of /diff: patches, or the whole document when the server no longer has the render named by since.", []string{"etag"}, map[string]any{
				"etag": str, "since": str, "patches": map[string]any{"type": "array", "items": ref("Patch")}, "document": ref("Document"),
			}),
			"ImageRef": obj("A picture and the size to show it at, in pixels.", []string{"url", "width", "height", "aspect"}, map[string]any{"url": str, "width": integer, "height": integer, "aspect": str}),
			"TopShelf": obj("Content for a tvOS Top Shelf extension.", []string{"version", "style", "sections", "expiresInSec"}, map[string]any{
				"version": integer, "style": map[string]any{"const": "sectioned"}, "expiresInSec": integer,
				"sections": map[string]any{"type": "array", "maxItems": MaxTopShelfSections, "items": obj("One titled row.", []string{"title", "items"}, map[string]any{
					"title": str,
					"items": map[string]any{"type": "array", "maxItems": MaxTopShelfItems, "items": obj("One picture.", []string{"id", "title", "shape", "displayRoute"}, map[string]any{
						"id": str, "title": str, "shape": map[string]any{"enum": []string{"poster", "wide", "square"}}, "image": ref("ImageRef"),
						"progress": map[string]any{"type": "number"}, "displayRoute": str, "playRoute": str,
					})},
				})},
			}),
		},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(schema) // map keys sorted: the same bytes every time
	return buf.Bytes()
}
