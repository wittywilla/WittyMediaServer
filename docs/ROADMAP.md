# Roadmap

See what's still to do on the project, and what has been tried in the real world and what hasn't. This is for developers and anyone curious, and isn't in the app's help. What has landed is in the changelog.

Names follow [Modules](MODULES.md).

## Finishing 0.6.0-alpha

| Step | State |
|---|---|
| A beginner-friendly, modular setup wizard, asking about logos, lyrics, listings and modules | In progress |
| A final pass over every screen, looking for faults | In progress |
| A second security pass over moved modules, apps and the Windows and macOS paths | In progress |
| Documentation and in-app help | In progress |
| Cleaning up the repositories, files and code comments | Not started |
| Heavy optimisation of the server and its resource use | Not started, and comes last |

## Still to build

### The server and its libraries

| What | State |
|---|---|
| **Watch together:** a shared, synchronised player across profiles and devices | Not started. Best built with real networks to test across |
| **Hosted relay:** a future paid way in from away | Not built. It's shown greyed out in **Settings → Remote Access** |
| **Photos:** batch rotate and edit | Not started |
| **Local AI:** a plan document, with image recognition for photos as the first use | Not written. The marked placeholders are in: **People & places** in photos, and `GET /api/photos/ai` says it isn't available |
| **Recordings in a series' own folder** | Not started. Recordings go to one folder, or a chosen library's |
| **A minimum hardware document:** realistic specifications for the server and for disc playback | Not written |
| **Sandbox on FreeBSD and macOS:** a module's helper runs with none there, only a small environment and a soft memory limit | Not built |

### Discs, tuners and games

| What | State |
|---|---|
| **Blu-ray:** mixing in secondary audio and picture-in-picture, and MVC 3D decoding | Not built. FFmpeg has no MVC decoder, so it plays the 2D view |
| **CDs:** CD+G from a raw `.sub` file, and double-byte CD-TEXT | Not built. The second is reported, and names come from elsewhere |
| **Tuners:** Windows BDA tuning, DiSEqC, and a viewer and a recording sharing one tuner | Not built |
| **Games in the browser:** in-browser emulation and on-device play | Designed, not built. See [Games](GAMING.md) |
| **Game PCs:** a fallback for networks that block UDP, HEVC and AV1, and adapting the bitrate | Not built. See [Game PCs](GAME-RIGS.md) |
| **A small Windows and macOS app** that makes a diagnostic file without a server | Not started. **Settings → WittyDiagnoser** does this with one |

### Apps

| What | State |
|---|---|
| **WittyMediaClient, native apps:** music, books, disc menus, channels and photos | Not started on Android and Apple. The desktop app has them through the web app |
| **WittyMediaClient, Apple apps** | Written, and never compiled. They need a Mac |
| **WittyMediaClient, theme picker on native apps** | They use the server's theme colours only |
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
| **The server for Arch, Flatpak and Snap** | Not started. The archive and `install.sh` work on other distributions |

### Documentation

| What | State |
|---|---|
| **New screenshots** for the README at 1920x1080 | Not retaken |

## What hasn't met the real world

These are written and tested as far as this machine allows, and haven't been tried for real.

| Not tried on | Waiting for |
|---|---|
| **A real copy of Windows:** the installers | A Windows Sandbox run: `scripts/test/README.md` |
| **A real Mac:** anything at all, and iOS and tvOS | A Mac |
| **FreeBSD hardware or arm64:** the amd64 package has run in virtual machines, and modules have only been built for it | A machine |
| **Real TVs, TV remotes, phones and game controllers** | The owner's devices. Android has only run on emulators |
| **Safari, Firefox and old TV browsers** for games and the visualizer | The owner's devices |
| **Screen readers** with real speech | A person |
| **Real tuners** through a helper | Hardware |
| **Retail Blu-rays,** with Java menus, UHD HDR and MVC 3D | Discs |
| **A real game with a real GPU** on a game PC | A gaming PC: [the checklist](GAME-RIGS.md#a-checklist-for-the-first-real-test) |
| **Game delay on a real screen:** the figures are headless, on one machine | Someone playing, and a slow-motion camera |
| **A real 3DS game:** the only file here is encrypted, so it's refused | An unencrypted dump |
| **A real release from GitHub,** and every release workflow | Publishing |
| **The complete Docker image** | Module tags on GitHub |

---

<sub>WittyMediaServer documentation</sub>
