# Server-Driven UI (SDUI)

WittyMediaServer can describe a screen — the home page, a library, an item's
details, the player, settings — as data rather than as code baked into each
client. The server sends a tree of components; the web app, a TV app or a
phone app walks that tree and draws it with its own widgets. Re-ordering the
home screen, adding a shelf when a module is installed, or hiding a section
from guests becomes a server-side change every client picks up at once.

The Go library behind this is [`pkg/sdui`](../pkg/sdui/README.md)
(`github.com/wittywilla/wittymediaserver/pkg/sdui`). It is pure Go, standard
library only, and knows nothing about HTTP or storage. This document is the
reference for the schema, the rendering rules, the patch protocol, and what a
client must do.

## Overview and motivation

- **One screen definition, every device.** The same `home` document serves
  the browser at 1440 px, a 10-foot TV and a phone; viewport and platform
  conditions choose what each sees.
- **Modules shape the UI without client releases.** A games shelf appears only
  when `wittyplay` is enabled; an "On now" shelf only with `wittytv`. New
  modules add shelves by data.
- **Administrators can customise.** The stock screens are templates that seed
  a `ui_config.json`; an admin edits the document, and `Validate` keeps it
  sane.
- **Cheap updates.** A client holding a previous render sends its ETag; the
  server answers with a list of patches rather than the whole tree.

## Lifecycle

```
          author                     request                      client
 ┌─────────────────────┐    ┌──────────────────────────┐    ┌────────────────────┐
 │ Template / builder / │    │ RenderContext             │    │ Walk tree, draw    │
 │ hand-written JSON    │    │  platform, role, modules, │    │ each node with     │
 │                      │    │  features, flags, viewport│    │ native widgets     │
 │   Document (authored)│    │  locale, data, overrides  │    │                    │
 │   - Visibility       │    └────────────┬─────────────┘    │ On activate →      │
 │   - Bindings         │                 │                  │ dispatch Action    │
 └──────────┬───────────┘                 ▼                  └─────────▲──────────┘
            │  Validate          ┌─────────────────┐                   │
            └───────────────────▶│     Render      │───────────────────┘
                                 │ prune hidden    │   Document (rendered)
                                 │ resolve binds   │   - no Visibility/Bindings
                                 │ apply tokens    │   - Meta.GeneratedAt, ETag
                                 │ stamp meta      │
                                 └────────┬────────┘
                                          │  previous render (by ETag)
                                          ▼
                                 ┌─────────────────┐
                                 │      Diff       │────▶ []Patch ────▶ client Apply
                                 └─────────────────┘
```

1. **Author.** A `Document` is produced by a template
   (`HomeTemplate`, …), the fluent builder (`NewDocument`, `N`) or JSON. It
   may contain `visibility` conditions and `bindings`.
2. **Validate.** `Validate(&doc)` enforces every rule in
   [Validation rules](#validation-rules-and-limits). Run it when a document
   is stored, not on every request.
3. **Render.** `Render(doc, ctx)` produces the concrete tree for one
   requester. Hidden subtrees are removed, bindings become props, module
   token overrides land in `style.tokens`, `meta.generatedAt` and `meta.etag`
   are stamped, and `visibility`/`bindings` are stripped.
4. **Diff.** `Diff(old, new)` yields ID-keyed patches so a client can update
   in place; `Apply` is the reference implementation of applying them.

## Schema reference (version 1)

All JSON keys are camelCase. Fields marked *omit* are omitted when empty.

### Document

| Field | Type | Notes |
|---|---|---|
| `version` | int | Must be `1` (`SchemaVersion`). |
| `screen` | string | Screen name: `home`, `library`, `detail`, `player`, `settings`, or module-defined. Non-empty. |
| `theme` | ThemeRef, *omit* | Theme id and per-module token overrides. |
| `root` | Node | Must have `type: "Screen"`. |
| `actions` | map[string]Action, *omit* | Named actions referenced by `node.action`. Names match `[A-Za-z_][A-Za-z0-9_-]*`. |
| `meta` | Meta, *omit* | Non-structural metadata. |

### Node

| Field | Type | Notes |
|---|---|---|
| `type` | ComponentType | See the [component catalog](#component-catalog). |
| `id` | string | Unique in the document, 1–128 chars of `[A-Za-z0-9_.:-]`. Keys for Diff/Apply; keep stable. |
| `props` | map[string]any, *omit* | Component-specific. Must be JSON-serialisable. |
| `style` | Style, *omit* | Layout and colour hints. |
| `children` | []Node, *omit* | In display order. Only on components that allow children. |
| `visibility` | Condition, *omit* | Must evaluate true for the node and its subtree to be rendered. Stripped by Render. |
| `bindings` | map[string]Binding, *omit* | prop name → data path. Stripped by Render. |
| `action` | string, *omit* | Key into `document.actions`, dispatched when the node is activated. |

### Style

| Field | Type | Notes |
|---|---|---|
| `direction` | string | `row` or `column`. |
| `gap` | int | Space between children, CSS px. ≥ 0. |
| `padding`, `margin` | string | CSS shorthand (`"16px"`, `"8px 16px"`). |
| `align` | string | Cross axis: `start`, `center`, `end`, `stretch`. |
| `justify` | string | Main axis: `start`, `center`, `end`, `between`, `around`. |
| `width`, `height` | string | CSS lengths (`"100%"`, `"240px"`, `"42vh"`). |
| `columns` | int | Grid column count. ≥ 0. |
| `aspectRatio` | string | `"w/h"`: `2/3` poster, `16/9` wide, `1/1` album. |
| `background`, `foreground`, `accent`, `border` | colour | See [Colours](#colours). |
| `radius` | int | Corner radius px; 0 = theme default. ≥ 0. |
| `elevation` | int | Shadow depth; 0 = flat. ≥ 0. |
| `opacity` | float | 0..1; 0 = unset. |
| `hidden` | bool | Client-side hide; node stays in the tree (unlike `visibility`). |
| `module` | string | Module id (`wittyplay`, `wittytv`, …) scoping token overrides. Lower-case `[a-z][a-z0-9_-]*`. |
| `class` | string | Free-form client styling hook. |
| `tokens` | map[string]string | token → colour, filled by Render from module overrides. |

#### Colours

A colour is one of:

- `token:<name>` — a theme colour by the names in `themes/*/theme.json`
  (`bg`, `bg2`, `bg3`, `fg`, `fg2`, `accent`, `accentFg`, `accentText`,
  `border`, `danger`, `ok`, `warn`, `onImage`, `scrim`, …). The client maps
  this to its `--<name>` CSS variable or native palette entry.
- Hex: `#rgb`, `#rgba`, `#rrggbb`, `#rrggbbaa`.
- Functional: `rgb(...)`, `rgba(...)`, `hsl(...)`, `hsla(...)` with numeric
  arguments only.

Named CSS colours, `var()`, `url()` and anything else are **rejected** by
`ValidateColor`, because clients emit these values straight into CSS.

### Action

| Field | Type | Notes |
|---|---|---|
| `type` | ActionType | `navigate`, `play`, `openModal`, `closeModal`, `fetch`, `toggle`, `submit`, `external`, `back`. |
| `target` | string | Route, item id, node id or URL depending on type. Required except for `closeModal` and `back`. |
| `params` | map[string]any, *omit* | Extra parameters (e.g. `{"resume": true}`, `{"prop": "subtitles"}` for `toggle`). |

A `target` may contain `$itemId`-style placeholders that the client fills
from the activated node's props (`props.itemId`); this is a client
convention, not something Render expands.

### Condition

Every set field must hold (implicit AND). The zero condition is always true.

| Field | Type | True when… |
|---|---|---|
| `all` | []Condition | every element is true (vacuously true when empty). |
| `any` | []Condition | at least one element is true (false when empty). |
| `not` | Condition | the nested condition is false. |
| `feature` | string | `ctx.Features` contains the value. |
| `role` | string | `ctx.Role` equals the value. |
| `platform` | string | `ctx.Platform` equals the value (`web`, `tv`, `ios`, `android`, `desktop`). |
| `module` | string | `ctx.Modules` contains the value. |
| `minWidth`, `maxWidth` | int | viewport width is within the inclusive bound. Ignored when the viewport width is unknown (0). |
| `minHeight`, `maxHeight` | int | likewise for height. |
| `locale` | string | `ctx.Locale` equals the value or starts with `value-`/`value_` (case-insensitive; `en` matches `en-GB`, not `eo`). |
| `flag` | string | `ctx.Flags[value]` is true. |
| `when` | string | reserved for a future expression language; carried, currently always true. |

### Binding

| Field | Type | Notes |
|---|---|---|
| `path` | string | See [Binding paths](#binding-paths-and-format-verbs). |
| `default` | any, *omit* | Used when the path is missing or resolves to `null`. Formatted too. |
| `format` | string, *omit* | `upper`, `lower`, `title`, `duration`, `date:<Go layout>`. |

If a binding resolves to nothing and has no default, any static value already
in `props` is left in place as the fallback.

### ThemeRef

| Field | Type | Notes |
|---|---|---|
| `id` | string | Theme id (`default`, `midnight`, `oled`, …). Non-empty. |
| `moduleOverrides` | map[module]map[token]colour, *omit* | e.g. `{"wittyplay": {"accent": "#3ad29f"}}`. |

### Meta

| Field | Type | Notes |
|---|---|---|
| `title`, `description` | string | For window titles and previews. |
| `generatedAt` | RFC 3339 time | Stamped by Render. Excluded from `Canonical`/`ETag`. |
| `etag` | string | Stamped by Render; see [ETag](#etag-and-304-flow). Excluded from `Canonical`. |
| `source` | string | Provenance (`template:home`, `ui_config.json`). |
| `ttlSeconds` | int | Client cache hint. ≥ 0. |

## Component catalog

`Catalog()` returns this table programmatically; `Spec(type)` looks up one
entry. "Children: any" means any component except `Screen`.

| Type | Children | Required props | Optional props | Description |
|---|---|---|---|---|
| `Screen` | any | — | `title`, `scroll` | Root of a screen; exactly one, at the root. |
| `Stack` | any | — | — | Vertical or horizontal stack (`style.direction`). |
| `Grid` | any | — | — | Responsive grid; `style.columns` sets the count. |
| `Shelf` | any | `title` | `subtitle`, `seeAll`, `items`, `cardShape` | Titled, horizontally scrolling row (the web client's Shelf/CardRow). |
| `HeroBanner` | any | — | `title`, `subtitle`, `backdropUrl`, `logoUrl`, `items`, `interval` | Full-bleed featured banner. |
| `Card` | `Badge`, `ProgressBar`, `Text`, `Image` | — | `title`, `subtitle`, `imageUrl`, `kind`, `progress`, `badges`, `itemId`, `module` | A poster/tile for one item. |
| `PosterRow` | `Card` | `title` | `subtitle`, `seeAll`, `items`, `cardShape` | Titled row of poster cards. |
| `Text` | none | `text` | `variant`, `maxLines` | A run of text. `variant`: `pageTitle`, `heading`, `body`, `hint`. |
| `Image` | none | `src` | `alt`, `fit` | A picture. |
| `Button` | none | `label` | `icon`, `variant`, `disabled` | Dispatches its `action`. `variant`: `primary`, `ghost`, `danger`. |
| `Tabs` | `Tab` | — | `selected` | Tab strip. |
| `Tab` | any | `label` | `icon` | One tab and its content. |
| `List` | `ListItem` | — | `items`, `dense` | Vertical list. |
| `ListItem` | `Badge`, `Button`, `Text`, `Image`, `ProgressBar` | `title` | `subtitle`, `imageUrl`, `icon`, `trailing`, `itemId` | One row. |
| `Spacer` | none | — | `size` | Empty space. |
| `Divider` | none | — | `inset` | Hairline rule. |
| `Video` | none | `src` | `poster`, `autoplay`, `controls`, `startAt`, `itemId`, `title` | Video player. |
| `Badge` | none | `text` | `variant` | Small label (NEW, 4K, LIVE). |
| `ProgressBar` | none | `value` | `max`, `label` | 0..1 unless `max` is set. |
| `Carousel` | any | — | `interval`, `autoplay`, `indicators` | Paged, rotating container. |
| `SearchBar` | none | — | `placeholder`, `query`, `scope` | Search field; `action` fires on submit. |
| `Toolbar` | `Button`, `Text`, `Spacer`, `SearchBar`, `Badge`, `Image` | — | — | Horizontal bar of controls. |
| `Modal` | any | — | `title`, `dismissible`, `size` | Dialog opened by an `openModal` action targeting its id. |

A required prop may be supplied either statically in `props` or through a
`bindings` entry of the same name.

`items` on `Shelf`, `PosterRow`, `HeroBanner`, `Grid`, `List` and `Card`
containers is a data-driven alternative to explicit children: the server
binds `items` to an array of `Item` objects and the client renders a card per
item. Explicit children take precedence when both are present.

## Condition semantics

`Evaluate(c, ctx)` follows the table in [Condition](#condition) exactly:

- Fields on one condition are ANDed; `all` is AND, `any` is OR, `not` is
  negation. `all: []` is true, `any: []` is false.
- Unknown viewport (`ctx.Viewport.Width == 0`) makes width bounds pass;
  likewise height. A client should always send its viewport.
- `locale` is a prefix match on language subtags, case-insensitive.
- `when` is carried through validation and ignored. It is reserved so an
  expression language can be added without a schema bump.
- A hidden node's whole subtree is removed; children are never evaluated.
- A hidden root is a render error.

## Binding paths and format verbs

Grammar (`ParseBindingPath`):

```
path    := "$" segment*
segment := "." ident | "[" digits "]"
ident   := [A-Za-z_] [A-Za-z0-9_-]*
```

`$` alone selects the whole data object. Bracketed string keys (`$['a']`),
wildcards and filters are not supported. The walk (`Lookup`) understands
`map[string]any`, `[]any`, `[]string` and `[]map[string]any`; anything else
ends the walk as a miss.

Format verbs (`Binding.format`), applied to the resolved value *or* the
default:

| Verb | Effect |
|---|---|
| `upper` | upper-cases the string form. |
| `lower` | lower-cases the string form. |
| `title` | upper-cases the first letter of each word (`willow poortenga` → `Willow Poortenga`). |
| `duration` | treats the value as seconds and renders `m:ss` or `h:mm:ss` (`5400` → `1:30:00`). Non-numbers pass through as strings. |
| `date:<layout>` | parses RFC 3339, `2006-01-02` or a Unix-seconds number and formats with the Go layout. Unparsable values pass through. |

Unknown verbs are rejected by `Validate`; at render time they pass the value
through unchanged.

## Patch protocol

`Diff(old, new) []Patch` and `Apply(doc, patches)` implement an ID-keyed tree
diff. Nodes are matched by `id`; order within a parent is reconciled with
moves.

### Operations

| `op` | `path` | Payload | Meaning |
|---|---|---|---|
| `add` | `#<parentId>/children/<n>` | `node` | Insert `node` (with its subtree) at index `n`. |
| `remove` | `#<id>` | — | Remove the node and its subtree. |
| `replace` | `#<id>` | `node` | Replace every field of the node **except `children`** with those of `node`. Children are patched separately. |
| `replace` | `/root` | `node` | Replace the whole tree (root id changed). |
| `replace` | `/screen`, `/theme`, `/actions`, `/meta` | `value` | Set the document-level field. `null` clears `theme`/`actions`. |
| `move` | `#<parentId>/children/<n>` | `from: "#<id>"` | Move an existing child of the same parent to index `n`. |

Patches apply **in order**. Indices refer to the children array as it stands
after the preceding patches. `meta.generatedAt` and `meta.etag` never appear
in patches; the client takes the new ETag from the response header.

### Client application algorithm

```
for p in patches:
  if p.path starts with "/":        # document-level
      set document[field] = p.value (or p.node for /root)
  else:
      (id, index) = parse "#id" or "#id/children/n"
      switch p.op:
        add:     parent = find(id); parent.children.insert(index, p.node)
        remove:  parent = parentOf(id); parent.children.delete(indexOf(id))
        replace: n = find(id); keep = n.children; n = p.node; n.children = keep
        move:    parent = find(id); src = parentOf(p.from)
                 assert src is parent
                 child = src.children.remove(indexOf(p.from))
                 parent.children.insert(index, child)
```

`find` is a depth-first search by `id`. If any step fails (unknown id, bad
index), discard the local tree and request a full document.

### ETag and 304 flow

`Canonical(doc)` is the canonical JSON: keys sorted, no whitespace,
`meta.generatedAt` and `meta.etag` omitted. `ETag(doc)` is
`W/"sha256-<first 32 hex chars of SHA-256(Canonical)>"`. Two renders of the
same screen for equivalent contexts produce identical ETags regardless of
when they ran.

```
client ──GET /api/v1/sdui/screens/home  If-None-Match: W/"sha256-…"──▶ server
                                                      render for this requester
                                                      if ETag == If-None-Match → 304
                                                      else if server still has the old render
                                                        → 200 {patches:[…]}  ETag: new
                                                      else → 200 {document}  ETag: new
```

The exact routes are defined in [HTTP API](#http-api).

## Theme and module override semantics

Colours in `style` are usually `token:<name>` references, resolved by the
client against its active theme (the names match `themes/*/theme.json`
`colors` and the `--<name>` CSS variables in `docs/THEMES.md`).

A module can recolour the parts of the UI that belong to it. Overrides come
from two places and are merged, later winning:

1. `document.theme.moduleOverrides[module][token]` — authored with the
   document (`DocumentBuilder.ModuleOverride`).
2. `RenderContext.ThemeOverrides[module][token]` — supplied per request
   (e.g. from the installed module's manifest).

During `Render`, every node whose `style.module` equals `module` receives the
merged map in `style.tokens`, merged over any tokens already on the node.
Nodes without `style.module` are untouched. The client applies `style.tokens`
as scoped variables on that subtree (`--accent: #3ad29f` on the games shelf),
so `token:accent` inside it resolves to the module colour.

Token names match `[A-Za-z_][A-Za-z0-9_-]*`; colours must pass
`ValidateColor`.

## Validation rules and limits

`Validate(&doc) error` returns `nil` or a `*ValidationError` whose `Issues`
list **every** problem with a JSON-pointer-like path
(`/root/children/2/props/title`).

| Rule | Limit / detail |
|---|---|
| `version` | must equal `1`. |
| `screen` | non-blank. |
| `root.type` | must be `Screen`; `Screen` may not appear elsewhere. |
| Node ids | non-empty, ≤ 128 chars, charset `[A-Za-z0-9_.:-]`, unique per document. |
| Component types | must be in the catalog. |
| Children | only on components that allow them; restricted types per catalog. |
| Required props | present in `props` or `bindings`. |
| Depth | ≤ 32 (root is depth 1). |
| Node count | ≤ 5000. |
| `props`, `actions[].params`, `bindings[].default` | JSON-serialisable. |
| `node.action` | must name an entry in `actions`. |
| Action names | `[A-Za-z_][A-Za-z0-9_-]*`; types known; `target` required except `closeModal`/`back`. |
| Style colours | `ValidateColor`; `direction` ∈ {row, column}; `gap`, `columns`, `radius`, `elevation` ≥ 0; `opacity` ∈ [0, 1]; `aspectRatio` like `16/9`; `module` lower-case id; `tokens` valid names and colours. |
| Bindings | prop name valid ident; `path` parses; `format` known. |
| Conditions | bounds ≥ 0; `minWidth ≤ maxWidth`, `minHeight ≤ maxHeight` when both set; `module` valid id; nesting ≤ 32. |
| Theme | `id` non-blank; module ids, token names and colours valid. |
| `meta.ttlSeconds` | ≥ 0. |

`Unmarshal` additionally rejects input over **2 MiB**, unknown JSON fields,
trailing data and a wrong `version` — before `Validate` runs.

## Template catalog

| Function | Screen | Data | Highlights |
|---|---|---|---|
| `HomeTemplate(HomeData)` | `home` | user name, continue watching, recently added, recommended, extra shelves | Greeting bound to `$.user.name` (title-cased); `home-continue` gated on feature `history`; `home-recommended` on `recommendations`; `home-games` on module `wittyplay`; `home-livetv` on `wittytv`; `home-music` on `wittymusic`; search bar only at ≥ 720 px; "Finish server setup" for admins with flag `setupIncomplete`. |
| `LibraryTemplate(LibraryData)` | `library` | library id, title, kind, items | Three grids (8/5/3 columns) gated on viewport width; title bound to `$.library.title`; empty-state text on flag `libraryEmpty`. |
| `DetailTemplate(DetailData)` | `detail` | item, overview, year, duration, rating, genres, related, children | Detail hero with duration formatted via `duration`; Play on feature `playback`; Resume on flag `hasProgress`; Artwork modal for admins; related row on `recommendations`. |
| `PlayerTemplate(PlayerData)` | `player` | item, stream URL, poster, start offset, next item | Video bound under `$.player.*`; Back hidden on `tv`; subtitles toggle on flag `hasSubtitles`; up-next card on feature `autoplayNext`. |
| `SettingsTemplate(SettingsData)` | `settings` | sections, version | One `Tab` per section; admin sections on role `admin`; module rows on `ModuleEnabled`; sign-out hidden from `guest`; About modal with version bound to `$.server.version`. |
| `DefaultScreens()` | all five | empty | `map[string]Document` keyed by screen name, used to seed `ui_config.json`. |

Every template panics if it would produce an invalid document, and the test
suite asserts each one validates, survives a strict JSON round trip, and
renders for a bare guest and a loaded admin.

### Example: rendered `home`

Rendered for an admin on the web at 1440×900 with `wittyplay` and `wittytv`
enabled, features `history` and `recommendations`, user `willow`, and a
`wittyplay` accent override. Abridged; the full file is the golden
[`pkg/sdui/testdata/home.golden.json`](../pkg/sdui/testdata/home.golden.json).

```json
{
  "version": 1,
  "screen": "home",
  "theme": { "id": "default" },
  "root": {
    "type": "Screen", "id": "home",
    "props": { "scroll": true, "title": "Home" },
    "style": { "direction": "column", "gap": 32, "background": "token:bg" },
    "children": [
      { "type": "Toolbar", "id": "home-toolbar", "children": [
        { "type": "Text", "id": "home-greeting", "props": { "text": "Willow", "variant": "pageTitle" } },
        { "type": "SearchBar", "id": "home-search", "props": { "placeholder": "Search your libraries" }, "action": "goSearch" },
        { "type": "Button", "id": "home-settings-btn", "props": { "icon": "settings", "label": "Settings", "variant": "ghost" }, "action": "goSettings" }
      ]},
      { "type": "HeroBanner", "id": "home-hero",
        "props": { "items": [ { "itemId": "m1", "title": "Metropolis" } ], "title": "Featured" },
        "style": { "height": "42vh", "aspectRatio": "16/9" } },
      { "type": "PosterRow", "id": "home-continue",
        "props": { "cardShape": "wide", "title": "Continue watching" },
        "children": [
          { "type": "Card", "id": "home-continue-card-0",
            "props": { "badges": ["HD"], "imageUrl": "/img/m1.jpg", "itemId": "m1", "kind": "movie", "progress": 0.4, "subtitle": "1927", "title": "Metropolis" },
            "action": "openItem",
            "children": [ { "type": "ProgressBar", "id": "home-continue-card-0-progress", "props": { "value": 0.4 } } ] }
        ]},
      { "type": "Shelf", "id": "home-games",
        "props": { "items": [ { "title": "Dragon Blaster" } ], "title": "Jump back into a game" },
        "style": { "module": "wittyplay", "tokens": { "accent": "#3ad29f" } } },
      { "type": "Shelf", "id": "home-livetv", "props": { "title": "On now" }, "style": { "module": "wittytv" } }
    ]
  },
  "actions": {
    "goSearch": { "type": "navigate", "target": "/search" },
    "goSettings": { "type": "navigate", "target": "/settings" },
    "openItem": { "type": "navigate", "target": "/item" }
  },
  "meta": { "title": "Home", "generatedAt": "2026-01-01T00:00:00Z", "etag": "W/\"sha256-…\"", "source": "template:home", "ttlSeconds": 60 }
}
```

Note what is absent: `home-music` (module not enabled), `home-admin-setup`
(flag unset), every `visibility` and `bindings` key.

## Versioning and compatibility policy

- `version` is bumped only for incompatible changes: renaming or removing a
  field, or changing the meaning of one.
- Adding a component type, a `style` field, a condition predicate, an action
  type or a format verb is **compatible** and does not bump the version.
- Clients must tolerate unknown component types (render children, ignore the
  type), unknown props, unknown style fields and unknown action types (ignore
  the action).
- `Unmarshal` rejects any version other than the one it was built for.
  Servers store authored documents and re-validate on load so a bump is
  caught at startup.
- The Go API follows the module's semver; `-alpha` releases may change it.

## Writing a client renderer

A renderer for any platform follows the same shape:

1. **Fetch and decode** the document. Reject `version != 1`. Keep the
   `ETag` header for the next request.
2. **Walk the tree** depth-first from `root`. For each node, pick a native
   widget by `type` using the [component catalog](#component-catalog). Pass
   `props` through; apply `style`:
   - layout fields map to your flex/stack primitives;
   - colours starting `token:` resolve to the current theme's variable or
     palette entry; hex/functional colours apply directly;
   - `tokens` become scoped variable overrides for that subtree;
   - `hidden` hides without unmounting; `class` is an opaque hook.
3. **Unknown type fallback.** If `type` is not one you know, render a plain
   container and recurse into `children`. Never fail the screen.
4. **Items.** When a container has `props.items` and no `children`, build a
   card per item (`id`, `title`, `subtitle`, `imageUrl`, `kind`, `module`,
   `progress`, `badges`). Explicit children win.
5. **Action dispatch.** When a node with `action` is activated, look up
   `document.actions[action]` and run it: `navigate` → your router,
   `play` → your player, `openModal`/`closeModal` → show/hide the `Modal`
   node with that id, `fetch` → request the target and replace/patch the
   screen, `toggle` → flip `props[params.prop]` on the target node,
   `submit` → POST field values, `external` → system browser, `back` → pop.
   Substitute `$itemId` in `target` from the node's `props.itemId`. Ignore
   unknown action types.
6. **Keyboard/remote focus.** Treat `Shelf`, `PosterRow`, `Toolbar` and
   `Tabs` as horizontal focus rows and `List`/`Stack(column)` as vertical, as
   described in `docs/DESIGN.md`.
7. **Updates.** Send `If-None-Match` with your ETag. On 304, keep the tree.
   On a patch response, apply with the algorithm above; on any failure,
   re-fetch the full document. Respect `meta.ttlSeconds` for polling.

## HTTP API

The server stores the authored screens in `ui_config.json` and renders them
per request. Every route is under `/api/v1/sdui` (and `/api/sdui` for the web
interface); `sdui` and `sdui-diff` in `/capabilities` `features` say a server
has them. Route-by-route reference: [CLIENT-API.md](CLIENT-API.md#server-driven-ui)
and `docs/openapi.json`.

| Route | Sign-in | What it does |
|---|---|---|
| `GET /sdui/screens/{screen}` | any account | The screen rendered for this account and device. `ETag`; 304 with `If-None-Match`. |
| `GET /sdui/screens/{screen}/diff?since=<etag>` | any account | Patches from the render the client has, or the whole document. |
| `GET /sdui/config` | admin | All of `ui_config.json`, with `etag`. |
| `PUT /sdui/config` | admin | Replace it; `If-Match` required. |
| `GET /sdui/config/history` | admin | The last 10 revisions. |
| `POST /sdui/config/rollback/{rev}` | admin | Make a kept revision current again. |

### How the request becomes a `RenderContext`

The client says what it is; the server says who is asking. For each client
field the query parameter wins over the header.

| `RenderContext` | From | Notes |
|---|---|---|
| `Platform` | `platform` / `X-WMS-Platform`, else derived from `formFactor` | `web` (default), `tv`, `ios`, `android`, `desktop`. Form factors map onto them: `phone`/`tablet` → `ios`, `tenfoot`/`10ft` → `tv`, `browser` → `web`. Another short lower-case name is accepted and simply matches no condition. |
| `Flags["formFactor:<ff>"]` | `formFactor` / `X-WMS-Form-Factor` | So a condition can say `{"flag": "formFactor:tablet"}`. |
| `Viewport` | `viewport=1440x900` / `X-WMS-Viewport`, or `width` and `height` | CSS pixels. Send it: width conditions pass when it is unknown. |
| `Locale` | `locale` / `X-WMS-Locale`, else the first `Accept-Language` | A language tag (`en-GB`). |
| `Flags[<capability>]` | `capabilities=hdr,touch` / `X-WMS-Capabilities` | Up to 64 names of letters, digits and `. _ : -`; each becomes a flag set to true. |
| `Role` | the account | `admin` for administrators, else the account's kind: `adult`, `kids`, `guest`. |
| `Features` | the server | `/capabilities` `features`, plus `history` and `playback`. |
| `Modules` | the server | The modules the screen's conditions name that are installed and on. |
| `Flags["setupIncomplete"]` | the server | For administrators while first-time setup is unfinished. |
| `Data` | the server | `$.user.name`, `$.user.kind`, `$.user.isAdmin`, `$.server.name`, `$.server.version`. Library data (continue watching, items) is not bound yet; templates fall back to their static props. |
| `ThemeOverrides` | `ui_config.json` `theme.modules` | The per-module colours of the [theme engine](THEMES.md#the-theme-engine), so a node with `style.module` gets them in `style.tokens`. |

An invalid value is a 400 naming the parameter. An unknown screen is 404; a
screen whose root is hidden for this requester is 422.

### Rendering, ETags and 304

`meta.etag` and the `ETag` header are `sdui.ETag` of the rendered document,
which is the same for every request that renders the same tree. Send it back as
`If-None-Match` for a 304. Responses carry `Cache-Control: private, no-cache`.

### Diffs

`GET /sdui/screens/{screen}/diff?since=<etag>` (or with `If-None-Match`), with
the same device parameters as the screen, renders the screen and then:

- answers **304** when `since` is the current ETag;
- answers `{"etag", "since", "patches": [...]}` when the server still has the
  render `since` names, kept for this account (the last 256 renders across all
  accounts, in memory, gone at restart). Apply the patches with the
  [client algorithm](#client-application-algorithm);
- answers `{"etag", "document": {...}}` otherwise: replace the local tree.

`patches` may be empty when only `meta` volatile fields differ.

### `ui_config.json`

The file is in the data folder. It is created from `DefaultScreens()` on first
start. Its layout:

```json
{
  "version": 1,
  "revision": 7,
  "updatedAt": "2026-11-10T14:00:00Z",
  "updatedBy": "owner",
  "theme": {
    "tokens":  { "accent": "#FF8800" },
    "modules": { "wittyplay": { "accent": "#3AD29F" } }
  },
  "screens": {
    "home": { "version": 1, "screen": "home", "root": { "type": "Screen", "id": "home" } }
  }
}
```

- **Validation.** Strict JSON (unknown fields and trailing data are refused,
  16 MiB at most, 64 screens at most). Each key under `screens` must equal its
  document's `screen` and be a lower-case name. Every document goes through
  `Unmarshal` and `Validate`; `theme` token names must be colour tokens themes
  have and values colours. A refused save answers 400 with every problem in
  `issues` (`{path, message}`, paths prefixed `/screens/<name>`).
- **Revisions and concurrency.** Every save is a new `revision`. `GET
  /sdui/config` gives the configuration's `etag` (`W/"ui-<revision>-<hash>"`);
  `PUT` needs it as `If-Match` (428 without; 412 when someone saved in
  between: read it again and redo the change). Writes are atomic (temporary
  file, sync, rename) and serialised.
- **History.** A copy of each of the last 10 revisions is kept in
  `ui_config.history/<revision>.json`. `POST /sdui/config/rollback/{rev}` makes
  one current again as a new revision (the history isn't rewritten).
- **Edited by hand.** Changing the file while the server runs is fine: on the
  next read the server sees the new modification time, validates the file and
  saves it as a new revision (`updatedBy: "file on disk"`).
- **Recovery.** A file that can't be read or doesn't validate (at start or
  after a hand edit) is kept as `ui_config.json.bad`, a warning is logged, and
  the newest valid revision in the history is used and written back. Without
  one, the stock screens are.

---

<sub>WittyMediaServer documentation</sub>
