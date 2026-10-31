# Platform status

What each client can do, and how far each claim has actually been checked. This page is meant to
stay honest: **written** does not mean **works**.

| Word | Meaning |
|---|---|
| **Ran** | Run for real on this development machine, against a real server, with automated checks. |
| **Compiled** | It builds and its unit tests pass, but it has never been started on a device or emulator. Expect problems at run time. |
| **Written** | The code exists but has never been compiled, let alone run on a device. Expect mistakes. |
| **Not started** | Nothing exists yet. |
| **Blocked** | Needs something only the owner can get. |

The whole Android app compiles, in debug and in the shrunk release form, with 68 unit tests passing,
including all the shared address cases, and one more against a real server when one is given. Only its
rules, data handling and network calls have actually been executed: none of its screens, its player or
its focus handling has ever run, and nothing has run on a device or emulator.

## Platforms

| Platform | Status | Notes |
|---|---|---|
| Windows 10 and later | **Ran** | From source and as a packaged app. Installers are configured but not built yet. |
| Linux | **Written** | Same code as Windows and CI packages it, but it has never been launched. |
| Android TV, Google TV | **Compiled** | Never run. |
| Fire TV | **Compiled** | Never run. Needs Fire OS 6 or later: the first Fire TV Sticks are too old for Compose. |
| Android phones, tablets | **Compiled** | Never run. |
| Apple TV | **Written** | Never compiled. Needs a Mac to install. |
| iPhone, iPad | **Written** | Never compiled. Needs a Mac to install. |
| Mac | **Written** | Never compiled. |
| Windows 7 | **Not started** | Needs the owner's decision: see [the design](DESIGN.md#windows-7). |
| LG webOS, Samsung Tizen | **Not started** | The connect screen they would use works without the desktop app, in the same browser engine as the desktop app. Older TV browsers are untested, and no package exists. |
| Roku | **Not started** | Needs its own app in BrightScript. |
| Browsers | Works today | The server's own web interface. Installing it as an app needs HTTPS. |
| PlayStation, Xbox, Nintendo | **Blocked** | These need approved developer programmes. |

## Connecting

| Step | Desktop | Android | Apple |
|---|---|---|---|
| Type an address, with the shared rules | Ran | Compiled | Written |
| Refuse plain HTTP to a public address | Ran | Compiled | Written |
| **Find servers on the network** | Ran, with packets built in tests only | Compiled. Its rules ran; the search itself never has | Written |
| **Security code typed on the device** | Ran | Compiled. The network side ran against a real server; the number pad has never been on a screen | Written |
| Check the server's identity before sending a sign-in | Ran | Compiled. The check ran against a real server and a real signed answer | Written |
| The warning when a different key answers | Ran | Compiled. The decision ran; the screen never has | Written |
| Look for a saved server by its ID when its address stops answering | Not built: it lists what it finds | Compiled, never run | Written |
| TV shows a code, approved on a phone | Not needed | Compiled | Written |
| Password and profiles | Ran, through the web interface | Compiled | Written |
| Remember the server | Ran | Compiled | Written |

Finding servers and the security code are the owner's main requirement. **They have only been run on
the desktop, and finding servers has not yet been tried on a real network anywhere.** The API for them
is on the `client-contract` branch, so the native apps need that merged before they can connect.

**Checked for Android,** on this machine and without a device:

- The real signed answer in `shared/fixtures/identity.json` verifies, and tampering with its signature,
  key, server ID or nonce makes it fail.
- Over real connections to a stand-in server that really signs: a fresh nonce each time, no token sent
  with the question, a look-alike noticed, a replayed answer refused, redirects not followed, and an
  oversized answer refused.
- Against a real server built from `test/all-merged`: it is recognised, its key is proved and pinned, a
  different pin is noticed, a wrong code is refused, a right code signs in and the token works, and a used
  code is refused.
- The number pad's rules, including that every key can be reached with arrows alone, and what each
  identity result leads to.
- That the Ed25519 code survives shrinking into the release build. Whether it then loads and runs on
  old Android versions hasn't been seen.

**Not checked for Android:** every screen, focus on a real remote, `NsdManager` finding anything, which
differs between devices and has known problems on some, and whether API 23 devices behave like newer ones.

**Apple has never been compiled,** since nothing here has Swift or a Mac. This code was reviewed by reading,
with a script confirming that its brackets balance and its wording keys exist, which proves little. Expect
compile errors. `NWBrowser` and the focus code are the most likely places, and how tvOS moves focus around
the pad can only be seen on one. On Linux WittyKit should build, but can't verify signatures: every answer
is refused there, and the signature tests only run on Apple platforms.

**Known limits of the identity check,** on every client: the very first connection trusts whatever key it
is shown, so someone already impersonating the server at that moment gets pinned, and would see a code
typed for them. Comparing the fingerprint with the one on the server's `/pair` page catches that, and HTTPS
prevents it.

**Differences from the desktop app.** The native apps work out the fingerprint from the key instead of
trusting the server's word, refuse a 404 from `/identity` when a key is already remembered, and re-check when
brought back to the front as well as at a cold start, since TV apps are rarely closed.

**Left out of the native apps:** the UDP question, because mDNS is the way the API guide gives and Apple
can't use it without a special permission. If a device or network turns out to block mDNS, adding it on
Android is the next step. An `http://` address that redirects to `https://` is reported as not being a
server, and needs the `https://` address typed. Apple doesn't limit the size of an answer from an untrusted
address, only how long it takes.

## Features

| Feature | Desktop | Android | Apple |
|---|---|---|---|
| Home, libraries, shows, item pages | Ran | Compiled | Written |
| Video with resume | Ran | Compiled | Written |
| Audio and subtitle tracks | Web interface; not checked here | Compiled | Written |
| Skip intro, next episode countdown | Web interface; not checked here | Compiled | Written |
| Themes | Ran, including the title bar | Colours from the server's theme only, no picker | Colours from the server's theme only, no picker |
| Search | Web interface; not checked here | Not started | Not started |
| Music, audiobooks, podcasts | Web interface; not checked here | Not started | Not started |
| Books and comics | Web interface; not checked here | Not started | Not started |
| Disc menus | Web interface; not checked here | Not started | Not started |
| Games on someone's own PC, through Moonlight | Written; its rules ran in unit tests | Compiled | Written; its rules ran in unit tests on Linux |
| Gaming, channels, tuners, photos | Arrive with the server | Not started | Not started |

Native apps say plainly that an item can't play yet, rather than failing, for anything but video and
games on someone's own PC.

## Games on someone's own PC

A game rig is a person's own gaming PC running Sunshine. Its games play in the Moonlight app, which is a
separate program under GPL-3.0: these apps only start it where the device allows that, and nothing of it
is copied, linked or bundled. **No part of this has ever reached a real Moonlight or a real PC from these apps.**

| | What happens on Play | How far it was checked |
|---|---|---|
| Desktop | The server's page asks the main process, which checks the request and starts Moonlight | The checks, finding the program and starting it ran in 14 unit tests, with a stand-in for the program. The window's side (`main.js`, `preload.js`) only had its syntax checked: **Written**, as the tests start no windows. |
| Android, phones and tablets | Starts Moonlight with an explicit intent, or opens the store, or offers browser play | **Compiled.** Its decisions ran in 12 unit tests, including that a server can't name any other app. The screen and the intent itself have never run. |
| Android TV, Fire TV | The same, without the browser | **Compiled**, as above. Focus with a remote has never been seen. |
| Apple TV, iPhone, iPad | Nothing can start Moonlight, as it has no URL scheme. "Start on <PC>" starts the game on the PC, then the app says to open Moonlight and Resume | The models and checks in WittyKit built and passed their tests on Linux, with Swift 6.1 in a container. The screen itself needs a Mac: **Written**, never compiled. |
| Mac | The same, plus running the desktop Moonlight if it's in `/Applications` | **Written**, as above. |

**Known limits:**

- The Mac app is sandboxed, and a program it starts stays inside that sandbox, so running Moonlight may not
  work there. Nobody has tried. Starting the game on the PC is still offered beside it.
- A server only offers a command to run to the desktop kind of client, so the Mac app asks as `desktop`
  rather than `apple`.
- A PC reached by a bare IPv6 address on the default port gets a command the desktop app refuses, because it
  only accepts `[ipv6]`. The server has to send the brackets, and until then those PCs fall back to the steps.
- Android can't tell that Moonlight doesn't know a PC yet. Moonlight says so itself, and the app has the setup
  steps a button away.

## Not done yet in the native apps

- A QR code on the TV sign-in screen, which the API guide recommends.
- A prompt for two-factor codes on password sign-in. Those accounts can use code sign-in instead.
- Asking the device what it can play. Both apps send a safe profile, so the server converts more
  than it needs to. Phones and tablets borrow the TV profiles.
- Watch next, 30 seconds before the end.
- Now-playing information for the system, and a baseline profile for faster starts on Android.
- App icons and TV artwork. Android has a placeholder, Apple has none, and the desktop app has
  Electron's default icon.

## Checked on the desktop client

On Windows, from source and as a packaged app, against both today's server and the `client-contract`
branch, 24 checks passed using real key presses:

- The address screen, including refusals in plain words.
- The server's page has no access to the app.
- Choosing a profile and typing a password.
- A video plays: the clock advances, it seeks, and a frame was measured and looked at to confirm a
  real picture.
- The resume position reaches the server.
- The title bar follows the theme.
- Up, down, OK and Back on the connect screen.

## Not checked on the desktop client

- **Sound.** Nothing here listens to it.
- **Game controllers** and real TV remotes. Key presses were simulated.
- **Linux** at all.
- Installers, updating from an old version, and very low-end hardware.
- Screen readers.
- Audio and subtitle tracks, and everything else marked "not checked here" above. They are the web
  interface's own features, displayed unchanged.
- A real look on screen. A capture of the whole window looks right, with a pale strip around its edge
  that is almost certainly how the capture draws the invisible Windows resize border.

Known and not explained: in about one in ten runs of the check without the desktop bridge (the path a
smart TV would use), simulated typing had no effect. It did not happen again once diagnostics were
added, and the cause is unknown. It could be the test or the screen.

## Other known limits

- `shared/data/themes.json` is generated. Merging the interface overhaul changes theme colours, so it
  must be regenerated at that point, or CI fails on purpose.
- Plain HTTP stays enabled on Android because its rules can't name "private addresses only". The app's
  own check is the only thing refusing public plain HTTP there.
- The desktop app is about 357 MB unpacked and uses Chromium's memory, so it is not the low-end
  option. On a weak PC, a browser uses less.
