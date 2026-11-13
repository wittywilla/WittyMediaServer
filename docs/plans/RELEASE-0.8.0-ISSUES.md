# Issue candidates for 0.8.0-alpha (deduplicated; for the owner to approve, then file)

Collected by S13 (release engineering). Nothing is filed: the owner approves, then files them (plan: [RELEASE-0.8.0-PLAN.md](RELEASE-0.8.0-PLAN.md), step 8).

Collated 2026-11-12 (read-only) from:
- `DESIGN-ISSUES` = server `v0.8.0-alpha:docs/plans/DESIGN-2-ISSUES.md` (primary)
- `QA-1`..`QA-5`, `RELEASE` ("Known gaps accepted"), `RESUME`, `SDUI-GAPS`, `FEATURE-GAPS`, `FEATURE-GAPS-CLIENTS` = the other
  `docs/plans/DESIGN-2-*.md` on `v0.8.0-alpha` (commit 97c8bfec). **No `DESIGN-2-PARITY-*.md` or `DESIGN-2-RC-*.md` exists on the tag
  yet**, so parity findings are not in this list.
- `HO` = `HANDOVER-0.8.0.md`, by section (e.g. `HO s2 #3`, `HO karaoke #6`, `HO W4 #2`).
- `note s4` etc. = the working sessions' own notes (not in the repository).

Only open / not done / not verified / deferred items. Items a later section marks fixed were dropped (for example QA D1, D2, D3, D5,
D7, D13, D15, D22, N2, N4, N5, N7, N8, M7, M1 except the file browser; the Apple Home spinner; the artist page; hero images; show
command templates; Watch later toggle; WittyCouch polling; HOST_API 4; Android connector library; books series source; cast and
crew source; disc negative cache and background-ranking tests; module test bundles without `sdui.json`; docscheck failures).

Repo is `wittywilla/<Repo>` and is given by each section heading (it applies to every issue under it; cross-repo pointers are
in "Where"). "Server" includes the web app (`web/`), which lives in WittyMediaServer.

Unmerged work at the time of collation (`git branch --no-merged v0.8.0-alpha`): server `wip/design-web-finish`,
`wip/design-web-gaps`, `-gapsQ1..Q3`; client `wip/design-android`, `-android-media-e`, `-android-rev8`, `wip/design-apple`,
`-apple-finish`, `-apple-mac-vision`, `-apple-pages`, `-apple-rev8`, `wip/s3`, `wip/s4`; WittyMediaPlayer `wip/design-player`.

---

## Decisions waiting on the owner

Decided since (2026-11-12): the peer-to-peer "address book" (public DHT rendezvous for joining by code) is **on by default whenever peer to peer is on**; it keeps its own switch, and an owner who turns it off keeps it off. It already behaved so; S1 added a test and plainer wording (server 79b2a4c7).

1. **SDUI library and detail pages on the web: bring back the old tools or accept their loss.** Bulk edit, saved views, category
   editor, view settings, Scan, music tracks/playlists tabs and Play all, Find intros and credits, show cards/playlist switch,
   collections manager. Related accepted gap: admin pages (bulk edit, edit match, fix matches, library management) as SDUI forms.
   Sources: HO "Waiting on the owner", HO SDUI web; RELEASE. Look: `web/src/sdui/extras/DetailExtras.tsx`.
2. **Peer-to-peer invite without the administrator's password.** `POST /api/v1/p2p/invite` signs in as an administrator without
   asking the password again, unlike `/pair/approve` and `/p2p/join` (later work sends the password "where the server asks";
   confirm the server's current rule). Sources: HO "Waiting on the owner", HO p2p #7, HO s1 #9, HO s3 5bb35bc.
3. **Join from away / invite signs the device in as the approving account; From away row shown wherever the library is.**
   Sources: HO s4 #9.
4. **Flatpak's permanent name.** Sources: HO "Waiting on the owner".
5. **Whether Flatpak or Snap failures hold up a release.** Sources: HO "Waiting on the owner".
6. **The Switch emulator in the catalogue** (Lichii removed from the catalogue after the Nintendo takedowns; it is added by hand
   and works). Sources: HO "Waiting on the owner", note s5, HO s5 follow-up #11.
7. **Others' modules on macOS under the `weak-sandbox` override, or refuse them again.** sandbox-exec is deprecated and cannot
   stop reads. Sources: HO s1 #13.
8. **May a parent lock a profile's own colours?** Sources: HO s1 #10.
9. **Series recording `keep` deletes recordings and files of a rule regardless of who made it** (before, only an admin could
   delete a recording file). Sources: HO s6 #6.
10. **Sleep timer fade-out** (board says volume fades over the last 20 s; player has none) and **whether Visualizer Speed and
    Colour also appear in Settings, Modules, Visualizer.** Sources: DESIGN-ISSUES (owner decisions).
11. **Karaoke control set**: board has Vocals, Instruments, Backing vocals sliders and Key, Mic; the app has Voice on/lower/lowest,
    Key, Words on time. Also "Mix page as rows". Sources: QA-5 P3, RELEASE.
12. **Apple apps starting offline**: show kept pages (needs a signed-in connection to a server not re-identified) or keep going to
    Connect. Sources: HO s3 known issues.
13. **Old games pages vs WittyPlay Online**: which replaces which on the web. Sources: FEATURE-GAPS appendix (Games row).
14. **WittyReader: vendored AGPL renderer files in a GPL repo** (NOTICE paragraph) to confirm. Sources: HO W17.
15. **"Module settings" page sits beside the admin-only Modules page** (profile settings are for everyone): confirm. Sources:
    HO s2 decisions.
16. **WittyMediaRemoteMedia**: port, disc images on a NAS, app rather than module (in its `docs/DESIGN.md`). Sources: HO s1 #8.

## Owner-only tasks

1. **Push every repo's `v0.8.0-alpha`, tag, and watch the first CI runs**; run the release checklist (RELEASE "Release
   checklist" steps 5 and 6). Nothing has run on GitHub. Includes the CI step for the facade `-check` (needs WittyModule
   `05692a3` on GitHub). Sources: HO top, RELEASE, HO s5 follow-up #18.
2. **Approve and file these issues.** Sources: DESIGN-ISSUES, RELEASE phase C.
3. **Delete old GitHub releases** (asked for originally, never done). Sources: HO "Waiting on the owner".
4. **Release WittyModule with module API 4, then publish in order** (`scripts/publish-all.sh`: WittyModule, WittyDisc 0.2.1,
   WittyDVD/WittyBluray 0.4.0 with player copies 0.3.0, WittyCD 0.2.1, WittyPlay 0.4.0, WittyCouch/WittyRigs/WittyCouchHost 0.3.0,
   WittyTuner, readers, WittyModuleCatalog (serial 22, signed on publish), then the server; WittyMediaPlayer after its modules).
   Sources: HO s7 publish order, HO s1 #3, HO s5 #7–8, HO s6 #9, HO s8 #5, HO s9 decisions, HO s11 #1.
5. **AppImage, Flatpak and Snap built on Windows in WSL** (`docs/handover/LINUX-PACKAGES.md`). Sources: HO "Waiting on the owner".
6. **Windows runs and packaging**: server `.msi`, desktop NSIS, WittyMediaPlayer MSI/D3D11, WittyReader installers, desktop
   Manage servers/From away on Windows. Sources: HO s7, HO s2 issues #6, HO s8 #6, HO s9.
7. **Real devices**: an Apple TV (HD and 4K; the 4K simulators render nothing here), iPhone/iPad, an Android TV box, a real
   controller (pad map A/B/X/Y). Sources: HO p2p #8, HO s9, HO s6, DESIGN-ISSUES.
8. **Two real routers / networks for peer to peer** (direct→relayed changes, DHT, hole punching). Sources: HO p2p #8, HO s10 #4,
   HO s1 issue 6.
9. **TMDB key** on a test server (Where to Watch on Apple and Android never seen). Sources: HO s3, HO s9 follow-up.
10. **Mac permissions for standalone emulators**: Screen & System Audio Recording and Accessibility for the program running the
    server; then PCSX2, RPCS3, Lichii through the server. Sources: HO s5 #1, #14.
11. **tvOS App Group (signing)** for Top Shelf; sideloaded builds stay empty by design. Sources: HO SDUI Apple, HO s3.
12. **Hardware and software to hand**: a Docker host (rootless/Podman) for emulators, a machine with Demucs/Spleeter/
    audio-separator, real libbluray, an encrypted DVD/Blu-ray image, a Sunshine host for WittyCouch, a Raspberry Pi with an SD
    card, a games library with emulator cores, RPCS3 system software, Python 3.10+ for `design-tokens.py --check`.
    Sources: HO "Waiting on the owner", HO s7 issues 2–3, HO s5 #4/#12, HO s5 follow-up #10, HO s1 #5–6, DESIGN-ISSUES.
13. **Housekeeping**: set the `wmc_tv`/`wmc_phone` emulators up again for the owner's server (app data cleared); stop stale test
    servers on `*:19811`/`*:19812`; delete server branch `wip/s8`; decide what to do with the unmerged `wip/*` branches listed above.
    Sources: HO s6, HO s9, HO s11, HO s8 #9, RESUME.

---

## wittywilla/WittyMediaServer (server and web)

1. **Merge or close the unmerged Design 2 web branches**
   - Repo: WittyMediaServer. Labels: design-2, not-verified, platform:web
   - `wip/design-web-finish`, `wip/design-web-gaps`, `wip/design-web-gapsQ1`, `Q2`, `Q3` are not merged into `v0.8.0-alpha`. Check
     each for work still needed (W5 gap pages, finishes) and merge or drop.
   - Where: those branches, `.worktrees/`.
   - Sources: RESUME, git state.

2. **SDUI editor: draft, publish and diff faults**
   - Labels: bug, area:sdui, area:admin, platform:web
   - `POST /sdui/validate` for a menu ignores pages that exist only in the draft; `DELETE /sdui/config/pages/<missing>?target=draft`
     answers 400; `Store.Publish` replaces the whole configuration with the draft, so a change published after the draft began is
     lost; `/config/diff?from=current&to=draft` reports both revisions as 1; the catalogue lists 16 native screens but the menu
     validator accepts 8. Publish and rollback were never run live. Editor widens the Settings column above 1281 px while a page is open.
   - Where: `pkg/sdui` store, `internal/server/api_sdui*.go`, `web/src/pages/settings/pages/`, `pages-editor.css`.
   - Sources: HO SDUI web (Editor).

3. **Pages editor preview follows the window, not the device frame**
   - Labels: bug, area:sdui, platform:web
   - Only spotlight/title-header heights follow the frame (`--pe-vh`); everything under `@media (max-width…)` and the preview hero
     are drawn for the window. Use container queries or an iframe of the frame's width.
   - Where: `web/src/pages/settings/pages/Preview.tsx`, `pages-editor.css`.
   - Sources: HO s2 #8, HO s2 issues #9, HO SDUI web.

4. **SDUI server: phone backdrops always 1920 px; tablet margins disagree with fixtures**
   - Labels: bug, area:sdui
   - Phone detail backdrops are sent 1920 px wide whatever the viewport; the specification's tablet margin (24) disagrees with
     the conformance fixtures (2×8).
   - Where: `internal/server/sdui_data.go`, `pkg/sdui` layout profiles, `pkg/sdui/conformance`.
   - Sources: HO SDUI "For every renderer".

5. **Stem sidecars are scanned as songs of their own**
   - Labels: bug, area:music
   - `01 Song.vocals.m4a` / `01 Song.instrumental.m4a` beside `01 Song.m4a` appear as tracks ("Song 1.vocals") and play in the
     queue; karaoke still finds them as parts.
   - Where: music scanner in `internal/library`, docs/LYRICS-KARAOKE.md naming.
   - Sources: HO s4 #8, note s4.

6. **"Never" / "only when an admin asks" don't block `POST …/stems/approximate`**
   - Labels: bug, area:music
   - Any signed-in account can still call the older approximate-stems route, bypassing the karaoke preparation policy.
   - Where: `internal/mediaextras/karaoke_cache.go`, karaoke routes in `internal/server`.
   - Sources: HO karaoke #6.

7. **Karaoke: real separators never run; first request waits up to 20 s; old quick versions**
   - Labels: not-verified, area:music
   - Demucs, Spleeter and audio-separator command lines and output names come from docs and ran only against stand-ins. The
     first karaoke or Settings request can wait 20 s while the server looks for Demucs through Python. Quick versions made before
     this change have no whole-song copy, so timing against the instrumental isn't guaranteed.
   - Where: `internal/separator/separator.go`, `separatorPart` in `internal/mediaextras/karaoke_cache.go`.
   - Sources: HO karaoke #5, #7, #8.

8. **Web karaoke parts: never listened to; Safari and TV remote not tried**
   - Labels: not-verified, area:music, platform:web
   - Parts play at 1.0 decoded on one clock; at other speeds or over 15 min they stream with rate nudges (30–300 ms). Both paths ran
     (206 responses) but nobody heard whether they stay together; quick-to-real swap never watched; Safari and TV remote untried.
     Instruments/Backing rows appear only with a separated stem set.
   - Where: `web/src/audio/stems.ts` (`StreamedStems`), `stemMix.ts` (`streamStep`).
   - Sources: HO karaoke #4, HO s2 #4, HO s2 issues #5, DESIGN-ISSUES (Karaoke).

9. **Make TestKaraokeAutoUpgradesFromApproximate steady under load**
   - Labels: bug, area:music
   - Timed out at 60 s twice while both heavy slots were busy; its quick version never left "preparing". Alone it passes in 2 s.
   - Where: `internal/server/api_karaoke_auto_test.go`, `media_jobs.go`.
   - Sources: HO s1 #17, HO s1 issues #9.

10. **Music player: effects menu stays open over karaoke; visualizer only thin side bars**
    - Labels: bug, design-2, area:music, platform:web
    - Visualizer → More opens the effects list; pressing Karaoke leaves it floating over the words (one Layer at a time is
      broken). With Visualizer chosen the stage shows thin bars at the edges and an empty middle; `MU_lumen` fills the stage. Speed's
      effect on the picture was never measured; Canvas video not seen (no canvas file).
    - Where: web full player / visualizer (`web/src/audio/`, W10's files).
    - Sources: QA-5 P1, P2; QA-3 M3; DESIGN-ISSUES (Karaoke); RELEASE (Visualizer Speed/Colour).

11. **Music home and album page details**
    - Labels: bug, design-2, area:music, platform:web
    - Music home says "Recently played" over an album never played (it shows the newest); the album page has no artist line under
      its title.
    - Where: music SDUI page source, `web/src/sdui/render/`.
    - Sources: QA-1 D24.

12. **Music bar sits under the pinned rail and over the video mini player**
    - Labels: bug, design-2, platform:web
    - With the rail pinned at 1440 the bar's cover, title and progress start behind the rail; a film's mini player controls are
      covered by the bar (one Layer at a time). The bar also covers the page footer (no bottom spacing).
    - Where: web shell and audio bar CSS.
    - Sources: QA-1 D11, QA-2 N6.

13. **"You might also like" opens seconds into a short film, translucent over the picture**
    - Labels: bug, design-2, platform:web
    - At about 6 s of a 20 s clip (2 s in pass 1); the card is translucent over the picture. Check the threshold for short files.
    - Where: `web/src/player/VideoPlayer.tsx` next-up logic.
    - Sources: QA-1 D12, QA-2, QA-4, QA-5 (not walked).

14. **TV-width WittyRemote: right-hand key group clipped**
    - Labels: bug, design-2, platform:web
    - At 1920x1080 `?tv=1` the transport keys on the right are partly cut at the sheet's edge.
    - Where: web remote sheet (`web/src/remote/`), W13 finishes.
    - Sources: QA-5 (M5 recheck).

15. **TV mode: no Dock or key legend on Home; hero misaligned; TV mode sticks on phones**
    - Labels: bug, design-2, platform:web
    - Home at `?tv=1` has no Dock/key legend (boards show X Look, Y Sort, LB Filter, Menu Edit layout, Start Options; server side,
      W16). Hero text starts at a different left edge from shelf titles. Visiting `/?tv=1` once keeps TV tabs even at 375 px
      until the stored flag is cleared (not rechecked). TV top menu lists every entry with no "More".
    - Where: `web/src/tvmode/nav.ts`, TV shell, server Dock data.
    - Sources: QA-1 D4, QA-3 M8, QA-4, HO SDUI web.

16. **Phone: top bar clips links at 375 px; touch targets under 44 px**
    - Labels: bug, design-2, platform:web
    - Top strip clips links at 375 px. Carousel dots and pause are 24x24, Activity 40x40, Skip to content 43 high (principle 16).
    - Where: shell CSS, `web/src/sdui/sdui.css`.
    - Sources: DESIGN-ISSUES (Phone top bar), QA-1 D27 (not rechecked).

17. **Rail look: drawer items centred; wrong current item**
    - Labels: bug, design-2, platform:web
    - The library drawer's views are centred (board `RailStates` left-aligns them); "Collections" was marked current while on Movies.
    - Where: web rail/drawer shell.
    - Sources: QA-1 D9, QA-2.

18. **Compact and Dashboard: Settings marked current on Home; Dashboard widgets empty**
    - Labels: bug, design-2, platform:web
    - The gear is filled while Home is the page; Dashboard's Clock card shows nothing under its title, counters bare, no Edit
      layout key. Not rechecked after pass 1.
    - Where: web shell, `Widget`/`WidgetGrid` renderer.
    - Sources: QA-1 D16, QA-5 polish.

19. **Looks that differ only in their top bar (finish the 14 shells)**
    - Labels: enhancement, design-2, area:themes, platform:web
    - Stage, Classic Satellite, Big picture and Marquee draw Lumen's shelves: Big picture doesn't enlarge type, Marquee's carousel is
      three centred cards, Satellite has no silver bars; Compact and Marquee Home read as lists with no hierarchy. The shells
      (cableBar, bevelBar, teleKeys, crossbar, floatingGlassBar, ornamentTabs) in CSS. Aero Sunset keeps the blue Day sky (board is
      orange-pink). Aero Night/Meadow/Sunset and every look at phone/TV width and light/dark not shot.
    - Where: web shell CSS, `themes/tokens.json` scenes.
    - Sources: QA-1 D17, QA-4, QA-5 P4 and polish.

20. **Poster cards: title on the art for Rail (plate style)**
    - Labels: bug, design-2, platform:web
    - Rail and Lumen draw titles beneath; the Rail board draws title-on-art with the accent bar.
    - Where: `web/src/sdui/render/components.tsx` card styles.
    - Sources: QA-1 D19.

21. **Cards and tiles without art**
    - Labels: bug, design-2, platform:web
    - Cards without art show an icon where old pages showed a coloured title; book and comic tiles with no cover show a gradient
      without a title; library tiles say "68 items" not "68 episodes".
    - Where: `web/src/sdui/render/components.tsx`.
    - Sources: HO SDUI web, DESIGN-ISSUES (Smaller).

22. **Settings, Appearance: sticky tabs overlap; stray controls; theme cards' names**
    - Labels: bug, design-2, platform:web
    - "On this page" tabs sit on the title when scrolled and the section list has its own scrollbar; "Crisp/Soft/Round" and "Use on
      this device / Add for everyone" show above the sticky tabs; each theme card's accessible name starts with the preview's
      dummy text (hide the preview from assistive tech).
    - Where: `web/src/pages/settings/AppearanceSettings.tsx`, settings shell CSS.
    - Sources: QA-1 D18, D23; QA-5 P7.

23. **Required looks: no link from Appearance; applied only after reload; confusing policy page**
    - Labels: bug, design-2, area:admin, platform:web
    - Appearance says "Required by … It can't be changed here" with no link to Pages & menu, Looks. A required look set in the
      wizard isn't applied until reload and Appearance names the wrong theme. The Looks page needs Save to draft then Publish (with a
      confirmation) and shows "A draft is waiting, the same as what is published".
    - Where: `web/src/pages/settings/pages/` (Looks), Appearance.
    - Sources: QA-2 N1 (withdrawn except the link, QA-4), N3, N9.

24. **Layout and name editors: other looks and phone width not checked; widget grid**
    - Labels: not-verified, design-2, platform:web
    - Layout editor seen in Lumen only; name editor and wizard in Lumen, Aero, Retro; Look cards at phone width not looked at.
      "+ Widget, + Shelf, + Banner, + Library tile" belong to the Home widget grid; hero and key legend have no server positions
      (show/hide/style only).
    - Where: `web/src/pages/ShellEditor.tsx`, `layoutModel.ts`, `components/NameEditor.tsx`.
    - Sources: HO W12 #3, #5; DESIGN-ISSUES (Layout editor).

25. **First-run wizard: module steps and small faults**
    - Labels: bug, not-verified, design-2, platform:web
    - Games, Discs and Live TV steps seen only dimmed (no catalogue). The media step's example path is `D:\Movies` on a Mac (show
      the host's form). `/setup/*` shows "no page" after setup, so walk on a fresh server.
    - Where: `web/src/pages/setup/Games.tsx`, `Discs.tsx`, `Tv.tsx`, media step.
    - Sources: HO W12 #1, #4; QA-1 D21.

26. **Two-step page: disabled Continue has low contrast**
    - Labels: bug, design-2, platform:web
    - Where: `/design/gaps/twofactor` page.
    - Sources: QA-5 (W5's pages).

27. **Item page: unknown audio, wrong source, and the file path shown to everyone**
    - Labels: bug, platform:web
    - An AAC-in-MP4 file shows "Audio: Unknown" and "Source: MOV", and the full server path of the file to anyone who opens the
      page. Check non-admins can't see the path; fix the probe.
    - Where: item page, media probe in `internal/`.
    - Sources: QA-1 D25.

28. **Activity badge turns red when a lookup finds no match**
    - Labels: bug, platform:web
    - "metadata lookup failed, not found" counts as a failure ("2 failed"); a missing match shouldn't alarm.
    - Where: `web/src/components/Activity*.tsx`, task status.
    - Sources: QA-1 D26.

29. **Module pages with no library are a bare title (empty states)**
    - Labels: bug, design-2, area:sdui
    - `/photos`, `/books`, `/page/wittyphotos`, `/page/wittyink-books` and WittyPlay Online with no games show only the title/tabs; no
      empty state or "Add a library"/"Add games". W16's `emptyHint`/`emptyAction`/`library.scan` batch was meant to fix this:
      confirm after it lands.
    - Where: module `sdui.json` pages, web renderer empty states.
    - Sources: QA-3 M2, QA-5 P5, RESUME (W16).

30. **WittyTV: library channel from short files shows no programme; More info thin**
    - Labels: bug, design-2, area:livetv, platform:web
    - A library channel of three short films shows one "We'll be right back" block 12:00 AM to 12:00 AM, "1440 min", no description,
      logo, cast, where to watch or trailers. Check `internal/livetv` schedules short files; check with real media.
    - Where: `internal/livetv`, `web/src/pages/tv/`.
    - Sources: QA-1 D14, QA-3 M6, QA-5 P6.

31. **WittyTV on the web: not checked with a real remote or a real broadcast**
    - Labels: not-verified, area:livetv, platform:web
    - TV mode with a real remote (PageUp/Down still switch TvFrame tabs; Channel Up/Down page the guide); recordings and sets with real
      programmes (library channels can't be recorded).
    - Where: `web/src/pages/tv/GuidePage.tsx`.
    - Sources: HO s6 #3, QA-1..5 "not walked".

32. **Live TV: recording conflicts and any-channel rules**
    - Labels: enhancement, area:livetv
    - Conflicts count only recordings, not viewers; any-channel rules match titles exactly (ignoring case).
    - Where: `internal/livetv/dvr.go` (`markConflicts`).
    - Sources: HO s6 #6.

33. **Remote control and watch parties: paths never exercised**
    - Labels: not-verified, platform:web
    - `navigate` from a phone remote into a TV-mode browser (unit-tested only); disc, reader, photo and game targets never driven;
      party panel in fullscreen (code-checked), music listening party, party over a forced transcode.
    - Where: `web/src/remote/tvTarget.ts`, `discTarget.ts`, `pageTargets.ts`, `DeviceRemote.tsx`, `PartyPanel.tsx`,
      `audio/partyMusic.ts`, docs/SYNCPLAY.md.
    - Sources: HO s2 #3, HO s2 #14, HO s2 issues #2–4.

34. **Module settings and override screens never walked with real data**
    - Labels: not-verified, area:admin, platform:web
    - Settings → Module settings walked only against few/no modules (walk with the worked example by keyboard and screen reader);
      install-from-file and override dialogs at phone width; held downloads (need a test catalogue).
    - Where: `web/src/pages/settings/ModuleSettings.tsx`, `pages/settings/store/InstallFromFile.tsx`, `OverrideDialog.tsx`.
    - Sources: HO s2 #1, HO s2 issues #1, HO s1 #12, HO s1 issues #5.

35. **Disc pages on the web: dialogs and refusal never opened**
    - Labels: not-verified, area:discs, platform:web
    - "Disc settings" and "Name titles and scenes" (Manage this title) only type-checked; the encrypted-disc refusal section never
      seen with an encrypted image; two later "why this title" fixes not re-seen in a browser; DVD/Blu-ray/CD playback not verified
      in Design 2.
    - Where: `web/src/components/edit/DiscDialogs.tsx`, `web/src/sdui/extras/DetailExtras.tsx`, `internal/server/disc_sources.go`.
    - Sources: HO s7 issues #1–2, HO s7 follow-up, DESIGN-ISSUES (Not verified).

36. **Player's per-disc panel should read the disc's own settings**
    - Labels: enhancement, area:discs, platform:web
    - The `discset` board (forced captions, restrictions, subtitle size, languages, angle) should read the same per-disc settings as
      the disc's page.
    - Where: `web/src/player/DvdPlayer.tsx`, `internal/server/api_dvd.go`.
    - Sources: HO s7 issues #13.

37. **libbluray second opinion never run against a real libbluray**
    - Labels: not-verified, area:discs
    - Only stand-ins. On Windows/macOS with libbluray (Linux `-tags libbluray`), turn on the setting and use Compare with libbluray.
    - Where: `internal/modules/bdremote/analyze.go`, WittyBluray `libbluray/`.
    - Sources: HO s7 known issues, HO s7 issues #3.

38. **Pages not seen in a browser under Design 2**
    - Labels: not-verified, design-2, platform:web
    - WittyIntegratedFileBrowser and games pages, TV big-screen mode, WittyRemote and TV layouts of the music boards; W5's 124 gap
      pages compared one by one with boards only for 5; the Back ladder end to end; theme import's file chooser; reduced motion.
    - Where: `/design/gaps/<key>`, `docs/design-2/images`.
    - Sources: DESIGN-ISSUES (Not verified), QA-1..5 "not walked".

39. **Accepted feature gaps: smart playlist count, download chooser, shorts and concerts, language**
    - Labels: enhancement
    - Smart playlist builder live match count; a download chooser (needs a server API); shorts and concerts pages; interface
      translation (none exists; Language dropped from the design).
    - Sources: RELEASE "Known gaps accepted", DESIGN-ISSUES (Language).

40. **Admin pages as SDUI forms**
    - Labels: enhancement, area:admin, area:sdui
    - Bulk edit, edit match, fix matches and library management have no SDUI forms (the web has its own pages, apps have none).
      See decision 1.
    - Sources: RELEASE.

41. **Design canvas: features with no or partial boards**
    - Labels: docs, design-2
    - "Drawn but incomplete": Statistics controls, notes filters, WittyTV/music/player/first-run boards in only some looks, WittyPlay
      Online only via Games row in the 17 looks. Client features with no board (profiles and screen time, connect/accounts, skip
      markers, remote/together, podcasts/radio/audiobooks/sleep timer, library tools, WittyTV extras, games pages, system states,
      Top Shelf/Watch Next). Frozen for this release; draw after.
    - Where: `docs/design-2/`.
    - Sources: FEATURE-GAPS "Drawn but incomplete", FEATURE-GAPS-CLIENTS.

42. **Logging: a terminal's SIGTERM is reported as a launchd stop**
    - Labels: bug, platform:macos
    - `pkill` logged `reason=service-stop detail=launchd`.
    - Where: `internal/logging/stop.go`, darwin service detection.
    - Sources: HO s10 #6.

43. **Logging coverage not verified on apps and macOS permission path**
    - Labels: not-verified
    - Apps' new reports checked by unit tests only; desktop path-change logging syntax-checked only; the macOS permission warning
      and Repair's emulator-permission finding appear only with the native host (steps in source); P2P path lines only for enrolled
      devices.
    - Where: `docs/handover/LOGGING-COVERAGE-0.8.0.md`, WittyPlay `sandbox/host`, `internal/diagnose/doctor_features.go`.
    - Sources: HO s10 #2–4.

44. **staticcheck findings that may be real, and deprecations**
    - Labels: bug
    - Nil-map assignment `internal/diagnose/diagnose_test.go:414` (SA5000), empty critical section `internal/logging/flight_test.go:117`,
      unused values in `modules/catalog_extras_test.go:139`, `seamcheck/stream_test.go:373`, `server/sdui_editor_test.go:108`;
      deprecated `database/sql/driver` (`internal/db/logged.go`), `strings.Title` (`internal/gamemeta/tgdb.go`). 114 findings total.
    - Sources: HO s11 second round.

45. **Watch GO-2024-3218 (go-libp2p-kad-dht)**
    - Labels: area:p2p
    - CI accepts it by name only while no fix exists; when a fixed version appears, upgrade.
    - Where: `scripts/vulncheck/main.go`, docs/P2P.md.
    - Sources: HO s1 #18, HO s1 issues #15, HO s11 #6.

46. **Read catalogue status fields directly once WittyModule is released**
    - Labels: enhancement
    - Server reads `status`/`statusNote`/`ui` from raw signed JSON beside the v0.3.0 struct.
    - Where: `internal/modules/catalog_extras.go`.
    - Sources: HO s1 #15, HO s1 issues #2.

47. **Measure the theme's cost on a Raspberry Pi**
    - Labels: not-verified, area:themes, platform:linux
    - 1.8 ms resolved theme, 0.12 ms page on an M-series core; Pi/SD card never tried (SQLite locks and per-theme stats dearer).
    - Where: `WMS_MEASURE=1 go test ./internal/server -run TestThemeCostPerPage -v`.
    - Sources: HO s1 #6, HO s1 issues #8.

48. **Switch 2 metadata ids beyond Wikidata**
    - Labels: enhancement, area:games
    - IGDB, TheGamesDB, ScreenScraper and RAWG ids are left out.
    - Where: `internal/gamemeta/platforms.go`.
    - Sources: HO s5 #13.

49. **Linux packages install only the 256 px icon; macOS packaging not run**
    - Labels: bug, area:packaging, platform:linux, platform:macos
    - The `hicolor` tree is generated in `server-package` but not installed by nfpm, Flatpak or Snap. `build-release.sh` (icns) and
      `build-pkg.sh` (installer background) never run end to end; Docker and the FreeBSD desktop job not run.
    - Sources: HO s9 known issues, HO s11 "Not run here".

50. **Retake the setup guide's pictures**
    - Labels: docs, design-2
    - `docs/FIRST-TIME-SETUP.md` pictures show the earlier style.
    - Where: `scripts/docs-images.py`.
    - Sources: HO s1 issues #13.

51. **`npm run test:keys` stalled once on `/you`**
    - Labels: bug, platform:web
    - A full run stalled with no output; a six-page run finished. Check the profile picker or a dialog.
    - Where: `web/scripts/test-keys.mjs`.
    - Sources: HO s2 issues #10.

52. **WittyPlay Online engine pieces (status not confirmed)**
    - Labels: enhancement, area:games
    - SDUI-GAPS §6 lists rollback netplay, voice chat (off for Kids), phone-as-controller pairing by QR, save-state cloud sync and a
      clips store as server/engine work (S5). The data sources and pages are merged; none of the sources says these engine parts
      were built. Confirm, then file what's missing.
    - Sources: SDUI-GAPS §6, HO s5 follow-up.

## wittywilla/WittyMediaClient (Apple, Android, desktop)

1. **Merge or close the unmerged Design 2 client branches**
   - Labels: design-2, not-verified, platform:ios, platform:tvos, platform:android, platform:androidtv
   - Apple: `wip/design-apple` (W3 hero fix, tvOS side menu), `-apple-finish` (W14), `-apple-mac-vision` (W15: Mac frame, visionOS
     ornament), `-apple-pages` (W6: Appearance, chooser, setup-required, More info), `-apple-rev8` (W19 revision 8 renderer),
     `wip/s3`. Android: `wip/design-android` (W4), `-android-media-e` (W9 remote/music/editors and the `lintSideload` crash in
     `PlayerScreen.kt`), `-android-rev8`, `wip/s4`. Gate 0c needs simulator/emulator screenshots against `docs/design-2/images`.
   - Sources: RESUME, RELEASE gates 3–4 and accepted gaps (Mac frame, visionOS ornament, Android headset frames), git state.

2. **Peer to peer on Apple: From away and iOS paths never seen**
   - Labels: not-verified, area:p2p, platform:ios, platform:tvos
   - You → From away never seen; Stop / Connect again never exercised; web view cookies from away, background audio keeping the
     tunnel, iPad; home↔away switch unit-tested only. `Wittyconnect.xcframework` isn't in simulator builds here
     (`apple/scripts/build-wittyconnect.sh`). `apple/Checks/AwayCheck.swift` not run since it gained the admin password. Hardware
     unknowns: memory kills on Apple TV HD, sockets after sleep, Keychain after re-signing.
   - Where: `apple/App/AwayViews.swift`, `AwayModel.swift`, `WebPlayerView.swift`, `PeerManager.swift`, `apple/scripts/p2p-check.sh`.
   - Sources: HO p2p #1–3, #8; HO s3 known issues; HO s1 #9, HO s1 issues #7.

3. **Join by code: approve in the apps; invitation links on iOS**
   - Labels: enhancement, area:p2p, platform:ios, platform:android
   - Approving a join-by-code is web-only (Settings → Remote Access); iOS registers no link scheme for invitations (paste only).
   - Sources: HO p2p #5, HO s3 known issues.

4. **Peer to peer on Android: TV, tablet, codes, links not tried**
   - Labels: not-verified, area:p2p, platform:android, platform:androidtv
   - TV and tablet from away, Join by code (needs DHT), invite link, Stop / Connect again, leaving the screen while away; on
     Android 6 network changes aren't heard.
   - Where: `p2p/PeerManager.kt`.
   - Sources: HO s4 #3.

5. **Desktop peer to peer across real networks**
   - Labels: not-verified, area:p2p, platform:desktop
   - `desktop/test/app/p2p-real.js` passed on one Mac, one network, DHT/relay/hole punching off, no two-factor; From away Stop /
     Connect again never used while actually away.
   - Where: `desktop/test/app/p2p-real.js`, `desktop/main.js` (`stopAway`, `startAway`, `learnP2P`), `desktop/state.js`.
   - Sources: HO s1 issues #6, HO s2 issues #7.

6. **Client CI enforces nothing until the server is published**
   - Labels: bug
   - Client CI checks out the server's default branch, which lacks the connector and `pkg/sdui/conformance`; P2P builds skip and
     conformance tests skip. Fixtures are not copied into the client repo. New fixtures (`wittydvd-disc-page-web`/`-tv`, `peek`,
     `exp-*`) need syncing.
   - Where: client `.github/workflows`, `apple/scripts/sync-sdui-fixtures.sh`, Android `-PsduiFixtures`.
   - Sources: HO p2p #4, HO SDUI "For every renderer", HO s7 issues #7, HO s6 (device run).

7. **Apple SDUI layout faults**
   - Labels: bug, area:sdui, platform:ios, platform:tvos
   - iPad: ~30 pt empty space above the first section under top tabs/sidebar; round Back half above a title's picture on the sidebar
     layout. Rail/top tabs not seen on iPhone. Apple TV: `-WMCOpen /show/<id>` lands on the profile tab; audiobook cards showed a
     placeholder (`SDUIPicture.load` or 404 from `/api/items/{id}/poster`). Colours per SDUI part come from the app's palette, not the
     catalogue's per-part map; Inter falls back to the system font.
   - Where: `SDUIPageBody`, `BackBar` in `MobileShell.swift`, `App/SDUI/SDUIKit.swift`.
   - Sources: HO s3 known issues, HO s3 9b6829a, HO SDUI Apple, note s3.

8. **Apps don't read `personal` from `/sdui/navigation`**
   - Labels: enhancement, area:sdui, platform:ios, platform:android
   - Hidden menu entries can't be listed.
   - Sources: HO SDUI "For every renderer".

9. **Apple Design 2 pages (W6) never seen on a device**
   - Labels: not-verified, design-2, platform:ios, platform:tvos
   - Appearance, chooser, setup-required overlay, WittyTV More info: no screenshots, nothing compared with `SB_lumen.jpg`. "Use on all
     my devices" writes five keys (server limit 32 per profile); Import a theme hidden on Apple TV and untried with a real ZIP;
     chooser once-per-server untried; More info untried against live `/tv/{key}/programme`. Built-in catalogue revision must be 8.
   - Where: `apple/App/AppearanceViews.swift`, `TvMoreInfoView.swift`.
   - Sources: HO W6, RELEASE gate 3.

10. **Apple: sound, artwork and Top Shelf never seen**
    - Labels: not-verified, platform:ios, platform:tvos
    - Simulator is silent (karaoke parts, stems). tvOS icon, parallax layers, Top Shelf images and sign-in backdrop never seen; iOS
      launch screen and final iPhone icon not captured.
    - Where: `apple/App/StemPlayer.swift`, `SignInKit.swift`.
    - Sources: HO karaoke #2, HO s3, HO s9 known issues.

11. **Removing servers: Apple and Android loose ends**
    - Labels: bug, platform:tvos, platform:androidtv, platform:ios, platform:android
    - Apple TV: after removing the server in use, the picker shows no focus until the first press; one flaky `forget-check.sh` run.
      Apple unit-tested only: P2P hook, logo/cookie removal, Undo restoring a Keychain sign-in; iPad, VoiceOver, controller not run.
      Android: first focus on Android TV with many servers is on "Enter an address" below the fold; TalkBack, tablet, keyboard/pad,
      removal while backgrounded and the identity warning's button not run. List still capped at eight; Android per-video view
      settings carry no server name (`ViewPrefs.kt`).
    - Where: `ConnectViews.swift`, `ConnectScreens.kt`, `ViewPrefs.kt`.
    - Sources: HO forget #4–6, #8.

12. **Desktop: removing and managing servers**
    - Labels: bug, platform:desktop, platform:windows, platform:linux
    - No Settings entry for removal (only on the server list); the server in use can't be removed without leaving it. Desktop never
      offers never-reached servers for "not reached for N days" (Apple and Android count from first run of this version: pick one
      rule). Two servers on one host share cookies; the multi-server undo announcement reads oddly. Not run on Windows, Linux,
      FreeBSD, TV mode, moved servers, "Remove the old server", P2P revoke.
    - Where: `desktop/main.js` (`rememberServer`, `showHelpMenu`), `shared/connect/connect.js` (`movedTo`, `toastNode`), `servers.js`.
    - Sources: HO forget #1–3, #7; note s4; HO s2 #7, HO s2 issues #6.

13. **Desktop: rerun `test:real` after the film-card fix**
    - Labels: not-verified, platform:desktop
    - Where: `desktop/test/app/real-server.js` (commit 6e81c47).
    - Sources: HO s2 issues #8, HO s11 #5.

14. **Desktop installers never made**
    - Labels: not-verified, area:packaging, platform:desktop, platform:windows, platform:linux, platform:macos
    - No `.dmg`, NSIS or Linux package was built, so installer pictures and disk-image background are unseen.
    - Where: `desktop/build`.
    - Sources: HO s9 known issues.

15. **Android karaoke: latency, effects, background**
    - Labels: bug, area:music, platform:android, platform:androidtv
    - Mixer within 80 ms of the master; a slider change heard ~0.4 s late; EQ and ReplayGain not applied to stems; stems stop in the
      background; tablet run underran; 24/32-bit and float parts only unit-tested; TV D-pad not tried; never heard on a real device.
    - Where: `StemMixer.run`, `ui/Karaoke.kt`.
    - Sources: HO karaoke #3, HO s4 #5.

16. **Android remote and watch parties: controller side untested; music parties missing**
    - Labels: not-verified, platform:android, platform:androidtv
    - Remote driving a real second device, cast handover, navigate keys, a party between two real apps (drift, rejoin), HLS in a
      party. Music listening parties aren't built (a notice shows); no invite-by-name screen.
    - Where: `player/Remote*.kt`, `player/WatchParty.kt`, `ui/RemoteScreens.kt`, `ui/PartyScreens.kt`.
    - Sources: HO s4 #2, HO s9 follow-up.

17. **Android Design 2: what is not built or not run**
    - Labels: enhancement, not-verified, design-2, platform:android, platform:androidtv
    - Not built: library drawer contents, finish/scene tokens, type sets, WittyRemote, layout/name editors, More info pop-up,
      music/karaoke/visualizer restyle, ~100 gap pages, WittyPlay Online, theme tokens beyond colour and a profile's own colours,
      per-profile density setting, headset frames. Not run: new frames and components on the AVD; WittyTV guide Dock and player
      OK/Left/Right; per-theme WittyTV identity. Nav spatial weights unused; key help on a phone only after a key; PageUp/Down not mapped
      on SDUI pages; tabs select on OK not focus; episode header crops a 16:9 still into a poster; the router fell back to the web for
      `/person/{person}` (`Nav.loadSdui`, `SduiRouter`); home-screen channel never seen.
    - Where: `ui/ExperienceFrames.kt`, `ui/SduiExperience.kt`, `tv/TvGuide.kt`, `tv/TvGuideDock.kt`, `tv/TvChannel.kt`, `SduiParts.kt`.
    - Sources: HO W4 #1–6, HO W4 follow-ups, HO SDUI Android, HO s4 #11, RELEASE.

18. **Android live TV details**
    - Labels: bug, area:livetv, platform:androidtv
    - Down from the set row in Add-to-set lands on Cancel before New set; page size 25 and renaming a set not seen; the guide bar's
      last control sits half off the edge until focused (Apple TV too); no Release build on a real box.
    - Where: Android `tv/` guide, `apple/App/TvGuide.swift`.
    - Sources: HO s6 device run, scrolling section.

19. **Native phone and tablet TV guide**
    - Labels: enhancement, area:livetv, platform:ios, platform:android
    - Phones and tablets show the server's web guide in a web view; page size inside the phone apps not seen.
    - Where: `Shell.swift`, `ui/App.kt`.
    - Sources: HO s6 #2, device run.

20. **The guide should follow a look's WittyTV suggestion in the apps**
    - Labels: enhancement, area:livetv, design-2, platform:ios, platform:android
    - The web follows `tvTheme` (QA D13 fixed); the apps follow only a theme's `--tv-theme`.
    - Where: the apps' `TvLook`.
    - Sources: HO s1 #20, HO s1 issues #10.

21. **Offer module overrides in the native apps**
    - Labels: enhancement, area:admin, platform:ios, platform:android
    - Only the web has the override dialog, install from a file and the audit; apps must not install past a rule without it.
    - Where: server docs/MODULE-STORE.md "Installing past a rule", web `src/pages/settings/store/`.
    - Sources: HO s1 #14, HO s1 issues #4.

22. **Apps: disc ranking, Where to Watch and module settings never seen with data**
    - Labels: not-verified, area:discs, platform:ios, platform:android
    - Disc ranking on a real disc (no disc module on test servers), Where to Watch (no TMDB key), You → Modules with a real module's
      settings.
    - Sources: HO s3, HO s9 follow-up, HO s4 #7.

23. **Build checks not run: Android lint/assemble, tvOS, visionOS**
    - Labels: not-verified, platform:android, platform:tvos
    - After session 10's logging, `lintSideload assembleSideload`, tvOS and visionOS builds weren't run; visionOS fix (90f8f49) not rebuilt
      in the combined pass.
    - Sources: HO s10 #2, HO s11 #3.

24. **First-connect "Choose how it looks" showed no looks against an older test server**
   - Labels: not-verified, design-2, platform:ios, platform:tvos
   - Against a server at catalogue revision 5 the chooser listed no looks. Probably expected (that server has no experiences);
     recheck against a 0.8.0-alpha server.
   - Where: `apple/App/` first-connect chooser (W6's area).
   - Sources: HO s3 issue candidates #5.

25. **Apple: read card styles from the server's catalogue rather than the built-in copy**
   - Labels: enhancement, area:sdui, platform:ios, platform:tvos
   - WittyKit's `SDUICatalog.swift` carries a built-in copy of the card styles; reading them from `GET /api/sdui/catalog` would
     stop the copy falling behind the server.
   - Where: `apple/WittyKit/Sources/WittyKit/SDUI/SDUICatalog.swift`, server `GET /api/sdui/catalog`.
   - Sources: HO s3 issue candidates #6 (raised by S5).

## wittywilla/WittyPlay

1. **WittyPlay Online with real data, controllers and all screens**
   - Labels: not-verified, design-2, area:games, platform:web
   - Recent, Classics grid, room panel (Start game, Invite, Leave), Memories and open rooms seen only empty or in `/design/play`.
     Controllers and the quick menu as a layer (seen as a page), pad map on a real controller, `wittyplay-kids`, `-room`, `-keep` not
     opened or compared with `docs/design-2/images/play` and `gaps/GB2_7.jpg`; phone and TV widths; privacy form Cancel missing on a
     phone (check the fixture sends `cancelLabel`).
   - Where: WittyPlay `sdui.json`, server `web/src/sdui/render/components.tsx`.
   - Sources: DESIGN-ISSUES (Web), QA-5 P5.

2. **Standalone emulators: never played through the server; per-OS settings unverified**
   - Labels: not-verified, area:games, platform:macos, platform:windows, platform:linux
   - Needs the Mac permissions; Windows path (job object, ddagrab, WASAPI, SendInput) compiled only. Command lines and config folders
     per OS come from docs (PPSSPP/Flycast/RPCS3/Cemu on a Mac, Windows `portable`). Ryujinx untried (full `Config.json` "version": 70,
     its Mac key name for Insert). RPCS3 needs Sony's system software from its own menu. Follow STANDALONE-EMULATORS.md test plans.
   - Where: `sandbox/host/platform_darwin.go`, `platform_windows.go`, `emulators/manifest.json`, `emulators/manual.go`, `emulators/inputmap`.
   - Sources: HO s5 #1, #3, #4, #11, #14.

3. **Dolphin on a Mac: keys may not reach it; libretro core stall on homebrew**
   - Labels: bug, area:games, platform:macos
   - Dolphin's Quartz backend reads `CGEventSourceKeyState`, which may not see `CGEventPostToPid` keys. The libretro Dolphin core
     stalls after frame one on Swiss homebrew (purego callbacks from a non-Go thread); real discs play.
   - Where: `emulators/inputmap`, `sandbox/host/platform_darwin.go`, `libretro/libretro.go`, `docs/PLATFORMS-AND-PLAN.md`.
   - Sources: HO s5 #2, #9, follow-up.

4. **Wii speed and crackle**
   - Labels: bug, area:games
   - 55–60 fps; with fast memory off, sound crackles where it dips.
   - Sources: HO s5 #16.

5. **Windows sound is the whole machine's**
   - Labels: enhancement, area:games, platform:windows
   - WASAPI loopback on the default output, not just the emulator.
   - Sources: HO s5 #6.

6. **Standalone emulators in Docker never tried**
   - Labels: not-verified, area:games
   - Needs Docker's socket (rootless/Podman) and `docker.hostPaths`.
   - Where: `sandbox/docker/backend.go` (`HostPath`), `rpc/docker.go`.
   - Sources: HO s5 #12.

7. **staticcheck deprecations**
   - Labels: bug
   - `httputil.ReverseProxy.Director` in `sandbox/docker/kasmproxy.go`; pion `SetNAT1To1IPs` in `stream/viewer.go`; 67 findings.
   - Sources: HO s11 second round.

## wittywilla/WittyModule

1. **After publishing: check every module's CI and release run with WittyModule 0.4.0**
   - Labels: not-verified, area:packaging
   - Done before the release (2026-11-12, S13): WittyModule is 0.4.0 (0.3.0 was published with API 3), and every module's and the
     catalogue's workflows and `refresh.sh` run `wittymodule@v0.4.0`; `publish-all.sh` raises go.mod pins and the WittyInk and
     WittyReader npm locks while publishing. Not seen: the module workflows' `check`, `needs-bump` and `go mod tidy -diff` steps
     running on GitHub against the published 0.4.0, and WittyModule's own `needs-bump`, which was reported failing. Watch the first
     runs; `kit/module-kit.sh --tool v0.4.0` regenerates a module's workflows if one has drifted.
   - Where: each module's `.github/workflows/`, WittyModule `kit/module-kit.sh`, `kit/workflow.py`; plan: `RELEASE-0.8.0-PLAN.md` G1, G4.
   - Sources: HO s1 #3, HO s1 issues #1, HO s5 #7, HO s6 #9, HO s8 #5, HO s10 #1, HO s11 #1, HO s7 issues #8; S13.

2. **Release the player copies (`wittydvdplayer`, `wittyblurayplayer`) so WittyMediaPlayer can update them**
   - Labels: enhancement, area:packaging, area:discs
   - WittyMediaPlayer's updater (`engine/internal/modupdate`) looks for a GitHub release tagged `wittydvdplayer-v<version>`
     (and `wittyblurayplayer-v…`) carrying that copy's signed SHA256SUMS, `.sig`, manifest and per-platform archives named by the
     copy's own id. Nothing makes those releases: the module release runs on `v*` tags and only the Go folder tags
     `wittydvdplayer/vX` are made. "Look for updates" in the player finds nothing; the built-in copies keep working. Fix in the kit: a
     second generated workflow that runs `wittymodule pack` in the subfolder on a `wittydvdplayer-v*` tag and signs it like the root
     module (or the root release packs the copy too), then `publish-all.sh` tags it.
   - Where: WittyModule `kit/module-kit.sh`, `kit/workflow.py`; WittyDVD and WittyBluray `.github/workflows/release.yml`;
     WittyMediaPlayer `engine/internal/modupdate`; server `scripts/publish-all.sh` (`player_copy`).
   - Sources: S7 HANDOVER issue candidate 14; plan G5.

## wittywilla/WittyModuleCatalog

1. **Add the API 4 module versions and raise the tool pins** (after WittyModule's release; see WittyModule #1)
   - Labels: area:packaging
   - Where: `ci.yml`, `publish.yml`, `refresh.sh`, README "Module API 4".
   - Sources: HO s1 issues #1, HO s7 issues #8.

## wittywilla/WittyTuner

1. **Catch-up and Xtream never tried against real providers; UTC offsets**
   - Labels: not-verified, area:livetv
   - Only `shift` tried, against a static server; `default`, `append`, `flussonic`, `xc` are unit-tested URL builders. Xtream tried
     against a stand-in; timeshift times are UTC, so a panel on local time is off by its offset.
   - Where: `catchup.go`, `xtream_test.go`.
   - Sources: HO s6 #4, #5.

2. **Tuner features not built: shared tuner, ATSC huffman, DiSEqC, BDA**
   - Labels: enhancement, area:livetv
   - A viewer and a recording sharing one tuner; ATSC huffman guide text; DiSEqC; BDA.
   - Sources: HO s6 #10.

## wittywilla/WittyDVD

1. **Labels for a DVD's other versions**
   - Labels: bug, area:discs
   - A second title sharing most of the film's cells was called "Shorter cut"; unknown whether it is a cut or the film without credits.
   - Where: `episodes.go` (Versions).
   - Sources: HO s7 issues #12.

2. **Stream files with unrecorded parts (also WittyBluray)**
   - Labels: enhancement, area:discs
   - WittyDisc 0.2.1 marks unrecorded parts (Offset -1); the engines refuse such a file. They could stream those parts as zeros.
   - Where: WittyDVD `open.go` (`VOBExtents`), WittyBluray `open.go` (`imageSource.Extents`).
   - Sources: HO s7 issues #9.

## wittywilla/WittyDisc

1. **One shape for the disc guide**
   - Labels: enhancement, area:discs
   - `wittydvdplayer` and `wittyblurayplayer` each define the guide WittyMediaPlayer reads; it belongs in WittyDisc's `session` package.
   - Sources: HO s7 issues #11.

2. **A small-input fuzz target**
   - Labels: enhancement, area:discs
   - `FuzzOpen` seeds are whole 0.6–1 MB images (~18 runs in 30 s); feed single descriptors and file entries.
   - Where: `isofs/isofs_test.go`.
   - Sources: HO s7 issues #10.

## wittywilla/WittyMediaPlayer

1. **WittyMediaPlayer on SDUI and themes (W18) never built**
   - Labels: design-2, not-verified, area:sdui, area:themes, platform:desktop
   - Branch `wip/design-player` unmerged and never built; accepted gap "WittyMediaPlayer pieces not done by the cut".
   - Sources: RESUME, RELEASE.

2. **Build and run the Linux AppImage; add VA-API**
   - Labels: not-verified, area:packaging, platform:linux
   - Never built on Linux (only Go helpers cross-built); run `scripts/setup-toolchain-linux.sh`, `scripts/build-linux.sh` on x86_64
     and aarch64, start on X11 and Wayland. Linux FFmpeg lacks VA-API (software decoding).
   - Where: `scripts/build-linux.sh`, `packaging/linux/`.
   - Sources: HO s7 follow-up, HO s7 issues #4, #6.

3. **Build and test the Windows package**
   - Labels: not-verified, area:packaging, platform:windows
   - MSI, windeployqt, `check-package.sh`, `check-msi.ps1`, D3D11 not run after this round. Disc menu tested offscreen only.
   - Where: `scripts/build-windows.sh`, `packaging/windows/`.
   - Sources: HO s7 known issues, HO s7 issues #5.

4. **Engine `go vet` fails: `wittydvdplayer.MethodGuide` undefined**
   - Labels: bug, area:discs
   - Where: `internal/disc/open.go:176` (may clear when the pins move to wittydvdplayer 0.3.0).
   - Sources: HO s9 known issues.

## wittywilla/WittyReader

1. **Desktop wrapper and installers not run**
   - Labels: not-verified, area:readers, area:packaging, platform:desktop, platform:windows, platform:linux
   - `npm i` in `desktop/`, `scripts/test/app-smoke.mjs` (keys `setBookConsumer`, `setBackground`), an EPUB and PDF through the wrapper;
     Windows and Linux installers (`scripts/test/installers-*.sh`); a second launch with two files sometimes needs longer.
   - Sources: HO W17, HO s8 #6, RELEASE (installers).

2. **Reader polish: PDFs, notes links, covers, server pages**
   - Labels: bug, area:readers
   - A PDF was not opened; pairing QR not re-looked at after the model fix; imported themes' scene layers not drawn; notes link to the
     book page, not the place; shelf has no covers; FilterBar says "titles" on notes; server pages can't play media and native links do
     nothing; README/wiki text not rewritten; remote provider polish.
   - Where: `src/pages/remote.ts`, `src/pages/connection.ts`, `src/web/`.
   - Sources: HO W17, RELEASE.

3. **Connect to a server: per-server content security policy**
   - Labels: enhancement, area:readers
   - Owner decided WittyReader may connect only to servers the person adds (per-origin CSP); the built app's `connect-src 'self'`
     blocks other addresses, so the Connect row is hidden. Confirm the W17 per-server CSP work landed and the row is back.
   - Where: `vite.config.ts`, `#/connect`.
   - Sources: HO W17, RESUME (owner decisions).

## wittywilla/WittyEbook

1. **A main EPUB chapter that fails to load says nothing**
   - Labels: bug, area:readers
   - Footnotes, search, PDFs and comic pages report; the main chapter frame doesn't.
   - Where: `EpubReader.tsx`.
   - Sources: HO s10 #5.

## wittywilla/WittyPhotos

1. **Photos page hierarchy: Timeline, Albums, Map with covers**
   - Labels: enhancement, design-2
   - `/photos` is a shelf of recent photos per library (under Cable, rows of "PIC3 2026"); the stock `photos` page has the same bare
     list. Needs a designed entry per the `photoalbums`, `photomap`, `photoview` boards.
   - Where: WittyPhotos `sdui.json`, server `pkg/sdui/stock.go`.
   - Sources: DESIGN-ISSUES (Photos), QA-5 P8.

## wittywilla/WittyStats

1. **Statistics page needs design polish**
   - Labels: enhancement, design-2
   - Dense cards of "Not available here" on a Mac.
   - Sources: DESIGN-ISSUES (Plain pages), QA-5 polish.

## wittywilla/WittyIntegratedFileBrowser

1. **No pages or navigation in `sdui.json`; long drive names overflow in Aero**
   - Labels: bug, design-2, area:sdui
   - `sdui.json` declares no pages or navigation (lists no UI in `uiAdds`); long drive names overflow the folder browser tile in
     Aero (module CSS).
   - Sources: QA-3 M1, QA-5, DESIGN-ISSUES (Smaller).

## wittywilla/WittyCouch

1. **No game stream taken through WittyCouch**
   - Labels: not-verified, area:games
   - WittyCouchHost builds and reports ready; a stream needs a Sunshine host.
   - Sources: HO s5 follow-up #10.

## wittywilla/WittyMediaServer.com

1. **Retake the website's pictures under Design 2**
   - Labels: docs, design-2
   - Home, movies, TV shows, show page, music, books, photos, phone and client, Settings → Appearance and module store show pre-SDUI
     pages. Publish the site's copy only with the release.
   - Where: `content/shots.json`, `images/shots`.
   - Sources: HO s1 #7, HO s1 issues #12.

## wittywilla/WittyMediaRemoteMedia

1. **A file-system seam in the server**
   - Labels: enhancement
   - Stays a design until the server reads media through a seam (server change too). See owner decision 16.
   - Sources: HO s1 #8, HO s1 issues #14.

---

Counts: WittyMediaServer 52, WittyMediaClient 23, WittyPlay 7, WittyModule 1, WittyModuleCatalog 1, WittyTuner 2, WittyDVD 2,
WittyDisc 2, WittyMediaPlayer 4, WittyReader 3, WittyEbook 1, WittyPhotos 1, WittyStats 1, WittyIntegratedFileBrowser 1,
WittyCouch 1, WittyMediaServer.com 1, WittyMediaRemoteMedia 1. Total 104 issues; 16 decisions; 13 owner-only tasks.
