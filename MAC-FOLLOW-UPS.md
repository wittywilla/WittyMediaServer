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

## Not run anywhere yet

6. **The macOS job in the server's `build` workflow fails on `v0.8.0-alpha`** (run 43675494472, job 132694687376). Its log wasn't readable from Windows. Read it with `gh run view --log-failed` and fix it.
7. **WittyMediaPlayer's installer picture:** its `welcome.bmp` let the scene run out under the words, fixed in the server's brand code in #343. Run `scripts/sync-brand.sh` in WittyMediaPlayer to pick it up. It doesn't need a Mac, but nobody has done it yet.

## Still open from NEXT-SESSION.md

The iPhone pass, anything needing a second account or a real controller, Refresh game details on
Apple, and screen-reader speech are unchanged: see "Not tested yet" in `NEXT-SESSION.md`.
