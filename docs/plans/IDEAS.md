# Further upgrades and overhauls

> **Status on 2026-11-04**
>
> **Done:** the CI check that scripts keep their executable bit (`.github/workflows/ci.yml`); backups on a schedule (**Settings → Backup**, Schedule tab); the docs contract tests now run on every pull request without a module catalogue ([#96](https://github.com/wittywilla/WittyMediaServer/issues/96)).
>
> **In progress:** server tests well under 10 minutes and flaky tests ([#97](https://github.com/wittywilla/WittyMediaServer/issues/97), [#74](https://github.com/wittywilla/WittyMediaServer/issues/74)); the minimum hardware guide ([#113](https://github.com/wittywilla/WittyMediaServer/issues/113)); performance budgets ([#89](https://github.com/wittywilla/WittyMediaServer/issues/89), [#90](https://github.com/wittywilla/WittyMediaServer/issues/90)).
>
> **Planned:** [notifications](https://github.com/wittywilla/WittyMediaServer/discussions/41), [search](https://github.com/wittywilla/WittyMediaServer/discussions/42), [offline downloads](https://github.com/wittywilla/WittyMediaServer/discussions/43), [parental controls](https://github.com/wittywilla/WittyMediaServer/discussions/44), [a library health page](https://github.com/wittywilla/WittyMediaServer/discussions/46), [other languages](https://github.com/wittywilla/WittyMediaServer/discussions/40). The rest have no idea posted yet.

The owner asked to "add further next major upgrades or overhauls as well as minor incremental updates we can do with already existing information". These come from what's already known about the project: gaps the docs and notes admit, things half there, and what an all-in-one media server is expected to have. None is scheduled. Each, if the owner wants it, becomes an issue in `Backlog` and later a milestone ([GITHUB-PRACTICES.md](GITHUB-PRACTICES.md)).

Sizes: **S** a patch, **M** a minor release item, **L** a headline feature, **XL** an overhaul.

---

## Small, incremental (could go in any 0.x.1)

| Idea | Size | Why |
|---|---|---|
| A CI check that docs contract tests didn't skip (count SKIP lines; fail if `WMS_TEST_CATALOG` isn't set in CI) | S | They passed with `openapi.json` out of date once |
| A CI check that commits keep the executable bit | S | Lost from Windows commits in 0.6.0 |
| `go test ./internal/server` under 5 minutes (parallel format rows, shorter rate-limit waits) | S | It's near Go's 10-minute default |
| A dry run of every release workflow before publish day | S | The player's build only failed on GitHub's runners |
| Light theme contrast sweep each release, automated | S | Low-contrast labels came back before |
| Error pages with a "copy details" button that includes a sanitised log excerpt | S | Easier reports |
| Keyboard shortcut sheet (press ?) on every page | S | Discoverability |
| "What's new" after an update, from the changelog, once per profile | S | People see what changed |
| Empty states that say what to do next on every library page | S | Beginners |

## Medium

| Idea | Size | Why |
|---|---|---|
| **The minimum hardware document**, from measured figures | M | On the roadmap, not written |
| **Notifications inside the app:** new episodes, recordings done, scans failed, module updates, with a bell in the top bar beside activity; per profile; nothing sent outside unless an admin sets up a channel they own (email through their own SMTP, for example) | M | Expected of a media server |
| **Search overhaul:** one search across every library kind, with typo tolerance, filters as you type, and recent searches; voice search where the platform provides it | M | Today's search is per kind (to verify) |
| **Offline downloads in the native apps**, with transcoding to the device's size and limits set by admins | M–L | Expected on phones and tablets |
| **Parental controls overhaul:** ratings per country, time limits per profile, bedtime, a PIN for leaving a kids' profile | M | Families |
| **Backups on a schedule**, to a folder or a second drive, keeping the last N | M | 0.6.0 added ZIP backup and restore by hand |
| **Library health page:** files missing, unmatched, duplicates, odd names, broken NFOs, with fixes in one place (ties the 0.6.0 repair tools together) | M | Big libraries |
| **Subtitles:** search the person's chosen open subtitle source (opt-in), sync shifting by hand, and styling per profile | M | Common need |
| **Audio description and accessibility tracks** picked by profile preference | M | Accessibility |

## Large

| Idea | Size | Why |
|---|---|---|
| **Localisation:** the whole UI, help and setup in other languages, with right-to-left support. Strings moved into message files, a translation workflow on GitHub, and locale-aware dates and numbers | L | The product is in English only (to verify); it's the largest single reach increase |
| **Multi-server for one person:** WittyMediaClient showing two servers' libraries side by side (family member's server), each with its own sign-in | L | Common in households |
| **Server-to-server sharing:** share one library with a friend's server, read-only, through WittyRelay (the free hosted relay, once it exists) or remote access, with limits | L | Expected of media servers |
| **Live TV time-shift for everyone:** start any library channel's current programme from the beginning, for one viewer | L | Natural after WittyTV sync |
| **A plugin SDK guide and examples** for new custom modules (now "in development" only): when the owner opens custom modules, a template repository per delivery kind | L | Modules are their own projects |
| **HDR everywhere:** HDR10 and HLG passthrough to HDR screens in every app, Dolby Vision profiles where licences and decoders allow, tone mapping in hardware | L | 4K libraries |

## Overhauls

| Idea | Size | Why |
|---|---|---|
| **Setup wizard v2:** after the 0.6.0 rewrite, a second pass with real beginners watched (the owner's friends or family), changing what confuses them | XL | "Noob friendly" was the owner's bar |
| **A design system package** shared by the web app, WittyMediaClient's native apps and every module's web part (tokens, components, focus rules), versioned like a module | XL | Cohesion across everything, as the 0.6.0 standards doc started |
| **Performance overhaul, finished:** the optimise pass the owner stopped early in 0.6.0, run to the end on a low-end box, with budgets per route and per page enforced in CI | XL | Performance first |
| **Security programme:** fuzzing in CI nightly for every parser, a second reader for every sandbox change, a yearly outside review once the project can afford one | XL | Security first |

---

<sub>Future plans, written 2026-11-02. Ideas only.</sub>
