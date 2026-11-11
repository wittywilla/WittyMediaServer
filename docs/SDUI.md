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
client must do. The same schema as a file for tools is
[`sdui.schema.json`](sdui.schema.json) (JSON Schema 2020-12), made from the
code and kept current by a test.

## Designed for tvOS first

An Apple TV is the strictest device a screen has to work on, so the schema is
built around its rules and every other device gets them for free.

| tvOS rule | What the schema does about it |
|---|---|
| There is no pointer. Everything a person can act on must take focus, and focus must start somewhere. | Every rendered node a person can act on has `focus.focusable`, a `focus.order` in reading order, and a `focus.group`; exactly one has `focus.default`. See [Focus](#focus). |
| Focus moves by direction, and things that can be focused can't be inside each other. | The validator refuses a pressable node inside another, and a focus guide that leads nowhere. |
| A screen scrolls one way and its rows scroll sideways. Nothing scrolls both ways. | The validator refuses a scrolling region inside another within one page (a `Shelf` in a `Shelf`, a `Shelf` in a `Grid`). `layout.scroll` is always `vertical`. |
| The edge of a television picture can't be relied on. | `layout.safeArea`: 60 points top and bottom, 80 at the sides, on the 1920×1080 canvas. |
| It is read from the sofa. | `layout.text`: tvOS's own sizes in points (page title 76, heading 57, title 48, body 29, caption 25). |
| An Apple TV HD has little memory. | A screen is cut to `layout.maxSections` rows of `layout.maxItems` cards (10 rows of 16 on an HD, 20 of 30 on a 4K), and `layout.truncated` says when rows were left out. |
| A 4K poster in a 250-point card wastes all of it. | Every card's `props.image` is the picture's address with the width to ask for, and its size and aspect: 250×375 for a poster on an HD, twice that on a 4K. |
| There is no web view and no browser. | There is no web-view component, and nodes whose action is `external` are left out for a TV. |
| The Top Shelf is not a screen. | It has its own small document: [Top Shelf](#top-shelf). |

## Device profiles

One authored screen is rendered for five form factors. The request says which
(`formFactor`, or it follows from `platform`), and a TV says its class with
`deviceClass`. The profile used comes back as `layout`, so the client lays out
with the same numbers the server cut the screen to.

| Form factor | Scale | Safe area | Grid columns | Rows | Cards a row | Cards a grid | Poster asked for | Opens web links |
|---|---|---|---|---|---|---|---|---|
| `tv`, class `hd` (the default) | 1 | 60/80/60/80 | 6 | 10 | 16 | 60 | 250×375 | no |
| `tv`, class `4k` | 2 | 60/80/60/80 | 6 | 20 | 30 | 120 | 500×750 | no |
| `phone` | 3 | the system's | 3 | 16 | 20 | 60 | 360×540 | yes |
| `tablet` | 2 | the system's | 5 | 20 | 30 | 100 | 320×480 | yes |
| `desktop`, `web` | 2 | none | the window's | 30 | 40 | 200 | 360×540 | yes |

`layout.images` lists the picture size for every card shape (`poster` 2/3,
`wide` 16/9, `square` 1/1, `hero`), in pixels. A row's `cardShape` prop, or a
card's own `shape`, says which applies. A condition can test the form factor:
`{"formFactor": "tv"}`.

A TV that doesn't say its class gets the HD profile: it is safe on both.

## Focus

`node.focus` is the node's place in the focus engine.

| Field | Set by | Meaning |
|---|---|---|
| `focusable` | Render | True on every node a person can act on: a `Button`, `Card`, `ListItem`, `Tab`, `SearchBar`, or anything with an `action`. A disabled button is not. |
| `order` | Render | Its place in reading order, from 1, with no gaps. The same screen always gives the same numbers. |
| `group` | Render | The ID of the row or bar it is in (`Shelf`, `PosterRow`, `Carousel`, `Grid`, `List`, `Tabs`, `Toolbar`, `HeroBanner`). Treat a group as one focus section, and remember the last focused node in it. |
| `default` | author, else Render | Focus starts here. An author may mark one node; when they didn't, or that node isn't on the screen for this account, Render marks the first node that isn't in a `Toolbar`, else the first of all. |
| `up`, `down`, `left`, `right` | author | Focus guides: the ID of the node focus moves to in that direction, in place of the nearest. A guide to a node that isn't on the rendered screen is dropped, so focus never goes nowhere. |

On tvOS, map `default` to `preferredFocusEnvironments`, a group to a
`UIFocusGuide`-bounded section or a collection view that remembers its last
focused index path, and the four guides to `UIFocusGuide`s. Elsewhere the same
fields give keyboard and remote order.

## Items and sections

A row is usually filled from data. When a `Shelf`, `PosterRow`, `Carousel`,
`Grid`, `List` or `HeroBanner` has an `items` prop (bound, or written out),
Render turns each item into a child node: a `Card`, or a `ListItem` in a
`List`. So every item is a node, with an ID to diff by and a place in the focus
order, and it counts toward the device's limits.

- The child's ID is `<row id>:<item id>`.
- Its props are the item's `title`, `subtitle`, `imageUrl`, `kind`,
  `progress`, `badges`, `module`, `route`, `shape`, and `itemId` (the item's
  `id`).
- Its action is the row's `itemAction` prop, or `openItem` when the document
  defines that.
- A row cut to the device's limit gets `more: true` and `total`. A
  `HeroBanner` takes at most 5.
- A row that ends up with nothing in it is left out. An empty "Continue
  watching" is never sent.

A `Stack` with a `sections` prop (a list of `{id, title, subtitle, cardShape,
seeAll, items}`) becomes one `Shelf` per section, each then filled the same
way. This is how Home has a row per library and Search a row per kind of
result.

In an action's `target`, `$name` stands for the activated node's prop of that
name: `{"type": "navigate", "target": "$route"}` opens the card's own route.

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
| `screen` | string | Screen name: `home`, `libraries`, `library`, `detail`, `search`, `livetv`, `music`, `player`, `settings`, or one an administrator added. Non-empty. |
| `theme` | ThemeRef, *omit* | Theme id and per-module token overrides. |
| `root` | Node | Must have `type: "Screen"`. |
| `actions` | map[string]Action, *omit* | Named actions referenced by `node.action`. Names match `[A-Za-z_][A-Za-z0-9_-]*`. |
| `meta` | Meta, *omit* | Non-structural metadata. |
| `layout` | Layout, *omit* | The [device profile](#device-profiles) the screen was fitted to. Stamped by Render; one written into an authored document is ignored. |

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
| `focus` | Focus, *omit* | See [Focus](#focus). An author sets only `default` and the guides. |

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
| Focus | Nothing a person can act on is inside something else they can act on (a `Tab`'s content is its own page). At most one `focus.default`, on a node a person can act on. Every focus guide names such a node. |
| Scrolling | Within one page (the `Screen`, a `Tab`'s content, a `Modal`) a scrolling region (`Shelf`, `PosterRow`, `Carousel`, `Grid`, `List`) is never inside another. |
| Where actions lead | `navigate`, `fetch` and `submit` targets are a path on this server beginning with one `/` (`navigate` may also be `$prop`). `external` is an `http://` or `https://` address. So an edited screen can't send a client, and its sign-in, to another host. |
| Pictures and streams | `src`, `imageUrl`, `backdropUrl`, `logoUrl`, `poster`: a path on this server or an `http(s)` address. No `javascript:` or `data:`. |
| Sizes | ≤ 200 children written into one node, ≤ 64 props a node, ≤ 16 KiB a prop, ≤ 256 actions. |
| Condition `formFactor` | `tv`, `phone`, `tablet`, `desktop` or `web`. |
| Colour tokens (the server's store) | `token:<name>` must name a colour the themes have. |

These exist so that an administrator's edit can't break every client at once.
What the store adds on top: the file is validated before it replaces the one in
use, the last 10 revisions are kept and any can be made current again, a file
that doesn't validate is set aside and the last good one used, and a stock
screen missing from the configuration (or one that can't be rendered for the
account asking) is served from its template.

`Unmarshal` additionally rejects input over **2 MiB**, unknown JSON fields,
trailing data and a wrong `version` — before `Validate` runs.

## Template catalog

| Function | Screen | Data | Highlights |
|---|---|---|---|
| `HomeTemplate(HomeData)` | `home` | user name, continue watching, recently added, recommended, extra shelves | Greeting bound to `$.user.name` (title-cased); a hero of featured titles; `home-continue` gated on feature `history`; `home-nextup`; a row per library from `$.home.sections`; `home-recommended` on `recommendations`; `home-games` on module `wittyplay`; `home-livetv` (channels) and `home-music` whenever there is something to show, in their module's colours; search bar only at ≥ 720 px; "Finish server setup" for admins with flag `setupIncomplete`. |
| `LibraryTemplate(LibraryData)` | `library` | library id, title, kind, items | One grid bound to `$.library.items`, its columns the device's; title bound to `$.library.title`; empty-state text on flag `libraryEmpty`. |
| `LibrariesTemplate()` | `libraries` | none | A grid of the libraries the account may open, bound to `$.libraries.items`. |
| `SearchTemplate()` | `search` | none | The search field (default focus), then a row per kind of result from `$.search.sections`. |
| `LiveTVTemplate()` | `livetv` | none | Favourite channels, then every channel the account may watch. |
| `MusicTemplate()` | `music` | none | Continue listening, then recently added albums per music library. |
| `DetailTemplate(DetailData)` | `detail` | item, overview, year, duration, rating, genres, related, children | Detail hero with duration formatted via `duration`; Play on feature `playback`; Resume on flag `hasProgress`; Artwork modal for admins; related row on `recommendations`. |
| `PlayerTemplate(PlayerData)` | `player` | item, stream URL, poster, start offset, next item | Video bound under `$.player.*`; Back hidden on `tv`; subtitles toggle on flag `hasSubtitles`; up-next card on feature `autoplayNext`. |
| `SettingsTemplate(SettingsData)` | `settings` | sections, version | One `Tab` per section; admin sections on role `admin`; module rows on `ModuleEnabled`; sign-out hidden from `guest`; About modal with version bound to `$.server.version`. |
| `DefaultScreens()` | all nine | empty | `map[string]Document` keyed by screen name, used to seed `ui_config.json` and as the fallback for a stock screen the configuration lacks. |

Every template panics if it would produce an invalid document, and the test
suite asserts each one validates, survives a strict JSON round trip, and
renders for a bare guest and a loaded admin.

### Example: rendered `home`

Rendered for an account on an Apple TV HD (`formFactor=tv`), with two titles
being watched and one library. Abridged; the full files are the goldens in
[`pkg/sdui/testdata/`](../pkg/sdui/testdata/): `home.tvhd.golden.json`,
`home.tv4k.golden.json`, `home.phone.golden.json` and `home.web.golden.json`
are this same screen for each device.

```json
{
  "version": 1,
  "screen": "home",
  "theme": { "id": "default" },
  "layout": {
    "formFactor": "tv", "class": "hd", "canvas": { "width": 1920, "height": 1080 }, "scale": 1,
    "safeArea": { "top": 60, "right": 80, "bottom": 60, "left": 80 },
    "scroll": "vertical", "focusEngine": true, "gridColumns": 6,
    "maxSections": 10, "maxItems": 16, "maxGridItems": 60, "externalLinks": false,
    "text": { "pageTitle": 76, "heading": 57, "title": 48, "body": 29, "caption": 25, "hint": 25, "badge": 23 },
    "images": { "poster": { "width": 250, "height": 375, "aspect": "2/3" }, "wide": { "width": 548, "height": 308, "aspect": "16/9" } }
  },
  "root": {
    "type": "Screen", "id": "home",
    "props": { "scroll": true, "title": "Home" },
    "style": { "direction": "column", "gap": 32, "background": "token:bg" },
    "children": [
      { "type": "Toolbar", "id": "home-toolbar", "children": [
        { "type": "Text", "id": "home-greeting", "props": { "text": "Willow", "variant": "pageTitle" } },
        { "type": "SearchBar", "id": "home-search", "props": { "placeholder": "Search your libraries" }, "action": "goSearch",
          "focus": { "focusable": true, "order": 1, "group": "home-toolbar" } },
        { "type": "Button", "id": "home-settings-btn", "props": { "icon": "settings", "label": "Settings", "variant": "ghost" }, "action": "goSettings",
          "focus": { "focusable": true, "order": 2, "group": "home-toolbar" } }
      ]},
      { "type": "PosterRow", "id": "home-continue",
        "props": { "cardShape": "wide", "title": "Continue watching" },
        "children": [
          { "type": "Card", "id": "home-continue:c0", "action": "openItem",
            "props": { "title": "Title c0", "itemId": "c0", "route": "/item/c0", "progress": 0.5,
              "imageUrl": "/api/items/c0/poster",
              "image": { "url": "/api/items/c0/poster?w=548", "width": 548, "height": 308, "aspect": "16/9" } },
            "focus": { "focusable": true, "order": 3, "group": "home-continue", "default": true } }
        ]},
      { "type": "Stack", "id": "home-libraries", "style": { "direction": "column" }, "children": [
        { "type": "Shelf", "id": "home-libraries:lib1",
          "props": { "title": "Recently added in Films", "seeAll": "/library/1" },
          "children": [ { "type": "Card", "id": "home-libraries:lib1:m0", "action": "openItem", "props": { "title": "Title m0", "route": "/item/m0" },
            "focus": { "focusable": true, "order": 5, "group": "home-libraries:lib1" } } ] }
      ]}
    ]
  },
  "actions": {
    "goSearch": { "type": "navigate", "target": "/search" },
    "goSettings": { "type": "navigate", "target": "/settings" },
    "openItem": { "type": "navigate", "target": "$route" }
  },
  "meta": { "title": "Home", "generatedAt": "1970-01-01T00:00:00Z", "etag": "W/\"sha256-…\"", "source": "template:home", "ttlSeconds": 60 }
}
```

Note what is absent: the hero, games, channels and music rows (nothing to put
in them), `home-admin-setup` (not an administrator), and every `visibility`
and `bindings` key. Focus starts on the first card, not in the toolbar.

## Versioning and compatibility policy

- `version` is bumped only for incompatible changes: renaming or removing a
  field, or changing the meaning of one.
- Adding a component type, a `style` field, a condition predicate, an action
  type or a format verb is **compatible** and does not bump the version.
- Clients must tolerate unknown component types (render children, ignore the
  type), unknown props, unknown style fields and unknown action types (ignore
  the action). `sdui.schema.json` says the same to a validator: no object
  forbids extra properties, and a node's `type` is any string.
- The component catalogue has a revision of its own within one schema version
  (`CatalogRevision`, 1 today; every type has a `since`). A client sends the
  revision it was built against as `catalog` (or `X-WMS-SDUI-Catalog`), and
  the server sends a type newer than that as its `fallback`, or a `Stack`.
  A client that sends nothing gets the current catalogue and relies on the
  rule above.
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
4. **Items.** The server has already turned a row's items into child `Card`
   and `ListItem` nodes: draw the children. Use `props.image` (`url`, `width`,
   `height`, `aspect`) for the picture, at that size; `props.more` says the
   row was cut and `seeAll` where the rest is.
5. **Action dispatch.** When a node with `action` is activated, look up
   `document.actions[action]` and run it: `navigate` → your router,
   `play` → your player, `openModal`/`closeModal` → show/hide the `Modal`
   node with that id, `fetch` → request the target and replace/patch the
   screen, `toggle` → flip `props[params.prop]` on the target node,
   `submit` → POST field values, `external` → system browser, `back` → pop.
   Substitute `$name` in `target` from the activated node's `props.name`
   (`$route`, `$itemId`). Ignore unknown action types.
6. **Focus.** Start focus on the node with `focus.default`. Move it in
   `focus.order` where the platform wants an order, treat each `focus.group`
   as one section that remembers where focus was, and follow the `up`,
   `down`, `left` and `right` guides before geometry. See [Focus](#focus).
7. **Layout.** Keep content inside `layout.safeArea`, size text from
   `layout.text` by each `Text` node's `variant`, and give a `Grid`
   `layout.gridColumns` columns when it is not 0.
8. **Updates.** Send `If-None-Match` with your ETag. On 304, keep the tree.
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
| `GET /sdui/topshelf` | any account | [Top Shelf](#top-shelf) content for a tvOS extension. |
| `GET /sdui/config` | admin | All of `ui_config.json`, with `etag`. |
| `PUT /sdui/config` | admin | Replace it; `If-Match` required. |
| `GET /sdui/config/history` | admin | The last 10 revisions. |
| `POST /sdui/config/rollback/{rev}` | admin | Make a kept revision current again. |

### How the request becomes a `RenderContext`

The client says what it is; the server says who is asking. For each client
field the query parameter wins over the header.

| `RenderContext` | From | Notes |
|---|---|---|
| `Platform` | `platform` / `X-WMS-Platform`, else derived from `formFactor` | `web` (default), `tv` (or `tvos`), `ios`, `android`, `desktop`. Form factors map onto them: `phone`/`tablet` → `ios`, `tenfoot`/`10ft` → `tv`, `browser` → `web`. Another short lower-case name is accepted and simply matches no condition. |
| `Profile` | `formFactor` / `X-WMS-Form-Factor`, else the platform; `deviceClass` / `X-WMS-Device-Class` | The [device profile](#device-profiles): `tv`, `phone`, `tablet`, `desktop`, `web`; `ios` and `android` without a form factor are a phone. `deviceClass` is `hd` or `4k`. |
| `Catalog` | `catalog` / `X-WMS-SDUI-Catalog` | The catalogue revision the client knows. |
| `Flags["formFactor:<ff>"]` | `formFactor` / `X-WMS-Form-Factor` | So a condition can say `{"flag": "formFactor:tablet"}`. |
| `Viewport` | `viewport=1440x900` / `X-WMS-Viewport`, or `width` and `height` | CSS pixels. Send it: width conditions pass when it is unknown. |
| `Locale` | `locale` / `X-WMS-Locale`, else the first `Accept-Language` | A language tag (`en-GB`). |
| `Flags[<capability>]` | `capabilities=hdr,touch` / `X-WMS-Capabilities` | Up to 64 names of letters, digits and `. _ : -`; each becomes a flag set to true. |
| `Role` | the account | `admin` for administrators, else the account's kind: `adult`, `kids`, `guest`. |
| `Features` | the server | `/capabilities` `features`, plus `history` and `playback`. |
| `Modules` | the server | The modules the screen's conditions name that are installed and on. |
| `Flags["setupIncomplete"]` | the server | For administrators while first-time setup is unfinished. |
| `Data` | the server | `$.user.name`, `$.user.kind`, `$.user.isAdmin`, `$.server.name`, `$.server.version`, and the screen's own data: see [What the server puts in Data](#what-the-server-puts-in-data). |
| `ThemeOverrides` | `ui_config.json` `theme.modules` | The per-module colours of the [theme engine](THEMES.md#the-theme-engine), so a node with `style.module` gets them in `style.tokens`. |

An invalid value is a 400 naming the parameter. An unknown screen is 404, and
so is a library or title the account may not open. A screen an administrator
added whose root is hidden for this requester is 422; a stock screen in that
state is served from its template instead.

### What the server puts in Data

Each stock screen is filled for the account asking, read the way the ordinary
routes read it: with the account's library, rating and title limits. A title,
show, album or channel an account may not see is never in a screen sent to it.
Only as much is read as the device is sent.

| Screen | Request | Data | Flags |
|---|---|---|---|
| `home` | | `$.home.featured` (5 newest films, backdrops), `continueWatching`, `nextUp`, `sections` (a row of the newest titles per library), `music` (newest albums), `channels` | `setupIncomplete` |
| `libraries` | | `$.libraries.items` | `librariesEmpty` |
| `library` | `library=<id>`, `sort=title\|added\|year`, `offset` | `$.library.id`, `title`, `kind`, `items`, `sort` | `libraryEmpty` |
| `detail` | `item=<id>` or `show=<id>` | `$.item.id`, `title`, `subtitle`, `overview`, `year`, `rating`, `duration`, `backdrop`, `progress`, `playId`, and for a show `children` (its episodes) | `hasProgress`, `notPlayable` (a show) |
| `search` | `q=<text>` | `$.search.query`, `sections` (a row per library with matches, and channels) | `searchIdle` (under two letters), `searchEmpty` |
| `livetv` | | `$.livetv.favourites`, `channels` | `livetvEmpty` |
| `music` | | `$.music.continueListening`, `sections` (newest albums per music library) | `musicEmpty` |
| `settings`, `player` | | none: `settings` is a list of places in the app's own settings, and `player` is the frame around a player the app drives with the playback routes | |

An item is `{id, title, subtitle, kind, imageUrl, progress, route, playRoute,
shape, module}`. `route` is the place in the app (`/item/12`, `/show/3`,
`/album/…`, `/tv/watch/lib-4`, `/library/2`), the same routes the web client
uses; an app maps them to its own screens.

A screen an administrator adds under another name gets `$.user` and
`$.server` only.

### Top Shelf

`GET /sdui/topshelf` answers what a tvOS Top Shelf extension shows: what the
account was in the middle of, then what is new in the first libraries it may
open.

```json
{ "version": 1, "style": "sectioned", "expiresInSec": 900,
  "sections": [ { "title": "Continue watching", "items": [
    { "id": "12", "title": "Metropolis", "shape": "poster", "progress": 0.4,
      "image": { "url": "/api/items/12/poster?w=404", "width": 404, "height": 608, "aspect": "2/3" },
      "displayRoute": "/item/12", "playRoute": "/watch/12" } ] } ] }
```

At most 4 sections of 10 items: the extension has seconds and little memory.
`shape` is `poster`, `wide` or `square`, the three tvOS has, and `image` is
sized for it (404×608, 908×512, 608×608 points, times 2 with `deviceClass=4k`).
Build a `TVTopShelfSectionedContent` from it, with `displayRoute` and
`playRoute` wrapped in the app's own URL scheme as the display and play
actions. It has an `ETag`; ask again after `expiresInSec`. The extension signs
in with the app's token, kept in a shared keychain group.

### Rendering, ETags and 304

`meta.etag` and the `ETag` header are `sdui.ETag` of the rendered document,
which is the same for every request that renders the same tree. Send it back as
`If-None-Match` for a 304. Responses carry `Cache-Control: private, no-cache`.

### Diffs

`GET /sdui/screens/{screen}/diff?since=<etag>` (or with `If-None-Match`), with
the same device parameters as the screen, renders the screen and then:

- answers **304** when `since` is the current ETag;
- answers `{"etag", "since", "patches": [...]}` when the server still has the
  render `since` names, kept for this account (its last 6, and at most 128
  across all accounts, 32 or 12 on a small machine; in memory, gone at
  restart). Apply the patches with the
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
