# Design 2: lean implementation and deferred work

Written 2026-11-12 on the owner's instruction: build the skipped features with the least effort that works, run ONE big
test, fix from it, then hand over for Windows (Docker and Linux). Everything not built lean stays here to finish later.
Revision 8 and the freeze still hold. No new catalogue revision, looks or themes. Nothing is pushed by agents.

## Rules for this pass
- Build the smallest thing that works. Reuse generic components and the `gp` page pattern; no bespoke screens.
- No per-feature test runs. Build, then ONE combined test per platform, then fix only what breaks.
- Anything not finished lands in the list below instead of continuing.

## Lean pass (do now)
| Area | Build lean | Owner |
|---|---|---|
| Android | Merge wip/design-android; `bevel` and `crt` finish tokens as simple colour/overlay; rail drawer lists libraries; WittyRemote as a row/button sheet; layout and name editors via W9's branch if it merges cleanly, else defer | S4 |
| Apple | Merge wip/design-apple; frames follow the look's nav style; finish tokens; theme import using the existing file importer; More info on tvOS | W3, W14 |
| Admin pages | Bulk edit, edit match, fix matches, library management as SDUI forms using the Form component, calling the existing admin routes | coordinator |
| Small pages | Shorts, concerts, smart-list match count, download chooser: generic list pages from existing routes; skip any that need a new server API | coordinator |
| Both | Join-by-code on Apple TV and Android TV (address book is on by default, server 79b2a4c7) | W3, S4 |

## One big test (after the lean pass)
Fresh scratch server, first run in a browser, connect from iPhone, Apple TV, Android phone, Android TV; choose a look and
a theme; play a film; open music, WittyTV, a module page; join by code from each TV. Record the 14-look screenshots per
platform. Fix what breaks the path; nothing else.

## Deferred (finish when there is time)
- Language (interface translation); visualizer Speed/Colour tuning; karaoke Mix as rows.
- Android: headset frames, scene `image` layers, per-profile density, administrator policy UI, WittyPlay Online views.
- Apple: Mac frame, visionOS ornament, iPad and Mac visual checks, admin policy chooser relaunch investigation.
- Standalone: WittyReader remote provider polish, WittyMediaPlayer remaining pieces, installers.
- Parity audits W21-W23 and W11 (paused until after the release).
- Open items in DESIGN-2-QA-*.md and DESIGN-2-ISSUES.md; owner files GitHub issues when ready.
- Known bugs: "Trust this server" skips the setup-required check; sessions share one simulator and leak saved servers.
- Games (S5): Mac standalone emulators (PCSX2, RPCS3, Lichii) through the server need the Screen Recording and Accessibility permissions; never run end to end.
- Games (S5): standalone emulators untested on Windows, Linux and Docker; Ryujinx never run; Lichii never through the server.
- Games (S5): Wii on a Mac runs at 55-60 fps with crackle on dips; the Dolphin core stalls with Swiss homebrew.
- Games (S5): WittyPlay Online's Your room and Friends (List) have no empty-section button: List lacks emptyAction in revision 8.
- Games (S5): no netplay between servers, clips or voice; friends' cards have no avatar picture; Switch 2 metadata is Wikidata only.
- Games (S5): WittyPlay 0.4.0, WittyCouch 0.3.0, WittyRigs 0.3.0, WittyCouchHost 0.3.0 not tagged or published; WittyCouch never took a real Sunshine stream.

## Handover to Windows (Docker, Linux)
The agents say when the big test is done and the alpha heads are merged. Then: Docker image build, Linux packages
(.deb, .rpm, AppImage, Flatpak, snap) and the Linux client path are checked on the Windows machine. See
RELEASE-0.8.0-PLAN.md for the owner-only tasks.

## From W14 (Apple finishes, scenes, type sets, editors), 2026-11-12
Merged at client 90e1273 (tip unbuilt after three small alpha merges; the big test builds it).
- Never seen on screen: scene layers behind pages, the layout and name editors, the tvOS bar scrim, every look except Aero.
- The iPhone tour stopped at "update needed": the test server was older than the app's revision 8 (not a bug; build the server fresh for the big test).
- Opaque page backgrounds (connect, chooser, sheets) hide the scene.
- "Fill" focus style (cable, retro) draws the ring instead; Big picture's 1.25x type and density not applied.
- Unchecked: whether card sets (list, tile, block) reach the SDUI renderer; 15 themes on every look beyond Aero.
- Layout editor: game-controller moves on iOS, preview frames for other form factors, "For everyone" for page sections.
- Give W6's LayoutThumb the finish (`.finished(...)`); other agents' files still use `.system(size:`.
