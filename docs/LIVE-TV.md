# Live TV: tuners, IPTV and the guide

Live TV comes from the **WittyTuner** module: network tuner boxes, IPTV playlists (M3U and M3U8), single streams, SAT>IP, TV cards and capture devices. This page is how it works in practice: getting channels in, keeping a big playlist tidy, getting the guide right, and what happens when a stream misbehaves. Channels made from your own library (WittyTV), the guide's look and the player are in [Channels and live TV](CHANNELS.md).

No channel lists or stream addresses come with the server. **Encrypted and DRM-protected channels are never decrypted**: they're shown as encrypted, with the reason, and refused everywhere (see [Encrypted channels](CHANNELS.md#encrypted-channels)).

- [Try it with no tuner: the demo tuner](#try-it-with-no-tuner-the-demo-tuner)
- [Adding a playlist](#adding-a-playlist)
- [Big playlists](#big-playlists)
- [When a playlist changes](#when-a-playlist-changes)
- [What a playlist can say about a channel](#what-a-playlist-can-say-about-a-channel)
- [Guide data](#guide-data)
- [Matching channels to the guide](#matching-channels-to-the-guide)
- [Network tuner boxes](#network-tuner-boxes)
- [How a channel plays](#how-a-channel-plays)
- [Recording](#recording)
- [When something goes wrong](#when-something-goes-wrong)
- [What it costs](#what-it-costs)
- [For developers and testers](#for-developers-and-testers)

## Try it with no tuner: the demo tuner

**Try a demo tuner** in first-time setup's Live TV step, or **Add a demo tuner** in **Settings → Live TV & tuners**, adds a pretend tuner that runs on the server itself. It needs no hardware and no internet:

- **Channels made as you watch:** Demo News (colour bars), Demo Pattern, Demo Life and Demo Colours, each with its name, a clock and a steady tone, made by the server's FFmpeg. Give it a **Folder of clips** and up to six videos from that folder become channels of their own, played in a loop.
- **Demo Protected** is marked as copy protected, the way a real tuner marks one, so you can see how an encrypted channel looks. Nothing is protected or decrypted: it simply never plays.
- **Its own guide**, from yesterday to three days ahead, so the guide, WittyTV, **Record** and the player's **Live** button all work.
- **It behaves like a real tuner box.** It answers the same discovery and lineup questions a network tuner does, on this computer only, so everything after adding it is the real network tuner's code: a channel scan, tuning, "all tuners in use".
- **For testing error handling,** its settings (open the tuner under **Settings → Live TV & tuners**) choose how many **Tuners** it has (1 to 8), **Slow tuning** (wait up to 30 seconds before a channel starts), and **Lose the signal every** so many minutes.

Everything about it says **Demo**. Remove it like any other tuner. It answers on port 47601 of this computer only (`127.0.0.1`); change **Port** if another program uses that.

## Adding a playlist

**Settings → Live TV & tuners → Add live TV → A playlist from a TV provider.** Paste its address (`http://` or `https://`) or the path of a file on the server, press **Check it** to see what it holds without saving anything, then **Add and read its channels**. A gzipped playlist is fine.

Where a playlist comes from decides what it may reach: one at home (a file, a private address, a `.local` name) may name devices on your network; one from the internet stays on the internet. See [Live TV playlists](CHANNELS.md#live-tv-playlists).

## Big playlists

A provider's playlist can hold thousands of channels. A tuner's **Channels** page narrows them:

- **Find a channel** searches names, numbers and groups.
- **Group**, **Country** and **Language** lists appear when the playlist uses `group-title`, `tvg-country` and `tvg-language`.
- **Which channels**: shown, hidden, **with no guide**, or **missing from the playlist**.
- With a filter on, **Show these** and **Hide these** change every channel it picks at once: hide everything, then show the one group you watch. Without a filter they're **Show all** and **Hide all**.
- Tick channels for the bar: **Renumber** from a number upward, **Make into one**, **Check health**.

Favourites, sets and the guide's own filters are the viewer's, in the guide.

A 5,000-channel playlist is read and saved in under a second on a desktop computer; the list shows 500 at a time.

## When a playlist changes

WittyTuner reads a playlist again when you press **Read the list**, and keeps what you chose for each channel: its number, name, logo, guide match, whether it's shown, and so the favourites, recordings and sets that name it.

- **A changed address keeps the channel.** Many providers put a login token in each address that changes from day to day. A channel without a `tvg-id` is known by its address, so a new token looks like a new channel. When exactly one channel of the same name and group disappeared and one appeared, it's the same channel: it keeps its place and everything chosen for it.
- **A channel that disappears is kept for a week**, hidden from the guide and marked **Missing from the playlist** on the Channels page, so a playlist that drops it for a day loses nothing. Back within the week, it's as it was. After a week it goes.

## What a playlist can say about a channel

| In the playlist | What it does |
|---|---|
| `tvg-id` | The channel's id in the guide |
| `tvg-name` | Its name, when the line has none after the comma |
| `tvg-logo` | Its logo (web addresses only), fetched and kept by the server |
| `tvg-chno` | Its number |
| `group-title`, `#EXTGRP` | Its group |
| `tvg-language`, `tvg-country` | Shown under its name, and filters on the Channels page |
| `tvg-shift` | Hours its guide is moved by (`1` for a "+1" channel, `-2.5` too); on `#EXTM3U` it applies to every channel |
| `catchup`, `catchup-days`, `catchup-source`, `timeshift` | How far back its source keeps what it showed: see catch-up in [Channels and live TV](CHANNELS.md#watching-a-channel) |
| `url-tvg`, `x-tvg-url` on `#EXTM3U` | The guide's address, added under **Guide data** for you |
| `#EXTVLCOPT:http-user-agent=…`, `#EXTVLCOPT:http-referrer=…` | The user agent and referrer the channel's stream wants |
| `address\|User-Agent=…&Referer=…` | The same, after the address |
| `#EXTHTTP:{"User-Agent": …}`, `#KODIPROP:…stream_headers=…` | The same |
| The same channel listed twice (the same `tvg-id` and name, another address) | One channel, with the second address as its fallback |
| `#KODIPROP` licence lines, DRM, Widevine, PlayReady, FairPlay or ClearKey | The channel is marked encrypted and never opened |

A user agent set on the tuner itself is for fetching the playlist; one the playlist names for a channel wins for that channel.

## Guide data

Guides come from:

- **XMLTV**, by address or file, under **Guide data**, plain or gzipped. A playlist's own `url-tvg` and a provider account's guide are added for you. Each is read when added and then twice a day. Its row says how many listings it gave, for how many channels, and how far ahead they go; a warning appears when one hasn't been read for two days, or its listings run out within 12 hours.
- **Broadcasts** themselves (DVB EIT, ATSC PSIP), while a channel is on.
- **The demo tuner's** own guide.

No guide service account or key is built in. Free and open sources work as XMLTV addresses: point **Guide data** at an XMLTV file you're allowed to use, such as one you make yourself with an open-source guide grabber, and respect that source's terms and how often it allows downloads. Guide data is cached by the server and only the coming two weeks are kept.

**Logos** come from the playlist (`tvg-logo`), the provider, or the logo you choose for a channel. The server fetches each once and keeps it, so viewers' devices never contact the logo's site.

## Matching channels to the guide

Each channel's listings come from one guide entry. WittyTuner matches them by itself, forgivingly:

1. **By guide id**, also when the guide writes it in another case or with a suffix (`BBCOne.uk@HD` finds `bbcone.uk`).
2. **By name**, without a playlist's country tags and quality words: `UK: BBC One HD`, `[UK] BBC One FHD` and `BBC One` are the same name. Names in any alphabet work.
3. **By call sign**, and by **number** among a guide channel's names.
4. **By a similar name**, scored by how alike the words are. Numbers must agree, so Channel 4 is never taken for Channel 5.

Each match has a **confidence score**. A match is made by itself when it's at least 80% sure and nothing else comes close. The rest wait for you on **Settings → Live TV & tuners → Match channels to the guide**:

- Channels with **no guide** and **unsure** matches, each with its likeliest guide entries and their scores: **one press** takes one.
- **Search the guide** beside each, for when the right one isn't suggested. **No guide** clears a match.
- **Match what is sure** runs the automatic matching again, and **Show every channel** lists the matched ones too.

**A match you choose is remembered** by the channel's name, so if the playlist is removed and added again, or a channel comes back, it gets the same guide. A guide id you type on a tuner's Channels page counts as chosen too. Matching runs after every guide read and every playlist read.

On a television, **Match channels to the guide** says to use a desktop app or web browser; what's chosen there applies on the TV. Watching the guide and changing channels stay fully usable on a TV.

## Network tuner boxes

**Search for tuners** finds boxes on your network by their discovery protocol (and SSDP for those that emulate one), including boxes that answer on another port. A box that doesn't answer a search is added by its address: **Add live TV → A tuner box on your network**, then type `192.168.1.50` (or `192.168.1.50:8080`). **Check it** says the model and how many tuners it has, or which way it failed: nothing answered, or something answered that isn't a tuner. **Scan for channels** shows its progress. Channels the box marks as protected are shown as encrypted.

While watching, a box that's busy says **all of this device's tuners are in use**, one that can't tune says the signal may be too weak, and one that no longer has a channel says to scan again.

## How a channel plays

- **One upstream stream per channel.** However many people watch a channel, the server opens it once, converts it once and serves every viewer from that, so a provider's "streams at once" limit counts channels, not viewers. A tuner's **Streams at once** setting caps how many channels it opens.
- **HLS and MPEG-TS** both work. HLS is read through a small gate on this computer, which checks every playlist for encryption keys before FFmpeg sees it. Redirects are followed, but never back to your home network for a playlist from the internet.
- **Fallbacks.** When a channel won't open, its other addresses from the playlist are tried, then its backups (the same channel from another tuner or playlist, see Duplicates in [Channels and live TV](CHANNELS.md#tuners-and-iptv)).
- **When a stream drops while people watch**, the server tries again at once, then after 1, 2, 4 and 8 seconds, before telling viewers it stopped. Encrypted is final.
- **Remux or convert.** With **Pass H.264 through** on (live TV's options), H.264 that needs no work passes through unconverted; otherwise it's converted to the height set there. Interlaced broadcasts are deinterlaced.
- **Sound and subtitles.** The first sound track plays. Sound that this server's FFmpeg can't decode (AC-4 on some ATSC 3.0 stations) plays without sound, with a notice. Choosing another sound or subtitle track on live TV isn't offered yet.

## Recording

Any tuner or IPTV channel that isn't encrypted or marked **can't record** can be recorded, once or every airing, from the guide or a programme's details (see [Recordings](CHANNELS.md#recordings)).

- **A stream that drops mid-recording is opened again** and added to the same file, trying every few seconds (up to every 30 seconds) until the programme's end. The finished recording says how many times the stream dropped, so you know a moment may be missing.
- HLS channels are recorded with their picture and sound; their subtitles and data tracks are left out, as they can't go in the recording's file.
- When no tuner is free, a live viewer on that tuner is stopped so the recording can start.

## When something goes wrong

Errors are said in words, with what to try:

| You see | What it means |
|---|---|
| **The stream refused this server (403)** | The link may have expired (read the playlist again), or the provider wants a particular user agent or referrer (set it on the tuner, or in the playlist) |
| **The stream isn't there any more (404)** | The playlist is out of date, or the channel is off the air |
| **The channel's playlist names video that isn't there** | Its HLS variants are all missing: off the air, or an old address |
| **The provider's server had a problem** | Their side; try again shortly |
| **The stream could not be reached** | No connection to that address |
| **All of this device's tuners are in use** | Every tuner, or the provider's stream limit, is taken |
| **Encrypted: can't be played here** | The broadcaster protects it. It's never decrypted, and nothing is wrong with your setup |
| **The guide's address could not be reached** | On a guide source: check its address |

The full detail of every failure is in the server's log, never shown on a page, as an address can hold a login.

## What it costs

- Reading a 5,000-channel playlist: under a second. Matching 3,000 channels against a 40,000-channel guide: a fraction of a second. A 13 MB XMLTV guide: about 5 seconds, a little memory, streamed rather than loaded whole.
- Watching: one FFmpeg per channel being watched, shared by its viewers. With **Pass H.264 through**, H.264 channels cost almost nothing.
- The demo tuner: one small FFmpeg (640×360) per demo channel being watched or recorded, and nothing when none is.
- Recording: a raw stream is written straight to disk; others are copied by FFmpeg without converting.

## For developers and testers

- The demo tuner is WittyTuner's `demo` driver (`demo.go`): an HTTP lineup (`/discover.json`, `/lineup.json`, `/lineup_status.json`, `/lineup.post`, `/auto/v<number>`), a discovery answer on `127.0.0.1:65001` naming its port, and `/guide.xml`. Its tests run against a real FFmpeg: `WMS_FFMPEG=<ffmpeg> go test . -run Demo`.
- Matching is WittyTuner's `match` package. The server lists the guide's channels to it (host API `GET /tv/guide/ids`: id, name, other names, icon, source, and how far ahead its listings go).
- A `tvg-shift` channel's programmes are kept under `<guide id>@<minutes>`, so every part of the server that reads a channel's guide sees them moved.
- Testing with a big playlist: WittyTuner's tests make playlists of thousands of channels; for a test server, any generated M3U and XMLTV of made-up channels whose addresses lead nowhere, or to the demo tuner, will do. Don't test with pirated IPTV lists.
