# Game PCs: WittyCouch and WittyRigs

People play the games on any PC running Sunshine, in their browser or in WittyMediaClient, through this server. That PC might be
theirs or a friend's. Nothing needs installing on the device they're holding.

Two modules, which aren't to be confused:

| | What it is | Who it's for | On by default |
|---|---|---|---|
| **WittyCouch** | **The main feature.** A Moonlight client in the page: finding a PC, pairing with a PIN, its games, and playing them with controllers, keyboard, mouse or touch | Anyone an admin allows, connecting to **any** Sunshine server | Yes, for adult profiles with no limits |
| **WittyRigs** | **An advanced option.** Hosting demanding PC games on this server's own hardware, through Sunshine as a separate program. It isn't a way to reach other PCs | An admin whose server has a good graphics card, in Settings, Advanced | No |

Handing off to the separately installed Moonlight app is still there, as the fallback for devices that can't play in a page.

**How much of this has really run:** a real stream from a real Sunshine reached a real browser, with controller, keyboard and mouse
input reaching Sunshine. See [What has been tested](#what-has-been-tested). A real game on a real gaming PC, and the app versions, haven't been tried.

## How a stream travels

```
 PC running Sunshine            wittycouch-host          wms (this server)              Browser or app
┌────────────────┐  GameStream ┌───────────────────┐  pipes ┌──────────────────────┐ WebRTC ┌──────────────────┐
│ the game       │────────────►│ moonlight-common-c│───────►│ WittyCouch relay │───────►│ <video>          │
│ its encoder    │  encrypted  │ whole frames, not │ frames │ never decodes either │  H.264 │ Gamepad, keys,   │
│ virtual pads   │◄────────────│ decoded           │◄───────│ checks all input     │◄───────│ mouse, touch     │
└────────────────┘    input    └───────────────────┘  input └──────────────────────┘  Opus  └──────────────────┘
      GPL-3.0                       GPL-3.0                        Apache-2.0              input on data channels
                                                                                                  Apache-2.0
```

- **`wms` talks to the PC's client API directly**, in its own Go code (`modules/wittycouch/gamestream`): identifying it, pairing, its games and art, and asking it to start one.
- **The stream itself** goes through Moonlight's real streaming core, in a program of its own. Moonlight's core is "too powerful a tool to make in house": RTSP, ENet control, encrypted video with error correction, and every Sunshine feature as it arrives.
- **The relay is thin.** It reads whole frames and Opus packets from a pipe and writes them to a WebRTC track. Frames are never decoded or re-encoded, so a low-end server can carry a 4K stream.

## What each device gets

"In the page" means WittyCouch's own client. Moonlight's app is only needed where it says so.

| Device | What Play does | Needs | How far it was checked |
|---|---|---|---|
| **Windows, Mac, Linux, Chromebook: a browser** | **Plays in the page** | Nothing but pairing the PC once | **Ran: Edge on Windows, the whole way, against a real Sunshine** (H.264 picture and input). Firefox, Chrome and Safari not tried |
| **WittyMediaClient desktop** | **Plays in the page**, as it shows the server's web interface. Starting Moonlight is the fallback | Nothing | Written: its Moonlight checks are unit tested. Pointer lock is allowed for the server's page. Not run with a stream |
| **Android phone or tablet: the app** | **Plays in a web view**, with Moonlight as a second choice | Nothing | Compiled, and its decisions unit tested. Never run on a device |
| **Android phone or tablet: a browser** | **Plays in the page** | Nothing | Not tried. Same WebRTC as above, and an on-screen controller |
| **iPhone, iPad: the app** | **Plays in a web view** (WKWebView has WebRTC) | Nothing | Written. Needs a Mac to compile |
| **iPhone, iPad: Safari** | **Plays in the page** | Nothing | Not tried |
| **Android TV, Google TV, Fire TV: the app** | **Moonlight first**, then the page as a second choice. A TV's web view may not give a page the Gamepad API, and a remote can't work a mouse | Moonlight installed, and paired once | Compiled. Never run on a device |
| **Apple TV: the app** | **Start on the PC**, then open Moonlight and press Resume. tvOS has no web view, and Moonlight no link | Moonlight installed, and paired once | Written. See the decision below |
| **Older TV browsers, LG webOS, Samsung Tizen** | Not expected to work in a page | Moonlight has community versions, opened by hand | Not tried |
| **Xbox, PlayStation, Switch, Roku** | **Doesn't work from this server** | Moonlight has community versions for some | Not tried |

Controllers in a browser need HTTPS on some browsers, as they hide the Gamepad API from plain http. WebRTC itself works on either.

**A decision for the owner: Apple TV.** This app can't do better than Start on the PC. The only way to is a separate Apple TV app under the GPL, built on Moonlight's iOS code and
published on its own terms. The App Store and GPL-3.0 are widely held to conflict. Moonlight's own tvOS app does exist in the App Store from its own authors, which is what this hands off to.

## Design decisions

**Moonlight's real code, in a program of its own.** It's the owner's call, and the right one: this protocol has years of quirks in it, and Sunshine changes. So the part that speaks the stream is
Moonlight's. The boundary is a documented protocol over pipes (`modules/wittycouchhost/PROTOCOL.md`), the same way WittyPlay's runner and FFmpeg stay separate. The relay and client lose nothing by it:
frames arrive whole, in order, ready to send on.

**Why this and not the existing web bridge.** [moonlight-web-stream](https://github.com/MrCreativ3001/moonlight-web-stream) already bridges Moonlight to WebRTC, and an earlier version of this design used it, but never
ran it. Reading its source: its roles can't limit anyone to a PC or a game, it has its own users, pairing and pages to keep out of the way, and video from PC to server isn't encrypted. Our relay is about 700 lines, reuses this
server's accounts and remote access on the same port, and asks for encrypted video. Its client also gets to match WittyPlay's.

**The same origin, unlike that design.** With our own client and relay there's no third party's page to keep apart, so the player is an ordinary part of the app, and it goes over the same port as everything else.

**Each person's own PCs.** A person's list, certificate and pairings are theirs (`wittycouch_hosts`, one key file per account). One account's pairing can't be used by another.

**A shared PC is still possible.** An admin can pair a PC once for everyone, from Settings, Game PCs. It gets a library of its own (`type: games`, `console: rig`), and its games are ordinary items, so every access rule applies with no new code. Kids and Guest
profiles (which don't allow unrated titles) see none of its games until given one. That's the way to give a child one game without giving them the PC.

**WittyRigs is just this, for this machine.** Turning it on makes that same kind of library for a Sunshine at `127.0.0.1`. It isn't a way to reach anything else.

**Desktop apps stay out of the way.** Sunshine's "Desktop" app hands over the whole computer. In a shared library, anything like it stays out until an admin adds it. On someone's own PC it's behind one more press. Game streams give keyboard and mouse, which can often get out of a game to the desktop, and Sunshine
has no way to limit that: the settings page says so.

**Sunshine's sign-in is never stored.** It can run any command on that PC. Typing a PIN for someone's Moonlight app takes it, once, sent only to the certificate pinned at pairing.

**H.264 only, for now.** It's what every browser can decode in WebRTC. HEVC and AV1 can follow where a browser has them.

**One picture size per stream.** Sunshine encodes what's asked for at the start, so changing it means starting again.

## Who may connect, and where: the defaults, and why

This feature makes the server connect wherever a person says. That's the riskiest thing about it. An earlier version of this design simply refused to let anyone but an admin name an address, and the owner asked for the opposite: anyone, to any server. So the protection moved rather than went away.

| Setting (Settings, Game PCs) | Default | Why |
|---|---|---|
| Who can use it | **Adult profiles with no limits set**, and admins | The owner's request. Kids and Guest profiles and limited accounts don't get it: a PC's desktop is outside anything age limits can reach. Shared PCs are how they play |
| PCs on this server's network | **On** | The main case: your own PC at home. It only ever speaks this protocol to them |
| PCs over the internet | **On** | The owner's "any and all": a friend's PC. Turn it off if this server shouldn't make outside connections for people |
| Other ports than Sunshine's own | **Off** | Reaching a custom port is rarely needed, and the choice of port is what makes a scanner. Admins can always |
| Streams at once | **2** | Each one's bitrate passes through this server |

**This machine, and link-local addresses, are never reachable by anyone but an admin.** Multicast, link-local (169.254.169.254 is where cloud machines keep credentials) and the like aren't reachable by anyone.

## Security review

Reviewed as an attacker would, with an account on the server and a PC of their own.

| Attack | What stops it | Tested |
|---|---|---|
| Using the server as a network probe (SSRF) | It only speaks GameStream, on its port family, to fixed paths with no user-chosen parts. Answers become a few parsed fields, with nothing passed back raw. What refused and what timed out read the same to non-admins. 20 attempts per person per 10 minutes, and 32 PCs per account | Yes |
| Reaching this machine's other services | Loopback is refused for non-admins, when an address is typed and again at connect time, so a DNS name can't sneak there later | Yes |
| A PC telling the stream to go somewhere else | The host program connects where it's told, so the address isn't taken from the PC: `wms` resolves and checks it, and rebuilds the stream address from the PC's scheme and port alone | Yes |
| Seeing or using someone else's PC | Every route looks a PC up by owner as well, and says "not found" | Yes |
| Using someone else's pairing | A certificate per account, in its own file, never in an answer or the database | Yes |
| Someone else's stream | Streams belong to their owner, and say "not found" to anyone else | Yes |
| Someone on the network posing as the PC | The certificate is pinned at pairing and compared byte for byte on every connection | Yes |
| Someone in the middle of pairing | The PC's secret must be signed by the certificate it offered | Yes |
| A listener saving the pairing messages, and trying all 10,000 PINs | **Not stopped.** It's the protocol's limit, shared by every Moonlight client. The page says to pair on a network you trust | Not applicable |
| **A hostile PC attacking the host program** | **The biggest remaining risk.** It parses a stranger's data in C, so it's a separate process that gets no address or key on its command line and an empty environment. Linux: no new privileges, no core files, no files at all (Landlock). Windows: low integrity, so it can't write to this account's files. A crash ends one stream. **It can still use the network, and read what its account can on Windows.** It's a real reason to keep "over the internet" to people you trust | Linux and Windows, as far as starting and streaming: no attack was tried |
| A hostile host program attacking the server | Everything it says has a size limit, and only known messages are read | Yes |
| A hostile browser | Input messages have exact sizes, and only input types pass. A browser can't start or end anything that way. Picture size, rate and bitrate are clamped | Yes, fuzzed |
| Hostile titles, names and pictures | Clipped, shown as text. Art must be PNG or JPEG, and is served as a picture only, with its own strict policy | Yes |
| Waking other machines | Only a PC its owner registered, its own MAC, never a group address, at most every ten seconds, and never for a PC over the internet | Yes |
| Someone ending another's game on a shared PC | Only admins and whoever started it | Yes |
| A shared PC's game after access is removed | Refused the next time. **A stream that has already begun carries on until it ends** | Yes |

**Logging:** who added or paired a PC, whether it's on a private network or the internet, and when a stream starts and ends, at info level. Never PINs, keys or Sunshine's password. Addresses aren't logged at info level, as a friend's address is theirs.

## Away from home

The PC doesn't need to be opened to the internet if it's at home with the server: **only the server is reached, and it reaches the PC.** That's the design's biggest advantage over running Moonlight directly.

| You are | The PC is | What it takes |
|---|---|---|
| Away | At home with the server | Remote Access, plus the UDP port in Settings, Game PCs (and its public address) forwarded to the server. Without one, WebRTC often finds its own way with a STUN server |
| Anywhere | Elsewhere: a friend's | That PC's owner has to make it reachable from this server. A private network (Tailscale, ZeroTier) is much safer than forwarding |
| Using the Moonlight app, away | At home | A private network on both ends. Put the PC's private address in its settings |

**Warn people off:** forwarding Sunshine's ports puts that PC on the internet, for anyone who finds it. Pairing is still needed to use it, but anyone can knock. **Never forward port 47990**, its web interface, which can run commands on that PC.

**WebRTC needs UDP.** If a network blocks it there's no WebSocket fallback yet, and the page says it couldn't connect.

## Sunshine's ports

Read from LizardByte/Sunshine at v2026.914.233613, and run against that same version. They move with its base port.

| Port | Protocol | What for |
|---|---|---|
| 47989 (base) | TCP, HTTP | `/serverinfo` and `/pair`, unencrypted. Recognising a PC, and pairing |
| 47984 (base − 5) | TCP, HTTPS with a client certificate | `/serverinfo`, `/applist`, `/appasset`, `/launch`, `/resume`, `/cancel` |
| 47990 (base + 1) | TCP, HTTPS, password | Its web interface. **It can run commands.** Only on the local network by default |
| 48010 | TCP | RTSP, while streaming |
| 47998, 47999, 48000 | UDP | Video, control and audio, while streaming |
| 5353 | UDP | mDNS: it announces `_nvstream._tcp` |

## Licensing

**The owner's decisions:** use Moonlight's real code rather than rewriting it; let what's built from it carry its license; keep everything else permissive; and keep every other project's license exactly as it is. This is an engineer's reading of the licenses, not legal advice.

| What | Where | License | Why |
|---|---|---|---|
| **WittyCouch Host** | `modules/wittycouchhost/` | **GPL-3.0** | It links moonlight-common-c, so as a whole it has to be. Its own `main.c` is what's new |
| moonlight-common-c | Fetched at a pinned commit, into that program | GPL-3.0 | Moonlight's streaming core, unchanged |
| ENet, nanors | Inside moonlight-common-c | MIT | |
| Mbed TLS | Linked into that program, under the Apache-2.0 it offers | Apache-2.0 | Compatible with GPL-3.0 |
| **WittyCouch** relay, client protocol, player | `modules/wittycouch/` (`relay/`, `gamestream/`, `web/`, `plan.go`) | **Apache-2.0** | Written here from the protocol and browser APIs. No Moonlight code, ported or otherwise. They exchange only the messages in `PROTOCOL.md` with a separate program |
| The API and pages in the server | `internal/server/api_moonlight.go`, `web/src/pages/Moonlight.tsx` | **Apache-2.0**, with the server | Same |
| **WittyRigs** | `modules/wittyrigs/` | **Apache-2.0** | It only looks for a program and talks to it over the network |
| WittyMediaServer, WittyMediaClient, WittyPlay and everything else | | **Unchanged** | They contain nothing of the above |
| Sunshine | On someone's PC, or this server if an admin installs it | GPL-3.0 | Installed separately by whoever owns that computer |
| Moonlight's own apps | On someone's device | GPL-3.0 | Installed separately. Only started, where a device allows |

**Is the permissive part really permissive?** Reading GPL source to learn a protocol doesn't make what's written from that protocol GPL, but copying or translating code would. So `gamestream/` and `input.ts` were written from the wire format, and share names only where the protocol requires. **Worth a lawyer's look before this is sold:** that a program built to work with a GPL program over pipes is separate is the usual reading (FSF's GPL FAQ, "mere aggregation"), and it's the same reasoning as FFmpeg and [WITTYRETROARCH.md](WITTYRETROARCH.md), but it depends on how much the two are designed for each other. The protocol is deliberately general and documented for that reason.

**What shipping the host program takes** (GPL-3.0 section 6): each binary's release has the matching complete source next to it, including moonlight-common-c, ENet, nanors and Mbed TLS at their pinned commits; its notices stay in; it says what it's modified from. Its release workflow does all this, and it runs in its own repository.

**It's installed separately, and people are told first.** The server's release doesn't contain it, and works without it. Settings, Game PCs says what it is, its license and where its source is before anyone turns to it. Settings, About shows each piece's license and source, and the Modules page shows the license beside each module.

**The App Store:** GPL-3.0 and Apple's App Store terms are widely held to conflict, so nothing of Moonlight's or the host program's goes inside the iOS or tvOS apps. They use the permissive web client, in a web view, or hand off to Moonlight's own app.

### The names

**WittyCouch, WittyCouch Host and WittyRigs** have no one else's name in them, so they can't be taken for official releases of Sunshine or Moonlight. They began as WittyMoonlight, WittyMoonlight Host and WittySunshine, and the owner renamed them before anything was released, because the GPL gives rights to code, not names. WittyCouch Host is still built on Moonlight's code, so its README and NOTICE, and those of the other two, say plainly that they aren't official. The words Moonlight and Sunshine remain wherever the real projects are meant: the Moonlight app on a device, Sunshine on a PC, moonlight-common-c, and the code that starts Moonlight's app.

The page people use is still called **Game PCs**. WittyRigs is named for the rig this server becomes when it hosts games of its own.

## What has been tested

**Ran for real, end to end:** Sunshine v2026.914.233613 in Docker under WSL, with a virtual screen and software x264; this server beside it, on Linux; the real host program built on moonlight-common-c; and **real Microsoft Edge on Windows**, headless, driven by a test script:

- From a clean install: adding the PC by address on the page, its PIN shown and typed into the real Sunshine, its games and art appearing, with desktop apps set apart.
- Pressing Play: **the first picture in about a second, 30 frames a second with none or almost none dropped**, on the encrypted channel, with test colour bars recognisable pixel by pixel and moving.
- **Sound:** silence while the test PC played nothing, then, with a 440 Hz tone played on that PC, the browser's own count of audio energy rose from 0, with no samples concealed.
- **A controller, keyboard and mouse: Sunshine's own log shows each kind of packet arriving** (`--begin controller packet--`, keyboard, mouse button). Whether a game responds isn't shown, as this Sunshine has no virtual input device.
- Leaving a game running and resuming it, and closing it.
- **A shared PC:** paired by an admin, its games in a library, and one played from its item page. A Kids profile was refused the API and shown none of those games.
- The protocol client against the same Sunshine (`gamestream/real_test.go`, `relay/real_test.go`): certificates, a stranger refused, a wrong PIN refused, a second launch refused.
- **The Windows host program,** started at low integrity exactly as the server starts it, took 181 frames and 611 audio packets in six seconds from that Sunshine. That was without a browser, and the whole server hasn't been run on Windows with it.

**Only against stand-ins, in the automated tests:** who may use it, addresses, rate limits, separate pairings, failures, and the relay's WebRTC with a stand-in host program.

**Never run:**

- **A real game on a real gaming PC with a real GPU**, on Windows. Sunshine ran only in a Linux container, with a software encoder and no virtual input.
- **Real controllers.** The pad in the test is a pretend one, as a browser would report it. Rumble hasn't been felt.
- Firefox, Chrome, Safari, phones and tablets in a browser, and the player inside any of the apps.
- **Discovery on a real network** (only the parsing), and **Wake-on-LAN on a real PC** (only the packet).
- Reaching this over Remote Access, or through a real router.
- The host program's release workflow, its macOS builds, and static arm64. Its arm64 build has only printed its version, under an emulator.

### A checklist for the first real test

1. On the gaming PC, install Sunshine and set its password (the steps are in **Game PCs**, under How to install Sunshine).
2. Put `wittycouch-host` beside the server (`sh scripts/build.sh` in `modules/wittycouchhost` builds it).
3. In **Game PCs**, press **Find PCs on the network**. Does it appear? If not, does typing its address work?
4. Press **Pair this PC**, and type the PIN into Sunshine's PIN page. Do its games appear, with their pictures?
5. Press Play on a real game, with a real controller. Is it smooth? Does the controller work? Does sound come through? Does rumble?
6. On a phone: does it play, in Safari or Chrome? Does the on-screen controller work?
7. Sign in as a Kids profile: is there no Game PCs at all? In Settings, Game PCs, share the PC, then give that profile one game in Profiles. Can they play only that?
8. Put the PC to sleep. Does the page say so, and does **Wake it up** work? Wake-on-LAN must be on in its BIOS and network adapter, and usually needs a wired connection.
9. Away from home, on a phone's data: does it connect?

## Limits today

- No WebSocket fallback when UDP is blocked, and no adapting of the bitrate as the network changes.
- H.264 only, and stereo sound only.
- The host program can't be downloaded from inside the app yet: an admin puts it in place by hand, and its repository has to exist and publish releases first.
- Removing a PC here leaves this account on the PC's paired list. Sunshine has to remove it, on the PC.
- A person's streams go through this server, so a low-end server's network speed is the limit. It doesn't decode anything.
