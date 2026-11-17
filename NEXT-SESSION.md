# Next session: plans after 0.8.0-alpha

**On the Mac:** read `MAC-FOLLOW-UPS.md` first. It lists what the Windows and Linux pass (2026-11-15) left for a Mac.

Written 2026-11-15, when the 0.8.0-alpha work was finished on macOS. Read this first, then
`HANDOVER-0.8.0.md` (what each area built), `docs/plans/MODULAR-MODULES.md` (the module design) and
`ORCHESTRATION.md` in the parent folder, if it is there (how parallel sessions share work).

## Where things stand

- **Every repository is pushed.** All 25 repositories are on the branch `v0.8.0-alpha`, with no
  local-only commits. WittyPlay and WittyModuleCatalog also have `main` equal to `v0.8.0-alpha`.
- **The server is not built or released.** 0.8.0-alpha is pushed (CHANGELOG still says
  `## Unreleased`), but no tag has been made and nothing has been built. Build and release it only
  when the owner says to.
- **Modules are done: Module API 5 (M0–M9).** Each module:
  - has its own SQLite database, with no import from the server's;
  - is its own store update;
  - can be uninstalled while keeping its data.
  MinAPI is 5, and all the API 3 and 4 code is gone.
- **Released, and in the catalogue:** WittyModule 0.5.4, and all 17 API 5 modules, including
  WittyPlay 0.7.2 and WittyDisc 0.3.2. The catalogue is at serial 56.
- **Tag rule:** in all 22 repositories that release, a `v*` tag can't be moved or deleted.
  - Only each repository's **Release** workflow (`tag.yml`, run by hand) can make one.
  - It refuses to unless the version files agree and every check on the commit passed.
  - Release with `gh workflow run tag.yml -R wittywilla/<Repo> -f version=<x.y.z> -f branch=main`.
  - Then approve the `release` environment on the run it starts.
  - Then run the catalogue's `refresh.yml` and approve its `publish` run.
- **Device passes, on the owner's real media:** passed on the Android phone, tablet and TV, and on
  the Apple TV.
  - Android: accessibility checks.
  - Apple TV: accessibility audit, and backing out of a game with Menu.

## Next: Windows and Linux

0.8.0-alpha was built and tested only on macOS. Do these first, on the Windows PC:

1. **Build and run on Windows.**
   - Clone every repository on `v0.8.0-alpha`, in the same layout under one folder.
   - Build `wms` with Go 1.27, then the web client (`npm ci && npm run build` in `web/`).
   - Look for anything that assumes macOS:
     - path separators;
     - `os/exec` shell calls;
     - file locking on SQLite while a module holds its own database;
     - case-insensitive paths in the scanner;
     - drive letters and UNC paths (`\\NAS\share`) in library folders;
     - long paths over 260 characters.
2. **Modules on Windows.**
   - Install and update modules from the live catalogue: each one's `windows-amd64` and
     `windows-arm64` archives.
   - Uninstall with "keep data", then install again.
   - Check that a running module's `.exe` can be replaced on update. Windows locks running files,
     so the host must stop the module first.
   - WittyPlay: cores, controllers (XInput) and Refresh game details.
   - WittyRigs: PC game streaming. **Never tested so far**, because it needs a real PC. Windows is
     the first chance.
3. **Linux** (a VM or WSL2 counts for a first pass; real hardware later):
   - Build, run as a systemd service, and check the module install and update paths.
   - Check file permissions on the data folder, and hardware decoding where there is a GPU.
4. **Windows and Linux desktop client** (WittyMediaClient, S2's area): build it and check it draws
   the shared UI exactly as on the Mac. Every platform must look the same; no per-platform variants.
5. **Installers and release builds:**
   - Check that the server's release workflow builds and signs the Windows and Linux artifacts. The
     server is signed with `release.key`; modules and the catalogue are signed with `module.key`.
   - Check the first-run setup and the tray/service on each OS.
6. **Logs and the diagnoser on Windows and Linux.** Bug reports depend on them: make sure the log
   paths and the diagnoser bundle work there too.

## Not tested yet (from 0.8.0-alpha)

- **iPhone pass:**
  - buttons on friend rows (Invite, Accept, Join);
  - `?invite=` and `?join=` seats;
  - battery chips on the WittyPlay pages.
- **Anything that needs a second account:** friend invites, and joining a game end to end.
- **Anything that needs a real controller:** controller battery chips.
- **On Apple:** Refresh game details and the console-photos switch, because the test account wasn't
  an admin.
- **Screen-reader voice output** on every platform (the audits passed; the actual speech wasn't
  heard).
- **The TV scan layer** opening on "Scan for changes".

## Small follow-ups

- The module kit's templates don't fetch WittyModule before `build:bundle`. The release workflows
  were patched one by one; fix it in `kit/` in WittyModule and generate them again.
- `bump` leaves a stray `-E` file on macOS. Check the fix in WittyModule 2d52af2 covers every script.
- Games libraries can't be set read-only.
- Add the owner as a required reviewer on WittyDisc's `release` environment, if the tag-rule rollout
  didn't already.
- CI on WittyPlay `main` failed on an executable bit until 30098f6. Check the other repositories'
  `main` CI is green.
- Live TV's `TestRecordingRawStream` fails in CI: a second recording of the same airing is
  accepted once the first has finished (`internal/livetv/livetv_test.go:207`). Found on #389;
  fix the cause, not the assertion. #431.

## Rules that still apply

- **Test media:** test only with the owner's real media on `/Volumes/STUFF/ServerMedia` (on Windows,
  wherever the owner puts it).
  - Read-only, and set `saveMetadata=0` before adding a library.
  - Never put anything from it in a repository, docs, the website or release images.
  - Generated media is only for pictures in docs and on the website.
- **Owner's server:** never use port 9797 or the owner's installed server. Clear an emulator's app
  data before every pass.
- **Signing keys:** they live only in each repository's GitHub `release` environment secret
  `WMS_SIGNING_KEY`. Never print, copy or handle key contents; the owner adds keys.
- **Owner approval:** releases, pushes and the server build need the owner's explicit OK in the
  chat.
- **Early alpha:**
  - no backwards compatibility;
  - no support for older module versions;
  - delete legacy code, and update its users to the new way.
- **Licence and commits:** code is AGPL/GPL, with SPDX headers. Commits end with the Co-Authored-By
  trailer.
