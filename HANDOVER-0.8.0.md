
## Design 2 (W1)

Built: design tokens (type roles, space steps, elevation, focus by input, input and nav tables, `tv` units, breakpoints, density, motion) in `web/src/styles.css` and `scripts/design-tokens.py` -> `themes/tokens.json`; TV pass (`tv` block) on the eight themes; the theme response's `tv`, `elevation`, `density`; SDUI catalogue revision 3 (`Dock`, `KeyLegend` required; Shelf `peek` and `seeAllCard`; Hero `peek`; `layout.peek`, `layout.dock`, text role `display`; TV safe area 90, TV text sizes 57/48/38; client command `control.open`); conformance fixtures (85) and schema regenerated.

Open / not verified:
- Full `go test ./...` was not run under a heavy slot (both were busy). Run: `go test ./internal/server ./internal/themes/... ./pkg/...` all pass; `go vet ./...` clean.
- The web renderer's conformance test (`web/src/sdui/model.test.mjs`) and the Apple and Android renderers must learn Dock, KeyLegend, `peek`, `seeAllCard`, `layout.dock`; until then they fail on the new fixtures (W2 to W4).
- `control.open` jump: the client pages the grid until a title with the letter arrives (at most 20 pages); there is no server-side letter index.
- Focus ring vs accent fills: the ring is checked against surfaces and neutral raised fills; an accent-filled control is separated from its ring by the focus gap, not by 3:1 colour contrast (gold on gold cannot pass).
- Fallbacks, `catalog`/`components` negotiation and the first-format `ui_config.json` conversion are removed (catalogue revision 4): every renderer draws every component. Modules declaring `catalog` below 4 in their ui.json are still accepted by the module validator (1 to 4).

## Design 2 (W2): web and desktop

Merged into `v0.8.0-alpha`. Built: the one input model (keys, pad, zones, key legend, Dock, key help), SDUI catalogue 6 (Dock with `control.open`, KeyLegend, peek, See all card, Carousel, CrossBar, WidgetGrid, Widget, NumberedRow, card styles plate/block/logo; conformance green), experiences (Look rows in Settings, Appearance with required locks, the admin's Looks editor with required looks per everyone/kids/profile, finish/type/density from the theme engine's `look` on `<html data-experience, data-fx-*, data-type>`), theme import for profiles, frame from every nav style, rail with the mark badge, open menu, Keep menu open and the library drawer, the Mark row under Name and logo, first run with Look (14 experiences tried as chosen) and Theme steps, setup state and setup complete kept on the server, 503 `setup_required` goes to `/setup`, TV mode on the `--tv-*` units, WittyTV guide Dock with More info (I), the legacy WittyTV look readers deleted.

Not built (owner's list): finishing every one of the 14 shells in CSS (only the rail, sidebar and top bars are drawn; cableBar, bevelBar, teleKeys, crossbar, floatingGlassBar, ornamentTabs are laid out as top bars without their own finish), the other first-run steps the server names (name, people, remote), WittyRemote and the player bar variants, More info on the watch banner, recordings and sets, songs/karaoke/visualizer, module pages in every look, the layout editor and name editor, WittyPlay Online. See `docs/plans/DESIGN-2-FEATURE-GAPS.md` for web features with no canvas element.

## Design 2 (W3): Apple apps

Merged into the client's `v0.8.0-alpha` (client `3d48fe9`). Checked: `swift test` in apple/WittyKit 644 tests green (SDUI conformance 85 of 85 with the fixtures required), tvOS and iOS simulator builds clean; one real run of the tvOS app on the Apple TV simulator against a scratch server (generated media): Home with the top pills, billboard, the concentric focus ring on Play, shelf with peek.

Built: `scripts/gen-design-tokens.py` (strict, `--check`; writes `DesignTokens.generated.swift` and the Android `.kt`) and the pure rules `InputRules.swift` (key map from the `input` tokens, Back ladder, zones, focus memory, spatial scoring from `nav.weights`, letter jump, paging, density), `TvGeometry.swift` (peek widths, concentric ring). Theme: `focus`, ten-foot ring colour, `tvText`, `heroScrim` read from `/theme`. tvOS: one focus language (`TvFocus.swift`: ring + 1.06 lift, no system tilt), tokens in `Tokens`/`Metrics`, top pills from the tokens (no own-tab fallback), shelf peek and See all card from the page, Options key on cards (long OK context menu and Play/Pause). WittyTV guide: the strip under the grid is now the Dock (Look, Day, Channels, Filter, Recordings, Record) with sheets and the key legend; player: Left mini guide, Right Dock (seek, audio, subtitles, Look). iPad/Mac/visionOS: keyboard shortcuts (Cmd or Ctrl+K search, Ctrl+[ and ] tabs, Cmd+[ back) and a key focus ring from the tokens. S3 landed the SDUI Dock/control.open at the same time; theirs stands, mine was dropped in the merge.

Known issues / not verified:
- The simulator's keyboard could not drive the tvOS remote, so Dock, grid focus, the guide Dock and the player Dock were built and compiled but not seen running; no screenshots of them. Files: App/TvGuide.swift (`dock`), App/TvGuideDock.swift, App/TvChannel.swift, App/TvChannelDock.swift.
- Banner record dot, player Quality button and per-button key glyphs on the WittyTV Dock are not done (no live-quality API found; glyphs only in the legend).
- Gamepad Y (search) and Start (Dock) are not wired on tvOS; the options key is Play/Pause or long OK.
- macOS and visionOS targets not built (only tvOS and iOS). Density (compact/spacious) is in WittyKit but no screen reads it yet.
- Guide `TvStyle` ring has no gap off the thing (look identity kept); Settings only takes the new ring colour/scale.
- Generated ffmpeg test clips of 20 s and 130 s with 8-12 fps from my own commands were not indexed by the scratch server (copied clips were); cause not found.

## Session 12: cast and crew (actors, directors, set designers, every credit)

The owner asked for actors, directors, set designers and every credit under all films and shows. Built in the
server and the web client; the Apple and Android apps draw it from the same contract (sessions 3 and 4).

- **Data** (`internal/library/people.go`, metadata migrations 12–13): `people` (name, sort name, photo, birth and
  death, birthplace, biography, known for, TMDB/IMDb/TheTVDB/TVmaze ids) and `credits` (person, item or show,
  role cast / guest / crew, department, job, character, order, source), plus `credits_read`. All three are in
  backups (library part). People are kept once: by an external id, else by name. Each source's credits are kept
  apart and a page takes each part (cast, guests, each department) from the best source: NFO, TMDB, TheTVDB,
  TVmaze, file tags. Sources: `credits_nfo.go`, `credits_online.go`, `credits_local.go`, `credits_sources.go`;
  NFOs written with people when "save information beside the media" is on (`credits_nfo_write.go`); a paced
  backfill after each scan fills titles matched before credits existed and people's biographies
  (`credits_backfill.go`).
- **API**: `GET /api/v1/items/{id}/credits`, `/shows/{id}/credits?season=`, `/people?search=`, `/people/{id}`,
  `/people/{id}/photo`, `person=` on `/browse` and saved views; features `credits`, `people`, `sdui-credits`.
  The limits walk (`authz_parental_walk_test.go`) now has people on hidden titles and walks every person id.
- **SDUI catalogue revision 5** (on Design 2's no-fallback contract): `Credits` component under the header of the built-in film, show and episode
  pages (a show opened with `?season=` adds that season's guest stars), sources `credits` and `filmography`,
  people in `search` (`in: people`), the person page `detail-person` at `/person/{person}` with rows repeated
  over `personParts`, facts `knownFor`/`born`/`died`. Six new conformance cases. Docs: SDUI.md "Cast and crew",
  METADATA.md "Cast and crew", CLIENT-API.md "People".
- **Web**: `CreditsView` (cast and guest rows of round portraits, initials when there is no picture, crew in
  columns of departments, Show all), the person page, catalogue 3; TV mode styles.

### Known issues / not verified
- Seen on a scratch server against the owner's test media (saveMetadata off): a film's cast and crew from its
  movie.nfo, a show's from TVmaze with photos, a person page. No TMDB key there, so no biographies were seen;
  TMDB, TheTVDB and TVmaze parsing is verified against hand-written JSON fixtures only
  (`internal/library/credits_sources_test.go`).
- `RefreshShow` and the scan-time TheTVDB/TVmaze show lookups are not tested end to end (the scanner has no
  base-address hook for those two services); their parsing and storage are.
- File tags (`©dir` in MP4) are unit-tested only: the tests have no ffprobe.
- TVmaze gives no per-episode guest stars (one request per episode); TMDB and TheTVDB do.
- A film's or episode's NFO edited without the media file changing is read on Refresh metadata only (as every
  NFO field already was); show NFOs are read every scan.
- Apple and Android fail the six new fixtures until they draw `Credits` (S3/S4, W3/W4). The web's conformance
  test also fails on upstream's `peek` (Design 2, W2's work), not on credits.
- The design lead's canvas draws the crew as "Role · Name" chips and a separate "Cast and crew by department"
  page; the server sends the crew as person cards grouped by department in one `Credits` node with Show all.
  If the canvas's separate page is wanted, it needs a route and page of its own (not built).
- Fixed on the way: a film folder with an NFO this server wrote earlier (no people) beside a media centre's
  movie.nfo now takes the people from movie.nfo; the title header's poster no longer hides under its placeholder.

### Design 2 (W1), second round

Landed: the five layout options are gone; **experiences** (14, `pkg/sdui/experiences/*.json`) replace them, with their routes (`/sdui/experiences`, `/sdui/experience`, `/sdui/config/experience`, `experience=` on pages, navigation, theme and the preview) and a headset form factor. UI and theme are separate: 15 built-in themes with `finish` and `scene`, `look` in `GET /theme`, required look and theme (everyone, kids, one profile) with who set it, theme import (admin; per profile when allowed, `POST /themes/mine`), and themes are variables only (a ZIP with class rules is refused, naming the rule). First run: `setup_required` gate (503), `/setup/state`, `/setup/complete`, `/sdui/preview`. Brand: per-part colours (fg, accent, token, static, gradient), `brandSpans`, `brandMark`, contrast warnings. Catalogue revision 6: `Carousel`, `CrossBar`, `WidgetGrid`, `Widget`, `NumberedRow`, card styles `plate`, `block`, `logo`, `headingStyle`, nav styles, `serverStatus` source, rail library drawers, placements (regions, locked menu and You), WittyTV looks, WittyRemote keys and music/karaoke/visualizer controls as catalogue data.

Not done or not verified:
- WittyPlay Online sources (`consoles`, `friends`, `presence`, `rooms`, `saveStates`, `memories`, `controllers`) and commands (`room.*`, `state.*`, `controller.*`): the engine, rooms and presence are S5's; nothing is registered here because nothing backs them yet.
- More info data for any programme: built by S6, `GET /tv/{key}/programme` (see "Session 6: WittyTV More info" below).
- Logo for the name: the existing `PUT /branding/logo` (PNG or JPEG, 2 MB) is the mark's image; SVG and WebP are not accepted (no decoder).
- Theme ids are lower case (`aeroday`, ...), and Witty, True black and High contrast keep their ids `default`, `oled`, `contrast`.
- The layout editor's undo, redo, presets and per-form-factor preview are the apps' own over the existing revisions and `personal`; no new routes beyond placements.
- The setup gate keeps `/capabilities` and `/identity` open (`setupRequired`, `setupURL`); the OOBE's 14 steps are the web's to draw. FIRST-TIME-SETUP.md describes the new flow in its introduction; its page-by-page pictures are the old twelve.
- Full `go test ./...` and the web, Apple and Android checks were not run here.

## Session 6: WittyTV More info

Landed: `GET /api/v1/tv/{key}/programme?start=<unix>` (feature `tv-programme-info`) backs the Design 2 More info screen (`Exp.dc.html` `tvinfo`). It answers `channel`, `programme` (the facts line: rating, year, SxEy, length, genres, scores, CC/AD/audio/HD, languages, country, studio), `actions` (watch, record, remind/reminded, fromStart, watchLater/inList, open), `library` (item or show the profile may see), `credits` (S12's `library.Credits`: the library's, else the guide's XMLTV credits with `source: "guide"` and no person id), `trailers` (the library item's extras), `schedule` (other airings in 14 days on channels the profile may watch) and `ids` (imdb, tmdb). The XMLTV reader keeps credits, star ratings, languages, subtitles, sound, picture, new/repeat and ids (WittyTV migration 8, `epg_programmes.extra`, bounded; programme pictures are not kept). The SDUI `credits` source takes `programme: "<key>@<start>"`. Recordings (`GET`/`POST /recordings`) carry `channelKey` and `programme`, and SDUI `recordings` cards `channel` and `programme` (feature `tv-recording-refs`), only where the profile may watch that channel. Merged at ce29d9f8. Docs: CLIENT-API "More info", CHANNELS "For developers". Server tests: XMLTV details and bounds, `ProgrammeAt`/`Airings`, the route (guide-only, library match, refusals), the credits source, recording refs.

Release gate status (WittyTV guide and More info): the server side is done and tested. The screens are the client agents' (W6 reported Apple More info from the guide, the channel dock, the mini guide and recordings); S6 has not seen them running.

Issue candidates (not filed; for the owner to approve):
- More info "Advisory" line in the More section has no data source; critics' scores appear only when the guide or library gives one.
- Library matching is by title (shows) and title plus year (films): a guide title spelt differently from the library's falls back to the guide's own data. A match on the guide's IMDb/TMDB ids against the library's would be firmer.
- Where to watch on More info relies on the guide giving a TMDB id; guides without ids show only "On this server".
- No SDUI catalogue page or conformance fixture uses the `credits` source's `programme` parameter (revision 8 is final; the screens are native).
- More info for a recording older than the guide's kept past (about a month) answers 404.
- The guide grid itself was last profiled on Apple TV and Android TV before Design 2's Dock redesign; a fresh pass at 10,000 channels on each TV client is worth doing.

## W13 (web design finish)

Built: look finishes from the theme `look` block (scene layers, glass, gloss, bevel, glow, texture, focus style, shape multiple, `web/src/finish.css`, `ui/Scene.tsx`); bundled OFL type sets (Nunito, Source Sans 3, Source Serif 4, JetBrains Mono); one player control bar (timeline with scrub and preview, Prev, -10, Play, +30, Next, Audio, Subtitles, picture chip, speed, Chapters, WittyRemote, More, Full screen) and an error state with Try again; WittyRemote labels, heading, rule, bar permission and sheet pages from the catalogue `remote` table, sheet on TV; WidgetGrid placed by x, y, w, h with a live clock; Carousel driven by scroll, CrossBar circles with preview, NumberedRow numbers, band and underline headings, listing rows; WittyTV Look page redesign; QA D2, D4, D5, D10, D15 fixes.

Known issues / not verified:
- Nothing was run against a real server: both heavy slots were held throughout. Checked with tsc, build, design-check, contrast, 548 node tests, and CSS in static pages in a browser tab (Aero glass, gloss, scene, player bar at desktop and phone; Cable listing and band; carousel; crossbar). Not checked: the React player with a real stream, TV widths, WittyTV Look page in a browser, Dashboard with real data.
- Aero gloss text (accent-fg over the recipe's upper body) is about 3:1 at the mid line; the recipe is the theme's, so a contrast fix belongs in the theme's gloss colours.
- QA D16 (rail marks Settings current on Home in Compact and Dashboard) not reproduced in code: NavLink "/settings" cannot match "/". Needs a look in a browser. Search order on the TV bar (D4) comes from the server menu, not changed.
- Server name in a fixed colour that fails contrast against its bar still needs the theme engine's warning (server side).
- Seek preview pictures exist only where `chapterThumb` is given (discs); plain files show time and chapter name. The "mini player" and next-episode card keep their earlier layout.

### W13 round 2
Built (no live server, light slots were all taken for 15+ minutes): player errors classified (`player/errors.ts`: network, decode, unsupported, ended, subtitles) with a way forward and "Play it converted"; next-episode card (8 s bar, Play now, Watch the credits; appears only in the last 15% of a short file, D12); Viewing settings grouped with pills; mini player progress line; look marks for Rail (title-on-art), Big picture (type, focus), Satellite (silver bars), Retro, Paper; gloss paint kept at the recipe's stops with W7's darker gloss colours carrying contrast.
Still not verified in a browser against a real server: everything listed under round 1 "Not verified", plus D16 (Settings marked current on Home in Compact and Dashboard: code shows no cause; reproduce in a browser) and the 17-look side-by-side comparison.

### W13 round 3 (checked on a real scratch server, generated media, one tab)
Seen working: the React player with a real stream (bar, chips, skip, solid WittyRemote with dashed dimmed keys; phone sheet solid), TV mode with the server's name, Aero scene and glass, Cable listing, Rail, Retro, Stage, Paper on a phone (serif, hairlines, 44 px Customise, ALPHA badge), Satellite silver bar, Dashboard with real data (widgets packed, figures), Crossbar, the WittyTV Look in Settings, arrows reaching Legal in TV mode (not reproduced). Fixed on the way: Dashboard row gaps, crossbar panel width, phone server name, 44 px sheet keys, Satellite search and badge colours.
Not verified: the music bar over the film mini player and a pinned rail (no audio media; W2 landed a similar fix, both kept), D16 (Settings current on Home: not reproduced in Compact or Dashboard), 3+ looks against every canvas image and TV widths of the Look page, the next-episode card live (D12 only by code).

## W5 (web gap pages on SpecPage)
Built: `web/src/spec/` (SpecPage, spec model, 124 canvas pages as data at /design/gaps), form fields, row icons and actions, per-look finishes. Converted pages are listed in the W5 reports; specs are in `web/src/spec/specs/`.
Known issues / not verified:
- Converted dialogs and panels (bulk edit, collection rules, share, radio, creators, albums, games sheets, module store and detail, tuner setup, channel editor parts, Fix matches panel, You, unreachable screen, game review list) were type-checked and unit-tested; only Settings Backup, Diagnoser, Help, /pair and /design/gaps were seen rendered (Lumen, Retro, Cable, phone, TV). Walk `test:keys` on them with media; a partial full-crawl showed: `/settings/about` focus under `.set-toc-label` (big screen), `/settings/pages/home` help link under avatar name, `/` See all cards unreachable by arrows on TV (server page), `/you` at desktop redirects so the walk stops.
- No controller pad pass done on converted pages (`npm run test:pads` only covers its own simulated pads).
- The TV-mode dock inside embedded SpecPage was restyled after the last screenshot.
- Left hand-built: module cards, versions dialog, library transcoding recipes, channel editor colour/profile pickers, Fix matches list and shortcuts, Brand grid, comments, photo map, shorts, controllers (WittyPlay module), PC stream and in-game menu (module/emulator surface).
- Server pages missing against the canvas (W16): Search scope selector and People row, Home Recommended.
