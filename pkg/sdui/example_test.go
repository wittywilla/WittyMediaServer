// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui_test

import (
	"fmt"
	"time"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Example_builder assembles a small screen with the fluent builder and
// prints its JSON.
func Example_builder() {
	doc, err := sdui.NewDocument("home").
		Theme("default").
		Action("openLibrary", sdui.NavigateTo("/library")).
		Root(sdui.ScreenNode("home").
			Child(
				sdui.TextOf("greeting", "Welcome back").
					BindWith("text", sdui.Binding{Path: "$.user.name", Default: "Welcome back", Format: "title"}),
				sdui.ShelfOf("games", "Jump back into a game").
					Module("wittyplay").
					If(sdui.ModuleEnabled("wittyplay")),
				sdui.ButtonOf("browse", "Browse", "openLibrary"),
			)).
		Build()
	if err != nil {
		fmt.Println("invalid:", err)
		return
	}
	out, _ := sdui.MarshalIndent(doc)
	fmt.Println(string(out))
	// Output:
	// {
	//   "version": 1,
	//   "screen": "home",
	//   "theme": {
	//     "id": "default"
	//   },
	//   "root": {
	//     "type": "Screen",
	//     "id": "home",
	//     "children": [
	//       {
	//         "type": "Text",
	//         "id": "greeting",
	//         "props": {
	//           "text": "Welcome back"
	//         },
	//         "bindings": {
	//           "text": {
	//             "path": "$.user.name",
	//             "default": "Welcome back",
	//             "format": "title"
	//           }
	//         }
	//       },
	//       {
	//         "type": "Shelf",
	//         "id": "games",
	//         "props": {
	//           "title": "Jump back into a game"
	//         },
	//         "style": {
	//           "module": "wittyplay"
	//         },
	//         "visibility": {
	//           "module": "wittyplay"
	//         }
	//       },
	//       {
	//         "type": "Button",
	//         "id": "browse",
	//         "props": {
	//           "label": "Browse"
	//         },
	//         "action": "openLibrary"
	//       }
	//     ]
	//   },
	//   "actions": {
	//     "openLibrary": {
	//       "type": "navigate",
	//       "target": "/library"
	//     }
	//   }
	// }
}

// Example_render renders the same authored document for two different
// requesters: one with WittyPlay installed and one without.
func Example_render() {
	doc, _ := sdui.NewDocument("home").
		Root(sdui.ScreenNode("home").Child(
			sdui.TextOf("greeting", "Hello").
				BindWith("text", sdui.Binding{Path: "$.user.name", Default: "Hello", Format: "upper"}),
			sdui.ShelfOf("games", "Games").Module("wittyplay").If(sdui.ModuleEnabled("wittyplay")).
				Child(sdui.CardOf("game-1", sdui.Item{ID: "g1", Title: "Dragon Blaster"})),
		)).
		Build()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	gamer, _ := sdui.Render(doc, sdui.RenderContext{
		Modules:        []string{"wittyplay"},
		Data:           map[string]any{"user": map[string]any{"name": "willow"}},
		ThemeOverrides: map[string]map[string]string{"wittyplay": {"accent": "#3ad29f"}},
		Now:            now,
	})
	plain, _ := sdui.Render(doc, sdui.RenderContext{Now: now})

	fmt.Println("gamer greeting:", gamer.Root.Children[0].Props["text"])
	fmt.Println("gamer children:", len(gamer.Root.Children), "accent:", gamer.Root.Children[1].Style.Tokens["accent"])
	fmt.Println("plain greeting:", plain.Root.Children[0].Props["text"])
	fmt.Println("plain children:", len(plain.Root.Children))
	fmt.Println("bindings stripped:", gamer.Root.Children[0].Bindings == nil)
	fmt.Println("patches to go from plain to gamer:", len(sdui.Diff(plain, gamer)))
	// Output:
	// gamer greeting: WILLOW
	// gamer children: 2 accent: #3ad29f
	// plain greeting: HELLO
	// plain children: 1
	// bindings stripped: true
	// patches to go from plain to gamer: 2
}
