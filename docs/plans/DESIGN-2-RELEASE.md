# Design 2: the release cut

Written 2026-11-12 by the design lead on the owner's instruction: the priority is to get themes, UI and SDUI
implemented, then work toward a release with working client apps. Be as feature complete as the end goal allows, but
do not work forever: the first release is a starting point. Everything past the cut becomes a GitHub issue and is
worked from there.

## The freeze (in force now)

- No new catalogue revision. Revision 8 is final for this release. Fixes to it are allowed; new components, fields and
  commands are not.
- No new canvas screens, looks or themes. The 17 look boards, 15 themes and the 128 gap screens are the design.
- No new modules, routes or features. Only: finishing what is in flight, fixing bugs, and verifying.
- Every agent merges what works into `v0.8.0-alpha` under the lock, then stops adding. Unfinished work that is not
  needed for the gates below is written up as an issue candidate (see "Issue candidates") instead of continued.

## The release gates (must work, seen running)

1. **Server:** first-run gate (`setup_required`) and the 14-step browser setup; 14 experiences with per-device choice and
   admin policy; 15 themes with import; server name colours and mark; catalogue revision 8 and the server pages;
   conformance fixtures pass; `go test ./...` passes.
2. **Web app:** every look wears every theme; setup wizard; Settings Appearance with layout previews; layout and name
   editors; rail and drawer; player with WittyRemote and the control bar; WittyTV guide and More info; music with
   visualizers and karaoke; module pages; keyboard and controller reach.
3. **Apple (iPhone, iPad, Apple TV):** builds, runs, connects (including peer to peer), first-connect chooser with layout
   previews, Settings Appearance, the SDUI renderer at revision 8 (focus-grow, regions, layers, forms, QR),
   WittyTV More info on tvOS, play a video.
4. **Android (phone, Android TV):** the same list.
5. **Standalone apps (WittyReader, WittyMediaPlayer):** use SDUI and wear the themes where already built; the rest are
   issues. Not a blocker for the server and client release.
6. **Docs and housekeeping:** CHANGELOG, FIRST-TIME-SETUP, THEMES, SDUI docs current; licence headers; no secrets; the
   design README and canvas in `docs/design-2/`.

## Known gaps accepted for the first release (issue candidates)

Each becomes a GitHub issue after the owner says to file them:

- Admin pages: bulk edit, edit match, fix matches and library management as SDUI forms (the web has its own pages).
- Smart playlist builder live match count; download chooser (needs a server API); shorts and concerts pages.
- Language (no interface translation); Visualizer Speed/Colour measured visually; karaoke Mix page as rows.
- Standalone apps: remote provider polish, WittyMediaPlayer pieces not done by the cut, installers.
- Apple: Mac frame and visionOS ornament if not finished; Android: headset frames.
- Every item in `DESIGN-2-QA-*.md` marked open and not a gate.

## Phases

- **A. Finish and freeze (now):** merge what is in flight; fix only gate bugs; each agent posts a final report with its
  "Known issues / not verified" list written as issue candidates.
- **B. Release candidate:** build the server, web, iOS, tvOS, Android and Android TV from the `v0.8.0-alpha` heads;
  run the end-to-end path (fresh server, first-run in a browser, connect from each app, choose look and theme, play,
  music, WittyTV, peer to peer); fix what breaks the path; nothing else.
- **C. Issues:** the owner approves; the open items are filed on GitHub and worked from there.

Nothing is pushed by agents. The owner pushes.
