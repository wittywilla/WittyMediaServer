# TV mode: what a television offers

A television is for browsing, watching, listening and playing, with a remote or a controller from across the room.
Editors with many fields, typing, file pickers or drag handles are hard to use there, and can trap the remote. So
every screen declares a **TV level**, and TV mode follows it everywhere: the TV apps (Android TV, tvOS, Plasma
Bigscreen), a TV's own browser, and TV mode on a computer (the WittyMediaClient desktop app started with `--tv`, or a
web browser with TV mode on).

Out of TV mode, the WittyMediaClient app on a Windows or Mac computer and any web browser keep every screen.

## The three levels

| Level | On a television |
|---|---|
| `full` (the default) | Everything works with a remote or a controller. |
| `view` | The values are shown, nothing can be changed, with the note below. Each section is a stop for the remote, so a long page can still be read to its end. |
| `desktopOnly` | A card in its place, with a few lines of what is set now ("4 libraries", "149 commercials and bumpers"). What else it says depends on the client, below. |

### A TV device, or a computer in TV mode

The card says something different on the two kinds of client, because only one of them can simply leave TV mode:

| | A TV device | A computer in TV mode |
|---|---|---|
| Which | The Android TV and Apple TV apps, Plasma Bigscreen, a TV's own browser | The WittyMediaClient desktop app started with `--tv`, any web browser with TV mode on |
| The note | *To change this, use the WittyMediaClient app on a Windows or Mac computer, or a web browser.* (`note`) and *What you change there applies on this TV too.* | *Leave TV mode to change this.* (`noteDesktop`) |
| Also | The page's address and a QR code that opens it on a phone or computer | No address or QR code |
| Focus starts on | **Back** | **Leave TV mode**, which turns TV mode off and opens the same page in the normal layout. A bar there offers **Back to TV mode** (or Stay here). |
| Rows that lead to such screens | "On a computer" | "Outside TV mode" |

Which one it is comes from what the client says, never from the size of the window (`tvClientOf` in
`web/src/tvmode/levels.ts`): the apps add `?client=tv` (Android TV's `pageAddress`, the desktop app on a Plasma
Bigscreen session) or `?client=desktop` (the desktop app started with `--tv` anywhere else) to the address, and the
page remembers it on that device (`wms.tvclient`). With neither, a browser whose name says it is a TV's (Tizen, webOS,
a Fire TV, a BRAVIA, a Google TV and others) is a TV device, and everything else is a computer. The apps' own TV
screens (Android TV, tvOS) are always the TV device's card.

The card is one centred column, as the sign-in screens are ("Who's watching?"): the server's name under a soft
glow, the screen's name, one short line ("Change this from a computer or phone."), the QR code large on a white tile
with "or open" and the address under it, what is set now, the buttons, and on a TV device the note above at its foot.
The Android TV and Apple TV apps draw the same card (`OnAComputerCard`). No keyboard keys are shown on a television.

Navigation never dead-ends. A button has focus, Back (or Escape, or B on a controller) leaves the screen the way it
came, and there are no controls a remote can't reach. Settings in TV mode opens on its menu, with each such row saying
so, instead of jumping into its first page.

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

In the web app: `web/src/tvmode/levels.ts` (the rules, tested by `levels.test.mjs`, which has a test for each kind of
client), `mode.ts` (`tvClient()`, `leaveTvMode()`, `backToTvMode()`), `levelTable.ts` (the table),
`TvLevelGate.tsx` (every page TV mode shows passes through it) and `OnAComputer.tsx` (the card, the dialog, the part
note and the read-only wrapper). In the Android app: `net/TvLevels.kt` and `Nav.serverPage()`.

`npm run test:tv-levels` in `web/` walks both kinds of client on a running test server at 1920x1080 with keys: the TV
device's card (QR code, Back focused, Back leaves) and the computer's (no QR code, Leave TV mode focused, the same
page in the normal layout, Back to TV mode).

## Keys on a television

A television never shows a computer's keys. The legend of keys a page's Dock carries elsewhere is one **Keys** button
in TV mode (`KeyInfoButton` in `web/src/ui/KeyLegend.tsx`), which opens the Keys dialog with the remote's keys, or the
controller's when a pad is in use; Back closes it. Keycaps on buttons, the now-playing and karaoke footers and More
info's "I" are left out, and the player's Keyboard shortcuts is Keys. The keys all still work. A screen that can't be
used without knowing a key shows that one hint itself, in the remote's words. `npm run test:tv-keys` in `web/` checks
every TV page on a running test server at 1920x1080 and 1280x720, before and after a key is pressed, and the dialog.

## Adding a feature

Every new screen or editor decides its TV level when it is made:

1. Can it be used with a remote or a controller alone: a few choices, switches, steppers, a list to pick from? Then
   it is `full` and needs nothing.
2. Does it need typing, a file or folder, a drag handle, a colour picker, or many fields? Add it to
   `pkg/sdui/data/tvlevels.json` as `desktopOnly` (or `view` when seeing the values on a television helps), with a
   `summary` if a line or two of what is set now would help, and implement that summary in
   `internal/server/api_tv_levels.go`.
3. A button that opens an editor stays on a television and opens `OnAComputerDialog` instead (Customise this page,
   Edit layout). The walls for advanced settings on phones (`web/src/ui/advanced.tsx`, `UseAComputer`: the Canvas
   studio, Advanced quality) show the same dialog in TV mode. Karaoke's Voice settings open everywhere on their
   simple presets (`full`); only their Advanced pane is walled, as the part `karaoke.voice-advanced`.
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
| Metadata agents | `/settings/metadata` | metadata agents on |
| Channels and tuners, bumpers | `/settings/channels/*`, `/settings/tuners/*` | channels, commercials and bumpers |
| Channel editor | `/tv/channels/:id` | channels, commercials and bumpers |
| Live transcoding | `/settings/transcoding` | graphics chip, largest picture |
| Library transcoding | `/settings/library-transcoding` | |
| Network | `/settings/network` | the address it listens on |
| Data folder | `/settings/data-folder` | where the data is |
| Remote Access | `/settings/remote` | on or off, and the public address |
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
editor, the advanced quality menu's fields, the Canvas studio, karaoke's advanced voice settings, WittyPlay's custom cores and importing game
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
