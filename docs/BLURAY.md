# Blu-rays

Use this guide to add Blu-ray images and folders to your server, play them with their real menus in a browser, and decide about discs with Java menus. The module that does this is **WittyBluray**, an original implementation of the logic a player runs.

- [Adding discs](#adding-discs)
- [Playing a disc](#playing-a-disc)
- [Sound, subtitles and chapters](#sound-subtitles-and-chapters)
- [3D, 4K and HDR](#3d-4k-and-hdr)
- [BD-J (Java) menus](#bd-j-java-menus)
- [Java and licensing](#java-and-licensing)
- [On a small machine](#on-a-small-machine)
- [Diagnosing a disc](#diagnosing-a-disc)
- [Limits](#limits)
- [For developers](#for-developers)

## Adding discs

1. Go to **Settings → Modules** and add **WittyBluray**. WittyDisc, which reads disc images, comes inside it. See [Modules and the store](MODULE-STORE.md).
2. Go to **Settings → Libraries → Add library**, and choose **DVDs, Blu-rays & CDs**. Images of films can also go in a **Movies** library, and images of a series in a **TV shows** library.
3. Choose the folder that holds your discs, and press **Add library**.

It takes `.iso` and `.img` images, among those listed in [Supported formats](SUPPORTED-FORMATS.md), and folders that hold `BDMV`. AVCHD discs and camcorder cards, and BDAV recordings, are read too.

A scan recognises a Blu-ray by its structure, not its name, and records its label, the feature's length, resolution, codec, languages, chapters, and whether it has menus. 3D discs get a "3D" genre. Without WittyBluray, a disc stays listed with nothing known about what's in it, and playing it offers to add the module. Adding it scans your disc libraries.

**Encrypted discs are refused.** An AACS-encrypted disc is listed with this message, and can't be played: "This disc is encrypted, so it can't be played: encrypted media isn't supported, and nothing here bypasses copy protection." The check looks at the video itself, not at folder names. BD+ is never run. An unencrypted disc just plays.

## Playing a disc

On a Blu-ray's page:

| Button | What happens |
|---|---|
| **Insert Blu-ray** | Starts the disc like a player: first-play, studio intros and warnings, then the top menu. Optionally with the tray animation and drive sound |
| **Play movie**, or **Play from the start** on a series | Straight into the feature |
| **Resume from** | Back to where you stopped |
| **Main menu** | The disc's top menu, skipping the intros |

**Disc settings** on the page, for administrators, chooses what's on the disc, whether inserting it starts with the menu or the feature, and the insert effect. The defaults, the region, parental limits and country are under **Settings → Playback & discs**. Pick the region your discs are from: a disc for another region shows its own message instead of the movie.

**In a menu,** the arrow keys, the on-screen remote, a game controller or a TV remote move between buttons, and **OK** presses one. The disc's own program then runs, as its authors intended. **Top menu** returns to the top menu from the film.

**Pop-up menus.** When the film carries one, the **Menu** key, or **M**, opens it over the picture while the film keeps playing. The arrows and OK work it, and **Esc** or **Menu** closes it. Discs use these for chapters, sound and subtitle choices with their own artwork.

Menu buttons make the disc's own sounds, pages play their transitions, and menus that move on by themselves after a while do so. Keys and the remote work as they do for [DVDs](DVD.md#the-remote-and-keyboard).

A disc can forbid seeking or skipping at some points, as on a disc player. To ignore that, turn off **Disc restrictions** in the player's viewing settings.

## Sound, subtitles and chapters

- **Sound:** every track in the feature is listed, with its language and channels: DTS-HD Master Audio, TrueHD, DTS, Dolby Digital, Dolby Digital Plus and PCM. It's converted for the browser.
- **Subtitles:** the disc's own subtitle pictures, drawn over the video at their exact positions and times, in any of the disc's languages. Forced ones, which translate on-screen text, show even with subtitles off. **Forced captions** in the viewing settings turns them off.
- **Chapters:** the playlist's chapter marks.
- **Angles:** scenes with several angles play the one you choose, with **Angle**.

## 3D, 4K and HDR

- **3D discs (MVC)** play their 2D view, because FFmpeg has no decoder for the other. The disc is marked 3D in the library.
- **Side-by-side and over-under 3D**, on discs or files made that way: the viewing settings have **3D video**, and **Show 3D as**: one eye in 2D, or red and cyan for paper glasses.
- **4K UHD discs** are read like any other. With **Maximum resolution** on **Source**, they're sent as 1080p, because converting 4K in software is beyond most machines. Choose **2160p (4K)** under **Settings → Playback & discs → Conversion** to send them as 4K. No real UHD disc has been tried.
- **HDR** (HDR10, HDR10+ and the HDR10 layer of Dolby Vision) is mapped to an ordinary picture for the browser. An FFmpeg without the `zscale` and `tonemap` filters plays it unmapped: watchable, but flat. HDR screens haven't been checked.

## BD-J (Java) menus

Many commercial Blu-rays write their menus in Java. **This is off until you turn it on.** With it off, or with no Java, these discs play their main feature directly, and chapters, sound and subtitles are still there in the player's own controls.

To show those menus:

1. Install any Java 8 or newer on the server, with its graphics libraries. [Eclipse Temurin](https://adoptium.net) is a free build of OpenJDK. A stripped-down Java without graphics can't run them. On Linux, install `fontconfig` too. In Docker, add both to the image.
2. Go to **Settings → Playback & discs** and turn on **Run Blu-ray Java menus (BD-J)**. The page shows which Java it found.
3. If it found none, or you want another, name it in the server's [config file](CONFIG.md) as `javaPath`, or with `-java` or `WMS_JAVA`, and start the server again. **It can't be set from the web page**, so that a web account can't choose what the server runs. A location saved in Settings by an older version is ignored.
4. Insert the disc.

Each viewer's disc starts its own small Java process, with a 192 MB heap, which ends with the session. The disc's graphics reach the player as pictures at up to 12 a second, so fast animation looks less smooth than on a hardware player.

**How far this has been tested.** It has run with our own test discs and with open-source sample applications, **not yet on a retail disc**. None of the discs tried so far has Java menus. If an application can't run, the disc plays its main feature instead.

**Safety.** A BD-J disc contains code. It runs in a sandbox with no network and no access to files outside the disc's own areas, and a hostile test disc tries to break out every time the tests run. If a disc uses too much memory, too many threads or too much processor time, its Java process is ended and the feature plays. That's a strong barrier and not a guarantee, so turn this on only for discs you trust, and run the server as an unprivileged user. [The sandbox in detail](BLURAY-QUIRKS.md#bd-j).

## Java and licensing

- **Java isn't shipped.** The server and the module contain no Java runtime. Whoever runs the server installs one, under that Java's own licence, and it runs as a separate program.
- **The runtime is ours.** `runtime.jar` is original code under Apache-2.0, with no code from Sun, Oracle or other players. It provides the classes disc applications link against, so that discs can run.
- **It isn't certified.** The runtime hasn't passed any compatibility test and makes no claim to. It doesn't call itself Java, and "BD-J" and "Blu-ray" here only describe which discs it plays.
- **Copy protection is untouched.** AACS and BD+ are never decrypted or run.

## On a small machine

- Only the bytes being played are read. Nothing is copied to disk.
- **Settings → Playback & discs → Low-power mode for discs** uses the cheapest deinterlacer and the fastest encoder setting.
- A hardware encoder helps most: see [Live transcoding](TRANSCODING.md#hardware-encoders).
- Subtitle pictures are decoded in two-minute windows as the viewer reaches them, so a 20 GB film is never scanned whole.

## Diagnosing a disc

**Settings → WittyDiagnoser** checks a disc and makes a report to attach, with nothing copyrighted in it: see [WittyDiagnoser](DIAGNOSER.md).

When reporting a problem, attach that report, and never the disc image.

## Limits

- AACS-encrypted discs are refused, and BD+ is not run.
- 3D discs play in 2D, and 4K discs are sent as 1080p unless you choose 2160p.
- Secondary audio and picture-in-picture are not mixed in.
- BD-J graphics reach the player at up to 12 pictures a second.
- It has been tested on custom-authored discs. Retail discs use more of the format: BD-J, 3D and Ultra HD HDR are written, and have only been tested on synthetic data or not at all.
- On FreeBSD and macOS, the module's helper program runs without a sandbox. The BD-J limits above are separate from that.

## For developers

WittyBluray is a project of its own. Everything the format can do, and what happens with each, is in [Blu-ray disc quirks](BLURAY-QUIRKS.md).

**Looking inside a disc.** The server's old `bluray` command has gone. The tool now comes from the WittyBluray project, and needs Go 1.26 or newer:

```bash
go install github.com/wittywilla/wittybluray/cmd/wittybluray@latest
```

```bash
wittybluray info     MY_DISC.iso          # titles, program, playlists, clips, streams, 3D and HDR flags
wittybluray playlist MY_DISC.iso 800      # every item, stream and chapter of one playlist
wittybluray menu     MY_DISC.iso 800      # a playlist's menu: pages, buttons and every command
wittybluray walk     MY_DISC.iso          # simulate inserting the disc: the menu's buttons and commands
wittybluray walk     MY_DISC.iso 2 5      # then press buttons 2 and 5
wittybluray render   MY_DISC.iso out/     # the first menu's pages as PNG files
wittybluray bdj      MY_DISC.iso 20 a.png down enter  # BD-J titles; with Java, run them for 20 seconds, pressing those keys
```

Their output holds no video or artwork, so it's safe to attach to a bug report. If a BD-J disc falls back to its feature, `bdj` usually says why.

**How it works:**

1. WittyDisc reads the image directly, with no mounting, including the UDF 2.50 metadata partition Blu-rays use.
2. The disc's program is parsed: `index.bdmv`, `MovieObject.bdmv`, the playlists and the clip information.
3. A virtual machine runs the disc's commands, with 4096 general and 128 player status registers.
4. A navigator follows first play, menus and titles as the commands dictate, including calls, resumes and timers.
5. For a menu, it reads the interactive graphics stream and renders the page and each button's highlight. For a film, it reads the presentation graphics stream for subtitles.
6. On the server, `internal/bdsession` converts each playlist's video to HLS with FFmpeg from the clip's byte ranges, and the player puts the button and subtitle layers on top.

Everything is written from the public Blu-ray structure. The engine runs as a helper program, so it can update without a new server. Refusing encrypted discs stays with the server, on purpose, so a different helper can't change it.

---

<sub>WittyMediaServer documentation</sub>
