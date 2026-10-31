# WittyMediaClient design

WittyMediaClient is the family of apps that connect to a WittyMediaServer: TV boxes, phones,
tablets and desktops. This document records what we build, in which order, and why.

Status of each platform, including what has actually been run, is in [PLATFORMS.md](PLATFORMS.md).
What the owner has to do (accounts, signing, stores) is in [OWNER-STEPS.md](OWNER-STEPS.md).

## Goals, in priority order

1. **Easy to connect.** A non-technical person can get from "installed" to "watching" with only a
   TV remote. See [Connecting](#connecting-to-a-server).
2. **Smooth on low-end hardware.** A cheap TV stick is the reference device, not a flagship phone.
3. **Secure by default.** No token in a URL where a header will do, tokens in the platform's
   secret store, and no telemetry. Nothing contacts any host except the server the user chose.
4. **Matches the web client**, feature by feature, in the phases below.
5. **Honest sharing.** Share what is really the same; don't force a lowest-common-denominator UI.

## The approach

There are three kinds of client, chosen by what each platform does well.

| Kind | Platforms | UI | Player | Why |
|---|---|---|---|---|
| **Native** | tvOS, iOS, iPadOS, macOS | SwiftUI | AVPlayer | The system player gives HDR, Atmos passthrough, Siri remote scrubbing, AirPlay and VoiceOver. A web view on tvOS doesn't exist at all. |
| **Native** | Android TV, Google TV, Fire TV, Android phones and tablets | Jetpack Compose | Media3 (ExoPlayer) | Hardware decoding and passthrough, and it doesn't depend on the age of the device's WebView, which is often very old on TV boxes. |
| **Shell** | Windows, Linux, webOS, Tizen | The server's own web UI, behind a small connect screen | The browser engine, via the web UI | The web UI already has every feature, including TV navigation (`web/src/tv.ts`). A shell gets parity on day one and stays current with the server, with no second UI to maintain. |

Roku has its own language (BrightScript) and can't share code with anything else, so it is a
separate later client. Consoles are blocked: see [Platform order](#platform-order).

### Rejected alternatives

- **One cross-platform UI (Flutter, React Native) everywhere.** It would have to wrap AVPlayer and
  Media3 anyway, its tvOS support is community-maintained, and it adds weight on the low-end devices
  that goal 2 is about.
- **Kotlin Multiplatform for shared logic.** The shared logic here is small: JSON models and a
  connection flow. Sharing it would put Gradle and a Kotlin framework into the Apple build for about
  500 lines of code, which is a poor trade against test vectors both sides must pass.
- **The web UI in a WebView on Android and Apple.** Rejected for the reasons in the table. The
  existing `clients/mobile` Capacitor scaffold is superseded by the native apps.
- **Tauri for the desktop.** It uses the system web view, so behaviour differs by machine: WebKitGTK
  on Linux has weak HLS and codec support, and WebView2 needs Windows 10 for current versions.
  Electron bundles one known engine with H.264 and AAC, at the cost of size and memory.

### What is shared, and how

Everything shared is **data**, in [`shared/`](../shared), so no platform depends on another's
toolchain:

| File | What it holds | Used by |
|---|---|---|
| `shared/data/themes.json` | All server themes as plain colour values, generated from `themes/*/theme.css` by `shared/scripts/build-themes.mjs` | Native clients, connect screen |
| `shared/data/input-map.json` | What each remote and controller button means | All clients |
| `shared/data/connection-cases.json` | Test vectors: what a typed address normalises to, and which addresses are refused | Tests on every client |
| `shared/data/redaction-cases.json` | Test vectors: which secrets a client's log must hide, and what it must keep | Tests on every client |
| `shared/data/strings.en.json` | Wording of the connection flow, so every client says the same plain thing | All clients |
| `shared/connect/` | The connect screen (HTML, CSS, JS, no framework) | Desktop, webOS, Tizen |

API models are written by hand per language because they are short, and checked against one set of
captured responses in `shared/fixtures/`. The API contract itself (`docs/CLIENT-API.md`) belongs to
the server.

## Connecting to a server

The API is `docs/CLIENT-API.md`, owned by the `client-contract` work. The flow on every client:

1. **Find.** The app lists servers it finds on the local network, and always offers "Enter an
   address" beside them. On TV the first found server already has focus, so one press selects it.
2. **Prove.** Connecting takes a code, so a neighbour's device on the same network can't simply attach.
   There are two directions, and both end signed in:
   - **A code typed on the device:** someone already signed in gets six digits and types them with a
     number pad that works from a remote. This is the owner's requirement.
   - **A code shown on the device:** the TV shows a code, and someone approves it on a phone. This is
     the easier one when typing on the TV is the hard part.
3. **Check.** Before sending a sign-in to an address, the app has the server prove its identity with a
   fresh signature, against the key it remembered from the first connection. A mismatch is refused and
   explained, because it may be a different machine pretending to be the server.
4. **Remember.** The server and token are kept in the platform's secret store, and the next launch
   goes straight to the home screen.

A name and password remain available everywhere as the fallback.

**Status:** the whole flow is built on every client but has only been run on the desktop. On Android it
compiles and its logic and network calls are tested, but no screen has run and no real network has been
searched. On Apple it has never been compiled. See [what has been checked](PLATFORMS.md#connecting).

### Discovery by platform

The server announces `_wittymedia._tcp` by mDNS/DNS-SD, with a UDP broadcast as an extra for clients
with raw sockets.

| Platform | Mechanism | Limit |
|---|---|---|
| tvOS, iOS, iPadOS, macOS | `NWBrowser` (Bonjour) | iOS and tvOS show a local-network permission prompt. Raw UDP broadcast would need an entitlement Apple must grant. |
| Android family | `NsdManager` | Newer Android versions are adding a local-network permission; check this against the target SDK at each upgrade. |
| Windows, Linux | Node `dgram` in Electron's main process | The firewall may prompt. |
| webOS, Tizen, browsers | None available to web apps | **Manual address only**, stated plainly in the UI rather than showing an empty list. |

### Addresses and transport security

A typed address is normalised the same way everywhere (`shared/data/connection-cases.json`): add
`http://` if there's no scheme, add port 9797 only if the user gave neither a scheme nor a port, and
strip trailing slashes and paths. The app then confirms it really is a WittyMediaServer before saving
anything: through `GET /api/v1/capabilities` on the native apps, and `GET /api/status` on the desktop.

Plain HTTP is allowed to private and local addresses, because that's how home servers run. On a
public hostname it is refused with an explanation and a suggestion to use HTTPS, because a token
would cross the internet in the clear. Apple's App Transport Security enforces this by itself. On
Android, the network security config can't express "private addresses only", so cleartext stays
enabled there and the check in the app's own connection code is what refuses public HTTP.

### Tokens

- **Native clients** send `Authorization: Bearer` on API and artwork requests. Players get the address
  from `/playback`, which carries a short-lived stream token that is read-only and for one item. AVPlayer
  has no supported way to add headers, and a stream token is far less powerful to leak.
- **Storage:** Keychain on Apple, and an Android Keystore-wrapped value with backups disabled. Shells
  hold no token themselves: the web UI manages its own session as it does in a browser, and the
  desktop app only remembers server addresses and its window size.

## Platform order

1. **Windows and Linux desktop.** It is the only client that can really be run on this development
   machine, it reuses everything in the web UI, and it proves the connect flow.
2. **Android family.** One app reaches TV boxes, Fire TV, phones and tablets. Anyone can sideload it
   free of charge, so the owner can test it on real hardware with no account.
3. **Apple family.** Named first in the request and built in the same pass, but ordered after Android
   because it can't be compiled here and can't be installed on an Apple TV without a Mac.
4. **webOS and Tizen.** Packages of the connect screen. Cheap once step 1 exists, but each needs a
   developer-mode TV to test.
5. **Roku.** A separate client in BrightScript, once the API contract has settled.
6. **Browser / PWA.** Already served: the web UI has a manifest and works in any browser. Installing
   it as an app requires HTTPS, which most home servers don't have, so this is a server documentation
   matter and not a separate client.
7. **Game consoles: blocked.** PlayStation, Xbox and Nintendo need approved developer programmes, and
   their SDKs are under NDA. Nothing is faked here; this stays blocked until the owner is accepted to
   a programme.

## Windows 7

The roadmap asks for Windows 7 and up. **This is flagged, not assumed.** The cost:

- Electron 22 (Chromium 108) is the last version that runs on Windows 7. It stopped receiving security
  fixes in October 2023. Current Electron needs Windows 10.
- WebView2 stopped at version 109 on Windows 7, so Tauri has the same problem.
- A legacy build would mean a second release line, frozen on a browser engine with years of known
  vulnerabilities, displaying pages from the network. That conflicts with the security-first rule.

**Decision for this pass:** the desktop client targets Windows 10 and later, and no legacy build exists.
The owner may decide otherwise: such a build would be limited to private addresses and labelled as
unsupported. See [OWNER-STEPS.md](OWNER-STEPS.md).

## Input

Every client follows `shared/data/input-map.json`.

- **TV remotes:** D-pad moves focus, centre confirms, Back goes back one level and never quits from
  the middle of a flow. Focus is always visible and is restored when returning to a screen.
- **Game controllers:** Android and tvOS map controllers to focus events at system level, which Compose
  and SwiftUI receive natively. The player additionally reads shoulder buttons for seeking. In shells,
  `web/src/tv.ts` already handles the Gamepad API.
- **Touch:** phones and tablets use standard gestures and system controls.
- **Screen readers:** every control has a label, posters announce title and year, and the system player
  is used wherever possible because it is already accessible. **Not yet tested with VoiceOver or
  TalkBack on a device.**

## Themes

`/api/status` names the server's theme. Native clients read colours from `shared/data/themes.json`, which
is bundled, and fall back to the default theme for a custom theme uploaded to a server. The server's
`/themes/{id}/tokens.json` can replace the bundled copy later, and would cover uploaded themes too. Shells show themes exactly, because they display the
server's own page, and the desktop's title bar reads its colours from that page's CSS variables.

## Performance rules

- Lists are lazy, and images are requested at the size shown and cached on disk with a bound.
- No animation on focus beyond a border or small scale change.
- No background work while playing, other than saving progress every 10 seconds.
- The player stops the server's conversion when the viewer leaves, so a closed stream costs nothing.
- Android builds with R8 and ships a baseline profile when one can be generated on a device.

## Feature phases

| Phase | Features | Native clients | Shells |
|---|---|---|---|
| 1 | Connect, sign in, home, libraries, item pages, video with resume, subtitles and audio tracks | **This pass** | Done through the web UI |
| 2 | Profiles, shows and seasons, search, themes | Partly this pass: see PLATFORMS.md | Done through the web UI |
| 3 | Music, audiobooks, podcasts | Later | Done through the web UI |
| 4 | Books and comics | Later | Done through the web UI |
| 5 | Disc menus | Later | Done through the web UI |
| 6 | Gaming, channels and tuners, photos, as those server features land | Later | Arrive with the server |

Encrypted broadcasts and copy-protected discs are refused on every client, as on the server. There is
no bypass and there won't be.

## Licences

Every dependency is recorded in [`NOTICE`](../NOTICE). Only permissive licences (Apache 2.0, MIT, BSD)
are used in shipped code. Electron's bundled Chromium and FFmpeg licences ship with its builds.

## Repository

The code lives in `clients/wittymediaclient/` for now. The owner will create its own repository on
GitHub, after which it mirrors with `scripts/mirror.sh` like the modules.
