# Open requests

What the owner has asked for that isn't finished yet. It comes from reading the owner's messages and
checking each request against the code on 2026-10-30, on the merged test branch.

**How far this was checked:** one desktop browser with one quick phone-size check, using the media on
the development machine. No real TVs, tuners, retail Blu-rays or game cores were used. Requests from
before 2026-10-29 can't be seen, so this list can't cover them. Work still in progress on other branches may
close some of these.

## Fix first

- **The TV guide blanks the whole app when there are no channels.** The server sends an empty value
  instead of an empty list, and the page can't handle it. This happens on every new install that opens
  **TV guide** before making a channel. Fix it on the server and in the page.
- **Settings, About can say no hardware video was found when it was.** Opening the page starts the
  test, and the answer only shows the next time the page opens.

## Client apps

- **Finding a server and entering a security code:** being finished for desktop; Android and Apple are
  in progress.
- **Apps on every device:** Windows has run. Android compiles but has never run. Apple has never been
  compiled. Linux has never been launched. webOS, Tizen and Roku aren't started.
- **Matching the web app:** native apps only play video so far. Search, music, books, disc menus, games,
  channels and photos aren't started there.
- **Apple TV themes:** only the theme's colours are used, with no theme picker.
- **A server build for Android TV**, for boxes such as the NVIDIA Shield: not started.
- **Logs in the client apps:** logging, its folder and the link to open it exist only on the server.
- **Remotes, every game controller and screen readers:** written, but not tried on real devices.
- **Tidy-up:** the older `clients/desktop` and `clients/mobile` folders sit beside the new client, and
  the client has no published repository yet.

## Standalone apps and modules

- **WittyMediaPlayer**, with WittyDVD and WittyBluray as plugins: not started.
- **A real plugin system:** the modules are separate code, but they're still compiled into the server.
- **Updating modules on their own**, each with a minimum and maximum server version: in progress.
- **WittyMediaRemoteMedia:** the basics of connecting NAS libraries through a native or Docker app, in
  their own repository. Not started.
- **WittyReader desktop builds.**
- **Published repositories:** nothing is published under WittyPhotos, WittyPlay or WittyMediaClient yet.

## Installers and packages

- **Windows installers:** the server's is done, with a silent mode. Still to do: a client installer, a
  combined installer, and an option to install the portable version.
- **Linux packages:** native AMD64 and ARM64 packages for Fedora, Ubuntu and Arch, plus Flatpak, Snap and
  other popular distributions. Only general archives exist today.
- **A minimum hardware document:** realistic specifications for the server and for disc playback.

## Disc tools and engines

- **A diagnostic file:** one report that collects what's useful for fixing a disc problem, with nothing
  copyrighted in it. Today the tools only print to the terminal.
- **A small Windows and macOS app** that makes that file: not started.
- **Clearer program names:** nothing in the package explains what each program does.
- **Blu-ray:** secondary audio, picture-in-picture and MVC 3D decoding. Testing on retail discs, including
  Java menus, 4K and HDR, waits for discs from the owner. Neither Blu-ray image on hand has Java menus.
- **BD-J on a real disc:** the blu-disc.net examples are project files for an authoring tool, with no built
  disc or Java code to run, so BD-J was tested with open-source sample applications instead. To try real
  authored output, the owner could build `DEMO_menu` with the free Blu-Disc Studio Lite. What needs
  a disc: real menus' timing with video, and how far the API classes written from the published specs match.
- **Confirm by watching:** the Shrek 2 warning screen, the interactive games and the Idol credits have
  fixes, but the owner hasn't confirmed them. The same goes for the DVD chapter-change fix: on Shrek 2, play
  straight through 2:20 to 2:35 and 33:55 to 36:45. Only PAL discs could be measured, so an NTSC disc is
  still to be tried.

### Audio CDs

WittyCD is in, on its branch. Still to do:

- **CD images inside archives,** once the archive code is merged: it needs a real folder from `gamearchive.Cache`, and nothing else.
- **CD+G from a raw `.sub` file,** whose subcode is interleaved. A `.cdg` beside the image and `CDG` sectors work.
- **Double-byte CD-TEXT,** Japanese for example, which needs character tables. It is reported, and names come from elsewhere.
- **A real image from each ripping tool.** There were none to test with, so the formats were checked with generated images: CHD and TOC against the real
  tools, CCD and NRG only against their published descriptions.
- **WittyMediaPlayer and the native apps** can use the routes in `docs/CLIENT-API.md`, but none has a CD player yet.

## Live TV and recording

- **Recordings in the series' own folder:** an admin toggle to put a recording in the show's folder. Today
  there's one chosen folder, with a new folder per title.
- **Guide data:** make it easier to get, and mention it in the setup wizard. It needs no API key, but
  people have to find their own XMLTV address.
- **More tuners:** Windows BDA and DiSEqC, a viewer and a recording sharing one tuner, and testing on real
  hardware.
- **Show types:** with no API key, a show with no genres stays empty even when the metadata service gives
  a type such as Animation, so channels by type miss it.
- **Commercial breaks inside programmes:** commercials only play between programmes today.

## Games

- **Real games:** real cores haven't run end to end. The owner's game files are now on the test machine.
- **Every browser:** on plain http, some browsers hide gamepads and limit video features. In-browser play
  is designed but not built.

## Libraries

- **Photos:** people and places, and batch rotate and edit.
- **Books:** MOBI and AZW3 are listed, but no code to open them was found.
- **Local AI:** the future plan document, covering how it helps the server and its users, with image
  recognition for photos as an example. The placeholders are in.

## Interface and documents

- **README screenshots:** retake them at 1920×1080 from the current interface, including the guide, games
  and photos.
- **Navigation:** navigation items can't be rearranged by dragging (home widgets can), and the setup wizard
  doesn't offer "both".
- **"Preview not available":** only disc scenes show it. Other failed thumbnails go blank.
- **Version number:** it can't be seen with top-only navigation or on phones.
- **README:** explain how network and show type information is obtained, or link to the page that does.
- **UI review:** a detailed review of every page, edge cases and accessibility is in progress.

## Housekeeping

- **Remove old builds** from `dist/`. Look before deleting: one of those folders is a copy the owner has run
  in place, with its own data.
- **Push** the finished work to GitHub, when the owner is happy with it.

## Decisions for the owner

- **Windows 7 for the desktop client:** it would need a second app on a browser engine that stopped getting
  security fixes in 2023. The recommendation so far is not to build it.
- **Claude working continuously:** a recurring task doesn't exist yet and is waiting for a yes or no.
- **The app's ID, its visible name and its artwork** for the client apps.

## Not open

These stay closed:

- **Encrypted broadcasts are never decrypted, and there is no plugin to bypass encryption.** Clear channels
  and licensed devices are the route.
- **Copy-protected discs are refused.**
- **The module wikis were skipped by the owner.** The pages are written if that changes.
