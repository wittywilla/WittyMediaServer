# Code audit, October 2026

A full audit of the server, the modules, the web client, the scripts and the CI workflows, at
version 0.4.0-alpha (commit `4b1b1eb`). Every finding, what was done about it, and what was left and
why.

## How it was checked

- **Read:** all of `internal/`, `cmd/wms`, `modules/wittydisc`, `modules/wittyintegratedfilebrowser`,
  the session and navigation layers of the disc engines, `web/src/api.ts`, the app shell, the
  player and library pages, the build scripts and workflows.
- **Run against a real server** with the test media: sign-in and role checks on all 104 API routes;
  real calls on every read endpoint as admin and kid; hostile input (path traversal, bad numbers, SQL in parameters, wrong
  bodies, zip and logo attacks); every write endpoint; real transcodes of a film, a DVD and a
  Blu-ray; every page and reader in a browser, watching the console. The media folder was compared
  before and after: **nothing was written to it**.
- **Tools:** `go vet`, `staticcheck`, `govulncheck`, `npm audit` (0 findings), a new fuzz target.
- **Real discs after the changes:** `wms dvd crawl` on Shrek 2: 268 screens, 0 problems.

## Numbers

Same library before and after: 5000 films, 400 shows, 20000 episodes, one account with about 3500
entries of progress (in the HTTP test, 500 of the films are discs). Before is commit `4b1b1eb`.

### Queries (`go test ./internal/library -bench 'Store|HomeQueries'`, 12th-gen laptop processor)

| What | Before | After | Memory allocated per call |
|---|---|---|---|
| **Everything behind the home page** | 146 ms | **33 ms** | 12.1 MB → 1.9 MB (606k → 23k allocations) |
| One library's details | 7.4 ms | 0.3 ms | |
| All libraries | 7.6 ms | 1.5 ms | |
| A page of a library, by title | 3.1 ms | 0.8 ms | |
| Recently added in a library | 9.8 ms | 0.6 ms | |
| All shows of a library | 29.6 ms | 6.7 ms | |
| Recently added shows | 9.3 ms | 5.3 ms | |
| Continue watching | 6.5 ms | 0.3 ms | 438 KB → 58 KB |
| Progress for 20 items | 2.7 ms | 0.05 ms | 384 KB → 4 KB |
| Next episode | 0.45 ms | 0.07 ms | 119 KB → 37 KB |
| Genres | 9.6 ms | 7.2 ms | 934 KB → 45 KB |

### Over HTTP, on a machine at 100% load from other work

The medians of 9 requests. The load makes these slow, and it also makes them a fair picture of a
small machine. Timings on the same machine with less load were lower, with the same proportions.

| Request | Before | After |
|---|---|---|
| `/api/home` | 962 ms | **310 ms** |
| A library page, by title (120 items) | 111 ms, 115 KB | 12 ms, 81 KB |
| A library page, newest first (120 items) | 192 ms, 108 KB | 16 ms, 80 KB |
| `/api/shows` | 315 ms | 66 ms |
| `/api/shows/{id}` | 29 ms | 10 ms |
| `/api/items/{id}` of an episode | 26 ms | 5 ms |
| `/api/libraries` | 48 ms | 11 ms |
| A whole library's list, no limit | 4.48 MB | 3.34 MB (the page now asks for 120 at a time) |

Idle memory was the same within measurement error (17 MB → 15 MB resident). Start time was the same
on a quiet machine (about 510 ms, both). The database upgrade to the new indexes took about half a
second on this library, once.

### Web bundle (`npm run build`)

| Script | Before | After |
|---|---|---|
| **First load, uncompressed** | 1218 KB | **370 KB** |
| **First load, as sent (gzip)** | 1218 KB (the server didn't compress) | **117 KB** |
| hls.js, with the player | in the first load | 614 KB (192 KB gzip), when playing |
| Readers | in the first load | 59 KB, when reading |
| PDF engine and worker | loaded with a PDF | the same; the worker now travels as 417 KB, not 1418 KB |
| Settings, setup, other pages | in the first load | 1 to 60 KB each, when opened |
| Stylesheet | 138 KB | 138 KB, sent as 27 KB |

The compressed copies add 965 KB to the program (about 26 MB).

### Transcoding, on a real 2.5-hour film

- FFmpeg stopped itself at segment 92 of 2286 for a viewer at the start, instead of converting the
  whole film, and started again when asked for segment 87. Timestamps across the restart differ by
  about 0.1 s (video) and 0.03 s (audio), the same as after a seek.
- Five transcodes at once: 4 ran, the 5th got `503` with `Retry-After` (before: all five ran, with no
  limit at all, whatever `-max-transcodes` said).
- After stopping: no FFmpeg process left, and the transcode folder empty.

## Findings

Severity is what it means for someone running this on their home network with a few accounts.
Commits: **A** `b9a706b`, **T** `914891c`, **D** `b321f5b`, **W** `57b8c0f`, **C** `488e84b`,
**R** `6fdd16c` (tests), **N** `f3c442b` (docs).

### High

| # | Finding | Fix |
|---|---|---|
| 1 | **Stored script injection through podcasts.** The server passes on an episode from whoever hosts the feed, with their content type, on this site's address. A hostile host answers `text/html` with a script; a signed-in person opening that link runs it with their account, including an administrator's. `/api` had no content policy. | A: every `/api` answer has a policy that runs nothing; the proxy passes only audio and video types. Test: R |
| 2 | **Any file on the machine could be copied beside the media.** With "save next to media" on (the default), a sidecar file saying `"poster": "<any path>"` or `cache:../../wms.db` made a scan copy that file to `poster.jpg`. Anyone who can write to a shared media folder could read the database, with everyone's session tokens. | A: only pictures are copied, cache names stay in the cache. Test: A |
| 3 | **The same path reached the local network.** Artwork being saved was fetched with the plain HTTP client, so an NFO could make the server call devices on the home network and store the answer. | A: internet-only client |
| 4 | **Starting the server deleted the whole transcode folder**, whatever was in it. Pointing `transcodeDir` at an existing folder lost its contents. | T: only folders the server made are removed. Test: T |
| 5 | **Releases shipped known vulnerabilities.** CI used the Go version `go.mod` names as a *minimum* (1.26.0). Govulncheck finds 11 reachable standard library vulnerabilities (HTTP/2, `net/http`, TLS, X.509) in 1.27.0 too, fixed in 1.27.3. | C: CI and releases use the newest 1.27 patch. **Your own builds still use your installed Go: update it** |
| 6 | **A crash during a database upgrade could stop the server from ever starting.** Steps weren't transactions or recorded one by one, and `ALTER TABLE` can't run twice. A locked database was also read as "new". | A: one transaction per step. Tests: R |
| 7 | **`-max-transcodes` did nothing.** Any signed-in account could start any number of FFmpeg processes, for example with a different `maxHeight` each. | T |
| 8 | **A drive that is away marked its whole library missing**, as did an unmounted mount point (which reads as an empty folder). Coming back re-read everything from scratch, losing hand edits when saving beside media was off. | A: such a folder's items stay, returning files keep their edits. Test: R |

### Medium

| # | Finding | Fix |
|---|---|---|
| 9 | A film paused for over 10 minutes hung for ever when resumed: the session was deleted and the player retried 404s without end. | T: the stream stays for 24 hours (only its segments go); the player reports a stream really lost |
| 10 | Hidden shows' artwork was served to accounts not allowed to see the show. Genres, networks, podcasts and scan progress ignored account limits. Lists ignored a show's genres when a genre was blocked, while playback refused. Saving progress worked on hidden items. | A. Tests: A, R |
| 11 | TMDB API keys reached logs and error messages, inside the address an HTTP client's error quotes. Log redaction only looked at text, and errors are logged as values. | A, D. Tests: D |
| 12 | Logging in: guessing was limited per name and address only, so one address could try one password on every account. Names were stored at any length (a body can be 4 MB), and a flood of made-up names could grow the limiter without bound and take every core with password hashing. | A: limit per address too, at most 2 checks at once, bounded memory. Test: R |
| 13 | Stopping a stream ended it for everyone sharing it (streams are shared between viewers of the same file and settings). | T: it stops only when nobody else used it lately |
| 14 | The player never said it was done: FFmpeg ran on for 90 seconds after leaving, or after changing track. | T |
| 15 | FFmpeg's error output was kept in memory without limit; damaged files warn for every frame. | T: the last 16 to 32 KB |
| 16 | Data race on the newest finished segment, between the watcher and requests. | T |
| 17 | A crafted disc image could hang a scan (directories containing themselves, exponential within the depth limit) or panic it (negative sizes, lengths wrapping on 32-bit systems, which are release targets). The package had no tests. | D: limits and first tests, with a fuzz target. 30 s clean |
| 18 | One DVD in 256 opened as "expired": a random session id could start with `bd`, which routes to Blu-ray sessions. | T |
| 19 | The server exited as soon as shutdown began, closing the database under requests still running. | T |
| 20 | One slow disc step stalled every other disc: looking up a session held the manager's lock while waiting for that session. Sessions could also close under a running step, and the number of open discs had no limit. | T: separate locks, at most 16 open. A DVD session waits for its running step before closing; a Blu-ray session doesn't yet, as the BD-J work changed that function (a read then fails harmlessly) |
| 21 | Every rescan (each 6 hours) saved every show, changing its pictures' addresses, so every browser fetched all posters again. | A. Test: R |
| 22 | Items without a poster were read again and looked up online on every scan, and rebuilt from nothing, losing hand edits. Episodes the online service doesn't know were re-probed on every scan. | A |
| 23 | Scans of all libraries ran at once after each 6-hour tick. | A: one at a time |
| 24 | Podcasts: each episode of each feed scanned the whole library (quadratic, every 2 hours). A renamed feed failed for ever. One bad entry stopped the refresh. | A: one read of known episodes |
| 25 | `play?audio=-1` panicked. The surround choice wasn't part of a stream's identity, so whoever came first decided for everyone. | A |
| 26 | File handle leak in the disc probe, and archives stayed open for ever. Both lock files on Windows (no renaming or deleting a book after reading it). | A |
| 27 | Release jobs could all write to the repository, though only packaging publishes. | C: permissions per job |
| 28 | Library pages loaded a library's whole list and that many cards, and flickered to a spinner every 1.5 s during a scan. | W: 120 at a time, no flicker. Tested with 5000 items |
| 29 | Web files took their types from the machine: a Windows registry entry can make `.js` `text/plain`, which with `nosniff` means a blank page. `index.html` could be cached across an update. | W. Test: W |
| 30 | The name of another media player appeared in code, docs and the UI. | C |

### Low

| # | Finding | Fix |
|---|---|---|
| 31 | Non-public ranges missing from the internet-only check: shared address space (which is also how mesh VPNs reach all of the owner's devices), benchmarking ranges, NAT64, `0.0.0.0/8`, `240.0.0.0/4`. | A. Test: A |
| 32 | File locations went to every account, which can include a person's name. | A: administrators only |
| 33 | An agent's API key header followed redirects to other hosts. | A |
| 34 | A database path with `#` or `%` opened another file. | A. Test: R |
| 35 | Disc thumbnails: any number was accepted, each making one more picture on disk; protected discs were read for them. Missing titles answered 500. | A |
| 36 | Pictures were marked `public` to caches, though behind a sign-in. | A |
| 37 | The image downloader stored whatever came back and kept a name part from the address (`a.jpg:x` is a hidden stream on Windows), and two requests at once shared a temporary file. | A |
| 38 | Logout ended only the first of the tokens a request carried; expired sessions stayed for ever; an escaped `%74oken=` got past log redaction; scan flags stayed on after a crash; a NFO was replaced in place, not atomically; names were cut inside a character. | A |
| 39 | Settings handed to FFmpeg weren't checked, showing up later as every transcode failing. | C. Test: C |
| 40 | Hardware encoder tests (up to 5 processes that look for a GPU and fail) ran at every start, and the slow dummy password hash on every command. | C: when needed |
| 41 | `ffprobe` ran again on every track change. | T: the last 64 files are remembered |
| 42 | Dead code and helpers the language now has; lists loaded each disc's titles for nothing (33.9 KB → 8.6 KB for 6 discs). | A, C, W |
| 43 | The systemd unit could be tighter. | C |

### Checked, and found sound

Role checks on every route (now a test that covers new routes too). Path traversal in EPUB, theme,
log, still and subtitle routes. SQL: no value is built into a query from input, and sort orders are
fixed lists. CSRF rules, and CORS without credentials. Cookie flags. Theme ZIP and logo validation.
Passwords never logged. HLS stream ids can't be guessed. Copy-protected discs are refused and never
decrypted. Update signing and the protected folders. Panics in background goroutines: none can
reach parsers without `guard`.

## Left, and why

These are decisions or larger changes, written up rather than done, so the parallel work merges
easily. **The first four would be my first choices.**

| # | What | Why it matters | Why it was left |
|---|---|---|---|
| a | **Store session tokens hashed** (SHA-256) in the database | A copied database or backup holds working logins for every account | Touches sign-in, which the remote access work is changing |
| b | **Check the `Host` header, and a longer minimum password** (now 4) | A web page can use DNS rebinding to reach this server from a visitor's browser: it can't read a signed-in session, but it can finish setup on a new server and use password-free profiles. Important once reachable from outside | Remote access decides what the allowed hosts and password rules are. `X-Forwarded-For` trust belongs there too: behind a proxy that doesn't add it, everyone looks local, and behind one that does, everyone shares one sign-in limit |
| c | **Should "save next to media" default to off?** | It writes into people's media folders on the first scan, unasked. It is careful now (finding 2), but a first run that changes nothing is easier to trust | The owner's decision: it is a behaviour people may rely on |
| d | **Pin downloads in the release build** | `build-release.sh` bundles the "latest" FFmpeg with no checksum, and third-party actions (release upload, FreeBSD VM, Docker) are pinned by tag, not by commit, in the job that holds the signing key | Pinning needs a routine for updating them. It isn't a one-off change |
| e | Keep the token out of media addresses | `?token=` lives in `localStorage` and in addresses. The cookie could do this for the web client, but a second server on the same host shares cookies, which is why it was added | A design change for the client and native wrappers |
| f | Separate transcodes per viewer, or a smarter shared one | Two viewers of one file at different positions keep restarting FFmpeg for each other | A trade-off against CPU: sharing is what you want when they are together |
| g | Filter restricted accounts' lists faster | A kid's list scans the library (45 ms here on 25000 items) as the rules can't use an index. Fine for now | No clear gain without a bigger change |
| h | Paged lists for shows and albums, compressed JSON | Only films and episodes page so far. 400 shows are 146 KB | Worth it for remote use; follows the UI work |
| i | Bound the image cache | It grows with every picture ever seen | Needs a policy (what may go) |
| j | Disc streams on leaving | The disc player closes its session, but its FFmpeg runs on up to 90 seconds | Streams are shared and the session doesn't own them; the idle stop covers it |
| k | Per-day log size limit | One day of debug logging can grow without limit (files are capped, not their number) | Only with debug on, by the owner |
| l | Update `golang.org/x/net` (0.60 to 0.62) and others. `staticcheck` style notes in `wittybluray` and `wms-setup` | No reachable vulnerability. Routine | Not an audit fix; the Blu-ray module is being worked on |
| m | `.tools/...` is looked in for FFmpeg, relative to the current folder | A leftover for development; harmless unless started from a folder someone else can write to | Removing it changes how the project is run during development |

Administrators remain trusted, by design: they can give FFmpeg raw options (Expert mode can write any
file the server can), point agents at the local network, and choose folders for logs.

## Not verified

- **The race detector** has not run here: it needs a C compiler this machine doesn't have. It is now
  in CI, so its first run there is its first real test.
- **macOS, FreeBSD, Linux and ARM** were built, not run. The tests ran on Windows only.
- **Online services** (TMDB, TheTVDB, TVmaze, Wikidata, AniList) and a real podcast feed were not
  called: the test server had them off, and the checks that refuse local addresses were tested
  instead. The podcast proxy was tested against a local server.
- **Hardware encoders**, Blu-ray menus (a Blu-ray title and a DVD were played), and audio in a real
  browser (its stream was fetched). Old browsers and TVs.
- `clients/`, `apps/wittyreader`, `cmd/wms-tray` and `cmd/wms-setup` were skimmed for dangerous
  calls only. The update engine and its tests were reviewed lightly: it was written days ago with
  an end-to-end test.

## Merging

This branch merges into `master` without conflicts, and the merged code builds and passes its tests
(checked at `48250ea`). **BD-J (`48250ea`) arrived after this audit and has not been audited**: it
runs Java code from disc images on the server, so it deserves its own review. The other sessions will likely touch `internal/db/db.go`
too: if two branches add a migration, both go at the end of the list, each with its own number.
`TestEveryRouteChecksWhoIsAsking` will fail for a new public route until it is added to
`publicRoutes`, which is on purpose.
