# Live transcoding

Use this guide to choose the settings under **Settings → Live transcoding**, which decide what happens while someone watches a video their device can't play as it is. Your files aren't changed, and the work stops when they stop watching.

To convert files ahead of time, see [Library transcoding](LIBRARY-TRANSCODING.md).

- [What happens when you press play](#what-happens-when-you-press-play)
- [The settings page](#the-settings-page)
- [Hardware encoders](#hardware-encoders)
- [Quality modes](#quality-modes)
- [Which choice for which machine](#which-choice-for-which-machine)
- [How much a conversion costs](#how-much-a-conversion-costs)
- [NVIDIA NVENC by GPU](#nvidia-nvenc-by-gpu)
- [AMD AMF by GPU](#amd-amf-by-gpu)
- [Intel Quick Sync by CPU](#intel-quick-sync-by-cpu)
- [Apple, Linux and Raspberry Pi](#apple-linux-and-raspberry-pi)
- [If it isn't working](#if-it-isnt-working)

## What happens when you press play

Your player tells the server what it can do, and the server takes the simplest way that plays, in this order:

1. **Direct play.** Your player opens the file as it is. This costs no processor work and keeps the original quality.
2. **Rewrapped.** Only the wrapper is the trouble, such as an H.264 film in MKV for a browser. The picture and sound are copied into a stream, which costs next to nothing.
3. **Sound converted.** Only the sound is the trouble, such as DTS or TrueHD in MKV. The sound alone is converted to AAC, and the picture is copied.
4. **Converted.** Nothing simpler would play, so the picture is converted to H.264 too.

The **How this is playing** button in the player, or the **I** key, says which of these is happening, and why.

**HDR.** A screen that shows HDR, on a player that can decode it, gets it as it is. Anything else gets the picture mapped to fit an ordinary screen. That needs FFmpeg's `zscale` and `tonemap` filters. Without them the picture plays unmapped: flat and dim, but watchable.

**Subtitles.** Text subtitles are shown by your player. Subtitles stored as pictures, from discs, are drawn into the picture, which means converting it.

This hasn't been checked on Safari, on phones, with hardware decoding of HEVC or on HDR screens.

## The settings page

| Setting | What it does |
|---|---|
| **Allow direct play** | On by default. Off sends every file through conversion, which is mainly useful for testing. |
| **Maximum resolution** | Taller videos are converted down to this while playing, even ones that could otherwise play as they are. 720p is about half the work of 1080p. The default, **Source**, is no limit, except that 4K Blu-ray discs go as 1080p: choose **2160p (4K)** to send them as 4K. |
| **Sound format while converting** | AAC works nearly everywhere. Choose MP3 only for a browser that can't play AAC, such as some Linux builds of Chromium. |
| **Hardware encoders** | Below. |
| **Picture quality** | [Easy, Advanced or Expert](#quality-modes). These apply to streams started after saving. |

For viewers away from home, the limits in [Remote access](REMOTE.md) leave playback at home alone, and they always win.

How many conversions can run at once is set with `maxTranscodes` in the [config file](CONFIG.md). When that many are running and all have someone watching, the next viewer is told the server is busy. A viewer who has asked for nothing for 20 seconds gives up their place if someone else needs it, and carries on when they come back.

## Hardware encoders

Your graphics chip can convert video for a fraction of the processor work, which lets a small machine serve several streams. Software gives the best quality for the size and uses the most processor.

| On the page | Works with |
|---|---|
| **NVIDIA NVENC** | GeForce, RTX and Quadro cards, on Windows and Linux |
| **Intel Quick Sync** | Intel CPUs with built-in graphics, and Intel Arc, on Windows and Linux |
| **Apple VideoToolbox** | Macs. This has not been run on a real Mac yet |
| **AMD AMF** | Radeon cards and Ryzen APUs, on Windows |
| **VAAPI** | Intel and AMD graphics on Linux, through `/dev/dri/renderD128`. FreeBSD also tries it, and it has not been run there |

1. Go to **Settings → Live transcoding → Hardware encoders**.
2. Tick the ones you want. The first that works, in the order above, is used. With none ticked, it's all software.
3. Press **Save changes**.

Each is tested when the server starts, and the page says what it found:

| What it says | What it means |
|---|---|
| **Working on this machine** | Ready to tick |
| **Not found on this machine** | FFmpeg has it, and your hardware or driver doesn't. Hover over it for the reason |
| **Not included in this FFmpeg build** | Your FFmpeg wasn't built with it |
| **Not supported on this operating system** | Such as VideoToolbox on Linux |

**In Docker,** pass the graphics chip into the container: `--device /dev/dri` for VAAPI and Quick Sync, or `--gpus all` with the NVIDIA container toolkit for NVENC.

## Quality modes

### Easy

Three profiles, which also apply to hardware encoders through their nearest settings.

| Profile | Software setting | Quality level | Stereo sound | Best for |
|---|---|---|---|---|
| **Speed** | superfast | 23 | 160 kb/s | Raspberry Pi, Intel N100 and J-series, NAS boxes, laptops on battery, several viewers at once |
| **Balanced** (the default) | veryfast | 21 | 192 kb/s | Any desktop or laptop from the last 8 years or so, or any working hardware encoder |
| **Quality** | medium | 19 | 256 kb/s | 6 or more cores, or an NVIDIA or Intel hardware encoder. About three times the processor work of Balanced |

### Advanced

| Control | What it does | Try |
|---|---|---|
| **Encoder speed** | Faster settings use less processor and need more data for the same picture. Hardware encoders use their own nearest setting. | `veryfast` while watching. `medium` needs 8 or more cores or a hardware encoder |
| **Quality level** | 16 to 30. Lower looks better and sends more data. | 18 to 21 at home, 23 to 26 for phones and slow connections |
| **Bitrate limit** | The most data the picture may use. | 4 Mb/s for 720p and 8 Mb/s for 1080p over the internet |
| **Stereo sound bitrate** | Surround sound always uses 384 kb/s. | 160 to 192 kb/s, or 256 kb/s for music |

### Expert

Everything in Advanced, plus two fields for FFmpeg options:

- **Encoder options** replace the encoder's options. For example:
  - Software, film archive: `-preset slow -crf 19 -tune film -x264-params aq-mode=3`
  - NVENC, high quality: `-preset p6 -tune hq -rc vbr -cq 21 -b:v 0 -spatial-aq 1 -temporal-aq 1`
  - Quick Sync: `-preset slower -global_quality 20 -look_ahead 1`
  - VAAPI: `-qp 20 -quality 1`
- **Extra options** are added to the output, such as `-threads 4`.

Keyframes stay on segment boundaries, so seeking keeps working. Quotes group words: `-x264-params "aq-mode=3:psy-rd=1.0"`. A wrong option makes conversions fail, and **Settings → WittyLog** explains why. Starting the server with `-log debug` shows every FFmpeg command line.

Only options that tune encoding are accepted, each with a plain value: presets, tunes, profiles, quality and bitrate settings, keyframe settings, threads, `-pix_fmt`, `-movflags`, and `-x264-params` and `-x265-params` without parameters that name a file. Anything that can open or write files or reach the network, such as `-i`, output names, `-vf`, `-filter_complex` or `-attach`, is refused, because a web account must not be able to read or change files on the machine that way.

## Which choice for which machine

| Machine | Encoder and profile | Maximum resolution |
|---|---|---|
| Raspberry Pi 4 or 5 | Software, Speed. Turn on **Low-power mode for discs**. Avoid 1080p HEVC files | 720p |
| Intel N100, N95, N305, J4125, N5105 | Quick Sync (or VAAPI on Linux), Balanced | 1080p |
| Older quad-core (Core i5 4th to 7th generation, Ryzen 1000) | Quick Sync if it has it, Balanced. Software: Speed for 2 or more viewers | 1080p |
| Modern 6 to 8 cores (Ryzen 5 5600 and up, Core i5-12400 and up) | Software or Quick Sync. Quality for 1 or 2 viewers, otherwise Balanced | Source |
| NVIDIA GTX 10 series | NVENC, Balanced | Source |
| NVIDIA RTX 20 to 50, GTX 1660 | NVENC, Quality | Source |
| AMD Radeon RX 5000 to 9000, on Windows | AMF, Balanced. Its H.264 needs a bit more data | Source |
| Mac | VideoToolbox, Balanced | Source |

Direct play matters most on small machines: MP4 files with H.264 and AAC cost nothing to play.

## How much a conversion costs

- FFmpeg stops once it's about 6 minutes ahead of the viewer, and starts again as they near that point, so a fast machine doesn't convert a whole film for someone who stops after a minute.
- Parts more than about 10 minutes behind the viewer are removed, so a long film stays small on disk.
- FFmpeg stops 90 seconds after the last request, such as a paused player, and when the player closes.
- The transcode folder (`transcodeDir` in the [config file](CONFIG.md)) is cleaned of the server's own stream folders at every start. Anything else in it stays.

## NVIDIA NVENC by GPU

This table and the two below are a summary for choosing hardware, from the makers' documents and how FFmpeg behaves. Features vary with drivers and board makers, and **Hardware encoders** in Settings shows what works on your machine. Live transcoding uses H.264. Library transcoding can also use H.265 and AV1.

Consumer GeForce drivers limit how many streams encode at once: recent ones allow 8, older ones 3 to 5. Professional cards aren't limited.

| Family | Typical cards | Encodes | Notes |
|---|---|---|---|
| Kepler | GTX 650 to 780 | H.264 | Not supported by current drivers and FFmpeg. Use software |
| Maxwell, first generation | GTX 750, 750 Ti | H.264 | Low quality. Old driver branch only |
| Maxwell, second generation | GTX 950 to 980 Ti | H.264, 8-bit H.265 | Fair quality |
| Pascal | GTX 1050 to 1080 Ti | H.264, 8 and 10-bit H.265 | Good quality. The GT 1030 has no NVENC |
| Volta-era | GTX 1650 (not Super) | H.264, 8 and 10-bit H.265 | Good quality |
| Turing | GTX 1650 Super, 1660, RTX 2060 to 2080 Ti | H.264, 8 and 10-bit H.265 | Very good: close to software on `medium` |
| Ampere | RTX 3050 to 3090 Ti | H.264, 8 and 10-bit H.265 | Same encoder as Turing |
| Ada Lovelace | RTX 4060 to 4090 | H.264, 8 and 10-bit H.265, AV1 | Two encoders on the RTX 4070 Ti and up |
| Blackwell | RTX 5060 to 5090 | H.264, 8 and 10-bit H.265, AV1 | More encoders on higher models |

## AMD AMF by GPU

AMF works on Windows. On Linux, use **VAAPI** with the Mesa driver for the same hardware.

| Engine | Typical cards and APUs | Encodes | Notes |
|---|---|---|---|
| VCE 1 and 2 | HD 7000, R7 260, R9 290 and 390 | H.264 | Low quality. Prefer software |
| VCE 3 | R9 285, 380, Fury, RX 400 and 500 | H.264, 8-bit H.265 on VCE 3.4 | |
| VCE 4 | RX Vega 56 and 64, Radeon VII | H.264, 8-bit H.265 | |
| VCN 1 | Ryzen 2000G and 3000G | H.264, 8-bit H.265 | |
| VCN 2 | RX 5000, Ryzen 4000 and 5000G | H.264, 8 and 10-bit H.265 | |
| VCN 3 | RX 6000, Ryzen 6000 | H.264, 8 and 10-bit H.265 | The RX 6500 XT and 6400 have no encoder |
| VCN 4 | RX 7000, Ryzen 7040 and 8040 | H.264, 8 and 10-bit H.265, AV1 | Much better H.264 |
| VCN 5 | RX 9000 | H.264, 8 and 10-bit H.265, AV1 | AMD's best so far |

## Intel Quick Sync by CPU

Quick Sync is part of the built-in graphics. **CPUs ending in F, such as the i5-12400F, and most KF models have none, so they have no Quick Sync.** On Linux, VAAPI with `intel-media-driver` and Quick Sync use the same hardware.

| Generation | Example CPUs | Encodes | Notes |
|---|---|---|---|
| 2nd to 5th (Sandy Bridge to Broadwell) | i5-2500, i7-3770, i5-4590, i7-5775C | H.264 | 2nd is too old for current Linux drivers |
| 6th (Skylake) | i5-6500, i7-6700 | H.264, 8-bit H.265 | |
| 7th (Kaby Lake) | i5-7500, i7-7700 | H.264, 8 and 10-bit H.265 | |
| 8th to 10th (Coffee and Comet Lake, Ice Lake) | i5-8400, i7-9700, i5-10400, i7-1065G7 | H.264, 8 and 10-bit H.265 | Very common in home servers |
| 11th (Tiger and Rocket Lake) | i5-11400, i7-1165G7 | H.264, 8 and 10-bit H.265 | |
| 12th to 14th (Alder and Raptor Lake) | i3-12100, i5-13500, i7-14700 | H.264, 8 and 10-bit H.265 | Excellent for several 1080p streams |
| Core Ultra | Core Ultra 5 125H, 245K | H.264, 8 and 10-bit H.265, AV1 | |
| Atom, Celeron, Pentium | J4105, J4125, N5105 | H.264, 8 and 10-bit H.265 | Popular NAS chips |
| N-series | N100, N95, N305 | H.264, 8 and 10-bit H.265 | The best low-power choice |
| Arc cards | A310, A380, A750, A770, B580 | H.264, 8 and 10-bit H.265, AV1 | Great dedicated cards for this |

## Apple, Linux and Raspberry Pi

- **Apple VideoToolbox.** Macs have a hardware H.264 encoder, and Apple silicon encodes many 1080p streams at once with little power. Use Balanced. This has not been run on a real Mac yet.
- **VAAPI on Linux.** Covers Intel (iHD driver) and AMD (Mesa radeonsi). The server's user needs access to `/dev/dri/renderD128`, usually through the `render` or `video` group.
- **Raspberry Pi.** The Pi 5 has no hardware H.264 encoder, and this server doesn't use the Pi 4's. Use software with Speed, 720p and **Low-power mode for discs**.

## If it isn't working

| What you see | Try |
|---|---|
| An encoder is greyed out although the card is there | Update the driver. On Linux, check permissions on `/dev/dri`. NVENC needs NVIDIA's driver, not nouveau. Restart the server to test again |
| Playback stutters or buffers | The Speed profile, a lower **Maximum resolution**, a hardware encoder, or a bitrate limit for remote viewers |
| The picture is blocky | A lower quality level, a higher bitrate limit, or the Quality profile |
| Hardware encoding fails partway | Too many NVENC streams on a GeForce card. Untick NVENC, or update the driver |
| Conversions fail after changing Expert options | Clear both Expert fields, and look in **Settings → WittyLog** |
| HDR looks flat and dim | Your FFmpeg has no tone mapping filters. [Supported formats](SUPPORTED-FORMATS.md) covers how HDR plays |

---

<sub>WittyMediaServer documentation</sub>
