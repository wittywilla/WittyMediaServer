
## Design 2 (W1)

Built: design tokens (type roles, space steps, elevation, focus by input, input and nav tables, `tv` units, breakpoints, density, motion) in `web/src/styles.css` and `scripts/design-tokens.py` -> `themes/tokens.json`; TV pass (`tv` block) on the eight themes; the theme response's `tv`, `elevation`, `density`; SDUI catalogue revision 3 (`Dock`, `KeyLegend` required; Shelf `peek` and `seeAllCard`; Hero `peek`; `layout.peek`, `layout.dock`, text role `display`; TV safe area 90, TV text sizes 57/48/38; client command `control.open`); conformance fixtures (85) and schema regenerated.

Open / not verified:
- Full `go test ./...` was not run under a heavy slot (both were busy). Run: `go test ./internal/server ./internal/themes/... ./pkg/...` all pass; `go vet ./...` clean.
- The web renderer's conformance test (`web/src/sdui/model.test.mjs`) and the Apple and Android renderers must learn Dock, KeyLegend, `peek`, `seeAllCard`, `layout.dock`; until then they fail on the new fixtures (W2 to W4).
- `control.open` jump: the client pages the grid until a title with the letter arrives (at most 20 pages); there is no server-side letter index.
- Focus ring vs accent fills: the ring is checked against surfaces and neutral raised fills; an accent-filled control is separated from its ring by the focus gap, not by 3:1 colour contrast (gold on gold cannot pass).
- Fallbacks, `catalog`/`components` negotiation and the first-format `ui_config.json` conversion are removed (catalogue revision 4): every renderer draws every component. Modules declaring `catalog` below 4 in their ui.json are still accepted by the module validator (1 to 4).

## Design 2 (W2): web and desktop

Branch `wip/design-web` (worktree `.worktrees/design-web-WittyMediaServer`), NOT yet merged into `v0.8.0-alpha`: no heavy slot was free for `npm run build`, the browser checks and `npm run test:keys`/`test:tv` for the whole session. tsc, design-check, design-tokens --check, theme-contrast, the node tests, `npm run test:pads` and the 85 SDUI conformance cases pass.

Built: input model (`ui/inputModel.ts`, `ui/shortcuts.ts`, `ui/zones*.ts`, `ui/inputKind.ts`, key legend, Dock, key help), pad map in two contexts (app, player/book/games), spatial weights from `nav.weights`, SDUI catalogue 3 (Dock with `control.open`, KeyLegend, peek, See all card, grid/list view, letter jump), density row in Settings, `design2.css`, TV mode on `--tv-*` units, WittyTV guide Dock and half-window paging.

Not done / not verified:
- Nothing was run in a browser: no screenshots at 390/820/1440/1920/TV, `test:keys`, `test:tv` (its Look selector was changed to `.ctv-dock`), `npm run build`.
- Hero peek (phone swipe), the TV "Dock reached by Start" on the SDUI page, and Back-ladder wiring inside TvFrame (it uses `backStep`, the Dock step is in `ui/shortcuts.ts`) are not exercised.
- Fallback/legacy still present, per owner's "no legacy" rule: `TV_TABS` and the no-menu path in `tvmode/TvFrame.tsx`, the no-sets/no-paging branches in `pages/tv/GuidePage.tsx` (S6's logic), the games/reader pad map (`ownPad()` in `tv.ts`; games pages read the old buttons by key).
- Desktop (Electron) needs no change: it loads the web.
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
