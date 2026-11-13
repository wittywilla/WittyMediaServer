# Design 2: issue candidates for GitHub

Collected by the design lead from the agents' final reports. Nothing here is filed yet: the owner approves filing.
Format: title, area, what is known, what is needed. Items marked (gate) are release gates and are fixed before release,
not filed.

## Web (W12, W10, W5, W2, W13)

- **WittyPlay Online with real data.** Recent, Classics grid, room panel (Start game, Invite, Leave from the server's
  actions), Memories (SAVE/SHOT) and open rooms were only seen empty or in the `/design/play` harness: the test install
  had no games, friends or rooms. Needs a games library with emulator cores. Files: `web/src/sdui/render/components.tsx`,
  WittyPlay `sdui.json` pages.
- **WittyPlay controllers and quick menu as a layer.** Not seen with a connected controller; navigate-as-layer seen as
  a page, not a sheet over a game.
- **Pad map on a real controller** (A/B/X/Y on the new pieces): keyboard only so far.
- **WittyPlay extra screens** `wittyplay-kids`, `wittyplay-room`, `wittyplay-keep` not opened in a browser; none
  compared side by side with `docs/design-2/images/play` and `gaps/GB2_7.jpg`.
- **WittyPlay on phone and TV widths:** only the privacy form on a phone was seen.
- **Privacy form Cancel button** missing on the phone form: check the fixture sends `cancelLabel`.
- **Layout editor in looks other than Lumen, and the Look cards at phone width,** not looked at in a browser.
- **Language** setting: no interface translation exists (not built; dropped from the design).

## Music and modules on the web (W10)

- **Photos page hierarchy.** `/photos` is WittyPhotos' own `sdui.json` page (a shelf of recent photos per library) and the
  stock `photos` page in `pkg/sdui/stock.go` has the same bare list. A designed entry (Timeline, Albums, Map with covers,
  per the `photoalbums`, `photomap` and `photoview` boards) needs a change in the WittyPhotos repo. Owner: S8 and server core.
- **Plain pages:** Statistics and the WittyPlay Online home need design polish (owners S8, S5).
- **Phone top bar clips links at 375 px** (shell).
- **Sleep timer fade-out.** The board says the volume fades over the last 20 seconds; the player has no fade-out.
- **Karaoke:** Instruments and Backing rows appear only with a separated stem set; Speed's effect on the picture was not
  measured (animation frames are throttled in the headless browser).
- **Not verified in a browser:** WittyIntegratedFileBrowser and games pages, DVD/Blu-ray and CD playback, TV big-screen
  mode, WittyRemote and TV layouts of the music boards.
- **Smaller:** long drive names overflow the folder browser tile in Aero (module CSS); WittyIntegratedFileBrowser's
  `sdui.json` declares no pages or navigation; book and comic tiles with no cover show a gradient without a title.
- **Owner decisions:** whether the player should fade out for the sleep timer; whether Speed and Colour also appear in
  Settings, Modules, Visualizer (the stored keys already match).

## Android (W9) and web looks (W13)

- **Android visualizer:** the app has only Bars, Waves and Drift (`ui/Visualizer.kt`): none of the 17 effects or Aurora Rings.
  The effects are GLSL ES 3.00 in `WittyVisualizer` (`effects/*.glsl`, `shaders/*.glsl`, `engine.ts`, flash guard in
  `safety.ts`) and can run unchanged on a GLES30 renderer.
- **Android WittyRemote, revamped player controls, music toggle (Album art / Canvas video / Visualizer), lyrics modes,
  sleep timer, queue and the effects picker:** not built.
- **Android layout and name editors:** built on `wip/design-android-media-e` (commits 916b45a, 43b973a) but unmerged and
  unverified: run `gradle testDebugUnitTest lintSideload assembleSideload` before merging.
- **Android module pages** are SDUI-rendered (no native book or PDF readers).
- **Dashboard look:** no recently-added widget kind (server); Continue and On TV tiles appear only with history or live TV.
- **Big picture look:** its "What do you want to do?" tile Home needs a server page structure.
- **Crossbar look** shows a "Saved views" tab with no data behind it.
- **Item page shows the file's full server path** (QA D25).
- **Settings, Appearance, Look:** stray controls peek above the sticky tab bar (P7).
- **Phone top strip:** a partial word may still show at 320 px.

## Server SDUI pages and catalogue (W16)

Nothing here is built; all were cut from the release. Each would be additive to catalogue revision 8 unless it says otherwise.

- **Admin pages as SDUI (bulk edit, edit match, fix matches, library management).** The web has its own pages for all
  four, so nothing is missing there; an SDUI page would let Apple and Android draw them. The routes exist:
  `POST /api/meta/bulk` (bulk edit), `POST` and `DELETE /api/meta/{target}/{id}/match` and `POST /api/meta/{target}/{id}/preview`
  (edit match), `GET` and `POST /api/match/queue`, `/api/match/accept`, `/api/match/suggest-all` and `/api/match/rules/try`
  (fix matches), `GET` and `POST /api/libraries`, `POST /api/libraries/{id}/scan` and `DELETE /api/libraries/{id}`
  (library management; `library.scan` already exists as a command). Needs: a source for each list (the review queue, a title's
  match candidates, the libraries with their scan state), commands with their bodies, Form controls for the fields (the
  `object` param and `<param>.<key>` controls exist), and for edit match a way to show a candidate's "Why?" line. A match
  queue needs paging by cursor.
- **Smart playlist live match count.** The builder (`/smart-list`) saves a rule but cannot show how many titles match as the
  rule is typed: a Form has no way to send its current values to a source. Needs a new idea: a source that reads the Form's
  values (for example the page's query parameters filled from the controls) and `POST /api/collections/preview`.
- **Shorts and concerts.** No server route: shorts (vertical videos) have only the creators and comments APIs, and there is no
  concert listing (setlist from chapters, the artist as a channel). Needs the routes first (a shorts feed by For you, Following,
  Liked, Creators; a concert's chapters as a setlist), then sources and pages.
- **Creator pages with videos and playlists.** `/creator/{item}` shows the creator and a Subscribe switch only. A creator's videos
  and playlists need a source over `GET /api/creators/{id}` (playlists) and the creator's videos.
- **Download chooser (original file or a smaller copy).** There is no server API for download options; `native download` takes
  the title only. Needs the route (sizes and quality of each option), then a page like `/versions/{item}` whose rows start a
  download with the chosen option.
- **Second search chip group on every library.** Year and Score narrow film, show and anime libraries only (they use the
  library's own filters); music, books and games searches ignore them.
- **Notifications are reminders and the update only.** There is no notification store (watch-party invitations, task results
  and "Backup finished" are not listed); `/notifications` needs one, or sources over the task history.
- **A book's formats list is informational.** `formats` lists the other files with the same name beside a book; the server
  uses only the best one, so there is nothing to open for the others. Needs the scanner to keep the other formats as versions.
- **Empty states on the remaining pages.** The generic, photos, books and games library pages offer administrators "Scan now";
  music, audiobooks, podcasts and the live TV pages still have a bare message.
- **Module kit and clients on revision 8:** Apple and Android have not confirmed the revision-8 pieces (Form and list
  controls, regions, detail panel, QR, `object` params, the new stock-page routes); web has not drawn the Shelf `detail`
  panel, `document.regions`, the QR component or the settings slots.
