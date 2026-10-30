# Troubleshooting

Quick answers to the problems people run into most. Still stuck? Turn on the relevant category in
*Settings → Logs & privacy*, reproduce the problem, and attach the log (it never contains passwords
or session tokens) to an issue.

| Problem | What to do |
|---|---|
| **The tray says “Stopped — address already in use”** | Another copy (or the installed service) uses port 9797. Stop it, or start the tray with `-listen :9798`. |
| **Other devices at home can't open the server** | Use this computer's network address, not `localhost` (the setup wizard and *Settings → About* show it). Check that the server listens on every network card (`-listen :9797`, the default) and that the firewall allows the port. |
| **It doesn't work away from home** | That's expected: the server is on your home network only until you turn on *Settings → Remote Access*. See [Remote access](REMOTE.md), which also has [its own troubleshooting](REMOTE.md#when-it-doesnt-work). |
| **"This server is only available on its home network"** | Remote Access is off and the server sees you as outside. Turn it on from a device at home. Behind a reverse proxy, also list the proxy as trusted: see [REMOTE.md](REMOTE.md#a-reverse-proxy-you-already-run). |
| **“ffmpeg not found”** | Keep `bin/ffmpeg` next to the program, install FFmpeg on `PATH`, or set `WMS_FFMPEG`. *Settings → About* shows which one is used. |
| **Video stalls or has no sound in one browser** | Some Chromium builds lack AAC. Set *Settings → Transcoding → Stream audio codec* to MP3. |
| **“This disc is CSS-scrambled” / “encrypted (AACS)”** | The image is still protected. WittyMediaServer never decrypts anything; make a decrypted image of a disc you own with a tool that is legal where you live. |
| **A DVD or Blu-ray menu shows no buttons** | Run `wms dvd walk disc.iso` or `wms bluray walk disc.iso` and attach the output to an issue (never the disc image). |
| **Seeking in a DVD title lands slightly off** | DVD video is seeked by estimated bit-rate, then FFmpeg decodes forward to the exact point; on very high-bitrate discs this can take a second. |
| **A title is named strangely** | Rename the file (`Title (Year).ext`), or use **Edit** on its page. Titles named by an earlier faulty parse are corrected on the next scan. |
| **Posters are missing** | Add a free TMDB key, or a custom agent, in *Settings → Metadata agents*, then **Refresh** the title. |
| **Where is my data?** | The data folder (see [CONFIG.md](CONFIG.md)): `wms.db` is the database, `cache/images` the artwork, `transcode/` is temporary. |
| **Can I use an LGPL-only FFmpeg?** | Yes: nothing requires GPL parts. Point `WMS_FFMPEG` at an LGPL build with `libopenh264` or a hardware encoder. |

---
<sub>WittyMediaServer documentation · [Back to the README](../README.md)</sub>
