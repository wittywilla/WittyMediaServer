# sdui

Server-Driven UI core for WittyMediaServer: a JSON schema for screens, a
validator, a per-requester renderer, an ID-keyed tree differ and templates
for the stock screens. Pure Go, standard library only, no HTTP.

```go
import "github.com/wittywilla/wittymediaserver/pkg/sdui"
```

## Quick example

```go
// Author once.
doc, err := sdui.NewDocument("home").
    Theme("default").
    Action("openLibrary", sdui.NavigateTo("/library")).
    Root(sdui.ScreenNode("home").Child(
        sdui.TextOf("greeting", "Welcome back").
            BindWith("text", sdui.Binding{Path: "$.user.name", Default: "Welcome back", Format: "title"}),
        sdui.ShelfOf("games", "Jump back into a game").
            Module("wittyplay").
            If(sdui.ModuleEnabled("wittyplay")),
        sdui.ButtonOf("browse", "Browse", "openLibrary"),
    )).
    Build() // runs Validate

// Render for one requester.
screen, err := sdui.Render(doc, sdui.RenderContext{
    Platform: "web", Role: "user",
    Modules:  []string{"wittyplay"},
    Viewport: sdui.Viewport{Width: 1440, Height: 900},
    Data:     map[string]any{"user": map[string]any{"name": "willow"}},
})
// screen.Root has no Visibility/Bindings; screen.Meta.ETag is set.

// Incremental update for a client holding the previous render.
patches := sdui.Diff(previous, screen)
```

Stock screens: `sdui.HomeTemplate`, `LibraryTemplate`, `DetailTemplate`,
`PlayerTemplate`, `SettingsTemplate`, and `DefaultScreens()` for all five
with empty data.

Full schema, condition/binding semantics, the patch protocol and a guide to
writing a client renderer: [docs/SDUI.md](../../docs/SDUI.md).

## Tests

```sh
go test -cover ./pkg/sdui/...
go test ./pkg/sdui/ -run TestHomeTemplate_Golden -update   # regenerate testdata/home.golden.json
go test ./pkg/sdui/ -run '^$' -fuzz FuzzUnmarshal -fuzztime 30s
```
