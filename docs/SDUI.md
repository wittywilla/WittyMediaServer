# Server-Driven UI (SDUI)

WittyMediaServer describes the pages of its apps as data. An administrator
says what Home holds, in what order, for whom; the server works out what that
comes to for one profile on one device and sends a tree of components; the web
app, the Apple apps and the Android apps each draw that tree with their own
widgets. Re-ordering Home, giving Kids profiles their own Home, adding a page
for film night or a row for one collection is a change on the server that
every app picks up at once.

This document is the contract: the model, every component and data source,
what a client sends, what it is sent, and exactly how it must draw it. The
same thing for tools:

- [`sdui.schema.json`](sdui.schema.json): a JSON Schema of everything sent and
  accepted, written from the code;
- `GET /api/v1/sdui/catalog`: the catalogue as data, for editors;
- [`pkg/sdui/conformance`](../pkg/sdui/conformance/README.md): fixtures that
  pin the contract down case by case, for renderers' test suites.

The Go library behind it is [`pkg/sdui`](../pkg/sdui/README.md): pure Go,
standard library only, no HTTP and no storage.

**Settings is not part of this.** Every app keeps its own Settings, built by
hand. SDUI can link to it (a button, a navigation entry) and nothing more:
there is no Settings page kind, and the navigation always holds the native
Settings entry.

## Design

### What it is now

- **Semantic components.** A page is a list of *sections*, each one a
  component a person would name: `Hero`, `Shelf`, `Grid`, `DetailHeader`,
  `SeasonList`, `TrackList`, `FilterBar`, `Tabs`, `Banner`, `Notice`, `Text`.
  Each renderer maps a component to its platform's own idiom (a tvOS shelf
  with the focus engine's lift and parallax, a Compose `LazyRow`, a CSS grid),
  which is what makes the result good everywhere. A small set of layout
  components (`Stack`, `Split`, `Spacer`, `Divider`, `Button`, `Image`,
  `List`) is there for pages of one's own.
- **Data sources.** A section says where its items come from by name:
  `{"type": "recentlyAdded", "library": 3}`, `{"type": "genre", "genre":
  "Comedy"}`, `{"type": "collection", "collection": 12}`. The server answers
  each with the library's own indexed queries, through the profile's limits,
  only for sections that are on the page, a page of items at a time. How an
  item becomes a card is fixed by the card's style: nobody writes a field
  path.
- **Layers.** Built-in pages; over them the administrator's; a page per
  library, kind of library or kind of title; a variant for a kind of profile
  or device; conditions on any section; and each profile's own order, hidden
  and pinned rows, density and card sizes, merged by the server.
- **A catalogue that describes itself**, validation that says what to do,
  presets, a draft with a preview on a real device, import and export,
  history with a diff, and reset to default by deleting a page.
- **Navigation in the model**, with Settings always present and always
  native.
- **Rules a renderer can be tested against.** Sizes are abstract and resolved
  by the device profile; colours are theme tokens only; focus, states,
  truncation, labels and pictures are specified here and pinned down by
  fixtures.

Two things were deliberately left out. There is no expression language:
conditions are a fixed set of fields, because anything else is code an
administrator has to debug. And there are no scripts, no web views and no
arbitrary addresses: a page can only open places in the app, the app's own
screens, and (where the device has a browser) a web page in the browser.

## Designed for tvOS first

An Apple TV is the strictest device a page has to work on, so the model is
built around its rules and every other device gets them for free.

| tvOS rule | What the model does about it |
|---|---|
| There is no pointer. Everything a person can act on must take focus, and focus must start somewhere. | Every node a person can act on has `focus.order` in reading order and a `focus.group`; exactly one has `focus.default`. See [Focus](#focus). |
| A page scrolls one way and its rows scroll sideways. Nothing scrolls both ways, and scrolling regions don't nest. | The components are that shape: a `Page` of sections, `Shelf` rows inside it. The validator refuses a row inside a row, and anything after the one `NativeView` or below a `Grid` that never ends. |
| Focus moves by direction, and guessing by geometry jumps between rows of different lengths. | Movement between sections follows the page's order and each group remembers its member; geometry is never used to pick a row. |
| The edge of a television picture can't be relied on. | `layout.safeArea`: 60 points top and bottom, 90 at the sides, on the 1920×1080 canvas. |
| It is read from the sofa. | `layout.text` is the television's type sizes (`tv.type` in themes/tokens.json: 57, 48, 38, 29, 25 points, and 76 for `display`) and cards are sized for three metres. |
| An Apple TV HD has little memory and a slow network stack. | A page is cut to `budget.maxSections`; only the first `budget.eagerSections` rows arrive with cards, 12 a row; the rest are fetched as they come near. |
| A 4K poster in a 250-point card wastes all of it. | Every picture's address carries the width to ask for, from a short list of widths: 320 pixels for a poster on an HD, 640 on a 4K. |
| There is no web view and no browser. | There is no web component; a button that opens a web page is not sent to a television. |
| The system has its own search keyboard, focus effect and player. | `SearchBox` is the system's field, a card takes the system's focus effect, and playing is an action the app's own player carries out. |
| The Top Shelf is not a page. | It has its own small document: [Top Shelf](#top-shelf). |

## The model in one page

| Term | What it is |
|---|---|
| **Page** | An ordered list of sections with an ID, a kind, a title. Authored; stored in `ui_config.json` when it differs from the built-in one. |
| **Kind** | What a page is for: `home`, `library`, `detail`, `search`, `livetv`, `music`, `photos`, `books`, `games`, `collections`, `libraries`, `custom`. It decides the page's route and what its sources may refer to. |
| **Section** | One component on a page: `{id, component, props, source, when, repeat, scope, locked, children}`. |
| **Component** | A type from the catalogue, with properties. |
| **Source** | Where a section's items come from: a type and its parameters, written flat. |
| **Condition** (`when`) | When a section, variant or navigation entry is shown. |
| **Repeat** | One copy of a section per library, pinned saved view or collection, or per series or author of the page's books library. |
| **Variant** | Another list of sections for the same page, used when its condition holds. |
| **Match** | Narrows a library page to one library or kind of library, a detail page to a kind of title. |
| **Scope** | The theme scope (a module's ID, or a name of one's own) whose colour overrides a page or section takes. |
| **Navigation** | The main menu: entries that open pages, libraries, native screens or routes. |
| **Personal** | What one profile changed about one page for itself. |
| **Device profile** (`layout`) | What a page is fitted to: sizes, budgets, text limits. |
| **Document** | A page rendered for one profile on one device: a tree of nodes. What a client is sent. |

A page as an administrator writes it:

```json
{
  "id": "home", "kind": "home", "title": "Home", "icon": "home",
  "sections": [
    { "id": "hero", "component": "Hero", "source": { "type": "featured" } },
    { "id": "jump", "component": "Shelf", "props": { "title": "Jump back in" },
      "source": { "type": "continueWatching", "media": "all" } },
    { "id": "onnow", "component": "Shelf", "props": { "title": "On TV now" }, "scope": "wittytv",
      "when": { "can": "liveTv", "feature": "live-tv" },
      "source": { "type": "liveNow" } },
    { "id": "recent", "component": "Shelf",
      "props": { "title": "Recently added in {each}", "seeAll": "always" },
      "repeat": { "over": "libraries" },
      "source": { "type": "recentlyAdded", "library": "$each", "limit": 20 } }
  ],
  "variants": [
    { "id": "kids", "when": { "profileKind": ["kids"] }, "sections": [
      { "id": "jump", "component": "Shelf", "props": { "title": "Keep watching", "cardSize": "l" },
        "source": { "type": "continueWatching", "media": "watch" } } ] }
  ]
}
```

What an Apple TV is sent for it (abridged; the whole of this and fifty more
are in [`pkg/sdui/conformance/fixtures`](../pkg/sdui/conformance/fixtures/index.json)):

```json
{
  "version": 1, "catalog": 2,
  "page": { "id": "home", "kind": "home", "title": "Home", "icon": "home", "route": "/" },
  "layout": { "formFactor": "tv", "class": "hd", "scale": 1, "breakpoint": "large",
              "safeArea": { "top": 60, "right": 80, "bottom": 60, "left": 80 }, "focusEngine": true, "…": "…" },
  "root": { "type": "Page", "id": "page", "props": { "title": "Home" }, "children": [
    { "type": "Hero", "id": "hero", "focus": { "order": 1, "default": true },
      "props": { "style": "billboard", "autoAdvanceSec": 8, "size": { "width": 1920, "height": 620 } },
      "children": [ { "type": "Card", "id": "hero:101", "props": { "style": "hero", "title": "Film 1", "…": "…" } } ] },
    { "type": "Shelf", "id": "jump",
      "props": { "title": "Jump back in", "cardStyle": "progress", "cardSize": "m",
                 "card": { "width": 420, "height": 236, "aspect": "16/9", "shape": "landscape" }, "total": 3 },
      "children": [
        { "type": "Card", "id": "jump:103", "focus": { "order": 2, "group": "jump" },
          "action": { "type": "navigate", "to": "/item/103" },
          "props": { "style": "progress", "shape": "landscape", "title": "Film 3", "subtitle": "1993",
                     "progress": 0.42, "remaining": "1 hr 1 min left", "itemId": "103", "kind": "movie", "playId": "103",
                     "image": { "url": "/api/items/103/backdrop?w=480", "width": 480, "height": 270, "aspect": "16/9", "kind": "backdrop" } } } ] },
    { "type": "Shelf", "id": "recent-2", "state": "deferred",
      "props": { "title": "Recently added in Shows", "cardStyle": "poster", "cardSize": "m",
                 "card": { "width": 250, "height": 375, "aspect": "2/3", "shape": "poster" },
                 "more": { "url": "/api/v1/sdui/pages/home/sections/recent-2/items?cursor=0" } } }
  ] },
  "personal": { "editable": true, "allow": ["reorder", "hide", "pin", "density", "cardSize", "defaultTab"],
                "sections": [ { "id": "hero", "title": "Featured" }, { "id": "jump", "title": "Jump back in" } ],
                "url": "/api/v1/sdui/pages/home/personal" },
  "meta": { "etag": "W/\"sha256-…\"", "generatedAt": "2026-11-11T19:30:00Z", "source": "config", "configRevision": 7, "ttlSeconds": 60 }
}
```

## Precedence

What a profile sees on a page is decided in this order. Each step works on
the result of the one before.

1. **The built-in page.** Every stock page ships in the server
   ([Built-in pages](#built-in-pages)), written in this same model.
2. **The administrator's page with the same ID replaces it whole.** Deleting
   that page from the configuration brings the built-in one back ("reset to
   default"). Pages with other IDs are the administrator's own.
3. **The most specific page of the kind asked for wins.** For
   `pages/library?library=3`: a page whose `match.library` is 3, else one
   whose `match.libraryKind` is that library's kind, else the page with no
   match. For `pages/detail`: `match.mediaKind` (and `libraryKind`) likewise.
   Among equals, the administrator's page beats a built-in one.
4. **The first variant whose condition holds replaces the page's sections.**
   Variants are tried in order; none holding, the page's own sections stand.
5. **Sections whose `when` doesn't hold are dropped**, and repeated sections
   become their copies (`recent` becomes `recent-1`, `recent-2`, ...: the
   section's ID, `-`, the library's, view's or collection's ID).
6. **The profile's own arrangement** is applied to the top-level sections:
   hidden ones go; pinned ones come first, in the order they were pinned;
   then the sections its order names, in that order; then the rest in the
   page's order. A `locked` section is never hidden and never moves. A copy
   of a repeated section the profile never placed sits where the section it
   is a copy of was placed. Only what the page's `personal.allow` lists is
   applied, and nothing is when the profile's `editLayout` switch is off or
   the server has personalisation off.
7. **The device's budget**: sections past `budget.maxSections` are left out
   (`layout.truncated`), and data sections past `budget.eagerSections` arrive
   `deferred`.
8. **Each section's source is asked**, through the profile's limits. A
   section with nothing in it is left out, unless it says `"empty":
   "message"`.

## What a client sends

With every SDUI request, as query parameters or headers (the query wins):

| Parameter | Header | Values |
|---|---|---|
| `platform` | `X-WMS-Platform` | `web` (default), `tvos`, `ios`, `android`, `androidtv`, `desktop`. |
| `formFactor` | `X-WMS-Form-Factor` | `tv`, `phone`, `tablet`, `desktop`, `web`. Left out, it follows from the platform (tvos and androidtv: tv; ios and android: phone). |
| `deviceClass` | `X-WMS-Device-Class` | `hd` (default) or `4k`, for a television. |
| `views` | `X-WMS-SDUI-Views` | The native views it has for `NativeView`: `guide`, `photoTimeline`, `photoMap`, `coverFlow`. |
| `viewport` | `X-WMS-Viewport` | The window in points: `1440x900`. Send it on anything that isn't a television, and ask again when the breakpoint changes. |
| `locale` | `X-WMS-Locale` | A language tag; else the first of `Accept-Language`. |
| `capabilities` | `X-WMS-Capabilities` | What the device has: `reducedMotion`, `touch`, `pointer`, `keyboard`, `hdr`. `reducedMotion` changes what is sent; all of them can be asked about by a condition. |
| `preview` | `X-WMS-SDUI-Preview` | A preview token: see [Draft and preview](#draft-and-preview). |

Who is asking (the profile, its kind, its limits and feature switches) the
server knows from the sign-in. An invalid value is a 400 naming the
parameter.

## The rendered document

All keys are camelCase; fields marked *omit* are left out when empty. The
Go types are `sdui.Document`, `sdui.Node`, `sdui.Action`, `sdui.Focus`.

### Document

| Field | Type | Notes |
|---|---|---|
| `version` | int | `1`. A client refuses any other. |
| `catalog` | int | The catalogue revision the tree was written for (8). A client built for another revision refuses it. |
| `page` | PageInfo | `{id, kind, title, icon, route, variant, params}`. `route` is the page's place in the app; `params` what it was opened with, to send again with a refresh. |
| `layout` | Layout | The [device profile](#device-profiles) the page was fitted to: lay out with these numbers. |
| `theme` | ThemeScope, *omit* | `{scope, tokens}`: the page's theme scope and the colour tokens it overrides. |
| `root` | Node | The tree. Its type is `Page`. |
| `screenTime` | ScreenTime, *omit* | `{reason, message, nextAt}`, present when the profile's screen time is over: see [Profiles with limits](#profiles-with-limits). |
| `regions` | PageRegions, *omit* | What the page puts into the shell's regions while it (or its theme scope) is showing: `statusBar` items and `systemDock` entries. See [Shell regions](#shell-regions-and-what-a-page-supplies-to-them). |
| `personal` | PersonalInfo, *omit* | What the profile may change about this page and what it has changed: see [Personalisation](#personalisation). |
| `meta` | Meta | `{etag, generatedAt, source, configRevision, ttlSeconds}`. `source` is `config` or `draft`. Ask again after `ttlSeconds` (0: only when something happens). |

### Node

| Field | Type | Notes |
|---|---|---|
| `type` | string | A [component](#components). Draw a type you don't know as a plain column of its children. |
| `id` | string | Unique in the document, stable between renders, at most 128 characters of `A-Z a-z 0-9 _ . : - ~`. A section's node has the section's ID; a card's is `<section>:<item id>`. |
| `props` | object, *omit* | The component's properties, resolved. Ignore properties you don't know. |
| `children` | []Node, *omit* | In display order. |
| `action` | Action, *omit* | What activating the node does. |
| `focus` | Focus, *omit* | `{order, group, default}`: see [Focus](#focus). Present on every node a person can act on. |
| `state` | string, *omit* | `deferred`, `empty` or `error`; absent when ready. See [States](#states). |
| `scope`, `tokens` | string, map, *omit* | The node's theme scope and the colour tokens it overrides, for it and everything in it. |

### Action

| `type` | Fields | What the client does |
|---|---|---|
| `navigate` | `to` | Opens a route: look it up in [Routes](#routes). It is an SDUI page (ask for it) or one of the app's own screens. |
| `play` | `item`, `params.resume` | Starts playback of the item with the app's own player, from where the profile left off unless `resume` is false. |
| `native` | `to`, `params` | Opens one of the app's own screens ([Native screens](#native-screens)), for the item in `params.item` when there is one. |
| `external` | `url` | Opens a web page in the system browser. Never sent to a device that has none. |
| `back` | | Goes back. |
| `command` | `op`, `item`, `params` | Carries out one of the [Commands](#commands), then asks for the page again. |

A command action may carry `confirm`, a question to ask first ("Delete this memory?"): the client shows it with the command's label as the confirming button and Cancel beside it, and runs the command only on a yes.

A `navigate` action may carry `as: "layer"`: the page is shown over the current screen (a sheet, a quick menu, a dialog) and Back closes it, instead of replacing the screen. A client with no layers shows it as a page. Any page can be a layer; there is no separate kind.

Ignore an action type you don't know.

A command's request is built from its catalogue entry: put the action's
`item`, exactly as sent (`"5"`, `"show-5"`), in place of `{item}`,
URL-escaped in the path and JSON-escaped in the body, where it always stands
inside a string (`{"items":["{item}"]}`), so the body is JSON for any title.
The routes take a show's `show-<n>` as they take a number: Watch later adds
the show, `watched.set` and `watched.clear` mark every episode of it.

A command may take more values: its catalogue entry lists them as `params`
(`name`, `type` of `string`, `int`, `bool` or `ints`, `required`). The action's `params` give
them by name, and each goes in place of `{name}` the same way: URL-escaped in
the path, JSON-escaped inside the body's string. For an `int` parameter the
string and its quotes are replaced by the number (`{"type":"save","slot":"{slot}"}`
becomes `{"type":"save","slot":3}`), A `bool` parameter is replaced the same way by `true` or `false`, an `ints` one (a list of numbers, such as the profiles a Kids profile may see) by a JSON array (`[3,5]`), and an `object` one (a collection's rule) by a JSON object. A Form builds an `object` param from its controls named `<param>.<key>` (`rule.yearFrom`): each becomes a key of the object, a number field a number, an empty control no key, and a key ending `[]` (`rule.genres[]`) splits a comma-separated text into a list of texts. Either way the template is JSON. A
parameter that wasn't given is an empty string. `rewind.open` and `controller.remap` have no request:
the game player opens its own panels.

## Rules for renderers

These are requirements. The conformance kit checks the ones that can be
checked without pixels.

### Unknown and future things

- An unknown `type` is drawn as a column of its children; unknown props and
  unknown top-level fields are ignored; an unknown action does nothing.
- An unknown card `style` is drawn as its `shape` (`poster`, `landscape`,
  `square`, `circle`, `chip`): the picture, with the title and subtitle under
  it.
- An unknown `state` is treated as ready.

### Units, sizes and breakpoints

- Every size is in **points**: iOS and tvOS points, Android dp, CSS pixels.
  `layout.scale` is pixels per point and is used only to choose pictures.
- Spacing is one of six steps (`none`, `xs`, `s`, `m`, `l`, `xl`);
  `layout.spacing` says how many points each is on this device. The distance
  between a page's sections is `l` (`m` at the compact breakpoint).
- Text is one of six roles (`display`, `pageTitle`, `heading`, `title`,
  `body`, `caption`; `badge` is a seventh, for badges); `layout.text` gives each one's size. Use the platform's
  dynamic type for the role where it has one; the numbers are the sizes at
  the default setting.
- A row or grid says how big its cards are in `props.card` (`width`,
  `height`, `aspect`, `shape`): the size of the picture. Text goes under it.
- **Breakpoints** are by the window's width: `compact` under 600 points,
  `medium` under 840, `expanded` under 1200, `large` from there.
  `layout.breakpoint` is the one the page was rendered for. When the window
  crosses a breakpoint, ask for the page again with the new `viewport`.
- **Grid columns**: `props.columns` is the count for the viewport sent. When
  the window changes without crossing a breakpoint, recompute:
  `max(minimum, floor((content + gap) / (card + gap)))`, where `content` is
  the window's width less the side margins (two `m`, or the safe area on a
  television), `gap` is `m` (`s` + `xs` at the compact breakpoint), and
  `minimum` is 2 (3 for cards under 130 points). Cards stretch to fill the
  row. A television's grid has `layout.cards[style].columns` columns.
- Content keeps inside `layout.safeArea`; where it is all zero, inside the
  system's own safe area.

### Focus

`node.focus` is on every node a person can act on, and on nothing else.

| Field | Meaning |
|---|---|
| `order` | Its place in reading order, from 1, without gaps. The same page gives the same numbers. Tab and Shift-Tab, and a screen reader's next and previous, follow it. |
| `group` | The ID of the row, grid, list, tab strip or row of buttons it is in. A group is one focus section: moving into it lands on the member that last had focus in it, else its first. |
| `default` | Exactly one node has it: focus starts there. It is a search box that asks for it, else the page's `DetailHeader`, else the first focusable node that isn't a tab, a filter bar or a search box, else the first of all. |

Directional movement (a remote, a game pad, arrow keys in TV mode):

- Inside a `Shelf`, left and right move between its cards and stop at the
  ends. Inside a `Grid`, the four directions move by row and column.
- Up and down between top-level sections go to the previous or next group in
  `order`, landing on its remembered member. Geometry is never used to pick a
  row: the page's order is.
- A `Hero`, a `DetailHeader` and a `FilterBar` are one stop each: inside,
  left and right move between their own buttons or fields in the order
  given, starting on the primary one.
- A `deferred` section takes no focus until its items have arrived; cards
  that arrive later join their group at the end, in the order sent.
- With a pointer or touch, `focus` is only the keyboard order.

### States

| `state` | What to draw |
|---|---|
| absent | The section, ready. |
| `deferred` | The section's heading and placeholders, as if loading. When it comes within one screen of being seen, GET `props.more.url`: replace the placeholders with the `items`, merge `props`, and continue from `next`. If the answer's `state` is `empty`, take the section off the page. |
| `empty` | The heading and `props.emptyText` in the caption style where the items would be. |
| `error` | The heading and one line, "Couldn't load this", with a Retry button that GETs `props.more.url`. |

While any request is in flight a section shows **placeholders**: grey
rounded rectangles of the card's size, as many as are at least partly on
screen (`floor((content + gap) / (card + gap)) + 1`, at most the row's first
page), pulsing unless motion is reduced. A whole page loading for the first
time shows three such rows. A page that fails to load shows the app's own
error view with Retry; a 404 shows "This isn't available" with a way Home.

### Paging

A `Shelf`, `Grid`, `List` or `SeasonList` with `props.more` has more items
than it was sent. GET `more.url` when the person scrolls within one screen
(a grid: two rows) of its end, with the same device parameters as the page.
The answer is a `SectionItems`:

```json
{ "version": 1, "section": "all",
  "items": [ { "type": "Card", "id": "all:161", "props": { "…": "…" } } ],
  "next": "/api/v1/sdui/pages/library-movies/sections/all/items?cursor=96&library=1" }
```

Append `items` to the section's children; ask `next` in turn; no `next`
means the end. With `replace: true` (another season) the items take the
place of the ones there. `props`, present on a first page, are the section's
own props as they are now. Never ask for the same cursor twice at once.

### Text

- The server has already cut text to the device's limits
  (`layout.textLimits`): at a word where one is within 20 characters, with
  an ellipsis. A renderer shortening text itself (it shouldn't need to) uses
  the same rule, `sdui.Truncate`.
- Each card style allows its title and subtitle a number of lines
  ([Card styles](#card-styles)); a synopsis on a card gets 2, on a hero
  slide 3; a `DetailHeader`'s title 2 and overview 4, with "More" to read
  the rest. Past that, truncate with an ellipsis at the end.
- Text is never scaled down to fit, and never scrolls sideways.

### Pictures

- `props.image` is `{url, width, height, aspect, kind}`. Ask for `url` as it
  is: it already carries the width (`?w=`), chosen from
  `layout.imageWidths` as the smallest that covers the card at the device's
  scale. Don't ask for another size. A backdrop that spans the window (a
  title's header, a hero's slide) is chosen for the window's width (the
  `viewport` sent, else the breakpoint's usual window) at no more than two
  pixels a point, whatever the device's scale: 960 on a 402-point phone.
  Only a real backdrop is sent as one: a title that has only a poster gets
  no `image` on a hero slide or banner and no `images.backdrop` in its
  header, never its poster stretched to 16/9; draw it on the theme's colours.
- Draw it in the card's aspect, cropped to fill (`cover`), except a channel's
  logo and a title's logo, which are fitted inside (`contain`) on a plain
  tile in token `bg3`.
- While it loads, and if it fails, fill its place with token `bg3` and the
  item kind's icon in token `fg2`. A card with `props.icon` and no image is
  drawn the same way on purpose.
- Decode off the main thread and cancel when the card leaves the screen.

### Colour and theme

- Renderers use the theme's tokens ([THEMES.md](THEMES.md)) and nothing
  else: no colour is ever in a page. Backgrounds `bg`, `bg2`, `bg3`; text
  `fg`, `fg2`; the focus ring, progress bars, selected tabs and primary
  buttons `accent` with `accentFg` on it; hairlines `border`; text over
  pictures `onImage` on `scrim`.
- A node with `tokens` overrides those tokens for itself and everything in
  it; `document.theme.tokens` does so for the whole page. That is how a
  module's colours, or a page's own accent, reach a renderer: see
  [Theme scopes](#theme-scopes).
- Each component and card style says in the catalogue which tokens it is
  drawn with (`tokens`: colours by part, text roles, shape tokens), and
  `GET /api/v1/theme` has the values: colours (with the profile's own and
  the colours derived from each override), `shape` (corner radii and the
  focus ring, in points) and `type` (typefaces and weights). A renderer
  draws from those and has no colour, corner or typeface of its own: see
  [THEMES.md](THEMES.md#sdui-tokens) and the [table](#theme-tokens). A
  node's `tokens` are what its scope's palette has that the account's
  `tokens` don't; with the automatic theme they are its dark look's, so a
  client following the device takes `variants[scheme].modules[scope]`.

### Motion

- With `layout.reducedMotion` (the client sent `capabilities=reducedMotion`)
  a `Hero` has no `autoAdvanceSec` and must not move by itself; cross-fades
  become cuts; placeholders don't pulse; focus effects keep their ring and
  drop their scale and parallax. A renderer also honours the system setting
  directly.
- A `Hero` never advances while one of its buttons has focus or the pointer
  is over it, and pauses when it is off screen.

### Accessibility

Every component's role and label derivation is in its entry under
[Components](#components); the fixtures hold the exact strings. In short:

- A card is a button (a link on the web) labelled with what its style
  shows, in the style's order, joined by ", ": `Film 3, 1993, 42 percent
  watched, 1 hr 1 min left`. A title that isn't drawn (a photo's) is still
  in the label.
- A row is a list labelled by its title; a grid a grid (a list on touch
  devices); headings are headings at levels 1 to 3 by role.
- Touch targets are at least 44 points; a television's buttons at least 66
  tall.
- Markdown is read as plain text: links by their text.

### Components

Every component in the catalogue is drawn by every renderer: there is no
negotiation, no optional component and no fallback. The server never sends a
tree in place of another; a client built for a catalogue revision other than the
document's `catalog` refuses the document. The only thing a client chooses is
which native views it has for `NativeView` (`views=`); where it has none, the
section's own children are sent in a `Stack`.

### Shell regions and what a page supplies to them

Two regions of the shell are off in every look and belong to WittyPlay Online: `statusBar` (position top) and `systemDock` (bottom or side). A page turns them on by supplying content in its `regions` field, which is sent as `document.regions` (an administrator or a profile can still hide them with placements, and the placement of an experience or module scope turns them on too):

- `statusBar`: up to six items, `{id, kind, text}`. `kind` is `profile` (the profile's name, `text` the line under it), `server` (the server's name drawn from the brand, so the coloured name survives) or `text`. The clock is drawn by every client, and so are a controller's battery and its kind, from the device's own Gamepad API: the server never sends them. Text may hold placeholders (`{user}`, `{server}`).
- `systemDock`: up to seven round buttons, `{id, label, icon, route, as}`; the one whose route is the page showing is labelled under its button. `as: "layer"` opens that page as a layer.

The regions stay while the module's pages are showing (the same theme scope) and go when the person leaves them.

### Forms and controls

A List row can carry a **control** besides a route: `control: {type, name, value, ...}` in the row's props, from an item's `control` (a source's, or a module's: the server cleans it). Types: `switch` (value a bool), `select` (value a string, `options` the choices), `field` (value a string; `placeholder`, `maxLength`, `multiline`, `input` of text, number, url or search) and `slider` (value a number, `min`, `max`, `step`, `unit`). A disabled control is dimmed and takes no focus, and `why` says why in a line under it.

- A **switch** row's `action` is what flipping it does. The server has already put the new value in the action's params (`value`, a bool), so a client never works out the opposite; after the command it asks for the page again, and the switch shows the stored state.
- A **select, field or slider** row's `action` is a command to run with the chosen value added to its params as `value`: a field on Enter or when it loses focus, a slider on release.
- A row's `trailing` text and `action` work as on any list row; a row with a control and no action is read-only.

A `Form` component (a container of Lists, Text and Notices with a `submitLabel`, an optional `cancelLabel` and one command) draws a Save and a Cancel at its foot instead: controls inside a Form run nothing when changed, and Save runs the Form's command with every named control's value under its `name`. See the Form entry in the component reference.

### Detail panels, focusGrow and item actions

A Shelf with `focusGrow` scales the focused card up (1.45 on a television, 1.25 on a tablet and desktop, none on a phone, whose first card is simply larger) and moves the cards after it aside; with reduced motion it gets a ring and no animation. A Shelf with `detail` draws a panel under the row for the focused item (the first until focus moves): the title large, the subtitle as one line, and the item's `actions` as buttons carrying their key glyph (`a` select, `b` back, `x` and `y` the secondary and options keys of the shared pad map). A card carries `props.actions` (`[{id, label, glyph, action}]`, at most four); an item with none shows title and subtitle, and the card's own action is the A action. `focusable` on a Shelf, Grid or List makes a card with no action a focus stop anyway (a friend in a lobby). A **person** card draws a presence dot on its avatar from its badge text, by the card style's `badgeTones` in the catalogue (Playing and Online ok, Away warn, Offline muted): the same on every app.

### Pages the server serves for every app

These pages are drawn from the server's own data by the same renderer as every other page; no app needs a screen of its own for them (an app that has a hand-built one may keep it, but it must open these routes through the renderer when a row or button names them). None is in the menu. Opened with `GET /sdui/pages/{page}?item=…&name=…` as the route table says.

| Route | Page | Opened about | What it holds |
|---|---|---|---|
| `/studio/{name}`, `/network/{name}`, `/category/{name}` | `studio`, `network`, `category` | a name | Everything under it across the libraries the profile can open: Movies (n) and Shows (n), each a grid; the heading is the name. A category is a genre. |
| `/versions/{item}` | `versions` | a title | The files it plays from, one row each (label and quality, length, size); a row plays that file. |
| `/save-to/{item}` | `save-to` | a title | The profile's lists as switch rows (Watch later and its own manual lists): flipping one runs `collection.add` or `collection.remove`; a Form with a field makes a new list (`collection.create`). |
| `/devices` | `devices` | | Where the profile is signed in, this device first; a row signs another out (`session.end`, after asking); Sign out the others. |
| `/year` | `year` | | Totals, months, top titles, shows, artists and genres from the profile's own play log; Clear my history (asks first). |
| `/activity` | `activity` | | Tabs Now and History of what the server is doing; what the profile may see follows its limits. |
| `/notifications` | `notifications` | | Reminders and, for an administrator, a new version. |

A page opened about something is a `custom` page that takes `item`, `name` and `of`; its sources read `$item`, `$name` and `$of`, and `{title}` is the title (item) or the name. Each source checks what it is asked about against the profile's limits: a title a profile may not see has no versions and no lists, whatever the URL says.

**Search** is scoped: below the search box, Tabs for Everything, Movies, Shows, Music, People and Live TV, each holding what it searches (a repeat over libraries of those kinds, the people row, the channels row). Home's *Recommended for you* row says why under its heading (`{reason}`, from the source: "Because you watch Sci-Fi").

**The key legend.** Every page rendered for a television or a headset ends with a Dock holding the key legend (Select, Back, Options), whether or not the page has controls to put in a Dock: a client draws no legend of its own and never needs to put the Dock back.

### QR codes

The `QR` component draws a QR code for a link, for a phone to scan (a room's join link, a pairing address). Its `value` is a route of the app (`/games?join=TIDE-4821`) or an https address; with a source, the first item's `link` is the value and its title the caption (a source whose item has no link gives nothing, and the section is left out: a room's link is the host's alone). The node's props are `value`, `isRoute`, `label` and `size` (s, m, l): when `isRoute` is true the client makes the address absolute with the server's own base address before it encodes it. Clients draw it black on white with a quiet zone of four modules and error correction M whatever the theme, because a scanner needs the contrast; the node takes no focus, and on a desktop the address is shown under it as selectable text.

### Books and search filters

A books library's Books tab has a FilterBar with two more filters: `reading` (Reading, Finished, Not started: what the profile has started, finished or never opened) and `format` (Books, PDFs, Comics), and the grid's `books` source follows them (`filterable`). A book's page can list its formats with the `formats` source (the format that opens and the other files of the same name beside it, with sizes; never paths). On the search page a FilterBar with no target (`narrow`) offers Year and Score; every filterable search section follows them, so a film, show or anime library's results are narrowed by the library's own filters.

### Empty sections that say what to do

A Shelf, Grid or List with `empty: "message"` and nothing to show is drawn as an empty state: its `emptyText`, then the optional `emptyHint` (a line of advice) and the optional `emptyAction` (one button). The node has `state: "empty"` and props `emptyText`, `emptyHint` and `emptyAction`, where `emptyAction` is `{label, action}` and `action` is an ordinary action (navigate, native or a command, with `as: "layer"` and `confirm` as everywhere). The author gives the button a `when` condition, so only administrators are offered "Scan now" or "Add a games library" and everyone else sees the hint alone; the server has already applied it, so a client draws what it is sent. A page with no data should say what to do rather than draw a bare title: every client draws the empty state generically from these props, the same on every page. The built-in library pages for photos, books, games and other libraries offer an administrator `library.scan` (the library is the item).

### The conformance kit

[`pkg/sdui/conformance/fixtures`](../pkg/sdui/conformance/fixtures/index.json) holds
one JSON file per case: an authored page, a request, canned data, the
document the server must send for them, and the *normalised layout* a
renderer must make of that document (what is visible in what order, cards
per row, card sizes, columns, the picture each card asks for, text and line
limits, labels, focus order and default). A renderer's test suite reads the
files, builds its views from `expected.document`, describes them in the
normalised shape and compares with `expected.layout`. The
[README](../pkg/sdui/conformance/README.md) says how, for Swift, Kotlin and
TypeScript. A renderer conforms when every fixture passes.

## Navigation

`GET /api/v1/sdui/navigation` answers the main menu for the profile and
device:

```json
{ "version": 1, "formFactor": "phone", "etag": "W/\"nav-…\"", "items": [
  { "id": "home", "label": "Home", "icon": "home", "kind": "page", "page": "home", "route": "/", "placement": "primary" },
  { "id": "search", "label": "Search", "icon": "search", "kind": "page", "page": "search", "route": "/search", "placement": "primary" },
  { "id": "lib-1", "label": "Films", "icon": "movie", "kind": "page", "page": "library", "params": { "library": "1" }, "route": "/library/1", "placement": "more" },
  { "id": "you", "label": "You", "icon": "user", "kind": "native", "screen": "profiles", "route": "/you", "placement": "primary" },
  { "id": "settings", "label": "Settings", "icon": "settings", "kind": "native", "screen": "settings", "route": "/settings", "placement": "more" } ] }
```

- `placement` says where each entry sits: `primary` in the bar itself (a
  phone's tab bar, a television's top menu, the web app's sidebar), `more`
  behind More (the sidebar's lower group), `end` pinned to the bar's far end.
  A bar holds only so many: past 5 on a phone, 7 on a television and 8 on a
  tablet, primary entries are sent as `more`.
- A `page` entry is opened by asking for `pages/{page}` with `params`; a
  `native` entry opens the app's own screen named by `screen`.
- Entries are authored as `{id, type, page | library | screen | route,
  label, icon, when, placement}`. `type` is `page`, `libraries` (one entry
  per library the profile can open, IDs `lib-<id>`), `library`, `native` or
  `route`. `placement` maps a form factor to a placement or `hidden`.
- **Settings is always there.** It can be moved; it can't be hidden, given a
  condition or left out: a navigation without it gets it at the end.
- A profile's own order and hidden entries (`navigation/personal`) apply
  here as they do to a page. Settings can't be hidden by a profile either.
- For a profile that may rearrange, the answer carries `personal`, in the
  shape a page's has: `{editable, allow: ["reorder", "hide"], sections,
  url}`. `sections` are all the entries the profile could have, in its
  order, hidden ones included and marked, which is what "Arrange the menu"
  lists; the body it PUTs to `url` is `{order, hidden}`.

The built-in navigation: Home, Search, Live TV (where the profile may watch
it), the libraries (behind More on a phone; one "Libraries" entry on a
television), Collections, You and Settings.

## Personalisation

A profile may arrange a page for itself. `document.personal` tells an app
everything it needs to offer "Customise" without knowing the page:

```json
"personal": { "editable": true, "allow": ["reorder", "hide", "pin", "density", "cardSize", "defaultTab"],
  "sections": [ { "id": "libraries", "title": "Your libraries", "pinned": true },
                { "id": "hero", "title": "Featured", "hidden": true },
                { "id": "onnow", "title": "On TV now", "empty": true },
                { "id": "setup", "title": "Notice", "locked": true } ],
  "density": "compact", "url": "/api/v1/sdui/pages/home/personal" }
```

`sections` are the page's top-level sections in the profile's order, hidden
ones included so they can be shown again; `empty` ones have nothing in them
now and aren't on the page. The app shows that list, lets the person drag,
hide and pin what isn't `locked`, and PUTs the result to `url`:

```json
{ "order": ["shuffle", "recent-1"], "hidden": ["hero"], "pinned": ["libraries"],
  "density": "compact", "cardSize": { "recent-1": "l" }, "defaultTab": { "views": "artists" } }
```

- `GET`, `PUT` and `DELETE` on `pages/{page}/personal` and
  `navigation/personal`. Only ever the caller's own.
- Who may: any profile whose **`editLayout`** feature switch is on (it is by
  default; whoever manages a profile can turn it off with the other
  parental switches: [PROFILES.md](PROFILES.md)), unless the server's
  `personalisation.disabled` is set. Otherwise `editable` is false, `PUT`
  answers 403 and what was stored is ignored, not deleted.
- A page says what may be changed with `"personal": {"allow": [...]}`; an
  empty list is nothing. A section with `"locked": true` can't be hidden or
  moved.
- `density` is `compact`, `comfortable` or `spacious`: spacing at three
  quarters, as the device has it, or at five quarters. `cardSize` is `s`,
  `m` or `l` for a section.
- The arrangement lives with the profile's other interface choices (the
  same store `/api/v1/me/ui` reads, under `sdui`) and goes when the profile
  does. Home's order and hidden rows from before pages were server-driven
  carry over: the built-in Home's sections have the same IDs.

## Profiles with limits

Titles are one half of a profile's limits ([PROFILES.md](PROFILES.md)), and
every data source follows every kind of them: libraries, an age for each kind
of media, genres hidden or the only ones allowed, tags, collections, single
titles. A library, title or collection asked for by number that the profile
may not open is 404, a games library included when games are off for it; so
is a section's own page of items.

The other half is what a profile may do. For a profile that has been kept
from something, the server answers `feature` and `module` conditions as if
the thing weren't on the server at all, `can` conditions say no, and the data
sources and buttons that belong to it answer nothing. A page needs no
conditions of its own for this, and one an administrator wrote is covered
like the built-in ones.

| For the profile | Conditions that are false | And |
|---|---|---|
| Games off | `can: games`; features `game-metadata`, `game-controller-profiles`, `game-settings`, `custom-game-cores`, `game-rigs`, `standalone-emulators`; modules `wittyplay`, `wittycouch`, `wittyrigs` | Games libraries are in no page, row or navigation entry; `continuePlaying` is empty. |
| Live TV off | `can: liveTv`; features `live-tv`, `live-tv-stream-tokens`, `tv-favourites`, `tv-reminders`, `recordings`; modules `wittytv`, `wittytuner` | `liveNow` and channel search are empty; the built-in Live TV entry is not in the navigation. |
| Recording off | `can: recordings`; feature `recordings` | `recordings` is empty. |
| Watch parties off | `can: syncplay`; feature `syncplay` | No **Watch party** button. |
| Playing on other devices off | `can: remoteControl`; feature `remote-protocol` | No **Play on…** button. |
| Downloads or sharing off | `can: downloads`, `can: sharing` | No **Download** or **Share** button. |
| Rearranging pages off | `can: editLayout` | `personal.editable` is false; see [Personalisation](#personalisation). |
| Screen time over | feature `playback`; the flag `screenTimeUp` is true | See below. |

All of this is decided after anything the client sent: a client can't claim a
feature back through `capabilities`, nor set or clear `screenTimeUp`. A
profile's own arrangement can't bring anything back either: it only orders,
hides and pins sections that are on the page for that profile.

**When screen time is over**, pages still render, so the profile can browse,
but nothing in them offers to play: no **Play**, **Resume** or **Play from the
start** button, no `play` action on a card, track or banner, no `playId`, and
no `playRoute` on Top Shelf. The document carries `screenTime`: `{reason,
message, nextAt}` (when there is time again, Unix seconds) for an app to show,
and a section with `"when": {"flag": "screenTimeUp"}` is shown. The player is
the app's own screen, so there is no page to refuse: the playback routes
answer 403 with `code: "screen-time"`, as they do for any app. While there is
time left nothing in a page changes with the clock, so its ETag holds; ask
`GET /me/screen-time` for the time left.

The feature `sdui-limits` in `/capabilities` says a server does all this.

## How a row chooses its cards

A section whose `cardStyle` is `auto` (or left out) takes its source's usual
style, with two refinements the server makes so every app gets them:

- A source that gives whatever a library holds (`recentlyAdded`, `browse`,
  `random`, `search`, ...) on a library of music, audiobooks or podcasts draws
  `album` cards, on books `book`, on games `game`, on photos `photo`: the
  library is the section's `$each` or the page's own `$library`. A cover is
  never stretched into an upright poster.
- On a phone, a row of the libraries (`libraries` source) drawn as `tile` is
  sent as `chip`: each library's icon and name in a pill, a row a phone can
  take in at a glance.

A title's page (`detail`) on a phone or a television carries no `personal`:
nothing there offers Customise. A profile's arrangement of the page, made on
a bigger screen, still applies.

## Books, statistics and likes

Three sources fill pages that otherwise only the web app had, so a television
or a phone draws them with the same Shelf, Grid and cards as everything else.
No renderer needs to know them.

**A books library's shelf.** `books` is the bookshelf WittyInk draws on the
web: a books library's books, narrowed to a `series`, an `author` or a
`format` (`epub`, `pdf` or `cbz`: what each reader opens), by `title`,
`author` (the books with none last), `added` (newest first), `year` (latest
first) or `series` (each series in its own order, the books in none last);
`desc` turns any of them round. With a series and no order, the series' own
order. A book's series is its `album` and its place in it its `track`; its
author is its album artist, else its artist, as the scanner reads them from
the book. To give the shelf rows, repeat a section over `bookSeries` or
`bookAuthors`: one copy per series or author of the page's books library (so
only on a library page), by name, with `$each` its name and `{each}` the
title. The books in no series, or with no author, come last, as
`"~none"` (`$each` then stands for it, and a source may name it outright),
titled "No series" or "No author". There are as many copies as the device's
budget for copies allows ([Device profiles](#device-profiles)), the first by name. The built-in books library page has tabs
for all three: every book, each series, each author.

A series' name is free text, and two can be the same once cut down to what a
node ID may hold, so each copy's ID ends with a hash of the name rather than
the name itself (`in-series-1f0c4a7d2b9e3c11`). Only the server uses it, to
find the copy again for its next page.

`GET /music/albums` and `GET /music/artists` answer a books library too: its
series (each with its author and how many books) and its authors. They used
to answer nothing for one.

**Statistics.** `statistics` adds up the play log, most time first: the
titles (`stat: titles`), the shows with their episodes together (`shows`) or
each library (`libraries`, as tiles with how many titles), over the last
`week`, `month` (the default), `year` or `all` time, in one `library` or
all. Each card's subtitle says how long, `3 h 20 min`. A profile is shown its
own record. `"who": "everyone"` is the whole server's, with how many people
watched each title, and is answered for administrators only: for anyone else
it is empty, so the section is left out. It names and counts only what the
profile may open now: a title it has since been kept from is neither shown
nor added to a library's time. Nothing leaves the server.

**Likes.** `liked` is what the profile gave a thumbs up (the like button on
songs, videos and any title), most recently liked first, narrowed by
`media` like `continueWatching`. It is the server's favourites; a TV
channel's favourites are `liveNow`'s `favourites`.

**What there is no source for.**

- *Playlists* are collections: a list someone made or a rule (the server's
  smart playlists). `collection` gives one, `collections` all of them, and a
  row can be repeated over them. There is no separate playlist store.
- *Cast and crew.* The library keeps no people: a film's metadata keeps its
  director as one line of text (in `title_meta`), and nothing of its cast.
  The metadata lookups read the credits only to find that director. A
  `people` source (an item's cast, the titles with someone in them) needs
  people stored with each title, and every title looked up again to fill
  them, so it isn't offered rather than made up from one line of text.

## Cast and crew

Catalogue revision 5 adds who is in a title and who made it. Every built-in
film, show and episode page has a `Credits` section right under its header
(under the synopsis), and a person's page shows their picture, dates,
biography and their titles in this library.

**The `Credits` component** draws the `credits` source. Its children are
person cards (style `person`), in this order: the cast in billing order, the
guest stars, then the crew by department in the catalogue's order
(`CreditGroups`: Directing, Writing, Production, Camera, Lighting, Editing,
Sound, Art, Costume & Make-Up, Visual Effects, Crew). Each card's `group` is
the part it is in (`cast`, `guests`, or a department's key), its `subtitle`
the part played or the job ("Production Design", "Director, Screenplay"; a
season's guest star "Diver · 3 episodes"), its action opens `/person/<id>`.
A person without a picture has no `image`: draw their initials on token
`bg3`, hidden from assistive technology. `props.groups` lists the parts in
order, `{id, label, role, total, shown}`; at first `castLimit` (20) of the
cast and of the guests and `crewLimit` (6) of each department are sent, and
when any part holds more, `props.more` is `{url, label: "Show all"}`: its
answer has everyone, with `replace` true, in place of the cards you have.
`parts` and `departments` choose what is shown.

Where the people come from, and what a profile may see: the source reads a
title only after the title itself was found visible to the profile, and a
season's guest stars only from the episodes it may see. On an episode's page
the cast is its show's regulars, the guests the episode's own, and the crew
the episode's own where it names a department (the show's otherwise). On a
show's page opened with `?season=<n>` (a season's page: the `$season`
variable) the season's guest stars come after the cast, most episodes first.
See [METADATA.md](METADATA.md#cast-and-crew) for the sources.

**A person's page** is the detail page `detail-person` (`match.mediaKind`
`person`), at the route `/person/{person}` (variable `$person`). Its
`DetailHeader` (layout poster, no buttons) has the person's photo as its
poster, their name as the title, the biography as the overview, and the
facts `knownFor` ("Acting", "Directing"), `born` ("Born 3 February 1970,
Lisbon") and `died`. Under it one row per part of their work: a section
repeated over `personParts` makes one copy per part the person has titles in
that the profile may see (`acting`, then each department), `$each` the
part's key and `{each}` its label, and the `filmography` source gives that
part's films and shows, newest first, each card's subtitle the year and the
part or job ("2010 · as The Boss", "2004 · Director", "2019 · as Diver · 3
episodes"). A person credited only on titles the profile may not see has no
page (404), as a hidden title has none.

**Search** finds people: the `search` source with `in` `people` gives person
cards for names matching the query among people credited on titles the
profile may see, the most titles first, each subtitled with what they are
known for and how many titles ("Acting · 3 titles"). The built-in search
page has a People row.

## Experiences

An experience is a whole look of an app: where its menu is and how it is drawn, the
corners, finish, type set, density and motion it wears, how its Home starts, and a set
of changes to pages that the server makes itself. There are fourteen, the same on every
app: `lumen` (the default), `rail`, `stage`, `aero`, `classic`, `classicCable`,
`classicSatellite`, `compact`, `paper`, `marquee`, `crossbar`, `bigpicture`,
`dashboard` and `retro`. `GET /sdui/experiences` has each as data (the reference table
[Experiences](#experiences-1) below is written from the same files,
`pkg/sdui/experiences/*.json`).

**`wire`: the layout a chooser draws.** Each experience in `GET /sdui/experiences` has `wire`, a map from form
factor (`tv`, `phone`, `tablet`, `desktop`, `web`, `headset`) to the layout in a few words, so a picker shows the
layout a person is choosing and not only its colours (the existing `preview` is the colour swatch). It is worked
out from the experience's own data (its menu on that form factor, its page changes, its effects), never written
in its file, and it is what the design canvas's wire thumbnails draw. An optional, additive property: no
revision change. Each is `{nav, open?, style, home, cards, dock?, brand, finish}`:

| Field | Values and where they come from |
|---|---|
| `nav` | `top` (topTabs, floatingGlassBar, cableBar, bevelBar, teleKeys, tvTopMenu), `side` (sidebar, tvSideMenu), `rail` (rail, railDrawer), `bottom` (bottomTabs, ornamentTabs), `none` (crossbar: the categories are part of the page) |
| `open` | true when the menu is drawn open all the time (a sidebar) |
| `style` | how the entries are drawn: `pill`; `gloss` (floatingGlassBar), `cable`, `bevel`, `tele`, `crossbar`; `under` (a top bar whose headings are underlined, Paper's) |
| `home` | `cross` (pages.crossbar), `widgets`, `list` (rowsAsLists), `tiles` (Home rows tiles), `banner`, `carousel`, else `hero` |
| `cards` | `poster`, `landscape`, `plate`, `block` or `logo`: the page's cardStyle, else the shape's cardSet (a tile is drawn as landscape) |
| `dock` | a dock of keys: on a television and in a headset |
| `brand` | where the server's name sits: `left` of a top bar, `top` of a side menu, rail or tab bar, `centre` for a crossbar |
| `finish` | `crt` (teleKeys), else `gloss`, `bevel` or `glass` when that effect is full, else `flat` |

On a desktop window the fourteen are: Lumen top/hero, Rail rail/banner, Stage top/hero, Aero top (gloss)/hero,
Classic side (open)/hero, Classic Cable top (cable)/list, Classic Satellite top (bevel)/hero, Compact rail/list,
Paper top (under)/banner, Marquee top/carousel, Crossbar none/cross, Big picture top/tiles, Dashboard
rail/widgets, Retro top (tele)/list. Compact and Dashboard are a side menu on a television. Conformance:
`pkg/sdui/conformance/experience-wire-table.json` lists every wire.

**An experience is not a theme.** Colour belongs to the theme (see
[THEMES.md](THEMES.md)); each experience only names the theme it wears unless one is
required or chosen, and the WittyTV look it suggests. Any experience can wear any
theme, built in or imported.

**What decides it.** In order: what an administrator *requires* of the profile; the
profile's own choice for the device; the administrator's default for the form factor;
`lumen`. Requirements (`PUT /sdui/config/experience`, body `{default, allowed,
required: {everyone, kids, profiles: {"<id>": ...}}, profileThemes}`) are `{experience,
theme}`, either part or both, for everyone, for every kids profile or for one profile;
the most specific wins part by part. The server stamps each with who set it and when.
A required look is sent finished: `GET /sdui/experience` answers `choice.required`
(`{value, by, at, scope}`) and `editable: false`, `PUT /sdui/experience` answers 403, and
the theme engine's `look` has `requiredExperience` and `requiredTheme`, so every client
shows it locked ("Required by Willow"). `allowed` limits what profiles may pick and never
overrides a requirement. Each profile chooses its own for each device
(`PUT /sdui/experience {experience}`; `DELETE` to follow the server again). An app names
itself with `device=` (or `X-WMS-Device`): a stable name of its own, letters, digits, `-`
and `_`, never anything that identifies the hardware. `experience=` (or `X-WMS-Experience`)
on a page, the navigation or the theme draws that one request with another experience, so
an app can show a look before it is chosen; a requirement wins over it.

**What a client is sent.** Every page carries `layout.shell` and the navigation carries
`shell`: `{experience, nav, overlay, size, open, drawer}`. `nav` is the menu's style
(`sidebar`, `rail`, `railDrawer`, `bottomTabs`, `topTabs`, `floatingGlassBar`, `cableBar`,
`bevelBar`, `teleKeys`, `crossbar`, `ornamentTabs`, `tvTopMenu`, `tvSideMenu`), `size` its
breadth in points, `overlay` that it lies over the top of the page, `open` how wide it is
while open (a rail's full menu, a television's side menu) and `drawer` the width of a
rail's library drawer beside it (`railDrawer` only: choosing a library opens a second
column headed by the library's name, listing its views). An app draws its frame from the
navigation's shell and lays the page out in what is left; it never picks a style of its
own and never switches on an experience's id. The navigation also carries `experiences`:
`{current, chosen, options, url, required}`, for an **Experience** row in the app's own
Settings (Settings is never SDUI). On a headset (`formFactor=headset`, `platform=visionos`
or `quest`) the form factor is `headset`: ornament tabs, 60-point targets, nothing at the
edges.

**Placements.** Everything in the shell is movable, by data. The shell is made of regions
(`brand`, `nav`, `search`, `profile`, `clock`, `dock`, `legend`, `hero`), each with a
`position` and `visible` (and the hero a `style`); the experience sets them, the
administrator overrides them for everyone (`placements` in the policy, by experience) and a
profile for its own device (`PUT /sdui/placements`), and the server sends the merged result in
`layout.shell.placements`, so a renderer only draws it. The menu and You are locked: they can't
be hidden, so Settings and the way out are always there. Page sections are arranged by the
page's own personalisation (reorder, hide, pin, density, card size). The layout editor is each
app's own and drives these routes and `personal`.

**What the server does for the app.** An experience's changes to pages are made in the
document with the catalogue's own components: a renderer that follows the document draws
every experience with nothing more. Where it asks, a Hero becomes a `Carousel` (`marquee`),
Home becomes a `CrossBar` of its rows (`crossbar`) or a `WidgetGrid` (`dashboard`), rows
and grids become lists (`compact`, `classicCable`, `retro`), films' and shows' cards take
the experience's card style (`plate`, `landscape`, `tile`, `block`), headings take its
heading style (`band`, `underline`) and See all becomes a row's last card. The conformance
kit has a case for every experience (`exp-*.json`).

The theme engine's `look` carries the rest an experience brings: `effects` (glass, gloss,
glow, texture and bevel, each `none`, `subtle` or `full`), `shape`, `type` set, `density`,
`motion`, the WittyTV look it suggests, and the theme's `finish` and `scene`.

## Modules

Every module carries its part of the apps in its own release, as data: its
pages, rows on the built-in pages, named sections of its own, the data
sources its helper answers, views of its own for `NativeView`, menu entries,
settings and theme scope. The server checks them against this catalogue
before a module is used and merges them in: a module's pages come after the
administrator's and before the built-in ones, its rows go only into built-in
pages, its menu entries sit before Settings unless the administrator placed
them, and its components reach an app as the catalogue's own. An app draws a
module's work with nothing of the module's. [SDUI for modules](SDUI-MODULES.md)
is the whole contract.

## Theme scopes

Colours come from the theme engine ([THEMES.md](THEMES.md)); a page never
holds one. A page or a section may say which **scope** it belongs to:

```json
{ "id": "onnow", "component": "Shelf", "scope": "wittytv", "source": { "type": "liveNow" } }
```

A scope is a module's ID (`wittytv`, `wittymusic`, `wittyplay`) or a name
of the administrator's own (`family`, `evening`). The colour overrides set
for it with `PUT /api/v1/theme/modules/{scope}` (the theme engine's module
overrides, which accept any well-formed name) are sent as the node's
`tokens`, and as `document.theme.tokens` for a page's scope. Items carry
their own scope too: a channel's card is in `wittytv` wherever it appears.
A scope with no overrides changes nothing.

## Text placeholders and Markdown

Titles and text may hold [placeholders](#placeholders): `{user}`,
`{greeting}`, `{library}`, `{each}`, `{title}`. The server fills them in.

A `Text` with `"format": "markdown"` is sent as written and drawn from this
subset, and nothing else: `**bold**`, `*italic*`, `` `code` ``, `#` to `###`
headings, `-` lists, `1.` lists, blank-line paragraphs, and `[text](to)`
where `to` is a route in the app or an `http(s)` address (opened in the
browser; dead where there is none). Raw HTML is shown as the characters it
is. The validator refuses a link that is neither.

## HTTP API

Every route is under `/api/v1/sdui`. `sdui`, `sdui-diff`, `sdui-pages`,
`sdui-catalog`, `sdui-navigation`, `sdui-personal` and `sdui-editor` in
`/capabilities` `features` say a server has them, with `sdui-data`,
`sdui-form-factors`, `sdui-topshelf` and `sdui-limits`. Route by route:
[CLIENT-API.md](CLIENT-API.md#server-driven-ui) and `docs/openapi.json`.

### For every app

| Route | What it does |
|---|---|
| `GET /sdui/catalog` | The catalogue, as data. `ETag`. |
| `GET /sdui/navigation` | The main menu for this profile and device. `ETag`. |
| `GET /sdui/pages/{page}` | A page, rendered. `{page}` is a kind (`home`, `library`, `detail`, `search`, ...) or one page's ID. `ETag`; 304 with `If-None-Match`. 404 for a page, library or title that isn't there or isn't this profile's to see. |
| `GET /sdui/pages/{page}/diff?since=<etag>` | Patches from the render the client has, or the whole document. |
| `GET /sdui/pages/{page}/sections/{section}/items?cursor=` | A page of a section's items. The address comes from the page. |
| `GET`, `PUT`, `DELETE /sdui/pages/{page}/personal` | The profile's own arrangement of a page. |
| `GET`, `PUT`, `DELETE /sdui/navigation/personal` | And of the main menu. |
| `GET /sdui/topshelf` | [Top Shelf](#top-shelf) content for a tvOS extension. |

Which page a route in the app is, and with which parameters, is the
[Routes](#routes) table, also in the catalogue: a renderer needs one router
driven by it, not a screen per route.

Responses carry `Cache-Control: private, no-cache` and an `ETag` that is
the same for every request that renders the same tree. Send it back as
`If-None-Match`.

### Diffs

`GET /sdui/pages/{page}/diff?since=<etag>`, with the same parameters as the
page, answers 304 when `since` is current; `{"etag", "since", "patches":
[...]}` when the server still has the render `since` names (it keeps each
account's last 6, at most 128 in all, 32 or 12 on a small machine; in
memory); `{"etag", "document": {...}}` otherwise. See
[Patch protocol](#patch-protocol).

### The editor's contract

Everything an editor needs is data; it hard-codes nothing about components.

| Route (administrators only) | What it does |
|---|---|
| `GET /sdui/catalog` | Components, card styles, sources, conditions, page kinds, icons, device profiles: each with label, description, icon, category and, for every property, type, label, help, default, choices, range and where it applies. Build the palette and the property forms from it. |
| `GET /sdui/config/pages` | Every page in force with its `source`: `builtin`, `changed` or `custom`. `?target=draft` for the draft's. |
| `GET /sdui/config/pages/{id}` | The page as authored, the built-in page it would reset to, and the validator's issues. |
| `PUT /sdui/config/pages/{id}` | Save one page. |
| `DELETE /sdui/config/pages/{id}` | Reset a built-in page to default, or delete one of your own. 404 for a page that is in neither. |
| `GET`, `PUT`, `DELETE /sdui/config/navigation` | The navigation; DELETE resets it. |
| `POST /sdui/validate` | Check `{"page"}`, `{"navigation"}` or `{"config"}` without saving. With `?target=draft` a navigation may name the draft's pages. |
| `GET /sdui/presets`, `POST /sdui/presets/{id}/apply` | The gallery; apply a page preset. |
| `GET`, `PUT`, `DELETE /sdui/draft`, `POST /sdui/draft/publish`, `POST /sdui/draft/preview` | The draft. |
| `GET /sdui/export`, `POST /sdui/import` | A page or the whole configuration as a file. |
| `GET`, `PUT /sdui/config`, `GET /sdui/config/history`, `GET /sdui/config/diff`, `POST /sdui/config/rollback/{rev}` | The whole file, its last 10 revisions, what differs between two (`{from, to, fromDraft, toDraft, changes}`; for the draft, the revision it was begun from with `toDraft: true`), and going back. |

**Saving.** A change to the published configuration needs `If-Match` with
the ETag it was made against: 428 without, 412 when someone saved in
between (read again, redo the change). With `?target=draft` the change goes
to the draft instead and `If-Match` is optional.

**Validation.** A refused save answers 400 with every problem:

```json
{ "error": "the UI configuration has 2 problem(s): …", "issues": [
  { "path": "/pages/night/sections/0/props/cardStyle", "severity": "error",
    "message": "\"postr\" is not a choice for cardStyle; the choices are auto, poster, landscape, …",
    "fix": "Did you mean \"poster\"?" },
  { "path": "/pages/night/sections/0/source/library", "severity": "error",
    "message": "$item means nothing on a custom page",
    "fix": "A custom page has no variables; write the value itself." } ] }
```

`path` locates the problem in what was sent; `message` says what is wrong
in words; `fix` says what to do. `severity` `warning` doesn't stop a save
(a grid that isn't last, a repeated row that never uses `$each`).

#### Draft and preview

`ui_config.draft.json` is a second configuration nobody is shown. Any page,
navigation, preset or import route with `?target=draft` writes to it (it
starts as a copy of the published one). `POST /sdui/draft/preview` answers
`{"token", "expiresAt"}`: a random token, good for two hours, **for the
administrator who asked and nobody else**. That administrator's apps send
it as `preview=<token>` (or `X-WMS-SDUI-Preview`) with the page and
navigation routes and are served the draft, with `meta.source: "draft"`, on
a real phone or television. Anyone else sending it gets 403. `POST
/sdui/draft/publish` applies the draft to the configuration, as a new
revision, and ends every preview; `DELETE /sdui/draft` throws it away.

**Publishing merges.** The draft remembers the published revision it was
begun from (`baseRevision`). Publishing does not copy the draft over what is
published: it takes what the draft changed since that revision, item by item
(each page by ID, the navigation, the theme, the personalisation switch), and
applies it on top of what is published now, so a change published after the
draft was begun is kept. When the draft and what was published since changed
the same item in different ways the answer is 409, nothing changes, and the
body names them: `{"error", "conflicts": ["/pages/home", "/navigation"],
"baseRevision", "currentRevision"}`. The editor reloads both, shows them, and
publishes again with `?prefer=draft` (the draft's version of those items
wins) or `?prefer=published` (what is published stays; the draft's other
changes still go in), or edits the draft. When the revision the draft was
begun from is no longer among the 10 kept, `conflicts` is `["/"]` and only
`prefer=draft` publishes (the draft as it is). `If-Match` keeps its meaning:
the draft's ETag.

#### Presets, import and export

`GET /sdui/presets` lists [ready-made pages and sections](#presets). A page
preset is applied in one step (`POST /sdui/presets/{id}/apply`), then
edited like any page; a section preset is a section for the editor to drop
into a page. `GET /sdui/export?page=<id>` (or without `page`, everything)
answers a file `{"format": "wittymedia-ui", "formatVersion": 2, "kind":
"page" | "config", ...}`; `POST /sdui/import` takes one, into the draft
unless `target=published`. `dryRun=1` reports the issues and what would
change, and changes nothing.

### `ui_config.json`

In the data folder. It holds only what an administrator changed:

```json
{
  "version": 2, "revision": 7, "updatedAt": "2026-11-11T14:00:00Z", "updatedBy": "owner",
  "theme": { "tokens": { "accent": "#FF8800" }, "modules": { "wittytv": { "accent": "#2A6FDB" } } },
  "navigation": { "items": [ { "id": "home", "type": "page", "page": "home" } ] },
  "pages": { "home": { "id": "home", "kind": "home", "title": "Home", "sections": [] } },
  "personalisation": { "disabled": false }
}
```

- **Strict.** Unknown fields and trailing data are refused; at most 16 MiB
  and 200 pages; each key under `pages` is its page's `id`; a page with a
  built-in page's ID must be of the same kind. Every page and the
  navigation go through the validator; theme tokens must be colour tokens
  themes have, with colour values.
- **Revisions.** Every save is a new `revision`, written atomically, with a
  copy of each of the last 10 in `ui_config.history/`. Rolling back makes an
  old one current as a new revision.
- **Edited by hand.** Changing the file while the server runs is fine: on
  the next read it is validated and saved as a new revision (`updatedBy:
  "file on disk"`). One that doesn't validate is kept as
  `ui_config.json.bad` and the last good configuration stays in force.

## How the web client uses it

The web app (`web/src/sdui`), which the desktop app shows unchanged in its
own window, is one of the three renderers.

- **When.** It asks `/capabilities` once per sign-in. With `sdui-pages` it
  resolves every address through the [Routes](#routes) table
  (`routes.ts`): an address that is a page is asked for and drawn; anything
  else is one of the app's own screens. With `sdui-navigation` the side
  menu, the top bar, a phone's bottom bar and TV mode's top menu are drawn
  from `GET /sdui/navigation`, in the shell the app always had. Without
  those features (an older server) it keeps the pages and menu it worked
  out itself.
- **What it says it is.** `platform` `web` (or `desktop`
  inside the desktop app), and a form factor from the window: TV mode is
  `tv` (`deviceClass` by the screen), a window under 600 points is `phone`,
  a touch screen under 1200 is `tablet`, anything else `web` or `desktop`.
  It sends `viewport`, `locale`, `capabilities` (`reducedMotion` from the
  system setting) and `views=guide,photoTimeline,photoMap,coverFlow`, and
  asks again when the window crosses a breakpoint.
- **The layout step.** `model.ts` turns a document into a tree of views:
  what each component draws, its line limits, card sizes, placeholders,
  role and label. The React components (`render/`) take their props from
  those views, and the same step, described in the conformance kit's shape,
  is what `model.test.mjs` runs against every fixture in
  `pkg/sdui/conformance/fixtures`.
- **Sizes.** Card widths, grid columns, section gaps and spacing steps are
  the document's (`props.card`, `layout`), set as CSS custom properties on
  the page; a television's page is laid out on its 1920-point canvas and
  scaled to the window. Type sizes follow the app's own scale for each text
  role, which grows with the reader's browser setting. Colours are the
  theme's variables; a node's `tokens` override them for its subtree.
- **Keeping pages.** A page is kept with its ETag and shown at once on the
  way back to it, then asked for through `/diff?since=`; patches are
  applied to the kept tree (`patch.ts`), and anything that doesn't apply
  drops it. It is asked for again after `meta.ttlSeconds` while the tab is
  visible, and after a command or a change to the profile's arrangement.
  Rows that had been filled or paged stay filled when the page comes again.
- **Focus.** Tab order is the document's order. In TV mode, and with a
  game pad or arrow keys anywhere, `focus.ts` decides where an arrow goes
  from `focus.order` and the groups: along a row and stopping at its ends,
  by row and column in a grid with the column count as drawn, up and down
  to the previous or next stop and the member it was left on. Focus starts
  on `focus.default`.
- **Actions.** `navigate` is a link; `play` asks what the item is and
  starts the app's own player for it (video, disc with its menus, audio as
  the album from that track, reader, game); `native` opens Settings, the
  guide, You, the share dialog, the device picker, a watch party or a
  download; `external` opens a new tab; `command` calls the route in
  [Commands](#commands) and asks for the page again; `selectTab` and the
  filter bar change the address's query, so Back undoes them.
- **Native views.** `guide` is the guide's grid, `photoTimeline` and
  `photoMap` the photo library, `coverFlow` the music library's cover flow.
  A module's view (`<module>.<name>`) is drawn by its web bundle, and the
  app sends the views of the bundles in use with its own
  ([Modules in server-driven UI](SDUI-MODULES.md#what-apps-need-to-do)).
- **Experiences.** The frame follows the navigation's `shell`, whatever the
  experience (see [Experiences](#experiences)). The app names itself with `device=`, a
  random name kept in the browser. Settings, Appearance offers the experiences,
  themes and Import a theme; Settings, Pages & menu, Experiences is the
  administrator's `config/experience`, with the required looks.
- **Modules.** Their settings are under Settings, Module settings, drawn
  with Settings' own rows; their pages of the app's own (`/m/...`) and their
  views take the module's colours where its palette differs from everyone's.
- **Where the app's own screens stand in.** A game's page, a book's front
  page, a photo, a radio station, a creator's channel and the short-video
  feed are not in the model. When the catch-all built-in `library` or
  `detail` page is what the server answers for one of those, the web app
  shows its own screen for it (`native.ts`); a page an administrator wrote
  for such a library or title is drawn as written. Under a title's header
  the app adds what the model has no component for: the credit a metadata
  source asks for, the file's technical facts, a disc's titles and extras,
  and, for administrators, artwork, details and refresh (for a show also
  "Find intros and credits"). Above a show's episodes is the Cards or
  Playlist switch (an administrator's choice is kept for the show).
- **The app's own tools.** Under the title of a library of films or shows:
  Filters with every filter of the app's Filters panel (the page passes
  them on: `series`, `kind`, `disc`, `minRating`, `audio`, `subtitle`,
  `format`, `edition`, `search`, `seed` beside the filter bar's own),
  saving a view and the saved views, and for administrators Select (to
  edit titles together), Categories, Fix matches and Scan. A music library
  has Play all, Songs and Playlists (those two open the app's own music
  page on that tab) and Scan; Collections has New collection; a collection
  has Play all, Edit, Remove titles and Delete (`extras/PageTools.tsx`).
- **Customise and the editor.** `document.personal` drives "Customise this
  page"; the menu's `personal` drives "Arrange the menu" in Settings,
  Appearance. The administrator's editor is Settings, Pages & menu, built
  from `GET /sdui/catalog`. Both are described for the people using them in
  [PAGES.md](PAGES.md).
- **What carried over.** Home's order and hidden rows (`/me/ui` `home`) are
  read by the server. The web app's own menu arrangement (`/me/ui` `nav`)
  is moved once: its order and hidden entries become `navigation/personal`,
  how the menu is drawn (grouping, overflow, folding) stays a choice of the
  web app's own (`navLook`), and the old record is removed.

## Top Shelf

`GET /sdui/topshelf` answers what a tvOS Top Shelf extension shows: what
the profile was in the middle of, then what is new in the first libraries
it may open.

```json
{ "version": 1, "style": "sectioned", "expiresInSec": 900,
  "sections": [ { "title": "Continue watching", "items": [
    { "id": "12", "title": "Film 3", "shape": "wide", "progress": 0.4,
      "image": { "url": "/api/items/12/backdrop?w=908", "width": 908, "height": 512, "aspect": "16/9" },
      "displayRoute": "/item/12", "playRoute": "/watch/12" } ] } ] }
```

At most 4 sections of 10 items: the extension has seconds and little
memory. `shape` is `poster`, `wide` or `square`, the three tvOS has, and
`image` is sized for it (404×608, 908×512, 608×608 points, twice that with
`deviceClass=4k`). Build a `TVTopShelfSectionedContent` from it, with
`displayRoute` and `playRoute` wrapped in the app's own URL scheme. It has
an `ETag`; ask again after `expiresInSec`.

Between those, a module may have rows of its own: a source it declares with
`"topShelf": true` ([SDUI-MODULES.md](SDUI-MODULES.md#top-shelf)) is asked
for 10 items, with no parameters, and becomes a row titled with the source's
`title` (its `label` when the title has a placeholder). At most two such rows,
in the order of the modules and their files; a source that fails or has
nothing gives none, and a module the profile is kept from gives nothing. The
items are checked as on any page. The extension draws them like the rest:
nothing new for it to know.

## Patch protocol

`Diff(old, new) []Patch` and `Apply(doc, patches)` are an ID-keyed tree
diff. Nodes are matched by `id`.

| `op` | `path` | Payload | Meaning |
|---|---|---|---|
| `add` | `#<parentId>/children/<n>` | `node` | Insert `node`, with its subtree, at index `n`. |
| `remove` | `#<id>` | | Remove the node and its subtree. |
| `replace` | `#<id>` | `node` | Replace every field of the node **except `children`**. |
| `replace` | `/root` | `node` | Replace the whole tree. |
| `replace` | `/page`, `/layout`, `/theme`, `/personal`, `/screenTime`, `/catalog`, `/meta` | `value` | Set the document-level field; `null` clears it. |
| `move` | `#<parentId>/children/<n>` | `from: "#<id>"` | Move a child of the same parent to index `n`. |

Patches apply in order; indices refer to the children as they stand after
the patches before. `meta.generatedAt` and `meta.etag` are never in a
patch: the new ETag is the response's. If any step fails (an unknown ID, a
bad index), drop the local tree and ask for the page.

```
for p in patches:
  if p.path starts with "/":  document[field] = p.value   (p.node for /root)
  else:
    (id, index) = parse "#id" or "#id/children/n"
    add:     find(id).children.insert(index, p.node)
    remove:  parentOf(id).children.delete(indexOf(id))
    replace: n = find(id); keep = n.children; n = p.node; n.children = keep
    move:    c = parentOf(p.from).children.remove(p.from); find(id).children.insert(index, c)
```

`Canonical(doc)` is the document as JSON with keys sorted, no white space,
and without `meta.generatedAt` and `meta.etag`; `ETag(doc)` is `W/"sha256-`
+ the first 32 hex digits of its SHA-256 + `"`.

## Performance and limits

- **Only what is on the page is read.** A section's source is one or two of
  the library's indexed queries, asked for one page of items. Sections past
  the device's `eagerSections` aren't asked at all until the client wants
  them, nor are sections after many empty ones.
- **Small payloads.** A television gets 4 filled rows of 12 cards (6 of 20
  on a 4K one) and fetches the rest as they are approached; text is cut to
  the device's limits; pictures are asked for at the size they are drawn.
- **Renders are kept briefly.** The same request from the same profile
  within 3 seconds (a page and its diff, an app starting up) is one render.
  Longer would need a version number for the library's data, which the
  store does not have; `ETag` and 304 make the repeat cheap for the client
  either way. A profile's own change to its arrangement drops its kept
  renders at once.
- **Bounded memory.** Renders kept for diffs, recent renders and preview
  tokens are capped, lower on small machines.
- **Limits on what is authored**: see [Limits](#limits).

## Security

- **No scripts, no web views, no addresses of one's own.** A page can open
  routes on this server, the app's own screens and, in the browser, an
  `http(s)` page. Pictures are paths on this server. Markdown is a fixed
  subset with the same link rule. The validator refuses the rest, and a
  renderer never interprets HTML.
- **Every item is read through the profile's limits**: library, rating,
  title and collection limits, Live TV and feature switches. A page that
  names a library the profile can't open shows it nothing; a page *about*
  something it can't see is not found. This is checked for every built-in
  page, for sections' own pages of items, and for pages an administrator
  wrote to name a library outright.
- **Buttons follow the profile's feature switches**: a profile that may not
  download or share isn't offered either.
- **Configuration is for administrators**, with `If-Match` against lost
  updates. A profile's arrangement is written only by that profile, only
  when its `editLayout` switch allows.
- **A preview token** shows the draft only to the administrator it was made
  for.
- **Caps** on sizes, depth and counts everywhere ([Limits](#limits)).

## Versioning

- `version` (1) changes only for something a client built for it can't
  survive. Clients refuse a version they don't know.
- The **catalogue revision** (8) counts changes to the catalogue. **The rule:
  any change to what an existing component or card style shows or does bumps
  the revision** (a card style that gains badges, a property that changes
  meaning, a component that draws something else). Only a new component, card
  style, source, command, condition, action type or optional property is
  additive, and keeps the revision it came in. Client and server must be built
  for the same revision. Unknown props and actions are ignored, an unknown
  card style is drawn as its shape.
- **Where a client gets the card styles.** From `GET /api/sdui/catalog`, which it
  fetches at sign-in and when the revision changes (the web app does). A table of
  them built into the app is only for offline tests, and a conformance test must
  check it against the catalogue the server serves (see the
  [conformance kit](../pkg/sdui/conformance/README.md)), so a change to what a
  style shows can't be missed.
- `ui_config.json` has its own `version` (2).
- The conformance kit has a `kit` number; a change to a fixture's expected
  output is a change to the contract and is called out in the changelog.

### The music table

`GET /api/sdui/catalog` carries `music` (`pkg/sdui/data/music.json`): the mini player's controls and the visualizer's.
`music.visualizer.presets` are the effects the effects picker offers, the same as WittyVisualizer's `effects/*.glsl`
(the 17 classics and Aurora Rings, in the order of its README): `{id, name, motion, calm, cost}`. `motion` is
`gentle` or `lively`, `calm` is true when calm mode keeps the effect (the gentle ones; it is on by default when the
system asks for reduced motion), `cost` is how hard it works the graphics card (1 to 3). The `preset` control's
default is `skyline`. Every effect takes both of the other two controls, so show them for every effect: `speedPct` (an int, 50 to 200 %, default 100; it multiplies the animation clock) and `colour` (`effect`, the default: each effect's own colour families, or `theme`: a family built from the theme's accent). There is no album-art colour source. They are also the module's settings keys, `speedPct` and `colour`. A new effect in WittyVisualizer is a new entry here.

## Reference

Written from the catalogue (`pkg/sdui/catalog_data.go`) by a test, so it is
what the server does.

<!-- catalogue:begin (written by go test ./pkg/sdui -update; edit pkg/sdui/catalog_data.go) -->
### Components

35 components. Every renderer draws all of them.

| Component | Label | Category | Takes a source | Focus |
|---|---|---|---|---|
| [`Page`](#page) | Page | structure | none | none |
| [`Hero`](#hero) | Spotlight | media | required | stop |
| [`Shelf`](#shelf) | Row | media | required | group |
| [`Grid`](#grid) | Grid | media | required | group |
| [`Card`](#card) | Card | media | none | item |
| [`Tabs`](#tabs) | Tabs | navigation | none | group |
| [`Tab`](#tab) | Tab | navigation | none | item |
| [`Carousel`](#carousel) | Carousel | media | required | group |
| [`CrossBar`](#crossbar) | Cross bar | navigation | none | group |
| [`WidgetGrid`](#widgetgrid) | Widgets | layout | none | group |
| [`Widget`](#widget) | Widget | layout | optional | group |
| [`NumberedRow`](#numberedrow) | Numbered rows | media | required | group |
| [`FilterBar`](#filterbar) | Filter and sort | navigation | none | stop |
| [`Dock`](#dock) | Dock | navigation | none | group |
| [`KeyLegend`](#keylegend) | Key legend | text | none | none |
| [`DetailHeader`](#detailheader) | Title header | detail | none | stop |
| [`SeasonList`](#seasonlist) | Seasons and episodes | detail | required | group |
| [`TrackList`](#tracklist) | Track list | detail | required | group |
| [`Credits`](#credits) | Cast and crew | detail | required | group |
| [`Track`](#track) | Track | detail | none | item |
| [`Banner`](#banner) | Banner | media | optional | item |
| [`Text`](#text) | Text | text | none | none |
| [`Notice`](#notice) | Notice | text | none | item |
| [`Divider`](#divider) | Divider | layout | none | none |
| [`Spacer`](#spacer) | Space | layout | none | none |
| [`Stack`](#stack) | Stack | layout | none | none |
| [`Split`](#split) | Two columns | layout | none | none |
| [`Button`](#button) | Button | navigation | none | item |
| [`Image`](#image) | Picture | text | none | none |
| [`SearchBox`](#searchbox) | Search box | navigation | none | item |
| [`List`](#list) | List | media | optional | group |
| [`ListItem`](#listitem) | List row | media | none | item |
| [`Form`](#form) | Form | structure | none | group |
| [`QR`](#qr) | QR code | text | optional | none |
| [`NativeView`](#nativeview) | The app's own view | structure | none | stop |

#### Page

*Page.* The root of every rendered page. It scrolls down; nothing inside it scrolls down by itself. Made by the server; not something an administrator places.

Holds: any component.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | string, at most 120 |  | Title. The page's heading. |
| `density` | enum: compact, comfortable, spacious | `comfortable` | Density. How tightly the page is packed. |

How it is drawn:

- **Television.** A full-screen column inside the safe area (60 points top and bottom, 90 at the sides), sections 40 points apart. The title isn't drawn: the top menu says where you are.
- **Phone.** A scrolling column under a large title that collapses into the navigation bar; sections 24 points apart, rows bleed to the screen's edges.
- **Tablet.** As a phone, with 16-point side margins (the m spacing, as on every form factor but the television).
- **Desktop and web.** A scrolling column beside the sidebar, the title as a heading, content at most 1600 points wide.

Focus: takes none. Accessibility: Role: main. Label: the title.

#### Hero

*Spotlight.* A large rotating spotlight of a few titles: backdrop, title or logo, a line of facts, a synopsis and buttons.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | text, at most 120 |  | Title. Named to assistive technology; not drawn. |
| `style` | enum: billboard, banner | `billboard` | Style. A full billboard, or a shorter banner. |
| `autoAdvanceSec` | int 0 to 60 | `8` | Seconds per title. How long each title stays before the next one. 0 never moves by itself. |
| `showSynopsis` | bool | `true` | Show the synopsis. Two or three lines about the title. |
| `showLogo` | bool | `true` | Use the title's logo. The title's logo art in place of its name, when it has one. |
| `actions` | enumList: play, details, watchLater | `play, details` | Buttons. The buttons on each title, in order. |
| `maxItems` | int 1 to 8 | `5` | Most titles. How many titles it rotates through. |
| `peek` | bool | `false` | Show the next title's edge. A sliver of the next title (layout.peek of the slide's width) shows at the trailing edge of the spotlight on a phone or tablet, so it is clear there is more to swipe to. A television's buttons row and a desktop's arrows say it already. |

How it is drawn:

- **Television.** The top 620 points of the screen: backdrop full width behind a left-to-right scrim, the logo or title at the heading size bottom-left, facts, at most three lines of synopsis, then the buttons in a row. Left and right on the buttons' row past its ends change title; it never advances while one of its buttons has focus.
- **Phone.** A 16/9 backdrop the width of the screen (2/3 with style billboard), title and facts over a bottom scrim, buttons below; swipe sideways to change title, page dots under it. With peek, the slide is narrowed by layout.peek of its width so the next one's edge shows.
- **Tablet.** A 21/9 backdrop with the text block at the bottom-left third.
- **Desktop and web.** A 21/9 backdrop at most 620 points tall, text block bottom-left, arrows at the edges on hover, page dots bottom-right.

States:

- **empty.** Left out.
- **error.** Left out.
- **loading.** A grey block of the hero's size.

Focus: one stop in the page's focus order; it moves focus among its own parts. Accessibility: Role: region, labelled by the title prop or "Featured". Each slide is a group labelled "<title>, <facts joined by comma>", announced politely when it changes. With reduced motion it does not advance by itself and cross-fades are cuts.

```json
{
  "id": "hero",
  "component": "Hero",
  "source": {
    "type": "featured",
    "limit": 5
  },
  "props": {
    "style": "billboard"
  }
}
```

#### Shelf

*Row.* A titled row of cards that scrolls sideways. The workhorse of a home page.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | text, at most 120 |  | Title. The heading above the items. Placeholders such as {library} and {each} are filled in. |
| `subtitle` | text, at most 160 |  | Subtitle. A smaller line under the title. |
| `headingStyle` | enum: plain, band, underline | `plain` | Heading style. How the title is drawn: plain, a coloured band across the row, or underlined. Left plain, the experience chooses. |
| `cardStyle` | enum | `auto` | Card style. How each item is drawn. Auto uses the style that suits the source. |
| `cardSize` | enum: s, m, l | `m` | Card size. Small, medium or large cards. A profile may choose its own. |
| `showTitles` | bool | `true` | Show titles under cards. Off draws the pictures alone; the titles are still read out. |
| `seeAll` | enum: auto, always, never | `auto` | See all. A link to everything the row is drawn from, when the source has such a place. |
| `numbered` | bool | `false` | Number the cards. Large rank numbers beside the cards, for a top ten. |
| `maxItems` | int 0 to 60 |  | Most cards at first. How many cards are sent before the row asks for more. 0 uses the device's own limit. |
| `peek` | bool | `true` | Show the next card's edge. layout.peek of the next card's width stays visible at the row's trailing edge, so it is clear the row scrolls on. Off cuts the row at a whole card. |
| `seeAllCard` | enum: auto, always, never | `auto` | See all as a card. See all as the last card of the row, in the shape of the row's cards, in place of the link beside the heading. Automatic: on a television and in the experiences that ask for it (Stage, Big picture). |
| `focusGrow` | bool | `false` | The focused card grows. The card with focus scales up (1.45 times on a television, 1.25 on a tablet or desktop, not at all on a phone, where the first card is simply larger) and the cards after it step aside; the others stay small. With reduced motion the card does not animate: it gets a ring. |
| `detail` | bool | `false` | Detail panel under the row. A panel under the row shows the focused card's item (the first until focus moves): its title large, its subtitle as one line, and its actions as buttons carrying their key glyphs. An item with no actions has its route as the one A action. |
| `focusable` | bool | `false` | Cards without a route take focus. A card with no route (a friend) is still a stop for focus, and OK on it is the page's to use. Off: it is not a button and takes no focus. |
| `empty` | enum: hide, message | `hide` | When there is nothing to show. Leave the section out, or keep it with a message. |
| `emptyText` | text, at most 200 |  | Message when empty. Shown in place of the items when there are none. |
| `emptyHint` | text, at most 300 |  | Hint when empty. A line under the message that says what to do about it: "Add a games library in Settings." |
| `emptyAction` | object |  | Button when empty. One button under the message: {label, and one of to (a route), native, op (a command, with item and params and confirm); optional as: layer, and when: a condition such as {"admin": true}, so only administrators are offered it}. |

How it is drawn:

- **Television.** A heading at the title size, then one row of cards that scrolls sideways under focus (cards layout.spacing.m apart, the next card peeking in by layout.peek of its width), the focused card kept a safe-area's width from the left edge and raised with the system's focus effect. The row is one focus section and remembers its last card. With props.seeAllCard, "See all" is a final card of the row's shape (its label in the title style, an arrow icon, the same corner), reached by moving past the last title; otherwise it is a link beside the heading.
- **Phone.** A heading with a chevron when there is a See all, then a row that scrolls by touch and bleeds off both edges; cards 12 points apart.
- **Tablet.** As a phone with 16-point gaps.
- **Desktop and web.** A heading with a "See all" link at its end, a row that scrolls by wheel, drag or the arrow buttons that appear at its ends on hover; arrow keys move focus card to card.

States:

- **deferred.** Drawn as loading until the client has asked props.more.url, which it does when the section is within one screen of being seen.
- **empty.** Left out of the page; with empty "message", the heading and emptyText in the caption style where the items would be.
- **error.** The heading and one line, "Couldn't load this", with a Retry button that asks props.more.url again.
- **loading.** Its heading, and as many grey placeholder cards of the card's size as fit, pulsing unless the device asks for reduced motion.

Focus: its children take focus, and it is their group. Accessibility: Role: list, labelled by the title. Each card is a list item. When numbered, a card's label starts with "Number <n>".

```json
{
  "id": "recent",
  "component": "Shelf",
  "props": {
    "title": "Recently added in {each}"
  },
  "repeat": {
    "over": "libraries"
  },
  "source": {
    "type": "recentlyAdded",
    "library": "$each"
  }
}
```

#### Grid

*Grid.* Cards in rows and columns, as many columns as fit. It pages as the person scrolls: the body of a library.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | text, at most 120 |  | Title. The heading above the items. Placeholders such as {library} and {each} are filled in. |
| `headingStyle` | enum: plain, band, underline | `plain` | Heading style. How the title is drawn: plain, a coloured band across the row, or underlined. Left plain, the experience chooses. |
| `cardStyle` | enum | `auto` | Card style. How each item is drawn. Auto uses the style that suits the source. |
| `cardSize` | enum: s, m, l | `m` | Card size. Small, medium or large cards. A profile may choose its own. |
| `showTitles` | bool | `true` | Show titles under cards. Off draws the pictures alone. |
| `maxItems` | int 0 to 200 |  | Most cards at first. How many cards the first page holds. 0 uses the device's own limit. |
| `focusable` | bool | `false` | Cards without a route take focus. A card with no route (a friend) is still a stop for focus, and OK on it is the page's to use. Off: it is not a button and takes no focus. |
| `empty` | enum: hide, message | `hide` | When there is nothing to show. Leave the section out, or keep it with a message. |
| `emptyText` | text, at most 200 |  | Message when empty. Shown in place of the items when there are none. |
| `emptyHint` | text, at most 300 |  | Hint when empty. A line under the message that says what to do about it: "Add a games library in Settings." |
| `emptyAction` | object |  | Button when empty. One button under the message: {label, and one of to (a route), native, op (a command, with item and params and confirm); optional as: layer, and when: a condition such as {"admin": true}, so only administrators are offered it}. |

How it is drawn:

- **Television.** layout.gridColumns columns for the card's shape (6 posters, 4 landscape cards), 40 points apart, focus moving by row and column; the next page is asked for when focus is within two rows of the end.
- **Phone.** As many columns of the card's width as fit, at least 2 (3 for posters at size s or m), 12 points apart.
- **Tablet.** As many columns as fit, 16 points apart.
- **Desktop and web.** As many columns as fit: floor((width + gap) / (card width + gap)), cards stretched to fill the row; the next page is asked for when the end is within one screen.

States:

- **deferred.** Drawn as loading until the client has asked props.more.url, which it does when the section is within one screen of being seen.
- **empty.** Left out of the page; with empty "message", the heading and emptyText in the caption style where the items would be.
- **error.** The heading and one line, "Couldn't load this", with a Retry button that asks props.more.url again.
- **loading.** Its heading, and as many grey placeholder cards of the card's size as fit, pulsing unless the device asks for reduced motion.

Focus: its children take focus, and it is their group. Accessibility: Role: grid (list on touch devices), labelled by the title or the page's title.

```json
{
  "id": "all",
  "component": "Grid",
  "source": {
    "type": "browse",
    "library": "$library",
    "filterable": true
  }
}
```

#### Card

*Card.* One item in a Shelf, Grid, Hero or SeasonList. Made by the server from the source's items; see the card styles for what each style shows. Made by the server; not something an administrator places.

| Property | Type | Default | What it is |
|---|---|---|---|
| `style` | string |  | Style. The card style. |
| `shape` | string |  | Shape. poster, landscape, square, circle or chip: what a renderer that doesn't know the style draws. |
| `title` | string |  | Title. Always sent, and always in the label, even where the style doesn't draw it. |
| `subtitle` | string |  | Subtitle |
| `synopsis` | string |  | Synopsis. Episode cards and hero slides. |
| `remaining` | string |  | Time left. Progress cards: "42 min left". |
| `itemId` | string |  | Item. The item's ID. |
| `kind` | string |  | Kind. The item's kind. |
| `playId` | string |  | Plays. What a play affordance on the card starts, when the item can be played. |
| `image` | object |  | Picture. {url, width, height, aspect, kind}: the address with the width to ask for. |
| `progress` | number |  | Progress. 0 to 1. |
| `watched` | bool |  | Watched |
| `unwatched` | int |  | Unwatched count |
| `badges` | list |  | Badges. At most three short labels. |
| `rank` | int |  | Rank. In a numbered row. |
| `number` | string |  | Channel number |
| `now` | object |  | On now. {title, start, end} |
| `next` | object |  | On next. {title, start, end} |
| `icon` | icon |  | Icon. For tiles and chips without a picture. |
| `count` | int |  | Count |
| `facts` | list |  | Facts. Hero slides: [{id, value}]. |
| `actions` | list |  | Buttons. Hero slides: [{id, label, icon, primary, action}]. |
| `logo` | object |  | Logo. Hero slides: the title's logo art. |

How it is drawn:

- **Television.** The picture at the row's card size with the system focus effect (lift, parallax on tvOS; scale 1.08 and a 3-point ring in token accent elsewhere); the title appears under the focused card only, unless showTitles.
- **Phone.** Picture, then title and subtitle under it in the title and caption sizes. A long press opens the app's own item menu.
- **Desktop and web.** Picture with a hover scrim and play button when it can be played, title and subtitle under it; a context menu on right click.

States:

- **error.** The picture's place filled with token bg3 and the item kind's icon.
- **loading.** A grey rounded rectangle of the card's shape.

Focus: the node takes focus when it can be acted on. Accessibility: Role: button (link on the web). Label: the props its style shows, in the style's order, joined by ", ": progress as "<n> percent watched", watched as "Watched", unwatched as "<n> unwatched", badges as written, a channel as "Channel <number>, <title>, now <now.title>, next <next.title>".

#### Tabs

*Tabs.* A strip of tabs, each with its own sections. Only the selected tab's content is sent; choosing another asks for the page again with tab=<id>.

Holds: `Tab`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `style` | enum: tabs, segments | `tabs` | Style. Tabs for sections of a page, segments for views of the same thing. |
| `default` | section |  | Opens on. The tab shown first. A profile may choose its own. |

How it is drawn:

- **Television.** A row of pills centred above the content; left and right move between them and the content changes as focus moves; down enters the content.
- **Phone.** A segmented control (segments) or a scrolling row of text tabs with an underline (tabs); swiping the content sideways changes tab.
- **Desktop and web.** Text tabs with an underline in token accent, left-aligned; arrow keys move between them, Enter selects.

Focus: its children take focus, and it is their group. Accessibility: Role: tablist; each Tab is a tab with selected state; the content is its tabpanel.

```json
{
  "id": "views",
  "component": "Tabs",
  "children": [
    {
      "id": "albums",
      "component": "Tab",
      "props": {
        "label": "Albums"
      },
      "children": [
        {
          "id": "albums-grid",
          "component": "Grid",
          "source": {
            "type": "albums",
            "library": "$library",
            "sort": "title"
          }
        }
      ]
    },
    {
      "id": "artists",
      "component": "Tab",
      "props": {
        "label": "Artists"
      },
      "children": [
        {
          "id": "artists-grid",
          "component": "Grid",
          "source": {
            "type": "artists",
            "library": "$library"
          }
        }
      ]
    }
  ]
}
```

#### Tab

*Tab.* One tab of a Tabs section: a label and the sections shown when it is chosen.

Holds: any component.

| Property | Type | Default | What it is |
|---|---|---|---|
| `label` (required) | text, at most 40 |  | Label. The tab's name. |
| `icon` | icon |  | Icon. One of the built-in icons. |

Focus: the node takes focus when it can be acted on. Accessibility: Role: tab. Label: the label.

#### Carousel

*Carousel.* A centred carousel of posters with the focused one large and its neighbours smaller: one big title at a time.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | text, at most 120 |  | Title. The heading above the items. Placeholders such as {library} and {each} are filled in. |
| `headingStyle` | enum: plain, band, underline | `plain` | Heading style. How the title is drawn: plain, a coloured band across the row, or underlined. Left plain, the experience chooses. |
| `cardStyle` | enum | `auto` | Card style. How each item is drawn. Auto uses the style that suits the source. |
| `cardSize` | enum: s, m, l | `m` | Card size. Small, medium or large cards. A profile may choose its own. |
| `showTitles` | bool | `true` | Show the focused title. The focused card's title and facts under it. |
| `maxItems` | int 0 to 40 |  | Most cards at first. How many cards are sent before the row asks for more. 0 uses the device's own limit. |
| `empty` | enum: hide, message | `hide` | When there is nothing to show. Leave the section out, or keep it with a message. |
| `emptyText` | text, at most 200 |  | Message when empty. Shown in place of the items when there are none. |
| `emptyHint` | text, at most 300 |  | Hint when empty. A line under the message that says what to do about it: "Add a games library in Settings." |
| `emptyAction` | object |  | Button when empty. One button under the message: {label, and one of to (a route), native, op (a command, with item and params and confirm); optional as: layer, and when: a condition such as {"admin": true}, so only administrators are offered it}. |

How it is drawn:

- **Television.** A row of cards centred on the focused one, which is drawn at 1.25 times the others with the focus ring; left and right move the row so the focused card stays in the middle; its title, facts and a Play button are under it. The row is one focus section.
- **Phone.** The same, swiped: the card in the middle is large, the next and previous peek in by layout.peek.
- **Desktop and web.** The same, with arrow buttons at both edges on hover and the arrow keys moving the focused card.

States:

- **deferred.** Drawn as loading until the client has asked props.more.url, which it does when the section is within one screen of being seen.
- **empty.** Left out of the page; with empty "message", the heading and emptyText in the caption style where the items would be.
- **error.** The heading and one line, "Couldn't load this", with a Retry button that asks props.more.url again.
- **loading.** Its heading, and as many grey placeholder cards of the card's size as fit, pulsing unless the device asks for reduced motion.

Focus: its children take focus, and it is their group. Accessibility: Role: list, labelled by the title. The focused card is announced with its position, "<title>, <n> of <total>".

```json
{
  "id": "marquee",
  "component": "Carousel",
  "props": {
    "title": "Tonight"
  },
  "source": {
    "type": "featured",
    "limit": 8
  }
}
```

#### CrossBar

*Cross bar.* A horizontal bar of categories with a vertical list under the chosen one, as a game console has. Only the selected category's content is sent; choosing another asks for the page again with tab=<id>.

Holds: `Tab`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `default` | section |  | Opens on. The category shown first. A profile may choose its own. |

How it is drawn:

- **Television.** Icons with names along a bar a third of the way down the screen; left and right move the bar, the chosen category's list opens below it in the title size, up and down move through it, OK opens an entry.
- **Phone.** A scrolling row of icons and names under the title; the list under it is the page.
- **Desktop and web.** The same as the television's, with the pointer choosing a category.

Focus: its children take focus, and it is their group. Accessibility: Role: tablist; each Tab is a tab with selected state; the list under it is its tabpanel.

```json
{
  "id": "cross",
  "component": "CrossBar",
  "children": [
    {
      "id": "films",
      "component": "Tab",
      "props": {
        "label": "Films",
        "icon": "movie"
      },
      "children": [
        {
          "id": "films-list",
          "component": "List",
          "props": {
            "dense": true
          },
          "source": {
            "type": "recentlyAdded",
            "library": "$library"
          }
        }
      ]
    }
  ]
}
```

#### WidgetGrid

*Widgets.* A grid of widgets, each in its own cell: continue watching, what is on TV now, statistics, the server's status, a clock. Every cell can be moved and resized by the administrator and by the profile.

Holds: `Widget`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `columns` | int 1 to 12 | `4` | Columns. How many columns the grid has on a wide screen; narrower windows use fewer. |

How it is drawn:

- **Television.** layout.dock aside, the safe area's width divided into the columns, cells layout.spacing.m apart; focus moves cell to cell by direction, and a cell with items is a row inside it.
- **Phone.** Two columns at most; a widget wider than that takes the full width.
- **Desktop and web.** The columns the grid says, widgets keeping their cells.

Focus: its children take focus, and it is their group. Accessibility: Role: group, labelled by the page's title. Each widget is a region labelled by its title.

```json
{
  "id": "widgets",
  "component": "WidgetGrid",
  "children": [
    {
      "id": "clock",
      "component": "Widget",
      "props": {
        "kind": "clock",
        "x": 0,
        "y": 0,
        "w": 1,
        "h": 1
      }
    }
  ]
}
```

#### Widget

*Widget.* One cell of a widget grid. Its kind says what it shows; kinds that list titles take their items from their kind's source, or from the section's own.

| Property | Type | Default | What it is |
|---|---|---|---|
| `kind` | enum: continue, liveNow, stats, serverStatus, clock, recordings | `continue` | Kind. What the widget shows. |
| `title` | text, at most 60 |  | Title. The widget's heading; left empty it is the kind's own. |
| `x` | int 0 to 11 |  | Column. The cell's column, from 0. |
| `y` | int 0 to 40 |  | Row. The cell's row, from 0. |
| `w` | int 1 to 12 | `1` | Width. Columns the cell spans. |
| `h` | int 1 to 8 | `1` | Height. Rows the cell spans. |

How it is drawn:

- **Everywhere.** A panel at elevation 1 with its title in the heading size. continue, liveNow and recordings hold a row of cards (or a list when the cell is narrow); stats holds the statistics source as a figure or a short list; serverStatus holds a line per figure (libraries, playing now, free space); clock draws the time and date in the device's own format and needs no data.

Focus: its children take focus, and it is their group. Accessibility: Role: region, labelled by the title.

```json
{
  "id": "now",
  "component": "Widget",
  "props": {
    "kind": "liveNow",
    "w": 2,
    "h": 1
  }
}
```

#### NumberedRow

*Numbered rows.* Listing rows with a number column: a channel's number, or the place in the list. A guide's channel list, a top ten.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | text, at most 120 |  | Title. The heading above the items. Placeholders such as {library} and {each} are filled in. |
| `headingStyle` | enum: plain, band, underline | `plain` | Heading style. How the title is drawn: plain, a coloured band across the row, or underlined. Left plain, the experience chooses. |
| `numbers` | enum: auto, index | `auto` | Numbers. Channel numbers where the items are channels, else their place in the list. |
| `maxItems` | int 0 to 100 |  | Most rows at first. How many rows are sent before the list asks for more. 0 uses the device's own limit. |
| `empty` | enum: hide, message | `hide` | When there is nothing to show. Leave the section out, or keep it with a message. |
| `emptyText` | text, at most 200 |  | Message when empty. Shown in place of the items when there are none. |
| `emptyHint` | text, at most 300 |  | Hint when empty. A line under the message that says what to do about it: "Add a games library in Settings." |
| `emptyAction` | object |  | Button when empty. One button under the message: {label, and one of to (a route), native, op (a command, with item and params and confirm); optional as: layer, and when: a condition such as {"admin": true}, so only administrators are offered it}. |

How it is drawn:

- **Television.** A row per item at layout.text.title: the number in a fixed column at the start (as wide as the longest), the logo or picture, then the title and subtitle; the focused row is filled.
- **Phone.** Full-width rows 56 points tall with the number at the start.
- **Desktop and web.** A table: number, title, subtitle; double-click opens.

States:

- **deferred.** Drawn as loading until the client has asked props.more.url, which it does when the section is within one screen of being seen.
- **empty.** Left out of the page; with empty "message", the heading and emptyText in the caption style where the items would be.
- **error.** The heading and one line, "Couldn't load this", with a Retry button that asks props.more.url again.
- **loading.** Its heading, and as many grey placeholder cards of the card's size as fit, pulsing unless the device asks for reduced motion.

Focus: its children take focus, and it is their group. Accessibility: Role: list, labelled by the title. Each row: "<number>, <title>, <subtitle>".

```json
{
  "id": "channels",
  "component": "NumberedRow",
  "props": {
    "title": "Channels"
  },
  "source": {
    "type": "liveNow"
  }
}
```

#### FilterBar

*Filter and sort.* The controls that filter and sort a Grid on the same page. The server fills in each filter's choices from what the library holds; a change asks for the page again with the choice as a query parameter. With showCount, the server sends count (a number) and countText, the words to show ("1 title", "12 titles").

| Property | Type | Default | What it is |
|---|---|---|---|
| `target` | section |  | Filters. The Grid this bar filters. Its source must be filterable. Left out (the search page), the bar filters the page's search by the page's own parameters: every filterable source on it follows them. |
| `fields` | enumList: genre, decade, year, content, resolution, watched, studio, network, tag, service, reading, format, minRating | `genre, decade, watched` | Filters. Which filters are offered, in order. One with no choices in this library is left out. |
| `sorts` | enumList: title, added, newest, oldest, rating, shuffle | `title, added, newest, rating` | Sort by. The orders offered; the first is the default. |
| `showCount` | bool | `true` | Show how many. The number of titles found. |

How it is drawn:

- **Television.** One row of pills: Sort first, then each filter showing its current choice; OK on a pill opens a full-height list from the right edge to choose from.
- **Phone.** A row of chips that scrolls sideways; a chip opens a bottom sheet of choices. The count sits above it in the caption style.
- **Desktop and web.** A row of drop-downs with the count at the far end and a Clear button when anything is chosen.

Focus: one stop in the page's focus order; it moves focus among its own parts. Accessibility: Role: toolbar, labelled "Filter and sort". Each field is a pop-up button labelled "<label>: <current choice or Any>".

```json
{
  "id": "filters",
  "component": "FilterBar",
  "props": {
    "target": "all",
    "fields": [
      "genre",
      "decade",
      "watched"
    ]
  }
}
```

#### Dock

*Dock.* The persistent controls of one screen, in one row that never moves: for a grid, its sort, filters, view and jump to a letter. It is where the screen's options key leads, and it carries the key legend. Put it last on the page.

Holds: `Button`, `KeyLegend`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | text, at most 40 |  | Name. Named to assistive technology: "Options" when left empty. |
| `target` | section |  | Controls. The Grid its generated controls act on, which a FilterBar on the page filters. Without one only its own Buttons are drawn. |
| `controls` | enumList: sort, filter, view, jump | `` | Controls. Generated buttons, in order: sort and filters (when a filter bar drives the target), the view (grid or list) and jump to a letter. One that has nothing to offer is left out. |
| `legend` | bool | `true` | Key legend. A KeyLegend at the end, for the keys of the device in use. |

How it is drawn:

- **Television.** A bar layout.dock.height points tall across the bottom of the screen, inside the safe area, outside the page's scroll: one row of buttons at the title size with the key legend at the trailing end. It never moves and never wraps. It takes focus with Down from the last row of the page, with the options key from anywhere, or with Start; Up returns to the same place. Left and right wrap inside it, the only place that does.
- **Phone.** No bar: layout.dock.placement is sheet. The controls are one "Options" button at the end of the page's header that opens them as a bottom sheet, in the same order; the legend is left out.
- **Tablet.** The same bar, layout.dock.height points tall, above the bottom bar.
- **Desktop and web.** The same bar across the bottom of the content, layout.dock.height points tall, below the page's scroll; the options key (a period) or Shift+F10 moves focus to it.

Focus: its children take focus, and it is their group. Accessibility: Role: toolbar, labelled by its name. Each control is a button labelled as written ("Sort: Title"). The legend is hidden from assistive technology: its keys are in every control's hint.

```json
{
  "id": "dock",
  "component": "Dock",
  "props": {
    "target": "all",
    "controls": [
      "sort",
      "filter",
      "view",
      "jump"
    ]
  }
}
```

#### KeyLegend

*Key legend.* A line of key hints ("OK Select", "Menu Options") in glyphs matched to the input in use: a keyboard's key caps, a game pad's buttons, a remote's. Hidden when only touch or a mouse is in use. Normally the last thing in a Dock.

| Property | Type | Default | What it is |
|---|---|---|---|
| `actions` | enumList: select, back, options, search, tabs, page, top, dock, help | `select, back, options` | Hints. Which actions are shown, in order. The glyphs come from the input table in themes/tokens.json for the device in use. |

How it is drawn:

- **Television.** One row of hints at tv.dock.hintSize (25 units), the glyph then its label in the caption style, in token fg2, at the trailing end of the Dock. Always drawn: a remote is always in use.
- **Phone.** Not drawn unless a keyboard or pad is in use; touch alone never shows it.
- **Tablet.** As the desktop's, shown only with a keyboard or pad.
- **Desktop and web.** The same row at the caption size, shown only while the last input was a key or a game pad; a mouse or touch hides it, the first key brings it back. Glyphs are key caps (the key, or its name).

Focus: takes none. Accessibility: Role: none (hidden). The keys are announced as each control's hint instead.

```json
{
  "id": "keys",
  "component": "KeyLegend",
  "props": {
    "actions": [
      "select",
      "back",
      "options",
      "search"
    ]
  }
}
```

#### DetailHeader

*Title header.* The top of a title's page: its art, name, facts, score, synopsis and the buttons that play it and act on it. It shows the title the page was opened for. A button that is a toggle (Watch later) has pressed true or false: draw it as on or off and say so to a screen reader; its label and command already match the state.

| Property | Type | Default | What it is |
|---|---|---|---|
| `layout` | enum: backdrop, poster, compact | `backdrop` | Layout. Backdrop behind the text, poster beside it, or a compact strip. |
| `actions` | enumList: play, restart, watchLater, watched, download, share, playOn, watchParty | `play, restart, watchLater, watched, download, share` | Buttons. The buttons offered, in order. One the title or the profile can't use is left out. |
| `facts` | enumList: year, runtime, contentRating, rating, genres, studio, artist, count, season, knownFor, born, died | `year, runtime, contentRating, rating, genres` | Facts. The line of facts under the title, in order. |
| `showOverview` | bool | `true` | Show the synopsis |
| `showTagline` | bool | `true` | Show the tagline |

How it is drawn:

- **Television.** Backdrop across the whole screen under a scrim; the logo or title at the page title size at the left, the facts on one line, at most four lines of synopsis, the buttons in a row with Play first and focused. Sections below scroll up over the backdrop.
- **Phone.** Backdrop 16/9 at the top, title and facts under it, a full-width primary button, the other actions as a row of icon buttons with captions, then the synopsis, cut to four lines with More.
- **Tablet.** As a phone with the poster beside the text block.
- **Desktop and web.** Backdrop behind; poster at the left (layout poster or backdrop), text block beside it, buttons in a row: primary filled, the rest outlined.

States:

- **error.** A Notice: "This title isn't available" with a button back to Home.
- **loading.** A grey backdrop and three grey text lines.

Focus: one stop in the page's focus order; it moves focus among its own parts. Accessibility: Role: region labelled by the title. The title is a level-1 heading. Each button's label is its label, the primary button's "Resume from <time>" when it resumes.

```json
{
  "id": "header",
  "component": "DetailHeader",
  "props": {
    "layout": "backdrop"
  }
}
```

#### SeasonList

*Seasons and episodes.* A show's seasons with the chosen season's episodes. Choosing a season asks for its episodes through the season's own URL.

Sources: `episodes` only.

| Property | Type | Default | What it is |
|---|---|---|---|
| `style` | enum: cards, list | `cards` | Episodes as. Cards with a still and synopsis, or a compact list. |
| `showSynopsis` | bool | `true` | Show each synopsis |

How it is drawn:

- **Television.** A row of season pills, then the episodes as a row of episode cards (still, "<n>. <title>", runtime, two lines of synopsis) that scrolls sideways; focus starts on the next episode to watch.
- **Phone.** A season menu button, then the episodes as a vertical list: still at the left with a progress bar, title, runtime and two lines of synopsis.
- **Desktop and web.** Season tabs, then a vertical list (style list) or a wrapping grid of episode cards (style cards).

States:

- **deferred.** Drawn as loading until the client has asked props.more.url, which it does when the section is within one screen of being seen.
- **empty.** Left out of the page; with empty "message", the heading and emptyText in the caption style where the items would be.
- **error.** The heading and one line, "Couldn't load this", with a Retry button that asks props.more.url again.
- **loading.** Its heading, and as many grey placeholder cards of the card's size as fit, pulsing unless the device asks for reduced motion.

Focus: its children take focus, and it is their group. Accessibility: Role: region labelled "Episodes". The season picker is a pop-up button "Season <n>"; each episode's label is "Episode <n>, <title>, <runtime>, <n> percent watched".

```json
{
  "id": "episodes",
  "component": "SeasonList",
  "source": {
    "type": "episodes",
    "show": "$show"
  }
}
```

#### TrackList

*Track list.* An album's songs, an audiobook's chapters or a podcast's episodes, numbered, with their lengths.

Sources: `tracks` only.

| Property | Type | Default | What it is |
|---|---|---|---|
| `showNumbers` | bool | `true` | Show track numbers |
| `showArtist` | bool | `false` | Show each track's artist. For compilations. |
| `showDuration` | bool | `true` | Show lengths |

How it is drawn:

- **Television.** A list down the right two thirds of the screen, one row per track: number, title, length; the playing track in token accent.
- **Phone.** Full-width rows 56 points tall: number, title over artist, length; swipe actions are the app's own.
- **Desktop and web.** A table: #, Title, Artist (when shown), Length; double-click plays.

States:

- **deferred.** Drawn as loading until the client has asked props.more.url, which it does when the section is within one screen of being seen.
- **empty.** Left out of the page; with empty "message", the heading and emptyText in the caption style where the items would be.
- **error.** The heading and one line, "Couldn't load this", with a Retry button that asks props.more.url again.
- **loading.** Its heading, and as many grey placeholder cards of the card's size as fit, pulsing unless the device asks for reduced motion.

Focus: its children take focus, and it is their group. Accessibility: Role: list. Each track: "Track <n>, <title>, <artist>, <minutes> minutes <seconds> seconds".

```json
{
  "id": "tracks",
  "component": "TrackList",
  "source": {
    "type": "tracks",
    "library": "$library",
    "artist": "$artist",
    "album": "$album"
  }
}
```

#### Credits

*Cast and crew.* Who is in a title and who made it: the cast as a row of portraits with the part each played, then the crew by department (Directing, Writing, Production, Camera, Editing, Sound, Art, Costume & Make-Up, Visual Effects ...), with Show all when there are more. Each person opens their own page.

Sources: `credits` only.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | text, at most 120 |  | Title. The heading above the cast. |
| `parts` | enumList: cast, guests, crew | `cast, guests, crew` | Shows. Which parts are shown, in this order. |
| `castLimit` | int 0 to 100 | `20` | Cast at first. How many of the cast (and of the guest stars) are sent before Show all. 0 sends the device's row of cards. |
| `crewLimit` | int 0 to 50 | `6` | Crew at first. How many people of each department are sent before Show all. |
| `departments` | enumList: directing, writing, production, camera, lighting, editing, sound, art, costume, effects, crew | `directing, writing, production, camera, lighting, editing, sound, art, costume, effects, crew` | Departments. Which crew departments are listed, in order. |
| `cardSize` | enum: s, m, l | `m` | Card size. Small, medium or large cards. A profile may choose its own. |

How it is drawn:

- **Television.** The title at the heading size, then the cast as one row of round portraits 40 points apart that scrolls sideways under focus, name under each and the part played under that; then the guest stars the same way; then each department as a heading and a row of focusable name-and-job chips. Show all is a final card of the cast row and asks props.more.url.
- **Phone.** The title with a Show all chevron, the cast as a row of round portraits that bleeds off both edges, then each department as a caption-sized heading and a wrapping list of "<name>, <job>" lines, each one a link.
- **Tablet.** As a phone with 16-point gaps.
- **Desktop and web.** The title and a Show all button at its end; the cast row with arrow buttons on hover; the crew in columns of departments (as many as fit, at least 220 points each), each a heading and its people, name in body and job in caption.

States:

- **deferred.** Drawn as loading until the client has asked props.more.url, which it does when the section is within one screen of being seen.
- **empty.** Left out of the page; with empty "message", the heading and emptyText in the caption style where the items would be.
- **error.** The heading and one line, "Couldn't load this", with a Retry button that asks props.more.url again.
- **loading.** Its heading, and as many grey placeholder cards of the card's size as fit, pulsing unless the device asks for reduced motion.

Focus: its children take focus, and it is their group. Accessibility: Role: region labelled by the title. The cast row is a list labelled "Cast"; each card's label is "<name>, <part>". Each department is a list labelled by its name; each person's label is "<name>, <job>". A person without a photo shows their initials on token bg3, hidden from assistive technology.

```json
{
  "id": "credits",
  "component": "Credits",
  "source": {
    "type": "credits",
    "item": "$item"
  }
}
```

#### Track

*Track.* One row of a TrackList. Made by the server. Made by the server; not something an administrator places.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | string |  | Title |
| `artist` | string |  | Artist |
| `number` | int |  | Number |
| `durationSec` | int |  | Length in seconds |
| `itemId` | string |  | Item |
| `progress` | number |  | Progress. 0 to 1. |

Focus: the node takes focus when it can be acted on. Accessibility: Role: button. Label as TrackList says.

#### Banner

*Banner.* One wide promotional block: a picture, a title, a line of text and one button. Written by hand (a collection's banner, a way into the TV guide), or drawn from the first item of a source (a random pick).

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | text, at most 120 |  | Title. With a source, {title} is the item's title. |
| `text` | text, at most 300 |  | Text. With a source, {title}, {subtitle} and {overview} are the item's. |
| `image` | image |  | Picture. A picture on this server. With a source, the item's backdrop is used. |
| `icon` | icon |  | Icon. One of the built-in icons. |
| `style` | enum: promo, compact | `promo` | Style |
| `buttonLabel` | text, at most 40 |  | Button. The button's label. |
| `action` | enum: open, play | `open` | The button. With a source: open the item's page, or play it. |
| `to` | route |  | Opens. A place in the app, such as /library/3 or /page/family-night. |
| `native` | enum: settings, guide, profiles, downloads, remote, nowPlaying, pair, collections, player |  | Opens a screen of the app. A screen the app draws itself. |

How it is drawn:

- **Television.** A 1740 by 300 point block: the picture behind a scrim, the title at the heading size and text at the left, one button. The block is one focus stop.
- **Phone.** A full-width 16/9 card (promo) or a 96-point row with a square picture (compact); the whole banner is the button.
- **Desktop and web.** A block at most 360 points tall, picture behind a left-to-right scrim, text and button bottom-left.

States:

- **empty.** Left out.
- **error.** Left out.
- **loading.** A grey block.

Focus: the node takes focus when it can be acted on. Accessibility: Role: button (link on the web). Label: "<title>, <text>, <button label>".

```json
{
  "id": "shuffle",
  "component": "Banner",
  "props": {
    "title": "Can't decide?",
    "text": "{title}",
    "buttonLabel": "Play something",
    "action": "play"
  },
  "source": {
    "type": "random",
    "limit": 1
  }
}
```

#### Text

*Text.* A heading or a block of text, plain or in the small Markdown subset (bold, italic, links to places in the app, lists, headings).

| Property | Type | Default | What it is |
|---|---|---|---|
| `text` (required) | markdown |  | Text. Placeholders such as {user}, {greeting}, {server} and {library} are filled in. |
| `format` | enum: plain, markdown | `plain` | Format |
| `role` | enum: display, pageTitle, heading, title, body, caption | `body` | Size. Sizes come from the device, not from numbers. |
| `align` | enum: start, center, end | `start` | Align |
| `maxLines` | int 0 to 20 |  | Most lines. Cut with an ellipsis after this many lines. 0 doesn't cut. |

How it is drawn:

- **Everywhere.** Text in token fg (caption in fg2) at layout.text[role] points, at most 70 characters wide for body text. Markdown links are drawn in token accentText and are focusable on a television only when the page has nothing else to focus.

Focus: takes none. Accessibility: Role: heading for pageTitle (level 1), heading (2) and title (3); otherwise text.

```json
{
  "id": "hello",
  "component": "Text",
  "props": {
    "text": "{greeting}, {user}",
    "role": "pageTitle"
  }
}
```

#### Notice

*Notice.* A message that stands out: something to know, a warning, or a nudge with one button.

| Property | Type | Default | What it is |
|---|---|---|---|
| `tone` | enum: info, success, warning, danger | `info` | Tone |
| `title` | text, at most 120 |  | Title |
| `text` (required) | text, at most 400 |  | Text |
| `icon` | icon |  | Icon. One of the built-in icons. |
| `buttonLabel` | text, at most 40 |  | Button |
| `to` | route |  | Opens. A place in the app, such as /library/3 or /page/family-night. |
| `native` | enum: settings, guide, profiles, downloads, remote, nowPlaying, pair, collections, player |  | Opens a screen of the app. A screen the app draws itself. |
| `url` | url |  | Opens a web page. An http or https address, opened in the browser. Left out on a television. Only on phone, tablet, desktop, web. |
| `op` | enum |  | Runs a command. One of the catalogue's commands; the page is asked for again after it. |
| `item` | ref |  | Command's item. The item the command acts on, such as $item. |
| `params` | object |  | Command's values. The command's other values by name, each a text or a $variable. |
| `confirm` | text, at most 120 |  | Ask first. A question the app asks before the command runs, such as Delete this memory? |
| `back` | bool | `false` | Goes back. Closes the page: Back. |

How it is drawn:

- **Everywhere.** A rounded block in token bg2 with a 4-point bar at its leading edge in the tone's colour (accent, ok, warn, danger), the icon, title in the title size, text in body, and the button at the end (below on a phone). It takes focus only when it has a button.

Focus: the node takes focus when it can be acted on. Accessibility: Role: status (alert for warning and danger). Label: "<title>. <text>".

```json
{
  "id": "setup",
  "component": "Notice",
  "when": {
    "admin": true,
    "flag": "setupIncomplete"
  },
  "props": {
    "tone": "info",
    "title": "Finish setting up",
    "text": "A few things are left to choose.",
    "buttonLabel": "Open settings",
    "native": "settings"
  }
}
```

#### Divider

*Divider.* A hairline between sections.

| Property | Type | Default | What it is |
|---|---|---|---|
| `inset` | bool | `false` | Inset. Start the line at the content's edge rather than the screen's. |

How it is drawn:

- **Everywhere.** A 1-point line in token border across the content width.

Focus: takes none. Accessibility: Role: separator; not announced.

```json
{
  "id": "rule",
  "component": "Divider"
}
```

#### Spacer

*Space.* Empty space between sections.

| Property | Type | Default | What it is |
|---|---|---|---|
| `size` | enum: xs, s, m, l, xl | `m` | Size |

How it is drawn:

- **Everywhere.** layout.spacing[size] points of nothing, in the direction of its container.

Focus: takes none. Accessibility: Hidden from assistive technology.

```json
{
  "id": "gap",
  "component": "Spacer",
  "props": {
    "size": "l"
  }
}
```

#### Stack

*Stack.* Sections one after another, down or across. The plain container for a custom page.

Holds: any component.

| Property | Type | Default | What it is |
|---|---|---|---|
| `direction` | enum: column, row | `column` | Direction |
| `gap` | enum: none, xs, s, m, l, xl | `m` | Space between. The distance between the things inside. |
| `align` | enum: start, center, end, stretch | `stretch` | Align. Across the stack's direction. |
| `wrap` | bool | `true` | Wrap. A row that doesn't fit continues on the next line. |

How it is drawn:

- **Everywhere.** A flex column or row with layout.spacing[gap] between children. A row of Buttons is one focus group.

Focus: takes none. Accessibility: No role of its own.

```json
{
  "id": "links",
  "component": "Stack",
  "props": {
    "direction": "row",
    "gap": "s"
  },
  "children": [
    {
      "id": "guide",
      "component": "Button",
      "props": {
        "label": "TV guide",
        "icon": "guide",
        "native": "guide"
      }
    }
  ]
}
```

#### Split

*Two columns.* Two sections side by side on a wide screen, one above the other on a narrow one.

Holds: any component.

| Property | Type | Default | What it is |
|---|---|---|---|
| `ratio` | enum: 1:1, 1:2, 2:1 | `1:1` | Widths |
| `gap` | enum: none, xs, s, m, l, xl | `m` | Space between. The distance between the things inside. |
| `stackBelow` | enum: compact, medium, expanded | `medium` | One column below. The window width under which the two stack. |

How it is drawn:

- **Television.** Always side by side.
- **Phone.** Always stacked.
- **Desktop and web.** Side by side at or above the stackBelow breakpoint, stacked under it.

Focus: takes none. Accessibility: No role of its own.

```json
{
  "id": "two",
  "component": "Split",
  "props": {
    "ratio": "2:1"
  },
  "children": [
    {
      "id": "left",
      "component": "Text",
      "props": {
        "text": "About this server"
      }
    },
    {
      "id": "right",
      "component": "Button",
      "props": {
        "label": "Settings",
        "native": "settings"
      }
    }
  ]
}
```

#### Button

*Button.* A button that opens a place in the app, one of the app's own screens, or a web page.

| Property | Type | Default | What it is |
|---|---|---|---|
| `label` (required) | text, at most 60 |  | Label |
| `icon` | icon |  | Icon. One of the built-in icons. |
| `variant` | enum: primary, secondary, ghost | `secondary` | Look |
| `to` | route |  | Opens. A place in the app, such as /library/3 or /page/family-night. |
| `native` | enum: settings, guide, profiles, downloads, remote, nowPlaying, pair, collections, player |  | Opens a screen of the app. A screen the app draws itself. |
| `url` | url |  | Opens a web page. An http or https address, opened in the browser. Left out on a television. Only on phone, tablet, desktop, web. |
| `op` | enum |  | Runs a command. One of the catalogue's commands; the page is asked for again after it. |
| `item` | ref |  | Command's item. The item the command acts on, such as $item. |
| `params` | object |  | Command's values. The command's other values by name, each a text or a $variable. |
| `confirm` | text, at most 120 |  | Ask first. A question the app asks before the command runs, such as Delete this memory? |
| `back` | bool | `false` | Goes back. Closes the page: Back. |

How it is drawn:

- **Everywhere.** The platform's own button: filled in token accent with accentFg text (primary), outlined (secondary) or text only (ghost); at least 44 points tall on touch devices, 66 on a television.

Focus: the node takes focus when it can be acted on. Accessibility: Role: button (link on the web when it navigates). Label: the label.

```json
{
  "id": "open-settings",
  "component": "Button",
  "props": {
    "label": "Settings",
    "icon": "settings",
    "native": "settings"
  }
}
```

#### Image

*Picture.* A picture from this server, optionally a link.

| Property | Type | Default | What it is |
|---|---|---|---|
| `src` (required) | image |  | Picture. A path on this server beginning /api/. |
| `alt` | text, at most 200 |  | Description. For people who can't see the picture. Leave empty when it is decoration. |
| `aspect` | enum: 16/9, 21/9, 4/3, 1/1, 2/3 | `16/9` | Shape |
| `fit` | enum: cover, contain | `cover` | Fit |
| `rounded` | bool | `true` | Rounded corners |
| `to` | route |  | Opens. A place in the app, such as /library/3 or /page/family-night. |

How it is drawn:

- **Everywhere.** The picture at the content width (at most 960 points) in its aspect, asked for at the width in props.image. Focusable only with a to.

Focus: takes none. Accessibility: Role: img with alt as its label; hidden when alt is empty and it isn't a link.

```json
{
  "id": "pic",
  "component": "Image",
  "props": {
    "src": "/api/collections/4/cover",
    "alt": "Family films",
    "aspect": "21/9"
  }
}
```

#### SearchBox

*Search box.* The field people type a search into. On the search page it holds the current search; anywhere else it opens the search page.

| Property | Type | Default | What it is |
|---|---|---|---|
| `placeholder` | text, at most 80 |  | Prompt |
| `autofocus` | bool | `false` | Start here. Focus starts in the field. |

How it is drawn:

- **Television.** The system's search field and keyboard (UISearchController on tvOS, the leanback search bar on Android TV); results appear under it as they are typed.
- **Phone.** A search field under the title; typing asks for the page again with q after 300 ms without a keystroke.
- **Desktop and web.** A search field at most 560 points wide; Enter or 300 ms without a keystroke searches.

Focus: the node takes focus when it can be acted on. Accessibility: Role: searchbox labelled "Search".

```json
{
  "id": "q",
  "component": "SearchBox",
  "props": {
    "placeholder": "Films, shows, music…",
    "autofocus": true
  }
}
```

#### List

*List.* Rows of text with an icon or small picture, written by hand or drawn from a source.

Holds: `ListItem`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | text, at most 120 |  | Title. The heading above the items. Placeholders such as {library} and {each} are filled in. |
| `headingStyle` | enum: plain, band, underline | `plain` | Heading style. How the title is drawn: plain, a coloured band across the row, or underlined. Left plain, the experience chooses. |
| `dense` | bool | `false` | Compact rows |
| `focusable` | bool | `false` | Cards without a route take focus. A card with no route (a friend) is still a stop for focus, and OK on it is the page's to use. Off: it is not a button and takes no focus. |
| `empty` | enum: hide, message | `hide` | When there is nothing to show. Leave the section out, or keep it with a message. |
| `emptyText` | text, at most 200 |  | Message when empty. Shown in place of the items when there are none. |

How it is drawn:

- **Everywhere.** Full-width rows (56 points, 44 dense, 90 on a television): picture or icon, title over subtitle, trailing text, and a chevron when the row opens something.

States:

- **deferred.** Drawn as loading until the client has asked props.more.url, which it does when the section is within one screen of being seen.
- **empty.** Left out of the page; with empty "message", the heading and emptyText in the caption style where the items would be.
- **error.** The heading and one line, "Couldn't load this", with a Retry button that asks props.more.url again.
- **loading.** Its heading, and as many grey placeholder cards of the card's size as fit, pulsing unless the device asks for reduced motion.

Focus: its children take focus, and it is their group. Accessibility: Role: list labelled by the title.

```json
{
  "id": "lists",
  "component": "List",
  "props": {
    "title": "Your collections"
  },
  "source": {
    "type": "collections"
  }
}
```

#### ListItem

*List row.* One row of a List.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` (required) | text, at most 120 |  | Title |
| `subtitle` | text, at most 160 |  | Subtitle |
| `icon` | icon |  | Icon. One of the built-in icons. |
| `trailing` | text, at most 40 |  | Text at the end |
| `to` | route |  | Opens. A place in the app, such as /library/3 or /page/family-night. |
| `native` | enum: settings, guide, profiles, downloads, remote, nowPlaying, pair, collections, player |  | Opens a screen of the app. A screen the app draws itself. |
| `url` | url |  | Opens a web page. An http or https address, opened in the browser. Left out on a television. Only on phone, tablet, desktop, web. |
| `op` | enum |  | Runs a command. One of the catalogue's commands; the page is asked for again after it. |
| `item` | ref |  | Command's item. The item the command acts on, such as $item. |
| `params` | object |  | Command's values. The command's other values by name, each a text or a $variable. |
| `confirm` | text, at most 120 |  | Ask first. A question the app asks before the command runs, such as Delete this memory? |
| `back` | bool | `false` | Goes back. Closes the page: Back. |

Focus: the node takes focus when it can be acted on. Accessibility: Role: button when it opens something, else text. Label: "<title>, <subtitle>, <trailing>".

#### Form

*Form.* A group of rows with controls (switches, choices, fields, sliders) saved together: Save runs one command with every named control's value, Cancel goes back. Settings itself stays hand-built; a Form is for a page the server or a module serves.

Holds: `List`, `Text`, `Notice`, `Divider`, `Spacer`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `title` | text, at most 120 |  | Title. The heading above the items. Placeholders such as {library} and {each} are filled in. |
| `submitLabel` | text, at most 24 |  | Save button |
| `cancelLabel` | text, at most 24 |  | Cancel button. Left out: no Cancel; Back still goes back. |
| `op` (required) | enum |  | Command. The command Save runs, one of the catalogue's commands. |
| `item` | ref |  | Item. The command's item, such as $item. |

How it is drawn:

- **Television.** The rows as a list, then a Dock of Save (primary) and Cancel; Save is disabled until something has changed.
- **Phone.** The rows full width, a Save and Cancel bar fixed at the foot above the keyboard.
- **Desktop and web.** The rows in a column at most 720 points wide, Save and Cancel at its foot.

States:

- **empty.** Left out.
- **error.** The error under Save, in the danger tone; the values stay as typed.
- **loading.** The rows as loading rows.

Focus: its children take focus, and it is their group. Accessibility: Role: form, labelled by the title. Each control is labelled by its row's title and described by its subtitle and why; Save announces the result politely.

```json
{
  "id": "privacy",
  "component": "Form",
  "props": {
    "title": "Games",
    "op": "wittyplay.online.set"
  },
  "children": [
    {
      "id": "rows",
      "component": "List",
      "source": {
        "type": "wittyplay.privacy"
      }
    }
  ]
}
```

#### QR

*QR code.* A QR code for a link, for a phone to scan: a room's join link, a pairing address. The value is a route of the app (/games?join=CODE) or an https address; with a source, the first item's link is the value and its title the label (a source with no link gives nothing, and the section is left out).

| Property | Type | Default | What it is |
|---|---|---|---|
| `value` | text, at most 500 |  | Value. A route of the app or an https address. Left out when the section has a source. |
| `label` | text, at most 80 |  | Caption. Under the code. |
| `size` | enum: s, m, l | `m` | Size |

How it is drawn:

- **Television.** A square of 280 points (s 160, m 240, l 320) with its caption under it.
- **Phone.** A square of 140 points (s), 200 (m) or 280 (l), centred.
- **Desktop and web.** As a phone, with the address under the caption as selectable text.

States:

- **empty.** Left out.
- **error.** Left out.
- **loading.** A grey square.

Focus: takes none. Accessibility: Role: img, labelled "<caption>: QR code for <address>". Not focusable.

```json
{
  "id": "join",
  "component": "QR",
  "props": {
    "label": "Scan to join"
  },
  "source": {
    "type": "collection",
    "collection": 1,
    "limit": 1
  }
}
```

#### NativeView

*The app's own view.* A place where the app draws one of its own rich views: the TV guide's grid, the photo timeline. An app that has the view says so; any other is sent the sections inside instead.

Holds: any component.

| Property | Type | Default | What it is |
|---|---|---|---|
| `view` (required) | enum: guide, photoTimeline, photoMap, coverFlow |  | View |
| `library` | library |  | Library. The library the view shows, where it shows one. |

How it is drawn:

- **Everywhere.** The app's own view fills the rest of the page; nothing may follow a NativeView in a page.

Focus: one stop in the page's focus order; it moves focus among its own parts. Accessibility: Whatever the app's own view provides.

```json
{
  "id": "timeline",
  "component": "NativeView",
  "props": {
    "view": "photoTimeline",
    "library": "$library"
  },
  "children": [
    {
      "id": "photos",
      "component": "Grid",
      "props": {
        "cardStyle": "photo"
      },
      "source": {
        "type": "recentlyAdded",
        "library": "$library"
      }
    }
  ]
}
```

### Card styles

A `Card` has a `style` and a `shape`. A renderer that doesn't know a style draws its shape: the picture, with the title and subtitle under it. `Shows` is what the style displays of what the card carries, in reading order, which is also the order of its accessibility label.

| Style | Shape | Aspect | Picture, in order of preference | Shows | Title lines | Subtitle lines | For |
|---|---|---|---|---|---|---|---|
| `poster` | poster | 2/3 | poster | title, subtitle, progress, watched, unwatched, badges | 1 | 1 | Upright film and show art with the title and year under it. |
| `landscape` | landscape | 16/9 | backdrop, poster | title, subtitle, badges | 1 | 1 | A wide still or backdrop with the title under it. |
| `progress` | landscape | 16/9 | backdrop, poster | title, subtitle, progress, remaining | 1 | 1 | A wide card with a progress bar along its bottom edge and how long is left: Continue watching. |
| `episode` | landscape | 16/9 | backdrop, poster | title, subtitle, synopsis, progress, watched | 1 | 1 | An episode's still with "<n>. <title>", its running time and two lines of synopsis. |
| `square` | square | 1/1 | square, poster | title, subtitle | 1 | 1 | A square picture with a title and subtitle. |
| `album` | square | 1/1 | square, poster | title, subtitle | 1 | 1 | An album cover with the album and artist under it. |
| `circle` | circle | 1/1 | square, poster | title | 2 | 0 | A round picture with a centred name. |
| `person` | circle | 1/1 | square, poster | title, subtitle, badges | 1 | 1 | A round portrait with a name and a line under it: an artist, an author, someone in a title's cast or crew (the line is the part they played or their job), a friend (the line is where they are and the badge their presence). A person's initials stand in for a missing photo. The card opens something only when the item has a route; without one it is not a button and takes no focus. |
| `book` | poster | 2/3 | poster | title, subtitle, progress | 2 | 1 | A book or comic cover with the title and author, and how far it has been read. |
| `game` | poster | 3/4 | poster | title, subtitle, badges | 1 | 1 | Box art with the game's name and console. |
| `photo` | square | 1/1 | poster, square |  | 0 | 0 | The picture alone, cropped square; its name is only read out. |
| `channel` | landscape | 16/9 | logo | number, title, now, next, progress | 1 | 2 | A channel's logo on a plain tile, its number and name, what is on now with how far through it is, and what is next. |
| `tile` | landscape | 16/9 | backdrop, poster | title, count | 1 | 0 | A wide tile with its name over the picture, or over a plain tile with an icon: a library, a collection. |
| `plate` | poster | 2/3 | poster, backdrop | title, subtitle, badges | 2 | 1 | Upright art with the title over it on a scrim along its foot: the Rail look's card. |
| `block` | landscape | 16/9 | backdrop, poster | title, subtitle | 2 | 1 | A flat block of colour with the title in it, no picture unless there is room: teletext's cell. |
| `logo` | landscape | 16/9 | logo | number, title | 1 | 0 | A channel's logo, contain-fitted on a plain tile, with its number: a satellite box's channel list. |
| `chip` | chip |  |  | title | 1 | 0 | A pill with a name alone: a genre. |

The hero's slides are cards of style `hero`: shape landscape, a 16/9 backdrop the hero's width, with `facts`, `synopsis`, `logo` and `actions`.

### Card sizes

The width of a medium (`m`) card in points; small is 80% and large 125%, rounded down to an even number. The picture's height follows from the aspect. A television's grid has a fixed number of columns for medium cards (one more for small, one fewer for large).

| Style | tv | tv columns | phone | tablet | desktop and web |
|---|---|---|---|---|---|
| `poster` | 250 | 6 | 120 | 160 | 180 |
| `landscape` | 420 | 4 | 224 | 288 | 320 |
| `progress` | 420 | 4 | 224 | 288 | 320 |
| `episode` | 420 | 4 | 224 | 288 | 320 |
| `square` | 300 | 6 | 144 | 180 | 200 |
| `album` | 300 | 6 | 144 | 180 | 200 |
| `circle` | 200 | 7 | 96 | 120 | 140 |
| `person` | 200 | 7 | 96 | 120 | 140 |
| `book` | 250 | 6 | 120 | 160 | 180 |
| `game` | 250 | 6 | 120 | 160 | 180 |
| `photo` | 300 | 6 | 144 | 180 | 200 |
| `channel` | 420 | 4 | 224 | 288 | 320 |
| `tile` | 420 | 4 | 224 | 288 | 320 |
| `plate` | 250 | 6 | 120 | 160 | 180 |
| `block` | 420 | 4 | 224 | 288 | 320 |
| `logo` | 420 | 4 | 224 | 288 | 320 |

### Theme tokens

Every colour, typeface and corner comes from the theme: colours from `GET /api/v1/theme` `tokens` (with a node's scope `tokens` over them), corners from its `shape`, typefaces and weights from its `type`, sizes from `layout.text`. A renderer has none of its own. Each component and card style has `tokens` in the catalogue: which colour token each part is drawn in, which text role each piece of text takes, and which shape token rounds each part. [THEMES.md](THEMES.md#sdui-tokens) explains them.

Text roles: `badge` (body family, strong weight), `body` (body family, body weight), `caption` (body family, body weight), `display` (display family, heading weight), `heading` (display family, heading weight), `pageTitle` (display family, heading weight), `title` (body family, strong weight).

Colour roles:

- `background`: What the component fills its own area with.
- `surface`: A raised area inside it, and the place of a picture that hasn't loaded.
- `text`: Its main text.
- `secondaryText`: Subtitles, facts, inactive tabs.
- `hintText`: Numbers, placeholders, hints.
- `link`: Links and highlighted words: the accent as text.
- `icon`: Icons beside text.
- `accent`: Filled things that stand out: a primary button, a selected tab's underline.
- `onAccent`: Text and icons on accent.
- `accentHover`: accent while hovered or pressed.
- `selected`: The tint behind a selected row or chip.
- `selectedBorder`: The outline of a selected chip.
- `selectedText`: The text of a selected tab, chip or row.
- `hover`: The tint over a hovered row (pointer devices).
- `pressed`: The tint over a pressed row.
- `border`: Hairlines.
- `outline`: Outlines of secondary buttons and chips.
- `fieldOutline`: Outlines of text fields and other controls: 3:1 on every surface.
- `focus`: The focus ring.
- `onImage`: Text and icons over a picture.
- `scrim`: The plate behind text over a picture.
- `badge`: A badge's fill (over a picture).
- `badgeText`: A badge's text.
- `progress`: A progress bar's filled part.
- `progressTrack`: A progress bar's empty part.
- `watched`: The watched mark's fill.
- `onWatched`: The watched mark's tick.
- `playing`: The track or item playing now.
- `info`: The information tone's bar and icon.
- `success`: The good-news tone's bar and icon.
- `warning`: The warning tone's bar and icon.
- `danger`: The problem tone's bar, icon, and a destructive button.
- `dangerTint`: The tint behind a hovered destructive button.
- `brand`: The server's name, its coloured parts (brandParts with accent).

Shape tokens:

- `card`: Cards and their pictures.
- `button`: Buttons.
- `field`: Text fields, selects, search boxes.
- `row`: List rows.
- `thumb`: Small pictures in a row.
- `chip`: Chips and filter pills.
- `badge`: Badges and small labels.
- `image`: Pictures and banners placed on a page; the spotlight on a television.
- `sheet`: Menus, popovers and sheets.
- `dialog`: Dialogs.
- `pill`: Fully round whatever the size: a circle on a square picture.
- `focusWidth`: The focus ring's width.
- `focusOffset`: The gap between the focus ring and what it surrounds.

| Component | Colours | Text | Shape |
|---|---|---|---|
| `Page` | background `bg`, brand `brandAccent`, secondaryText `fg2`, text `fg` | title `pageTitle` |  |
| `Hero` | accent `accent`, accentHover `accentHover`, background `bg`, badge `scrim`, badgeText `onImage`, focus `focus`, onAccent `accentFg`, onImage `onImage`, outline `onImage`, scrim `scrim`, secondaryText `onImage`, surface `bg3`, text `onImage` | badge `badge`, button `body`, facts `caption`, synopsis `body`, title `display` | badge `badge`, button `button`, corner `image`, focus `focusWidth`, focusGap `focusOffset`, pageDots `pill` |
| `Shelf` | focus `focus`, link `accentText`, secondaryText `fg2`, surface `bg3`, text `fg` | seeAll `body`, subtitle `caption`, title `heading` | focus `focusWidth`, focusGap `focusOffset`, scrollButton `pill` |
| `Grid` | focus `focus`, link `accentText`, secondaryText `fg2`, surface `bg3`, text `fg` | seeAll `body`, subtitle `caption`, title `heading` | focus `focusWidth`, focusGap `focusOffset` |
| `Card` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `Tabs` | accent `accent`, border `border`, focus `focus`, hover `hover`, selected `accentSoft`, selectedText `fg`, surface `bg3`, text `fg2` | label `title` | focus `focusWidth`, focusGap `focusOffset`, segment `chip` |
| `Tab` | accent `accent`, focus `focus`, hover `hover`, selected `accentSoft`, selectedText `fg`, text `fg2` | label `title` | focus `focusWidth`, focusGap `focusOffset`, segment `chip` |
| `Carousel` | accent `accent`, focus `focus`, link `accentText`, onAccent `accentFg`, secondaryText `fg2`, surface `bg3`, text `fg` | focused `display`, subtitle `caption`, title `heading` | button `button`, focus `focusWidth`, focusGap `focusOffset`, scrollButton `pill` |
| `CrossBar` | accent `accent`, border `border`, focus `focus`, hover `hover`, icon `fg2`, selected `accentSoft`, selectedText `fg`, surface `bg3`, text `fg2` | item `heading`, label `title` | focus `focusWidth`, focusGap `focusOffset`, row `row`, segment `chip` |
| `WidgetGrid` | background `bg`, focus `focus`, text `fg` |  | focus `focusWidth`, focusGap `focusOffset` |
| `Widget` | background `bg2`, border `border`, focus `focus`, hintText `fg3`, link `accentText`, progress `accent`, progressTrack `bg4`, secondaryText `fg2`, surface `bg3`, text `fg` | caption `caption`, figure `display`, text `body`, title `heading` | corner `card`, focus `focusWidth`, focusGap `focusOffset` |
| `NumberedRow` | border `border`, focus `focus`, hintText `fg3`, hover `hover`, playing `accentText`, secondaryText `fg2`, selected `accentSoft`, surface `bg3`, text `fg` | number `title`, row `body`, subtitle `caption`, title `heading` | art `thumb`, focus `focusWidth`, focusGap `focusOffset`, row `row` |
| `FilterBar` | fieldOutline `controlLine`, focus `focus`, hintText `fg3`, hover `hover`, outline `borderStrong`, secondaryText `fg2`, selected `accentSoft`, selectedBorder `accentLine`, selectedText `fg`, surface `bg3`, text `fg` | count `caption`, label `body` | chip `chip`, field `field`, focus `focusWidth`, focusGap `focusOffset`, menu `sheet` |
| `Dock` | accent `accent`, accentHover `accentHover`, background `bg2`, border `border`, danger `danger`, dangerTint `dangerSoft`, focus `focus`, hover `hover`, onAccent `accentFg`, outline `borderStrong`, pressed `pressed`, secondaryText `fg2`, selected `accentSoft`, selectedText `fg`, surface `bg3`, text `fg` | label `body` | button `button`, focus `focusWidth`, focusGap `focusOffset` |
| `KeyLegend` | outline `borderStrong`, surface `bg3`, text `fg2` | label `caption` | keycap `badge` |
| `DetailHeader` | accent `accent`, accentHover `accentHover`, background `bg`, badge `scrim`, badgeText `onImage`, border `borderStrong`, danger `danger`, dangerTint `dangerSoft`, focus `focus`, hover `hover`, link `accentText`, onAccent `accentFg`, onImage `onImage`, outline `borderStrong`, pressed `pressed`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg` | badge `badge`, button `body`, facts `caption`, synopsis `body`, title `pageTitle` | badge `badge`, button `button`, focus `focusWidth`, focusGap `focusOffset`, poster `card`, progress `pill` |
| `SeasonList` | focus `focus`, hintText `fg3`, hover `hover`, onWatched `accentFg`, outline `borderStrong`, progress `accent`, progressTrack `bg4`, secondaryText `fg2`, selected `accentSoft`, selectedBorder `accentLine`, selectedText `fg`, surface `bg3`, text `fg`, watched `accent` | season `body`, synopsis `caption`, title `title` | focus `focusWidth`, focusGap `focusOffset`, row `row`, season `chip`, still `card` |
| `TrackList` | border `border`, focus `focus`, hintText `fg3`, hover `hover`, playing `accentText`, pressed `pressed`, secondaryText `fg2`, selected `accentSoft`, text `fg` | artist `caption`, length `caption`, number `caption`, title `body` | focus `focusWidth`, focusGap `focusOffset`, row `row` |
| `Credits` | border `border`, focus `focus`, hintText `fg3`, hover `hover`, link `accentText`, pressed `pressed`, secondaryText `fg2`, surface `bg3`, text `fg` | department `title`, name `body`, part `caption`, showAll `body`, title `heading` | chip `chip`, focus `focusWidth`, focusGap `focusOffset`, portrait `pill`, scrollButton `pill` |
| `Track` | focus `focus`, hintText `fg3`, hover `hover`, playing `accentText`, pressed `pressed`, secondaryText `fg2`, selected `accentSoft`, text `fg` | artist `caption`, length `caption`, number `caption`, title `body` | focus `focusWidth`, focusGap `focusOffset`, row `row` |
| `Banner` | accent `accent`, accentHover `accentHover`, background `bg3`, border `border`, danger `danger`, dangerTint `dangerSoft`, focus `focus`, hover `hover`, onAccent `accentFg`, onImage `onImage`, outline `borderStrong`, pressed `pressed`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg` | button `body`, text `body`, title `heading` | button `button`, corner `image`, focus `focusWidth`, focusGap `focusOffset`, thumb `card` |
| `Text` | hintText `fg3`, link `accentText`, secondaryText `fg2`, surface `bg3`, text `fg` | body `body`, caption `caption`, heading `heading`, pageTitle `pageTitle`, title `title` | code `badge` |
| `Notice` | accent `accent`, accentHover `accentHover`, background `bg2`, border `border`, danger `danger`, dangerTint `dangerSoft`, focus `focus`, hover `hover`, icon `fg2`, info `accentText`, onAccent `accentFg`, outline `borderStrong`, pressed `pressed`, secondaryText `fg2`, success `ok`, surface `bg3`, text `fg`, warning `warn` | button `body`, text `body`, title `title` | button `button`, corner `card`, focus `focusWidth`, focusGap `focusOffset` |
| `Divider` | border `border` |  |  |
| `Spacer` |  |  |  |
| `Stack` | secondaryText `fg2`, text `fg` | title `heading` |  |
| `Split` |  |  |  |
| `Button` | accent `accent`, accentHover `accentHover`, danger `danger`, dangerTint `dangerSoft`, focus `focus`, hover `hover`, onAccent `accentFg`, outline `borderStrong`, pressed `pressed`, surface `bg3`, text `fg` | label `body` | corner `button`, focus `focusWidth`, focusGap `focusOffset`, iconOnly `pill` |
| `Image` | focus `focus`, surface `bg3` |  | corner `image`, focus `focusWidth`, focusGap `focusOffset` |
| `SearchBox` | fieldOutline `controlLine`, focus `focus`, hintText `fg3`, icon `fg2`, surface `bg3`, text `fg` | text `body` | corner `field`, focus `focusWidth`, focusGap `focusOffset` |
| `List` | border `border`, focus `focus`, secondaryText `fg2`, text `fg` | title `heading` | focus `focusWidth`, focusGap `focusOffset` |
| `ListItem` | accent `accent`, danger `danger`, fieldOutline `controlLine`, focus `focus`, hintText `fg3`, hover `hover`, icon `fg2`, onAccent `accentFg`, pressed `pressed`, progress `accent`, progressTrack `controlLine`, secondaryText `fg2`, selected `accentSoft`, selectedText `fg`, surface `bg3`, text `fg` | subtitle `caption`, title `body`, trailing `caption`, why `caption` | art `thumb`, corner `row`, field `field`, focus `focusWidth`, focusGap `focusOffset`, icon `row` |
| `Form` | accent `accent`, accentHover `accentHover`, border `border`, danger `danger`, dangerTint `dangerSoft`, focus `focus`, hover `hover`, onAccent `accentFg`, outline `borderStrong`, pressed `pressed`, secondaryText `fg2`, surface `bg3`, text `fg` | button `body`, error `caption`, title `heading` | button `button`, focus `focusWidth`, focusGap `focusOffset` |
| `QR` | secondaryText `fg2`, surface `bg2`, text `fg` | text `caption` |  |
| `NativeView` | background `bg`, focus `focus`, outline `borderStrong`, secondaryText `fg2`, surface `bg2`, text `fg` | text `body` | corner `image`, focus `focusWidth`, focusGap `focusOffset` |

| Card style | Colours | Text | Shape |
|---|---|---|---|
| `poster` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `landscape` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `progress` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `episode` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, subtitle `caption`, synopsis `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `square` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `album` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `circle` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, subtitle `caption`, title `title` | badge `badge`, corner `pill`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `person` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, subtitle `caption`, title `title` | badge `badge`, corner `pill`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `book` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `game` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `photo` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` |  | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `channel` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg2`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, next `caption`, now `caption`, number `caption`, subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `tile` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, icon `fg2`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `onImage`, watched `accent` | badge `badge`, subtitle `caption`, title `heading` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `plate` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `onImage`, watched `accent` | badge `badge`, subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `block` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg3`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `logo` | badge `scrim`, badgeText `onImage`, border `border`, focus `focus`, hintText `fg2`, onImage `onImage`, onWatched `accentFg`, progress `accent`, progressTrack `bg4`, scrim `scrim`, secondaryText `fg2`, surface `bg3`, text `fg`, watched `accent` | badge `badge`, next `caption`, now `caption`, number `caption`, subtitle `caption`, title `title` | badge `badge`, corner `card`, focus `focusWidth`, focusGap `focusOffset`, progress `pill` |
| `chip` | focus `focus`, hover `hover`, outline `borderStrong`, selected `accentSoft`, selectedBorder `accentLine`, selectedText `fg`, text `fg2` | title `body` | corner `chip`, focus `focusWidth`, focusGap `focusOffset` |

### Device profiles

| | tv (hd) | tv (4k) | phone | tablet | desktop | web |
|---|---|---|---|---|---|---|
| Scale (pixels per point) | 1 | 2 | 3 | 2 | 2 | 2 |
| Canvas | 1920×1080 | 1920×1080 | the window | the window | the window | the window |
| Breakpoint when the window isn't known | large | large | compact | expanded | large | large |
| Safe area (top, right, bottom, left) | 60, 90, 60, 90 | 60, 90, 60, 90 | the system's | the system's | the system's | the system's |
| Side margins | 90 | 90 | 16 | 16 | 16 | 16 |
| Focus is the only pointer | yes | yes | no | no | no | no |
| Opens web links | no | no | yes | yes | yes | yes |
| Spacing xs, s, m, l, xl | 10, 20, 40, 60, 100 | 10, 20, 40, 60, 100 | 4, 8, 16, 24, 40 | 4, 8, 16, 24, 48 | 4, 8, 16, 24, 48 | 4, 8, 16, 24, 48 |
| Text pageTitle, heading, title, body, caption | 57, 48, 38, 29, 25 | 57, 48, 38, 29, 25 | 28, 22, 17, 16, 13 | 34, 24, 20, 17, 13 | 28, 20, 16, 14, 12 | 28, 20, 16, 14, 12 |
| Hero | 1920×620 | 1920×620 | 430×242 | 1024×440 | 1600×620 | 1600×620 |
| Most sections a page | 12 | 20 | 16 | 20 | 30 | 30 |
| Sections that arrive filled | 4 | 6 | 5 | 6 | 8 | 8 |
| Cards in a row's first page | 12 | 20 | 12 | 16 | 20 | 20 |
| Cards in a grid's first page | 36 | 60 | 30 | 48 | 60 | 60 |
| Rows in a list's first page | 30 | 50 | 30 | 40 | 50 | 50 |
| Cards in each page after | 24 | 36 | 30 | 36 | 48 | 48 |
| Titles in a hero | 5 | 5 | 5 | 5 | 6 | 6 |
| Copies of a repeated section | 8 | 12 | 10 | 12 | 16 | 16 |
| Characters: title, subtitle, synopsis, overview | 80, 80, 160, 480 | 80, 80, 160, 480 | 80, 80, 140, 400 | 100, 100, 200, 700 | 120, 120, 220, 900 | 120, 120, 220, 900 |

Pictures are asked for at one of these widths, in pixels: 120, 160, 240, 320, 480, 640, 960, 1280, 1920, 2560, 3840.

### Data sources

| Source | Label | Category | Gives | Usual card style | Pages by cursor | On pages |
|---|---|---|---|---|---|---|
| [`continueWatching`](#continuewatching) | Continue watching | personal | movie, episode, video, disc, book, track, audiobook, podcast | progress |  | any |
| [`nextUp`](#nextup) | Next up | personal | episode | episode |  | any |
| [`recommended`](#recommended) | Recommended for you | personal | movie, show | poster |  | any |
| [`watchLater`](#watchlater) | Watch later | personal | movie, show, episode, video | poster | yes | any |
| [`liked`](#liked) | Liked | personal | movie, episode, video, disc, track, book, game | poster | yes | any |
| [`statistics`](#statistics) | Statistics | personal | movie, episode, video, disc, show, track, book, game, library | poster |  | any |
| [`serverStatus`](#serverstatus) | Server status | structure | status | tile |  | any |
| [`savedView`](#savedview) | A saved view | personal | movie, show | poster | yes | any |
| [`recentlyAdded`](#recentlyadded) | Recently added | library | movie, show, album, book, game, photo, video, podcast, audiobook | poster | yes | any |
| [`recentlyReleased`](#recentlyreleased) | Recently released | library | movie, show | poster | yes | any |
| [`topRated`](#toprated) | Top rated | discover | movie, show | poster | yes | any |
| [`random`](#random) | Random picks | discover | movie, show | poster |  | any |
| [`browse`](#browse) | A filter | discover | movie, show, disc | poster | yes | any |
| [`genre`](#genre) | A genre | discover | movie, show | poster | yes | any |
| [`studio`](#studio) | A studio or network | discover | movie, show | poster | yes | any |
| [`tag`](#tag) | A tag | discover | movie, show | poster | yes | any |
| [`collection`](#collection) | A collection | discover | movie, show, episode, video | poster | yes | any |
| [`collections`](#collections) | Collections | structure | collection | tile |  | any |
| [`featured`](#featured) | Featured | discover | movie, video | landscape |  | any |
| [`libraries`](#libraries) | Libraries | structure | library | tile |  | any |
| [`genres`](#genres) | Genres | structure | genre | chip |  | any |
| [`liveNow`](#livenow) | On TV now | live | channel | channel |  | any |
| [`recordings`](#recordings) | Recordings | live | recording | landscape |  | any |
| [`books`](#books) | Books | library | book | book | yes | any |
| [`formats`](#formats) | A book's formats | context | view | square |  | detail, custom |
| [`albums`](#albums) | Albums | music | album | album | yes | any |
| [`artists`](#artists) | Artists | music | artist | person | yes | any |
| [`continuePlaying`](#continueplaying) | Continue playing | personal | game | game |  | any |
| [`credits`](#credits) | Cast and crew | context | person | person |  | detail |
| [`filmography`](#filmography) | A person's titles | context | movie, show | poster | yes | detail |
| [`episodes`](#episodes) | Episodes | context | episode | episode | yes | detail |
| [`tracks`](#tracks) | Tracks | context | track | square |  | detail |
| [`similar`](#similar) | More like this | context | movie, show | poster |  | detail |
| [`versions`](#versions) | Versions of a title | context | video | square |  | custom |
| [`listsFor`](#listsfor) | Lists a title can be in | context | collection | tile |  | custom |
| [`newList`](#newlist) | A new list's name | context | view | tile |  | custom |
| [`devices`](#devices) | Signed-in devices | personal | view | tile |  | any |
| [`year`](#year) | Your year | personal | view | tile |  | any |
| [`activity`](#activity) | What the server is doing | structure | view | tile |  | any |
| [`notices`](#notices) | Notifications | personal | view | tile |  | any |
| [`creators`](#creators) | Creators | library | person | person |  | any |
| [`comments`](#comments) | Comments | context | view | tile |  | custom |
| [`markers`](#markers) | Skip markers | context | view | tile |  | custom |
| [`canvas`](#canvas) | Canvas video | context | view | tile |  | custom |
| [`smartRule`](#smartrule) | A smart playlist's rule | context | view | tile |  | custom |
| [`libraryAdmin`](#libraryadmin) | Libraries to manage | structure | view | tile |  | custom |
| [`newLibrary`](#newlibrary) | A new library's fields | context | view | tile |  | custom |
| [`matchQueue`](#matchqueue) | Titles to check | structure | view | tile | yes | custom |
| [`matchCandidates`](#matchcandidates) | A title's possible matches | context | view | tile |  | custom |
| [`formField`](#formfield) | A field for a Form | context | view | tile |  | custom |
| [`search`](#search) | Search results | context | movie, show, album, book, game, channel, person | poster |  | search |

#### continueWatching

*Continue watching.* What the profile is part-way through, most recent first.

| Property | Type | Default | What it is |
|---|---|---|---|
| `media` | enum: all, watch, read, listen | `all` | What |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"continueWatching","media":"watch"}
```

#### nextUp

*Next up.* The next episode of every show the profile has started and not finished.

| Property | Type | Default | What it is |
|---|---|---|---|
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"nextUp"}
```

#### recommended

*Recommended for you.* Films and shows picked from what this profile has watched on this server. Nothing leaves the server.

| Property | Type | Default | What it is |
|---|---|---|---|
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"recommended","limit":20}
```

#### watchLater

*Watch later.* The profile's own Watch later list. See all opens `/collections`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"watchLater"}
```

#### liked

*Liked.* What the profile gave a thumbs up, most recently liked first: songs, films, videos, books, games.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `media` | enum: all, watch, read, listen | `all` | What |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"liked","media":"listen"}
```

#### statistics

*Statistics.* What has had the most time from the play log, most first, each card saying how long: titles, shows, or each library. A profile sees its own record; everyone's is for administrators only, and is empty for anyone else. Only titles the profile may open are named or counted.

| Property | Type | Default | What it is |
|---|---|---|---|
| `stat` | enum: titles, shows, libraries | `titles` | Show |
| `who` | enum: me, everyone | `me` | Whose. Everyone's is answered for administrators only. |
| `period` | enum: week, month, year, all | `month` | Over |
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"statistics","stat":"shows","period":"year","limit":10}
```

#### serverStatus

*Server status.* A few figures about the server: libraries, what is playing now, how long it has been up. Answered for administrators only; empty for anyone else.

| Property | Type | Default | What it is |
|---|---|---|---|
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"serverStatus"}
```

#### savedView

*A saved view.* What one of the profile's saved library views finds now. With a row repeated over pinned views, $each is each view.

| Property | Type | Default | What it is |
|---|---|---|---|
| `view` (required) | ref |  | View. The saved view's ID, or $each. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"savedView","view":"$each"}
```

#### recentlyAdded

*Recently added.* The newest things in a library, or across every library of a kind. Shows for a TV library, albums for music, the items themselves for the rest. See all opens `/library/{library}`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `kind` | enum: movies, discs, shows, anime, videos, shorts, channels, musicvideos, commercials, music, audiobooks, podcasts, radio, books, photos, games |  | Kind of library. Only libraries of this kind. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"recentlyAdded","library":"$each","limit":20}
```

#### recentlyReleased

*Recently released.* Films and shows by release date, newest first. See all opens `/library/{library}?sort=newest`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"recentlyReleased"}
```

#### topRated

*Top rated.* Films and shows by score, best first. See all opens `/library/{library}?sort=rating`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `minRating` | int 0 to 10 |  | At least. Only titles scoring this or more, out of 10. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"topRated","minRating":7,"limit":10}
```

#### random

*Random picks.* A shuffled handful of films and shows. The shuffle changes when reshuffle says.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `reshuffle` | enum: hourly, daily, weekly | `daily` | New picks |
| `watched` | enum: no, yes |  | Watched |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"random","limit":12,"watched":"no"}
```

#### browse

*A filter.* Anything the library page's filters can find: the server's own filter and sort engine. With filterable on, the page's FilterBar and its query parameters are added to it. See all opens `/library/{library}`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `genre` | string, at most 100 |  | Genre |
| `studio` | string, at most 120 |  | Studio |
| `network` | string, at most 120 |  | Network |
| `tag` | string, at most 60 |  | Tag |
| `service` | string, at most 80 |  | Streaming on. A streaming service, by its key: what it streams in the server's region. Needs streaming listings on. |
| `series` | string, at most 120 |  | Film series |
| `year` | int 0 to 2200 |  | Year |
| `decade` | int 0 to 2200 |  | Decade. The decade's first year: 1990. |
| `content` | string, at most 20 |  | Certificate. A content rating, such as PG. |
| `minRating` | int 0 to 10 |  | Score at least. Out of 10. |
| `resolution` | enum: 4k, 1080p, 720p, sd |  | Resolution |
| `watched` | enum: no, yes |  | Watched |
| `titles` | enum: video, show, disc |  | Only |
| `sort` | enum: title, added, newest, oldest, rating, shuffle |  | Order |
| `desc` | bool | `false` | Reverse the order |
| `filterable` | bool | `false` | Follows the page's filters. The page's FilterBar and query parameters narrow and sort this source. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"browse","library":"$library","genre":"Animation","sort":"rating"}
```

#### genre

*A genre.* Films and shows of one genre. See all opens `/library/{library}?genre={genre}`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `genre` (required) | string, at most 100 |  | Genre |
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `sort` | enum: title, added, newest, oldest, rating, shuffle |  | Order |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"genre","genre":"Comedy"}
```

#### studio

*A studio or network.* Films from one studio, or shows from one network.

| Property | Type | Default | What it is |
|---|---|---|---|
| `studio` | string, at most 120 |  | Studio |
| `network` | string, at most 120 |  | Network |
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `sort` | enum: title, added, newest, oldest, rating, shuffle |  | Order |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"studio","studio":"Studio Ghibli"}
```

#### tag

*A tag.* Everything carrying one tag.

| Property | Type | Default | What it is |
|---|---|---|---|
| `tag` (required) | string, at most 60 |  | Tag |
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `sort` | enum: title, added, newest, oldest, rating, shuffle |  | Order |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"tag","tag":"christmas"}
```

#### collection

*A collection.* What one collection holds: a list someone made, or a rule. $collection on a collection's page. See all opens `/collections/{collection}`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `collection` (required) | ref |  | Collection. The collection's ID, $collection or $each. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"collection","collection":12}
```

#### collections

*Collections.* The collections the profile can see, as tiles. See all opens `/collections`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"collections"}
```

#### featured

*Featured.* Recent films with a backdrop: what a Spotlight is usually made of.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"featured","limit":5}
```

#### libraries

*Libraries.* The libraries the profile can open, as tiles.

| Property | Type | Default | What it is |
|---|---|---|---|
| `kind` | enum: movies, discs, shows, anime, videos, shorts, channels, musicvideos, commercials, music, audiobooks, podcasts, radio, books, photos, games |  | Kind of library. Only libraries of this kind. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"libraries"}
```

#### genres

*Genres.* A library's genres, each opening the library filtered to it.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` (required) | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"genres","library":"$library"}
```

#### liveNow

*On TV now.* The channels the profile may watch, with what is on now and next. See all opens `/tv`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `favourites` | bool | `false` | Favourite channels only |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"liveNow","favourites":true}
```

#### recordings

*Recordings.* The profile's finished Live TV recordings, newest first. Each card carries channel and programme, for opening its More info, when the profile may still watch that channel. See all opens `/tv/recordings`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"recordings"}
```

#### books

*Books.* A books library's books, as its bookshelf has them: narrowed to a series, an author or a format, in an order. A series is in its own order unless another is chosen. With a row repeated over the library's series or authors, $each is each one; ~none is the books with none.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` (required) | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `series` | string, at most 300 |  | Series. Only this series. $each in a row repeated over bookSeries; ~none for books in no series. |
| `author` | string, at most 300 |  | Author. Only this author's books. $each in a row repeated over bookAuthors; ~none for books with no author. |
| `format` | enum: all, epub, pdf, cbz | `all` | Format. Only what one reader opens. |
| `sort` | enum: title, author, added, year, series |  | Order. Left empty: the series' order with a series, title otherwise. |
| `reading` | enum: reading, finished, unread |  | Reading state. What the profile has started and not finished, finished, or never opened. |
| `filterable` | bool | `false` | Follows the page's filters. The page's FilterBar (format, reading state, order) narrows and sorts this source. |
| `desc` | bool | `false` | Reverse the order |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"books","library":"$library","series":"$each"}
```

#### formats

*A book's formats.* The formats one book is in: the one that opens, then the other files with the same name beside it, each with its size. A book kept in more than one format shows them all; the others are not opened by the server (the best is used: comics first, then EPUB, then PDF). Only formats and sizes, never paths.

| Property | Type | Default | What it is |
|---|---|---|---|
| `item` (required) | ref |  | Book. $item on a book's page. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"formats","item":"$item"}
```

#### albums

*Albums.* A music or audiobook library's albums. See all opens `/library/{library}`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` (required) | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `sort` | enum: added, played, title, artist, year | `added` | Order |
| `artist` | string, at most 200 |  | Artist. Only this artist's albums. $artist on an artist's page. |
| `genre` | string, at most 100 |  | Genre |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"albums","library":"$each","sort":"played"}
```

#### artists

*Artists.* A music library's album artists, or an audiobook library's authors. See all opens `/library/{library}`.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` (required) | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"artists","library":"$library"}
```

#### continuePlaying

*Continue playing.* Games the profile has played, most recent first.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"continuePlaying"}
```

#### credits

*Cast and crew.* A title's people: the cast in billing order, the guest stars (an episode's, or a season's with the most episodes first), then the crew by department, from the best source that names each part. Each card's subtitle is the part played or the job; its group is the part (cast, guests or a department).

| Property | Type | Default | What it is |
|---|---|---|---|
| `item` | ref |  | Title. $item on a film's or an episode's page. |
| `show` | ref |  | Show. $show on a show's page (when there is no item). |
| `season` | ref |  | Season. $season: a season's guest stars on a show's page opened with ?season=. |
| `part` | enum: all, cast, guests, directing, writing, production, camera, lighting, editing, sound, art, costume, effects, crew | `all` | Part |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |
| `programme` | string, at most 40 |  | Programme. A WittyTV programme: a channel's key and a moment in it, tun-12@1767225600. Its title's credits in the library when the profile may see it, else the guide's own. |

```json
{"type":"credits","item":"$item"}
```

#### filmography

*A person's titles.* The films and shows in this library a person is in or worked on, newest first, that the profile may see; one part of them (their acting, or one department's work), or all. Each card's subtitle is the year and the part or job; a show's says how many episodes.

| Property | Type | Default | What it is |
|---|---|---|---|
| `person` (required) | ref |  | Person. $person on a person's page. |
| `part` | string, at most 40 |  | Part. all (the default), acting, or a department: directing, writing, production, camera, lighting, editing, sound, art, costume, effects, crew; $each in a row repeated over personParts. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"filmography","person":"$person","part":"$each"}
```

#### episodes

*Episodes.* One season of a show. Left to itself it picks the season of the next episode to watch.

| Property | Type | Default | What it is |
|---|---|---|---|
| `show` (required) | ref |  | Show. $show on a show's or an episode's page. |
| `season` | ref |  | Season. A season number, or $season; empty picks one. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"episodes","show":"$show"}
```

#### tracks

*Tracks.* An album's songs or a book's chapters, in order; without an album, every song by the artist, by title.

| Property | Type | Default | What it is |
|---|---|---|---|
| `library` (required) | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `album` | string, at most 300 |  | Album. $album on an album's page. Left out: every album's (an artist's page). |
| `artist` | string, at most 300 |  | Album artist. $artist on an album's or an artist's page. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"tracks","library":"$library","artist":"$artist","album":"$album"}
```

#### similar

*More like this.* Titles in the same library sharing the title's first genre, best scored first, without the title itself.

| Property | Type | Default | What it is |
|---|---|---|---|
| `item` (required) | ref |  | Title. $item on a film's page, $show on a show's. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"similar","item":"$item","limit":12}
```

#### versions

*Versions of a title.* The files one title plays from, its own and the versions attached to it, each a row that plays that file with its label and quality, its length and size.

| Property | Type | Default | What it is |
|---|---|---|---|
| `item` (required) | ref |  | Title. $item on a page opened about a title. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"versions","item":"$item"}
```

#### listsFor

*Lists a title can be in.* The profile's own lists (Watch later and the lists it made), each a switch row: on when the title is in it, and flipping it adds or takes the title out.

| Property | Type | Default | What it is |
|---|---|---|---|
| `item` (required) | ref |  | Title. $item on a page opened about a title. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"listsFor","item":"$item"}
```

#### newList

*A new list's name.* One row holding a field for the name of a new list, for a Form whose command is collection.create.

```json
{"type":"newList"}
```

#### devices

*Signed-in devices.* Where the profile is signed in: this device first, then the others with when each was last used, a row that signs it out after asking.

| Property | Type | Default | What it is |
|---|---|---|---|
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"devices"}
```

#### year

*Your year.* The profile's year from its own play log: the totals, or the top titles, shows, artists or genres, or the months.

| Property | Type | Default | What it is |
|---|---|---|---|
| `part` | enum: totals, months, titles, shows, artists, genres | `totals` | Part |
| `year` | int 0 to 3000 |  | Year. 0 is this year. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"year","part":"titles","limit":10}
```

#### activity

*What the server is doing.* The tasks running now, with how far each has got, or those just finished. What the profile may see of them follows its limits.

| Property | Type | Default | What it is |
|---|---|---|---|
| `part` | enum: now, history | `now` | Part |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"activity","part":"now"}
```

#### notices

*Notifications.* What the profile may want to know: a reminder that a programme is on, and for an administrator that a new version of the server is ready.

| Property | Type | Default | What it is |
|---|---|---|---|
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"notices"}
```

#### creators

*Creators.* The creators of the libraries the profile can open (shorts, channels, music videos), or one by item: a person row with how many videos, and a Subscribe switch (not for a Guest).

| Property | Type | Default | What it is |
|---|---|---|---|
| `item` | ref |  | Creator. One creator's ID, or $item on a creator's page. |
| `subscribed` | bool | `false` | Only those subscribed to |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"creators","subscribed":true}
```

#### comments

*Comments.* What people wrote about a title (item): the writer, the text and when; a row the profile may delete (its own, or any for an administrator) has a Delete that asks first.

| Property | Type | Default | What it is |
|---|---|---|---|
| `item` (required) | ref |  | Title. $item on a page opened about a title. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"comments","item":"$item"}
```

#### markers

*Skip markers.* Where the intro, recap, credits and breaks of a title (item) are, as the player's Skip uses them; None where there is no marker.

| Property | Type | Default | What it is |
|---|---|---|---|
| `item` (required) | ref |  | Title. $item on a page opened about a title. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"markers","item":"$item"}
```

#### canvas

*Canvas video.* Whether a title (item) has a looping silent video behind its page, and for an administrator whether one is being cut from the film.

| Property | Type | Default | What it is |
|---|---|---|---|
| `item` (required) | ref |  | Title. $item on a page opened about a title. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"canvas","item":"$item"}
```

#### smartRule

*A smart playlist's rule.* The fields of a smart playlist as controls for a Form whose command is collection.createSmart: the name, then the rule's fields, named rule.<key> (a name ending [] is a comma-separated list).

```json
{"type":"smartRule"}
```

#### libraryAdmin

*Libraries to manage.* Administrators: every library with its kind, how many titles and when it was last scanned, each row scanning it again. Empty for anyone else.

```json
{"type":"libraryAdmin"}
```

#### newLibrary

*A new library's fields.* Administrators: the name, kind and folder of a new library as controls for a Form whose command is library.create.

```json
{"type":"newLibrary"}
```

#### matchQueue

*Titles to check.* Administrators: titles whose match is missing or doubtful (or in the state asked for), with how sure the match is and why, each row opening its title's match page.

| Property | Type | Default | What it is |
|---|---|---|---|
| `state` | enum: attention, unmatched, low, manual, none, all | `attention` | Which |
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"matchQueue","state":"attention"}
```

#### matchCandidates

*A title's possible matches.* Administrators: what the online sources suggest for a title (item: an item's ID or show-<n>), best first, each with how sure and why, a row matching the title to it after asking; last, a row that takes the match off.

| Property | Type | Default | What it is |
|---|---|---|---|
| `item` (required) | ref |  | Title. $item. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"matchCandidates","item":"$item"}
```

#### formField

*A field for a Form.* One row holding a text field named name, for a Form that sends it as a command param.

| Property | Type | Default | What it is |
|---|---|---|---|
| `name` (required) | string, at most 40 |  | Name. The param's name. |
| `label` | string, at most 80 |  | Label |
| `placeholder` | string, at most 80 |  | Prompt |
| `multiline` | bool | `false` | Several lines |
| `maxLength` | int 1 to 2000 | `200` | Longest |

```json
{"type":"formField","name":"text","label":"Comment","multiline":true}
```

#### search

*Search results.* What a search finds in one library, among the channels, or among the people credited on titles the profile may see. Nothing under two characters.

| Property | Type | Default | What it is |
|---|---|---|---|
| `query` (required) | ref |  | Search. $query on the search page. |
| `library` | library |  | Library. One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries. |
| `in` | enum: library, channels, people | `library` | Look in |
| `filterable` | bool | `false` | Follows the page's filters. The page's FilterBar (year, score) narrows what a library search finds. |
| `limit` | int 0 to 500 |  | Most items. How many items it gives in all. 0 has no limit of its own; the device still pages it. |

```json
{"type":"search","query":"$query","library":"$each"}
```

Every source also takes `map`: `{"subtitle": "studio"}` puts another field of the item into a card's `title`, `subtitle` or `synopsis`. The fields are `title`, `subtitle`, `overview`, `tagline`, `year`, `studio`, `contentRating`, `artist`, `showTitle`, `count`, `number`, `runtime`, `genres`.

### Conditions

| Field | Type | True when |
|---|---|---|
| `all` | list of conditions | every one holds |
| `any` | list of conditions | at least one holds |
| `not` | condition | it doesn't hold; `{"not": {}}` never holds |
| `formFactor` | enumList: tv, phone, tablet, desktop, web | Device. The kind of device. |
| `platform` | enumList: web, tvos, ios, android, androidtv, desktop | App. Which app. |
| `profileKind` | enumList: adult, kids, guest | Kind of profile |
| `admin` | bool | Administrator. Only administrators (on) or only everyone else (off). |
| `feature` | string, at most 64 | Server feature. A name from /capabilities features, such as live-tv. |
| `module` | string, at most 64 | Module installed. A module's ID, such as wittyplay. |
| `can` | enum: games, recordings, downloads, sharing, comments, syncplay, remoteControl, remote, editProfile, editLayout, liveTv | Profile may use. One of the profile's parental feature switches. |
| `hasLibrary` | enum: movies, discs, shows, anime, videos, shorts, channels, musicvideos, commercials, music, audiobooks, podcasts, radio, books, photos, games | Has a library of kind. The profile can open at least one such library. |
| `libraryKind` | enum: movies, discs, shows, anime, videos, shorts, channels, musicvideos, commercials, music, audiobooks, podcasts, radio, books, photos, games | This library is of kind. On a library page. |
| `capability` | string, at most 64 | Device capability. Something the client said it has: hdr, touch, pointer, keyboard. |
| `flag` | enum: setupIncomplete, scanning, librariesEmpty, hasQuery, screenTimeUp | The server says |
| `minWidth` | int 0 to 10000 | Window at least. Points wide. |
| `maxWidth` | int 0 to 10000 | Window at most. Points wide. |
| `timeFrom` | string, at most 5 | From time of day. HH:MM on the server's clock. |
| `timeTo` | string, at most 5 | Until time of day. HH:MM; a range may pass midnight. |
| `weekday` | enumList: mon, tue, wed, thu, fri, sat, sun | Days |
| `locale` | string, at most 35 | Language. A language tag; en matches en-GB. |

### Page kinds

| Kind | What it is | Route | Query parameters | Variables for sources | Narrowed by |
|---|---|---|---|---|---|
| `home` | The first page of the app. | `/` | tab |  |  |
| `libraries` | Every library the profile can open. | `/libraries` | tab |  |  |
| `library` | One library's own page. | `/library/{library}` | library, tab, sort, desc, genre, decade, year, content, resolution, watched, studio, network, tag, service, series, kind, disc, minRating, audio, subtitle, format, reading, edition, search, seed | $library | library, libraryKind |
| `detail` | The page of one film, show, episode, album, artist, book, game or collection, or of a person in the cast or crew. | `/item/{item}` | item, show, season, collection, library, artist, album, person, tab | $item, $show, $season, $collection, $library, $artist, $album, $person | mediaKind, library, libraryKind |
| `search` | Search across everything. | `/search` | q, tab, year, minRating | $query |  |
| `livetv` | Channels and the guide. | `/tv` | tab |  |  |
| `music` | Every music library together. | `/music` | tab |  |  |
| `photos` | Every photo library together. | `/photos` | tab |  |  |
| `books` | Books, comics and audiobooks together: the book libraries and the audiobook libraries. | `/books` | tab |  |  |
| `games` | Every games library together. | `/games` | tab |  |  |
| `collections` | The profile's collections and lists. | `/collections` | tab |  |  |
| `custom` | Any other page: a family film night, a page for one collection. It may be opened about something: a title (item), a studio, network or category (name, with of), which its sources read as $item, $name and $of; a module page in a Settings slot of people.profile is opened about a profile (item), read as $item. | `/page/{id}` | tab, item, name, of | $item, $name, $of |  |

### Routes

What shows each place in the app: an SDUI page, asked for with the parameters given (`{name}` is from the route, `?name` from its query), or a screen the app draws itself.

| Route | Shown by | Parameters |
|---|---|---|
| `/` | page `home` |  |
| `/libraries` | page `libraries` |  |
| `/library/{library}` | page `library` | library={library} |
| `/item/{item}` | page `detail` | item={item} |
| `/show/{show}` | page `detail` | show={show} |
| `/album/{library}` | page `detail` | album=?album, artist=?artist, library={library} |
| `/artist/{library}` | page `detail` | artist=?name, library={library} |
| `/collections/{collection}` | page `detail` | collection={collection} |
| `/person/{person}` | page `detail` | person={person} |
| `/collections` | page `collections` |  |
| `/search` | page `search` | q=?q |
| `/tv` | page `livetv` |  |
| `/music` | page `music` |  |
| `/photos` | page `photos` |  |
| `/books` | page `books` |  |
| `/games` | page `games` |  |
| `/studio/{name}` | page `studio` | name={name} |
| `/network/{name}` | page `network` | name={name} |
| `/category/{name}` | page `category` | name={name} |
| `/versions/{item}` | page `versions` | item={item} |
| `/save-to/{item}` | page `save-to` | item={item} |
| `/devices` | page `devices` |  |
| `/creators` | page `creators` |  |
| `/creator/{item}` | page `creator` | item={item} |
| `/comments/{item}` | page `comments` | item={item} |
| `/markers/{item}` | page `markers` | item={item} |
| `/canvas/{item}` | page `canvas` | item={item} |
| `/smart-list` | page `smart-list` |  |
| `/manage/libraries` | page `manage-libraries` |  |
| `/manage/matches` | page `fix-matches` |  |
| `/manage/match/{item}` | page `match` | item={item} |
| `/year` | page `year` |  |
| `/activity` | page `activity` |  |
| `/notifications` | page `notifications` |  |
| `/page/{id}` | page `{id}` |  |
| `/settings` | native `settings` |  |
| `/settings/{section}` | native `settings` |  |
| `/tv/guide` | native `guide` |  |
| `/tv/recordings` | native `guide` |  |
| `/tv/watch/{key}` | native `livePlayer` |  |
| `/watch/{item}` | native `player` |  |
| `/play/{item}` | native `player` |  |
| `/read/{item}` | native `reader` |  |
| `/game/{item}` | native `game` |  |
| `/you` | native `profiles` |  |
| `/pair` | native `pair` |  |

### Native screens

A menu entry (`type: native`) may open the screens marked *menu*; the others need a title or a channel and are opened by actions.

| Screen | What it is | Route | Menu |
|---|---|---|---|
| `settings` | The app's own Settings. Always in the navigation; never an SDUI page. | `/settings` | menu |
| `guide` | The guide's grid of channels and times. | `/tv/guide` | menu |
| `profiles` | The profile's own page and the profile switcher. | `/you` | menu |
| `downloads` | What is kept on this device. | `/downloads` | menu |
| `remote` | Remote control of another device. | `/remote` | menu |
| `nowPlaying` | The music player. | `/now-playing` | menu |
| `pair` | Sign another device in. | `/pair` | menu |
| `collections` | The app's own collections manager, where it has one; else the collections page. | `/collections` | menu |
| `player` | The video player. | `/watch/{item}` | no |
| `livePlayer` | A channel, playing. | `/tv/watch/{key}` | no |
| `reader` | The book and comic reader. | `/read/{item}` | no |
| `game` | A game's page and player. | `/game/{item}` | no |
| `download` | Keep a copy of a title on this device (action params: item). | `` | no |
| `share` | The share sheet for a title (action params: item). | `` | no |
| `playOn` | The device picker for a title (action params: item). | `` | no |
| `watchParty` | Start a watch party for a title (action params: item). | `` | no |

### Commands

| `op` | What it does | Request |
|---|---|---|
| `watchLater.add` | Adds the title to the profile's Watch later list. | `POST /api/v1/collections/watch-later/items` with `{"items":["{item}"]}` |
| `watchLater.remove` | Takes the title off the profile's Watch later list. | `DELETE /api/v1/collections/watch-later/items` with `{"items":["{item}"]}` |
| `watched.set` | Marks the title watched; for a show, every episode. | `POST /api/v1/items/{item}/progress` with `{"watched":true}` |
| `watched.clear` | Forgets the profile's progress in the title; for a show, in every episode. | `DELETE /api/v1/items/{item}/progress` |
| `selectTab` | Asks for the page again with tab=<params.tab>, or shows the tab's content when the client already has it. |  |
| `control.open` | Client-handled, from a Dock: params.control is sort, filter, view or jump and params.target the Grid. sort and filter open the page's FilterBar choices for the target; view switches it between its grid and a list and remembers it for the profile on the device; jump opens a letter picker (A to Z and #) and brings the grid to the first title starting with the letter by asking its more.url until one has arrived, at most 20 pages. |  |
| `disc.choose` | Discs: an administrator makes params.ttn (a DVD's title or a Blu-ray's playlist) the disc's film (item), for everyone, instead of the engine's pick. | `PUT /api/v1/items/{item}/disc/choice` with `{"ttn":"{ttn}"}`; `ttn` (int, required) |
| `disc.clearChoice` | Discs: forgets the film an administrator chose for the disc (item): the engine's pick plays again. | `DELETE /api/v1/items/{item}/disc/choice` |
| `disc.rank` | Discs: an administrator has the disc's module work out which title of the disc (item) is the film again. | `POST /api/v1/items/{item}/disc/analysis` with `{"secondOpinion":false}` |
| `disc.compare` | Blu-rays: an administrator has a libbluray installed on the server compare its reading of the disc (item); the setting has to allow it. | `POST /api/v1/items/{item}/disc/analysis` with `{"secondOpinion":true}` |
| `wittyplay.room.create` | WittyPlay Online: opens the game being played (item: its session) to the friends who may see it, so they join with no code. Answers the host's code. | `POST /api/play/{item}/room` with `{"open":true}` |
| `wittyplay.room.join` | WittyPlay Online: takes a seat in an open room (item: its session) or, with params.code, the room that code belongs to (no item). | `POST /api/play/join` with `{"room":"{item}","code":"{code}"}`; `code` (string) |
| `wittyplay.room.invite` | WittyPlay Online: a join code for the game being played (item: its session), good for a short while, which a phone's QR join link carries. Answers {code, expiresAt}. | `POST /api/play/{item}/invite` |
| `wittyplay.state.save` | WittyPlay: saves the game being played (item: its session) into params.slot. | `POST /api/play/{item}/action` with `{"type":"save","slot":"{slot}"}`; `slot` (int, required) |
| `wittyplay.state.load` | WittyPlay: loads params.slot into the game being played (item: its session). | `POST /api/play/{item}/action` with `{"type":"load","slot":"{slot}"}`; `slot` (int, required) |
| `wittyplay.state.delete` | WittyPlay: deletes the profile's save in params.slot of the game (item: the game's ID, not a session). | `DELETE /api/games/{item}/saves/{slot}`; `slot` (int, required) |
| `wittyplay.controller.assign` | WittyPlay Online: the host puts the profile params.user in seat params.seat of the game being played (item: its session); a held seat is swapped. | `PUT /api/play/{item}/seats/{user}` with `{"seat":"{seat}"}`; `user` (string, required); `seat` (int, required) |
| `wittyplay.room.leave` | WittyPlay Online: frees seat params.user in the game being played (item: its session). The host may free anyone's seat; anyone may leave their own. | `DELETE /api/play/{item}/seats/{user}`; `user` (string, required) |
| `wittyplay.room.close` | WittyPlay Online: closes the game being played (item: its session) to friends. The host's own seat and anyone already in stay. | `POST /api/play/{item}/room` with `{"open":false}` |
| `wittyplay.online.set` | WittyPlay Online: what this profile lets friends know. params.sharePlaying (friends see the game) and params.appearOnline (appears online at all). | `PUT /api/games/online/me` with `{"sharePlaying":"{sharePlaying}","appearOnline":"{appearOnline}"}`; `sharePlaying` (bool, required); `appearOnline` (bool, required) |
| `wittyplay.online.share` | WittyPlay Online: params.value, whether friends see the game this profile plays. | `PUT /api/games/online/me` with `{"sharePlaying":"{value}"}`; `value` (bool, required) |
| `wittyplay.online.appear` | WittyPlay Online: params.value, whether this profile appears online to friends at all. | `PUT /api/games/online/me` with `{"appearOnline":"{value}"}`; `value` (bool, required) |
| `wittyplay.kids.friends` | WittyPlay Online, administrators: replaces the profiles a Kids profile (item: its ID) sees and is seen by with params.friends. | `PUT /api/games/online/kids/{item}` with `{"friends":"{friends}"}`; `friends` (ints, required) |
| `wittyplay.screenshot.keep` | WittyPlay: keeps a screenshot of the game being played (item: its session) in the profile's Memories. | `POST /api/play/{item}/screenshot` |
| `wittyplay.memory.delete` | WittyPlay: deletes one of the profile's kept screenshots (item: the game's ID, params.name its name). | `DELETE /api/games/{item}/shots/{name}`; `name` (string, required) |
| `collection.createSmart` | Makes a smart playlist of the profile's own: params.name and params.rule, an object (the rule: libraryTypes, genres, search, yearFrom, yearTo, ratingMin, addedWithinDays, watched, sort, limit...). A Form fills the rule from the controls named rule.<key>. | `POST /api/collections` with `{"name":"{name}","kind":"smart","rule":"{rule}"}`; `name` (string, required); `rule` (object, required) |
| `creator.subscribe` | Subscribes the profile to a creator (item), or takes the subscription off (params.on false). | `PUT /api/creators/{item}/subscribe` with `{"on":"{on}"}`; `on` (bool, required) |
| `comment.add` | Adds a comment to a title (item); params.text is up to 2000 characters. Not for Kids or Guest profiles. | `POST /api/items/{item}/comments` with `{"body":"{text}"}`; `text` (string, required) |
| `comment.delete` | Deletes a comment (item: its ID): its writer's, or any as an administrator. | `DELETE /api/comments/{item}` |
| `library.scan` | Looks through the library's folders for new and changed files (administrators): the library is the item. | `POST /api/libraries/{item}/scan` |
| `library.create` | Administrators: adds a library named params.name of kind params.type over the folder params.path, and scans it. | `POST /api/libraries` with `{"name":"{name}","type":"{type}","paths":["{path}"]}`; `name` (string, required); `type` (string, required); `path` (string, required) |
| `meta.match` | Administrators: matches the title params.target (item or show) params.title to params.ref at the source params.source. | `POST /api/meta/{target}/{title}/match` with `{"source":"{source}","id":"{ref}"}`; `target` (string, required); `title` (string, required); `source` (string, required); `ref` (string, required) |
| `meta.unmatch` | Administrators: leaves the title params.target params.title unmatched; scans don't look it up again. | `DELETE /api/meta/{target}/{title}/match`; `target` (string, required); `title` (string, required) |
| `collection.add` | Puts the title (item) in the profile's list params.collection. | `POST /api/collections/{collection}/items` with `{"items":["{item}"]}`; `collection` (string, required) |
| `collection.remove` | Takes the title (item) off the profile's list params.collection. | `DELETE /api/collections/{collection}/items` with `{"items":["{item}"]}`; `collection` (string, required) |
| `collection.create` | Makes a list of the profile's own, named params.name. | `POST /api/collections` with `{"name":"{name}","kind":"manual"}`; `name` (string, required) |
| `session.end` | Signs out the device params.session. | `DELETE /api/sessions/{session}`; `session` (string, required) |
| `sessions.endOthers` | Signs out every device of the profile but this one. | `DELETE /api/sessions` |
| `history.clear` | Forgets what the profile watched and listened to. It cannot be undone. | `DELETE /api/me/history` |
| `rewind.open` | Client-handled, in the game player: opens the player's rewind panel. No request. |  |
| `controller.remap` | Client-handled, in the game player: opens the player's own controller-mapping panel. No request. |  |
| `refresh` | Asks for the page again. |  |

### Experiences

The menu's style on each form factor, the theme and WittyTV look each wears by default, and what the server changes about every page. Menu styles: `sidebar`, `rail`, `railDrawer`, `bottomTabs`, `topTabs`, `floatingGlassBar`, `cableBar`, `bevelBar`, `teleKeys`, `crossbar`, `ornamentTabs`, `tvTopMenu`, `tvSideMenu`.

| Experience | What it is | TV | Phone | Tablet | Desktop | Web | Headset | Theme | WittyTV | Pages |
|---|---|---|---|---|---|---|---|---|---|---|
| `lumen` | **Lumen.** Top pills, a billboard spotlight and rows that peek at the next card. The default. | tvTopMenu | bottomTabs | topTabs | topTabs | topTabs | ornamentTabs | default | modern | spotlight: billboard |
| `rail` | **Rail.** An icon rail that opens into a full menu and a drawer for each library, with title-on-art cards. | tvSideMenu | bottomTabs | railDrawer | railDrawer | railDrawer | ornamentTabs | cinema | classic | spotlight: banner, plate cards for films and shows |
| `stage` | **Stage.** A see-through menu over a full-bleed spotlight, then rows of wide cards that overlap the art. | tvTopMenu | bottomTabs | topTabs | topTabs | topTabs | ornamentTabs | midnight | bigpicture | spotlight: billboard, landscape cards for films and shows, comfortable, menu over the page |
| `aero` | **Aero.** Glossy glass bars, bevelled pills, sky and leaf gradients and rounded corners. Gloss and gradients, no heavy blur. | tvTopMenu | bottomTabs | floatingGlassBar | floatingGlassBar | floatingGlassBar | ornamentTabs | aeroday | modern | spotlight: billboard |
| `classic` | **Classic.** The menu down the side, a tab bar on a phone and the top menu on a television. Today's design, polished. | tvTopMenu | bottomTabs | sidebar | sidebar | sidebar | ornamentTabs | default | modern | spotlight: billboard |
| `classicCable` | **Classic Cable.** Flat blue rows, yellow highlights and coloured band headings, like a cable guide. | cableBar | bottomTabs | cableBar | cableBar | cableBar | ornamentTabs | cable | classic | rows and grids as lists, spotlight: row, landscape cards for films and shows, band headings |
| `classicSatellite` | **Classic Satellite.** Silver bevelled bars, orange highlights and logo tiles for channels. | bevelBar | bottomTabs | bevelBar | bevelBar | bevelBar | ornamentTabs | satellite | satellite | spotlight: billboard, underline headings |
| `compact` | **Compact.** A rail and a line per title: the most on the screen at once, for power users and long libraries. | tvSideMenu | bottomTabs | rail | rail | rail | ornamentTabs | oled | compact | rows and grids as lists, spotlight: row, compact |
| `paper` | **Paper.** Top tabs, big serif type and generous space, on warm paper. | tvTopMenu | topTabs | topTabs | topTabs | topTabs | ornamentTabs | paper | listings | spotlight: banner, underline headings |
| `marquee` | **Marquee.** One big title at a time in a centred carousel of posters. | tvTopMenu | bottomTabs | topTabs | topTabs | topTabs | ornamentTabs | cinema | bigpicture | spotlight: carousel, cards l |
| `crossbar` | **Crossbar.** A horizontal bar of categories with a vertical list under the chosen one. Games and music first. | crossbar | bottomTabs | crossbar | crossbar | crossbar | ornamentTabs | evergreen | satellite | spotlight: row, Home as a cross bar |
| `bigpicture` | **Big picture.** Huge tiles, few choices and the strongest focus, with larger type and targets. | tvTopMenu | bottomTabs | topTabs | topTabs | topTabs | ornamentTabs | contrast | bigpicture | grids as rows, spotlight: billboard, cards l, tile cards for films and shows |
| `dashboard` | **Dashboard.** A tile grid of widgets: continue watching, on TV now, statistics, server status, a clock. Every tile movable. | tvSideMenu | bottomTabs | rail | rail | rail | ornamentTabs | midnight | outline | spotlight: row, Home as widgets |
| `retro` | **Retro.** Teletext and CRT: block type, eight colours and colour-coded keys. Scanlines are off until turned on. | teleKeys | bottomTabs | teleKeys | teleKeys | teleKeys | ornamentTabs | teletext | teletext | rows and grids as lists, spotlight: row, block cards for films and shows, band headings |

### Icons

`home`, `library`, `movie`, `show`, `tv`, `live`, `guide`, `record`, `music`, `album`, `artist`, `mic`, `radio`, `podcast`, `book`, `audiobook`, `comic`, `photo`, `video`, `disc`, `game`, `collection`, `list`, `folder`, `search`, `settings`, `user`, `users`, `kids`, `star`, `heart`, `clock`, `history`, `calendar`, `sparkles`, `fire`, `shuffle`, `play`, `download`, `share`, `globe`, `info`, `warning`, `check`, `plus`, `grid`, `filter`, `sort`, `plug`, `chart`, `devices`, `more`

### Placeholders

| Placeholder | Becomes |
|---|---|
| `{user}` | The profile's name |
| `{server}` | The server's name |
| `{greeting}` | Good morning, afternoon or evening |
| `{library}` | The page's library |
| `{title}` | The page's title, or a source's first item |
| `{query}` | What was searched for |
| `{each}` | What a repeated section is repeated over |
| `{count}` | How many items a source gave |
| `{reason}` | Why a source chose its items (a row's subtitle) |
| `{genre}` | The source's genre |
| `{studio}` | The source's studio |
| `{tag}` | The source's tag |

### Limits

| Limit | Value |
|---|---|
| `maxChildren` | 60 |
| `maxDepth` | 6 |
| `maxIdLength` | 64 |
| `maxNavItems` | 40 |
| `maxPageBytes` | 262144 |
| `maxPages` | 200 |
| `maxPersonalBytes` | 8192 |
| `maxSections` | 200 |
| `maxSourceLimit` | 500 |
| `maxVariants` | 8 |

### Built-in pages

| Page | Kind | For | Sections |
|---|---|---|---|
| `home` | home | every home page | `setup` (Notice), `welcome` (Notice), `hero` (Hero of featured), `libraries` (Shelf of libraries), `jump` (Shelf of continueWatching), `onnow` (Shelf of liveNow), `foryou` (Shelf of recommended), `nextup` (Shelf of nextUp), `view` (Shelf of savedView, per pinnedView), `recent` (Shelf of recentlyAdded, per librarie), `shuffle` (Banner of random) |
| `libraries` | libraries | every libraries page | `all` (Grid of libraries) |
| `library` | library | every library page | `all` (Grid of recentlyAdded) |
| `manage-libraries` | custom | every custom page | `not` (Notice), `all` (List of libraryAdmin), `add` (Form), `fields` (List of newLibrary) |
| `fix-matches` | custom | every custom page | `not` (Notice), `tabs` (Tabs), `check` (Tab), `queue` (List of matchQueue), `byhand` (Tab), `manual` (List of matchQueue), `left` (Tab), `none` (List of matchQueue) |
| `match` | custom | every custom page | `not` (Notice), `lead` (Text), `candidates` (List of matchCandidates) |
| `creators` | custom | every custom page | `tabs` (Tabs), `all` (Tab), `everyone` (List of creators), `subscribed` (Tab), `mine` (List of creators) |
| `creator` | custom | every custom page | `who` (List of creators) |
| `comments` | custom | every custom page | `all` (List of comments), `write` (Form), `text` (List of formField), `readonly` (Text) |
| `markers` | custom | every custom page | `lead` (Text), `marks` (List of markers), `pick` (Button) |
| `canvas` | custom | every custom page | `lead` (Text), `state` (List of canvas) |
| `smart-list` | custom | every custom page | `lead` (Text), `rule` (Form), `fields` (List of smartRule) |
| `studio` | custom | every custom page | `films` (Grid of browse), `shows` (Grid of browse) |
| `network` | custom | every custom page | `films` (Grid of browse), `shows` (Grid of browse) |
| `category` | custom | every custom page | `films` (Grid of browse), `shows` (Grid of browse) |
| `versions` | custom | every custom page | `lead` (Text), `files` (List of versions) |
| `save-to` | custom | every custom page | `lead` (Text), `lists` (List of listsFor), `new` (Form), `name` (List of newList) |
| `devices` | custom | every custom page | `lead` (Text), `list` (List of devices), `others` (Button) |
| `year` | custom | every custom page | `totals` (List of year), `months` (List of year), `titles` (List of year), `shows` (List of year), `artists` (List of year), `genres` (List of year), `clear` (Button) |
| `activity` | custom | every custom page | `tabs` (Tabs), `now` (Tab), `running` (List of activity), `history` (Tab), `done` (List of activity) |
| `notifications` | custom | every custom page | `recent` (List of notices) |
| `library-movies` | library | movies | `filters` (FilterBar), `all` (Grid of browse) |
| `library-shows` | library | shows | `filters` (FilterBar), `all` (Grid of browse) |
| `library-anime` | library | anime | `filters` (FilterBar), `all` (Grid of browse) |
| `library-music` | library | music | `played` (Shelf of albums), `views` (Tabs), `albums` (Tab), `albums-grid` (Grid of albums), `artists` (Tab), `artists-grid` (Grid of artists) |
| `library-audiobooks` | library | audiobooks | `listening` (Shelf of continueWatching), `all` (Grid of albums) |
| `library-podcasts` | library | podcasts | `listening` (Shelf of continueWatching), `all` (Grid of albums) |
| `library-photos` | library | photos | `timeline` (NativeView), `all` (Grid of recentlyAdded) |
| `library-books` | library | books | `reading` (Shelf of continueWatching), `shelf` (Tabs), `books` (Tab), `filters` (FilterBar), `all` (Grid of books), `series` (Tab), `in-series` (Shelf of books, per bookSerie), `authors` (Tab), `by-author` (Shelf of books, per bookAuthor) |
| `library-games` | library | games | `playing` (Shelf of continuePlaying), `all` (Grid of recentlyAdded) |
| `detail` | detail | every detail page | `header` (DetailHeader), `credits` (Credits of credits), `similar` (Shelf of similar) |
| `detail-show` | detail | show | `header` (DetailHeader), `credits` (Credits of credits), `episodes` (SeasonList of episodes), `similar` (Shelf of similar) |
| `detail-episode` | detail | episode | `header` (DetailHeader), `credits` (Credits of credits), `season` (Shelf of episodes) |
| `detail-album` | detail | album | `header` (DetailHeader), `tracks` (TrackList of tracks), `more` (Shelf of albums) |
| `detail-artist` | detail | artist | `header` (DetailHeader), `albums` (Shelf of albums), `tracks` (TrackList of tracks) |
| `detail-collection` | detail | collection | `header` (DetailHeader), `all` (Grid of collection) |
| `detail-person` | detail | person | `header` (DetailHeader), `titles` (Shelf of filmography, per personPart) |
| `search` | search | every search page | `q` (SearchBox), `idle` (Text), `narrow` (FilterBar), `scope` (Tabs), `all` (Tab), `found` (Shelf of search, per librarie), `people` (Shelf of search), `channels` (Shelf of search), `movies` (Tab), `found-movies` (Shelf of search, per librarie), `shows` (Tab), `found-shows` (Shelf of search, per librarie), `music` (Tab), `found-music` (Shelf of search, per librarie), `people-tab` (Tab), `people-only` (Shelf of search), `tv` (Tab), `channels-only` (Shelf of search) |
| `livetv` | livetv | every livetv page | `guide` (NativeView), `favourites` (Shelf of liveNow), `recordings` (Shelf of recordings), `channels` (Grid of liveNow) |
| `music` | music | every music page | `listening` (Shelf of continueWatching), `played` (Shelf of albums, per librarie), `added` (Shelf of albums, per librarie), `artists` (Shelf of artists, per librarie) |
| `photos` | photos | every photos page | `recent` (Shelf of recentlyAdded, per librarie) |
| `books` | books | every books page | `reading` (Shelf of continueWatching), `recent` (Shelf of recentlyAdded, per librarie) |
| `games` | games | every games page | `playing` (Shelf of continuePlaying), `recent` (Shelf of recentlyAdded, per librarie) |
| `collections` | collections | every collections page | `later` (Shelf of watchLater), `all` (Grid of collections) |

### Presets

| Preset | Kind | What it is |
|---|---|---|
| `home-default` | page | **The built-in Home.** Home as it ships: spotlight, libraries, jump back in, on TV now, recommendations, next up, pinned views, a row per library and a shuffle. |
| `home-streaming` | page | **Streaming-style Home.** A big spotlight, continue watching, recommendations, a numbered top ten, new releases and a row per library. |
| `home-minimal` | page | **Minimal Home.** A greeting, what you were watching, and large tiles for the libraries. Nothing else. |
| `home-kids` | page | **Home with a kids' variant.** The built-in Home for everyone, and a simpler one with large cards for Kids profiles. |
| `home-music` | page | **Music-first Home.** Music on top: continue listening, recently played and new albums, artists; films and shows below. |
| `home-tv` | page | **TV-first Home.** Live TV on top: favourite channels, what's on now, the guide and recordings; the libraries below. |
| `page-photo-frame` | page | **Photo frame.** A page of its own that cycles slowly through the newest photos, for a screen left on. Add it to the navigation, or open /page/photo-frame. |
| `continue-watching` | section | **Continue watching.** Everything the profile is part-way through. |
| `next-up` | section | **Next up.** The next episode of each show in progress. |
| `spotlight` | section | **Spotlight.** A rotating spotlight of recent films. |
| `recent-per-library` | section | **Recently added, a row per library.** One row for each library the profile can open. |
| `recommended` | section | **Recommended for you.** Picked from what the profile has watched here. |
| `top-ten` | section | **Top 10.** The ten best-scored titles, numbered. |
| `genre-row` | section | **A genre.** One genre as a row. Change the genre to taste. |
| `collection-row` | section | **A collection.** What one collection holds. Set the collection's ID. |
| `pinned-views` | section | **Pinned saved views.** A row for each saved view the profile pinned to Home. |
| `watch-later` | section | **Watch later.** The profile's Watch later list. |
| `random-pick` | section | **Can't decide?.** A banner offering one random title to play. |
| `on-tv-now` | section | **On TV now.** Channels with what is on now and next. |
| `guide-entry` | section | **A way into the TV guide.** A banner that opens the app's TV guide. |
| `library-tiles` | section | **Library tiles.** The libraries as a row of tiles. |
| `genre-chips` | section | **Genre chips.** A library's genres as chips, each opening the library filtered to it. |
| `greeting` | section | **A greeting.** "Good evening, Sam" as the page's heading. |
| `search-box` | section | **Search box.** A search field that opens Search. |
| `filter-grid` | section | **Filters over a grid.** The body of a film or show library: a filter bar and the grid it filters. Add both sections. |
| `title-header` | section | **Title header.** The top of a title's page. |
| `seasons` | section | **Seasons and episodes.** A show's seasons and episodes. |
| `track-list` | section | **Track list.** An album's tracks. |
| `more-like-this` | section | **More like this.** Titles like the one the page is about. |
| `kids-notice` | section | **A note for a kind of profile.** A notice only Kids profiles see. Change the words and the condition. |
| `evening-row` | section | **A row for the evening.** A row shown only between 18:00 and 23:00. Change the times and the source. |

<!-- catalogue:end -->

---

<sub>WittyMediaServer documentation</sub>
