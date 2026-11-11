# What hardware it needs

What the server takes from the computer it runs on, from measurements rather than guesses, so you can tell whether an old computer, a small board or a NAS is enough. Each figure says what it was measured on. What hasn't been measured yet says so.

- [The short answer](#the-short-answer)
- [The server on its own](#the-server-on-its-own)
- [Small machines](#small-machines)
- [Playing](#playing)
- [DVDs and Blu-rays](#dvds-and-blu-rays)
- [WittyTV channels](#wittytv-channels)
- [Games](#games)
- [A NAS and Docker](#a-nas-and-docker)
- [Disk space](#disk-space)
- [What hasn't been measured](#what-hasnt-been-measured)
- [Where the figures come from](#where-the-figures-come-from)

## The short answer

What costs the most is **converting video while someone watches** (transcoding). Playing a file the device can play as it is costs very little. So:

| What you want | Memory | Processor |
|---|---|---|
| A library, with people playing files their devices can play | About 50 MB for the server (128 MB to be comfortable), plus about 40 MB for each viewer | A small share of one core: each viewer is about a twentieth of one |
| One film converted to 1080p while you watch | About 1 GB more | A bit over one core, fully busy, for each viewer |
| One film converted to 720p, for a slow connection | About 450 MB more | Three quarters of a core for each viewer |
| 4K HDR converted to an ordinary picture | Over 2 GB more | Far more than a home computer has to spare (15 cores' worth). Use hardware encoding, or keep a 1080p copy |

**To need less:** play files the devices can play as they are ([Supported formats](SUPPORTED-FORMATS.md) and [Playing](PLAYING.md) say which), let the server convert a library ahead of time while nobody is watching ([Library transcoding](LIBRARY-TRANSCODING.md)), or turn on a hardware encoder ([Transcoding](TRANSCODING.md#hardware-encoders)).

## The server on its own

Measured on 4 November 2026, a server with no libraries and then with the demo media's libraries scanned (760 files on Linux and 1,360 on Windows: films, TV, music, audiobooks, podcasts and short videos), doing nothing:

| | Windows 11 | Linux (Ubuntu, in WSL) |
|---|---|---|
| Memory the system counts | 33 MB with no libraries, 46 MB after the scans | 42 MB both ways |
| Processor while idle | 0.01 to 0.05 % of one core | 0.01 to 0.02 % of one core |
| Start-up, an existing data folder | Under a second when the machine is quiet; up to several seconds while it's busy | About half a second |

**Scanning** a library adds little to the server itself: its memory peaked at 46 to 49 MB while scanning. Most of the work of a first scan is FFmpeg reading each file's details, a few tenths of a second a file. A scan with nothing changed reads only the folders. The server rescans every six hours.

## Small machines

The server looks at the machine when it starts, and holds itself back on a small one so that playing stays smooth. Nothing needs setting.

| | What counts | Examples |
|---|---|---|
| **Tiny** | Up to 1.5 GB of memory, or one or two processors with up to 2 GB | Raspberry Pi 3, Pi Zero 2, an old dual-core Atom |
| **Small** | Up to 4 GB of memory, or up to four processors | A Celeron or Atom mini PC, a Raspberry Pi 4 or 5, an N100 |
| **Roomy** | Anything larger | Nothing is held back |

In a container the memory limit you gave it counts: a NAS with 8 GB that gives the server 1 GB is a tiny machine.

On a tiny or small machine:

- **Memory.** The server keeps its own memory within about a quarter of what the machine has (128 to 384 MB on a tiny one, 256 MB to 1 GB on a small one), and tidies up more often, so there's room left for FFmpeg and for the system to keep the film being played in memory. If you set `GOMEMLIMIT` or `GOGC` yourself, yours are used.
- **Scanning gives way to watching.** While a video is being transcoded or a channel is running, a scan reads the next file only every 20 seconds on a tiny machine and every 5 on a small one, and carries on at full speed when the stream ends. A scan started during a long film still finishes. The files a scan reads are read at a lower priority at all times.
- **Less at once.** A tiny machine reads the details of two files at a time and makes one thumbnail at a time; a small one up to four and two.

A film that plays without transcoding is sent by the system itself, straight from the disk to the network, on every size of machine, with or without a speed limit for [remote access](REMOTE.md). The server doesn't read the file, which is why a Raspberry Pi can send several at once. That holds for plain HTTP, which is what a home network and a reverse proxy use; where the server does the HTTPS itself it has to read the file to encrypt it.

The log says what it decided when the server starts: `this is a small machine, so the server holds itself back`, with the class and the limits. To try another class, set `WMS_MACHINE` to `tiny`, `small` or `roomy` ([configuration](CONFIG.md)).

## Playing

These are FFmpeg's figures, for one viewer, in addition to the server's own. **Cores** is how much of the processor stays busy: 1 means one core fully busy while the film plays.

| What plays | Cores | Memory | |
|---|---|---|---|
| A 1080p film the device can play, sound converted | 0.06 | 36 MB | Direct play, as most browsers and apps get it |
| Music or an audiobook converted for a player that can't play it | 0.02 | 25 MB | |
| A 1080p film converted to H.264 at 1080p | 1.1 | 880 MB | |
| The same at 720p and 4 Mb/s, for away from home | 0.8 | 440 MB | |
| A 1080p HEVC episode converted to H.264 | 1.3 | 900 MB | |
| Subtitles drawn into the picture | 1.6 to 2.0 | 870 to 990 MB | Text and picture subtitles, while converting |
| A 4K HDR film mapped to an ordinary 1080p picture | 15 | 2.4 GB | Not practical without hardware encoding |

A machine converting for two viewers at once needs twice that. The server converts at most four at once unless told otherwise (`maxTranscodes` in [Config](CONFIG.md)): lower it on a small machine. FFmpeg uses more memory on a machine with more cores, as it starts a thread for each; these figures are from a 20-thread laptop, so a 4-core machine should need less, but that hasn't been measured.

## DVDs and Blu-rays

| | Cores | Memory |
|---|---|---|
| A DVD's main title, with its menus | 0.4 | 120 MB |
| A Blu-ray's main title | 0.8 | 370 MB |

Most of the wait before a disc starts is opening it (20 to 30 seconds on the busy test machine), not FFmpeg. Menus with Java (BD-J) start Java too, which hasn't been measured.

## WittyTV channels

A channel playing a 1080p file live, copied as it is, took 0.04 of a core and 40 MB. A channel that has to convert its files costs what converting costs, above. Tuners and recordings haven't been measured.

## Games

**Not measured yet.** Games run on the server and stream to the screen you play on ([Gaming](GAMING.md)), so the server needs the processor for the emulator and for encoding the picture. Older consoles need far less than newer ones.

## A NAS and Docker

**Not measured yet.** The server's own needs are the same in a container. A NAS with an ARM or a low-power Intel processor will usually manage direct play for a household; converting video is where it struggles, unless its processor has a hardware encoder the server can use. [Docker](DOCKER.md) says how to give a container the graphics, and has a guide for each NAS.

## Disk space

The data folder grows with the library: pictures for each title, thumbnails (at most 4 GB, trimmed by themselves) and, while someone watches a converted stream, its segments in the transcode folder. How big the program itself is depends on the system and the FFmpeg it comes with. [Backup](BACKUP.md) and [Config](CONFIG.md) say where each folder is.

## What hasn't been measured

- **ARM boards and small machines** (a Raspberry Pi, an ARM NAS, a 4-core mini PC). All the figures above are from one laptop.
- **Hardware encoding** (NVENC, Quick Sync, VA-API, VideoToolbox): what it saves in processor and memory.
- **Games**, **tuners and recordings**, **BD-J menus**, and **macOS** and **FreeBSD**.
- **Many viewers at once**: the figures are for one, and two viewers are taken to cost twice as much.

If you run the server on one of these, what it shows in **Statistics** while something plays ([Server statistics](STATS.md), from the WittyStats module) is a welcome addition: say what you measured on in a discussion on the project's GitHub page.

## Where the figures come from

- **The server's own figures** come from its benchmark (#89 on GitHub, [wittywilla/WittyMediaServer#89](https://github.com/wittywilla/WittyMediaServer/issues/89)), run on 4 November 2026 on the 0.7.0-alpha work, on the demo media.
- **FFmpeg's figures** come from FFmpeg's benchmark, [Measuring FFmpeg](FFMPEG-BENCHMARK.md#figures), measured on 0.6.2-alpha.
- **The machine** was a laptop with an Intel Core i7-12700H (14 cores, 20 threads) and 64 GB of memory, on Windows 11, with Linux in WSL on the same machine, and FFmpeg N-128142 with libx264. It was busy with other work the whole time, so times were slower than on a quiet machine. Memory and the processor time used change much less with that.

These figures will be measured again for each release, and this page updated.

---

<sub>WittyMediaServer documentation</sub>
