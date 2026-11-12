# Design 2 QA, pass 1

Written 2026-11-12 by W11 (design QA). Build: `v0.8.0-alpha` at `68bd4f9c`, web built fresh (`npm ci && npm run build`), a scratch server on port 19810 with an empty data folder and generated media only (FFmpeg lavfi clips and tones: three films, three episodes, one four-track album). Browser: the built-in pane, mostly 1440x900, plus 375x812 (phone) and 1920x1080 with `?tv=1`. Compared against `docs/design-2/images/*`, `docs/plans/DESIGN-2.md` and the 16 principles in `docs/design-2/README.md`.

Screenshots are kept outside the repository, in the QA scratch folder `/private/tmp/claude-501/-Users-willowpoortenga--claude-spares-spare-LJRByV/dd69ad04-94a2-4322-b0f5-8b1b95c40429/scratchpad/shots/` (file names are given per defect).

Owners: **W2** shell, player, editors, wizard, WittyTV. **W5** DesignPage gap pages. **W10** music and module pages. **W1** is named where the fix is on the server.

Severity: **Blocker** (a flow cannot be finished), **High** (breaks a stated rule or hides a feature), **Medium** (visibly differs from the picture or is hard to use), **Low** (polish).

## Summary

| Severity | Count |
|---|---|
| Blocker | 1 |
| High | 6 |
| Medium | 12 |
| Low | 7 |

The worst: first run cannot add a library or save the look (D1), the Classic Cable look makes the server's name invisible (D2), UIs do not bring their default theme (D3), the server's name is missing on TV mode and on the phone (D4, D5), and music has none of the Album art, Canvas and Visualizer views (D8).

## Defects

### Blocker

**D1. First run: after the account step every call is refused, so libraries and the look are never saved.** Screen: First run, `/setup/media` and `/setup/look`. Any look, any theme, 1440. What is wrong: "Add Movies" shows the raw text "Setup_required."; choosing a theme shows the toast "The look couldn't be saved. You can choose it later in Settings, Appearance."; the logos and listings steps show "Try again". The browser's calls to `/api/libraries`, `/api/settings`, `/api/me/ui`, `/api/prefs` and others answer 503 `setup_required`. Cause: `internal/server/setup_state.go` `setupAdminBrowser` lets only the administrator's cookie through and refuses any request that carries an `Authorization` header, but the web app sends `Authorization: Bearer <wms.token>` on every call (checked: the same `fetch('/api/libraries')` without the header answers 200, with it 503). Rule broken: README principle 13 and `DESIGN-2.md` first run ("every step is complete by keyboard, touch, remote and controller"); `OobeLive`/`OF_*` boards. Severity: Blocker. Owner: W2 (the wizard should not send the header during first run) with W1 (or the gate should accept the token the setup itself issued). Shots: `02-setup-library-setup_required.jpg`, `03-setup-look-not-saved-toast.jpg`. Workaround used for the rest of this pass: posted the libraries with the cookie only, then pressed "Start watching".

### High

**D2. Classic Cable: the server's name cannot be read.** Screen: Home, top bar. Look Classic Cable with the Witty theme (and with Cable blue), 1440. The bar is yellow; "Witty" and "Server" are white (contrast 3.8 and 1.4 against yellow) and "Media" is the theme accent yellow on yellow (contrast 1.00 to 1.35). Rule: standing rule "the custom coloured server name survives every look and theme"; principle 8; the `All_classicCable` board has a deep-blue bar with the name in static yellow and white; WCAG 2.2 AA. Severity: High. Owner: W2 (shell CSS for the Cable bar; the brand parts need a contrast fallback). Shots: `01-cable-brand.png`, `04-cable-look-light-theme-transition.jpg`. Checked all 14 looks with the Witty theme and Lumen with all 16 themes: only Cable fails.

**D3. A UI does not bring its default theme.** Screen: Settings, Appearance, and Home. Looks Paper, Classic Cable, Retro, Aero, Rail, Marquee and others, 1440. Choosing the look leaves the Witty theme on (dark violet): Paper is dark, Cable is not blue, Retro is not Teletext, Rail is not amber. Rule: principle 2 ("each UI has one default theme, always the same"), `UiTheme` board, `All_*` boards (Paper is cream, Rail amber, Cable deep blue). Severity: High (all 14 looks look alike except for their layout). Owner: W2 (with W1 for the table of default themes in `themes/tokens.json`).

**D4. TV mode has no server name.** Screen: Home at 1920x1080 with `?tv=1`. The top bar shows Home, Search, Live TV, Libraries, More and the profile; no name or mark anywhere in the DOM. Rule: standing rule (TV top bar), principle 8, `All_lumen` (name left of the pills). Also: Search sits second, not last before You (`DESIGN-2.md` section 1, item 4); no Dock or key legend is shown (the boards show X Look, Y Sort, LB Filter, Menu Edit layout, Start Options); the hero text starts at a different left edge from the shelf titles. Severity: High. Owner: W2. Shot: `09-tv-mode-no-brand.jpg`.

**D5. Phone: the server's name is not drawn.** Screen: Home at 375x812, Lumen. The top strip shows "Movies Music TV Shows Collectic..." (clipped, no Home or Live TV) and the notification badge; the name link exists in the DOM (215x40) but nothing shows it. Rule: standing rule, principle 6 (same sections and words on a phone), `PhoneA/PhoneB` boards. Severity: High. Owner: W2. Shot: `10-phone-home-no-brand.jpg`.

**D6. Music has no Album art, Canvas video or Visualizer views.** Screen: full player, More menu, Karaoke; Lumen, 1440. The full player shows only the cover with Lyrics, Karaoke, Read and speed. There is no Album art / Canvas video / Visualizer switch, no Visualizer, no effect picker (the 17 classics and Aurora Rings), no Follow along / Read split with Sooner and Later, and the karaoke screen has no Vocals / Instruments / Backing sliders or Canvas behind the words. Rule: principle 12; `MU_lumen`, `MU_aero`, `MU_classicCable`, `MU_rail`, `MU_retro`. Known gap in `HANDOVER-0.8.0.md` ("songs/karaoke/visualizer"); listed so it is not lost. Severity: High. Owner: W10. Shot: `14-now-playing-no-art-canvas-visualizer.jpg`.

**D7. First run has 12 or 13 steps, not the 14 drawn, and misses what the boards show.** Screen: `/setup/*`. The header says "Step 1 of 13" on Welcome and "Step 3 of 12" after the account (the total changes under the user). Missing against `OF_lumen`: the left step list; Language and time zone on the first step; Name, Username, Two-step sign-in on the account step; the "Name your server" step (name, colours, port, find files automatically, encrypted connections; the web offers only a Rename button on Welcome); "How people use it" (start with the look's own theme / let people choose) and "WittyTV look" on the theme step; "Require a look / theme" rules on People; a "Skip for now" on every step (Look and Theme have none). The server's own `SetupSteps` list has 14 ids (`welcome, account, name, look, theme, modules, libraries, details, livetv, games, discs, people, remote, ready`); the web plan (`web/src/pages/setup/plan.ts`) has no `name`. Severity: High (principle 13 and the "14 steps" bar). Owner: W2 wizard.

### Medium

**D9. Rail: the open state is reachable only by hovering, and the library drawer is drawn under it.** Screen: Look Rail, 1440. The "Open the side menu" button does nothing (the app stays `nav-rail`); the menu opens on hover only, and while the pointer is on it the drawer ("Recently added, Top rated, ...") is clipped behind it. The drawer is headed "Movies, 3 titles" (good) but its items are centred (the board left-aligns them) and "Collections" is marked as current while the page is Movies. The open rail lists libraries without counts or colour dots, has no "Keep menu open"-linked pin icon, and the profile is not at the foot (boards `RailStates`: counts such as 1,248, avatar and name at the bottom). Rule: principle 9, `RailStates`. Owner: W2. Shot: `11-rail-drawer-under-open-rail.jpg`.

**D10. WittyRemote floating panel is see-through.** Screen: `/play/1`, Remote button, Lumen, 1440. The panel is drawn at 45% opacity with no body fill (`.remote` opacity .45, background transparent), so the video shows through the keys and the label text; the boards show a graphite body with raised keys. It also sits on top of the "You might also like" card. Rule: principle 10, `PB_lumen` ("WittyRemote, the familiar floating remote"). Owner: W2. Shot: `05-wittyremote-translucent.jpg`.

**D11. Audio bar over the video mini player.** Screen: start an album while a film's mini player is open. The video mini player stays and the audio bar is drawn across its lower edge; both keep playing controls. Also the audio bar covers the page footer (no bottom spacing) and the WittyTV Dock legend. Rule: `DESIGN-2.md` zones (one Layer at a time). Owner: W2. Shot: `06-audio-bar-over-video-mini.jpg`.

**D12. "You might also like" appears 2 seconds into a 20-second film.** Screen: player. The next-up card opens at the start rather than near the end and sits over the picture while the Remote is open. Owner: W2 (check the threshold for short files).

**D13. WittyTV look: the default is Classic everywhere; the look's suggestion is ignored.** Screen: Settings, Appearance, WittyTV theme (Classic selected on a fresh server) and `/tv`. In Lumen the guide wears the Classic blue and yellow; `TvDefaults` has Lumen, Aero and Classic suggest Modern, Rail Classic, Stage Big picture, Retro Teletext and so on ("what a person picked, then the look's suggestion, then the owner's default"). Owner: W2.

**D14. WittyTV More info lacks most of what the board lists.** Screen: `/tv`, More info on the programme. The pop-up has two tabs ("This programme", "The whole day"), a line of times, a note and buttons. No programme logo, no description, no cast and crew, where to watch, trailers or also on; the title "We'll be right back" shows "12:00 AM to 12:00 AM" (a library channel of three 20-second files had no real programme: please check that `internal/livetv` fills a library channel from short files, W6 may own that part). Rule: principle 11, `TS_lumen`. Owner: W2 (screen) with the livetv owner. Shot: `13-wittytv-more-info.jpg`.

**D15. Crossbar is not a crossbar and its top bar overflows.** Screen: Home, Crossbar, 1440. "Collections" is cut by the search field, the category strip (Featured, Your libraries, Jump back in, On TV now, Recommended for you, Next up, ..., Recently add) is cut at both edges with a tab that has no label, and the shelf is a short single column. Rule: `All_crossbar`, `S_crossbar`. Owner: W2. Shot: `07-crossbar-home.jpg`.

**D16. Compact and Dashboard: the rail marks Settings as current on Home.** Screen: Home, Compact and Dashboard, 1440 (the gear is filled while Home is the page). The Dashboard's widgets are empty or bare ("Clock" has no content, three counters) with no "Edit layout" key. Rule: `S_compact`, `S_dashboard`. Owner: W2. Shots: `12-compact-gear-highlighted.jpg`, `08-dashboard-home.jpg`.

**D17. Looks that differ only in the shell.** Stage, Classic Satellite, Big picture and Marquee draw the same Lumen shelves with different top bars; Big picture does not enlarge its type ("Easy to see"), Marquee's "poster carousel" is three centred cards, Satellite has no silver bars. Already in `HANDOVER-0.8.0.md` as "not finished: the 14 shells in CSS" (cableBar, bevelBar, teleKeys, crossbar, floatingGlassBar, ornamentTabs). Listed for the next pass. Owner: W2.

**D18. Settings shell on a laptop: the section list has its own scrollbar and sticky tabs overlap the title.** Screen: `/settings/appearance`, 1440x900. The "On this page" tab strip sits on the "Appearance" title when scrolled; the left list shows a visible scrollbar. Owner: W2.

**D19. Cards: the poster card has the title below the art, not on it.** Screen: Home, Rail and Lumen. The Rail board draws title-on-art cards with the accent bar; the web draws titles beneath. Owner: W2.

**D20. Feature-gap screens are not in the web.** Screen: `/design` (DesignPage) lists tokens, buttons and states only; none of the 128 `gaps/GB2_*` screens (playback panels and errors, disc dialogs, library filters, audiobooks, podcasts, radio, WittyTV problems, accounts, offline, creators) is reachable as a page in the app or on `/design`. I could not walk modules (`/m/...`: the fresh server had no module installed and the store was not reachable), recordings (a library channel cannot be recorded) or the layout and name editors beyond the "Rename..." button. Owner: W5 for the pages, W10 for module pages.

### Low

**D21. Setup, media step: the folder box example is a Windows path on macOS.** Placeholder "...or type a path, e.g. D:\Movies" on a Mac. Owner: W2 (show the example for the host's system).

**D22. Setup account step: the two checkboxes have no accessible name** in the accessibility tree ("Show what I'm typing" and the legal acknowledgement read as unnamed `checkbox`). Owner: W2.

**D23. Settings, Appearance: each theme card's accessible name starts with the preview's dummy text** ("Home Movies Title A line of detail Play Witty default ..."). The preview should be hidden from assistive tech. Owner: W2.

**D24. Music home says "Recently played" over an album that has not been played** (the section shows the newest album). The album page has no artist line under the title. Owner: W10.

**D25. Item page shows "Audio: Unknown" and "Source: MOV" for an AAC-in-MP4 file, and the full server path of the file** to everyone who opens the page. Check that non-admins do not see the path. Owner: W2 (page) with W1 (probe).

**D26. Activity badge turns red ("2 failed") when a scan of invented titles finds no online match** (log: "metadata lookup failed, not found"). A missing match is not a failure to alarm about. Owner: W2.

**D27. Touch targets below 44 px on the phone:** the carousel dots and pause button are 24x24, "Activity" is 40x40, "Skip to content" 43 high. Rule: principle 16 (44 touch). Owner: W2.

## Not walked in this pass

- Module pages (no module installed on a fresh server), WittyTV recordings and sets with real programmes, WittyRemote on TV and phone widths, the layout editor, the server-name and mark editor (only the "Rename..." button on first run was seen), the policy ("Required by ...") locked states, Look/Theme import.
- Reduced motion: only checked in the CSS (`prefers-reduced-motion` blocks exist in 17 files); the pane cannot emulate the setting.
- Keyboard-only: Tab from the page start reached the hero buttons and the shelf cards with a visible ring and lift; the full Back ladder (Layer, Dock, Stage, Bar, Home) was not walked end to end, apart from Esc closing a menu before the player.
- Tablet widths and the other four first-run looks (the boards show Lumen, Rail, Aero, Cable) were not repeated.

## Method notes

Screenshots taken straight after a theme or look change can show a transition frame; every contrast and state above was re-read from computed styles after the page settled.
