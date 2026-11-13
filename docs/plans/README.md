# Future plans

> **Status on 2026-11-04**
>
> **Copied into the repository on 2026-11-04** ([#96](https://github.com/wittywilla/WittyMediaServer/issues/96)), so the plans travel with the code. This folder is now where they're kept: change them here, through pull requests. The owner's own list, `futureplans1.txt`, stays outside the repository, as it names other products. Every file starts with its status: **done**, **in progress** (with its issue) or **planned** (an idea in [Ideas](https://github.com/wittywilla/WittyMediaServer/discussions/categories/ideas), with no release yet).
>
> **0.6.3-alpha was not made.** After the urgent 0.6.1-alpha and 0.6.2-alpha, its items moved into the [0.7.0-alpha milestone](https://github.com/wittywilla/WittyMediaServer/milestone/1), which started on 2026-11-04. Read "0.6.3" in these plans as "0.7.0-alpha" where an issue says so.

Plans, roadmaps and design documents for WittyMediaServer, its modules and its apps, written on 2026-11-02, the day 0.6.0-alpha was published. Built from the owner's list (`futureplans1.txt`, kept outside the repository) and from what the 0.6.0-alpha work already knows.

**Nothing here has been built, tested or measured for these documents.** Guesses are marked **to verify**. The project stays in alpha until every feature the owner wants is at least somewhat implemented.

Start with the roadmap.

---

## The big picture

| File | What it is |
|---|---|
| [ROADMAP.md](ROADMAP.md) | The release train, every request in the owner's list and where it went, and the standing rules |
| [GAME-PLAN.md](GAME-PLAN.md) | How the work gets done: the shape of each release, the order, what runs side by side, where the owner is needed |
| [GITHUB-PRACTICES.md](GITHUB-PRACTICES.md) | Issues first, milestones, branches and pull requests, labels, releases, security, and catching up after 0.6.0 |
| [IDEAS.md](IDEAS.md) | Further upgrades and overhauls, small to large, not scheduled |
| [WEBSITE.md](WEBSITE.md) | Rebuilding wittymediaserver.com as plain files for Hostinger, in the product's look, kept current from GitHub (written 2026-11-03) |

## Design 2 (0.8.0-alpha)

| File | What it is |
|---|---|
| [DESIGN-2.md](DESIGN-2.md) | The contract: principles, the fourteen looks, the removal list |
| [DESIGN-2-RELEASE.md](DESIGN-2-RELEASE.md) | The release cut: the freeze, the gates, the issue candidates and the release checklist |
| [DESIGN-2-SDUI-GAPS.md](DESIGN-2-SDUI-GAPS.md) · [DESIGN-2-FEATURE-GAPS.md](DESIGN-2-FEATURE-GAPS.md) · [DESIGN-2-FEATURE-GAPS-CLIENTS.md](DESIGN-2-FEATURE-GAPS-CLIENTS.md) | What the catalogue and the apps lacked, and where each gap went |
| [DESIGN-2-QA-1.md](DESIGN-2-QA-1.md) to [DESIGN-2-QA-5.md](DESIGN-2-QA-5.md) | The QA passes: defects, owners and what is open |
| [TV-MODE.md](TV-MODE.md) · [module-isolation.md](module-isolation.md) | TV mode, and how modules are kept apart |

The design itself (the boards, pictures and tables) is in [docs/design-2](../design-2/README.md).

## Server releases

| File | Release |
|---|---|
| [releases/0.6.3-alpha.md](releases/0.6.3-alpha.md) | Incremental: single instance, about 15 UI fixes, the accessibility pass, leftovers |
| [releases/0.7.0-alpha.md](releases/0.7.0-alpha.md) | Mainline: studios, Remote Access page, poster picker, Module API 4 with channels |
| [releases/0.8.0-alpha-next-major.md](releases/0.8.0-alpha-next-major.md) | Next major: library order, right-click menus, deep customisation, watch together |
| [releases/beta-and-1.0.md](releases/beta-and-1.0.md) | The beta gate (every wanted feature at least somewhat implemented) and what "proper versioning" means |

## Design documents

| File | For |
|---|---|
| [design/windows-smart-app-control.md](design/windows-smart-app-control.md) | **Top priority for 0.6.3:** installing and running with Windows Smart App Control on |
| [design/code-signing-pipeline.md](design/code-signing-pipeline.md) | Signing every Windows (and later macOS) file in CI, the certificate options, the verify gate |
| [design/channel-audio-and-subtitles.md](design/channel-audio-and-subtitles.md) | Choosing language and subtitles while watching a TV channel, without changing it for anyone else (next major) |
| [design/audio-language-preferences.md](design/audio-language-preferences.md) | Preferred audio and subtitle languages per person, remembered per show, and requiring a language (0.7.0 and the next major) |
| [design/keep-off-channels.md](design/keep-off-channels.md) | A switch to keep any title (a 3D Blu-ray, say) off TV channels, with 3D titles left out by default (0.7.0) |
| [design/3d-settings-per-disc.md](design/3d-settings-per-disc.md) | Why DVDs looked stretched (a 3D setting saved for everything), and keeping 3D to the disc it was chosen for |
| [design/reliability-audit.md](design/reliability-audit.md) | **The case for a massive reliability audit and overhaul:** the music and lyrics incident, what to audit and how, a flight recorder and hang detector, and the rules the overhaul applies |
| [design/logging-improvements.md](design/logging-improvements.md) | What the 2026-11-02 "crash" was, and logs that explain every stop, start and fault (0.6.3 to the next major) |
| [design/server-updates.md](design/server-updates.md) | Updates that are easy to find, readable notes, and a real button for every kind of install (0.6.3 to the next major) |
| [design/single-instance.md](design/single-instance.md) | One server per data folder, and the question on a second start (0.6.3) |
| [design/module-release-channels.md](design/module-release-channels.md) | Alpha, Beta and Stable channels; every first-party module on alpha (Module API 4) |
| [design/wittytv-live-sync.md](design/wittytv-live-sync.md) | Full screen without retuning, viewers in sync, guide focus |
| [design/console-insert-animation.md](design/console-insert-animation.md) | WittyPlay's cartridge and disc animations, custom packs, `wittyplay-pack`, docs |
| [design/wittyplay-streaming-performance.md](design/wittyplay-streaming-performance.md) | **The big one:** latency, sound, hitching, picture faults, streaming options, resource allocation, remote, measuring, testing many games |
| [design/module-icons.md](design/module-icons.md) | Matching disc icons with D and B, clear of real logos |
| [design/photos-map-rebuild.md](design/photos-map-rebuild.md) | Finding what's broken in the photo map, then rebuilding it |
| [design/library-navigation-customisation.md](design/library-navigation-customisation.md) | Arranging libraries, context menus, customisation |

## Module plans

Every module has one.

| File | Modules |
|---|---|
| [modules/WittyPlay.md](modules/WittyPlay.md) | **WittyPlay's timeline, 0.3.5 to 1.0.0:** about seventy small versions, everything on the server and streamed, with a scorecard, tracks and a "done when" for each |
| [modules/WittyTV.md](modules/WittyTV.md) | WittyTV (guide and channels, in the server) |
| [modules/WittyDVD.md](modules/WittyDVD.md) | WittyDVD |
| [modules/WittyBluray.md](modules/WittyBluray.md) | WittyBluray |
| [modules/WittyPhotos.md](modules/WittyPhotos.md) | WittyPhotos |
| [modules/WittyTuner.md](modules/WittyTuner.md) | WittyTuner |
| [modules/WittyCD.md](modules/WittyCD.md) | WittyCD |
| [modules/WittyDisc.md](modules/WittyDisc.md) | WittyDisc |
| [modules/Readers-WittyEbook-WittyPDF-WittyComics-WittyInk.md](modules/Readers-WittyEbook-WittyPDF-WittyComics-WittyInk.md) | WittyEbook, WittyPDF, WittyComics, WittyInk |
| [modules/WittyCouch.md](modules/WittyCouch.md) | WittyCouch, WittyCouchHost, WittyRigs |
| [modules/Platform-WittyModule-WittyModuleCatalog-WittyIntegratedFileBrowser.md](modules/Platform-WittyModule-WittyModuleCatalog-WittyIntegratedFileBrowser.md) | WittyModule, WittyModuleCatalog, WittyIntegratedFileBrowser |
| [modules/WittyStats.md](modules/WittyStats.md) | WittyStats |
| [modules/WittyVisualizer.md](modules/WittyVisualizer.md) | WittyVisualizer |
| [modules/Built-in-WittyVideo-WittyChannels-WittyShorts-WittyAudio-WittyPodcasts.md](modules/Built-in-WittyVideo-WittyChannels-WittyShorts-WittyAudio-WittyPodcasts.md) | The built-ins, including music, radio, lyrics and karaoke |

## App plans

| File | Apps |
|---|---|
| [apps/WittyMediaClient.md](apps/WittyMediaClient.md) | WittyMediaClient on every platform |
| [apps/WittyMediaPlayer.md](apps/WittyMediaPlayer.md) | WittyMediaPlayer, starting with its broken release build |
| [apps/WittyReader-and-smaller-apps.md](apps/WittyReader-and-smaller-apps.md) | WittyReader, WittyMediaRemoteMedia, a WittyDiagnoser app |

---

## Decisions waiting for the owner

1. **Studios:** combine closely related names (this plan's reading), or keep them apart? [0.7.0](releases/0.7.0-alpha.md#studios-that-are-one-studio)
2. **Another port:** offered only with its own data folder, as planned? [single instance](design/single-instance.md#a-decision-for-the-owner)
3. **Disc icons:** identical except the letter, or a slight warm/cool tint? A matching C for WittyCD? [icons](design/module-icons.md)
4. **Module API:** keep API 3 modules working through 0.7.x and raise the minimum at 0.8.0? [channels](design/module-release-channels.md#wittymodule-changes-api-4)
5. **Streaming:** benchmark hardware, your screens and camera, remote testing by UDP or relay, latency or steadiness first, games or transcodes first. [questions](design/wittyplay-streaming-performance.md#19-questions-for-the-owner)
6. **Code signing (start first, it takes the longest):** which certificate, and whose name appears as the publisher in Windows' prompts. [pipeline](design/code-signing-pipeline.md#getting-a-certificate)
7. **Game cores:** build and distribute the offered cores ourselves, pinned and signed, so games work with Smart App Control on? [design](design/windows-smart-app-control.md#2-game-cores-build-them-ourselves-pinned-and-signed)
8. **GitHub catch-up:** go-ahead to run the labels, templates and milestones script across every repository, and to open issues for what 0.6.0 left. [practices](GITHUB-PRACTICES.md#catching-up-after-060-alpha)

---

<sub>Future plans, written 2026-11-02.</sub>
