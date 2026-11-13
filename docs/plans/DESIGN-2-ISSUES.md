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
