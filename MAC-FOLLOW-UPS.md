# Mac follow-ups

**Start with [MAC-START-HERE.md](MAC-START-HERE.md)**: it says what the Mac needs, how to clone every repository, and
the order to do everything in, across the server, the apps and the modules. This file keeps the server's long step
lists that it points to. Add new Mac work in both: an item there, the steps here.

Newest first. Everything here needs a Mac. Tick each box as it's done, commit the ticks, and say in the PR what ran.
Merge nothing, and don't release or tag without the owner's OK. The GitHub issues labelled `mac` point at these
sections. The apps' Mac items (iPhone, iPad, Apple TV, the macOS desktop app) are in WittyMediaClient's
`MAC-FOLLOW-UPS.md`, under the same heading.

## 0.8.1-alpha: what the Mac must do

Written 2026-11-19 on the Windows PC, for Claude on the owner's MacBook. Nothing here was run on a Mac. Issues: #527
(the exit 78), #528 (data folder and setup), #529 (VideoToolbox), #530 (karaoke on Metal). Use the
server's `v0.8.1-alpha` branch:

```bash
cd ~/<where the repos are>/WittyMediaServer
git fetch origin && git switch v0.8.1-alpha && git pull
(cd web && npm ci && npm run build)
```

- [ ] **1. The installed service exits with 78 (EX_CONFIG).** CI's macOS job installs the `.pkg` and waits for
  `http://127.0.0.1:9797/api/health`. Once (PR #482's run, `gh run view 44458269476 -R wittywilla/WittyMediaServer --log-failed`)
  launchd said the service ended with 78 and it never answered; the same base passed with #496, and #482 changed
  nothing near it. The server itself never exits with 78 (no `os.Exit(78)` anywhere), so it's launchd's own
  EX_CONFIG: launchd couldn't start the program as its plist says. Likeliest: `postinstall` bootstraps the service
  before the helper or its folder is in place, a user or folder the plist names doesn't exist yet on a clean Mac, or
  the data-location start-up (#473) runs before `WMS_DATA_DIR` applies. On the Mac, as ci.yml's macOS job does:

  ```bash
  SKIP_WEB=1 scripts/build-release.sh 0.0.0-mac darwin/arm64
  packaging/macos/build-pkg.sh 0.0.0-mac dist/wittymediaserver-0.0.0-mac-darwin-arm64 arm64 installers
  sudo installer -pkg installers/wittymediaserver-0.0.0-mac-macos-arm64.pkg -target /
  sudo launchctl print system/com.wittywilla.wittymediaserver | grep -E "state|last exit|program"
  curl -fsS http://127.0.0.1:9797/api/health
  sudo grep -E 'postinstall|preinstall' /var/log/install.log | tail -30
  ```

  Expected: `state = running` and the health check answers within a minute. Uninstall
  (`sudo /usr/local/bin/wittymediaserver --uninstall --remove-data --yes`) and install again five times; a 78 even
  once is the bug. Read `packaging/macos/scripts/service/postinstall` for the order it does things in, fix it on a
  branch off `v0.8.1-alpha` with an issue, and say in the PR how many installs in a row passed.

- [ ] **2. Where settings and data are kept, on macOS (#473).** Items 8 to 16 of "Data folder choice (WittyMediaServer
  #471)" below, with a `.pkg` built from `v0.8.1-alpha` (the PR has merged since they were written). Expected results
  are written there. Also check first-time setup's twelve steps (#508) in Safari and in the menu-bar app's window: the
  data folder step appears after the account, and every step's pictures load.

- [ ] **3. The graphics chip by default for transcoding (#441, #510).** On a Mac that's VideoToolbox.

  ```bash
  go build -o /tmp/wms-081/wittymediaserver ./cmd/wittymediaserver
  /tmp/wms-081/wittymediaserver hardware
  go test ./internal/ffmpeg ./internal/libtranscode
  ```

  Expected: `hardware` lists the Apple graphics and `h264_videotoolbox` / `hevc_videotoolbox` as working. Then run the
  server (`/tmp/wms-081/wittymediaserver serve -data /tmp/wms-081/data -listen 127.0.0.1:19555`), play an HEVC film in
  Safari with the player's quality menu at 720p, and check Settings → Live transcoding and `GET /api/hls/<id>/work`
  say the graphics chip does the work. With FFmpeg from Homebrew; note its version. On an Intel Mac, the same.

- [ ] **4. Karaoke on Apple silicon (#436, #478).** No CoreML: Demucs runs on PyTorch's Metal (`mps`), else the processor.
  Settings → Lyrics and music information → install Demucs. Expected: **Runs on** says Apple graphics (Metal),
  about 900 MB is fetched into the data folder's `separator/`, and a song's karaoke gets "Separated" within a few
  minutes. A song that fails on Metal is done again on the processor (check the log, `cat=karaoke`). On an Intel Mac it
  says the processor. Then **Remove**, and check `separator/` is gone and nothing was left in `~/Library/Python` or
  the system's Python.

- [ ] **5. `go test` on macOS** for what 0.8.1 changed most: `go test ./internal/config ./internal/datahome ./internal/uninstall ./internal/separator ./internal/server`
  (the last takes a while). Expected: all pass.

- [ ] **6. The server's address on setup's last page, and the voice presets on Metal (added 2026-11-22).** Once #546
  is in `v0.8.1-alpha`: run the server listening on loopback (`-listen 127.0.0.1:19555`) and on all addresses
  (`-listen 0.0.0.0:19555`), each with Wi-Fi alone, then with Ethernet and Wi-Fi, then with a VPN on. Setup's last
  page (and `GET /api/system`'s `lanGuess`, `localOnly`, `lanUrls`) should name the Mac's home-network address, the
  one the default route uses (`mainLANIP` asks the system with a UDP socket, sending nothing), never the VPN's or a
  link-local one, and its QR code should open the server from a phone on the same Wi-Fi. Also, with Demucs on Metal
  (step 4), play a separated song in Safari's karaoke and choose a Voice settings preset (#524): heard at once, with
  no drop-out. The Apple apps' side of the 0.8.1 UI changes is in WittyMediaClient's `MAC-FOLLOW-UPS.md`, "0.8.1-alpha
  UI changes on Apple".

- [ ] **7. Report:** tick these, commit, and close the `mac` issues this section answers.

---

## 0.8.0 (2026-11-15): from the Windows and Linux pass

Written 2026-11-15, after the Windows and Linux pass on the owner's PC (`NEXT-SESSION.md`,
"Next: Windows and Linux"). Everything below needs a Mac. Nothing here was run on one.

**For Claude on the Mac:** work through this list in order. Each item names its pull request.
Merge nothing, and don't release or tag without the owner's OK. Tick items off here as they're
done, then delete this file once the list is empty.

## Pull requests to check on macOS

These changed code that also runs on a Mac.

1. **WittyMediaServer #343** (Windows pass): run `go test ./internal/modules ./internal/server` on macOS.
   - It changes how a module's programs are stopped before their folders are removed or replaced (`stopService`, `stopRunningFrom`, `proc.stop`, `removeRetry`).
   - It adds `pipe_other.go` for every system but Windows.
   - It also adds a `.gitattributes` that checks every text file out with LF. After pulling, check `git status` is clean.
2. **WittyMediaServer #349** (`wittyconnect join`): run `go test ./cmd/wittyconnect ./pkg/wittyconnect`, then build the helper for the desktop app (`node desktop/scripts/build-wittyconnect.mjs` in WittyMediaClient). `wittyconnect version` should say `wittyconnect 2`.
3. **WittyMediaClient #46** (From away on the desktop, as on Android and Apple; merged into `v0.8.0-alpha`):
   - `npm test` and `npm run test:app` in `desktop/`, with the helper from item 2.
   - The Mac's menu bar now has **From Away…** in place of Stop / Connect again. Check it opens the From away screen, that Back returns to the server, and that Stop asks first.
   - Compare the desktop's Join from away and From away screens with the iPhone's side by side. They should look the same (`docs/APP-DESIGN.md`, From away).
   - If there's a second network to try it on, run `test/app/p2p-real.js` with a code typed into the server. The desktop has never joined a real server by a code.
4. **WittyKit on Apple** (in #46): to build again on Linux, `GameStream.swift`, `GameTransport.swift`, `ForgetServer.swift`, `WittyClient.swift` and `GamesTests.swift` got `#if canImport(...)` guards and one Linux-only branch in `measureSpeed`. Build the iOS, tvOS and macOS apps and run `swift test` in `apple/WittyKit` to confirm nothing changed on Apple.
5. **The time zone** (#343, #350) and **first-run setup's Language and time** (#352): a new time zone now restarts the server once, through `s.Restart`, instead of changing `time.Local` while it runs. On macOS, check that the restart works from the menu-bar app and as a launchd service, and that it comes back in the new zone.

## Data folder choice (WittyMediaServer #471)

Setup's "Where should WittyMediaServer keep its settings and data?" page and Settings → Data folder were tested on
Windows and in WSL Linux. On a Mac, with the menu-bar app from a `.pkg` built from the branch:

8. **Fresh install:** the page appears after the account, with **Documents** chosen (`~/Documents/WittyMediaServer`) and
   **The install folder** shown as not available, saying a Mac app can't keep files inside itself. Keep it here: the
   data moves, `~/Library/Application Support/WittyMediaServer/data-location.json` names the new folder, the old folder
   keeps only `data-location.json` (and `wms.json` if there was one), and setup goes on to Name your server.
9. **The menu-bar app after the move:** it starts the server again by itself (exit code 75 relaunches it), **Open data
   folder** opens `~/Documents/WittyMediaServer`, and the log is in that folder's `logs`.
10. **Privacy prompt:** macOS asks before an app reads or writes `~/Documents`. Check the prompt appears once, names
    WittyMediaServer, and that saying **Don't Allow** leaves the page saying the server can't write there, not a crash.
    If the prompt comes from the helper (`Contents/Helpers/wittymediaserver`) rather than the app, note how it reads.
11. **iCloud Desktop & Documents:** turn it on in System Settings → Apple Account → iCloud → iCloud Drive. The page must
    say "Synced by iCloud Drive" under Documents and offer **Use a local folder**
    (`~/Library/Application Support/WittyMediaServer`). Detection looks for
    `~/Library/Mobile Documents/com~apple~CloudDocs/Documents`; check that folder really exists with the setting on, and
    is gone with it off. A folder under `~/Library/CloudStorage` (Dropbox, Google Drive, OneDrive for Mac) should say
    "a cloud storage program".
12. **An upgrade from 0.8.0:** install over a Mac with data in `~/Library/Application Support/WittyMediaServer`. The
    server starts on it, setup doesn't ask, and Settings → Data folder offers to move it.
13. **The background service** (`launchd`, `WMS_DATA_DIR` in its plist): setup doesn't ask, and Settings → Data folder
    says the folder is set by `WMS_DATA_DIR`.
14. **A drive that isn't connected:** move the data to an external drive in Settings → Data folder, quit, eject the
    drive, and open the app. It should show "WittyMediaServer can't start" with the folder's path, not start empty.
15. **Uninstall** with "remove my data": `~/Documents/WittyMediaServer` and `data-location.json` go.
16. `go test ./internal/config ./internal/datahome ./internal/uninstall` on macOS.

## Not run anywhere yet

6. **The macOS job in the server's `build` workflow fails on `v0.8.0-alpha`** (run 43675494472, job 132694687376). Its log wasn't readable from Windows. Read it with `gh run view --log-failed` and fix it.
7. **WittyMediaPlayer's installer picture:** its `welcome.bmp` let the scene run out under the words, fixed in the server's brand code in #343. Run `scripts/sync-brand.sh` in WittyMediaPlayer to pick it up. It doesn't need a Mac, but nobody has done it yet.

## Still open from NEXT-SESSION.md

The iPhone pass, anything needing a second account or a real controller, Refresh game details on
Apple, and screen-reader speech are unchanged: see "Not tested yet" in `NEXT-SESSION.md`.

## TV levels (#469, 2026-11-18)

What a TV offers is one table now (`docs/TV-MODE.md`, `pkg/sdui/data/tvlevels.json`). The web app's TV mode and
Android TV follow it. Still to do, in WittyMediaClient (its `MAC-FOLLOW-UPS.md` has the steps): tvOS reads
`tvLevels` from the catalogue and shows the card for `desktopOnly` screens, and the TV mode check on Plasma
Bigscreen and in the desktop app started with `--tv` on macOS.
