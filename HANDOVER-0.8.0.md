
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
