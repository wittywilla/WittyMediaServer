# WittyComics

**A reader for comics, manga and webtoons.** WittyComics shows one page at a time, a two-page
spread, or one long vertical strip, in either reading direction.

It runs inside [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) (your books
libraries) and the [WittyReader](https://github.com/wittywilla/WittyReader) app.

📖 **[Read the full guide](wiki/Home.md)** in the `wiki` folder.

> This repository is a read-only mirror of `web/src/modules/wittycomics` in
> [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer), where development happens.
> Please open issues and pull requests there.

## What it does

- **Three layouts:**
  - **Page:** one page at a time.
  - **Spread:** two pages side by side, with the cover on its own so the pairs line up as printed.
  - **Scroll:** every page in one long vertical strip, for webtoons.
- **Either reading direction:** left to right, or right to left for manga. The arrow keys, taps
  and swipes all follow the direction you choose.
- **Fit to height or width**, for a whole page on screen or tall pages read top to bottom.
- **Controls:**
  - tap the right or left third to turn, and the middle for the bars;
  - swipe on touch screens;
  - arrow keys, Space and Page Up/Down, Home and End;
  - a page slider that runs in the reading direction.
- **Smooth reading:** the next three pages are fetched ahead.
- **Remembered per book:** layout, direction and fit, so a manga stays right to left. The first
  time a book opens, the app can suggest a direction, for example from a comic's `ComicInfo.xml`
  or a "Manga" folder.
- **Resumes** at the page you were on, synced across devices in WittyMediaServer.

## Install

```bash
npm install github:wittywilla/WittyComics
```

TypeScript and React source, for a bundler such as Vite with `@vitejs/plugin-react`. Needs React
18 or newer. WittyInk (the reader host) is installed with it.

## Use

```tsx
import { HostContext } from '@wittywilla/wittyink'
import WittyComics from '@wittywilla/wittycomics'

<HostContext.Provider value={host}><WittyComics /></HostContext.Provider>
```

The host's `comic` field lists the pages: `{ pages: number, pageUrl: (n) => string }`. Set
`reading` to `rtl` (manga) or `webtoon` (vertical strip) to choose the first layout. See
[Embedding](wiki/Embedding.md).

## License

Apache-2.0, the same licence as [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).
Copyright 2026 WittyWilla and contributors.

You're welcome to use, change and share WittyComics. If you redistribute it or build on it, the licence
asks you to keep the [NOTICE](NOTICE) file's attribution, *"WittyComics, developed by WittyWilla"*, in
your own NOTICE file, documentation or About screen.
