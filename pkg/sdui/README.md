# sdui

WittyMediaServer's server-driven UI: the model pages are written in, the
catalogue that describes it, a validator that says how to fix what is wrong,
a renderer that fits a page to a profile and a device, an ID-keyed differ, the
built-in pages and presets, and a conformance kit for renderers. Pure Go,
standard library only, no HTTP.

```go
import "github.com/wittywilla/wittymediaserver/pkg/sdui"
```

The contract and the reference are [docs/SDUI.md](../../docs/SDUI.md).

## A page, rendered

```go
page := sdui.Page{ID: "night", Kind: "custom", Title: "Film night", Sections: []sdui.Section{
    {ID: "hello", Component: "Text", Props: map[string]any{"text": "{greeting}, {user}", "role": "pageTitle"}},
    {ID: "picks", Component: "Shelf", Props: map[string]any{"title": "This week"},
        Source: sdui.Source{"type": "collection", "collection": 12}},
}}
if issues := sdui.ValidatePage(&page); sdui.HasErrors(issues) {
    // each issue has a path, a message and a fix
}
doc, err := sdui.Render(page, &sdui.RenderContext{
    Profile:     sdui.ProfileFor(sdui.FormTV, "hd"),
    ProfileKind: "adult", UserName: "Sam",
    Resolver:    resolver, // answers the sources: see sdui.Resolver
})
```

`doc` is what a client is sent: a tree of nodes with every prop resolved,
every picture sized for the device, focus numbered, and an ETag.

## What is where

| File | What |
|---|---|
| `model.go` | Page, Section, Source, Condition, Navigation, Personal; Document, Node, Action, Focus; Item. |
| `catalog.go`, `catalog_data.go` | The catalogue: components, card styles, sources, conditions, page kinds, routes. |
| `profile.go` | Device profiles: sizes, budgets, text limits, picture widths, grid columns. |
| `validate.go` | ValidatePage, ValidateNavigation, ValidatePersonal: issues with fixes. |
| `render.go`, `cards.go`, `fallback.go`, `focus.go` | Render and RenderSection. |
| `nav.go` | RenderNavigation. |
| `stock.go`, `presets.go` | The built-in pages and navigation; the preset gallery. |
| `diff.go` | Diff, Apply, Canonical, ETag. |
| `migrate.go` | Conversion of screens from the first, unreleased, format. |
| `docgen.go`, `jsonschema.go` | The reference section of docs/SDUI.md and docs/sdui.schema.json. |
| `conformance/` | Fixtures and the reference normaliser renderers are tested against. |

## Changing the contract

Edit the catalogue or the renderer, then:

```sh
go test ./pkg/sdui -update                 # docs/SDUI.md's reference, docs/sdui.schema.json
go test ./pkg/sdui/conformance -update     # the fixtures
git diff pkg/sdui/conformance/fixtures     # read what changed for renderers
```

A new component gets a `Since` of the next `CatalogRevision` and, unless
every renderer must draw it, a fallback in `fallback.go`.
