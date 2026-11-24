# Minimum hardware

What kind of computer the server needs, in plain words, from figures measured on a deliberately small machine. [What hardware it needs](HARDWARE.md) has the detail behind each figure, and [Performance](PERFORMANCE.md) says how they're measured.

- [In one line](#in-one-line)
- [The machine these figures come from](#the-machine-these-figures-come-from)
- [A Raspberry Pi or another small board](#a-raspberry-pi-or-another-small-board)
- [An old laptop or mini PC](#an-old-laptop-or-mini-pc)
- [A NAS](#a-nas)
- [What needs a graphics card](#what-needs-a-graphics-card)
- [What the server itself takes](#what-the-server-itself-takes)
- [Kept in check on every change](#kept-in-check-on-every-change)

## In one line

**Any 64-bit computer with two processor cores and 2 GB of memory runs the server for a household, as long as the devices play the files as they are.** Converting video while someone watches, separating a song's voice for karaoke, and 3D game consoles are where a graphics card (or a much bigger processor) comes in.

## The machine these figures come from

A Linux container held to **two processor cores and 3 GB of memory, with no graphics card**, on the owner's laptop (an Intel Core i7-12700H). Two cores of that processor are faster than two cores of a Raspberry Pi 4 or an old Celeron, roughly two to three times, so the times below are best cases for those machines; the memory figures are the same everywhere.

The library was 2,000 items made for the test: 1,500 films (20-second clips at 480p, 720p and 1080p) and 500 songs in 50 albums. A real film takes no longer to scan than a short one, since a scan reads a file's details and one picture, not the whole file.

Measured on 24 November 2026, on the 0.8.4-alpha work (#643).

## A Raspberry Pi or another small board

A Raspberry Pi 4 or 5 with 2 GB or more, or a similar ARM board, running Linux.

- **Works well:** browsing and searching the library, playing films and music the devices play as they are, music for a household, books, photos, live TV channels copied as they come, and recordings.
- **Works, slowly:** the first scan of a big library. At a few films a second it is minutes for a few thousand titles, once; rescans read only folders and take under a second. Scans give way while someone is watching.
- **Not for this machine:** converting video while someone watches (it took two cores of a much faster processor 2.2 seconds just to start a 1080p film), karaoke separation (many minutes a song), and 3D game consoles. Older consoles are drawn by the processor and need no graphics card; how many play at full speed on a Pi hasn't been measured.
- **Not measured on a real Pi yet:** these come from a two-core container on a laptop (above). A Pi's cores are slower, so expect its scans and page times to take two to three times as long; its memory use is the same.
- A **Raspberry Pi 3** or anything with 1 GB works for a small music or book library; the server keeps itself to a few hundred megabytes there ([small machines](HARDWARE.md#small-machines)).

## An old laptop or mini PC

A dual-core or quad-core Intel or AMD from the last ten years, 4 GB or more: an old laptop, a Celeron, Pentium or N100 mini PC.

- **Works well:** everything a Pi does, quicker; first scans of a few thousand titles in minutes.
- **Converting video:** one 1080p film converted on the processor takes a bit over one core for as long as it plays. A dual-core manages one viewer; a quad-core two. **A processor with Intel Quick Sync** (almost every Intel since 2012, including Celeron, Pentium and N100) or an AMD with a graphics part converts in hardware instead, several streams at once, and the server uses it by itself when it's there ([Transcoding](TRANSCODING.md#hardware-encoders)).
- **Karaoke separation** on the processor took 65 seconds for a four-minute song on the 14-core laptop, so expect a few minutes a song on a dual or quad core; the quick version is ready in seconds.
- **Games:** older consoles, Nintendo 64 and PSP are drawn by the processor; GameCube and Wii too on a Linux server, at a heavy cost; PlayStation 2, Dreamcast and 3DS need a graphics card ([Gaming](GAMING.md#without-a-graphics-card)).

## A NAS

A Synology, QNAP, TrueNAS, Unraid, UGREEN or Asustor box running the server in Docker ([Docker](DOCKER.md) has a guide for each).

- **The server counts what the container is given,** not the whole box: a container held to two cores and 2 GB is treated as a small machine and holds itself back to match. Since 0.8.4 this counts a processor limit too, not only memory.
- **Most NAS processors** are low-power Intel (Celeron, Pentium, Atom) or ARM. On an Intel one, pass the graphics device to the container (`/dev/dri`) and converting video uses Quick Sync; without it, a NAS converts at most one stream, and slowly.
- **An ARM NAS** is like a Raspberry Pi above: a fine library and direct-play server, not a converting one.
- Keep the server's **data folder on the NAS's fastest disk** (an SSD if there is one): the database and pictures are read on every page.

## What needs a graphics card

| What | Without a graphics card | With one |
|---|---|---|
| **Converting video while someone watches** | About one core per 1080p stream; 4K HDR is out of reach (15 cores' worth) | Several streams at once, 4K HDR included, at a few percent of the processor ([GPU first](TRANSCODING.md#hardware-encoders)) |
| **Starting a converted stream** | 2.2 seconds to the first picture for a 1080p film on two cores | Sooner: the card encodes the first seconds faster than the processor (not timed for this page) |
| **Karaoke separation** (taking a song's voice out properly) | About a minute for a four-minute song on a fast 14-core laptop; several minutes on a small machine | About 10 seconds a song (NVIDIA, or DirectML on any Windows card) |
| **WittyPlay 3D consoles** (Nintendo 64 at its best, GameCube, Wii, PlayStation 2, Dreamcast, PSP, 3DS) | GameCube and Wii only on Linux, drawn by the processor: 60 frames a second took 1 to 3.5 of a fast laptop's threads | All of them |
| **Everything else** (browsing, scanning, direct play, music, books, photos, TV channels copied as they come) | Fine | No difference |

The server finds a graphics card by itself, tries it with a tiny real job, and goes back to the processor on its own when the card can't do something ([Settings, Transcoding](TRANSCODING.md) shows what it found).

## What the server itself takes

On the two-core, 3 GB container, with no graphics card:

| | Measured |
|---|---|
| Memory, idle, no libraries | 36 MB |
| Memory, idle, after scanning the 2,000 items | 62 to 80 MB (52 to 71 MB once Go has handed back what it can) |
| Memory at its highest while scanning | 60 to 80 MB |
| Processor while idle | 0.25 to 0.4 % of one core |
| Start-up | 0.16 s on a new data folder, 0.07 s after |
| First scan: 1,500 films | About 3 minutes (8 films a second): most of it is FFmpeg reading each file and making its picture |
| First scan: 500 songs | 17 seconds (30 songs a second) |
| A rescan with nothing changed | 0.3 s a library |
| Home, as the apps ask for it | Half a millisecond for the page, 3 ms for its rows |
| A library's page | Under a millisecond for the page; 2 to 5 ms for its titles and filters |
| A search across the library | 4 ms |
| The web app's first download | 328 KB of scripts and 69 KB of stylesheets for Home (compressed, once; kept by the browser after) |

## Kept in check on every change

Every pull request is measured against budgets, so these figures can't quietly get worse:

- **The web app:** what a first visit to each kind of page downloads (`web/bundle-budget.json`, checked by `web/scripts/bundle-budget.mjs`).
- **The server:** idle memory after a scan, a scan's speed, what the main pages cost, and how long a converted stream takes to start, on two processors (`scripts/perf/budget.json`, checked by `scripts/perf -budget`).

A change that goes over fails the check and says which figure and by how much. [Performance](PERFORMANCE.md#budgets) says what to do then.

---

<sub>WittyMediaServer documentation</sub>
