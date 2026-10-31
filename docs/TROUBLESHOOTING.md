# Troubleshooting

Quick answers to the problems people run into most. Still stuck? Turn on the relevant category in
*Settings → Logs & privacy*, reproduce the problem, and attach the log (it never contains passwords
or session tokens) to an issue.

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
| **A DVD or Blu-ray menu shows no buttons** | Run `wms dvd walk disc.iso` or `wms bluray walk disc.iso` and attach the output to an issue (never the disc image). |
| **Seeking in a DVD title lands slightly off** | DVD video is seeked by estimated bit-rate, then FFmpeg decodes forward to the exact point; on very high-bitrate discs this can take a second. |
| **A title is named strangely** | Rename the file (`Title (Year).ext`), or use **Edit** on its page. Titles named by an earlier faulty parse are corrected on the next scan. |
| **Everything from one drive is still listed while it is away** | That's on purpose: a library folder that can't be reached, or is empty (an unmounted drive looks like that), keeps its items until it is back. Files really deleted from a folder that is there disappear at the next scan. |
| **A video waits and the log says the server is busy** | As many transcodes run as *maxTranscodes* allows, all with someone watching. Raise it in [wms.json](CONFIG.md) if the machine can take it. |
| **Posters are missing** | Add a free TMDB key, or a custom agent, in *Settings → Metadata agents*, then **Refresh** the title. |
| **Where is my data?** | The data folder (see [CONFIG.md](CONFIG.md)): `wms.db` is the database, `cache/images` the artwork, `transcode/` is temporary. |
| **Can I use an LGPL-only FFmpeg?** | Yes: nothing requires GPL parts. Point `WMS_FFMPEG` at an LGPL build with `libopenh264` or a hardware encoder. |

---
<sub>WittyMediaServer documentation · [Back to the README](../README.md)</sub>
