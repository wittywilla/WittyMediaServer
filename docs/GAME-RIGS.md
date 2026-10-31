# Game rigs: your own gaming PC as a source of games

Next to emulated consoles, a person's own gaming PC can be a source of games. The PC runs **Sunshine**, the
open-source game stream host. They play on the couch through **Moonlight**, the open-source client. WittyMediaServer
finds and pairs the PC, lists its games in the library with everyone's permissions applied, wakes it up, and gets
each device as close to one press as that device allows.

Two modules do this. **WittySunshine** (`modules/wittysunshine`) is the host side: finding, pairing with and managing
a rig. **WittyMoonlight** (`modules/wittymoonlight`) is the client side: what each device can do, handing off to
Moonlight, and browser play. The server's part is `internal/server/api_rigs.go` and `api_rigs_browser.go`. The web
pages are `web/src/pages/Rigs.tsx` and `web/src/pages/settings/GameRigs.tsx`. The API is in
[CLIENT-API.md](CLIENT-API.md#game-rigs).

**How much of this has really run:** see [What has been tested](#what-has-been-tested). In short, the server and its pages have
run against a real Sunshine, as far as adding, pairing, listing and starting. Browser play's program has never been run.
No game has been streamed, and no real Moonlight app or real device has been tried.

## What each device can do

Moonlight has no link or URL scheme on any platform (checked in each client's source on 2026-10-31). So a web page can
never open it: only a native app can, and only where the platform allows one app to start another.

| Device | Through | What happens | Needs | Checked |
|---|---|---|---|---|
| **Windows, Linux, Mac: WittyMediaClient desktop app** | Moonlight's command line (`moonlight stream <PC> "<game>"`) | **One press** | Moonlight installed, and paired once | Its checks are unit tested with a stand-in program. Not tried with Moonlight |
| **Windows, Mac, Linux: a browser** | Browser play, if set up. Otherwise a command to copy | **One press** with browser play. Otherwise paste a command, or open Moonlight yourself | Browser play, or Moonlight installed and paired once | Plan tested. Browser play never run |
| **Android phone or tablet: WittyMediaClient app** | Moonlight's exported shortcut activity, by explicit intent | **One press** | Moonlight installed, and paired once | Written, never compiled: this machine has no Android tools |
| **Android TV, Google TV, Fire TV: WittyMediaClient app** | The same | **One press** with the remote | Moonlight installed, and paired once | Written, never compiled |
| **Android: a browser** | Browser play, if set up | **One press** with browser play. Otherwise open Moonlight yourself. Chrome won't start an activity like this from a web page | Browser play, or Moonlight | Plan tested. Browser play never run |
| **iPhone, iPad: a browser or the app** | Browser play, if set up, in Safari | **One press** with browser play. Otherwise open Moonlight yourself | Browser play, or Moonlight | Plan tested. Browser play never run on Safari |
| **Apple TV: WittyMediaClient app** | Nothing can start Moonlight. The app has a **Start on the PC** button, then you open Moonlight and press Resume | **Three steps**: the game is already running when you get there | Moonlight installed, and paired once | Written, never compiled. The start itself ran against a real Sunshine |
| **Mac: WittyMediaClient Apple app** | Moonlight's command line | **One press, if macOS allows it.** The app is sandboxed, which may stop it starting another program, and it then shows the steps | Moonlight installed, and paired once | Written, never compiled |
| **Chromebook** | Browser play, or the Android Moonlight app by hand | One press with browser play | Browser play, or Moonlight | Plan tested |
| **Older TV browsers, LG webOS, Samsung Tizen** | Moonlight has community versions for these, which you open by hand | Not from this server | Moonlight from its community port | Not tried. Browser play is not expected to work on them |
| **Xbox, PlayStation, Switch, Roku** | Moonlight has community versions for some of these, opened by hand | **Doesn't work from this server** | | Not tried |

**Pairing a device** happens once per device and is Moonlight's own: Moonlight shows a PIN, and it's typed into
Sunshine on that PC, or by an admin in Settings, Game rigs. WittyMediaServer can't do this for a device, because
Moonlight keeps its own certificate.

Controllers in browser play need HTTPS, as browsers hide the Gamepad API from plain http pages. On plain http,
browser play is keyboard and mouse only. Moonlight apps have no such limit, and are smoother: browser play adds a
second hop.

## Design decisions

**A rig's games are ordinary items in a library of their own** (`type: games`, `console: rig`). Every access rule then
applies with no new code: library limits, age limits, per-title allow and block lists, in Home, search and every other
list. Hidden titles answer "not found" and a Kids profile doesn't learn that a rig exists. A PC's games have no ratings, so
Kids and Guest profiles (which don't allow unrated titles) see nothing until an admin gives them a game in Profiles.
The alternative, separate tables with separate permission checks, would have been a second permission system to get
wrong.

**Items carry a made-up path** (`rig://<rig>/<app>`). Nothing can mistake it for a file, and a library marked as a rig's is
never scanned, or edited as a folder library.

**Moonlight is handed to, never embedded.** It has its own certificate on each device, its own pairing and its own list
of PCs. This is what keeps the licences apart, and is the only way anything works on Apple devices.

**Sunshine's sign-in is never stored.** It can run any command on that PC, because that's what its app list is. Nothing
needs it except typing a PIN for someone's Moonlight app, so an admin types it each time, it goes once to the pinned
certificate, and that's all. The first pairing can't use it at all, since there's no pinned certificate yet: the PIN is
typed at the PC, where Sunshine was just installed anyway. Art and the app list come over the client protocol, which needs
no sign-in.

**Desktop apps start hidden.** Sunshine's "Desktop" app hands over the whole computer. Anything that looks like that stays
out of the library until an admin shows it. Game streams also give keyboard and mouse, which can often get out of a game to
the desktop. Sunshine has no way to limit that, so the settings page says so plainly.

**A paired Moonlight app is outside this server's permissions.** Sunshine has no per-client limits on apps here. So a
device that has paired can run anything the PC offers, whatever its owner's profile says, which is why pairing a device takes
an admin or someone at the PC. Only browser play is limited by game.

**Browser play is optional and is a separate program.** Writing a Moonlight client into this server's Go would mean
reimplementing a large protocol, with no benefit over an existing, maintained one. [moonlight-web-stream](https://github.com/MrCreativ3001/moonlight-web-stream)
is the one that fits: it runs on a server and reaches the PC over the network, relaying frames without re-encoding. Another project,
[moonlight-web](https://github.com/linckosz/moonlight-web), is built to be installed on the gaming PC itself, which is a different design.

**The bridge's own permissions aren't enough, so a proxy stands in front of it.** From reading its source: its roles can't
limit by PC or by app, a user can launch any app ID including Desktop, a launch on a busy PC becomes a resume of whatever's
running, and it trusts its user header from anyone who reaches its port. So:

- It binds to the loopback address only.
- The proxy is the only way in, and a person arrives with a one-time ticket from the API, good for two minutes, for one game on
  one rig. It becomes a session for that alone.
- Only what the stream page needs is passed on: not its host list, admin pages, pairing, user or role changes.
- A stream request is read before it's passed on, and must name that rig and game exactly once. Its parser takes the last copy of
  a line, so two are refused rather than guessed at.
- Just before a stream starts, the server checks the account still exists, may still see the game, and isn't about to join
  someone else's session.
- WebSocket transport is switched off in the bridge, as its first message would need checking too. So browser play needs UDP.
- It asks no outside server for anything unless an admin names a STUN server. Otherwise it would ask Google's, and tell every browser to.

**It has a port of its own**, as the bridge's pages are someone else's code. On another origin, a flaw in them can't read this
server's pages or make signed-in calls whose answers it can read. It also means a reverse proxy in front of the server doesn't
cover it. Both it and the bridge start when someone presses Play, and stop after 30 idle minutes, so a server pays nothing
until this is used.

**A status check is only ever a question.** `GET /rigs` costs one request to each PC at most every five seconds. A PC's games
are read at pairing, on request, and at most every ten minutes while someone is looking. Nothing polls in the background, in keeping
with a low-end server.

**Art.** From the PC first. Sunshine answers with one default picture for any app without its own, so a picture that several apps
share counts as none, and the tile shows the game's name. Year, developer and summary come from Wikidata, with the same setting
as scanned games. Box art isn't taken from there, because it's copyrighted.

## Sunshine, from its own source

Read from LizardByte/Sunshine at v2026.914.233613 and run against that same version. Ports move with the base port.

| Port | Protocol | What for |
|---|---|---|
| 47989 (base) | TCP, HTTP | `/serverinfo` and `/pair`, unencrypted. This server uses it to recognise a PC and to pair |
| 47984 (base − 5) | TCP, HTTPS with a client certificate | `/serverinfo`, `/applist`, `/appasset`, `/launch`, `/resume`, `/cancel`. Everything after pairing |
| 47990 (base + 1) | TCP, HTTPS, password | Its web interface and JSON API. **It can run commands.** Only on the local network by default |
| 48010 | TCP | RTSP, while streaming |
| 47998, 47999, 48000 | UDP | Video, control and audio, while streaming |
| 5353 | UDP | mDNS: it announces `_nvstream._tcp` |

- **Pairing:** `/pair` with `phrase=getservercert` holds the connection until the PIN is typed in Sunshine. Both sides then prove
  they know the PIN (AES-128-ECB with a key from SHA-256 of salt and PIN), and the PC signs its secret with its certificate's key.
  Our code is written from the protocol: it uses the same message names as every Moonlight client, and no code from them.
- **Trust:** after pairing, each knows the other by certificate. An unknown client gets `status_code="401"`.
- **Web API:** Basic auth. `GET /api/pin` lists waiting devices, `POST /api/pin` takes `{pairing_id, pin, name}` and only answers once
  the device has finished, or after about ten seconds. The web port doesn't answer other requests in that time. Non-browser clients
  are exempt from its CSRF tokens.
- **Limits of the protocol:** `/launch` needs stream keys, which this server makes and discards, as it never streams. Nothing removes
  this server from a PC's list over this protocol: that's done in Sunshine, on the PC.

**`/launch` from here** exists only for devices that can't be handed a game, such as an Apple TV. It leaves the PC's display
settings alone (`sops=0`). If nobody picks the game up, it stays running until someone ends it.

## Away from home

| Way | The PC is exposed? | Works over Remote Access? | What it needs |
|---|---|---|---|
| **Browser play** | **No.** Only this server is. The rig only ever talks to the server, at home | Mostly. It needs its own TCP port (47900) and UDP ports (47910 to 47920) forwarded to the server, and the server's HTTPS. It isn't offered over plain http from outside. A reverse proxy or tunnel carries the main port only, so browser play needs those ports too, or a private network | |
| **Moonlight on a private network** (Tailscale, ZeroTier) | **No** | Not through this server. Moonlight talks straight to the PC. Put the PC's private address in "Address away from home" and the plan offers it | The private network's software on both. Moonlight's guide says a relayed Tailscale link won't stream |
| **Sunshine's UPnP, or forwarding its ports** | **Yes.** To everyone on the internet. Pairing is still needed to use it, but anyone can knock | Not through this server | Don't, unless you mean to. **Never forward 47990** |

GameStream can't go through this server's Remote Access itself: it needs UDP to the PC, and clients go straight to the PC's address.
This server won't add a rig at a public address without being told twice.

**Waking from outside works**, as the signal goes out from this server, on the rig's own network.

## Security review

Reviewed as an attacker would, since this reaches a computer that may hold everything.

| Attack | What stops it | Tested |
|---|---|---|
| A viewer makes the server connect somewhere (SSRF) | No route takes an address but admin ones. Viewers only ever trigger requests to registered rigs, on fixed paths | Yes: every admin route refused for every kind of account |
| An admin's request is tricked into reaching a metadata service or link-local address | `CleanAddress` takes a name or IP and nothing else. `Classify` refuses link-local, multicast, unspecified, and IPv4-mapped forms of them. The policy is checked again while connecting, so DNS rebinding is caught, and proxies from the environment aren't used | Yes |
| A public address by accident | Asks first, with the reason | Yes |
| Someone on the network poses as the PC | The certificate is pinned at pairing and compared byte for byte on every connection. Without a match, nothing is sent: not this server's questions, not Sunshine's sign-in | Yes |
| Someone in the middle of pairing swaps the certificate | The PC's secret must be signed by the certificate it offered | Yes |
| A listener saves the pairing messages, and tries all 10,000 PINs | **Not stopped.** It's the protocol's limit, shared by every Moonlight client. It's why the guide says to pair at home | Not applicable |
| A fake answer from anything on the network to discovery | mDNS only says where to ask next. Only private addresses are kept, only a real reply counts, and a paired rig only "moves" if it answers with its pinned certificate | Yes, parser fuzzed |
| Stealing this server's rig key | A file of its own, readable only by the server's account where the system allows. Not in the database, so not in a database backup. Never logged or in an API answer. Windows permissions come from the data folder | Yes, for the API and database |
| Stealing Sunshine's password | Never stored or logged. Only sent to the pinned certificate | Yes |
| A hostile rig or app names | Answers are size-limited and XML only. Titles are clipped and shown as text. Art must be PNG or JPEG, and is served as a picture. A name that starts with `-` or has a line break is never put on a command line | Yes |
| A hostile page in the desktop app | It can only ask for two fixed shapes of command, checked in the main process, with no shell. The path is never from the page | Unit tested |
| Waking other machines | Only a registered rig's own MAC address, never a group address. At most every ten seconds | Yes |
| A viewer ends someone else's game | Only admins and whoever started it from here | Yes |
| Joining someone's stream through browser play | Refused unless it's your own, or you're an admin. Checked at the last moment | Yes |
| Reaching the bridge directly | Loopback only. On a multi-user machine, anyone local could reach it and name any user: don't run this on a shared computer | Config tested. Bridge not run |
| The bridge's first sign-in becoming its admin | Done by this server before anyone is let in, with a generated password in the bridge's folder | Against a stand-in |
| A person's access removed mid-stream | **Not stopped.** A stream that has started carries on over UDP until it ends. New streams are refused | New streams, yes |
| The bridge's data file | **Holds its client key in plain text**, as the program itself does. Its folder is only readable by the server's account where the system allows. Removing browser play deletes it | Not applicable |
| Video on the home network between the bridge and the PC | **Not encrypted** in the bridge's protocol. Someone on that network could capture it. Audio is | Not applicable |
| A flaw in the bridge's pages | Its own origin, a strict Content-Security-Policy, nothing identifying passed through | Against a stand-in |

## Licensing

This is an engineer's reading of the licences, not legal advice. [WITTYRETROARCH.md](WITTYRETROARCH.md) is the model for it.

| Project | Licence (read from each repository's `LICENSE` file) | How it's kept apart |
|---|---|---|
| Sunshine | GPL-3.0 | Installed by the owner on their own PC. Only ever talked to over its network protocols |
| Moonlight (Qt, Android, iOS and tvOS) | GPL-3.0 | Installed by the person on their device. Started as another program, never linked, copied or bundled |
| moonlight-web-stream | GPL-3.0-or-later | Downloaded at an admin's request from its own release, and checked against a recorded digest. A separate program talking HTTP on loopback |
| WittySunshine, WittyMoonlight | Apache-2.0 | Ours |

- **No code is copied.** GameStream's message names and flow are facts about a protocol, needed to speak it. Sunshine's and Moonlight's
  sources were read for those, and our code is our own. `modules/wittysunshine/fake` is written from the protocol too.
- **Nothing of theirs ships in the server's release,** so there are no GPL obligations for us to meet: the admin's server gets the program from
  its own authors, with their source there. That's how cores work too. Their licence and source link are shown before installing.
- **No fork is needed.** If one is ever wanted, it must be its own GPL-3.0 repository with source offered for each release.
- **The App Store:** GPL-3.0 and Apple's App Store terms are widely held to be incompatible, so no Moonlight code can go inside the iOS or tvOS
  apps. Handing off to the separately installed Moonlight app avoids the question, as its project publishes that app itself.

### The names

**WittySunshine and WittyMoonlight could be mistaken for official releases of those projects**, or for forks. They're neither, and the GPL gives rights
to code, not names. The READMEs and NOTICE files say so, and the UI calls them by what they do ("Game rigs") and names Sunshine and Moonlight only
as the separate things they are. If either project objects, or for less risk now, **WittyRigs** (host side) and **WittyCouch** (client side) say the
same thing with no one else's name in them. Renaming is cheap: the two folders, their Go module paths, and a few lines in `go.mod`, `go.work`, `Dockerfile`,
`scripts/mirror.sh`, the CI file and `api_about.go`. It gets expensive once a repository and a release exist, so that's the time to decide.

## What has been tested

**Ran for real, against Sunshine v2026.914.233613** in Docker under WSL, with no changes to Windows
(`modules/wittysunshine/real_test.go`): recognising it, pairing with a PIN, its certificate pinned, status and MAC address, the app list, art, a stranger's
certificate refused, a wrong PIN refused, starting a game, a second start refused while it runs, ending it, typing another device's PIN through its web
interface, and a wrong Sunshine password. That real Sunshine ran without virtual input, and has no display but a virtual one.

**The whole server and its pages also ran against it, in a browser:** adding the PC by address on the settings page, its PIN shown and typed into the real
Sunshine (which listed it as "WittyMediaServer"), the library filling with its real games and art, its two desktop apps left out, the game's page, Play and what it offers,
**Start it on the PC first**, the status changing to "is running", and ending it. No errors in the browser's console.

**Only against stand-ins:**

- `modules/wittysunshine/fake`: permissions for each kind of account, Kids profiles, failures, and addresses. The fake behaves as the real one did where they were compared, but they are not the same thing.
- A stand-in for the bridge (in `wittymoonlight_test.go`), written from its published API types. The proxy's allow-list comes from reading its stream page's source.

**Never run:**

- **The real bridge program has never been downloaded or run**, so browser play has not been seen to work, on any browser. Running a program downloaded from the
  internet on this machine needs the owner's go-ahead, which hasn't been asked for yet. Expect fixes when it's run.
- **No real game has been streamed**, by anything.
- **No real Moonlight app** has been handed a game. The command line and Android extras come from reading their source.
- **mDNS discovery on a real network:** only the parsing has been tested. The Sunshine under test had no mDNS daemon.
- **Wake-on-LAN on a real PC:** the packet is tested, and a real PC hasn't woken.
- **Windows or Linux as the PC:** Sunshine ran only in a Linux container.

### A checklist for the first real test

1. On the gaming PC, install Sunshine and set its password ([the steps are in Settings, Game rigs](#what-each-device-can-do)).
2. In Settings, Game rigs: press **Find PCs on my network**. Does it appear? If not, does typing its address work?
3. Press **Pair this PC**, and type the PIN into Sunshine's PIN page. Do its games appear in the library, with pictures for any that have them?
4. Sign in as a Kids profile. Is the PC's library empty? Give that profile one game in Profiles. Does only that one show?
5. Install Moonlight on a computer, and pair it using **Pair a Moonlight app from here**. In the desktop app, press Play. Does the game start?
6. Do the same on an Android TV, with the Android app. Does Play start it?
7. On an Apple TV, press **Start on the PC**, open Moonlight, and press Resume on the game.
8. Put the PC to sleep. Does the page say so, and does **Wake it up** work? Wake-on-LAN must be on in its BIOS and network adapter, and usually needs a wired connection.
9. Install browser play, set it up on the PC, and press Play in a browser. If Windows Defender stops it, say so: it has flagged earlier versions.
10. Change the PC's address (or its router's reservation), and press Find. Does the rig follow it?

## Limits today

- Removing a rig here leaves this server on the PC's paired list. Sunshine has to remove it, on the PC.
- Moonlight only starts games it can name, so a game renamed in Sunshine needs a new read of the list.
- Browser play has no bandwidth adaptation beyond what the bridge does, and doesn't work through a reverse proxy on its own.
- The bridge's client certificate is valid for 365 days, in the program's own code. Whether Sunshine cares isn't known: if browser play stops after a year, pair it again.
- The Apple TV flow has three steps and always will, until Moonlight has a URL scheme.
- If the server crashes on Windows, Mac or FreeBSD, browser play's program can be left running on the loopback address until the machine restarts. On Linux it ends with the server.
- Browser play's program runs as the server's own account, unsandboxed, as it needs the network. It's checked against a recorded digest, and is only there if an admin asks.
