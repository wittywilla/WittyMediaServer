# Open requests

See what the owner has asked for and where each stands, checked against the code on 2026-11-01 at `a051f40`. This is an internal list for the people working on the project, and isn't in the app's help.

**How far this was checked:** by reading the code and each worker's report, and not by using the features. "Done" means the work is merged. What wasn't seen working is in the third column, and everything that needs real hardware is in the [Roadmap](ROADMAP.md#what-hasnt-met-the-real-world).

## The 0.6.0-alpha list

| Request | State | Not seen, or left |
|---|---|---|
| Modules completely separate from the server | Done | `--apply` and the workflows have never run. WittyMediaPlayer's disc engine isn't on module API 3 yet |
| Modules start at this release, with limited unofficial compatibility | Done | Older-API refusal is unit tests only |
| A module store with search, and third-party stores with warnings | Done | Screenshots section and automatic updates not run in the UI |
| **Update All Modules** and readable errors | Done | |
| What's new on each module, with a plain line when there's nothing | Done | |
| A list of the module repositories to make | Done | [Repositories](REPOSITORIES.md) |
| Radio playing and the user button overlapping | Done | Radio sound not heard |
| TV interface filled out, with more than the target has | Done | Not every new dialog seen in both light and dark |
| Scalable TV preview, TV themes with cable box and modern looks | Done | Title art only in the modern look |
| DVD main feature and chosen scenes on WittyTV | Done | |
| Bumpers, mandatory or by title, and cutting commercials short | Done | |
| Better metadata editing and match fixing | Done | A real TMDB key not tried |
| Studio and network logos as an optional download, with notices | Done | Real logo downloads not tried |
| Automatic categories such as comedy and horror | Done | |
| Browse by type, network, studio and more, and local listings by streaming platform with disclosures | Done | |
| Profile names apart from usernames, with a choice of which shows | Done | |
| Poster movement on hover, with a way to turn it off | Done | **Moving posters**, in **Settings → Appearance** |
| Settings scroll bug | Done | |
| Top bar that stays manageable when big | Done | |
| A design standards document, shared parts and revamped themes | Done | [Design](DESIGN.md). 357 older design check problems remain, none new |
| Intense UI pass, cohesive across the app | In progress | Session `ui-final` |
| Setup wizard for all of this, friendly to beginners | In progress | Session `oobe` |
| Security audit, hardening and updated libraries | Done, second pass in progress | Session `security-second-pass`. `install.ps1` is gone, so the service is no longer SYSTEM |
| Code audit for Android, iOS and the client | Done | Android has run on emulators only. Apple never compiled |
| Portability to Apple TV and Android TV | Partly | Android TV has run on an emulator with keys only. Live TV and the guide aren't on native apps |
| Keyboard, screen reader and remote everywhere | In progress | No real screen reader, TV or controller tried |
| WittyPlay UI overhaul | Done | |
| Game lag, input delay and the graphics fault | Done | Delay figures are headless on one machine, not to the glass. The owner hasn't played it on a real screen. PS2 shader cache is rebuilt each run under the sandbox |
| Reliable, fast emulation | Partly | Every console's cores were played on Linux under the sandbox, through the real stream (see `GAMING.md`). No retail 3DS or Dreamcast game has been played. Real saves on Linux under the sandbox not tried for PS2 |
| 3DS CIA files | Done as far as it can be | CIA is read. The owner's file is really encrypted, so it's refused with a clear message. Only an unencrypted dump can be run |
| Importing all game data, and non-games tied to random consoles | Done | |
| Console icons, as an optional download | Done | PlayStation and PS2 have no logo, so they show text |
| Server stats module, customisable, all metrics | Done | No real TV, sensors or touch drag tried |
| Activity button for scans, read-only for non-admins, enabled in Settings | Done | Thumbnails and intro detection have no tasks of their own |
| ZIP backup and restore | Done | Upload and save buttons not seen |
| Documentation and safe editing for broken metadata | Done | |
| CLI and UI tools for metadata, modules, corrupt data and the install | Done | `wittymediaserver doctor`, **Settings → Repair** |
| Sanitized log download | Done | |
| Standard installers and full uninstallers for every product | Done | No real Windows or Mac. See [Installing](INSTALL.md#what-has-been-tested) |
| Clearer program names | Done | |
| Every media type and file extension | Done | Safari, phones, HEVC hardware and HDR screens not checked |
| Tdarr integration, and transcoding options | Done | Tdarr isn't open source now, so there's our own engine and an optional connection to the user's own Tdarr. Real Tdarr was never run, as downloading it binds whoever does so to its licence |
| Music UI, full screen, visualizer shuffle, lyrics and karaoke, with a free source and a setup option | Done, setup option in progress | |
| Year and other metrics kept | Done | |
| Product documentation revamped, in-app help that replaces GitHub | In progress | |
| GitHub, files and code comments cleaned up | Not started | Session `repo-cleanup` |
| Heavy optimisation | Not started | Comes last |

## Older requests still open

- **Windows and macOS installers:** a combined installer isn't started. The client, player and reader each have their own.
- **WittyMediaPlayer:** a real window, sound, disc menus by eye and anything with a physical drive wait for the owner. Other systems, Java menus and surround sound aren't started.
- **Client apps:** a way to reach the log file on Apple. Android has it under **Profile**.
- **Wholly new kinds of module:** an "in development" section on the Modules page exists, and it's only that. Forks of existing modules work.
- **A diagnostic app** for Windows and macOS with no server: not started.
- **A minimum hardware document:** not written.
- **Blu-ray on a real disc:** the sample projects on hand have no built disc or Java code. To try real authored output, the owner could build one with a free authoring tool.
- **Confirm by watching:** a reference disc's warning screen, interactive games and credits, and the DVD chapter-change fix. An NTSC disc is still to be tried.
- **CDs:** a real image from each ripping tool, and WittyCD images inside archives.
- **Show types:** with no API key, a show with no genres may stay empty, so channels by type miss it. Not rechecked.
- **WittyVisualizer:** a professional check of the flash limit, as it's measured against the WCAG figures with our own counter. Effects written by people work in the web app only.
- **README:** new screenshots, and how network and show type information is obtained.

Everything else that was on this list has been built, is in the [Roadmap](ROADMAP.md), or has gone: the empty TV guide, the hardware video test on **Settings → About**, commercial breaks inside programmes, disk space for recordings, the version number on phones, and MOBI and AZW3 in the reader.

## Decisions for the owner

- **Windows 7 for the desktop client:** it would need a second app on a browser engine that stopped getting security fixes in 2023. The recommendation so far is not to build it.
- **The app's ID, its visible name and its artwork** for the client apps. Android and Apple still say "WittyMedia".
- **Whether to host an apt and dnf repository,** and whether to buy code signing.
- **ScreenScraper secrets,** which would be public in the programs.
- Each worker's decisions that can be overruled are in `D:\alpha6-todo\TRACKER.md`.

## Owner steps

These need a person: see [Publishing the modules](MODULES-CUTOVER.md) and [Publishing a release](PUBLISHING.md).

- Make the module key, back up both keys, and move `WMS_SIGNING_KEY` to a `release` environment with a required reviewer.
- Rename `WittySunshine` and `WittyMoonlightHost`, create `WittyStats` and `WittyModuleCatalog`, and delete `WittyMoonlight`.
- Add a legal contact address.
- Run the Windows Sandbox test of each product's installer, and try the APK on a phone and a TV stick.
- Remove old builds from `dist/`. Look before deleting: one of those folders is a copy the owner has run in place, with its own data.

## Not open

These stay closed:

- **Encrypted broadcasts, discs and game files are never decrypted, and there's no plugin to get around it.** They're detected and refused with a clear message. Clear channels and licensed devices are the route.
- **Non-commercial emulator cores** aren't offered.
- **The module wikis were skipped by the owner.** The pages are written if that changes.

---

<sub>WittyMediaServer documentation</sub>
