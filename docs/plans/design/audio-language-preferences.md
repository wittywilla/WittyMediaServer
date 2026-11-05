# Design: preferred and required audio languages

> **Status on 2026-11-04**
>
> **Planned:** [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/31). Not in 0.7.0-alpha: the one server-wide setting is still what there is.

The owner, 2026-11-02: "allow you to set a preferred audio language (if available). For instance, if a tv show or movie has multiple sound tracks, allow the user to prefer a language or enforce a language requirement."

Not built. Not tested.

---

## What exists today

From the published code:

- **One server-wide preference:** `audioLanguage` (and `subtitleLanguage`, `menuLanguage`), set by an admin as a text box in **Settings → Playback**. Playback (`playback.ChooseTracks` in `internal/server/api_playback.go`), DVDs and Blu-rays (`dvdsession`, `bdsession`) and WittyTV channels use it.
- **A request can override it** (`audioLanguage` in the playback request body), and the player's **Audio & language** tiles switch tracks mid-play, keeping the place.
- **Discs remember** the last audio and subtitle language chosen, per disc, in that browser (`discMemory`).
- **Library transcoding** knows languages (keep, drop, default track) for converting files.

**What's missing:**

1. A preference **per person**: everyone in the house gets the admin's choice.
2. **More than one** language, in order ("Japanese, else English").
3. **"Original language"** as a choice, which matters most for anime, foreign films and shows.
4. A choice **per show or per library**, remembered: switching a show to Japanese once should hold for every episode, on every device.
5. A way to **require** a language: only see, play or schedule what has it.
6. A typed text box isn't a good control: it should be a list of languages by name, in the person's own language.

---

## The design

### 1. Preferences per profile, in order

**Profile settings → Language** (each person sets their own; the admin's server setting becomes the default for new profiles):

| Setting | Choices |
|---|---|
| **Preferred audio** | An ordered list of languages, picked by name (English, Japanese, Spanish…), with **Original language** as an entry that can go anywhere in the list. Example: *Original language, then English* |
| **If none is available** | **Play the default track** (default) · **Ask me** · **Don't play: tell me** (the "enforce" setting below) |
| **Avoid** | Commentary tracks, audio description and other special tracks are never chosen automatically, unless the person turns **Prefer audio description** on (accessibility) |
| **Preferred subtitles** | The same ordered list; plus **Only when the audio isn't in a language I prefer** (common for "Japanese audio, English subtitles"), **Always**, **Forced only**, **Never** |

**How a track is chosen** (one function, used by every player and every app, so they agree):

1. A choice remembered for this show or film (section 2), if its language is there.
2. Down the person's list, the first language present. **Original language** matches the track whose language equals the title's original language from its metadata (TMDB and NFO carry it), or the track the file marks as original.
3. Within a language, the track the file marks as default, then the most channels the device can play, never commentary or description unless asked.
4. If nothing matches: what **If none is available** says.

Tracks with no language tag (`und`) are a fact of real libraries. They count as "unknown", never as a match; a library setting can say "untagged audio in this library is Japanese" to fix a whole collection at once.

### 2. Per show, per film, per library: remembered on the server

- Changing the audio language while watching asks nothing, and is **remembered for that show** (all its episodes) or that film, for that profile, **on the server**, so the next episode on the TV picks the same track the phone used. Discs move from browser memory to the same place.
- A show or film page has **Audio language** in its menu: **Use my preferences** (default), or a fixed language for this show.
- A **library** can have a default too (an Anime library set to *Japanese, then English*), which profiles can override.

### 3. Enforcing a language

The "enforce a language requirement" part, in three places, each optional:

| Where | What it does |
|---|---|
| **When playing** | With **If none is available: Don't play**, a title without any required language shows "This has no Spanish audio" with **Play anyway** (one time) or **Choose a track** |
| **When browsing** | A library filter **Has audio in…** and a profile switch **Only show titles with my languages**, so a Spanish-speaking profile sees only what it can follow. Titles hidden this way are counted, never silently gone: "12 titles hidden: no Spanish audio" |
| **On TV channels** | A channel rule **Audio language required** (for example, a Spanish channel that only schedules titles with Spanish audio), and the shared stream then plays that language. Today a channel's stream takes the server's single setting; per channel is better, since a channel is one stream for everyone |

Admins can set a requirement **for a profile** (a child learning a language, or a household member who needs one), and profile limits already exist for ratings, so this sits beside them.

### 4. Knowing which languages a title has

- Scans already read each file's tracks; store each title's **audio languages** (and subtitle languages) as indexed data, not just inside the stream list, so filters and channel rules are one query.
- Show them on the item page ("Audio: English 5.1, Japanese 2.0, commentary") and as small badges on cards for libraries where it varies (anime, foreign films), optional.
- Discs: the disc modules already report each title's audio languages; use them the same way.

### 5. Apps

The choice and the remembered per-show choices live on the server, and the playback answer says which track was chosen and why, so WittyMediaClient's native players follow the same rules without their own logic. The **Audio & language** screen in every app offers **Remember for this show**.

### 6. Library transcoding

Transcoding's language options (keep, drop, default) read the same language lists, so "keep only English and Japanese" and "people here prefer Japanese" never disagree. A file that would lose the only track in a language someone requires is flagged before it's converted.

---

## Where it goes

| Release | Work |
|---|---|
| **0.7.0-alpha** | Per-profile ordered audio and subtitle preferences with **Original language** and a language picker; the shared chooser used by every player; remembered per show and film on the server (discs too); commentary and description avoided; item pages listing languages |
| **0.8.0-alpha** (next major) | Enforcing: **Don't play / Ask me**, **Has audio in…** filter, **Only show titles with my languages**, admin requirements per profile, channel rules **Audio language required** with the stream's language per channel, library defaults and "untagged audio here is…", transcoding agreement |

## Done when

Two profiles on one TV, one preferring Japanese with English subtitles and one preferring English dub, each get their own tracks on the same anime without touching anything; a language switched mid-episode holds for the rest of the show on every device; and a profile that requires Spanish never starts a title without Spanish audio without saying so first.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
