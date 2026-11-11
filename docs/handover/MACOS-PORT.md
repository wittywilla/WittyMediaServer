# Hand-over: the macOS port

For the next Claude chat, on the owner's MacBook Pro, starting with nothing but a clone. Written 2026-11-04, at the start of 0.7.0-alpha, for wittywilla/WittyMediaServer#85.

The owner, 2026-11-04: "After we're done with this, we're fully porting to macos with my macbook pro. Ensure you leave enough documents and fully feature complete features before I do so on Claude on that macbook pro."

Read this page, then [Working on a Mac](../MACOS.md), then [the GitHub practices](../plans/GITHUB-PRACTICES.md). The owner's plans that matter here are in [docs/plans](../plans/README.md).

- [Start here](#start-here)
- [What the owner needs to bring from Windows](#what-the-owner-needs-to-bring-from-windows)
- [Where every repository stands on macOS](#where-every-repository-stands-on-macos)
- [Open issues and pull requests](#open-issues-and-pull-requests)
- [Order of work](#order-of-work)
- [Standing rules](#standing-rules)
- [Facts worth knowing](#facts-worth-knowing)

## Start here

1. **Set up the Mac** with [Working on a Mac](../MACOS.md#setting-up-a-mac-from-nothing): Apple's command line tools, Homebrew, `go node ffmpeg gh`, then `gh auth login` (the owner signs in; the account is `wittywilla`).
2. **Clone side by side** in `~/projects`, which plays the part of `D:\projects` on the Windows machine:
   ```bash
   mkdir -p ~/projects && cd ~/projects
   for r in WittyModule WittyMediaServer WittyMediaClient WittyPlay; do gh repo clone wittywilla/$r; done
   cd WittyMediaServer && node scripts/dev-modules.mjs
   ```
   Clone the others when their turn comes. [Repositories](../REPOSITORIES.md) lists all 23.
3. **Check the server builds and its tests pass here**, as in [Tests](../MACOS.md#tests). CI's `macos` job does the same on GitHub's runners, so a difference is about a real Mac.
4. **Look at what's open** (below), and pick up the [order of work](#order-of-work).

There is no `D:\` on the Mac. Every path in old notes, memories and plans that starts `D:\projects\<Repo>` means `~/projects/<Repo>` here, and `D:\0.FuturePlans` means [docs/plans](../plans/README.md), where every plan now lives and is changed through pull requests.

## What the owner needs to bring from Windows

Some things are private on purpose and never go in a repository. The owner copies them across by hand (a USB stick, or AirDrop from a phone), when the Mac chat asks:

| What | From (Windows) | To (Mac) | Why |
|---|---|---|---|
| The list of names that must never appear | `D:\projects\forbidden-names.txt` | `~/projects/forbidden-names.txt` | `scripts/publish-all.sh` searches every repository for them before a release, and a chat greps for them before each commit |
| Claude's memory for this project | `C:\Users\WittyWilla\.claude\projects\D--projects-WittyMediaServer\memory` | `~/.claude/projects/<the Mac's folder for ~/projects/WittyMediaServer>/memory` (Claude Code makes the folder the first time it runs there; its name is the path with `/` turned into `-`) | The standing rules and facts below in full. Paths in it are Windows paths: the Mac chat should correct them as it goes |
| The private legal review | `D:\projects\WittyMediaServer-private` | A folder of its own outside any repository, such as `~/projects/WittyMediaServer-private` | Never in a repository, never linked, never repeated publicly |
| Test media, if the owner wants the same | `D:\ServerMedia` | An external drive, or `~/Movies` and similar (not Desktop, Documents or Downloads, which the background service can't read) | Real discs and libraries to test against. Read-only: never write into it, and turn off saving metadata next to media while testing |

## Where every repository stands on macOS

As of 2026-11-04. "Built" means in CI, on GitHub's Mac runners or cross-compiled; none of it has run on a real Mac.

| Repository | What it is | macOS today | Next on a Mac |
|---|---|---|---|
| **WittyMediaServer** | The server, its menu-bar app, uninstaller and installers | Everything builds; on a Mac runner every pull request runs `go vet`, the tests (Apple silicon), builds the app and `.pkg` for both chips, installs it, checks the service answers, uninstalls it. Release builds the `.pkg` unsigned. Mac-only code: `*_darwin.go` in `cmd/wittymediaserver-tray`, `cmd/wittymediaserver-uninstall`, `internal/desktop`, `internal/diagnose`, `internal/modules` (sandbox), `internal/play/hostsandbox`, `internal/sysinfo`, `internal/uninstall/plan_mac.go`; packaging in `packaging/macos` | #136 to #141 |
| **WittyModule** | What modules share; `wittymodule pack` builds helpers for every system | Cross-compiles darwin in CI | Nothing Mac-specific |
| **WittyModuleCatalog** | The signed module catalogue | Data only | Nothing |
| **WittyPlay** | Games: runner, encoder host, cores | Builds for darwin; CI was red on its test probe (fixed in wittywilla/WittyPlay#20). `hostsandbox/*_darwin.go`, `stream/sandbox_darwin.go` written, never run. GPU consoles don't play on a Mac | wittywilla/WittyPlay#23 |
| **WittyCouch**, **WittyCouchHost**, **WittyRigs** | Streaming from a game PC | WittyCouch and WittyRigs cross-compile. WittyCouchHost (C and C++) builds natively on both Mac chips and runs `--version` | wittywilla/WittyCouchHost#1 |
| **WittyDVD**, **WittyBluray**, **WittyCD**, **WittyDisc** | Discs | Cross-compile. BD-J looks for Java in `/Library/Java/JavaVirtualMachines` | wittywilla/WittyBluray#4 |
| **WittyTuner** | Live TV sources | Cross-compiles; AVFoundation capture has parser tests only | wittywilla/WittyTuner#3 |
| **WittyPhotos**, **WittyIntegratedFileBrowser**, **WittyStats**, **WittyVisualizer**, **WittyEbook**, **WittyPDF**, **WittyComics**, **WittyInk** | Other modules and web bundles | Cross-compile, or are browser code | Run each under the Mac sandbox (#139) |
| **WittyMediaClient** | The apps: desktop (Electron), Android, Apple (SwiftUI: Mac, iPhone, iPad, Apple TV) | Desktop `.dmg` built on every push; Apple apps compiled unsigned on a Mac runner, WittyKit's tests run. CI was red (Apple build, theme data, a script check: wittywilla/WittyMediaClient#4). Its own [docs/PORTING.md and PLATFORMS.md](https://github.com/wittywilla/WittyMediaClient/tree/main/docs) | wittywilla/WittyMediaClient#5, #6 |
| **WittyMediaPlayer** | The disc player app (Qt and a Go helper) | **Windows only.** No Mac build, installer or toolchain script. Its DESIGN.md lists what a port needs: VideoToolbox in `player.cpp`, drives in `internal/drives`, a toolchain script | A planned feature (0.3.0 in [its plan](../plans/apps/WittyMediaPlayer.md)), not an issue: start a discussion or issue when work begins |
| **WittyReader** | Reader app (Electron and web) | `.dmg` built by its release workflow | wittywilla/WittyReader#13 |
| **wittymediaserver.com** | The website | Plain files | Nothing |

## Open issues and pull requests

**Pull requests from this work:**

| | What | State |
|---|---|---|
| wittywilla/WittyMediaServer#99 | This: Mac CI, docs/MACOS.md, this hand-over, the tests fixed for a Mac | Closes #85 and #134. The false "copy-protected" on some MKVs that the Mac job also found (#184) was fixed on master by #243 |
| wittywilla/WittyMediaClient#4 | CI green again: Apple build, theme data, script check | Draft, closes wittywilla/WittyMediaClient#3. Its FreeBSD job fails because FreeBSD's package repository had no Electron that day, unrelated |
| wittywilla/WittyPlay#20 | The test probe builds on macOS and FreeBSD; WittyPlay 0.3.5 | Merged |

**Issues for a real Mac** (`not-run`, `platform:macos`):

| Issue | What |
|---|---|
| #136 | The menu-bar app, the `.pkg` and the uninstaller with a person at the screen. 2026-11-10: built and looked inside, not installed (see [order of work](#order-of-work), step 2) |
| #137 | The background service, the firewall, Local Network and protected folders. 2026-11-10: read through, keys added; needs an install (step 3) |
| #138 | VideoToolbox transcoding. 2026-11-10: works on Apple silicon, Intel's fixed; needs real films (step 4) |
| #139 | The module helper sandbox; then whether forks can be allowed on a Mac. 2026-11-10: signal rule fixed; `sandboxAvailable` is the owner's call (step 5) |
| #140 | Drive health, system figures and the diagnoser |
| #141 | Owner step: the Apple Developer Program, for signing (`needs-owner`) |
| wittywilla/WittyMediaClient#5 | Apple apps on a Mac, iPhone, iPad and Apple TV |
| wittywilla/WittyMediaClient#6 | The desktop `.dmg` |
| wittywilla/WittyReader#13 | WittyReader's `.dmg` |
| wittywilla/WittyCouchHost#1 | WittyCouch Host against a real PC |
| wittywilla/WittyPlay#23 | Games: runner, sandbox, cores |
| wittywilla/WittyTuner#3 | AVFoundation capture devices |
| wittywilla/WittyBluray#4 | BD-J with an installed Java |

**Always current:** [every open `platform:macos` issue in every repository](https://github.com/search?q=org%3Awittywilla+label%3Aplatform%3Amacos+is%3Aopen&type=issues), and the same for [`not-run`](https://github.com/search?q=org%3Awittywilla+label%3Anot-run+is%3Aopen&type=issues). Other platforms have their own labels (`platform:windows`, `platform:linux`, `platform:freebsd`, `platform:android`, `platform:ios`, `platform:tvos`, ...) where a repository has them; the full set is in [GitHub practices](../plans/GITHUB-PRACTICES.md#labels). The server's release milestone is `0.7.0-alpha`, with its checklist in #84.

## Order of work

Each step is issues first, one branch and draft pull request per issue, as the [practices](../plans/GITHUB-PRACTICES.md) say.

1. **Green on the Mac.** Server tests, web build and design checks pass on the MacBook Pro as they do in CI. Anything that differs is a `bug` issue.
   - ✅ **2026-11-10, on the MacBook Pro** (M3 Pro, macOS 27.0.1, Go 1.27.3, Homebrew's FFmpeg 9.0.2 with VideoToolbox): the whole of `go test ./...` with no failures, `go vet`, the web build and the design checks. FFmpeg comes from Homebrew (`brew install ffmpeg`). `TestCellSeams` found a DVD's sound drifting after a join between cells, fixed in the server's sound filter (`aresample` `min_hard_comp=0.01`, `internal/hls/hls.go`). The discovery tests were made robust on a real Mac's network. `bump-server.sh`, `publish-all.sh` and `check-executable.sh` were made to run with the Mac's bash 3.2 and BSD tools.
2. **The server as people get it** (#136): build the app and `.pkg` on the Mac (`scripts/build-release.sh`, `packaging/macos/build-pkg.sh`), install it by double-click, use the menu-bar app, uninstall it. The owner clicks the system's questions; tell them exactly what to click.
   - ✅ **Built, 2026-11-10**, as [Packaging](../MACOS.md#packaging) says, into `dist/mine`, for both chips: each app holds thin binaries for its chip (`lipo -archs`: `arm64`, `x86_64`), signed ad hoc, `codesign --verify --strict` passes on both apps, `plutil -lint` passes on both `Info.plist`s. The `.pkg` is unsigned as expected (`pkgutil --check-signature`: no signature); `pkgutil --expand` and `--payload-files` show the two apps in `/Applications`, the `/usr/local/bin/wittymediaserver` link, `preinstall` and `postinstall` in the app part and `postinstall` in the service part, all passing `sh -n` and `bash -n`. The apps in the payload still verify after extracting it. The service's `postinstall` was run against a copy of its property list, with `launchctl`, `sudo` and `chown` stubbed: an upgrade keeps the data folder (with `&` and `<` in its name) and moves an old port into settings, an address with a host stays as `WMS_LISTEN`, a second run changes nothing, and the result passes `plutil -lint`. No script failed on bash 3.2 or BSD tools.
   - **Fixed:** the service's `postinstall` took PlistBuddy's "Error Reading File: ..." for the data folder when an old property list couldn't be read, and would have made a folder by that name; the uninstaller read a data folder written as XML text (`&`) literally, so it couldn't find a folder with `&` in its name; the menu-bar app's **Start at login** wrote its own path into a property list without escaping it.
   - **Still needs the owner, at the Mac:** double-click the `.pkg` (right-click, **Open**, as it's unsigned), try **Customize** without the background part, open the menu-bar app and use each menu item, turn **Start at login** on and off and sign out and in, run **Uninstall WittyMediaServer** (it asks for the administrator's password), and install over the 0.7.2-alpha copy to check an upgrade keeps the library. None of this was done: it changes the running copy.
3. **The service and the Mac's questions** (#137): restart the Mac, firewall on, Local Network, a library in a protected folder. Where a silent failure is found, make the server say why: that's the biggest likely gap for real people.
   - ✅ **Read through, 2026-11-10.** The service is a LaunchDaemon (`/Library/LaunchDaemons/com.wittywilla.wittymediaserver.plist`) that runs as the person at the screen (`UserName`), with `RunAtLoad`, `KeepAlive` and `WMS_SUPERVISED=1`, so an update's restart is left to launchd; its output goes to the server's own logs, as it has no `StandardOutPath`. The label is the same in the property list, `preinstall`, `postinstall`, the uninstaller (`internal/uninstall/plan_mac.go`) and the menu-bar app; the menu-bar app's own LaunchAgent is `com.wittywilla.wittymediaserver.tray`. `NSBonjourServices` (`_wittymedia._tcp`) matches `discovery.ServiceType`. Nothing in the server touches the firewall (`socketfilterfw`): the person answers its question.
   - **Fixed:** the app's `Info.plist` had no reason for protected folders, so a question would show no explanation. It now has `NSDesktopFolderUsageDescription`, `NSDocumentsFolderUsageDescription`, `NSDownloadsFolderUsageDescription`, `NSRemovableVolumesUsageDescription`, `NSNetworkVolumesUsageDescription`, and the camera and microphone for capture tuners (`avfoundation`).
   - **Still needs an install:** restart the Mac and check the service starts before anyone signs in; turn the firewall on and see whether the service (a LaunchDaemon can't show the question) is blocked silently; whether the service and the menu-bar app are asked about Local Network, and what a "no" does to discovery; a library in Desktop, Documents, Downloads and on a USB drive, from the service and from the menu-bar app alone, and whether a scan says why it's empty; whether the service needs Full Disk Access for those, as [MACOS.md](../MACOS.md#firewall-privacy-and-sleep) says.
4. **Transcoding with VideoToolbox** (#138), with real files, against the CPU.
   - ✅ **2026-11-10:** `go run ./cmd/wittymediaserver hardware` says `h264_videotoolbox  works`, with Homebrew's FFmpeg and with the one a release bundles. `TestVideoToolboxReallyEncodes` (new, skipped without VideoToolbox) makes a 2-second HLS stream with the server's own options, with a quality and with a bitrate limit, and `ffprobe` reads back H.264 at 1280x720. H.265 with `-tag:v hvc1` into MP4, as library transcoding does, came out `hvc1`, Main, `yuv420p`.
   - **Fixed:** on an Intel Mac, VideoToolbox never worked. FFmpeg refuses `-q:v` for it on Intel ("-q:v qscale not available for encoder. Use -b:v bitrate instead."), so the server's own test said it failed. Seen with the bundled Intel FFmpeg under Rosetta 2: `failed` before, `works` after. Intel now gets a bitrate for the picture's size; Apple silicon keeps `-q:v`.
   - **Still needs:** real films, against libx264, for speed, size and how it looks, on Apple silicon; and a real Intel Mac, as Rosetta 2 isn't one.
5. **The helper sandbox** (#139): on GitHub's runner it refused everything but signalling the server (the profile's `deny signal` rule didn't hold: find out why); on the MacBook, `go test ./internal/modules -run TestMac -v`, then every starter module under it. If it holds, propose setting `sandboxAvailable` true on a Mac (forks then allowed, and the fork tests stop skipping); that's a security decision for the owner.
   - ✅ **2026-11-10:** the signal rule is now `(deny signal)(allow signal (target same-sandbox))`, and `TestMacHelperSandbox` passes on the MacBook with every probe refused, signalling the server too (`signal REFUSED operation not permitted`). The Modules page's words say so.
   - **Still needs:** every starter module run under it; then the owner's decision on `sandboxAvailable`, which is still `false`.
6. **Modules one by one:** discs (WittyDVD, WittyBluray with Java: wittywilla/WittyBluray#4), WittyTuner (wittywilla/WittyTuner#3), WittyPhotos, readers. Faults go to the module's repository.
7. **Games** (wittywilla/WittyPlay#23), then WittyCouch Host (wittywilla/WittyCouchHost#1).
8. **WittyMediaClient**: the Apple apps (`apple/scripts/check-on-mac.sh`, wittywilla/WittyMediaClient#5), then the desktop app (#6). iPhone and Apple TV need signing.
9. **Signing and notarising**, once the owner has joined the Apple Developer Program (#141): sign locally first, then add it to the release workflows (server, WittyMediaClient, WittyReader, modules' helpers, WittyPlayCores), as [the code signing plan](../plans/design/code-signing-pipeline.md) says.
10. **WittyReader** (wittywilla/WittyReader#13), then **WittyMediaPlayer's** Mac build, which is new work from its [plan](../plans/apps/WittyMediaPlayer.md).
11. **Docs:** each step updates [Working on a Mac](../MACOS.md)'s table and "Known not to work", and the "not run on a real Mac" lines in INSTALL.md, UNINSTALL.md, GAMES.md, GAMING.md, STATS.md, MODULES.md, MODULE-STORE.md, SECURITY-REVIEW.md, CHANNELS.md, BLURAY.md and DVD.md.

## Standing rules

These come from the owner, over many chats. The Mac chat's memory holds them in full once the owner copies it across; until then, this is the list.

**Working on GitHub**

- **No work without an issue**, in the repository whose code is at fault. Bugs are issues labelled `bug`; features and plans are **Ideas** discussions labelled `planned`, which become an issue when work starts. Every issue is in a milestone, or `Backlog`.
- **One branch per issue, named `<issue>-<short-name>`**, and a draft pull request that says `Closes #<n>`, how it was tested (commands and results, SKIP lines counted), what wasn't tried, screenshots for UI, and a changelog line.
- **Run `/code-review` before marking a pull request ready.** CI must be green; a flaky test gets an issue, not re-runs.
- **Pushing branches and opening pull requests is fine. Merging, tagging and publishing need the owner's go-ahead** for that release. During a release the owner has approved, fix what fails and carry on, telling them in a line.
- **Things that need a person or real hardware** get `not-run` or `needs-owner` issues, with steps.
- **Versions:** bump each module's version (`wittymodule bump`) when its code changes, as its CI insists, and the server's in small steps as work lands. A bump isn't a release. Tags never move. When several chats work at once, a coordinator may do the server bump and `CHANGELOG.md`: ask.
- **Security problems** go through private vulnerability reporting, never a public issue.

**What the product never does**

- **Never decrypt copy protection or encrypted broadcasts**, and never add a way to: no plugin, option or documented bypass. Detect, explain and refuse. Licensed devices are sources only through their own documented interfaces. This was asked for and declined; it isn't pending.
- **Never name competing media servers or players** in code, comments, docs, UI, changelogs, issues or pull requests; describe features generically. The names are in `forbidden-names.txt` (above). The one exception is VLC's libraries in WittyMediaPlayer, optional and credited, never used to read protected discs.
- **The legal review is private.** Never commit it, link it, or repeat its questions in anything public.
- **Licences:** the server and modules are AGPL-3.0-or-later and client apps GPL-3.0-or-later; in forked work, upstream code keeps its own licence, labelled with the upstream project. Third-party tools can be any licence, but must be named clearly in the UI (Settings, About), not only in a file.
- **Games run on the server and stream**: no emulation in the browser or on a device.
- **Java is never shipped** for BD-J; it uses one the owner installs, off by default.

**How to work**

- **Performance and security first**: it must run well on low-end hardware. Audit what you touch, validate input and permissions on every endpoint.
- **The owner is new to GitHub** and happy to help: when a step needs them (an administrator password, a system question, an Apple account, a real device), ask plainly with exact clicks and commands.
- **Never open native windows on the owner's screen during tests**: not **Choose folder…** (`POST /api/fs/pick`), not file dialogs. Use **Browse folders** or type a path. On a Mac the same goes for anything that raises a system dialog.
- **Never build over a copy that may be running**: build into a new folder.
- **Test against real media read-only**, with saving metadata next to media turned off, and a `-data` folder of its own.
- **Long alpha:** no beta date; beta comes when every feature the owner wants is at least partly built ([the beta checklist](../plans/releases/beta-and-1.0.md)).
- **Finish the brief, then improve around it** where it's solid and finished, and say so in the report.

## Facts worth knowing

- **The server's module path is lowercase**, `github.com/wittywilla/wittymediaserver`; the repository is `wittywilla/WittyMediaServer`. The default branch is `master` here and `main` in every other repository.
- **`go.work` is made by `node scripts/dev-modules.mjs`**: never run `go work use`. Outside it, `GOWORK=off`.
- **Tests that read the API docs skip without real modules** (`WMS_TEST_CATALOG`, made by `scripts/make-test-catalog.sh`). Count SKIP lines before calling a run green; `-v` shows them.
- **FFmpeg differs by build.** CI's Linux job uses Ubuntu's FFmpeg 8; releases bundle 9.0.2 (`scripts/ffmpeg.lock`); Homebrew's is whatever is current. A test that passes on one can fail on another: #134 has a case.
- **Game speed tests mislead** on a busy machine: a fresh unsigned program may stall on its first launch, other work starves a low-priority runner, and the server log's `gameFps` beside `fps` tells the game from the encoder.
- **The test catalogue's key is throwaway**, trusted only by builds tagged `updatetest`.
- **Release signing key** (`WMS_SIGNING_KEY`) is a GitHub secret for the release workflow; it has nothing to do with Apple's signing.
- **The Windows machine keeps working** in parallel: chats there may have branches open in the same repositories. Pull before starting, and don't force-push.
