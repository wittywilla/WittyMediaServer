# Roadmap

See what's still to do on the project, and what has been tried in the real world and what hasn't. This is for developers and anyone curious, and isn't in the app's help. What has landed is in the changelog.

Names follow [Modules](MODULES.md).

## Now: 0.8.0-alpha

0.8.0-alpha is the next release. It brings a new interface on every app (a top bar or side menu, fourteen layouts and fifteen themes, all drawn from screens the server describes), a new first-time setup, Module API 5 (each module its own project, with its own database and updates), From away (joining a server by a code, peer to peer, from every app) and WittyMediaClient on every platform. The server moves to AGPL-3.0-or-later. The [CHANGELOG](../CHANGELOG.md) lists it all, and the [next-major plan](plans/releases/0.8.0-alpha-next-major.md) what's still to come. Features wait as ideas in [Discussions](https://github.com/wittywilla/WittyMediaServer/discussions/categories/ideas) until work on them starts.

The plans and designs for what comes after, each with what's done, in progress or planned, are in [Plans](plans/README.md).

## Still to build

### The server and its libraries

| What | State |
|---|---|
| **WittyRelay:** a hosted relay for From away, planned as a free service | Not built. It's shown greyed out in **Settings → Remote Access** |
| **Photos:** batch rotate and edit | Not started |
| **Local AI:** a plan document, with image recognition for photos as the first use | Not written. The marked placeholders are in: **People & places** in photos, and `GET /api/photos/ai` says it isn't available |
| **Recordings in a series' own folder** | Not started. Recordings go to one folder, or a chosen library's |
| **A minimum hardware document:** realistic specifications for the server and for disc playback | Written for the server from a two-core container ([Minimum hardware](MINIMUM-HARDWARE.md), [#643](https://github.com/wittywilla/WittyMediaServer/issues/643)); not yet measured on a real Pi, ARM NAS or for disc playback |
| **A real sandbox on FreeBSD:** a module's helper has limits there, not a sandbox | Not built. macOS uses Apple's own sandbox |

### Discs, tuners and games

| What | State |
|---|---|
| **Blu-ray:** mixing in secondary audio and picture-in-picture, and MVC 3D decoding | Not built. FFmpeg has no MVC decoder, so it plays the 2D view |
| **CDs:** CD+G from a raw `.sub` file, and double-byte CD-TEXT | Not built. The second is reported, and names come from elsewhere |
| **Tuners:** Windows BDA tuning, DiSEqC, and a viewer and a recording sharing one tuner | Not built |
| **Game PCs:** a fallback for networks that block UDP, HEVC and AV1, and adapting the bitrate | Not built. See [Game PCs](GAME-RIGS.md) |
| **A small Windows and macOS app** that makes a diagnostic file without a server | Not started. **Settings → WittyDiagnoser** does this with one |

### Apps

| What | State |
|---|---|
| **WittyMediaClient, native apps:** anything an app says it can't play yet | The pages come from the server, so most arrive with it. The desktop app has everything through the web app |
| **WittyMediaClient, iPhone and iPad on real devices** | Run in Apple's simulator only |
| **WittyMediaClient, other TVs:** webOS, Tizen, Roku | Not started |
| **WittyMediaClient, Windows 7** | Not started, and needs the owner's decision |
| **A server build for Android TV,** for boxes such as a Shield | Not started |
| **WittyMediaPlayer:** macOS and Linux, Java menus, built-in surround sound | Not started. It runs on Windows |
| **WittyVisualizer in the native apps** | Not started |
| **WittyMediaRemoteMedia:** the basics of connecting NAS libraries through a native or Docker app, in its own repository | Not started |

### Packaging

| What | State |
|---|---|
| **A combined Windows installer,** for the server and client together | Not started. Each has its own |
| **apt and dnf repositories** | The script is ready, and none is hosted |
| **Code signing on Windows and macOS** | Not bought: see [Publishing a release](PUBLISHING.md#signing-the-programs-so-nothing-warns-people) |
| **The server for Arch, and in Flathub and the Snap Store** | Not started. The AppImage, Flatpak and snap are on the Releases page, and haven't been run |

### Documentation

| What | State |
|---|---|
| **Pictures of WittyTV's looks** in the guides | Waiting for the WittyTV changes in progress |

## What hasn't met the real world

These are written and tested as far as this machine allows, and haven't been tried for real.

| Not tried on | Waiting for |
|---|---|
| **A clean copy of Windows 11** with Smart App Control on: the installers and every program | A code-signing certificate: [#65](https://github.com/wittywilla/WittyMediaServer/issues/65) |
| **A real Mac, with a person at it:** the `.pkg`, the menu-bar app and the uninstaller app | Someone to install it. The server, its tests and WittyPlay have run on one |
| **FreeBSD hardware or arm64:** the amd64 package has run in virtual machines, and modules have only been built for it | A machine |
| **Real iPhones, iPads and game controllers** | The owner's devices. Android phones, tablets and TVs and an Apple TV have run |
| **Safari, Firefox and old TV browsers** for games and the visualizer | The owner's devices |
| **Screen readers** with real speech | A person |
| **Real tuners** through a helper | Hardware |
| **Retail Blu-rays,** with Java menus, UHD HDR and MVC 3D | Discs |
| **A real game with a real GPU** on a game PC | A gaming PC: [the checklist](GAME-RIGS.md#a-checklist-for-the-first-real-test) |
| **Game delay on a real screen:** the figures are headless, on one machine | Someone playing, and a slow-motion camera |
| **A real 3DS game:** the only file here is encrypted, so it's refused | An unencrypted dump |
| **WittyMediaPlayer's release build** on GitHub's Windows machines | A fix: [WittyMediaPlayer#1](https://github.com/wittywilla/WittyMediaPlayer/issues/1) |

---

<sub>WittyMediaServer documentation</sub>
