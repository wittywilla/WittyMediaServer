# Security review

A full review of WittyMediaServer 0.4.0-alpha, October 2026: reading all of the server, updater,
installer, parsers and CI, attacking test servers on this machine, and fuzzing every parser of
untrusted files. Everything marked fixed has a regression test unless it says otherwise.

To report a problem, see [SECURITY.md](../SECURITY.md).

**Severity** says what an attacker gets, for this kind of home server:

- **Critical**: takes over the server, or crashes it for good, with little or no access.
- **High**: reads or changes files or accounts, or stops the server, starting from a low level of access.
- **Medium**: gets around a limit, or needs an unusual position.
- **Low**: small leaks and hardening.

## Findings

| # | Severity | Finding | Commit |
|---|---|---|---|
| 1 | Critical | Anyone could claim a fresh server, including a web page using DNS rebinding | `4a1a668` |
| 2 | Critical | A crafted Blu-ray image crashed the whole server | `186af3b` |
| 3 | High | A podcast feed could run scripts on the server's own site | `7512d45` |
| 4 | High | A sidecar file could copy any server file beside the media, or fetch internal addresses | `afbb714` |
| 5 | High | Links in a media folder gave read and overwrite access to other files | `afbb714` |
| 6 | High | The admin's FFmpeg settings could read and write any file | `5ce69f8` |
| 7 | High | Anyone signed in could exhaust the machine with transcodes | `e48bdb4` |
| 8 | High | The signing key was exposed to everything that ran during a release build | `e54eed3` |
| 9 | High | 11 reachable vulnerabilities in the Go standard library | `d588cd6` |
| 10 | High | Sign-in limits failed under parallel guesses, and sign-in could fill memory | `4a1a668` |
| 11 | Medium | A crafted disc image could hang a scan for ever | `186af3b` |
| 12 | Medium | A limited adult could create an unlimited profile and sign in as it | `5ce69f8` |
| 13 | Medium | Restricted accounts could reach hidden titles' artwork and learn what exists | `5ce69f8` |
| 14 | Medium | Session tokens were stored as they are | `4a1a668` |
| 15 | Medium | A token in an address opened the whole API | `4a1a668` |
| 16 | Medium | Other accounts could see server paths and private feed addresses | `5ce69f8`, `fea45b3` |
| 17 | Medium | Requests and books could use up memory, CPU or disk | `e48bdb4` |
| 18 | Medium | The database was readable by all accounts on a shared machine | `fcb7463`, `e54eed3` |
| 19 | Medium | CI gave every job write access and ran actions by moving tag | `e54eed3` |
| 20 | Low | Weak passwords and smaller issues | `4a1a668`, `5ce69f8`, `e48bdb4`, `e54eed3` |

### 1. Claiming a fresh server

Until the first account exists, `POST /api/setup` made whoever called it the administrator. Anyone
who reached a server left waiting could take it. Worse, a web page could do it from a visitor's
browser: a domain the attacker controls is pointed at the server's address (DNS rebinding), and to
the browser that is the page's own site. Proven against a test server with a request naming another
host, which got back an admin session.

**Fix.** From outside this network, setup needs a code the server writes to its log at start. A
request whose `Host` isn't an IP address or a name only a local network can resolve never counts as
local, which also protects password-free profiles, the profile list and the desktop-only actions.

### 2. Blu-ray that crashes the server

A disc's program jumping to its own first-play title recursed without limit until the Go stack
overflowed. That is a fatal error, not a panic: no `recover` catches it, so one image in a library
ended the server for everyone. Proven by a test that crashed before the fix.

**Fix.** Programs running inside one another are limited to a small depth.

### 3. Script from a podcast feed

The stream proxy passed on the `Content-Type` of a feed's enclosure. An address answering with an
HTML page was served as a page on the server's own site, so opening `/api/items/N/stream` ran the
feed's script as whoever was signed in. Library files with unknown extensions had the same problem:
their type was guessed from their first bytes.

**Fix.** Every `/api/` response carries a script-free sandbox policy, whatever it holds. Proxied
episodes keep only audio and video types, and library files get their type from a fixed list. The
app's own policy no longer lets it talk to other sites.

### 4. Sidecar files reading the machine

A `metadata.json` beside the media names its artwork, and saving metadata read that reference with
no limit: a local path or a `cache:` name with `..` copied any server file (the database, say) next
to the media. A web address was fetched by a client with no limits on where it may connect, and the
answer stored there, so internal services could be read. On Windows, a network path sent the
server's credentials to that host.

**Fix.** Only pictures are read, and only from the internet, the artwork cache or inside a library.
Sidecar references must be web addresses or relative to the sidecar. All catalogue clients use the
internet-only client, which now also refuses CGNAT, reserved, benchmark, NAT64 and 6to4 ranges.

### 5. Links in a media folder

A link named `.pdf`, `.iso` or `.mp3` was indexed and served, so anyone who can write to a shared
folder could read any file the server can. A `movie.nfo` link was copied to `movie.nfo.bak` (handing
over its target) and written through (overwriting it).

**Fix.** Scans skip links that leave their library, plus pipes and devices, and serving checks again
so a link swapped in later doesn't work. Metadata is written to a new file and renamed into place.

### 6. FFmpeg settings

The expert fields passed any option to FFmpeg, which can read and write any file. An administrator's
web account is not meant to have the machine's file system, and an attacker who gets one would then
have a way to run code.

**Fix.** Only listed encoder options with plain values are accepted, and values saved earlier are
checked again before use. Bitrates, presets and the audio codec are validated too. The list is
documented in [TRANSCODING.md](TRANSCODING.md#expert).

### 7. Transcode exhaustion

`maxTranscodes` was read but never enforced. Each resolution and track is a stream of its own, so
anyone signed in, a Kids profile included, could start an FFmpeg process per variant of a title.

**Fix.** The limit holds. The least recently used stream gives way, and when all are in use the
client gets a 503 and retries. In the live test, 8 parallel requests gave 3 streams and 5 refusals.
Streams, disc sessions, `ffprobe` and audio transcodes are bounded too, and a viewer can't raise the
admin's resolution limit from the address.

### 8. The signing key in the build

The key was in the environment of the step that runs `npm ci` and every build script, so a single
bad package could have stolen the key that makes updates trusted.

**Fix.** Signing is a separate job on a clean machine that builds only the signing tool and sees
only the checksum list. Publishing is a third job that runs no project code. npm runs without
install scripts and never falls back to ignoring the lock file. See *What's left* for the owner's
step that finishes this.

### 9. Go standard library

`govulncheck` found 11 reachable issues in Go 1.27.0, in HTTP/2, `net/http`, `net/textproto`,
`crypto/tls` and `crypto/x509`. Builds now use Go 1.27.3, and a weekly workflow checks again.

### 10. Sign-in limits

A failure was counted after its password check, so guesses sent at once all passed. Attempts now
count when they start: 30 parallel guesses in the live test got 5 checks. One address now also has a
limit across names, and password checks run a few at a time. A 4 MB name became a key kept in memory
per attempt; requests are now capped at 16 KB and the map is bounded.

### 11. Disc image that never finishes

Directories that list each other were walked once per path, which is exponential: a 128 KB image
would never finish. Each directory is now read once, within limits on files, entries and bytes.
Negative or huge sizes in an image could panic or ask for gigabytes, and on 32-bit systems a
descriptor length could wrap past its bounds check. All fixed.

### 12. Getting around your own limits

An adult with limits could create a Kids profile with no limits and a password they know. A limited
adult no longer manages profiles.

### 13. Hidden titles

Show artwork had no access check, progress could be saved on hidden titles (showing they exist), and
the genre and network lists named them. The SQL filter ignored a show's genres although the
single-item check used them. All checked now, and a test walks every admin route for each role.

### 14 and 15. Session tokens

The database held tokens as they are, so a backup was a set of working sign-ins. They are stored
hashed now, and existing sessions are converted at start. A `?token=` worked on every route: it now
works only on media routes, and only for reading, so a token leaked from a media link can't change
anything or read settings.

A session token in a media address still works, for existing clients. An HLS playlist opened that way
names its segments with a **stream token** that opens only that stream, only for reading, for up to
6 hours, and ends with its session, so the session's own token never appears in a playlist.

### 16. Things other accounts could see

Item, show and library responses gave every account full server paths (often including an account
name), poster and backdrop paths included. The podcast list gave every account all subscriptions and
their addresses, and a private feed's address is a password. Scan progress named files. Now only for
administrators.

### 17. Other limits

Added: 32 MB and 2 minutes for any request body, 64 KB of headers, a 10-minute limit on a listener
who stops reading, thumbnails only for titles and chapters that exist (any number used to make a new
file), a window on subtitle reads, 64 MB of text per request from a book (a small crafted EPUB could
unpack gigabytes each time), a pixel limit for covers, and caps on notes, comic pages and feed
episodes. A negative track number in an address panicked the handler; missing tracks are now ignored.

### 18. File permissions

`wms.db` holds password hashes, API keys and sessions, and was world-readable on Linux. It is now
`0600`. On Windows, `install.ps1` put data in a `ProgramData` folder every account could read and add
files to; it now locks that folder down.

### 19. CI

All release jobs had write access, third-party actions ran by moving tag, and `checkout` kept a
token on disk. Each job now asks only for what it needs, actions are pinned by commit (Dependabot
proposes updates), and FFmpeg's rolling builds are checked against their published checksums.

### 20. Smaller items

New full-account passwords need 8 characters (PINs stay at 4, and existing passwords keep working).
Expired sessions are removed. Artwork is cached privately. Podcast episode names can't replace files
or use Windows device names, and neither can theme ids. Folder names with line breaks can't add
entries to FFmpeg's lists. Library folders must be full paths. FFmpeg in the project's `.tools` folder
is used only in a source checkout. `wms-setup` runs system tools by full path, because `cmd.exe` looked
in the temp folder first. The systemd unit and Kubernetes deployment run with fewer privileges.

## Checked and found sound

- **Passwords and tokens:** bcrypt, 256-bit tokens from the system's random source, no session
  fixation, password changes end other sessions.
- **CSRF and CORS:** state changes from other sites are refused, and cookies are never honoured across
  origins.
- **SQL:** all values are parameters, and sort orders come from fixed lists.
- **Paths:** traversal attempts on theme, log, artwork, still and EPUB routes all failed, including
  Windows forms (`..\`, drive letters, network paths, device names).
- **XML:** `encoding/xml` reads no external entities and expands no entity bombs. A test keeps it so.
- **XSS:** the web UI uses no raw HTML, EPUB chapters render in a sandboxed frame under a strict
  policy, and theme files are inert.
- **Update chain:** Ed25519 over the exact checksum list, the package name inside the signed list ties
  it to its version so a downgrade can't be offered, downloads only from GitHub over HTTPS, zip-slip
  and links refused, rollback when a swap fails. The test override is left out of release builds.
- **FFmpeg input:** with the bundled build, a playlist or concat script disguised as a movie is
  refused, and `-protocol_whitelist file,pipe` blocks the network.
- **Windows DLLs:** the Go programs load system DLLs from the system folder only.
- **Desktop client:** context isolation, sandbox, no Node in pages, navigation held to the chosen server.
- **Secrets:** none in the git history. `npm audit` finds nothing.

## How it was tested

- **Fuzzing:** `FuzzOpen` (disc images, 8 million runs), `FuzzNav` (DVD and Blu-ray programs),
  `FuzzParsers` (IFO, SPU, navigation packs, MPLS, CLPI, index, IG, PG), `FuzzReadEPUB` and
  `FuzzParseFeed`, several minutes each, with no crash after the fixes. The seeds run with `go test`,
  and `go test -fuzz=FuzzOpen` in each package explores further.
- **Live attacks:** 74 scripted checks against a server on `127.0.0.1` with throwaway accounts.
- **Regression:** EPUB, comic, PDF, HLS video, DVD menus and thumbnails were opened in a browser with
  the tighter policies, with no policy errors.

## What's left

### Needs the owner

1. **Protect the signing key.** The signing job names an environment called `release`. Moving the
   `WMS_SIGNING_KEY` secret into it, with yourself as required reviewer, means no workflow can use
   the key without your approval. [PUBLISHING.md](PUBLISHING.md) has the steps.
2. **Turn on private reporting.** *Settings → Security → Private vulnerability reporting*, which
   `SECURITY.md` relies on.
3. **Release the modules.** The disc modules have fixes, and programs using them through `go install`
   get them only when new versions are tagged.
4. **The `install.ps1` service runs as SYSTEM**, far more than a media server needs. A less privileged
   account can't read media in other users' folders or on network shares, so changing it would break
   those setups. Your decision.
5. **Choices made here that you may want to change:**
   - Links leading out of a library are refused, which breaks setups that keep media elsewhere and
     link it in. Linking the folder as a library works.
   - `maxTranscodes` stays at 4, so a fifth simultaneous viewer needing a transcode is told the server
     is busy.
   - The `install.ps1` firewall rule no longer covers networks Windows calls public.
   - Adult passwords need 8 characters where 4 used to do.

### For the remote-access work

These matter once a server is reachable from outside, and that work is the right place for them:

- **HTTPS is not built in**, so without a proxy, passwords and tokens cross the network unencrypted.
- **Forwarded headers aren't trusted**, which is the safe direction but means everyone behind a proxy
  shares one address: the per-name limit is shared, and the per-address limit is off there.
- **A proxy that adds no `X-Forwarded-For` and passes a local `Host`** makes outside visitors look
  local. The review could only document this; the fix is an explicit list of trusted proxies.
- **Per-name limits don't span addresses**, so a guesser with many addresses gets more tries.
- **There is no list of active sessions** to review or end.

### Accepted for now

- **The token is in `localStorage` and in the web UI's media addresses.** A script running on the site
  could read it, and addresses appear in browser history. They exist because cookies are shared between
  servers on one host (ports don't separate them), so a cookie alone can break when two run side by
  side. The script policy, the route limits and log redaction reduce the risk.
- **Artwork may load from any `https:` address**, needed for podcast search results. Theme CSS could
  use this to call out, but only an administrator installs themes.
- **API keys are stored as they are** in the database, protected by file permissions. Encrypting them
  would need a key kept somewhere on the same machine.
- **An administrator can pick any folder** as a library or for logs, by design. On Windows, a network
  path contacts that host with the server's credentials.
- **Rolling FFmpeg builds aren't pinned**, and only those with published SHA-256 checksums are verified.
  Docker base images are pinned by tag, not digest.
- **The mobile app allows plain HTTP**, because home servers use it.
- **`/api/status` shows the version** to anyone, which the app uses at start.
- **No limit on open connections, or on total artwork cache size.** A reverse proxy can limit the first.
- **`golang.org/x/crypto`** has one advisory with no fix yet (GO-2026-5932). `govulncheck` finds no path
  to it from this code.

### Not tested

- The server ran only on Windows here. Linux and macOS got cross-builds and CI-style checks, not a run.
- The new CI workflows were checked for correctness but can only really run on GitHub.
- `wms-setup` and `install.ps1` were reviewed and built, not run.
- The setup code field in the wizard is tested through the API, not seen in a browser.
- FFmpeg's own decoders are outside this review, and hostile media still reaches them. Keeping FFmpeg
  current is the defence.
