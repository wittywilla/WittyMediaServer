# Mac follow-ups from the Windows and Linux pass

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
