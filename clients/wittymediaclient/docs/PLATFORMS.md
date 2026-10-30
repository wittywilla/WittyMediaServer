# Platform status

What each client can do, and how far each claim has actually been checked. This page is meant to
stay honest: **written** does not mean **works**.

| Word | Meaning |
|---|---|
| **Ran** | Run for real on this development machine, against a real server, with automated checks. |
| **Written** | The code exists but has never been compiled, let alone run on a device. Expect mistakes. |
| **Not started** | Nothing exists yet. |
| **Blocked** | Needs something only the owner can get. |

**One exception to "never compiled" on Android:** its non-screen code (address rules, API layer and data
models) has compiled on a plain JVM with the app's own library versions, and its 9 tests pass, including
all the shared address cases. That says nothing about the screens, the player or the app as a whole.

## Platforms

| Platform | Status | Notes |
|---|---|---|
| Windows 10 and later | **Ran** | From source and as a packaged app. Installers are configured but not built yet. |
| Linux | **Written** | Same code as Windows and CI packages it, but it has never been launched. |
| Android TV, Google TV | **Written** | Never compiled. |
| Fire TV | **Written** | Never compiled. Needs Fire OS 6 or later: the first Fire TV Sticks are too old for Compose. |
| Android phones, tablets | **Written** | Never compiled. |
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
| Type an address, with the shared rules | Ran | Written | Written |
| Refuse plain HTTP to a public address | Ran | Written | Written |
| **Find servers on the network** | **Not started** | **Not started** | **Not started** |
| **Security code typed on the device** | **Not started** | **Not started** | **Not started** |
| Check the server's identity before sending a sign-in | **Not started** | **Not started** | **Not started** |
| TV shows a code, approved on a phone | Not needed | Written | Written |
| Password and profiles | Ran, through the web interface | Written | Written |
| Remember the server | Ran | Written | Written |

Finding servers and the security code are the owner's main requirement, and **neither works yet on
any client**. The server side is being built on the `client-contract` branch. Client work starts when
that lands, because it can't be tested until then.

## Features

| Feature | Desktop | Android | Apple |
|---|---|---|---|
| Home, libraries, shows, item pages | Ran | Written | Written |
| Video with resume | Ran | Written | Written |
| Audio and subtitle tracks | Web interface; not checked here | Written | Written |
| Skip intro, next episode countdown | Web interface; not checked here | Written | Written |
| Themes | Ran, including the title bar | Colours from the server's theme only, no picker | Colours from the server's theme only, no picker |
| Search | Web interface; not checked here | Not started | Not started |
| Music, audiobooks, podcasts | Web interface; not checked here | Not started | Not started |
| Books and comics | Web interface; not checked here | Not started | Not started |
| Disc menus | Web interface; not checked here | Not started | Not started |
| Gaming, channels, tuners, photos | Arrive with the server | Not started | Not started |

Native apps say plainly that an item can't play yet, rather than failing, for anything but video.

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
