# WittyMediaServer: the future roadmap

> **Status on 2026-11-04**
>
> **In progress:** 0.7.0-alpha, in the [0.7.0-alpha milestone](https://github.com/wittywilla/WittyMediaServer/milestone/1) (release checklist [#84](https://github.com/wittywilla/WittyMediaServer/issues/84)). It holds what this roadmap put in 0.6.3-alpha, plus the owner's additions of 2026-11-04 (performance, a code audit, Docker, macOS readiness, modules that stand on their own), and not the bigger 0.7.0 features below, which stay ideas.
>
> **Planned:** every feature here is an idea in [Ideas](https://github.com/wittywilla/WittyMediaServer/discussions/categories/ideas), labelled planned, until work starts.

Written 2026-11-02, the day 0.6.0-alpha was published. This is a plan, not a promise: nothing here has been built, tested or measured for this document. Every figure quoted comes from work already done for 0.6.0-alpha (the release notes, the docs in the repository and the coordinator's notes in `D:\alpha6-todo`). Anything this plan guesses at is marked **to verify**.

The owner's own list is `futureplans1.txt`, kept with the original plans outside the repository. Everything in it is placed below, word for word in meaning, and then built out with what the 0.6.0-alpha work already taught us.

---

## Where things stand on 2026-11-02

| Piece | Published | Notes |
|---|---|---|
| WittyMediaServer | v0.6.1-alpha | Published later on 2026-11-02 as an urgent fix: DVD and Blu-ray pictures in Firefox. The incremental release planned as 0.6.1 is now 0.6.3-alpha |
| Module API | 3 | `minServer` 0.6.0-alpha is the start of the line. Every module says `maxServer: 0.6.x` |
| WittyMediaClient | v0.2.3 | Desktop (Electron), Android phone and TV (emulators only). Apple apps written, never compiled |
| WittyMediaPlayer | v0.1.2 tagged, **not released** | GitHub's Windows runners crash in Qt's `mingw32-make`. The owner has a local MSI |
| WittyReader | v0.1.1 | |
| Modules | WittyDVD 0.3.1, WittyBluray 0.3.0, WittyPlay 0.3.4, WittyPhotos 0.3.0, WittyTuner 0.2.0, WittyCD 0.2.0, WittyCouch 0.2.1, WittyCouchHost 0.2.0, WittyRigs 0.2.0, WittyDisc 0.2.0, WittyModule 0.3.0, WittyStats 0.1.2, WittyVisualizer 0.2.1, WittyEbook 0.2.0, WittyPDF 0.2.1, WittyComics 0.2.0, WittyInk 0.2.0, WittyIntegratedFileBrowser 0.2.0 | Signed catalogue with 16 versions |

**Carried over from 0.6.0-alpha, never run as a separate pass:** the final screen reader, keyboard-only and every-controller verification. It is the first item of 0.6.3-alpha below, because the owner called it vital.

---

## The release train

```
2026-11 ──► 0.6.3-alpha ──► 0.7.0-alpha ──► 0.8.0-alpha ──► … more alphas, as many as it takes … ──► beta ──► stable
           incremental     mainline        next major       every feature the owner wants         only once      much
           (small, safe)   (planned        (navigation,     reaches "at least somewhat            the gate        later
                           features,       customisation,   implemented"                          below is met
                           Module API 4    channels in
                           with channels)  every module)
```

**The project stays in alpha for a while.** The owner, 2026-11-02: "We're only going to beta once every feature I want is at a point where it's at least somewhat implemented." So there's no date and no version number for the beta. The alpha line keeps going (0.9, 0.10, 0.11 and on are all fine under semantic versioning) until the [beta gate](releases/beta-and-1.0.md#the-beta-gate) is met. Nothing in this folder should be read as pushing towards beta sooner.

Version numbers for the server after 0.7.0 are the owner's to choose. "Next major" in the owner's list is written here as **0.8.0-alpha**, the next minor number, as semantic versioning treats a minor bump below 1.0 as the breaking one.

Modules move on their own trains (see [Module release channels](design/module-release-channels.md)). A module's "next major" is its next minor number: WittyPlay 0.3.4 → the **0.4.x** series (WittyPlay moves in many small versions: see its timeline), WittyTV's parts ship with the server, WittyDVD 0.3.1 → **0.4.0**, WittyBluray 0.3.0 → **0.4.0**, WittyPhotos 0.3.0 → **0.4.0**.

### 0.6.3-alpha: incremental, small and safe

Full plan: [releases/0.6.3-alpha.md](releases/0.6.3-alpha.md).

| Item | Source |
|---|---|
| **Top priority: installs and runs with Windows Smart App Control on.** Every Windows file signed in CI, FFmpeg and game cores included; clear messages when something is still blocked. [Design](design/windows-smart-app-control.md), [pipeline](design/code-signing-pipeline.md) | Owner, 2026-11-02 (installer fails with it on) |
| **3D settings stay with the 3D disc:** the real cause of the stretched DVD pictures. Per-disc 3D, stuck values cleared, an on-screen chip. Clearer panel in 0.7.0. [Design](design/3d-settings-per-disc.md) | Owner, 2026-11-02 |
| **A reliability audit and overhaul:** the server went down playing music with online lyrics and the logs showed nothing. Evidence first (flight recorder, hang detector, stderr kept), reproduce and fix, then an audit of every area across 0.7.0 and 0.8.0. [Design](design/reliability-audit.md) | Owner, 2026-11-02 |
| **Logs that explain every stop:** a reason on every stop, how the last run ended on every start, the cache folder bug after a reset, support bundle fixes. Tray supervisor and a log timeline in 0.7.0. [Design](design/logging-improvements.md) | Owner, 2026-11-02 |
| **Updates you can find and use:** a banner and activity item for admins, readable "what's new", a button that downloads the right installer. One-click MSI installs in 0.7.0, the rest in the next major. [Design](design/server-updates.md) | Owner, 2026-11-02 |
| **One server at a time:** a second start asks whether to start another instance on another port, open the web UI, or exit | Owner's list |
| **Basic UI bugs and small improvements**, capped so it stays an incremental release | Owner's list |
| The final accessibility pass that 0.6.0-alpha left: screen reader, keyboard, every controller | 0.6.0 tracker |
| WittyMediaPlayer's release build fixed and published | 0.6.0 tracker |
| Small, already-known leftovers: flaky tests to settle, Dependabot recheck, Windows `go test` recheck | 0.6.0 tracker |
| WittyPhotos map: start the rebuild if it fits; otherwise it goes to WittyPhotos 0.4.0 | Owner's list ("might be for the next major release of the module") |

### 0.7.0-alpha: mainline

Full plan: [releases/0.7.0-alpha.md](releases/0.7.0-alpha.md).

| Item | Source |
|---|---|
| **Studios that are really one studio** (a streaming service and the same service sold through another store's channel) combine, with a manual combine and split in the UI | Owner's list |
| **Remote Access page that says plainly what is off** ("Only on your home network" shows "off" while it works) | Owner's list |
| **Changing a poster opens a real picker window**, centred and easy to use, on the web and on WittyMediaClient | Owner's list |
| **Language and subtitles on TV channels:** each viewer picks among every language and subtitle track in the shared stream (next major; tuner captions after). [Design](design/channel-audio-and-subtitles.md) | Owner, 2026-11-02 |
| **Preferred audio languages per person:** an ordered list with Original language, remembered per show on the server; requiring a language in the next major. [Design](design/audio-language-preferences.md) | Owner, 2026-11-02 |
| **Keep a title off TV channels:** a switch on any title, 3D titles left out by default, a per-channel exception. [Design](design/keep-off-channels.md) | Owner, 2026-11-02 |
| **One-click updates for Windows MSI installs:** download, verify, back up, install through the tray app or the service. [Design](design/server-updates.md) | Owner, 2026-11-02 |
| **Module API 4: Alpha, Beta and Stable channels for modules.** Every first-party module moves to the Alpha channel | Owner's list ("might apply when we do our next major alpha version") |
| WittyTV live sync fix, if it is ready (it ships in the server): see [WittyTV](modules/WittyTV.md) | Owner's list |
| WittyPlay's 0.4.x series, WittyDVD 0.4.0 and WittyBluray 0.4.0 land as modules on the Alpha channel when ready | Owner's list |

### 0.8.0-alpha: next major server version (not a priority yet)

Full plan: [releases/0.8.0-alpha-next-major.md](releases/0.8.0-alpha-next-major.md).

- Customise the order of libraries across the top bar and side menu, in one customisation menu.
- Right-click (and long-press, and the controller's menu button) on a library listing for its settings, edit metadata and refresh metadata.
- As many customisation options as the best media servers offer, and more: home rows, per-profile layouts, hidden libraries, pinned items, default sort per library, per-library looks.
- Watch together, the local AI plan, recordings in a series' folder, from the existing roadmap.

### WittyPlay: about seventy small releases to 1.0

**Every game runs on the server and is streamed; nothing is emulated in a browser, phone or TV** (the owner, 2026-11-02). The goal is to be as seamless as the leading commercial cloud gaming service, with a better interface. Full timeline, with a goal and a "done when" line for every version: [modules/WittyPlay.md](modules/WittyPlay.md).

| Series | Theme |
|---|---|
| 0.3.5 to 0.3.9 | Stabilise and measure: signed, pinned cores (Smart App Control), known faults, end-to-end stamps, a baseline |
| 0.4.0 to 0.4.12 | Quick wins and the play experience: sound, smoothness, insert animations and custom packs, core store and overrides, disc sets and swapping, server-side picture filters |
| 0.5.0 to 0.5.10 | The encoder host: shared memory, live bitrate, healing without keyframes, zero-copy, HEVC and AV1 |
| 0.6.0 to 0.6.10 | Transport and remote play: congestion control, error correction, relay on the HTTPS port, fallbacks, reconnect without losing the game |
| 0.7.0 to 0.7.9 | Seamless sessions: instant start, suspend and resume, hand-off between devices, a queue, presets, save history |
| 0.8.0 to 0.8.8 | Resources: hardware probe, automatic limits, admin overrides, degradation, priorities, several GPUs |
| 0.9.0 to 0.9.9 | Thin native clients on every platform |
| 0.10.0 to 0.10.9 | The dashboard interface |
| 0.11.0 to 0.11.6 | Playing together: co-op from your own device, spectating, clips, recordings |
| 0.12.0 to 0.12.5 | More consoles, licences checked |
| 1.0.0 | Proven by camera, on real hardware, signed everywhere |

Designs: [streaming and resources](design/wittyplay-streaming-performance.md), [insert animations](design/console-insert-animation.md).

### Every other module and app

Every module and app has an update plan of its own in [modules/](README.md#module-plans) and [apps/](README.md#app-plans). Each plan has a next incremental release, a next major, and a further-out list.

### Beta and 1.0: a long way off

Plan: [releases/beta-and-1.0.md](releases/beta-and-1.0.md). The beta comes only when every feature the owner wants is at least somewhat implemented, and it's where the owner said modules get "proper versioning support". Until then, backwards compatibility stays unofficial and limited, as decided for 0.6.0-alpha, and module release channels exist mainly so testers can choose how new they want things: while the server is alpha, almost everything sits on the alpha channel.

---

## Standing rules every plan here keeps

These come from the owner and are not up for change by any plan in this folder.

1. **Performance and security first.** Every feature must run without stutters on low-end hardware. Audit and trim touched code as it's built. Validate input and auth on every new route.
2. **Encrypted media stays refused.** No decryption, no bypass plugin, no hook. Licensed devices through their own documented interfaces are the route. WittyTuner can't be forked.
3. **No competitor names** in code, comments, docs, the README, the changelog or the UI. Describe features generically. VLC is the one credited exception, in WittyMediaPlayer.
4. **Licences:** the server and modules AGPL-3.0-or-later, client apps GPL-3.0-or-later; in forked work, upstream code keeps its own licence, labelled with the upstream project; third-party licences shown in the UI.
5. **Modules are their own projects:** each builds, signs and releases from its own repository. Nothing new may depend on the monorepo.
6. **Small version bumps as work lands.** A bump isn't a release. Tags never move.
7. **Every platform counts:** server on Windows, Linux, macOS, FreeBSD; clients add Android ARM, x86, TV, iOS, tvOS and Plasma Big Screen. A platform that can't be tried stays listed and is marked not run.
8. **Accessible everywhere:** keyboard, screen reader, TV remote and every kind of controller (XInput, DualShock and DualSense, Nintendo).
9. **The owner's machine is shared:** test on Linux first, never open native windows during tests, never rebuild over `dist\test-all`.
10. **Logos and pictures** of brands, networks, studios and consoles are optional downloads, with permission asked in setup and a notice on screen.
11. **Work is tracked on GitHub** (the owner, 2026-11-02): an issue before work starts, a milestone per release, a branch and pull request per issue that closes it, green CI before merging, and security reports kept private. Full rules: [GITHUB-PRACTICES.md](GITHUB-PRACTICES.md). Every order of work in this folder starts with opening the milestone and its issues.

---

## Where each request in the owner's list went

| Owner's line | Plan |
|---|---|
| Module API: Alpha, Beta, Stable channels; move internal modules to Alpha | [design/module-release-channels.md](design/module-release-channels.md), in 0.7.0-alpha |
| WittyDVD and WittyBluray images that match: disc images with a D and a B, no copyrighted logo | [design/module-icons.md](design/module-icons.md), [modules/WittyDVD.md](modules/WittyDVD.md), [modules/WittyBluray.md](modules/WittyBluray.md) |
| WittyTV: full screen retunes, gap, earlier time; sync across accounts; wrong channel selected on exit | [design/wittytv-live-sync.md](design/wittytv-live-sync.md), [modules/WittyTV.md](modules/WittyTV.md) |
| WittyPlay: insert animation, custom build option, documentation | [design/console-insert-animation.md](design/console-insert-animation.md) |
| WittyPlay: core store, cross-console override, custom core upload | [modules/WittyPlay.md](modules/WittyPlay.md#046--core-store--c-u) (0.4.6 to 0.4.8) |
| WittyPlay: multi-disc and custom dual-disc setups | [modules/WittyPlay.md](modules/WittyPlay.md#049--disc-sets--l) (0.4.9, 0.4.10) |
| WittyPlay: a clearer, expansive timeline with many small versions; everything on the server, nothing in the browser; as seamless as the leading cloud gaming service with a better interface | [modules/WittyPlay.md](modules/WittyPlay.md) |
| WittyPlay: optimisation and resource allocation, cloud-gaming class | [design/wittyplay-streaming-performance.md](design/wittyplay-streaming-performance.md) |
| 0.6.3: Windows Smart App Control blocks the app and its installer | [design/windows-smart-app-control.md](design/windows-smart-app-control.md), [design/code-signing-pipeline.md](design/code-signing-pipeline.md) |
| Change language and subtitles while watching a TV channel | [design/channel-audio-and-subtitles.md](design/channel-audio-and-subtitles.md), [releases/0.8.0-alpha-next-major.md](releases/0.8.0-alpha-next-major.md) |
| Prefer or require an audio language when a title has several tracks | [design/audio-language-preferences.md](design/audio-language-preferences.md), [releases/0.7.0-alpha.md](releases/0.7.0-alpha.md#preferred-audio-languages-per-person) |
| Let single titles (like a 3D Blu-ray) be unavailable to the TV guide | [design/keep-off-channels.md](design/keep-off-channels.md), [releases/0.7.0-alpha.md](releases/0.7.0-alpha.md#keeping-a-title-off-tv-channels) |
| 3D Blu-ray settings applied to every video (the stretched DVD pictures) | [design/3d-settings-per-disc.md](design/3d-settings-per-disc.md), [releases/0.6.3-alpha.md](releases/0.6.3-alpha.md#3d-settings-stay-with-the-3d-disc) |
| Server went down during music with lyrics; massive audit, overhaul, better logging | [design/reliability-audit.md](design/reliability-audit.md), [releases/0.6.3-alpha.md](releases/0.6.3-alpha.md#the-server-went-down-while-playing-music-with-lyrics-evidence-first-then-the-fix) |
| Server "crashed" and the logs don't say why; improve the log module | [design/logging-improvements.md](design/logging-improvements.md), [releases/0.6.3-alpha.md](releases/0.6.3-alpha.md#logs-that-explain-every-stop) |
| Updating isn't obvious in the UI; easier to find | [design/server-updates.md](design/server-updates.md), [releases/0.6.3-alpha.md](releases/0.6.3-alpha.md#updates-you-can-find-and-use) |
| 0.6.3: single instance | [design/single-instance.md](design/single-instance.md), [releases/0.6.3-alpha.md](releases/0.6.3-alpha.md) |
| 0.6.3: basic UI fixes, not too many | [releases/0.6.3-alpha.md](releases/0.6.3-alpha.md#small-ui-fixes) |
| WittyPhotos: world map rebuild | [design/photos-map-rebuild.md](design/photos-map-rebuild.md), [modules/WittyPhotos.md](modules/WittyPhotos.md) |
| 0.7: studio names | [releases/0.7.0-alpha.md](releases/0.7.0-alpha.md#studios-that-are-one-studio) |
| 0.7: Remote Access page wording | [releases/0.7.0-alpha.md](releases/0.7.0-alpha.md#remote-access-that-says-what-is-off) |
| Next major: library order, right-click menus, customisation | [design/library-navigation-customisation.md](design/library-navigation-customisation.md) |
| 0.7: poster picker as a real window, also in WittyMediaClient | [releases/0.7.0-alpha.md](releases/0.7.0-alpha.md#a-poster-picker-you-can-see) |
| Plans for every module, including ones not mentioned | [modules/](README.md#module-plans), [apps/](README.md#app-plans) |

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
