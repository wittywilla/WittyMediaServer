# WittyMediaClient: update plan

> **Status on 2026-11-04**
>
> **Done:** the desktop app brings its window forward on a second start. **Planned:** [everything on Android and Apple](https://github.com/wittywilla/WittyMediaServer/discussions/61), [more TVs](https://github.com/wittywilla/WittyMediaServer/discussions/62). Its own repository has no milestones yet.

**Now:** 0.2.3. Desktop (Electron) for Windows, Linux, macOS and FreeBSD (FreeBSD uses the system's Electron); Plasma Bigscreen opens it as a TV app; Android phone and TV (one sideload APK signed with a throwaway key, emulators only); Apple apps (iOS, tvOS) written and never compiled. Finds servers on the local network with a code, or by address.

**Not built (0.6.0 notes):** Android TV channels, live TV and guide on native apps, richer music browsing, background audio and picture in picture on Android, music and books on native apps (they need the desktop app or a browser), a log screen on Apple, theme picker on native apps.

**Owner steps open:** the SDK licence is done; a Mac with `xcodegen generate` and a team; real devices (the APK on a phone and a TV stick); artwork. See `docs/OWNER-STEPS.md` in the client repository.

| Release | Theme |
|---|---|
| 0.2.x (with server 0.6.3) | Real-device findings from the owner's phone and TV stick; the final accessibility and controller pass; single-instance on desktop (focus the existing window) |
| **0.3.0** (with 0.7.0) | Poster picker for admins on native apps; live TV and the guide on Android and Android TV, with live sync; music browsing and background audio on Android |
| **0.4.0** (with 0.8.0) | Library order and context menus from the server; books and the visualizer in a web view; photos and the new map; theme picker |
| Later | Apple apps compiled and run (needs a Mac); store listings; other TVs |

## Platforms

| Platform | Next step |
|---|---|
| Windows, Linux, macOS, FreeBSD desktop | Code signing on Windows and macOS (a purchase, the owner's call) |
| Plasma Bigscreen | Phase 1: the desktop app opens the server's [TV mode](../TV-MODE.md), every feature at once. Phase 2: a native Qt/QML Kirigami app (libmpv, CEC, SDL controllers). The plan, input, packaging and how to build and test it on the owner's PCs: [PLASMA-BIGSCREEN.md](https://github.com/wittywilla/WittyMediaClient/blob/main/docs/PLASMA-BIGSCREEN.md) in the client's repository |
| Android phone (ARM, x86) | A release-signed APK and a store listing (the owner's account); Android 7 to 15 and tablets untested |
| Android TV, Fire TV, Google TV | Real sticks; game mode (ALLM) where exposed; home screen channels |
| iOS, tvOS | A Mac to build on; TestFlight with the owner's developer account |
| webOS, Tizen, Roku | Not started: decide which first (webOS and Tizen are web apps, so the server's web app in a packaged shell is the shortest path; Roku needs its own app) |
| Windows 7 | Needs the owner's decision (roadmap) |

## Games in the client

Games always run on the server; the client only decodes, displays, plays sound and sends input. WittyPlay 0.9.x's thin native path (MediaCodec low-latency decoding, AAudio; VideoToolbox and a small audio IO buffer on Apple; TV game mode) is where the TV apps can beat any browser, and hand-off between devices (WittyPlay 0.7.3) needs the client's "Continue on this device". See [WittyPlay's timeline](../modules/WittyPlay.md#09x-thin-native-clients).

## Practices

Each release: a milestone in WittyMediaClient with an issue per item; owner steps as `owner-step` issues with clicks and commands ([../GITHUB-PRACTICES.md](../GITHUB-PRACTICES.md)).

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
