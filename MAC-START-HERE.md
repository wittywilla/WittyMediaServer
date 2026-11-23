# Start here on the Mac

One guide for every piece of work that needs a Mac, across the server, the apps and every module. Work through it
top to bottom on the owner's MacBook. It was written on the Windows PC on 2026-11-23 (issue #570). **None of the
work it lists has been run on a Mac**, unless an item says so.

How it fits with the rest:

- **This file** is the order of work: what to install, what to clone, then every item with its commands, what to
  expect, the pictures to compare and the issue it closes.
- **`MAC-FOLLOW-UPS.md`**, here and in WittyMediaClient, keeps the long step lists. When an item below says "Steps:
  *section*", that section has the detail and its own boxes to tick. Tick in both places.
- **[docs/MACOS.md](docs/MACOS.md)** is the reference for developing on a Mac: where files live, the service, the
  firewall, signing.
- **Issues labelled `mac`**, in every repository, each point at one item here:
  [all open ones](https://github.com/search?q=org%3Awittywilla+label%3Amac+is%3Aopen&type=issues).

## The rules, as for every session

- Use your own server on **port 19555** with a fresh `-data` folder. Never use or stop an installed server on 9797.
- Never write competitors' product names, and never decrypt or bypass copy protection. Use unencrypted test media only.
- Push branches and open pull requests freely. **Merging, tagging and publishing need the owner's go-ahead.**
- No work without an issue: a fault found here gets an issue in the repository whose code is at fault, in the
  `0.8.2-alpha` milestone, labelled `mac` and `bug`.

---

## 1. What the MacBook needs

Everything here is free. The owner's MacBook (Apple M3 Pro, macOS 27) already passed the server's tests on
2026-11-10 with these tools.

| What | Version | How | Needed for |
|---|---|---|---|
| macOS | 27 is what the owner's MacBook runs | | Everything |
| Xcode | 27, the newest in the App Store (CI uses `macos-latest`'s) | App Store, then open it once and accept the licence | The Apple apps (part 4 to 6) |
| Xcode's command line tools | With Xcode | `xcode-select --install` | Everything: git, clang, make, python3 |
| Simulators | iOS and tvOS, the newest | `xcodebuild -downloadPlatform iOS` and `xcodebuild -downloadPlatform tvOS` | iPhone, iPad, Apple TV |
| Homebrew | Current | the one command on [brew.sh](https://brew.sh), then its "Next steps" | Everything below |
| Go | 1.27 (`go.mod` names `go1.27.4`, which Go fetches itself) | `brew install go` | The server, modules, the apps' connection library |
| Node.js | 24 (WittyMediaClient's CI uses 22; both work) | `brew install node@24 && brew link --overwrite node@24` | The web build, the desktop app, WittyReader |
| GitHub CLI | Current | `brew install gh`, then `gh auth login` | Cloning, issues, pull requests |
| FFmpeg | Any with VideoToolbox (Homebrew's has it) | `brew install ffmpeg` | Transcoding, karaoke, tests |
| XcodeGen | 2.46.0 or newer | `brew install xcodegen` | Generating the Xcode project |
| CMake | Current | `brew install cmake` | WittyCouch Host, WittyMediaPlayer, WittyVisualizer |
| Qt | 6.10 (6.7 at least) | `brew install qt` | WittyMediaPlayer and WittyVisualizer only (part 7) |
| libbluray | Current | `brew install libbluray` | WittyBluray's second opinion only (part 7) |
| Java | Temurin 21 | `brew install --cask temurin@21` | WittyBluray's Java menus only (part 7) |
| Sunshine | Current, from its own project | its macOS installer | WittyRigs only (part 7) |

Not needed: Docker (WittyPlay runs games on the host on a Mac), Android Studio, and an Apple Developer account
(only for part 8).

Check it all at once:

```bash
sw_vers -productVersion; xcodebuild -version | head -1; go version; node --version; gh --version | head -1
ffmpeg -hide_banner -encoders | grep -c videotoolbox; xcodegen --version; cmake --version | head -1
xcrun simctl list runtimes | grep -E 'iOS|tvOS'
```

Expected: every command answers. The VideoToolbox count is 2 or more, and there's at least one iOS and one tvOS runtime.

## 2. Get every repository, side by side

`~/projects` does the job `D:\projects-0.8.0` does on the PC: one folder per repository, all side by side. The
scripts find each other that way.

```bash
gh auth login
mkdir -p ~/projects && cd ~/projects
for r in WittyMediaServer WittyMediaClient WittyModule WittyModuleCatalog WittyRelay WittyReader WittyMediaPlayer \
  WittyVisualizer WittyPlay WittyCouch WittyCouchHost WittyRigs WittyTuner WittyBluray WittyDVD WittyCD WittyDisc \
  WittyPhotos WittyIntegratedFileBrowser WittyMediaRemoteMedia WittyInk WittyEbook WittyPDF WittyComics WittyStats; do
  [ -d "$r" ] || gh repo clone "wittywilla/$r"
done
for r in Witty*; do (cd "$r" && git fetch -q origin && (git switch -q v0.8.1-alpha 2>/dev/null || git switch -q main) && git pull -q); done
cd WittyMediaServer && node scripts/dev-modules.mjs
```

Every repository uses `v0.8.1-alpha`, except WittyRelay, which only has `main`. When `v0.8.2-alpha` exists, use that
instead. `dev-modules.mjs` makes a `go.work` so the server builds against the module folders beside it.

Then build the server once. Most items below use it:

```bash
cd ~/projects/WittyMediaServer
(cd web && npm ci && npm run build)
go build -o /tmp/wms-081/wittymediaserver ./cmd/wittymediaserver
/tmp/wms-081/wittymediaserver serve -data /tmp/wms-081/data -listen 0.0.0.0:19555 -open-browser=false
```

Open `http://127.0.0.1:19555` in Safari and go through setup:

- an account;
- metadata writing off;
- a film library with a film or episode three minutes long or more, that has a backdrop;
- a music library with an album.

Leave the server running in its own Terminal tab. "The server" below means this one.

---

## 3. The server (WittyMediaServer)

Detailed steps: this repository's `MAC-FOLLOW-UPS.md`, "0.8.1-alpha: what the Mac must do".

- [ ] **3.1 Tests on macOS.**

  ```bash
  cd ~/projects/WittyMediaServer
  gofmt -l . ; go vet ./...
  go test ./internal/config ./internal/datahome ./internal/uninstall ./internal/separator ./internal/modules ./internal/ffmpeg ./internal/libtranscode
  go test ./... -count=1 -timeout 45m
  ```

  Expected: `gofmt` prints nothing and everything passes. Count the SKIP lines before calling it green. Leave
  `WMS_TEST_LAN` unset unless the firewall is off. A failure here gets a `bug` issue.

- [ ] **3.2 The installed service exits with 78** (#527). Build the `.pkg`, install it, and check it runs. Then
  uninstall and install again, five times in a row:

  ```bash
  SKIP_WEB=1 scripts/build-release.sh 0.0.0-mac darwin/arm64
  packaging/macos/build-pkg.sh 0.0.0-mac dist/wittymediaserver-0.0.0-mac-darwin-arm64 arm64 installers
  sudo installer -pkg installers/wittymediaserver-0.0.0-mac-macos-arm64.pkg -target /
  sudo launchctl print system/com.wittywilla.wittymediaserver | grep -E "state|last exit|program"
  curl -fsS http://127.0.0.1:9797/api/health
  sudo /usr/local/bin/wittymediaserver --uninstall --remove-data --yes
  ```

  Expected: `state = running`, and the health check answers within a minute, every time. A 78 even once is the bug;
  `packaging/macos/scripts/service/postinstall` is where to look. This one item uses port 9797, because the
  installed service always does. Run it only on a Mac with no installed server you want to keep.

- [ ] **3.3 Where settings and data are kept, and the new setup** (#528). Steps: "Data folder choice
  (WittyMediaServer #471)", items 8 to 16, with the `.pkg` from 3.2. Then setup's twelve steps in Safari and in the
  menu-bar app's window. Expected:
  - **Documents** is chosen by default.
  - The install folder is offered but not available.
  - With iCloud Drive syncing Documents, the page says "Synced by iCloud Drive".
  - macOS asks once before using `~/Documents`.
  - The menu-bar app restarts the server by itself after a move.

- [ ] **3.4 The graphics chip by default for transcoding** (#529).

  ```bash
  /tmp/wms-081/wittymediaserver hardware
  ```

  Expected: `h264_videotoolbox` and `hevc_videotoolbox` both say `works`. Then, with the server running:
  1. Play an HEVC film in Safari, with the player's quality menu set to 720p.
  2. Check Settings → Live transcoding and `GET /api/hls/<id>/work`: both should say Apple VideoToolbox does the work.

  Note FFmpeg's version.

- [ ] **3.5 Karaoke on Apple silicon** (#530). Settings → Lyrics and music information → install Demucs. Expected:
  - **Runs on** says Apple graphics (Metal).
  - About 900 MB lands in the data folder's `separator/`.
  - A song gets "Separated" within a few minutes.
  - In Safari's karaoke, a Voice settings preset is heard at once, with no drop-out.
  - **Remove** leaves nothing in `~/Library/Python`.

- [ ] **3.6 The server's address on setup's last page** (#528, from #546). Run the server on `-listen 127.0.0.1:19555`
  and on `0.0.0.0:19555`, each with Wi-Fi alone, then Ethernet and Wi-Fi, then a VPN on.

  Expected: the last page and `GET /api/system` (`lanGuess`, `lanUrls`) name the home network's address, never the
  VPN's or a link-local one. The QR code opens the server from a phone on the same Wi-Fi.

- [ ] **3.7 The menu-bar app, the service and the Mac's questions** (from 0.8.0; no issue of its own, report in #528).
  With the `.pkg` from 3.2, the owner at the screen:
  - Open the menu-bar app and use each menu item.
  - Turn **Start at login** on and off, and sign out and in.
  - Restart the Mac: the service answers before anyone signs in.
  - Turn the firewall on: is the service blocked silently?
  - Local Network: is the question asked, and what does "Don't Allow" do to discovery?
  - Put a library in Desktop, Downloads and on a USB drive: if a scan comes back empty, does it say why?
  - Change the time zone in setup: the server restarts and comes back in the new zone.

  Each surprise gets an issue.

- [ ] **3.8 The connection helper.**

  ```bash
  go test ./cmd/wittyconnect ./pkg/wittyconnect
  ```

  Expected: it passes, and `go run ./cmd/wittyconnect version` says `wittyconnect 2`.

## 4. The macOS desktop app (WittyMediaClient)

The desktop app is the server's website in an Electron window, so every change to the web is already in it. What's
checked here is its own part: the window, the menu bar, TV mode, peer to peer and the `.dmg`. Detailed steps:
WittyMediaClient's `MAC-FOLLOW-UPS.md`, "0.8.1-alpha: what the Mac must do" step 9, and "0.5.2" sections 3 and 4.

- [ ] **4.1 Tests and real runs against the server** (WittyMediaClient #70).

  ```bash
  cd ~/projects/WittyMediaClient/desktop
  npm ci && npm test && npm run test:app
  WMC_SERVER=http://127.0.0.1:19555 WMC_USER=<you> WMC_PASSWORD=<your password> WMC_SHOTS=/tmp/wmc-mac npm run test:real
  WMC_SERVER=http://127.0.0.1:19555 WMC_USER=<you> WMC_PASSWORD=<your password> WMC_SHOTS=/tmp/wmc-mac-tv npm run test:real -- --tv
  ```

  Expected: "All 8 passed", and with `--tv` "All 11 passed". The TV menu matches `docs/mac-081/desktop-tv-09-menu.png`.

- [ ] **4.2 The helper inside the app, and peer to peer for real** (WittyMediaClient #70). Steps: "0.5.2", sections 3
  and 4: `build-wittyconnect.mjs mac-arm64`, `helper-packaged.sh` prints `ok:`, then `p2p-real.js` home, away and
  invite. Each must print `ok`.

- [ ] **4.3 The `.dmg`** (WittyMediaClient #70).
  1. Run `npm run package:mac`, open the `.dmg`, and check the window shows the app, Applications and the uninstaller,
     with nothing overlapping.
  2. Drag the app across and open it (right-click, Open: it's unsigned).
  3. Connect, then play a film, a song and live TV.
  4. Start it with `open -a WittyMediaClient --args --tv`. Settings → Libraries should say **Leave TV mode to change
     this**, not show a QR card.
  5. Run **Uninstall WittyMediaClient.command** from the `.dmg`.
  6. Do the same with 0.6.0's `.dmg` from the GitHub release.

- [ ] **4.4 What 0.8.1 changed on the web, seen in the app.**
  - Press **K** on a song: karaoke's new first screen and Mix. Compare `docs/mac-081/ui/karaoke-desktop-kar-first-screen.jpg` and `karaoke-desktop-kar-mix.jpg`.
  - The mini player's More menu: compare `layout-miniplayer-more-menu-open-1440.jpg`.
  - A film's backdrop in Aero Sunset: compare `hero-edges-after-widths.jpg`.
  - In `--tv` mode, no keyboard hints anywhere.

## 5. iPhone and iPad (WittyMediaClient, Swift)

These were written on Windows and only compiled by CI's macOS runner. Expect fixes. The pictures to compare are in
WittyMediaClient's `docs/mac-081/`.

- [ ] **5.1 WittyKit's tests, the build and the simulators** (#68, #69).

  ```bash
  cd ~/projects/WittyMediaClient
  (cd apple/WittyKit && swift test)
  WITTYMEDIASERVER_SRC="$PWD/../WittyMediaServer" bash apple/scripts/build-wittyconnect.sh
  bash apple/scripts/check-on-mac.sh
  ```

  Expected: every test passes, `apple/Frameworks/Wittyconnect.xcframework` exists, and every line of
  `apple/build-logs/report.txt` says `ok`. A `FAIL building` is most likely a newer Xcode: fix it on a branch.

- [ ] **5.2 Peer to peer in the simulators** (#68, #69).

  ```bash
  WITTYMEDIASERVER_SRC="$PWD/../WittyMediaServer" bash apple/scripts/p2p-check.sh ios
  WITTYMEDIASERVER_SRC="$PWD/../WittyMediaServer" bash apple/scripts/p2p-check.sh tvos
  ```

  Expected: home, away and invite all pass. The pictures are in `apple/build/p2p-check`.

- [ ] **5.3 The 0.8.1 changes on iPhone** (#68). Steps: "0.8.1-alpha: what the Mac must do" step 5, against the
  server, with the skip-test `.wittyinfo` from its step 2. Expected:
  - The Skip buttons show at the times given.
  - "Skipped dream sequence" offers Undo.
  - Credits go to the scene after them.

  Compare `android-phone-07-playing-on-this-device.png` and `android-phone-11-karaoke-mix.png`.

- [ ] **5.4 The 0.8.1 UI changes on iPhone and iPad** (#68, from #82). Steps: "0.8.1-alpha UI changes on Apple",
  steps 4 to 7. Expected:
  - Karaoke starts with the singer's voice out, and **Guide voice** brings it back at 20%.
  - The Mix is a sheet on a phone and a side panel on an iPad.
  - Voice settings has nine presets.
  - The idle hiding works.
  - Backdrops fade into the page with no line.
  - The iPad bar has its More menu.
  - The strip of sections shows only libraries and Collections.

  Compare `docs/mac-081/ui/karaoke-phone-*.jpg`, `hero-edges-real-phone.jpg` and `537-nav-after-phone.jpg`. **The
  real risk is the sound:** the new audio units must never drop out or clip.

- [ ] **5.5 The hero scrim and theme colours on Apple** (#68). Steps: "0.8.1-alpha: what the Mac must do" step 8, and
  "0.5.2" section 8. This is code to write, not only a check: port Android's `ui/HeroScrim.kt` (it reads
  `tv.heroScrim`, default 0.72) and decode `focus`, `scrim`, `onImage`, `borderStrong`, `accentHover` and
  `controlLine`. Check Light and Aero Sunset: dark words on a light wash, light words on a dark one.

- [ ] **5.6 The 0.6.0 IPA on a real iPhone or iPad** (from the 0.6.0 owner list). Download
  `wittymediaclient-0.6.0-ios.ipa` from the release, sign it with a free Apple ID through AltStore, SideStore or
  Sideloadly, then connect to the server and play something. Expected: it starts, finds the server and plays. The
  install lasts 7 days.

## 6. Apple TV (WittyMediaClient, Swift)

Use the simulator with the remote only: arrows, Select and Menu, in the simulator's Siri Remote window. No keyboard
hints should show anywhere.

- [ ] **6.1 The 0.8.1 changes** (#69). Steps: "0.8.1-alpha: what the Mac must do" step 7.
  - The skip buttons take focus as they show.
  - A server editor kept for a computer is the card, with the QR code.
  - Karaoke's Mix works with the remote, Left and Right moving a level 5% at a time.

  Compare `android-tv-05-libraries-card.png` and `android-tv-07-karaoke-remote.png`.

- [ ] **6.2 The centred card for a computer's settings** (#69, from #83). Steps: "0.8.1-alpha: the centred card for a
  computer's settings". Each card is centred, its QR code is the largest thing on it, and **Back** has focus.
  Compare `docs/mac-081/tv-card-android-1080p.png`.

- [ ] **6.3 The 0.8.1 UI changes on Apple TV** (#69, from #82). Steps: "0.8.1-alpha UI changes on Apple" step 8.
  - The karaoke bar has Guide voice, Mix and Voice settings.
  - Home's billboard fades with no line.

  Compare `docs/mac-081/ui/karaoke-tv-*.jpg`.

- [ ] **6.4 TV levels on tvOS** (#69). Steps: "TV levels (WittyMediaServer #469)", items 1 to 6.
  - Decode `tvLevels` from `/sdui/catalog`.
  - A `desktopOnly` screen is the card.
  - Console and gaming screens are never the card.

- [ ] **6.5 The 0.6.0 Apple TV IPA on a real Apple TV** (from the 0.6.0 owner list). Sign it with Sideloadly or Xcode,
  not AltStore. It holds two things to sign: the app and its Top Shelf. Expected: it starts, connects and plays. An
  empty Top Shelf is fine without the App Group.

- [ ] **6.6 The Apple TV ports for 0.8.2** (draft WittyMediaClient #91, branch `apple-082-ui`). These are **to write**
  first, then run:
  - The WittyTV guide's Shortcuts toggle and whole rows (#84).
  - The centred Who's watching? picker that starts on the last-used profile (#84, #88).
  - The pairing screen as one column (#88).
  - The video player's Quality menu (#87, iPhone too).
  - The TV album page with disc groups (#89).

  Steps: "0.8.2-alpha: Apple ports left for the next update" on that branch, and "WittyTV guide and Who's watching?
  on Apple TV". Android TV's pictures in `docs/mac-082/` and `docs/mac-081/` are the reference.

- [ ] **6.7 Apple TV's own Settings pages** (#90, draft WittyMediaClient #81, branch `tv-settings-082`). Steps:
  "0.8.2-alpha: Apple TV's own Settings pages" on that branch. Run WittyKit's `TvSettingsTests`, then each new page
  with the remote, once as an administrator and once as an adult who isn't one. Compare `docs/mac-082/02-you.png`
  to `09-about.png`.

## 7. Modules and other apps with macOS code

Each module below has macOS-specific code. Most modules have none: they're plain Go, cross-compiled for darwin by
their CI. 7.1 covers them all at once.

- [ ] **7.1 Every module, installed from the store** (WittyMediaServer #571).
  1. In the server, Settings → Modules: install each module from the store (the released darwin/arm64 builds).
  2. Check each one opens and does its main job: Photos shows a folder, the Folder browser lists one, DVD and CD read
     an unencrypted image, Statistics fills, and the readers open a book.
  3. Run `go test ./internal/modules -run TestMac -v` in the server.

  Expected: the test passes, the Modules page says each helper is sandboxed (except WittyPlay, which has its own),
  and nothing in the log says a helper was refused something it needs. Then the owner decides whether
  `sandboxAvailable` turns on for a Mac. That's a security call.

- [ ] **7.2 WittyPlay: games on a Mac** (WittyPlay #97). Last run on this Mac at 0.3.6; 0.7.10 changed a lot.

  ```bash
  cd ~/projects/WittyPlay && go test ./...
  ```

  Then install WittyPlay from the store and play these in Safari:
  - a Game Boy game;
  - an NES game;
  - a PSP game (PPSSPP, on the Mac's OpenGL);
  - a PlayStation 2 game (Play!).

  Expected:
  - Full speed, as in WittyPlay's README table ("A Mac's graphics card").
  - The cores that can't draw with a Mac's OpenGL fall back to the processor with a sentence.
  - A standalone emulator (PPSSPP or Dolphin) asks for Screen Recording and Accessibility once, and streams.
  - Diagnostics names a refused permission.

  Use the owner's own games only.

- [ ] **7.3 WittyCouch Host** (WittyCouchHost #9).

  ```bash
  cd ~/projects/WittyCouchHost && sh scripts/build.sh && dist/wittycouch-host-macos-arm64 --version && otool -L dist/wittycouch-host-macos-arm64
  ```

  Expected: it prints its version, and `otool` lists only system libraries. Then stream from a Sunshine PC through
  WittyCouch on the server.

- [ ] **7.4 WittyRigs: this server's own Sunshine** (WittyRigs #9). With Sunshine installed on the Mac, turn the
  option on. Expected: Sunshine is found, pairs with a PIN, and its games show as a library.

- [ ] **7.5 WittyTuner: capture devices** (WittyTuner #26). Run `go test ./...`, then add a capture device (a USB
  capture stick, or the camera as a stand-in). Expected: it's listed, and macOS asks for the camera once, with the
  server's words. A channel made from it plays. A "Don't Allow" gives a sentence, not an empty list.

- [ ] **7.6 WittyBluray: libbluray and Java menus** (WittyBluray #18), with an unencrypted image only.

  ```bash
  cd ~/projects/WittyBluray && go test ./... && go run ./cmd/wittybluray rank <image> && go run ./cmd/wittybluray bdj <image> 20 /tmp/menu.png
  ```

  Expected: `rank` names libbluray as a second opinion, and `bdj` runs on Temurin 21 and writes the menu's picture.

- [ ] **7.7 WittyReader's `.dmg`** (WittyReader #36). From the latest release:
  1. Open it (right-click, Open) and drag the app across.
  2. Open an `.epub`, a `.pdf` and a `.cbz` from Finder.
  3. Take notes, quit and reopen: the notes are kept.
  4. Remove it, and check what it asks and what it deletes.

- [ ] **7.8 WittyMediaPlayer and WittyVisualizer's Qt part** (WittyMediaPlayer #24). Build both with Homebrew's Qt and
  CMake, run `ctest`, and open a film and a song with a visualizer. Expected: both draw with Metal. A `.dmg` is new
  work: list what it needs in the issue.

- [ ] **7.9 WittyRelay builds and its tests pass on macOS** (no issue; it's a Linux and Docker service).

  ```bash
  cd ~/projects/WittyRelay && go test ./... && go build ./cmd/wittyrelay
  ```

## 8. What needs the owner's Apple developer account

Nothing above does. A free Apple ID covers the simulators, Xcode builds on the owner's own devices (they expire
after about a week) and sideloading the IPAs. Signing and publishing need **the Apple Developer Program**, about
US$99 a year, which only the owner can join (WittyMediaServer #572):

| What | Without the programme | With it |
|---|---|---|
| Server `.pkg`, menu-bar app, uninstaller | Unsigned: right-click, Open | Developer ID signed and notarised: [docs/MACOS.md, Signing and notarising](docs/MACOS.md#signing-and-notarising) has the seven steps, and `packaging/macos/build-pkg.sh` signs when `APPLE_APP_IDENTITY`, `APPLE_INSTALLER_IDENTITY` and `APPLE_NOTARY_PROFILE` are set |
| WittyMediaClient `.dmg` | Unsigned, with a stand-in signature | Set `CSC_LINK` and `CSC_KEY_PASSWORD` in the release workflow, remove `CSC_IDENTITY_AUTO_DISCOVERY: 'false'`, and remove `identity` and `hardenedRuntime` from `mac` in `desktop/package.json` (WittyMediaClient's `docs/OWNER-STEPS.md`) |
| iPhone, iPad and Apple TV | Unsigned IPAs, sideloaded for 7 days | TestFlight and the App Store. Apple asks about plain HTTP on the home network, local network access, and background audio (picture in picture), and wants a way to test: a reviewer's server or a demo mode, which doesn't exist yet |
| Apple TV's Top Shelf | Empty unless the signer adds the App Group | `group.com.wittywilla.wittymediaclient` on the app and its extension |
| WittyReader `.dmg` | Unsigned | The same Developer ID as the server |
| Module helpers and game cores | Unsigned, which works because nothing is hardened | Signed with **the same Team ID**: a hardened, signed runner refuses libraries from another team, so WittyPlay's cores must be signed by us |

The order, once the owner has joined:

1. Sign the server's `.pkg` by hand, as `docs/MACOS.md` says, and check it with `spctl --assess`.
2. Then add signing to the release workflows: the server first, then WittyMediaClient, WittyReader, the modules'
   helpers and the game cores. The plan is [docs/plans/design/code-signing-pipeline.md](docs/plans/design/code-signing-pipeline.md).
   It needs the `.p12`, its password and an App Store Connect API key as secrets in a protected `release`
   environment.

Never put a key or certificate in a repository.

## 9. Reporting back

- **Tick** each box here and in the `MAC-FOLLOW-UPS.md` section it names, and commit the ticks.
- **Pictures:**
  - put them beside the reference ones, with `-mac` in the name: WittyMediaClient's `docs/mac-081/` and
    `docs/mac-081/ui/`, and `docs/mac-082/` for 0.8.2;
  - for the server and modules, put them in the issue.
- **Fixes:** one branch per issue (`<issue>-<short-name>`) off `v0.8.2-alpha` (or `v0.8.1-alpha` until it exists),
  and a draft pull request titled "[0.8.2] …" that says `Closes #<n>`, how it was tested, and what wasn't.
- **Issues:** comment on each `mac` issue with what ran and what didn't, and close the ones fully answered.
- **The next session:** add a short dated section to the top of this file, "What the Mac found (date)", with links,
  so the PC can pick up the fixes.

## Kept in step

When work for the Mac is added anywhere, it gets the same three things:

- an item in the right part of this file;
- the long steps in that repository's `MAC-FOLLOW-UPS.md` (or the issue, for a module);
- a `mac` issue in `0.8.2-alpha` that names the item.

The 0.8.2 sessions add theirs here as they land: security-apps-082, p2p-harden-082, apps-082 and modules-082.

### Added for 0.8.2

Nothing yet beyond 6.6 and 6.7.
