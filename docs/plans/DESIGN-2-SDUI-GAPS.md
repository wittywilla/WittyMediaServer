# Design 2: what SDUI does not have yet

Written 2026-11-12 from the concept canvas (17 looks, WittyPlay Online, the first-run trial, the layout and name editors).
Catalogue revision 4 is the base. Everything below is new work; nothing here is a fallback or a compatibility shim.

## 1. Already there (use as is)

Pages of `Hero`, `Shelf`, `Grid`, `Tabs`, `FilterBar`, `DetailHeader`, `SeasonList`, `TrackList`, `Banner`, `Dock`,
`KeyLegend`; card styles and sizes; five layouts (`sidebar`, `topTabs`, `tvRows`, `compactList`, `streaming`) with
`navStyles`; `NativeView` (`guide`, `photoTimeline`, `photoMap`, `coverFlow`); theme tokens, scopes and per-profile
colours; personalisation (reorder, hide, pin, density, card size); the `brand` parts of the server's name; the page
editor, presets, drafts and preview.

## 2. Looks (experiences)

| Need | What to build |
|---|---|
| An **Experience** as data (17 shipped: lumen, rail, stage, aero, aeroNight, aeroMeadow, aeroSunset, classic, classicCable, classicSatellite, compact, paper, marquee, crossbar, bigpicture, dashboard, retro) | `GET /sdui/experiences`, admin default per form factor, `allowed` list, per-profile per-device choice, `experience=` request override; extend the layouts model, do not add a second one. Contract: DESIGN-2.md section 7. |
| **Finish** (flat, gloss, bevel, glass) on Button, Card, Tabs, Dock, nav bars, Notice, dialogs, menus | Theme token group `finish` and per-role recipes (fill stops, highlight edge, outer edge, shadow, text shadow). Renderers draw recipes; no per-look code. |
| **Scenes** (sky gradient, bokeh bubbles, hill, light sweep, aurora, soft vignette) | Theme `scene`: ordered layers (`gradient`, `shape`, `radial`, `image`) with positions in percent, drawn behind the page and under the nav; reduced to the first gradient on modest hardware and with Reduce Transparency/Motion. |
| **Type sets** (system, humanist, rounded, mono, serif) | `type.set` tokens; bundle OFL fonts (Nunito, Source Serif 4, IBM mono) in every client with credits. |
| **Nav styles** beyond today's six | `cableBar` (flat blue rows), `bevelBar` (silver bevel), `teleKeys` (colour-coded keys), `crossbar` (category bar), `railDrawer` (7.4), `floatingGlassBar`, `ornamentTabs` (headset). Each is data in `navStyles` with sizes and states. |
| **Card styles** | `plate` (title on art), `block` (teletext cells), `tile` (huge tile with label), `logo` tile (satellite channels). |
| **Section styles** | `band` heading (cable's coloured header row), underline/pill/gloss `Tabs` styles. |
| New **components** | `Carousel` (centred poster carousel with focus scale; marquee), `CrossBar` (category bar and a vertical list under the chosen one; crossbar), `Widget` and `WidgetGrid` (cells `{x,y,w,h}`; kinds `continue`, `liveNow`, `stats`, `serverStatus`, `clock`, `recordings`; dashboard), `NumberedRow` (listing row with channel or index column). |
| **Placements** | `experience.placements` (admin) and `personal` (profile): regions brand, nav, search, profile, clock, dock, legend, hero and sections, with locked Settings and You. Merge with the existing precedence. |
| **Layout editor** | Routes for editing placements with undo/redo, preview per form factor, presets, rollback; native editors in each client (pick up with OK, arrows, drop; drag on pointer and touch). Reached from the hand-built Settings, Appearance. |
| **Rail** | Navigation document carries each library's views for the drawer (`navStyles.rail.drawer`), pinned state per device. |
| **Server name** | `brandname.Part.color` kinds (fg, accent, token, static, gradient), per letter or word, presets, contrast warnings per theme and look; theme engine `brand` carries it. |

## 3. WittyTV inside any look

The guide stays independent of the look:
- `tvLook` (exists in `/me/ui`) gains the precedence **person, then the look's `tvTheme`, then the owner's default**. The experience carries `tvTheme`.
- The eight looks (modern, classic, satellite, teletext, listings, bigpicture, outline, compact) plus custom become **catalogue data** (`tvThemes`: colours by token or static, cell shape, row height, preview place and size, fonts, dock style, rows per page), so every client draws them from the same table instead of porting `tvThemes.ts`.
- `NativeView guide` takes `tvTheme` and reads the resolved theme from the document; the guide's Dock takes the look's `finish`.

## 4. The headset (visionOS, Quest)

- `platform=visionos|quest`, `formFactor=headset`; `layout.shell.nav = ornamentTabs`; window sizes and depth tokens (focus lift in depth, gaze highlight), minimum target 60 pt, no edge-hugging content.
- The highlighted menu item uses the same nav-style recipe as desktop (pill, tab, gloss...). The concept canvas now draws it that way; the shell must not restyle it per platform.

## 5. First run: try a look

- `GET /sdui/preview?experience=<id>&platform=<p>`: a sample Home document built from generated demo data (no library access), served before sign-in with a rate limit and a strict auth rule, so the chooser can draw every look with the client's own renderer.
- `experience=` accepted by `/theme` and `/sdui/navigation` so the setup screens (native, hand-built) restyle themselves from the trial look's tokens, finish and type set.
- Setup stays native; it only adopts tokens. The choice is stored as the server default, changeable under Settings, Appearance.

## 6. WittyPlay Online (Switch-Online-style experience)

Original design; no third-party marks. Screens: home (focus tile grows), classic collections by era, game page, in-game menu (save states, rewind, screen and sound, controls, invite), play together (friends, room, open rooms), controllers, memories, discover, phone companion and phone as a controller.

| Need | What to build |
|---|---|
| Home row where the **focused tile grows** and the rest stay small | `Shelf` prop `focusGrow` (scale and neighbour offset), with the info panel under the row bound to the focused item (`Shelf` `detail` slot). |
| **Native views** | `NativeView` views `gamesHome`, `quickMenu`, `lobby`, `controllers`, `memories` (clients that lack one get the sections inside). |
| **Shell regions** | `statusBar` (clock, controller batteries, server name) and a circular `systemDock` (Discover, Together, Controllers, Memories, Settings). |
| **Data sources** | `consoles` (collections by era with counts), `friends`, `presence`, `rooms`, `saveStates`, `memories`, `controllers`. All through the profile's limits; Kids profiles see only approved friends and no open rooms. |
| **Commands** | `room.create`, `room.join`, `room.invite`, `state.save`, `state.load`, `state.delete`, `rewind.open`, `controller.assign`, `controller.remap`. |
| **Server and engine (S5)** | Rooms and presence service, friend graph with privacy and parental rules, rollback netplay in WittyPlay, save-state storage per profile with cloud sync, voice chat (optional, off for Kids), phone-as-controller pairing by QR, clips and screenshots store. |
| **Theme** | WittyPlay is a theme scope (`wittyplay`): coral and teal pair, glossy tiles. It may be worn by any look. |

## 7. Order to build

1. Experience model, placements and brand colour kinds (server), with the OOBE preview route.
2. `finish`, `scene`, type sets in the theme engine, then the shell styles.
3. Components: `Carousel`, `CrossBar`, `Widget`/`WidgetGrid`, `NumberedRow`, card and section styles.
4. WittyTV looks as data; precedence; guide Dock finish.
5. Layout editor routes and native editors; name editor.
6. WittyPlay Online: native views, data sources, commands, with S5's rooms, presence and netplay.
