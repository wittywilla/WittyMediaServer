# Security model

How WittyMediaServer is meant to hold up, and how that gets checked. To report a problem, see
[SECURITY.md](../SECURITY.md).

This describes a design and a way of testing it. It isn't a promise that nothing is wrong, and it isn't an
independent audit. For what to do about exposure, read [REMOTE.md](REMOTE.md).

## Who it's defended against

| Who | What they can do | What should stop them |
|---|---|---|
| **Stranger on the internet** | Reach whatever the owner opened | Nothing answers from outside until Remote Access is on, and then only over HTTPS or a VPN. Sign-in limits, optional two-factor sign-in, connection and request limits |
| **Stranger on the home network** | Reach every port, and get there before the owner on a fresh server | Setup takes a code only the machine shows, away from the server's own desktop, and so does restoring a backup on a new server, as a backup brings its own accounts. Password-free profiles give only what a Kids or Guest profile may see |
| **A web page in the browser of someone at home** | Send requests from inside the home, with DNS rebinding to get round names | A page elsewhere never counts as being at home, whatever it sends. Cookies aren't honoured across origins, and a `Host` that isn't a local name isn't trusted |
| **Kids, Guest or limited accounts** | Ask for anything by number or name, and upload where allowed | Limits are checked on every route that returns a title, a picture, a stream, a count or a comment. Hidden titles answer "not found". Each route's rule is listed, and a test fails for a new route with none |
| **Someone who can put files in a library** | Hostile media, sidecars, links, pipes, archives, disc images | Parsers are bounded, fuzzed and time-limited. Files are served through a handle that can't leave the library. FFmpeg opens library files only as real media formats, with no network |
| **A host behind a feed or playlist** | Answer anything, redirect anywhere, change its answer between requests | The internet-only client checks each connection as it is made, including the machine's own addresses. A playlist from the internet stays on the internet. FFmpeg never reads an internet stream by itself |
| **A stolen administrator session** | Everything in Settings | It's not meant to reach the machine: no running programs, no reading arbitrary files, no second administrator or reset without the password and code, and no taking away the backups or a library's originals without them either. Custom cores need a switch set on the machine |
| **A helper, game core or module** | Run with a hostile file as input | Helpers and cores run in a sandbox, as far as the system allows: no writes outside their folder, no sockets unless needed, no changing who may read a file. [What each system really gives](#what-a-helper-is-held-to) differs, and Settings, Modules says so wherever it's short of this |
| **Whoever runs a store an admin added** | Answer anything, change their list, name another key, list as many pictures as a list can hold | Its key is pinned and its list refused if it changes. Its words are cleaned, its pictures checked and bounded, and it can't reach a module it isn't the chosen source of. A store that's off isn't asked anything and installs nothing |
| **Whoever controls what GitHub answers** | Offer old or altered releases | Everything installed is signed, the signature covers each file's name and hash, older versions aren't offered, and a withdrawn version stays withdrawn |

**Outside the model:** someone with the server's own operating system account, or physical access; an administrator who
knows the password and means harm; FFmpeg's own decoders, whose defence is keeping FFmpeg current; and what the owner
chooses to trust, such as a reverse proxy listed as trusted, a fork of a module, or a custom core.

## The rules everything follows

- **Off until the owner turns it on.** Remote access, port mapping, forks, custom cores, BD-J and anything contacting a third party
  all start off.
- **Encrypted is refused, never decrypted.** Protected discs, broadcasts and game files are detected and refused, and no
  extension point exists to change that. For HLS, a key is refused before FFmpeg sees the playlist.
- **Things that can't be undone ask for the password again.** So do downloading, deleting or keeping fewer backups, starting to
  replace a library's originals, adding or removing an administrator, reset, moving a module to a fork or an older version, and
  having this server talk to an address on its own network for the first time, as what comes back says what's listening there.
- **What a web account can set can't name a program.** Paths to programs live in the config file or the environment.
- **A file's name, a feed's answer and a stream's address are data.** None is ever handed to a shell, read as an FFmpeg
  option, or shown to people who don't need it.
- **Secrets stay out of logs, addresses and error messages.** Support bundles are redacted again on the way out.

## What a helper is held to

A module's helper is someone's code handling a hostile file. How much the system can do about that differs, so here it is, and
**Settings, Modules** and the store say the same wherever it's short of the first row.

| System | What a helper can't do, unless it's a module allowed to | What it can still do | Modules from other publishers |
|---|---|---|---|
| **Linux** | Change any file, reach the network, change who owns a file, reach into another process. Needs Landlock, from kernel 5.13; the page says if it's missing | Read what the server's account can read | Allowed, and don't start unless every limit holds |
| **Windows** | Change this account's files, start other programs | Read what the account can read, and **use the network** | Allowed, under those limits |
| **macOS** | Change files, reach the network, start programs, signal other programs, by Apple's `sandbox-exec`, which is tried before it's relied on. Written and compiled, and **not yet run on a real Mac** | Read what the account can read | Refused |
| **FreeBSD, as an ordinary account** | Start programs, open any new socket, write into any file, or raise those limits. These are limits, not a sandbox, because FreeBSD's jails take root and Capsicum takes away opening files by name | Read what the account can read, **delete and rename this account's files, and signal its other programs** | Refused |
| **FreeBSD, as root** | Reach the network, or see other programs, from inside a jail | **Change any file, and start programs**, being root. Run it as an ordinary account if you can | Refused |

## How it's checked

- **The route table:** [SECURITY-ROUTES.md](SECURITY-ROUTES.md) lists every HTTP route and the WebSocket: who may call it and what
  it validates. Tests walk the routes: each must refuse someone not signed in and, where it says so, anyone but an
  administrator. A route open to any signed-in account fails the build until its rule has been written down.
- **A test for each fixed hole.** Each was written to fail against the unfixed code first, and is kept.
- **Fuzzing:** every parser of untrusted input has a target, run for minutes at a time: names, NFO and JSON sidecars, EPUB,
  feeds, playlists, ID3, lyrics, disc images and programs, archives and backups, game lists and databases, themes, log lines, pictures, subtitles,
  HLS playlists, saved views, match rules, channel definitions, checksum lists and packages. Those where a slow answer is the danger fail on time too.
- **Live attacks** against test servers, in Linux, with FFmpeg: for instance, a playlist that gains a key while the real FFmpeg
  reads it, and a link swapped back and forth while a file is being served. A second pass did the same to what this release added:
  hostile stores, backups, support bundles, the task API and game cores, with a limit tried from inside a helper for each thing the
  page says about FreeBSD, in a real FreeBSD.
- **Every build:** `go vet`, the tests, the race detector, `govulncheck` and `npm audit`, which stop the build on a
  vulnerability the code can reach. `go mod verify`, dependency review on pull requests, and a weekly run for what was published since.
- **The build itself:** actions and Docker base images are pinned by digest, FFmpeg downloads by hash. npm code runs in its own job,
  away from the Go build, and signing happens on a clean machine that holds nothing else.

## What's where

| Area | Where |
|---|---|
| Choosing how to reach a server safely | [REMOTE.md](REMOTE.md) |
| Signing, keys and taking back a release | [PUBLISHING.md](PUBLISHING.md) |
| What FFmpeg is allowed to do | [TRANSCODING.md](TRANSCODING.md#expert) |
| Modules, their sandbox and forks | [MODULES.md](MODULES.md) |
| Custom game cores | [GAMING.md](GAMING.md) |
| Encrypted channels | [CHANNELS.md](CHANNELS.md) |
