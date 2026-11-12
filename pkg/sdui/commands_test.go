// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// fillCommand builds a command's body the way docs/SDUI.md tells a client to: the item,
// JSON-escaped, in place of {item}, which always stands inside a string.
func fillCommand(body, item string) string {
	q, _ := json.Marshal(item)
	return strings.ReplaceAll(body, `"{item}"`, string(q))
}

// Every command's body is JSON for every kind of item ID the server sends.
func TestCommandBodiesAreJSONForEveryItem(t *testing.T) {
	items := []string{"5", "show-5", "album-3-41", "collection-2", `odd"id\with`}
	for _, cmd := range Catalog().Commands {
		if cmd.Body == "" {
			continue
		}
		if strings.Count(cmd.Body, "{item}") != strings.Count(cmd.Body, `"{item}"`) {
			t.Errorf("%s: {item} must stand inside a JSON string in %s", cmd.Op, cmd.Body)
		}
		for _, it := range items {
			var v any
			if err := json.Unmarshal([]byte(fillCommand(cmd.Body, it)), &v); err != nil {
				t.Errorf("%s with %q: %v", cmd.Op, it, err)
			}
		}
	}
}

// A command's placeholders are {item} or one of its declared params; each stands inside a JSON string
// in the body, and filling them with a string, or with a number for an int param, leaves JSON.
func TestCommandParamsFillToJSON(t *testing.T) {
	re := regexp.MustCompile(`\{([a-z]+)\}`)
	for _, cmd := range Catalog().Commands {
		declared := map[string]CommandParam{}
		for _, p := range cmd.Params {
			if p.Type != "string" && p.Type != "int" && p.Type != "bool" && p.Type != "ints" {
				t.Errorf("%s: param %s has type %q", cmd.Op, p.Name, p.Type)
			}
			declared[p.Name] = p
		}
		for _, text := range []string{cmd.Path, cmd.Body} {
			for _, m := range re.FindAllStringSubmatch(text, -1) {
				if _, ok := declared[m[1]]; m[1] != "item" && !ok {
					t.Errorf("%s: {%s} is neither item nor a declared param", cmd.Op, m[1])
				}
			}
		}
		if cmd.Body == "" {
			continue
		}
		body := cmd.Body
		for name, p := range declared {
			switch p.Type {
			case "int":
				body = strings.ReplaceAll(body, `"{`+name+`}"`, "3")
			case "bool":
				body = strings.ReplaceAll(body, `"{`+name+`}"`, "true")
			case "ints":
				body = strings.ReplaceAll(body, `"{`+name+`}"`, "[1,2]")
			default:
				body = strings.ReplaceAll(body, `"{`+name+`}"`, `"a\"b"`)
			}
		}
		var v any
		if err := json.Unmarshal([]byte(fillCommand(body, "x")), &v); err != nil {
			t.Errorf("%s: %v in %s", cmd.Op, err, body)
		}
	}
}

// The WittyPlay Online commands are the ones docs/GAMING-API.md lists, each to the route it names.
func TestWittyPlayCommands(t *testing.T) {
	want := map[string]string{
		"wittyplay.room.create": "POST /api/play/{item}/room", "wittyplay.room.join": "POST /api/play/join",
		"wittyplay.room.invite": "POST /api/play/{item}/invite", "wittyplay.state.save": "POST /api/play/{item}/action",
		"wittyplay.state.load": "POST /api/play/{item}/action", "wittyplay.state.delete": "DELETE /api/games/{item}/saves/{slot}",
		"wittyplay.controller.assign": "PUT /api/play/{item}/seats/{user}",
		"rewind.open":                 " ", "controller.remap": " ",
	}
	for _, cmd := range Catalog().Commands {
		if w, ok := want[cmd.Op]; ok {
			if w != " " && cmd.Method+" "+cmd.Path != w {
				t.Errorf("%s: %s %s, want %s", cmd.Op, cmd.Method, cmd.Path, w)
			}
			delete(want, cmd.Op)
		}
	}
	for op := range want {
		t.Errorf("command %s is missing", op)
	}
}
