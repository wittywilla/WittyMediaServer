# Design 2: where we stopped (resume note)

Written by the design lead when the owner put the Mac to sleep. To resume: open the design-lead session ("SDUI
infrastructure revamp"), say "resume", and the lead re-reads this file and re-wakes the agents.

## State
- Phase A of `DESIGN-2-RELEASE.md` (finish and freeze) is in progress. Freeze in force: no new catalogue revision
  (8 is final), no new screens/looks/themes/routes/features. Gates: parity with the previous release, peer to peer
  vital, apps look and work as intended (screenshots).
- Merged to `v0.8.0-alpha`: server (experiences, 15 themes, setup gate, catalogue 8, server pages, WittyPlay Online
  data), web app (editors, wizard, player bar, music, SpecPage, layout thumbnails), Apple Connect with peer to peer
  (S3), Android revision 8 renderer (W20), WittyReader and module repos, QA-1..5, the design canvas and images
  (`docs/design-2/`), the release cut, the issue candidates list.
- NOT merged (in worktrees, committed or WIP; resume them): W16 empty-state batch (`emptyHint`, `emptyAction`,
  `library.scan`, branch wip/design-sdui-pages, verification job queued); W7 last kit sync (after W16); W3 hero fix
  and tvOS side menu (wip/design-apple); W6 Appearance/chooser/More info builds; W14 finish/editors (wip/design-apple-finish);
  W15 Mac/visionOS (wip/design-apple-mac-vision); W19 Apple revision 8 renderer (wip/design-apple-rev8); W8 Apple remote and
  music (wip/design-apple-media); W9 Android remote/music/editors and the lintSideload crash in PlayerScreen.kt
  (wip/design-android-media); W4 Android frames/finishes (wip/design-android); S3 and S4 peer to peer Connect and the
  p2p native libraries (wip/s3, wip/s4); W17 WittyReader (per-server CSP, desktop wrapper; wip/design-reader); W18
  WittyMediaPlayer SDUI and themes (wip/design-player, never built yet); S5/S8 module work.
- Auditors running or queued: W21 web/server parity, W22 Apple parity, W23 Android parity (reports in
  docs/plans/DESIGN-2-PARITY-*.md when written); W11 release-candidate pass (DESIGN-2-RC-1.md).
- Release engineering: S13 (own session) writes `docs/plans/RELEASE-0.8.0-PLAN.md`, RC builds and the HANDOVER top section.
  S11 runs the full CI re-run. Nothing is pushed or filed: the owner does that (GitHub issues from
  `DESIGN-2-ISSUES.md` after approval).

## Rules to remember
- Slots: light `/tmp/witty-light-1..3`, heavy `/tmp/witty-heavy-1` (Android and general) and `-2` (Apple), 25 minutes
  max, never both, one simulator/emulator at a time; merge lock `/tmp/witty-merge.lock`; the Mac is memory-starved
  (swap near 16 GB): fewer simultaneous builds.
- Owner decisions: WittyReader may connect only to servers the person adds (per-origin CSP); disc modules need no
  SDUI; Settings stays hand-built.

## Resume order
1. Check `/tmp/witty-*` for stale slots and locks; `git worktree list`; each agent's last report.
2. Merge W16's batch, then W7's kit sync; resume W3, W6, W8, W14, W15, W19 (Apple, one heavy-2 session each) and W4, W9, W20
   (Android, heavy-1).
3. Read the parity reports; route missing/regressed core features to owners.
4. Release candidate pass (W11, S13), then the owner's issues and publish steps.
