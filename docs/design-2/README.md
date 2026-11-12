# Design 2: the interface for every platform

This folder holds the design of the new interface: the boards as pictures, the boards as source, and the rules they follow. It is what to look at while testing on Windows, Linux or anything else: "does the app look and behave like this?"

The old design is gone. Design 2 is the only interface; there is no fallback to the earlier one. Settings stays hand-built in each app (same structure as before), wearing the new colours and type.

## Where things are

| What | Where |
|---|---|
| **Pictures of every board** (JPEG, open these first) | [`images/`](images/), one folder per page of the canvas (list below) |
| **Board sources** (`.dc.html`) and the canvas index | [`canvas/`](canvas/) (`canvas.json` has each board's page, position and size) |
| **The renderer all boards share** | [`canvas/Exp.dc.html`](canvas/Exp.dc.html): its tables are the spec for every look (colours, shapes, finish, type), every theme, every screen and every platform size |
| **WittyPlay Online renderer** | [`canvas/Play.dc.html`](canvas/Play.dc.html) and the themed copies `PlayP_*.dc.html` |
| **The contract** (principles, tokens, input, zones, screens) | [`../plans/DESIGN-2.md`](../plans/DESIGN-2.md) |
| **What SDUI needs that it didn't have** | [`../plans/DESIGN-2-SDUI-GAPS.md`](../plans/DESIGN-2-SDUI-GAPS.md) |
| **Features still without a drawn screen** | [`../plans/DESIGN-2-FEATURE-GAPS.md`](../plans/DESIGN-2-FEATURE-GAPS.md) (server and web) and [`../plans/DESIGN-2-FEATURE-GAPS-CLIENTS.md`](../plans/DESIGN-2-FEATURE-GAPS-CLIENTS.md) (apps) |
| **Tokens as data** | `themes/tokens.json` and `themes/<id>/theme.json` (generated; see [DESIGN.md](../DESIGN.md), [THEMES.md](../THEMES.md)) |
| **The SDUI reference** | [SDUI.md](../SDUI.md) and `docs/sdui.schema.json` |
| **Re-drawing the pictures** | [`tools/export-boards.py`](tools/export-boards.py) (Chrome or Edge, any OS) |

The live canvas, with every board in one zoomable page, is the Claude Design artifact
https://claude.ai/artifact/UhfRD9j84KVC84qJj4aXie (private to its owner).

### The pages of the canvas, and the picture folders

| Folder in `images/` | Page | What is on it |
|---|---|---|
| `looks/` | Looks on every platform | The first three concepts at full size, then each look on TV, desktop, tablet, phone and headset (`All_<look>.jpg`), and the rail's open and drawer states |
| `screens/` | All screens and menus | One board per look (`S_<look>.jpg`): library, item page with cast and crew, search, Settings, context menu, dialog, now playing, games, You, sign-in, TV guide, TV player, phone |
| `wittytv/` | WittyTV inside every look | The guide's look for each UI (`TvDefaults`), all eight WittyTV looks inside a UI (`T_<look>`), and the guide, More info pop-up and watch banner (`TS_<look>`), recordings, sets and Look (`TM_<look>`) |
| `firstrun/` | First run and editors | The interactive trial (`OobeLive`), the choose-a-look step, all 14 steps in four looks (`OF_<look>`), the layout editor and the server-name editor |
| `play/` | WittyPlay Online | Home, classics, game page, in-game menu, play together, controllers, memories, discover, phone, phone as controller, and the theme variants |
| `player/` | Player and WittyRemote | Standard and remote-strip player, DVD and Blu-ray with dimmed keys, WittyRemote on TV, desktop, phone and headset (`PB_<look>`) |
| `themes/` | UI and themes | `UiTheme`: UIs on the rows, themes across: they are separate choices |
| `modules/` | Modules | Every module drawn in three looks (`M_<look>`) and the table of every module function and where it is drawn (`ModuleMatrix`) |
| `music/` | Songs, karaoke, visualizers | Songs, now playing, lyrics (Follow along and Read), karaoke, the visualizer and all its effects, with Album art or Canvas video (`MU_<look>`) |
| `settings/` | Settings, policy, first use | Appearance, required look and theme, first connect on phone and TV, "server isn't set up yet" (`SB_<look>`) |
| `appscreens/` | App screens | Connect and away, profiles, screen time, pairing, remote and watch party, disc ranking, skip, WittyTV sheets and states, credits (`GB_<look>`) |
| `gaps/` | Every feature, drawn | The 128 screens that close the feature-gap list (`docs/plans/DESIGN-2-FEATURE-GAPS.md`): playback panels and errors, discs, library filters and collections, music, audiobooks, podcasts, radio, WittyTV problems, games, readers, photos, accounts, modules, admin, offline, creators (`GB2_1` to `GB2_7`). Each is a `gp` entry in `Exp.dc.html`, drawn in every look. |

The names of the boards use short look ids: `lumen`, `rail`, `stage`, `aero`, `aeroNight`, `aeroMeadow`, `aeroSunset`, `classic`, `classicCable`, `classicSatellite`, `compact`, `paper`, `marquee`, `crossbar`, `bigpicture`, `dashboard`, `retro`.

## Principles

1. **The artwork is the loudest thing.** Chrome stays quiet; surfaces step by tone. One accent colour means "do this next", progress and focus.
2. **A UI is not a theme.** A **UI** (a "look") is layout, shell, shapes, finish and type. A **theme** is colour (and scene). Each UI has one default theme, always the same; any UI can wear any theme, including imported ones. WittyTV's own look is a third, separate choice. WittyPlay Online is a UI for the games module that can be switched on or off on its own.
3. **Fourteen UIs, fifteen themes, and imports.** Lumen, Rail, Stage, Aero, Classic, Classic Cable, Classic Satellite, Compact, Paper, Marquee, Crossbar, Big picture, Dashboard, Retro. Aero's four themes are Day, Aurora, Meadow and Sunset.
4. **One input model.** Arrows, a controller and a TV remote do the same things: select, back, options, search, tabs, pages, with a key legend that matches what is in use. Back always steps outward one level. Nothing needs a pointer.
5. **Zones, not a maze.** Every screen has the bar (top menu or rail), the page, the Dock (the screen's controls, always in the same place) and layers (dialogs, menus, the player).
6. **The same screen on every platform.** A TV, tablet, desktop, phone and headset show the same sections and the same words; a phone is one column. The headset's menu uses the same item style as the desktop's.
7. **Everything can be moved.** Administrators and people can rearrange the shell and pages with the layout editor (pick up with OK, arrows to move, OK to drop, Back to cancel), for everyone or just for themselves.
8. **The server's name is a feature.** It is drawn from the theme engine's `brand` in every frame, with colours that follow the theme, fixed colours, gradients or per letter; the rail shows a centred mark (letter, initials, your own one or two characters, or a square PNG or JPEG logo up to 2 MB) that can be changed.
9. **The rail is a real side menu.** Closed it shows icons and the mark; open it shows the full name, every item with its full label, the libraries with counts and **Keep menu open**; choosing a library opens a drawer headed by its name.
10. **WittyRemote is the existing disc remote, everywhere.** Graphite body, raised keys, red eject, accent ENTER (TOP MENU, MENU, RETURN, ANGLE, AUDIO, SUBTITLE, transport, digits, colour keys). Keys a disc or video doesn't use stay in place, dimmed with a dashed outline, never hidden. Floating on desktop and headsets, a two-page sheet on TVs and phones, or a bar above or below the timeline.
11. **WittyTV is drawn on four fixed places** (preview, grid, Dock, banner and mini guide). Every WittyTV screen shows the programme's logo, title, times, a description and a **More info** pop-up (overview, cast and crew, where to watch, trailers, also on). Channel logos are contain-fit tiles, never cropped; titles wrap to two lines before they are cut.
12. **Music keeps everything it has.** All visualizer effects stay (the seventeen classics and the new one). Every music view offers Album art, the looping Canvas video or the Visualizer. Lyrics have **Follow along** (timed, press a line to jump, Sooner and Later) and **Read**; karaoke has both.
13. **First run is done in a web browser only.** Apps cannot connect until it is finished (the server answers `setup_required`); they show how to reach the setup page. The first-run page wears the look being tried.
14. **Administrators can require a look, a theme, or both** per profile or for everyone; the server resolves it and apps show it locked ("Required by …"). Themes can be imported in Settings and in first run. Apps offer the choice the first time they connect.
15. **Modules ship their own UI as SDUI** and draw in every look. Module settings stay in the hand-built Settings.
16. **Accessibility and speed:** WCAG 2.2 AA, targets of 44 (touch) and 66 (TV), reduced motion respected, no blur on anything that scrolls, effects that degrade on modest hardware.

## What is real and what is a drawing

The boards are designs, not the apps. They use invented titles and generated artwork only; no media of anyone's is in them. A feature drawn here may not be built yet in every app: check the "feature gaps" and the status in `HANDOVER-0.8.0.md` under "Design 2". Tell the design lead (or open an issue on the board) before building something that has no drawing.

## Looking at the sources

Boards are `.dc.html` files for the Claude Design canvas. They need that canvas's runtime script, which is not part of this repository. To draw them again (for instance after a change), save the runtime from the live canvas (artifact file `artifact-type/dc-runtime.js`) as `support.js` and run:

```
python3 docs/design-2/tools/export-boards.py --runtime support.js --jpeg
```

On Windows, use `py -3`, and pass `--chrome "C:\Program Files\Google\Chrome\Application\chrome.exe"` if Chrome or Edge isn't found. The picture of each board comes out at the size `canvas/canvas.json` gives it. `--only S_lumen` draws one board.

## Testing on Windows and Linux with these designs

Use the pictures as the reference: the same screen, at the same size, in the same look. For each look and platform check: focus is visible and moves as in the board; the Dock and legend are where the board puts them; the server's name is drawn in the colours chosen; the rail opens and its drawer shows the library's name; WittyTV titles are not cut and logos fit; the player and the WittyRemote match the player boards (dimmed keys stay in place); music views offer the art and Canvas toggle and both lyrics modes; first run only runs in a browser. Anything that differs from a picture is a bug, or a drawing that needs updating: say which.

## Decisions recorded

- Theme ids are lower case. Witty, True black and High contrast keep their older ids (`default`, `oled`, `contrast`); the new ones are `aeroday`, `aeroaurora`, `aeromeadow`, `aerosunset`, `cable`, `satellite` and `teletext`.
- During first run `/capabilities` stays open so apps can read `setupRequired` and show where to finish setup.
- The server's name logo is PNG or JPEG for now.

## Layout previews and the Connect screen

Look pickers show a wireframe of each look's layout, not only colours (`wireFor`/`wireSpec` in `canvas/Exp.dc.html`; the server's `wire` descriptor is the same data). Peer to peer is a prominent "Connect from anywhere" panel on the Connect screen and on You, From away.
