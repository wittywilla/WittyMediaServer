# The settings menu

One list of settings, drawn the same way by every client: the server's web pages (computer and phone widths), the
web's TV mode, the desktop app (which shows the web pages), the Android phone and TV app, and the iPhone, iPad and
Apple TV app. It is the Android phone app's **You** page: its groups, rows, order and words.

The list is [`pkg/sdui/data/settingsmenu.json`](../pkg/sdui/data/settingsmenu.json). The SDUI catalogue carries it as
`settingsMenu`, and the apps keep a copy in `shared/data/settingsmenu.json` for a server older than it (their CI
checks that copy against this file).

## Groups and rows

Account, Household, Yours, Modules, On this device, Server, Maintenance, Help and about. Each row has:

| Field | What it is |
| --- | --- |
| `id`, `key`, `title` | its name in code, the apps' word for it (`strings.en.json`), and the web's English words |
| `icon` | a symbol for each platform: `android` (Material Symbols), `apple` (SF Symbols), `web` (`icons.tsx`) |
| `tint` | the icon tile's colour; `#5d6474` for the owner's tools, `danger` for Reset |
| `native` | an app's own screen for it (Name and username, Two-step sign-in, Theme, Servers on this device…) |
| `path` | the server's page for it, which a client opens where it has no screen of its own; also the address a TV's QR code leads to |
| `sdui` | the page is a server-drawn page every client draws itself (Libraries) |
| `part` | an app's own editor in the TV table (Edit layout, Name and logo) |
| `when` | conditions that must all hold; a condition a client doesn't know keeps the row out |
| `value` | what the row shows on its right: the name, two-step on or off, how many devices, the theme, the version… |
| `summary` | what a TV shows on a row it can't change (`GET /api/sdui/tv-summary/{id}`); else the page's in the TV table |

A client shows a row when every condition holds and it can open the row. The web shows the rows that have a page; the
desktop app adds its own (Servers on this device, From away, This app's log) through its bridge; the phone and TV apps
add their own screens (Top menu on a TV).

## Layouts

- **Phone:** one column: the person (picture, names, role, Switch profile), the groups, each one card of rows with a
  tinted icon tile, the name, what is set and a chevron, then This server and Sign out.
- **Computer and TV:** the adapted version Apple TV's You page made: the person on the left third, the groups on the
  right two-thirds. A computer's settings pages have the same rows beside them, smaller, with the open one marked, and a
  search box (a computer's extra). Sections that modules add and that the list doesn't name follow under From modules.
- **TV:** each row is a tile of its own. No keyboard keys are drawn.

## What a TV can't change

The TV table ([`tvlevels.json`](../pkg/sdui/data/tvlevels.json), [TV-MODE.md](TV-MODE.md)) decides, by a row's `path`
or `part`. A `desktopOnly` row stays in its place, shows what is set (its summary's first line, else "On a computer"),
and opens the panel that says where to change it: on a TV device the setting's own address in words and as a QR code,
with what is set now; on a computer in TV mode, Leave TV mode. A `view` row opens its page with the values shown and
nothing to change.

## Adding a setting

Add the row to `settingsmenu.json` in its group, with its words in the apps' `strings.en.json`, then copy the file to
the apps (`node shared/scripts/sync-settings-menu.mjs` in WittyMediaClient). If a TV shouldn't change it, give its page
a level in `tvlevels.json`, and a summary in `internal/server/api_tv_levels.go` if a line of what is set helps.
`go test ./pkg/sdui` checks the file; `web/src/pages/settings/menu/model.test.mjs`, the apps' `SettingsMenuTest` and
`SettingsMenuTests`, and `npm run test:settings-menu` (against a running server) check the clients.

---

<sub>WittyMediaServer documentation</sub>
