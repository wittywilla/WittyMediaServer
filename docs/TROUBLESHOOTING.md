# Troubleshooting

Quick answers to the problems people run into most. Still stuck? Open *Settings → WittyLog*: the viewer
explains common failures in plain words. Reproduce the problem, then attach a **support bundle** to an issue (it
never contains passwords or tokens, shows you what's inside first, and can hide titles and file paths). For anything
to do with discs or drives, run *Settings → WittyDiagnoser*.

| Problem | What to do |
|---|---|
| **The tray says “Stopped — address already in use”** | Another copy (or the installed service) uses port 9797. Stop it, or start the tray with `-listen :9798`. |
| **An app doesn't list the server** | Apps find it over UDP ports 5353 and 9797, which a firewall may block; Wi-Fi with client isolation or a guest network keeps devices apart; a container needs host networking. Check *Settings → Profiles → Signing in → Let apps find this server*. Typing the server's address in the app always works. |
| **Other devices at home can't open the server** | Use this computer's network address, not `localhost` (the setup wizard and *Settings → About* show it). Check that the server listens on every network card (`-listen :9797`, the default) and that the firewall allows the port. |
| **It doesn't work away from home** | That's expected: the server is on your home network only until you turn on *Settings → Remote Access*. See [Remote access](REMOTE.md), which also has [its own troubleshooting](REMOTE.md#when-it-doesnt-work). |
| **"This server is only available on its home network"** | Remote Access is off and the server sees you as outside. Turn it on from a device at home. Behind a reverse proxy, also list the proxy as trusted: see [REMOTE.md](REMOTE.md#a-reverse-proxy-you-already-run). |
| **“ffmpeg not found”** | Keep `bin/ffmpeg` next to the program, install FFmpeg on `PATH`, or set `WMS_FFMPEG`. *Settings → About* shows which one is used. |
| **Video stalls or has no sound in one browser** | Some Chromium builds lack AAC. Set *Settings → Transcoding → Stream audio codec* to MP3. |
| **“This disc is CSS-scrambled” / “encrypted (AACS)”** | The image is copy protected. Encrypted media isn't supported, and WittyMediaServer never decrypts anything or bypasses copy protection. |
| **A DVD or Blu-ray menu shows no buttons** | Run a check of the disc in *Settings → [WittyDiagnoser](DIAGNOSER.md)* with *Press every menu button* on, and attach its report to an issue (never the disc image). On the command line, `wms dvd walk disc.iso` or `wms bluray walk disc.iso` shows the same steps. |
| **A disc won't play, stutters or stops partway** | *Settings → [WittyDiagnoser](DIAGNOSER.md)* says what's wrong with a copy: scrambled, incomplete, or with sectors that can't be read. |
| **Video stalls, or a folder or drive misbehaves** | *Settings → [WittyDiagnoser](DIAGNOSER.md) → Storage* checks free space, permissions, read speed and known trouble such as a database on a network drive. |
| **You want to start again from a new install** | *Settings → Reset* puts the server back as it was first installed, with warnings first, and asks for your password. It keeps a copy you can bring back, unless you choose to delete everything. Your media files are never touched. Locked out, or the server won't start? Run `wms reset` on the server, then start it. `wms reset -list` and `-restore` work with the copies, and `-cancel` takes back a reset that's waiting. |
| **Seeking in a DVD title lands slightly off** | DVD video is seeked by estimated bit-rate, then FFmpeg decodes forward to the exact point; on very high-bitrate discs this can take a second. |
| **A title is named strangely** | Rename the file (`Title (Year).ext`), or use **Edit** on its page. Titles named by an earlier faulty parse are corrected on the next scan. |
| **Everything from one drive is still listed while it is away** | That's on purpose: a library folder that can't be reached, or is empty (an unmounted drive looks like that), keeps its items until it is back. Files really deleted from a folder that is there disappear at the next scan. |
| **A video waits and the log says the server is busy** | As many transcodes run as *maxTranscodes* allows, all with someone watching. Raise it in [wms.json](CONFIG.md) if the machine can take it. |
| **Posters are missing** | Add a free TMDB key, or a custom agent, in *Settings → Metadata agents*, then **Refresh** the title. |
| **Where is my data?** | The data folder (see [CONFIG.md](CONFIG.md)): `wms.db` is the database, `cache/images` the artwork, `transcode/` is temporary. |
| **Can I use an LGPL-only FFmpeg?** | Yes: nothing requires GPL parts. Point `WMS_FFMPEG` at an LGPL build with `libopenh264` or a hardware encoder. |

---
<sub>WittyMediaServer documentation · [Back to the README](../README.md)</sub>

## A game takes a long time to start the first time

On Windows, a security scanner may hold a program the first time it runs, which can take half a minute with the game
runner. It's quick after that.

The first start of a PSP game is also slow: the PSP core sets up its own files in that game's save folder, about 20 MB,
once per account for each game.
