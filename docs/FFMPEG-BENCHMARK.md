# Measuring FFmpeg

How to measure what FFmpeg costs the server when something plays, so that a change to how it's run can be judged by numbers before and after. This is for people working on the server; it changes nothing for someone using it.

- [What it measures](#what-it-measures)
- [Running it](#running-it)
- [Settings](#settings)
- [Reading the numbers](#reading-the-numbers)
- [Figures](#figures)

## What it measures

`TestFFmpegBench` in `internal/server/ffbench_real_test.go` plays real files the way a viewer does: it asks `POST /api/items/{id}/playback` (or opens a disc's title, or starts a channel's live session), fetches the playlist and the first segment, then the next segments in order, and then seeks. It runs a real server with its own data folder and reads the media without writing anything beside it. It does nothing unless `WMS_FFBENCH=1` is set.

| Case | What plays |
|---|---|
| `remux-h264-1080p` | A 1080p H.264 film in Matroska, as a browser gets it: the picture copied into HLS, the sound converted to AAC |
| `x264-1080p` | The same film converted to H.264 at its own size |
| `x264-1080p-to-720p-4M` | The same, at 720p within 4 Mb/s, as for a slow remote connection |
| `hevc-1080p-to-h264` | A 1080p HEVC episode converted to H.264 |
| `tonemap-4k-hdr-to-1080p` | A 4K HDR film, mapped to an ordinary picture at 1080p |
| `burn-text-subtitles` | A film with a subtitle file beside it, drawn into the picture |
| `burn-picture-subtitles` | A film with picture subtitles (Blu-ray or DVD style), drawn into the picture |
| `audio-transcode` | A long sound file converted to AAC for a player that can't play it |
| `live-channel-1080p` | A TV channel's live session on a 1080p file, for 30 seconds at viewing speed |
| `dvd-title` | A DVD image's main title, through the WittyDVD module |
| `bluray-title` | A Blu-ray image's main title, through the WittyBluray module |

Each case looks through the media folders for the first file that fits: the right codec and size, at least two minutes long. Encrypted discs are passed over, since the server refuses them. A case with nothing to play is skipped. The disc cases need the real modules (see [Running it](#running-it)) and are skipped without them.

## Running it

It reads `D:\WittyDemoMedia` and `D:\ServerMedia` on Windows, and `/mnt/d/WittyDemoMedia` and `/mnt/d/ServerMedia` elsewhere, unless told otherwise. From the repository's folder:

On Windows, in Git Bash:

```bash
WMS_FFBENCH=1 WMS_FFMPEG=$PWD/.tools/ffmpeg-master-latest-win64-gpl/bin/ffmpeg.exe WMS_FFPROBE=$PWD/.tools/ffmpeg-master-latest-win64-gpl/bin/ffprobe.exe go test ./internal/server -run TestFFmpegBench -v -count=1 -timeout 120m
```

On Linux, macOS and FreeBSD, with FFmpeg on the `PATH`:

```bash
WMS_FFBENCH=1 go test ./internal/server -run TestFFmpegBench -v -count=1 -timeout 120m
```

The table comes at the end. For the disc cases, make a catalogue of the two modules first (in WSL, or Git Bash on Windows, which builds them for Windows) and name it in `WMS_TEST_CATALOG`:

```bash
scripts/make-test-catalog.sh /tmp/wms-catalog /mnt/d/projects WittyDVD WittyBluray
```

A whole run with three runs of each case took 40 minutes on the laptop under [Figures](#figures), with other work on it.

**To compare two versions,** build each one's test program and run them one after the other on the same machine with the same settings, ideally twice, alternating. Other work on the machine changes the timings a lot; it changes memory much less.

```bash
go test -c -o bench-before.exe ./internal/server
```

## Settings

| Variable | What it does | Default |
|---|---|---|
| `WMS_FFBENCH` | Runs it at all | off |
| `WMS_FFMPEG`, `WMS_FFPROBE` | Which FFmpeg | the one beside the program, then the `PATH` |
| `WMS_FFBENCH_MEDIA` | Media folders to look in, in order, separated as the `PATH` is (`;` on Windows, `:` elsewhere) | see above |
| `WMS_FFBENCH_RUNS` | Runs of each case | 3 |
| `WMS_FFBENCH_SEGMENTS` | Segments timed after the first, for the speed | 8 (32 seconds) |
| `WMS_FFBENCH_ONLY` | A regular expression: only the cases it matches | all |
| `WMS_FFBENCH_SETTINGS` | Server settings as an administrator would set them, such as `hwEncoders=h264_nvenc,lowPower=1`. Naming hardware encoders tests them first, as the server does | none: libx264 |
| `WMS_FFBENCH_OUT` | A JSON file for every run's numbers, the machine and the FFmpeg version | none |
| `WMS_TEST_CATALOG` | The module catalogue, for the disc cases | none |

## Reading the numbers

Each cell is the median of the runs, with the lowest and highest in brackets.

| Column | Means |
|---|---|
| **Start ms** | The request that sets playing up: probing the file and deciding. For a disc it's opening it and reaching its title |
| **First segment ms** | From that request to the first segment in hand, which is about how long a viewer waits for the picture |
| **Speed ×** | Times real time, over the segments after the first. A copy is held to 4 times real time on purpose, and a channel to real time |
| **CPU s per min** | Processor seconds, over every core, that FFmpeg and ffprobe used for each minute of what was played. 60 means one core kept busy |
| **Peak MB** | The most memory the largest FFmpeg process held at any moment (peak working set on Windows, `VmHWM` on Linux) |
| **Seek ms** | The time to the first segment after a jump to 40% and to 70% of the way in, on average. A copied stream starts a new stream there, as players do |

Things that mislead:

- **The first run of a case** reads the file from disk; the others find it in the system's cache. A film nobody has opened lately behaves like the first run.
- **Other work on the machine** slows FFmpeg down: a build, a scan or other tests. Run comparisons back to back and report the spread. On a busy Windows machine even starting FFmpeg can take seconds, because the 170 MB program is scanned each time.
- **A copy's CPU** includes converting its sound, and the subtitle tracks a web player is offered, which are read in the background from the whole file.
- **Memory grows with the machine's core count.** FFmpeg and libx264 start more threads on a bigger machine, and each thread holds pictures.

## Figures

Measured on a laptop with a 12th-generation Intel Core i7-12700H (14 cores, 20 threads) and 64 GB of memory, Windows 11, FFmpeg N-128142 (2026-10-28), libx264, with other work running on it.

These are for 0.6.2-alpha, with the machine's processors fully busy with other work the whole time, so the times are slow and spread out. Processor seconds and memory hardly change with that. On a quieter moment, the same 1080p conversion had its first segment in 1 second and ran at 6.7 times real time.

| Case | Decision | First segment ms | Speed × | CPU s per min | Peak MB | Seek ms |
|---|---|---|---|---|---|---|
| `remux-h264-1080p` | remux | 5102 (4010–8065) | 4.1 | 3.3 | 36 | 6658 (3355–7080) |
| `x264-1080p` | transcode | 9892 (6016–13965) | 1.3 (0.5–2.2) | 66.8 | 880 | 5555 (5014–8415) |
| `x264-1080p-to-720p-4M` | transcode | 3484 (3471–4620) | 2.1 | 46.3 | 440 | 2938 (2235–3877) |
| `hevc-1080p-to-h264` | transcode | 4838 (3088–6466) | 1.6 | 80.6 | 903 | 4966 (3695–7063) |
| `tonemap-4k-hdr-to-1080p` | transcode | 11522 (10443–12962) | 0.5 | 903.5 | 2440 | 12845 (11523–22350) |
| `burn-text-subtitles` | transcode | 2689 (2546–3278) | 1.3 | 117.0 | 868 | 3576 (3233–3855) |
| `burn-picture-subtitles` | transcode | 3033 (2835–9086) | 2.1 | 97.6 | 988 | 3434 (3399–4271) |
| `audio-transcode` | audio-transcode | 309 (161–924) | 42.8 | 1.0 | 25 | 511 (426–1643) |
| `live-channel-1080p` | live, copied | 23253 (20232–29486) | 1.0 | 2.4 | 40 | |
| `dvd-title` | disc | 30444 | 1.6 | 21.7 | 119 | 2712 |
| `bluray-title` | disc | 26681 | 2.6 | 45.3 | 372 | 2734 |

The disc rows are one run each: the later runs found the first one's stream still there, which the benchmark now stops. Most of a disc's first segment is opening it (`Start ms` was 28 and 22 seconds), not FFmpeg.

What the numbers show, and the changes made for them, are in [#81](https://github.com/wittywilla/WittyMediaServer/issues/81).

---

<sub>WittyMediaServer documentation</sub>
