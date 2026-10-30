<h1 align="center">WittyComics</h1>

<p align="center">
  <b>A reader for comics, manga and webtoons.</b><br>
  One page, a two-page spread or one long strip, in either reading direction.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="React 18+" src="https://img.shields.io/badge/React-18%2B-61dafb.svg">
  <img alt="CBZ" src="https://img.shields.io/badge/reads-CBZ%20·%20manga%20·%20webtoons-8b6cff.svg">
</p>

<p align="center">
  <a href="#install">Install</a> ·
  <a href="#use">Use</a> ·
  <a href="wiki/Reading-comics.md">Reading comics</a> ·
  <a href="wiki/Home.md"><b>Full guide</b></a>
</p>

> Read-only mirror of `web/src/modules/wittycomics` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

Runs inside [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) and the
[WittyReader](https://github.com/wittywilla/WittyReader) app.

## Highlights

| | | |
|---|---|---|
| 📄 | **Page, Spread or Scroll** | one page; two side by side with the cover alone so pairs line up as printed; or one vertical strip for webtoons |
| ↔️ | **Either direction** | left to right, or right to left for manga; arrows, taps, swipes and the slider all follow it |
| 🔍 | **Fit height or width** | a whole page on screen, or tall pages read top to bottom |
| 👆 | **Easy to turn** | tap the outer thirds, swipe, or use arrows, Space, Page Up/Down, Home and End; tap the middle for the bars |
| ⚡ | **Smooth** | the next three pages are fetched ahead |
| 💾 | **Remembered per book** | layout, direction and fit, so a manga stays right to left; resumes on the same page |

The settings live in one **Reading** panel behind the gear button, so the bar stays clean on phones.

## Install

```bash
npm install github:wittywilla/WittyComics
```

TypeScript and React source, for a bundler such as Vite with `@vitejs/plugin-react`. Needs React 18
or newer. [WittyInk](https://github.com/wittywilla/WittyInk) is installed with it.

## Use

```tsx
import { HostContext } from '@wittywilla/wittyink'
import WittyComics from '@wittywilla/wittycomics'

<HostContext.Provider value={host}><WittyComics /></HostContext.Provider>
```

The host's `comic` field lists the pages: `{ pages: number, pageUrl: (n) => string }`. Set `reading`
to `rtl` (manga) or `webtoon` (vertical strip) to choose the first layout, for example from a comic's
`ComicInfo.xml`. See [Embedding](wiki/Embedding.md).

## The Witty family

| Project | Role |
|---|---|
| [WittyInk](https://github.com/wittywilla/WittyInk) | what the readers share |
| [WittyEbook](https://github.com/wittywilla/WittyEbook) · [WittyPDF](https://github.com/wittywilla/WittyPDF) · **WittyComics** | the book, PDF and comic readers |
| [WittyReader](https://github.com/wittywilla/WittyReader) · [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) | the apps they run in |

## Licence

**Apache-2.0**, the same licence as WittyMediaServer. © 2026 WittyWilla and contributors.
Use, change and share it; if you redistribute it or build on it, keep the [NOTICE](NOTICE)
attribution, *“WittyComics, developed by WittyWilla”*, in your NOTICE file, documentation or About screen.
