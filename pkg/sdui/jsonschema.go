// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

// JSONSchema returns docs/sdui.schema.json: a JSON Schema (2020-12) for
// everything the SDUI routes send and accept, written from the Go types so it
// can't drift from them, with the catalogue's vocabularies beside it. A test
// keeps the file in the repository equal to this.
//
// The schema is deliberately open: no object forbids extra properties and a
// node's type is any string. That is the forward-compatibility rule in
// machine-readable form: a client built today must accept a page from a newer
// server.
func JSONSchema() []byte {
	defs := map[string]any{}
	for _, v := range []any{Document{}, SectionItems{}, NavDocument{}, TopShelf{}, Page{}, Navigation{}, Personal{}, Patch{}, Catalogue{}, Preset{}, Issue{}, Item{}} {
		schemaOf(reflect.TypeOf(v), defs)
	}
	c := Catalog()
	var components, sources, styles []string
	for _, x := range c.Components {
		components = append(components, x.Type)
	}
	for _, x := range c.Sources {
		sources = append(sources, x.Type)
	}
	for _, x := range c.CardStyles {
		styles = append(styles, x.Style)
	}
	ref := func(n string) map[string]any { return map[string]any{"$ref": "#/$defs/" + n} }
	schema := map[string]any{
		"$schema":           "https://json-schema.org/draft/2020-12/schema",
		"$id":               "urn:wittymediaserver:sdui:schema:1",
		"title":             "WittyMediaServer server-driven UI, schema version 1, catalogue revision 2",
		"description":       "What the /api/v1/sdui routes answer and accept. Unknown properties must be ignored, and an unknown node type drawn as a plain column of its children. The component, card style and data source reference is GET /api/v1/sdui/catalog (the Catalogue definition here), which is also printed in docs/SDUI.md.",
		"x-schemaVersion":   SchemaVersion,
		"x-catalogRevision": CatalogRevision,
		"x-components":      components,
		"x-cardStyles":      styles,
		"x-sources":         sources,
		"x-icons":           c.Icons,
		"x-limits":          c.Limits,
		"oneOf":             []any{ref("Document"), ref("SectionItems"), ref("NavDocument"), ref("TopShelf"), ref("Page"), ref("Navigation"), ref("Personal"), ref("Catalogue")},
		"$defs":             defs,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(schema)
	return buf.Bytes()
}

var (
	timeType = reflect.TypeOf(time.Time{})
	rawType  = reflect.TypeOf(json.RawMessage{})
)

// schemaOf writes the schema of a Go type as JSON encodes it, putting named
// structs under defs and answering a reference to them.
func schemaOf(t reflect.Type, defs map[string]any) map[string]any {
	switch {
	case t == timeType:
		return map[string]any{"type": "string", "format": "date-time"}
	case t == rawType:
		return map[string]any{}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return schemaOf(t.Elem(), defs)
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64, reflect.Int32:
		return map[string]any{"type": "integer"}
	case reflect.Float64, reflect.Float32:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": schemaOf(t.Elem(), defs)}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaOf(t.Elem(), defs)}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Struct:
		name := t.Name()
		if _, done := defs[name]; !done {
			defs[name] = map[string]any{} // placeholder: types refer to themselves
			props := map[string]any{}
			var required []string
			for i := range t.NumField() {
				f := t.Field(i)
				if f.Anonymous {
					inner := schemaOf(f.Type, defs)
					if r, _ := inner["$ref"].(string); r != "" {
						if d, _ := defs[strings.TrimPrefix(r, "#/$defs/")].(map[string]any); d != nil {
							for k, v := range d["properties"].(map[string]any) {
								props[k] = v
							}
						}
					}
					continue
				}
				tag := f.Tag.Get("json")
				key, opts, _ := strings.Cut(tag, ",")
				if key == "-" || !f.IsExported() {
					continue
				}
				if key == "" {
					key = f.Name
				}
				props[key] = schemaOf(f.Type, defs)
				if !strings.Contains(opts, "omit") {
					required = append(required, key)
				}
			}
			d := map[string]any{"type": "object", "properties": props}
			if len(required) > 0 {
				d["required"] = required
			}
			defs[name] = d
		}
		return map[string]any{"$ref": "#/$defs/" + name}
	}
	return map[string]any{}
}
