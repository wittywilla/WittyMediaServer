# TV mode: what a television offers

A television is for browsing, watching, listening and playing, with a remote or a controller from across the room.
Editors with many fields, typing, file pickers or drag handles are hard to use there, and can trap the remote. So
every screen declares a **TV level**, and TV mode (the web app's ten-foot mode, `html.tv-mode`) and every TV app
(Android TV, tvOS, Plasma Big Screen, and the WittyMediaClient desktop app started with `--tv`) follow it.

The WittyMediaClient app on a Windows or Mac computer, and any web browser, keep every screen. Only TV mode and the TV
apps are limited.

## The three levels

| Level | On a television |
|---|---|
| `full` (the default) | Everything works with a remote or a controller. |
| `view` | The values are shown, nothing can be changed, with the note below. Each section is a stop for the remote, so a long page can still be read to its end. |
| `desktopOnly` | A card in its place: the screen's name, the note, the page's address, a QR code that opens it on a phone or computer, and a few lines of what is set now ("4 libraries", "149 commercials and bumpers"). |

The note is the table's own: *To change this, use the WittyMediaClient app on a Windows or Mac computer, or a web
browser.* Under it: *What you change there applies on this TV too.*

Navigation never dead-ends. Focus starts on **Back**, Back (or Escape, or B on a controller) leaves the screen the
way it came, and there are no controls a remote can't reach. Settings on a television opens on its menu, with each
computer-only row saying "On a computer", instead of jumping into its first page.

## Where the levels are declared

One table, `pkg/sdui/data/tvlevels.json`, is the list. The server sends it in the SDUI catalogue
(`GET /sdui/catalog`, `tvLevels`), and the web app builds it in, so the web app and every client make the same
choices.

- **screens** are found by address: `"/settings/libraries"`, `"/tv/channels/:id"` (one part), `"/settings/pages/*"`
  (the rest, or nothing). The first match wins, so a narrower address comes before a wider one. A screen may name a
  `summary` for its card.
- **parts** are found by id, for a part of a screen that is otherwise usable (the theme maker in Appearance).
- A **server-driven page** can say `"tv": "view"` or `"desktopOnly"` itself (`Page.tv`; rendered documents carry it
  as `page.tv`).
- A **module's settings page** (`sdui.SettingsPage.tv`) and each of its **rows** (`SettingRow.tv`) can say the same.
  On a television such a row shows its value, read only, with a line that says where it is changed.
- A module with a frame of its own (WittyPlay) uses `host.tv` (web host feature `tvLevels` 1): `screenFor(path)`,
  `partLevel(id)`, and the parts `OnAComputer`, `OnAComputerDialog`, `PartNote`, `TvPart` and `ViewOnly`.

`GET /sdui/tv-summary/{id}` gives a card its lines of what is set now (only what the person may already see: an
administrator's screens say nothing to anyone else) and the server's addresses on the home network, for a QR code
that a phone can open when the television itself uses an address only it can reach.

In the web app: `web/src/tvmode/levels.ts` (the rules, tested by `levels.test.mjs`), `levelTable.ts` (the table),
`TvLevelGate.tsx` (every page TV mode shows passes through it) and `OnAComputer.tsx` (the card, the dialog, the part
note and the read-only wrapper). In the Android app: `net/TvLevels.kt` and `Nav.serverPage()`.

## Adding a feature

Every new screen or editor decides its TV level when it is made:

1. Can it be used with a remote or a controller alone: a few choices, switches, steppers, a list to pick from? Then
   it is `full` and needs nothing.
2. Does it need typing, a file or folder, a drag handle, a colour picker, or many fields? Add it to
   `pkg/sdui/data/tvlevels.json` as `desktopOnly` (or `view` when seeing the values on a television helps), with a
   `summary` if a line or two of what is set now would help, and implement that summary in
   `internal/server/api_tv_levels.go`.
3. A button that opens an editor stays on a television and opens `OnAComputerDialog` instead (Customise this page,
   Edit layout).
4. Whatever it sets is kept on the server, per person or per server, never only in the browser, so that a change
   made on a computer reaches the television.

Console and gaming settings are a television's own: WittyPlay's controllers and mapping, profiles, per-console and
per-game settings, the controller screen, pointer and Wii options, players, vibration, cores and the game library
stay `full`, and must stay easy with a pad.

## The list

### Everyday: full

Home, libraries, titles, shows, people, collections, search, You and profiles, subtitles and audio, the player and
its basic quality, Live TV's guide and recordings, guide sets, WittyTV, karaoke, music, books, photos, games, help,
About, Legal, Activity, module settings (unless a module says otherwise), and in Appearance: the theme, the look,
motion, WittyTV's look and the player's look.

### Console and gaming: full

`/controllers` (the controller screen), `/settings/games` (My game settings), `/settings/gaming` and WittyPlay's
own settings: game settings, layout and buttons, game pictures, controllers and buttons, controller mapping,
friends, WittyPlay options, cores and emulators, console photos.

### View

| Screen | Address |
|---|---|
| Playback (an administrator's player, skipping and streaming settings) | `/settings/playback` |
| Profiles (people, passwords and limits) | `/settings/users` |
| Sign-in and security | `/settings/security` |
| Sharing and photos | `/settings/sharing` |
| WittyTV's channel list | `/tv/channels` |

### Desktop only

| Screen | Address | Card shows |
|---|---|---|
| Libraries | `/settings/libraries`, `/manage/*` | libraries, by kind |
| Pages and menu (page, menu and layout editors) | `/settings/pages/*` | |
| Metadata agents | `/settings/metadata` | |
| Channels and tuners, bumpers | `/settings/channels/*`, `/settings/tuners/*` | channels, commercials and bumpers |
| Channel editor | `/tv/channels/:id` | channels, commercials and bumpers |
| Live transcoding | `/settings/transcoding` | graphics chip, largest picture |
| Library transcoding | `/settings/library-transcoding` | |
| Network | `/settings/network` | the address it listens on |
| Remote Access | `/settings/remote` | |
| WittyLog | `/settings/logs` | |
| WittyDiagnoser | `/settings/diagnoser` | |
| Modules and the module store | `/settings/modules/*` | modules in use |
| Backup | `/settings/backup` | the automatic backup schedule |
| Repair, Reset | `/settings/repair`, `/settings/reset` | |
| Fix matches | `/fix-matches` | |
| Scenes and skips (markers) | `/markers/:item` | |
| Canvas | `/canvas/:item` | |
| Smart lists | `/smart-list` | |
| Design review | `/design/*` | |
| Game metadata (keys for game sources) | `/settings/game-metadata` | |
| WittyPlay's game libraries | `/wittyplay/settings/libraries` | libraries, by kind |

Parts, desktop only: making a theme, importing a theme, editing the layout (the shell editor and the menu
editor), name and logo, the server's colours, your colours, Customise this page, the `.wittyinfo` scenes and skips
editor, the advanced quality menu's fields, the Canvas studio, WittyPlay's custom cores and importing game
pictures; in the apps, their own layout editor, name and logo editor, and making a canvas.

## Settings apply everywhere

Everything these editors set is kept on the server, so a change made on a computer applies on the television:

| Setting | Kept |
|---|---|
| Libraries, metadata, transcoding, network, remote access, backups, modules | the server's settings |
| Channels, bumpers and commercials | the server (channels and their libraries) |
| Themes: installed and server-made themes | the server's themes folder |
| A person's own theme from the theme maker | their account (`/me/ui`, `customTheme`); each device then picks it from its theme list |
| Layouts and pages, menus | the server's SDUI configuration, per form factor and per person |
| WittyTV's look | the person's account |
| Skips and scenes (`.wittyinfo`), automatic skipping | the server, and each person's account |
| Quality defaults | the person's account (`/me/streaming`) |
| Controller layouts, profiles and mappings | the server (WittyPlay) |

Kept on each device on purpose, because each screen is different: which theme the device wears, TV mode itself,
the player's look (seek bar, hiding the controls, picture fit, disc subtitle size), the guide's set and rows, spacing,
the connection's streaming choice, and the quality picked from a player's menu for that device. All of these can be
changed on the television itself.

---

<sub>WittyMediaServer documentation</sub>
