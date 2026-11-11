# Peer-to-peer connections

For developers. This page is the design of the feature that lets an app reach a server at home with nothing set up on the router and no machine rented anywhere: what it is built on and why, what goes over the wire, who is contacted, what is defended against, and how the apps use it. [Remote access](REMOTE.md#peer-to-peer-connections) is the guide for owners.

## Contents

- [What it does](#what-it-does)
- [The layers](#the-layers)
- [What it's built on, and what else was looked at](#what-its-built-on-and-what-else-was-looked-at)
- [Who is contacted](#who-is-contacted)
- [The security model](#the-security-model)
- [On the wire](#on-the-wire)
- [The server's side](#the-servers-side)
- [The app's side](#the-apps-side)
- [Joining by a code](#joining-by-a-code)
- [Browsers](#browsers)
- [Limits](#limits)
- [What it costs](#what-it-costs)
- [Tests](#tests)
- [What isn't done](#what-isnt-done)

## What it does

The server and each app run a [libp2p](https://libp2p.io) node. The server's node has a key pair that it keeps; the public half, written as a libp2p peer ID (`12D3KooW…`), is the server's **connection ID**. An app dials that ID. Whatever path the two find, the connection is encrypted and authenticated to that key from end to end, and the server's ordinary HTTP API is carried inside it, one libp2p stream per HTTP connection, into the same `http.Handler` every other request reaches. Nothing about the API changes: ranges, server-sent events and WebSockets work as they do at home.

It is off until the owner turns it on. Off, no node exists, no socket is open, nobody is contacted and the router is asked for nothing.

| Where | What |
|---|---|
| `internal/p2p` | Both ends: the node (`host.go`), the server's end (`node.go`), the app's end (`client.go`, `keeper.go`), streams as connections (`conn.go`), invites (`p2p.go`, `enrol.go`) |
| `internal/portmap` | Asking the home router: UPnP IGD, NAT-PMP, PCP, IPv6 firewall pinholes (`holder.go`, `pcp.go`, `upnp6.go`) |
| `internal/remote/p2p.go` | The mark that makes a peer-to-peer request count as coming from the internet |
| `internal/server/api_p2p.go` | Settings, the device list, invites, the routes, the connection test |
| `pkg/wittyconnect` | The app's side, ready to use: a local HTTP address that leads to the server |
| `pkg/wittyconnect/mobile` | The same for gomobile: Android and Apple, Apple TV included |
| `internal/p2p/join.go`, `internal/server/api_p2p_join.go` | A device away from home that shows a code, and the server that is told it |
| `cmd/wittyconnect` | The same as a helper program, for the desktop app |

## The layers

An app tries these in order and uses the best that works. It keeps looking: a relayed connection is replaced by a direct one as soon as one can be made, and a network change starts the search again.

| | Layer | How | Switch |
|---|---|---|---|
| a | Same network | The existing LAN discovery and plain HTTP. Peer to peer isn't involved. An app that is already on a peer-to-peer connection and finds itself on the server's network connects straight to the server's private address | always |
| b | IPv6 | The node listens on IPv6. Home routers still refuse incoming connections, so the router is asked for a pinhole: PCP `MAP` (on Linux, where the IPv6 gateway can be read from `/proc/net/ipv6_route`) or UPnP `WANIPv6FirewallControl:AddPinhole` | `ipv6` |
| c | A port on the router | The same port for UDP and TCP, asked for with PCP, then NAT-PMP, then UPnP IGD. Renewed at half the lease, removed on turning off and on shutdown. A router whose own outside address is private or in 100.64.0.0/10 (a second router, CGNAT) is reported as such and its mapping is never announced | `portMap` |
| d | Hole punching | libp2p's DCUtR: a relay introduces the two, both dial each other at the same moment, and both routers let the connection through. QUIC first, TCP as well | `holePunch` |
| e | Relay | Circuit relay v2. See below for which relays carry what | `relay` |

Whether the server can be reached directly is not assumed from a mapping having been made. It is what other nodes found when they dialled back (AutoNAT v2, `EvtHostReachableAddrsChanged`), and Settings shows which addresses were proved, which failed and which nobody has tried yet.

**Finding the server** (rendezvous) is the public IPFS network's Kademlia DHT ("Amino"), in client mode: the server keeps itself known to the DHT servers nearest its ID, and an app asks them where that ID is (`FindPeer`). Apps also remember the addresses the server had last time and the ones an invite carried, try those first, and work with the DHT unreachable as long as one of them still leads somewhere. Switch: `dht`.

**Relays, honestly.** Any public node of the IPFS network that offers relay v2 will hold a place for the server. Those relays pass on 128 kB per connection for two minutes: enough to introduce two nodes and arrange a hole punch, useless for a page, let alone a film. So the app does not send HTTP through them. It waits (15 seconds by default) for the direct connection, asks again if the first try is lost, and otherwise says plainly that no direct connection could be made. A relay that carries a whole connection is a **community relay**: another WittyMediaServer whose owner turned on "Help other servers" (`relayForOthers`), or one the owner named (`staticRelays`). Community relays announce themselves in the DHT under the key of `wittymedia-community-relay-v1` and register `/wittymedia/relay/1.0.0`, which is how an app tells them apart. They allow 2 GiB and two hours per connection, 16 servers at once. With no community relay known and a router on either side that defeats hole punching (a symmetric NAT), there is no way through, and the status says so. Nobody has to run a relay for the feature to work in the common case; somebody has to for it to work in every case.

## What it's built on, and what else was looked at

**go-libp2p** (MIT; v0.51) with go-libp2p-kad-dht, QUIC through quic-go, and TCP with Noise and yamux.

| | go-libp2p | pion/webrtc and a signalling path | STUN and our own QUIC, plus a DHT | Tailscale `tsnet` | iroh |
|---|---|---|---|---|---|
| Rendezvous nobody has to run | The public Amino DHT, no account | None: signalling needs a server, or abusing someone's (a public MQTT broker, a tracker) | BitTorrent's DHT would do | Tailscale's coordination service, with an account | n0's relays and DNS, no account |
| Hole punching | DCUtR, maintained, QUIC and TCP | ICE, the best there is | To be written, with its own signalling | Excellent | Excellent |
| Relay when punching fails | Public relays are for setup only; our own community relays | TURN: somebody pays for it | None | DERP, Tailscale's | n0's, for now free |
| Identity and encryption | Peer ID is the key; TLS 1.3 in QUIC, Noise on TCP | DTLS; identity has to be bolted on through signalling | To be written | WireGuard | Ed25519 |
| Port mapping | UPnP and NAT-PMP built in (we use our own, which also has PCP) | none | ours | built in | built in |
| From a browser | Only to a server that is directly reachable (WebTransport, WebRTC-direct), with js-libp2p | Yes, which is its one real advantage | No | No | No (WASM, through relays) |
| iOS, Android, tvOS | Go, through gomobile (tvOS with a fork of it: see [the app's side](#the-apps-side)) | Go, through gomobile | Go | Go; needs the account | Rust; another toolchain in every app |
| License | MIT and other permissive, MPL-2.0 for two small parts | MIT | ours | BSD-3-Clause | MIT/Apache-2.0 |
| Health | Maintained by Shipyard, in production in Kubo and others; monthly releases | Healthy | n/a | Healthy | Healthy, young |

libp2p is the only one that gives a rendezvous **and** relays **and** hole punching with no account, no payment and nothing for the owner to run, in the language the server is written in. Its weak spot is the one the table shows: its public relays don't carry data, which community relays answer only as far as owners offer them. WebRTC would have reached browsers but leaves the question "who does the signalling?" with no answer that isn't a server someone runs. `tsnet` works better than anything here and is already [the recommended way](REMOTE.md#the-recommended-way-a-private-network); it needs an account with a company, which is exactly what this feature is for people who don't want.

The node is deliberately small: DHT **client** mode (it answers no DHT queries and stores nothing for anyone), a connection manager that trims to 16–48 connections on a server and 8–24 in an app, the resource manager's limits scaled to 96 MB (server) or 48 MB (app) of a small machine, no metrics, and only two transports listened on (QUIC and TCP).

## Who is contacted

With the feature on and every layer on. Each row stops with its switch.

| Who | Why | What they learn | Never |
|---|---|---|---|
| The bootstrap nodes, `bootstrap.libp2p.io` (four, run by the libp2p project's sponsors) and one fixed address, 104.131.131.82 | The way into the DHT | The node's ID, its IP address and port, its libp2p agent string (`wittymedia/1`) and the protocols it speaks | Anything about media, accounts or what is played |
| Some dozens of DHT servers: other people's computers, chosen by the DHT by how near their IDs are | Keeping the server findable; looking it up | The same. A DHT server asked "where is this ID?" also sees the IP address of whoever asked | |
| Nodes that dial back (AutoNAT) | Proving reachability | The addresses they were asked to try | |
| Relays | Introducing an app, and for community relays carrying the connection | Both ends' IP addresses, when, and how many bytes | The bytes: they are encrypted between the app and the server |
| The home router | Port mapping, IPv6 pinhole | A request on the home network. Nothing leaves the home for it | |

Anyone who knows a connection ID can ask the DHT where that node is and learn its IP address. The ID is therefore given only to signed-in accounts (`GET /api/v1/p2p`), is not in `/api/capabilities` or the mDNS announcement, and can be replaced (`POST /api/v1/p2p/rotate`). A crawler that walks the DHT can also list the nodes that speak `/wittymedia/http/1.0.0`, with their IP addresses: that a home runs this software is visible to the network. This is said in the guide.

## The security model

This is remote access to someone's home. The rules, each with the test that holds it:

1. **Everything is authenticated and encrypted to the server's key.** libp2p won't complete a connection to a peer ID without proof of its private key, on any path. A relay or a DHT server can refuse to help or lie about addresses; it can't read, change or impersonate. The app pins the ID it was given.
2. **An app learns the ID only from the server itself**: over a connection it already trusts (`GET /api/v1/p2p`, signed in), or from an invite an adult account made.
3. **Only listed device keys may open an HTTP stream** (`devicesOnly`, on by default). Any node on the network can connect to the server's node, as libp2p requires. A stream on `/wittymedia/http/1.0.0` from a key that isn't on the list is reset before a byte is read. The list is per account, shown with when each device last connected and how, and removing a device closes its connections at once. `TestOnlyListedDevicesMayOpenStreams`, `TestP2POnlyListedDevicesGetIn`.
4. **Being on the list signs nobody in.** A listed device reaches the sign-in page and nothing more. Passwords, two-factor codes, sign-in limits and lockouts are the same as for anyone on the internet.
5. **A peer-to-peer request is always "from the internet".** The classic mistake in a tunnel is that requests come out of it looking local: from 127.0.0.1, or from the home network. Here the connection is marked when it is accepted (`remote.WithP2P` in the HTTP server's `ConnContext`), `remote.Policy.Resolve` looks at the mark before any address, and the answer is `Internet` whatever the address, the forwarded headers, or the owner's lists of home networks and trusted proxies say. As a second line, the address the request is counted under (`presentedIP`) is never a loopback, private, link-local or CGNAT address: a direct connection from a public address keeps it; anything else gets an address in 2001:db8::/32 made from the device's key, or 192.0.2.1 for strangers. `TestP2PIsNeverAtHome` (every combination of policy, mark, remote address and forwarded header), `TestPresentedIPIsNeverLocal`, and `TestP2PRequestsCountAsAwayFromHome`, which sends real requests from this very machine over a real connection and checks that what the server shows only to itself stays hidden.
6. **Everything that applies away from home applies**: the gate (let in only while the feature is on), the limits on resolution and bitrate, the connection limits (the listener is wrapped by the same `ConnLimit`), sessions that only work at home, the hiding of the version from strangers.
7. **Nothing is asked of the router unless the owner turned the feature on, and the port-mapping layer with it.** `TestP2PSettingsAreForTheOwner`, `TestNoMappingUnlessTheLayerIsOn`.
8. **Invites** are 128 random bits, work once, for fifteen minutes, die with the sign-in that made them, and are kept only as a hash. Showing one is the single thing an unlisted key may do, and tries are limited (5 a minute per key, 10 per address, 60 in all).
9. **The log** records, under `auth` and `remote`: the feature and each layer being turned on or off, a device added (by whom, from where, how), a device removed, an invite made, an invite refused and why, refused streams (at most a line a minute), the key being replaced, and every connection test.

What it doesn't defend against: a stolen device (remove it from the list, and sign it out); an attacker who is on the list and has a password; traffic analysis by a relay (it sees sizes and times); the IP address of the home being learnt by anyone who has the ID; bugs in libp2p, quic-go or this code, none of which has had an independent audit.

## On the wire

Protocol IDs, all under `/wittymedia/`:

| ID | What |
|---|---|
| `/wittymedia/http/1.0.0` | HTTP/1.1, as on a TCP connection: requests and responses, keep-alive, `Upgrade`. One stream is one connection. The server reads no `Forwarded` or `X-Forwarded-*` header on it |
| `/wittymedia/enrol/1.0.0` | One request and one answer, each a 4-byte big-endian length and that many bytes of JSON (at most 4 kB), then the stream ends |
| `/wittymedia/join/1.0.0` | Four messages in the same framing as enrolment: see [joining by a code](#joining-by-a-code) |
| `/wittymedia/relay/1.0.0` | Registered by community relays, and closed as soon as it is opened. Only its presence in the relay's protocol list means anything |

Enrolment: `{"v":1,"secret":"…","name":"Kate's phone","platform":"android"}` is answered with `{"ok":true,"code":"123456","serverName":"…","addrs":["/ip4/…"]}` or `{"ok":false,"reason":"wrong|used|expired|too-many-tries|refused|off","error":"…"}`. `code` is an ordinary sign-in code: the app redeems it at `POST /api/v1/pair/redeem` over the new connection, like a code typed on a TV.

An **invite link**: `wittymedia://p2p/<connection ID>?s=<secret>&n=<server name>&a=<multiaddr>&a=…`, with at most six addresses, so it fits a QR code a phone reads easily. The addresses are hints and nothing in the link is trusted: the ID is what gets dialled and proved.

Keys are Ed25519. An ID is the libp2p peer ID of the public key (identity multihash, base58btc: always `12D3KooW` and 44 more characters). Other key types are refused. A key is kept as its 32-byte seed in base64.

## The server's side

Settings are one JSON value, `p2p`, in the settings table; the key's seed is `p2pKey`. A backup keeps the key and leaves out the settings and the device list, as it leaves out sign-ins: a restored server is off until someone turns it on, has the same ID, and its devices add themselves again the next time they are at home and signed in.

| Route | Who | What |
|---|---|---|
| `GET /api/v1/p2p` | signed in | `{enabled, serverId, addrs, devicesOnly, layers, staticRelays, serverName}`, and `enrolled` for `?device=<id>` |
| `GET /api/v1/p2p/devices` | signed in | Own devices; `?all=1` everyone's, for an admin |
| `POST /api/v1/p2p/devices` | signed in | `{id, name, platform}`: puts the calling device's key on the list |
| `DELETE /api/v1/p2p/devices/{id}` | signed in | Own; anyone's for an admin. Closes its connections |
| `POST /api/v1/p2p/invite` | adult | `{userId}` → `{link, expiresInSec}` |
| `POST /api/v1/p2p/join` | adult | `{code, userId, password, otp}` → `{ok, expiresInSec, user}`: the code a device away from home is showing |
| `GET` / `PUT /api/v1/p2p/settings` | admin | The settings, each layer's state in words (`steps`), who is contacted, every device |
| `GET /api/v1/p2p/status` | admin | Only the state |
| `POST /api/v1/p2p/test` | admin | The connection test |
| `POST /api/v1/p2p/rotate` | admin, with password | A new key and ID |

`steps` is a list of `{name, state, detail}` with `state` one of `ok`, `warn`, `fail`, `off`, `wait`: reachable from outside, IPv6, the port on the router, hole punching (from the kind of NAT libp2p observed), the address book, relays, devices. **The connection test** starts a second node in the server's process with a throwaway key, lets that key in for two minutes, has it find the server by ID through the DHT, connect, and send `GET /api/health`. Being on the same machine it can't prove a direct path through the router, and says so; it proves the server is announced, findable, holding a relay and answering.

The feature name in `/api/capabilities` is `p2p`; `p2p-join` says the server takes a code a device shows.

## The app's side

An app does three things:

1. **Once, while it reaches the server some other way and is signed in**: make a key (`NewKey`), ask `GET /api/v1/p2p?device=<its ID>`, and if `enabled` and not `enrolled`, `POST /api/v1/p2p/devices`. Keep `serverId` and `addrs`.
2. **When the server can't be reached directly**: start a connector with the key, the server's ID and the kept addresses. It answers with a port and a secret. The server is now at `http://127.0.0.1:<port>`, for requests that carry the secret in the header `X-Witty-Connect` or a cookie named `wittyconnect`. Requests wait while the server is being found; a failure comes back as `502` with the reason in words (`X-Witty-Connect-Error: 1`).
3. **Or, from an invite**: start the connector with the invite's ID and addresses, call `Enrol` with its secret, and redeem the code that comes back.

The connector reports its path (`connecting`, `offline`, `local`, `direct`, `relayed`, with `community` for a relay that carries data) so the app can show it; a relayed connection must be shown as one. It reconnects by itself with a wait that doubles from one second to a minute, and at once when told the network changed. When a relayed connection becomes direct, idle HTTP connections on the relay are closed, so the next request (the next HLS segment, the next range) goes direct while the one in flight finishes where it is.

The local address is bound to 127.0.0.1 only, on a random port (or the one asked for, if free). It refuses requests whose `Host` isn't `127.0.0.1`, `localhost` or a name under `.localhost` on that port, which stops a web page elsewhere from reaching it through a DNS name of its own, and requests without the secret, which stops other programs on the device.

**Desktop (Electron).** `cmd/wittyconnect` is that connector as a program. The app starts it, writes the configuration to its standard input as one line of JSON (never on the command line, where other programs could read the key), and reads events from its output: `ready` with the port and secret, `state`, `addrs`, the answer to an `enrol` command. It exits when its input closes, so it can't outlive the app. The app loads the server from `http://p2p-<end of the server's ID>.localhost:<port>`, a name of its own per server so that each server's cookies stay its own, and sets the secret as an `HttpOnly`, `SameSite=Strict` cookie no page can read. A JavaScript implementation (js-libp2p in the main process) was considered and set aside: it would be a second implementation of everything on this page to keep in step, without go-libp2p's hole punching over QUIC.

**Android and Apple.** `pkg/wittyconnect/mobile` is the same connector in the shape gomobile binds: `NewKey`, `DeviceID`, `ParseInvite`, `Start(configJSON, listener)` and a `Session` with `Port`, `Secret`, `State`, `KnownAddrs`, `NetworkChanged`, `Enrol`, `Close`; `NewJoinCode` and `StartJoin` with a `Joining` (`Wait`, `Cancel`) for [joining by a code](#joining-by-a-code); and `SetMemoryLimit` and `FreeMemory`. Build the libraries with:

```bash
scripts/build-wittyconnect-mobile.sh android   # → dist/wittyconnect/wittyconnect.aar
scripts/build-wittyconnect-mobile.sh apple     # → dist/wittyconnect/Wittyconnect.xcframework
```

It needs Go, and for Android the SDK and NDK (`ANDROID_HOME`, `ANDROID_NDK_HOME`), for Apple Xcode. It builds in a scratch module that points at this checkout, so nothing is added to the server's `go.mod`. The tools are pinned in the script and kept to themselves in `.tools/` in the checkout (or the folder in `TOOLS`): nothing is installed into `~/go/bin`.

The Apple framework has four slices: iPhone and iPad (`ios-arm64`), their simulator, **Apple TV** (`tvos-arm64`) and its simulator, each for Apple silicon and, in the simulators, Intel as well. It is a static library of about 30 MB a slice before linking, for iOS 16 and tvOS 17 and newer (`IOS_VERSION`, `TVOS_VERSION`; `APPLE_TARGETS` adds `macos` or `maccatalyst`). The script checks that every object in every slice says which system it is for, since Xcode refuses to link one that says another.

**How it is built for Apple TV.** The Go project's gomobile (`golang.org/x/mobile`) has no Apple TV target, which is all that stood in the way: Go's `ios` port is the same processor and the same calling rules as tvOS, and what differs is the system named in each object file and the SDK it is compiled against. Three ways were looked at.

1. **A gomobile that has the target.** `github.com/sagernet/gomobile` is a fork of the Go project's (the same BSD-3-Clause license) that adds `tvos` and `tvossimulator`; it is what sing-box builds its Apple TV app's Go library with. `gomobile bind -target ios,iossimulator,tvos,tvossimulator -tvosversion 17.0` built `pkg/wittyconnect/mobile` on the first try, with Xcode 27 and Go 1.27: every object is marked `TVOS` (or `TVOSSIMULATOR`) with a minimum of 17.0, and an app for a real Apple TV links against it. **This is the one used**, pinned to v0.1.13 (commit `9f03b8f2`), for iPhone and iPad as well so that there is one tool and one set of generated bindings. The Android library stays on the Go project's gomobile.
2. **No gomobile:** `GOOS=ios CGO_ENABLED=1 go build -buildmode=c-archive` with clang pointed at the Apple TV SDK and a hand-written C header in place of gobind's Objective-C. It works for the same reason the first way does, and would mean keeping by hand what gobind generates (the listener called back from Go, strings and errors across the boundary). Not needed while the fork is maintained; it is the way to go if the fork ever isn't.
3. **Another transport for Apple TV.** Not needed. A second implementation of everything on this page, in Swift, would have been the price.

What Apple TV is strict about, and how it stands: the Go runtime on `ios` never forks or runs another program, which tvOS forbids (the library does name `execve`, as Go's `syscall` package always does; nothing calls it); the app listens only on 127.0.0.1, which needs no permission; and the library is linked into the app, so there is nothing beside it to embed or sign. Apple Vision Pro has no slice: the fork has no visionOS target, and the app builds without the feature there.

**What an Apple TV or a phone needs that a desktop doesn't.**

- *Memory.* An Apple TV ends an app that takes too much, early and without asking. Inside an app the Go part collects at half again what is live (not double) and is given a soft limit, 64 MB unless the app says otherwise with `SetMemoryLimit` (the Apple TV app says 48); `FreeMemory` is for the system's warning. The node's own limits are the app's in [Limits](#limits). Measured: see [what it costs](#what-it-costs).
- *Leaving the screen.* The system stops an app that is off the screen and takes its sockets. The app closes the session then (`Close`) and starts one when it comes back, asking for the port it had (`port` in the configuration) so that addresses already handed to a player still lead somewhere; the secret is a new one each time, and the app hands it on.
- *The secret.* An app's own requests carry the header. A video player and a web view can't be given one and send cookies by themselves, so the app sets the cookie for `127.0.0.1` where they look.
- *The key.* In the system's keychain, not in a file the system may clear.

## Joining by a code

An invite link is long, and a TV has no keyboard worth the name and no camera. At home that doesn't matter: the TV shows a sign-in code, someone approves it, and the app puts the device on the list by itself. A TV that has never been at home with its server (taken to a holiday house, bought for a relative) knows nothing about the server at all, and has no way to be told.

So the TV makes up a short code, eight letters and digits from an alphabet with nothing that reads as something else across a room (`K7QM-2XHD`), and shows it. Someone signed in types it into the server (`POST /api/v1/p2p/join`, or Settings, Remote Access, "A TV that shows a code"). Then:

1. Both ends turn the code into two keys with Argon2id (2 passes over 32 MB): one to look each other up by, one to prove they know the code.
2. The server announces itself in the address book under the first key: a provider record, as community relays use under theirs. The TV, which has been asking the address book who is under that key every few seconds, learns the server's connection ID and addresses.
3. The TV connects and opens `/wittymedia/join/1.0.0`. It sends the lookup key and 16 random bytes. The server answers with 16 of its own and an HMAC-SHA-256, under the second key, over both random numbers and both ends' IDs (which libp2p has already proved). The TV checks it, and only then sends its own. The server checks that, and from there does exactly what it does for an invite: the code stands for an invite made when it was typed in, which never leaves the server.
4. The answer is enrolment's, with the server's ID beside it: the TV starts an ordinary connection and redeems the sign-in code.

What a stranger can do with it. The address book's computers see the lookup key, and anyone can announce themselves under it. A node that does is sent a random number and nothing else, since it can't make the proof, and the TV doesn't ask it twice (`TestAStrangerUnderTheCodesKeyLearnsNothing`). Working the code out from the lookup key, or from the server's proof, costs an Argon2id run per guess over 40 bits: centuries of computing for a code that is good for fifteen minutes and once. Guessing at the server itself shares the limits on invites (5 tries a minute per key, 10 per address, 60 in all), and three wrong proofs for one code give that code up (`TestGuessingAtACodeGivesItUp`). A proof heard on one connection is no use on another, because both IDs and both random numbers are in it.

What it gives whoever shows the code: a place on the list and a sign-in as the account that typed it (or a Kids or Guest profile it manages). So the same rule as approving a TV at home: signing a device in as an administrator takes the administrator's password again, and the page says to type in only a code you can see on your own device. It needs the address book layer, since that is how the TV finds the server, and says so when it is off (409). Sixteen codes can be waiting at once.

## Browsers

A web page can't open a UDP socket or a raw TCP one, so the web client can't be a node like the apps are. go-libp2p can listen for browsers (WebTransport, WebRTC-direct), but only a server that is *directly reachable* can be dialled that way, a browser can't take part in a hole punch, and the page itself would have to be served from somewhere that isn't the home server, which means infrastructure. So: **from away, peer-to-peer connections need the app.** The desktop app shows the same web interface over the connection. Someone who wants a browser on a computer they can't install the app on needs one of the other ways in [Remote access](REMOTE.md).

## Limits

| What | Limit |
|---|---|
| Devices on the list | 32 per account, 512 in all |
| HTTP connections open per device | 48 |
| Connections in all | The server's own `ConnLimit`, shared with its other ports |
| Through a relay | 4 Mbit/s by default (`relayKbps`), paced by the server; plus the limits for viewers away from home |
| The node's memory | 96 MB by libp2p's resource manager on a server, 48 MB in an app |
| Connections the node keeps | 16–48 on a server, 8–24 in an app; devices and the server are never trimmed |
| Invites open at once | 16; each 15 minutes, once |
| Codes waiting for their device | 16; each 15 minutes, once, and given up after 3 wrong proofs |
| Enrolment tries | 5 a minute per key, 10 per address, 60 in all |
| As a community relay | 16 servers, 4 connections each, 2 GiB and 2 hours per connection, 2 places per IP address |
| Timeouts | 15 s for request headers, 120 s idle, 30 s to dial, 5 s per hole-punch attempt |

## What it costs

Measured on an Apple M-series Mac (arm64), Go 1.27; sizes are release builds (`-trimpath -ldflags="-s -w"`).

| | Before | With the feature compiled in |
|---|---|---|
| Server binary, linux/arm64 | 34.9 MB | 50.6 MB (+15.7 MB) |
| Server binary, linux/arm (v7) | 36.1 MB | 51.8 MB (+15.7 MB) |
| Server binary, linux/amd64 | 37.4 MB | 54.3 MB (+16.9 MB) |
| Server binary, windows/amd64 | 37.9 MB | 54.8 MB (+16.9 MB) |
| Memory, idle, **feature off** | 14–20 MB | 17–21 MB (about 1–3 MB more: the libraries' own start-up) |
| Goroutines, sockets, timers with the feature off | | none |

With the feature **on**, on the public network, the whole server's resident memory was **80 to 95 MB** against 15 to 20 MB with it off: about 60 to 75 MB for the node, most of it in the first two minutes, level after five (three runs of 5 to 15 minutes; the last with `GOMEMLIMIT=100MiB`, which changed little). It peaks at some 90 connections while it finds the network and settles at around 20. CPU: about 3 seconds for the first two minutes, then 0.3 to 0.5% of one core while idle. These are from a Mac, not a Raspberry Pi; on a Pi 3 with 1 GB that is a real share of the memory, which is one more reason it is off unless wanted. `wittyconnect` is 24 to 26 MB as a program.

**In the Apple TV app** (the Apple TV 4K simulator on the same Mac, tvOS 27, a debug build; the app's node with the address book, relays and hole punching on, the soft limit at 48 MB): the app's whole footprint was 33 MB at the connect screen with no connection, of which the Go runtime by itself was about 3 MB; 50 MB signed in from away on its Home screen, level over five and a half minutes (peak 53 MB), of which Go's share was 17 to 18 MB; and 98 MB playing a film through the connection, where Go's share was 19 MB and the rest is the simulator decoding video in software. So the node costs an app about **15 to 20 MB**, a quarter of what it costs a server, because an app's node keeps few connections and serves nobody. CPU at rest was under 1% of one core. The library adds APPSIZE. A simulator has the Mac's memory and no limit to be ended at: what an Apple TV HD with 2 GB makes of it is not known.

About a third of the added size is transports the feature never turns on (WebRTC with the pion stack, WebSocket, WebTransport): go-libp2p's root package names them all, so the linker keeps them. Building the node from go-libp2p's `config` package instead would drop them at the price of following its internals. It wasn't judged worth that yet; nor was a build tag, since a Raspberry Pi 3 has the disk and the memory for the figures above, and off costs next to nothing at run time.

Throughput over loopback, one stream, 64 MiB (`go test -v -run Throughput ./internal/p2p`): QUIC 37 MiB/s (310 Mbit/s), TCP with Noise and yamux 55 MiB/s (460 Mbit/s). Both are far above what a home's upload gives.

## Tests

No test reaches the internet. Nodes listen on 127.0.0.1 or on a network simulated in the process, the DHT in tests speaks under a protocol name of its own (`/wittytest`) so it couldn't join the public one, and routers are fakes.

- `internal/portmap`: PCP, NAT-PMP (UDP and TCP) and UPnP with IPv6 pinholes against fake routers on local sockets; the `Holder`'s renewals, back-off, shared-address detection, refusal of another outside port, and clean removal, driven by a clock of its own.
- `internal/p2p`, unit: the connection keeper (back-off, reconnect, upgrade, network change) with a fake link and clock; back-off; the try limiter; the pacer; invites; the address a connection is counted under.
- `internal/p2p`, integration: HTTP with ranges, events and upgrades over a direct connection; the device list, revoking, the brief pass for the connection test; open mode; the per-device connection limit; enrolment and its limits; a relayed path through a community relay, paced; a public relay used for setup only; finding the server through a DHT; working with the DHT down; reconnecting when the server restarts; port mappings announced only when they lead somewhere and removed on close; nothing mapped with the layer off; prompt shutdown.
- `internal/p2p/join_test.go`: a device that shows a code, in an address book of this machine's own: joining, once only; a stranger announcing itself under the code's key; guessing; running out; nothing without the address book.
- `internal/p2p/holepunch_test.go`: a relayed connection turned into a direct one through [simnet](https://github.com/marcopolo/simnet)'s firewall, which lets a node behind it send out and nothing in uninvited.
- `internal/remote`, `internal/server`: the rules in [the security model](#the-security-model), end to end.
- `internal/server/p2p_join_test.go`: who may type a code in, the administrator's password, the device signed in as the account that typed it, a code dying with that sign-in.
- `pkg/wittyconnect`, `cmd/wittyconnect`: the secret, the `Host` check, ranges, events, sockets, errors in words, the helper's whole life.

All of them run with the race detector.

## What isn't done

- **Never on a real Apple TV, iPhone or iPad.** The Apple apps use it (WittyMediaClient, `docs/PLATFORMS.md`, "From away, peer to peer"), and its check runs the Apple TV and iPhone apps in simulators against a real server: paired at home, the home address taken away, a film played through the connection before and after leaving the screen, and a TV joining by a code through the public address book. A simulator is the Mac it runs on: how soon a real Apple TV ends an app for its memory, what it does to sockets in sleep, and a connection across two real routers have not been seen.
- **The Android app is not wired up.** The bindings build from `pkg/wittyconnect/mobile` and the package is tested, but no Kotlin in WittyMediaClient calls it yet. Known to need care: on Android 11 and later Go can't list network interfaces the usual way (the node then learns its addresses from its peers, which is enough to dial out and to be hole-punched, but has not been tried).
- **Apple Vision Pro** has no build of the library: the gomobile fork that has tvOS has no visionOS target. The c-archive way in [the app's side](#the-apps-side) would give one.
- **Joining by a code has only met itself on one machine.** The server and the TV found each other through the public address book in about 70 seconds, both on the same Mac. Between two homes it also needs the hole punch every connection from away needs.
- **Community relays have nobody offering yet.** The code and its tests are there; a network of them exists only once owners turn "Help other servers" on.
- **Not tried across two real home routers.** Hole punching is tested through a simulated firewall and the whole path on the public network from one machine (found by ID, connected through a public relay), not between two homes.
- **IPv6 pinholes by PCP** are only attempted on Linux; elsewhere only UPnP's are.
- **Game streaming** (WebRTC, Moonlight) doesn't go through the connection: it has its own transports. Everything on HTTP does.

---

<sub>WittyMediaServer documentation</sub>
