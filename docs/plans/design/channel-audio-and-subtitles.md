# Design: choosing the language and subtitles while watching a TV channel

> **Status on 2026-11-04**
>
> **Planned** for after 0.7.0-alpha: [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/49).

The owner, 2026-11-02: "if there is multiple languages in a video being played on TV, allow the user to change it. As well as subtitles."

For WittyTV. Builds on [audio-language-preferences.md](audio-language-preferences.md) and [wittytv-live-sync.md](wittytv-live-sync.md). Not built. Not tested.

---

## Why channels are different

On demand, each viewer gets their own stream, so switching a track is easy: the player asks for another one. **A channel is one stream shared by everyone watching it** (`internal/hls/live.go`): one FFmpeg run, paced to real time, H.264 with **one** stereo AAC track, which today comes from the server's single `audioLanguage` setting. There are no subtitles at all. So one viewer can't change the language without changing it for everyone, and nothing else is offered.

**The rule:** the shared stream carries **every** language and subtitle the programme has, and **each viewer's player chooses** among them. Nobody's choice affects anybody else, and the stream is still made once.

---

## Audio

### Library channels (films and episodes)

- The live stream becomes HLS with **alternate audio renditions** (`EXT-X-MEDIA:TYPE=AUDIO`), one per language track in the programme, each stereo AAC (as now, so a stream stays playable across programmes). Video is still encoded once.
- **Cost:** one extra AAC encode per extra language, about 1% of a CPU core each (measured for games' audio at 0.6.0), and a little disk in the pause buffer. A setting caps it: **Languages carried on channels**: All · Up to 3 · Only the preferred one (low-end servers). Commentary and audio description are only carried if someone asked for them in their preferences.
- **Programmes change languages.** One film has English and French, the next only English. The playlist's renditions are named by **language**, not by track number, so "French" stays selected across programmes when it exists, and quietly falls back (to the viewer's next preference, then the default) when it doesn't, with a short notice: "This programme has no French audio. Playing English."
- **Discs on channels** (main features and scenes): the disc modules already report each title's audio streams by language; the same applies.

### Tuner channels (live broadcasts)

- A broadcast often carries several audio programmes: a second language (SAP), audio description, or surround and stereo versions. Each becomes a rendition, named from the stream's own language tags (ISO 639 descriptors in the transport stream), with the description track marked as such.
- Encrypted channels stay refused: nothing here changes that.

---

## Subtitles

### Library channels

- **Text subtitles** (SRT, ASS, embedded text tracks, sidecar files) become **WebVTT subtitle renditions** (`EXT-X-MEDIA:TYPE=SUBTITLES`), segmented with the video, one per language. Each viewer turns them on or off and picks the language. Cheap: text only.
- **Picture subtitles** (DVD and Blu-ray subpictures, PGS, VobSub) can't be WebVTT. Two options, both per viewer:
  1. **Sent as pictures** beside the stream, drawn by the player over the video, as the disc player already does (`sub-layer` in `DvdPlayer.tsx`) and the server already serves for discs. Preferred: exact, and nothing burned in for everyone.
  2. **Turned into text** by on-server OCR, as a later option, for apps that can only show text.
- **Forced subtitles** (signs, foreign-language lines) are shown for everyone whose subtitles are off, as on demand, because they're part of the programme.

### Tuner channels

- **Closed captions** carried inside the video (CEA-608 and CEA-708 in ATSC; DVB subtitles and teletext in Europe) are extracted to WebVTT renditions (FFmpeg can read 608/708 from the video's data; DVB bitmap subtitles go the picture route above). Today they're thrown away.

---

## The viewer's controls

- The channel player gets the same **Audio & language** and **Subtitles** tiles as the on-demand player (keys: **A** and **S**, on the on-screen remote, and on controllers), listing what this programme has.
- **The choice follows the viewer, not the channel:** it starts from the profile's preferences ([audio-language-preferences.md](audio-language-preferences.md)), and a change made while watching is remembered as "on channels, prefer French audio, English subtitles" for that profile, so the next programme and the next channel start the same way when they can.
- The banner shows the current language and a **CC** mark when subtitles are on.
- **Live sync is unaffected:** renditions share the same timeline and program date-time, so viewers on different languages stay in step with each other ([wittytv-live-sync.md](wittytv-live-sync.md)).
- Native apps: ExoPlayer and AVPlayer both list HLS renditions natively; the app shows them in its own track picker.

## Channel owners

The channel editor's **Options** gets **Audio**: the default language for this channel (the stream's default rendition), which languages to carry, and **Subtitles on by default** for this channel (for example, a foreign-film channel). Channel rules can also *require* a language ([audio-language-preferences.md §3](audio-language-preferences.md#3-enforcing-a-language)).

---

## Where it goes

| Release | Work |
|---|---|
| **0.8.0-alpha** (next major) | Multiple audio renditions on library channels, named by language and kept across programmes; text subtitles as WebVTT renditions; the viewer's tiles, keys and remembered choices; the cap on languages carried; channel audio defaults |
| **0.9.x-alpha** | Picture subtitles drawn per viewer; tuner channels' extra audio programmes and closed captions; OCR as an option |

It comes after [live sync](wittytv-live-sync.md) (0.7.0), which reworks the same live stream; both changes to `live.go` are easier one at a time.

## Done when

Two people watching the same channel, one with French audio and English subtitles and one with English audio and none, each get their own choice, stay in sync, and keep their choice across programmes, with neither change touching the other's picture or sound; and a broadcast's closed captions can be turned on.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
