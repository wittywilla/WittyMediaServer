# Transcoding guide

WittyMediaServer sends a file to the browser **as it is** whenever the browser can play it
(*direct play*). Everything else — MKV, AVI, HEVC, interlaced video, DVDs, files with burned-in
subtitles — is converted on the fly to H.264 + AAC HLS by FFmpeg. This guide explains the
settings under **Settings → Transcoding** and which choices suit which hardware.

- [Direct play](#direct-play)
- [Hardware encoders](#hardware-encoders)
- [Quality modes](#quality-modes) — [Easy](#easy) · [Advanced](#advanced) · [Expert](#expert)
- [Which profile for which machine](#which-profile-for-which-machine)
- [NVIDIA NVENC by GPU](#nvidia-nvenc-by-gpu)
- [AMD AMF (VCE / VCN) by GPU](#amd-amf-vce--vcn-by-gpu)
- [Intel Quick Sync by CPU](#intel-quick-sync-by-cpu)
- [Apple, Linux VAAPI and Raspberry Pi](#apple-linux-vaapi-and-raspberry-pi)
- [Troubleshooting](#troubleshooting)

> The tables are a practical summary for choosing settings, compiled from vendor documentation
> and FFmpeg behaviour. Exact feature sets vary with drivers and board vendors; the
> **Hardware encoders** list in Settings shows what actually works on *your* machine, because
> WittyMediaServer test-encodes with every encoder when it starts.

## Direct play

**Allow direct play** (on by default) streams the original file with HTTP range requests when:

- the container is MP4, M4V, MOV or WebM, and
- the video is H.264, VP9 or AV1, and
- the first audio track is AAC, MP3, Opus or Vorbis, and
- the default audio track and no subtitles are selected, and
- the video is not taller than *Maximum resolution*.

Direct play costs no CPU and keeps the original quality. Turn it off to force every file through
the transcoder — useful to cap bandwidth with a *bitrate cap* (for viewers away from home, use the
limits in *Settings → Remote Access*, which leave playback at home alone), or to compare quality
profiles.

## Hardware encoders

| Setting | Encoder | Works with |
|---|---|---|
| NVIDIA NVENC | `h264_nvenc` | GeForce / RTX / Quadro, Maxwell (GTX 750) and newer, Windows and Linux |
| Intel Quick Sync | `h264_qsv` | Intel CPUs with integrated graphics, Intel Arc, Windows and Linux |
| Apple VideoToolbox | `h264_videotoolbox` | every Mac (Intel with T2/iGPU, Apple silicon) |
| AMD AMF | `h264_amf` | Radeon GPUs and Ryzen APUs on **Windows** |
| VAAPI | `h264_vaapi` | Intel and AMD GPUs on **Linux/FreeBSD** (`/dev/dri/renderD128`) |

Tick the ones you want. When several are ticked, the first *working* one in the list above is
used; if none works, WittyMediaServer falls back to software (libx264). Encoders that FFmpeg
was built with but that your hardware or driver lacks are shown greyed out with the reason in
their tooltip.

Hardware encoding is roughly 5–20× cheaper on the CPU than libx264 and lets small machines
serve several streams. Software x264 still gives the best quality per bit; with a fast CPU and
few viewers, *Quality* on software looks best.

Docker: pass the GPU into the container (`--device /dev/dri` for VAAPI/Quick Sync,
`--gpus all` with the NVIDIA container toolkit for NVENC).

## Quality modes

### Easy

Three tested profiles. They apply to libx264 and are mapped onto each hardware encoder.

| Profile | x264 preset | Quality (CRF) | Stereo audio | Relative CPU | Pick it when |
|---|---|---|---|---|---|
| **Speed** | superfast | 23 | 160 kb/s | 0.5× | low-power machines, many viewers, fastest start/seek |
| **Balanced** (default) | veryfast | 21 | 192 kb/s | 1× | almost everyone |
| **Quality** | medium | 19 | 256 kb/s | ~3× | strong CPU or a modern GPU, few viewers, big screens |

### Advanced

| Control | What it does | Recommendation |
|---|---|---|
| Encoder speed | x264 preset; NVENC `p1`–`p7`, Quick Sync `veryfast`–`slow`, AMF `speed`/`balanced`/`quality` are derived from it | `veryfast` for real-time; `medium` only on 8+ cores or GPUs |
| Quality level | x264 CRF 16–30 (lower = better, bigger); mapped to NVENC `-cq`, QSV `-global_quality`, VAAPI/AMF `-qp`, VideoToolbox `-q:v` | 18–21 at home, 23–26 for phones / remote |
| Bitrate cap | `-maxrate`/`-bufsize` | 4 Mb/s for 720p over the internet, 8 Mb/s for 1080p, none on the LAN |
| Stereo audio bitrate | AAC bitrate | 160–192 kb/s; 256 kb/s for concerts / music |

### Expert

Everything in Advanced, plus two raw FFmpeg fields:

- **Encoder options** replace everything WittyMediaServer would put after `-c:v <encoder>`.
  Examples:
  - libx264, film archive: `-preset slow -crf 19 -tune film -x264-params aq-mode=3`
  - NVENC, high quality: `-preset p6 -tune hq -rc vbr -cq 21 -b:v 0 -spatial-aq 1 -temporal-aq 1`
  - Quick Sync: `-preset slower -global_quality 20 -look_ahead 1`
  - VAAPI: `-qp 20 -quality 1`
- **Extra output options** are inserted before the HLS muxer options, e.g. `-threads 4`.

Keyframes are still forced on segment boundaries so seeking keeps working. Quotes group words
(`-x264-params "aq-mode=3:psy-rd=1.0"`). A wrong option makes transcodes fail; start the server
with `-log debug` to see every FFmpeg command line.

Only options that tune encoding are accepted, each with a plain value: presets, tunes, profiles,
quality and bitrate settings, GOP settings, threads, `-pix_fmt`, `-movflags`, `-x264-params` and
`-x265-params` (without parameters that name a file), and so on. Anything that can open or write
files or reach the network (`-i`, output names, `-vf`, `-filter_complex`, `-attach`...) is refused
when you save, because a web account must not be able to read or change files on the machine that
way. The full list is `expertOptions` in `internal/ffmpeg/encoders.go`.

## How much a transcode costs

The server stops FFmpeg once it is 6 minutes ahead of the viewer and starts it again as they near
that point, so a fast machine doesn't convert a whole film for someone who stops after a minute.
Segments more than 10 minutes behind the viewer are removed, so a long film stays small on disk.
FFmpeg stops 90 seconds after the last request (a paused player), and when the player closes.

The transcode folder (`transcodeDir`) is cleaned at every start, of the stream folders the server
made itself; anything else in it stays.

## Which profile for which machine

| Machine | Encoder | Profile | Max resolution | Notes |
|---|---|---|---|---|
| Raspberry Pi 4 / 5 | software | Speed | 720p | enable *Low-power mode for DVDs*; DVDs (SD) run in real time; avoid 1080p HEVC sources |
| Intel N100 / N95 / N305, J4125, N5105 NAS | Quick Sync (VAAPI on Linux) | Balanced | 1080p | several 1080p streams at once |
| Older quad-core (Core i5 4th–7th gen, Ryzen 1000) | Quick Sync if present, else software | Balanced (software: Speed for 2+ viewers) | 1080p | |
| Modern 6–8 core desktop (Ryzen 5 5600+, Core i5-12400+) | software or Quick Sync | Quality (1–2 viewers) / Balanced | Source | |
| NVIDIA GTX 10 series | NVENC | Balanced | Source | Pascal NVENC is fine at CQ 21–23 |
| NVIDIA RTX 20/30/40/50, GTX 1660/Super/Ti | NVENC | Quality | Source | Turing+ NVENC rivals x264 *medium* |
| AMD Radeon RX 5000 – 9000 (Windows) | AMF | Balanced | Source | AMD H.264 needs a bit more bitrate; CQ 20 |
| Mac (any) | VideoToolbox | Balanced | Source | Apple silicon handles many streams |

## NVIDIA NVENC by GPU

H.264 NVENC is on every GPU below except where noted. Consumer GeForce drivers limit the number
of simultaneous encode sessions (recent drivers allow 8; older ones 3–5); professional cards
are not limited.

| Architecture | Typical GPUs | H.264 | HEVC | AV1 | Quality (H.264) | Notes |
|---|---|---|---|---|---|---|
| Kepler | GTX 650–780, GTX Titan | ✔ | – | – | low | not supported by current drivers / FFmpeg NVENC; use software |
| Maxwell 1st gen | GTX 750 / 750 Ti, GTX 9xxM (some) | ✔ | – | – | low | old driver branch only |
| Maxwell 2nd gen | GTX 950–980 Ti, GTX Titan X | ✔ | 8-bit | – | fair | |
| Pascal | GTX 1050 – 1080 Ti, Titan Xp (**not GT 1030**) | ✔ | 8/10-bit | – | good | GT 1030 has no NVENC |
| Volta / Turing (TU117) | GTX 1650 (non-Super) | ✔ | 8/10-bit | – | good | GTX 1650 uses the older Volta-era NVENC |
| Turing | GTX 1650 Super, 1660 / Super / Ti, RTX 2060 – 2080 Ti | ✔ | 8/10-bit, B-frames | – | very good (≈ x264 medium) | best value used cards for transcoding |
| Ampere | RTX 3050 – 3090 Ti, RTX A-series | ✔ | 8/10-bit | – | very good | same encoder as Turing |
| Ada Lovelace | RTX 4060 – 4090 | ✔ | 8/10-bit | ✔ | very good | two encoders on RTX 4070 Ti and up |
| Blackwell | RTX 5060 – 5090 | ✔ | 8/10-bit, 4:2:2 | ✔ | excellent | more encoders on higher tiers |

## AMD AMF (VCE / VCN) by GPU

AMF (`h264_amf`) works on Windows. On Linux use **VAAPI** with the Mesa driver for the same
hardware.

| Engine | Typical GPUs / APUs | H.264 | HEVC | AV1 | Notes |
|---|---|---|---|---|---|
| VCE 1.0 – 2.0 | HD 7000, R7 260, R9 290/390, Kaveri APUs | ✔ | – | – | legacy; quality is low, prefer software |
| VCE 3.x | R9 285/380/Fury, RX 400/500 (Polaris) | ✔ | 8-bit (3.4) | – | |
| VCE 4.x | RX Vega 56/64, Radeon VII | ✔ | 8-bit | – | |
| VCN 1.0 | Ryzen 2000G/3000G APUs (Raven Ridge, Picasso) | ✔ | 8-bit | – | |
| VCN 2.x | RX 5000 (RDNA1), Ryzen 4000/5000G APUs | ✔ | 8/10-bit | – | |
| VCN 3.0 | RX 6000 (RDNA2), Ryzen 6000 APUs | ✔ | 8/10-bit | decode only | **RX 6500 XT / 6400 have no encoder** |
| VCN 4.0 | RX 7000 (RDNA3), Ryzen 7040/8040 APUs (Radeon 780M) | ✔ | 8/10-bit | ✔ | much improved H.264 quality |
| VCN 5.0 | RX 9000 (RDNA4) | ✔ | 8/10-bit | ✔ | best AMD quality so far |

## Intel Quick Sync by CPU

Quick Sync is part of the integrated GPU. **CPUs ending in "F" (e.g. i5-12400F) and most
"KF" models have no iGPU and therefore no Quick Sync.** On Linux use **VAAPI** (`intel-media-driver`)
or Quick Sync; both use the same hardware.

| Generation | Example CPUs | GPU | H.264 | HEVC encode | AV1 | Notes |
|---|---|---|---|---|---|---|
| 2nd gen (Sandy Bridge) | i5-2500, i7-2600 | HD 2000/3000 | ✔ | – | – | first Quick Sync; too old for current drivers on Linux |
| 3rd gen (Ivy Bridge) | i5-3470, i7-3770 | HD 2500/4000 | ✔ | – | – | |
| 4th gen (Haswell) | i5-4590, i7-4790 | HD 4600 | ✔ | – | – | |
| 5th gen (Broadwell) | i7-5775C | Iris 6200 | ✔ | – | – | |
| 6th gen (Skylake) | i5-6500, i7-6700 | HD 530 | ✔ | 8-bit | – | |
| 7th gen (Kaby Lake) | i5-7500, i7-7700 | HD 630 | ✔ | 8/10-bit | – | first with 10-bit HEVC |
| 8th–10th gen (Coffee/Comet Lake) | i5-8400, i7-9700, i5-10400 | UHD 630 | ✔ | 8/10-bit | – | very common in home servers |
| 10th gen mobile (Ice Lake) | i7-1065G7 | Iris Plus (Gen11) | ✔ | 8/10-bit | – | |
| 11th gen (Tiger / Rocket Lake) | i5-11400, i7-1165G7 | Xe / UHD 750 | ✔ | 8/10-bit | decode | |
| 12th–14th gen (Alder / Raptor Lake) | i3-12100, i5-13500, i7-14700 | UHD 730/770, Iris Xe | ✔ | 8/10-bit | decode | excellent for multiple 1080p streams |
| Core Ultra 1 (Meteor Lake) | Core Ultra 5 125H | Arc (Xe-LPG) | ✔ | 8/10-bit | ✔ | |
| Core Ultra 200 (Arrow / Lunar Lake) | Core Ultra 5 245K | Xe-LPG / Xe2 | ✔ | 8/10-bit | ✔ | |
| Atom / Celeron / Pentium | J4105/J4125 (Gemini Lake), N5105 (Jasper Lake) | UHD 600/605/Jasper | ✔ | 8/10-bit | – | popular NAS chips |
| Intel Processor N-series | N100, N95, N305 (Alder Lake-N) | UHD (Xe, 24–32 EU) | ✔ | 8/10-bit | decode | best low-power choice |
| Intel Arc discrete | A310, A380, A750, A770, B580 | Xe-HPG / Xe2 | ✔ | 8/10-bit | ✔ | great dedicated transcoding cards |

## Apple, Linux VAAPI and Raspberry Pi

- **Apple VideoToolbox**: every Mac since about 2012 has a hardware H.264 encoder; Apple silicon
  (M1 and later) encodes many 1080p streams at once with little power. Use *Balanced*.
- **VAAPI (Linux/FreeBSD)**: covers Intel (iHD driver) and AMD (Mesa radeonsi) GPUs.
  WittyMediaServer uses `/dev/dri/renderD128`; the server user needs access to it (usually the
  `render` or `video` group). In Docker add `--device /dev/dri`.
- **Raspberry Pi**: the Pi 5 has no hardware H.264 encoder, and the Pi 4's (V4L2 M2M) encoder is
  not used by WittyMediaServer. Use software with *Speed*, *Maximum resolution 720p* and
  *Low-power mode for DVDs*. Direct play matters most here: MP4/H.264/AAC files cost nothing.

## Troubleshooting

| Symptom | Try |
|---|---|
| An encoder is greyed out although the GPU is present | update the GPU driver; on Linux check permissions on `/dev/dri`; NVENC needs the NVIDIA driver, not nouveau; restart WittyMediaServer to re-test |
| Playback stutters / buffers | Speed profile, a lower *Maximum resolution*, a hardware encoder, or a bitrate cap for remote viewers |
| Picture is blocky | lower the quality level (CRF), raise the bitrate cap, or use Quality |
| Hardware encoding fails mid-stream | too many NVENC sessions on a GeForce card; untick NVENC or upgrade the driver |
| Transcodes fail after changing Expert options | clear both Expert fields; check the log with `-log debug` |

---
<sub>WittyMediaServer 0.5.0-alpha documentation</sub>
