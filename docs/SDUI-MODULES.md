# Modules in server-driven UI

A module's part of every app is data that ships with the module. Its pages, its rows on
Home, its menu entries, its data sources, its settings and its colours are written in one
file, `sdui.json`, in the module's own repository. The server checks the file against its
catalogue and merges it into what every app is sent. No app needs any of the module's
code to show it.

This document is the contract for that file: what goes in it, what the server checks,
what it does with it, and what apps and module helpers must do. It builds on
[Server-Driven UI](SDUI.md), which describes pages, sections, components, sources and
conditions; this document links to it rather than repeat it. How modules are built,
released and run is in [Making a module](MODULES.md).

The worked example throughout is a made-up board games module, `wittyboardgames`. Its
whole file is [`pkg/sdui/testdata/module-boardgames.json`](../pkg/sdui/testdata/module-boardgames.json),
and [The worked example, end to end](#the-worked-example-end-to-end) walks through it.

- [What and why](#what-and-why)
- [Module API 4](#module-api-4)
- [The file](#the-file)
- [Names](#names)
- [Theme](#theme)
- [Sources](#sources)
- [Components](#components)
- [Views](#views)
- [Pages](#pages)
- [Inserts](#inserts)
- [Presets](#presets)
- [Navigation](#navigation)
- [Settings](#settings)
- [Limits](#limits)
- [What the server checks, and when](#what-the-server-checks-and-when)
- [What a module may not do](#what-a-module-may-not-do)
- [How it is merged](#how-it-is-merged)
- [HTTP API](#http-api)
- [What apps need to do](#what-apps-need-to-do)
- [Conformance fixtures](#conformance-fixtures)
- [Writing the file](#writing-the-file)
- [The worked example, end to end](#the-worked-example-end-to-end)
- [Migrating a module to API 4](#migrating-a-module-to-api-4)

## What and why

Before this, what a module added to the apps was code in each app, or a web page only the
browser could show. A television app had to be released again for every module, and a
module written by someone else could not reach it at all.

Now a module describes what it adds with the catalogue's own components. The server
expands and renders it like any other page, so:

- **It is portable.** Every app that draws the catalogue draws every module, today and
  later, with no release of the app.
- **It lives with the module.** The file is in the module's repository and in each of its
  releases. A new release of the module can change its pages without a new server.
- **It is checked.** The server refuses a file that doesn't fit its catalogue before the
  module is used, so a broken module never breaks a page.
- **It is safe.** It is data, never code. A module's items pass through the profile's
  limits like the server's own.

Settings are the one exception to "it is SDUI". A module *describes* its settings as rows,
and each app draws them in its own Settings, in its own style. Settings is never an SDUI
page ([SDUI.md](SDUI.md#server-driven-ui-sdui)).

## Module API 4

Module API 4 makes `sdui.json` part of a module. The numbers are in
`internal/modules/sdui.go`:

| Constant | Value | What it means |
|---|---|---|
| `API` | 4 | The newest module API this server takes. |
| `MinAPI` | 3 | The oldest. A version built for API 3 still runs. |
| `SDUIAPI` | 4 | From this API on, a module must carry `sdui.json`. |

Which versions must carry the file:

| `api` in module.json | `delivery` | `sdui.json` |
|---|---|---|
| 4 or more | `helper`, `program`, `web` | **Required.** A version without one is refused. |
| 4 or more | `library`, `builtin` | Not read. A library reaches the server inside other modules. |
| 3 | any | Optional. API 3 modules keep working until they are upgraded. One that carries `sdui.json` anyway has it checked and used. |

A module with nothing to add to the apps still carries the smallest file:

```json
{ "format": "wittymodule-sdui", "formatVersion": 1, "module": "wittyx", "catalog": 2,
  "theme": { "scope": "wittyx" } }
```

## The file

`sdui.json` sits beside `module.json`: at the root of the module's repository, and at the
root of every archive of a release (`wittymodule pack` puts it in each one). The server
reads it from the version's folder.

It is read strictly (`sdui.ParseModuleUI`):

- at most 1 MiB (1,048,576 bytes);
- one JSON object, nothing after it;
- **an unknown field anywhere is refused**, so a misspelt field is an error, not ignored.

The top level:

| Field | Type | Required | What it is |
|---|---|---|---|
| `$schema` | string | no | The JSON Schema an editor checks the file with, such as `./sdui/kit/sdui-module.schema.json`. The server ignores it. |
| `format` | string | yes | Always `wittymodule-sdui`. |
| `formatVersion` | int | yes | Always `1`. |
| `module` | string | yes | The module's ID, the same as `id` in `module.json`. |
| `catalog` | int | yes | The [catalogue revision](SDUI.md#versioning) the file was written against, from 1 to the server's. A server refuses a file newer than its own catalogue. Today the catalogue is at revision 2. |
| `theme` | object | yes | The module's [theme scope](#theme). |
| `sources` | list | no | [Data sources](#sources) the module answers through its helper. |
| `components` | list | no | Named sections of the module's own: [components](#components). |
| `views` | list | no | [Views](#views) an app may draw itself in a `NativeView`. |
| `pages` | list | no | Whole [pages](#pages). |
| `inserts` | list | no | Sections put into built-in pages: [inserts](#inserts). |
| `presets` | list | no | Sections for the page editor's gallery: [presets](#presets). |
| `navigation` | list | no | [Menu entries](#navigation). |
| `settings` | list | no | [Settings pages](#settings), drawn by each app's own Settings. |

The JSON Schema is [`sdui-module.schema.json`](sdui-module.schema.json), written from the
Go types. The types themselves are `ModuleUI` and its parts in
[`pkg/sdui/module.go`](../pkg/sdui/module.go). Where this document and that code differ,
the code is right.

## Names

Everything a module adds is named after the module, so two modules never clash and a name
always says where it comes from. `<module>` is the module's ID: a lower-case letter, then
1 to 63 lower-case letters and digits (`wittyboardgames`).

| What | Form | Rule | Example |
|---|---|---|---|
| Page ID | `<module>` or `<module>-…` | Also a valid [page ID](SDUI.md#limits) (lower-case letters, digits, `-`, `_`, at most 64), and never a built-in page's ID. | `wittyboardgames` |
| Inserted section ID | `<module>` or `<module>-…` | Unique among the module's inserts into one page. | `wittyboardgames-tonight` |
| Preset ID | `<module>` or `<module>-…` | | `wittyboardgames-recent` |
| Menu entry ID | `<module>` or `<module>-…` | | `wittyboardgames` |
| Settings page ID | `<module>` or `<module>-…` | | `wittyboardgames` |
| Source type | `<module>.<camelName>` | Name: a lower-case letter, then up to 47 letters and digits. | `wittyboardgames.shelf` |
| View ID | `<module>.<camelName>` | As a source. | `wittyboardgames.scoreboard` |
| Component type | `<module>.<PascalName>` | Name: an upper-case letter, then up to 47 letters and digits. | `wittyboardgames.GameNight` |
| Property or parameter name | `camelName` | A lower-case letter, then up to 47 letters and digits; never `type` or `map`. | `players` |
| Setting key | `camelName` | A lower-case letter, then up to 63 letters, digits, `_`, `.` and `-`. Unique across all the module's settings pages. | `household` |

Sections inside a module's own pages, components and presets need no prefix: they live on
the module's page, or are given their ID by whoever uses them.

Two names are the same thing only within one module: the same name twice in one file is
an error ("already used at …").

## Theme

`theme` is the module's theme scope. Its sections, pages and items take the scope's
colours ([Theme scopes](SDUI.md#theme-scopes)).

| Field | Type | Required | What it is |
|---|---|---|---|
| `scope` | string | yes | Always the module's ID. |
| `label` | string | no | The name the theme editor shows for the scope, and the title of the module in Settings → Modules. At most 60 characters. |
| `tokens` | map | no | The colours the module would like: colour token → colour. A token must be one of the theme engine's colour tokens (such as `accent`; see [Themes](THEMES.md)), and a colour `#rrggbb`, `#rrggbbaa`, `rgb()` or `rgba()`. |

The module's tokens are the bottom layer. An administrator's overrides for the scope
(`PUT /api/v1/theme/modules/{scope}`) replace them token by token. The result reaches
apps two ways:

- in every rendered document, as `tokens` on each node in the scope and as
  `document.theme` for a page in it ([Theme scopes](SDUI.md#theme-scopes));
- in `GET /api/v1/theme`, under `modules.<module>`: `tokens` is the whole palette with
  the module's and the administrator's colours over it, and `overrides` is the
  administrator's alone. Each look in `variants` has its own `modules` too.

A module's page and every section it inserts default to its scope, and so does every
item its sources give.

## Sources

A source is a data source the module answers, so **only a module delivered as a helper
may declare one**: a web bundle or a program with sources is refused at install, because
nothing could answer them. Sections use it like any catalogue source:
`"source": { "type": "wittyboardgames.shelf", "sort": "name" }`.

| Field | Type | Required | What it is |
|---|---|---|---|
| `type` | string | yes | `<module>.<camelName>`. |
| `label` | string | yes | What the editor calls it. At most 80 characters. |
| `description` | string | no | At most 500 characters. |
| `icon` | string | no | One of the [icons](SDUI.md#icons). |
| `params` | list | no | Its parameters, as [property descriptions](#property-descriptions). |
| `yields` | list | yes | The kinds of item it gives, at least one: `movie`, `episode`, `show`, `season`, `video`, `disc`, `album`, `artist`, `track`, `book`, `audiobook`, `podcast`, `game`, `photo`, `channel`, `recording`, `collection`, `library`, `genre`, `view`. |
| `cardStyle` | string | yes | The [card style](SDUI.md#card-styles) a section's `"cardStyle": "auto"` means. |
| `pageKinds` | list | no | The [kinds of page](SDUI.md#page-kinds) it may be used on. Empty: any. |
| `paged` | bool | no | It gives more through a section's cursor URL ([Paging](SDUI.md#paging)). |
| `seeAll` | string | no | The route "See all" opens, with `{param}` placeholders: `/page/wittyboardgames`. |
| `title` | string | no | A good default title for a section using it. |
| `since` | int | no | The catalogue revision it appeared in. 0: the server's revision. |
| `example` | object | no | An example section, for the editor. |

`category` and `module` may be written but are set by the server: in the catalogue a
module's source has `"category": "module"` and `"module": "<module>"`.

### Property descriptions

Source parameters and component properties are described the same way as the catalogue's
own (`PropSpec`):

| Field | What it is |
|---|---|
| `name` | `camelName`, unique in its list. |
| `type` | `string`, `text`, `markdown`, `int`, `bool`, `enum`, `enumList`, `icon`, `route`, `image`, `url`, `section`, `library`, `ref` or `scope` ([Components](SDUI.md#components) says what each holds). |
| `label` | Required. What the editor's form shows. |
| `help` | A line under it. |
| `default` | What is assumed when it is left out. Checked against the type. |
| `required` | The editor asks for it. |
| `options` | The choices of an `enum` or `enumList`, `{value, label}`. Required for those. |
| `min`, `max` | Bounds of an `int`. |
| `maxLength` | Bound of a string. |
| `formFactors` | Where it has an effect; empty: everywhere. |
| `since` | The catalogue revision it appeared in. |

### How a source is answered

Only a module with a running helper can answer a source: `delivery: helper` (a service is
a helper too). The server starts the helper on first use, as for any call
([Writing a helper](MODULES.md#writing-a-helper)). A `web` or `program` module has no
helper to ask, so it declares no sources; its sections use the catalogue's.

For each page of a section's items, the server calls the helper's **`sdui.items`** method
over [the helper protocol](MODULES.md#the-helper-protocol-module-api-3):

```
server → {"id":7,"method":"sdui.items","params":{
  "source": {"type":"wittyboardgames.shelf","players":4,"sort":"shuffle","limit":12},
  "offset": 0, "limit": 12,
  "viewer": {"profileKind":"adult","cannot":["recordings"],"formFactor":"phone","platform":"ios","locale":"en-GB"},
  "settings": {"household":4,"ages":"all"}}}
helper → {"id":7,"result":{"items":[ … ],"more":false,"total":12}}
```

The request (`ItemsRequest`):

| Field | What it is |
|---|---|
| `source` | The section's source, every `$variable` filled in and every component placeholder expanded. |
| `filter` | The page's filter and sort parameters, for a filterable source. Left out when there are none. |
| `offset`, `limit` | Which items: skip `offset`, give at most `limit`. `limit` is never more than 500 and never past the source's own `limit`. |
| `viewer` | Who is asking (`Viewer`): `profileKind` (`adult`, `kids`, `guest`), `admin`, `cannot` (the profile's parental feature switches that are off), `formFactor`, `platform`, `locale`. Leave out what isn't for them. |
| `settings` | The module's [settings](#settings) for this profile: the server's values, then the profile's own over them, each with its default. Secrets are included. Left out when the module has no settings. |

The answer (`ItemsAnswer`):

| Field | What it is |
|---|---|
| `items` | The items, as [`Item`](#items)s. |
| `more` | There are more after these. |
| `total` | How many there are in all, when known. A total smaller than `offset` plus the items is taken as unknown. |

A `*wittymodule.CallError` from the helper (it answered no) puts the section in the
[`error` state](SDUI.md#states), with Retry. So does a helper that isn't running, such as
one resting after failures. Any other failure counts against the version, as every call
does ([What a server does with a release](MODULES.md#what-a-server-does-with-a-release)).
A section whose source gives nothing is left out, unless it says `"empty": "message"`.

The helper has the same time limit as every call. A page waits for its eager sections, so
answer quickly, from what the helper keeps, and page with `offset`.

### Items

An item is `sdui.Item` ([`pkg/sdui/model.go`](../pkg/sdui/model.go)), the same thing the
server's own sources give. The server never trusts it. Before it reaches a card it keeps
only this (`cleanModuleItem` in `internal/server/sdui_modules.go`):

| Field | Kept when, and how |
|---|---|
| `id` | Required, at most 200 bytes, unique and stable within the source. An item without one is dropped. |
| `kind` | Required, one of the kinds the source's own `yields` lists. Otherwise dropped. |
| `title` | Required, not blank, cut to 300 characters. Otherwise dropped. |
| `contentRating`, `genres` | Checked against the profile's limits (ratings, genres, kinds of title). An item the profile may not see is dropped. |
| `subtitle`, `tagline` | Cut to 300 characters. |
| `overview` | Cut to 2,000 characters. |
| `route` | Kept when it is a place in the app (`/page/wittyboardgames`, `/item/42`); otherwise removed. A card with a route opens it. |
| `images` | Only `poster`, `backdrop`, `square` and `logo`, and only paths on this server beginning `/api/`. Others are removed. The renderer adds the width to ask for. |
| `playId`, `nextId` | Kept only when it is the ID of a library item this profile may see and play, and its screen time isn't over. Otherwise removed. A module can't make anything playable that isn't in a library. |
| `can` | Only `play`, `download`, `share`, `watchLater`, `watched`, `playOn`, `watchParty`. Every one of them acts on a title of the library, so an item without a `playId` that survives the check above offers none. |
| `badges` | At most 3, each cut to 12 characters. |
| `icon` | Kept when it is one of the [icons](SDUI.md#icons). |
| `scope` | Defaults to the module's ID. |
| the rest | `year`, `rating`, `runtimeSec`, `studio`, `progress`, `watched`, `unwatched`, `resumeSec`, `season`, `episode`, `showTitle`, `showId`, `artist`, `track`, `count`, `number`, `now`, `next`: passed on as given. |

Items past `limit` are cut off, and `more` is set.

### Pictures

A module's own pictures are served by the server from the helper:

```
GET /api/v1/sdui/modules/{module}/images/{name}
```

`name` is a letter or digit, then up to 127 letters, digits, `.`, `_` and `-`, with no
`..`. The server calls the helper's **`sdui.image`** method with `{"name": "<name>"}`, and
the helper answers with the picture as the call's blob (`result` may be `null`). The
picture must be PNG, JPEG, WebP or GIF, by its content, and at most 4 MiB. The server
sends it with its real type, `X-Content-Type-Options: nosniff`, a sandboxing
`Content-Security-Policy` and `Cache-Control: private, max-age=3600`. A helper error is a
404, and anything that isn't a picture a 502.

So an item's picture is `"images": {"square": "/api/v1/sdui/modules/wittyboardgames/images/bg-1.png"}`.
An item may also point at the server's own pictures (`/api/items/42/poster`).

## Components

A component is a named section with properties of its own. It is a **template**: one
catalogue section (with children, if it has them) in which property values are written
`{{name}}`. Administrators find it in the page editor's palette, and the server expands
every use of it before rendering. **No app ever sees a module's component**: it is sent
the catalogue components it stands for.

| Field | Type | Required | What it is |
|---|---|---|---|
| `type` | string | yes | `<module>.<PascalName>`. |
| `label` | string | yes | What the palette calls it. At most 80 characters. |
| `description` | string | no | At most 500 characters. |
| `icon` | string | no | One of the [icons](SDUI.md#icons). The palette shows `plug` when left out. |
| `props` | list | no | Its properties, as [property descriptions](#property-descriptions). |
| `pageKinds` | list | no | The kinds of page it may be used on. Empty: any. |
| `section` | object | yes | The template: a [section](SDUI.md#the-model-in-one-page) of a catalogue component. |

In the catalogue (`GET /api/v1/sdui/catalog`) a module's component appears with
`"category": "module"`, `"module": "<module>"`, `"expands": "<catalogue component>"`, the
focus and form factors of that component, and an example.

### Placeholders

A placeholder is `{{name}}`, where `name` is one of the component's properties. It may be
used in the template's `props` and `source`, and in those of its children, at any depth.

- **Standing alone**, `"players": "{{players}}"`, it becomes the property's value with
  its type: the number 4, not the text "4". If the property has no value and no default,
  the key is left out (and an element of a list is dropped).
- **Inside text**, `"title": "{{title}} for {{players}}"`, it is replaced by the value as
  text. No value: it is replaced by nothing.
- A property's value is the use's, else the property's `default`.
- Every placeholder must name a property ("{{x}} names no property of the component").
- Placeholders are not read in `id`, `component`, `when`, `repeat`, `scope` or `locked`.

### What a use becomes

A use is a section whose `component` is the module's component:
`{ "id": "night", "component": "wittyboardgames.GameNight", "props": { "players": 4 } }`.
The server (`ModuleSet.ExpandPage`) makes it:

| Of the result | From |
|---|---|
| `component`, `props`, `source`, `children` | The template, filled in. |
| `id` | The use's. |
| Children's IDs | `<use's id>-<child's id>`, cut to 64 characters, so two uses on one page never clash. |
| `when` | The use's and the template's together (`all`); either alone if only one has one. |
| `repeat` | The use's, else the template's. |
| `scope` | The use's, else the template's, else the module's ID. |
| `locked` | Locked if either is. |

Rules for a template:

- **It is made of catalogue components only**, never of another of the module's
  components, so expansion is one step.
- It must make a valid section when every property has its default or an example value
  of its type, on the first of its `pageKinds` (or a `custom` page). The server checks
  this.
- Its sources may be the catalogue's or the module's own.

A use on a page (the module's own or an administrator's) is checked against the
component's properties like any section. An administrator's page that uses a component of
a module that isn't in use gets a warning, not an error, and the section is left out until
the module is back.

## Views

A view is something of the module's own that an app may draw itself, in a
[`NativeView`](SDUI.md#nativeview): the board games module's live scoreboard, drawn by its
web bundle.

| Field | Type | Required | What it is |
|---|---|---|---|
| `id` | string | yes | `<module>.<camelName>`. What a `NativeView`'s `view` names. |
| `label` | string | yes | At most 80 characters. |
| `description` | string | no | At most 500 characters. |
| `platforms` | list | no | Where it can be drawn, as a hint for the editor: `web`, `tvos`, `ios`, `android`, `androidtv`, `desktop`. Empty: unknown. |

A module's views are added to `NativeView`'s `view` options in the catalogue and in
`nativeViews`. An app that can draw a view says so in `views=` (`X-WMS-SDUI-Views`, at
most 16 names; see [What a client sends](SDUI.md#what-a-client-sends)). That app is sent
the `NativeView`; every other app is sent the `NativeView`'s children, its fallback. So a
`NativeView` of a module's view must always have children that make sense on a
television.

## Pages

A module's page is an ordinary [SDUI page](SDUI.md#the-model-in-one-page), with sections,
variants, conditions and all, written in the module's file. It may use the catalogue's
components and sources, and the module's own.

| Rule | |
|---|---|
| ID | `<module>` or `<module>-…`, and never a built-in page's ID. |
| Kind | `custom`, `library`, `detail`, `livetv`, `music`, `photos`, `books` or `games`. Never `home`, `search`, `libraries` or `collections`: a module adds to Home with an [insert](#inserts). |
| `match` | A `library` or `detail` page must have `match.libraryKind` or `match.mediaKind`: it is for a kind of library or title, never all of them. `match.library` is refused, since a module doesn't know this server's libraries. |
| `scope` | Defaults to the module's ID. |

Where it shows:

- A `custom` page opens at `/page/{id}`: `/page/wittyboardgames`.
- A page of another kind takes part in choosing the page of that kind
  ([Precedence](SDUI.md#precedence)). A module's `library` page with
  `"match": {"libraryKind": "games"}` is used for every games library, unless an
  administrator has a page that matches as well or better.

Precedence, from the top:

1. **The administrator's pages.** One with the same ID as the module's page replaces it
   whole. Deleting it brings the module's back.
2. **The modules' pages**, in order of module ID, each file's pages in order. Among pages
   of one kind that match equally, the first wins.
3. **The built-in pages.**

So a module's page beats a built-in page that matches as well, and never an
administrator's.

## Inserts

An insert puts one of the module's sections into a built-in page: a row on Home, a shelf
on the games page.

| Field | Type | Required | What it is |
|---|---|---|---|
| `page` | string | yes | A [built-in page](SDUI.md#built-in-pages)'s ID: `home`, `games`, `library-games`, … |
| `after` | string | no | A top-level section of that page to go after. |
| `before` | string | no | One to go before. Not both. Neither: at the end. |
| `section` | object | yes | The section. Its ID is `<module>` or `<module>-…`. It may use the module's components and sources. |

- Inserts go **only into built-in pages**. A page the administrator wrote, including one
  that replaces a built-in page, is never changed by a module.
- The section's `scope` defaults to the module's ID.
- A section whose ID the page already has is not inserted again.
- Modules' inserts are applied in order of module ID, each file's in order.
- The section is checked as a section of that page's kind.
- After insertion it is an ordinary section: a profile may move or hide it like any other.

## Presets

A preset is a section offered in the page editor's gallery, under the module's name.

| Field | Type | Required | What it is |
|---|---|---|---|
| `id` | string | yes | `<module>` or `<module>-…`. |
| `label` | string | yes | At most 80 characters. |
| `description` | string | no | At most 500 characters. |
| `section` | object | yes | The section the editor adds. Checked as a section of a `custom` page. |

`GET /api/v1/sdui/presets` lists them with `"kind": "section"` and `"module": "<module>"`.

## Navigation

Menu entries are [navigation entries](SDUI.md#navigation) (`NavItem`).

| Field | What it is |
|---|---|
| `id` | `<module>` or `<module>-…`. |
| `type` | `page` (the module's own page, or a built-in one), `native` (a [native screen](SDUI.md#native-screens) other than `settings`) or `route` (a place in the app). Never `libraries` or `library`. |
| `page`, `screen`, `route` | What it opens, by type. |
| `label`, `icon` | A label is at most 40 characters, and a `route` entry must have one. A `page` entry left without them takes the page's title and icon. |
| `when` | A [condition](SDUI.md#conditions). |
| `placement` | Form factor → `primary`, `more`, `end` or `hidden`. A form factor left out is `primary`. |

A module's entries go into the menu **just before Settings**, in order of module ID.
An entry whose ID the menu already has is left as it is: an administrator who placed or
hid the module's entry in their own navigation keeps that. Modules' entries are never
added past the menu's limit of 40.

## Settings

A module describes its settings. **It never draws them.** Every app's own Settings shows
them under **Settings → Modules → (the module)**, built with that app's own controls in
that app's own style. Settings is never an SDUI page and is never sent as a document.

### The description

`settings` is a list of settings pages:

| Field | Type | Required | What it is |
|---|---|---|---|
| `id` | string | yes | `<module>` or `<module>-…`. |
| `title` | string | yes | At most 80 characters. |
| `icon` | string | no | One of the [icons](SDUI.md#icons). |
| `admin` | bool | no | Shown only to administrators. |
| `groups` | list | yes | Titled groups of rows. A page has 1 to 64 rows in all. |

A group:

| Field | What it is |
|---|---|
| `title` | A heading, at most 80 characters. |
| `footer` | A note under the group, at most 500 characters. |
| `rows` | The rows. |

A row (`SettingRow`):

| Field | What it is |
|---|---|
| `key` | The value's name. Unique across the module's settings. Not on an `info` row. |
| `kind` | `bool`, `enum`, `int`, `string`, `secret` or `info`. |
| `label` | Required, at most 80 characters. |
| `help` | A line of help, at most 500 characters. |
| `scope` | `server` (the default) or `profile`. |
| `default` | The value until one is saved. Checked like a value. Never on a `secret` or `info` row. |
| `options` | `{value, label}` choices. Required on an `enum`, and only there. |
| `min`, `max` | Bounds of an `int`. |
| `unit` | Shown after a number: `MB`, `seconds`. At most 20 characters. |
| `maxLength` | For `string` and `secret`, 0 to 4,000. 0 means 1,000. |

The kinds, and what a value must be (`sdui.CheckSetting`):

| Kind | Drawn as | A value is |
|---|---|---|
| `bool` | A switch. | `true` or `false`. |
| `enum` | A choice of `options`: a picker, a list with a tick, or segments when there are few. | One of the options' `value`s. |
| `int` | A number field or a stepper, with `unit`, between `min` and `max`. | A whole number between `min` and `max`. |
| `string` | A one-line text field. | One line of text: at most `maxLength` characters, with no NUL, escape, carriage return or line feed. |
| `secret` | A secure text field, never filled in. Say "Set" when it is in `secretsSet`, with a way to change or clear it. | As `string`. |
| `info` | A line of text to read. | It has no value. |

### Scopes

| Scope | Who sets it | Where it is kept | Rules |
|---|---|---|---|
| `server` | Administrators. Others see the value, read-only, on pages they can see. | In the server's settings, under `module.<module>.settings`, as one JSON object. | |
| `profile` | Each profile, for itself. | In the profile's UI document, under `modules.<module>`. | Not on an `admin` page. Never a `secret`. |

A saved value that no longer fits its row (the module changed it) is ignored, and the
default applies. A value of a row the module no longer has is ignored.

### Routes

| Route | Who | What it does |
|---|---|---|
| `GET /api/v1/sdui/modules/settings` | Anyone signed in | The modules in use that have a settings page this profile may see: `{"modules": [{"module", "label", "icon", "url"}]}`. What Settings → Modules lists. |
| `GET /api/v1/sdui/modules/{module}/settings` | Anyone signed in | The module's settings for this profile. |
| `PUT /api/v1/sdui/modules/{module}/settings` | Administrators | Changes `server` values. |
| `PUT /api/v1/sdui/modules/{module}/settings/personal` | Anyone signed in | Changes this profile's `profile` values. |

`GET` answers:

```json
{ "module": "wittyboardgames", "label": "Board games",
  "pages": [ { "id": "wittyboardgames", "title": "Board games", "icon": "game", "groups": [ … ] } ],
  "values": { "household": 4, "ages": "all" },
  "secretsSet": [ "apiKey" ],
  "editable": true }
```

| Field | What it is |
|---|---|
| `module` | The module's ID. |
| `label` | `theme.label`, else the module's name from its manifest. Show it as the module's title. |
| `pages` | The settings pages this profile may see: `admin` pages only for administrators. |
| `values` | Every value with its default: the `server` values, then this profile's `profile` values. **Secrets are never sent.** A profile that isn't an administrator gets only the values of rows on pages it can see. |
| `secretsSet` | For administrators, the keys of the secrets that have a value. Empty for everyone else. |
| `editable` | The profile may change `server` values: it is an administrator. |

Both `PUT`s take `{"values": {"<key>": <value>, …}}`, at most 64 KiB, and answer the same
view as `GET`. Only the keys named change; `null` removes a saved value, so the default
applies again. Every value is checked first, and nothing is saved if one is wrong. A 400
says why, in words to show beside the row:

- `<key>: the module has no such setting`;
- `<key>: each profile sets this for itself` (a `profile` key sent to the server's route);
- `<key>: only an administrator sets this, for the whole server` (a `server` key sent to
  the personal route);
- what `CheckSetting` says: `household is between 1 and 12`, `ages is one of all, kids`,
  `listUrl is at most 300 characters`, `listUrl is one line of plain text`, … .

A 403 is a profile that isn't an administrator writing `server` values. A 404 is a module
that isn't in use, or one the profile is kept from by its parental switches. A 413 is a
profile whose UI document would pass 256 KiB.

### What the helper is told

| When | How | What |
|---|---|---|
| The helper starts | The environment variable `WITTYMODULE_SETTINGS` | The `server` values with defaults, secrets included, as a JSON object. |
| An administrator changes them | The method **`settings.changed`**, `{"values": {…}}` | The same, after the change. Only sent to a helper that is running. A helper that doesn't know the method may answer with a `CallError`; it reads the values when it next starts. |
| Every `sdui.items` | `settings` in the request | The `server` values, then this profile's `profile` values, with defaults. |

### Drawing them in an app

Each app adds **Modules** to its own Settings, if it hasn't one already, and under it one
entry per module that has settings the profile can see. Inside, each settings page is a
screen (or a section, where an app shows one module's pages together), each group a
grouped list with its `title` as the heading and `footer` under it, and each row a row of
the app's own:

- **The app's style, not the module's.** The same controls, fonts, spacing and colours as
  the rest of that app's Settings. No module colours here.
- **Read-only when not `editable`.** A `server` row shown to someone who isn't an
  administrator is drawn as a value, not a control.
- **Save as it changes**, as the app's other settings do: a switch or a choice at once, a
  text field when it is done. A `server` row goes to the server's route, a `profile` row
  to the personal one. Draw the answer, which is the new state. On a 400, keep the old
  value and show the message by the row.
- **Secrets** are never shown or filled in. Show whether one is set (`secretsSet`), with
  a field to type a new one and a way to clear it (`null`).
- `help` is the row's secondary text or note; `unit` follows a number.

Accessibility is part of the contract:

- **Every row is reachable without a pointer**: by keyboard and Tab on the web and
  desktop, by the Siri Remote's focus engine on Apple TV, by the D-pad on Android TV, and
  by a game controller wherever the app takes one. `info` rows and read-only rows are
  reachable too, so they can be read.
- **Every row is labelled for screen readers** (VoiceOver, TalkBack, a browser's reader):
  its `label` as the name, its value (with `unit`) as the value, its `help` as the hint
  or description, and its kind as the role (switch, picker, text field, secure field,
  static text). A read-only row says it is read-only. A group's title is a heading.
- Focus moves in reading order: groups top to bottom, rows in order.
- An error is announced when it is shown.

## Limits

| What | At most |
|---|---|
| The file | 1 MiB |
| Pages | 32 |
| Sources | 64 |
| Components | 64 |
| Views | 32 |
| Inserts | 32 |
| Presets | 64 |
| Menu entries | 16 (and the menu, 40 in all) |
| Settings pages | 16 |
| Rows on one settings page | 64 |
| A label | 80 characters (the theme's, 60) |
| A description, help or footer | 500 characters |
| A picture from `sdui.image` | 4 MiB |
| A settings `PUT` | 64 KiB |

Each page, section and source is also held to the [SDUI limits](SDUI.md#limits).

## What the server checks, and when

`sdui.ValidateModuleUI` checks everything above: the format, the module's ID and
catalogue revision, the names, the limits, every source, component, view, page, insert,
preset, menu entry and settings row against the catalogue and the module's own file, and
the colours against the theme engine. A file is refused when it has an error. Warnings
don't refuse it.

| When | What happens to a file that fails |
|---|---|
| **Install** (step 4b of [What a server does with a release](MODULES.md#what-a-server-does-with-a-release)) | The install fails, and the module stays on the version it had. |
| **Every time a version is picked for use**: at start, on a switch, when a module is turned on | That version isn't used. The one before takes over, or the module goes without, and the reason is shown on the Modules page. |

The refusal says what is wrong, in one of these forms:

- `it is built for module API 4, where a module carries its own SDUI, and it has no sdui.json`
- `sdui.json is 1200000 bytes; the most is 1048576`
- `sdui.json can't be read: json: unknown field "script"`
- `sdui.json has something after its end`
- `its sdui.json has data sources, and only a helper can answer them; it is delivered as "web"`
- `its sdui.json doesn't fit this server's catalogue: /pages/0/sections/1/component: …; …`
  (the first three errors, each with its JSON path)

On every render, a section of a module that isn't in use (turned off, not installed, or
one the profile is kept from) is left out, and the rest of the page is drawn. The page
validator gives an administrator a warning for it.

## What a module may not do

| It may not | Because |
|---|---|
| Use another module's components, sources or views | It must work wherever it is installed, alone. ("… is another module's component: a module uses only its own …") |
| Add a `home`, `search`, `libraries` or `collections` page | Those stay the server's. Add to Home with an insert. |
| Use a built-in page's ID, or replace an administrator's page | Only an administrator replaces pages. |
| Insert into an administrator's page | Inserts go only into built-in pages. |
| Add a Settings entry, or any SDUI for Settings | Settings is the app's own. Describe settings under `settings`. |
| Name this server's libraries (`match.library`, `library` entries, `libraries` entries) | It can't know them. Use `match.libraryKind` and `match.mediaKind`. |
| Make a library page or detail page for everything | It must narrow by kind. |
| Make anything playable that isn't a library item the profile may play | `playId` and `nextId` are checked. |
| Point pictures anywhere but this server | Only `/api/` paths are kept. |
| Carry code, HTML, scripts or addresses of its own | The file is data; unknown fields are refused, and the [SDUI security rules](SDUI.md#security) apply. |
| Be seen by a profile kept from it | A profile whose switches hide a module (games off hides `wittyplay`, `wittycouch`, `wittyrigs`; Live TV off hides `wittytv`, `wittytuner`) gets none of its pages, inserts, entries, sources or settings ([Profiles with limits](SDUI.md#profiles-with-limits)). |

## How it is merged

For each request the server takes the modules in use, less those the profile is kept
from, and (in `internal/server/sdui_modules.go`):

| What | Where it goes |
|---|---|
| Pages | Between the administrator's and the built-in ones ([Pages](#pages)). |
| Inserts | Into the built-in pages the administrator hasn't replaced. |
| Components | Expanded into catalogue sections on every page, the administrator's included. |
| Sources | Answered by the module's helper, through the profile's limits. |
| Views | Offered by `NativeView`; sent to an app that has them. |
| Menu entries | Before Settings, unless the menu already has them. |
| Theme | Under the administrator's overrides for the scope; in documents and in `/api/v1/theme`. |
| Catalogue | `GET /api/v1/sdui/catalog` lists every module's components, sources and views, with `module` set; its ETag changes when modules do. |
| Editor | `GET /api/v1/sdui/config/pages` lists the modules' pages with `"source": "module"` and `"module"`; presets are in the gallery. |

## HTTP API

| Route | Who | What it does |
|---|---|---|
| `GET /api/v1/sdui/modules` | Administrators | `{"modules": [ … ]}`: every module in use, with its whole `sdui.json`. What the editor shows where pages, rows and entries come from. |
| `GET /api/v1/sdui/modules/{module}/images/{name}` | Anyone signed in | A picture the module's cards show ([Pictures](#pictures)). |
| `GET /api/v1/sdui/modules/settings` | Anyone signed in | The modules with settings this profile sees ([Routes](#routes)). |
| `GET /api/v1/sdui/modules/{module}/settings` | Anyone signed in | The module's settings for this profile ([Routes](#routes)). |
| `PUT /api/v1/sdui/modules/{module}/settings` | Administrators | The server's values. |
| `PUT /api/v1/sdui/modules/{module}/settings/personal` | Anyone signed in | This profile's own values. |

Every route also answers without `/v1`. A module that isn't in use, or one the profile is
kept from, is a 404 on the routes that name a module.

`GET /api/v1/capabilities` lists two features: `sdui-modules` (modules' SDUI is merged and
the picture route exists) and `module-settings` (the settings routes exist). An app checks
them before using the routes.

## What apps need to do

A renderer needs almost nothing new. **It only ever sees catalogue components**: a
module's components arrive expanded, its sources arrive as cards, and a section of a
module that isn't there is never sent. The conformance fixtures below prove it.

The tasks for each app:

1. **Announce the module views it can draw**, in `views=` with its own. The web app
   draws a module's view with the module's web bundle: it sends the IDs of the views of
   every bundle it has loaded, and when the page holds a `NativeView` of one, mounts the
   bundle's component for it in that place. A bundle exports `views`, components by view
   ID, as it exports `pages`: `export const views = { 'wittyink.bookshelf': Bookshelf }`.
   The web app mounts one with the props `{ host, view, library?, ...the node's other
   props }`, where `host` is the object a module's pages get and `view` the view's ID;
   when the bundle or the export is missing, it draws the node's children, the fallback,
   as for any view it doesn't have. A native
   app sends a module's view only if it really draws it; usually it sends none, and its
   users see the fallback.
2. **Draw module settings in Settings → Modules**, as in [Drawing them in an app](#drawing-them-in-an-app),
   when the server lists `module-settings`. `GET /api/v1/sdui/modules/settings` lists the
   modules with settings the profile may see, for any profile; for each, `GET …/settings`
   gives the pages and values.
3. **Read module palettes from `/api/v1/theme`**: `modules.<module>.tokens` (and each
   variant's `modules`) for anything of a module the app draws outside a document, such
   as a module's own screen or a web bundle's view. Inside a document, nodes already carry
   `tokens`.
4. Nothing else changes: `scope` and `tokens` on nodes, fallbacks, states and paging are
   as in [Rules for renderers](SDUI.md#rules-for-renderers).

## Conformance fixtures

Four fixtures in [`pkg/sdui/conformance/fixtures`](../pkg/sdui/conformance/fixtures/index.json)
cover modules. They are made from the worked example (`moduleCases` in
`pkg/sdui/conformance/cases_test.go`). Each has `input.moduleUI`, the module's file, for
reference; a renderer reads only `expected.document`, as with every fixture
([The conformance kit](SDUI.md#the-conformance-kit)).

| Fixture | What it pins down |
|---|---|
| `module-page-tv` | The module's page on an Apple TV. `GameNight` arrives as a `Shelf` titled "Game night for 4"; the module's source gives square cards with the module's pictures; the page and cards carry `scope: wittyboardgames` and its accent; the `NativeView` of a view the app doesn't have arrives as its fallback, a `Stack` holding the `Notice`. |
| `module-page-web` | The same page in a browser that sent `views=wittyboardgames.scoreboard`: the `NativeView` itself is sent. |
| `module-home-insert` | Home on a phone on a Friday evening: the inserted `wittyboardgames-tonight` row sits after `jump`, in the module's scope, shown because its `weekday` condition holds. |
| `module-not-installed` | An administrator's page using the module's component and source with no module installed: both are left out, and the rest of the page is drawn. |

## Writing the file

### A module written in Go

A helper module imports **`github.com/wittywilla/wittymodule/sdui`**, builds its
definition in Go (`sdui.Module` and its parts are copies of the server's types, so a
module needs nothing of the server's), and keeps `sdui.json` equal to it with a golden
test. The package checks the definition as the server will, as far as that can be done
without a server (`sdui.Check`), and writes it the same way every time (`sdui.JSON`,
`sdui.WriteFile`). It also helps answer `sdui.items`, `sdui.image` and
`settings.changed` in the helper.

```go
// ui.go: the module's SDUI, in Go.
func UI() *sdui.Module { return &sdui.Module{Format: sdui.Format, FormatVersion: sdui.FormatVersion, Module: "wittyx", Catalog: sdui.CatalogRevision, /* … */} }

// ui_test.go: sdui.json is the definition, and the definition passes.
func TestSDUIFile(t *testing.T) {
	if err := sdui.Errors(sdui.Check(UI(), "wittyx")); err != nil {
		t.Fatal(err)
	}
	want, _ := sdui.JSON(UI())
	got, _ := os.ReadFile(sdui.File)
	if !bytes.Equal(got, want) {
		t.Fatal("sdui.json is out of date: write it again from UI()")
	}
}
```

### A web-bundle module

A `web` module writes `sdui.json` by hand, or builds it with
`kit/sdui/wittymodule-sdui.ts`, which `kit/module-kit.sh` copies into its repository, and
writes the result to `sdui.json` in its build.

### Either way

- **`wittymodule check`** reads `sdui.json` beside `module.json` and checks it as the
  server will. A module built for API 4 without one fails.
- **`wittymodule pack`** puts `sdui.json` in every archive of the release.
- The server is the last word: its catalogue and the theme engine's colour tokens decide.
  Test against a server ([Migrating a module to API 4](#migrating-a-module-to-api-4)).

## The worked example, end to end

[`module-boardgames.json`](../pkg/sdui/testdata/module-boardgames.json) is a module that
keeps a household's board games. Part by part:

**The header.** `"format": "wittymodule-sdui"`, `"formatVersion": 1`,
`"module": "wittyboardgames"` (its `module.json` has the same `id`), `"catalog": 2`.

**The theme.** `{"scope": "wittyboardgames", "label": "Board games", "tokens": {"accent": "#2E7D32"}}`.
Everything of the module is green unless an administrator sets
`PUT /api/v1/theme/modules/wittyboardgames` to something else. "Board games" is its name
in the theme editor and in Settings → Modules.

**The source.** `wittyboardgames.shelf` gives `game` items in the `square` style, takes
`players` (0 to 12), `sort` (`name`, `played`, `shuffle`) and `limit`, is `paged`, and
its "See all" opens `/page/wittyboardgames`. The module's helper answers it:

```
server → {"id":3,"method":"sdui.items","params":{"source":{"type":"wittyboardgames.shelf","players":4,"sort":"shuffle","limit":12},
          "offset":0,"limit":12,"viewer":{"profileKind":"adult","formFactor":"tv","platform":"tvos","locale":"en-GB"},
          "settings":{"household":4,"ages":"all"}}}
helper → {"id":3,"result":{"items":[{"id":"bg-1","kind":"game","title":"Harbour Lights","subtitle":"3–5 players",
          "images":{"square":"/api/v1/sdui/modules/wittyboardgames/images/bg-1.png"},"route":"/page/wittyboardgames"}, …],"total":6}}
```

The server keeps those items as they are: their kind, title, picture path and route all
pass. The card's picture is fetched from the helper with `sdui.image` `{"name": "bg-1.png"}`.

**The component.** `wittyboardgames.GameNight` has `title` (default "Game night") and
`players` (default 4). Its template is a `Shelf` titled `"{{title}} for {{players}}"`,
large cards, "See all" always, with the source `{"type": "wittyboardgames.shelf",
"players": "{{players}}", "sort": "shuffle", "limit": 12}`. A use
`{"id": "night", "component": "wittyboardgames.GameNight", "props": {"players": 4}}`
becomes:

```json
{ "id": "night", "component": "Shelf", "scope": "wittyboardgames",
  "props": { "title": "Game night for 4", "cardSize": "l", "seeAll": "always" },
  "source": { "type": "wittyboardgames.shelf", "players": 4, "sort": "shuffle", "limit": 12 } }
```

`players` is the number 4 in the source, because it stands alone; it is text in the title.

**The view.** `wittyboardgames.scoreboard`, drawn by the module's web bundle, for `web`
and `desktop`.

**The page.** `wittyboardgames`, kind `custom`, at `/page/wittyboardgames`: the
`GameNight` row, a `Shelf` of every game by name, and a `NativeView` of the scoreboard
whose child is a `Notice`, "The scoreboard is on the web app." An Apple TV is sent the
notice (`module-page-tv`); a browser that loaded the bundle and sent
`views=wittyboardgames.scoreboard` is sent the `NativeView` (`module-page-web`).

**The insert.** On Home, after `jump`, the section `wittyboardgames-tonight`: a
`GameNight` with its defaults, shown only on Fridays and Saturdays
(`"when": {"weekday": ["fri", "sat"]}`). It is expanded like any use, with the use's
condition. If an administrator has written their own `home` page, it isn't inserted.

**The preset.** `wittyboardgames-recent`, "Board games played lately": a `Shelf` of the
source by `played`, limit 20, in the editor's gallery.

**The menu entry.** `wittyboardgames`, a `page` entry for its page, behind More on phones
and televisions and in the bar elsewhere, placed just before Settings. It takes the page's
title and icon: "Board games", `game`.

**The settings.** One page, "Board games", in two groups. Under "The shelf", for the
server: `listUrl` (text, at most 300), `apiKey` (a secret) and `household` (a number from
1 to 12, default 4). Under "For you", with the footer "Each profile chooses its own.":
`ages`, a profile's own choice of "Everyone" or "Children", default `all`. Every app shows
the page under Settings → Modules → Board games. An administrator sets `listUrl` and
`apiKey`; a profile chooses `ages`. The helper starts with
`WITTYMODULE_SETTINGS={"household":4}` (and `listUrl` and `apiKey` once set), hears
`settings.changed` when an administrator changes them, and gets
`"settings": {"household": 4, "ages": "all"}` with each question about its source.

## Migrating a module to API 4

For each module repository:

1. **Raise `api` to 4 in `module.json`**, with the WittyModule that speaks API 4.
2. **Write the module's SDUI.**
   - A Go module: build it in Go with `github.com/wittywilla/wittymodule/sdui`, and write
     `sdui.json` from it.
   - A web-bundle module: write `sdui.json` by hand, or with
     `kit/sdui/wittymodule-sdui.ts` (run `kit/module-kit.sh` to copy it in).
   - Move what the module drew in the apps into it: its pages, its rows on Home, its menu
     entries, its colours. A module with nothing to add carries the smallest file.
   - Name everything after the module ([Names](#names)).
3. **Add the golden test** (Go): `sdui.json` equals the definition, and the definition
   passes `sdui.Check`.
4. **Answer `sdui.items`** in the helper if the file has `sources`, and **`sdui.image`**
   if its items show the module's own pictures. Give each item an `id`, a `kind` from the
   source's `yields`, a `title`, and pictures under
   `/api/v1/sdui/modules/<module>/images/`. Page with `offset` and `limit`, and leave out
   what the `viewer` shouldn't see.
5. **Describe its settings** under `settings`, choose `server` or `profile` for each, and
   read them from `WITTYMODULE_SETTINGS`, `settings.changed` and each request's
   `settings`. A web module's settings that the bundle drew itself become rows here.
6. **Run `wittymodule check`** on the folder, and fix what it says.
7. **Try it on a server.** Build the server against your checkout with
   `node scripts/dev-modules.mjs <Repo>=<your folder>`, install the module from a test
   catalogue ([Developing a server and a module together](MODULES.md#developing-a-server-and-a-module-together)),
   and check: the install isn't refused; its page, rows and menu entry appear on the web
   app and one television app; its sections fill; its settings show under
   Settings → Modules and save.

---

<sub>WittyMediaServer documentation</sub>
