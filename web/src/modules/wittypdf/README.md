# WittyPDF

**A PDF reader for papers, textbooks and scans, with the study tools and handwriting serious
readers need.** WittyPDF draws pages with [pdf.js](https://mozilla.github.io/pdf.js/), keeps the
text selectable, and lets you highlight, annotate and write on any page.

It runs inside [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) (your books
libraries) and the [WittyReader](https://github.com/wittywilla/WittyReader) app.

📖 **[Read the full guide](wiki/Home.md)** in the `wiki` folder.

> This repository is a read-only mirror of `web/src/modules/wittypdf` in
> [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer), where development happens.
> Please open issues and pull requests there.

## What it does

### Viewing

- **Pages in a continuous scroll**, one or two across.
- **Zoom:** fit width, fit page, or steps from 50 to 400 %, with Ctrl + scroll, **+** and **−**.
- **Page colours:** as printed, sepia, or **dark** (light text on dark pages, for night reading).
- **Selectable text** on every page with a text layer.

### Getting around

- **Contents:** the document's own outline, nested, jumping to the exact spot.
- **Page thumbnails**, drawn as you scroll the list.
- **Search across the whole document**, with results appearing while long documents are searched.
- **Go to page**, a page slider, previous and next buttons, and Home and End.
- **Exact resume:** the page and how far down it you were, synced across devices in
  WittyMediaServer.

### Study tools (from [WittyInk](https://github.com/wittywilla/WittyInk))

- **Highlights:**
  - drawn as boxes over the selected text, line by line, even across pages;
  - any colour, with **notes** and **colour meanings**;
  - click a highlight to edit it.
- **Selection toolbar:** highlight, note, define, copy, cite, search, read aloud.
- **Notes panel:** filters by kind, colour or text; jump to any entry; **export to Markdown**,
  grouped by page.
- **Citations with page numbers:** *"quote" (Author, Title (Year), p. 12)*.
- **Bookmarks**, **definitions** (when the app provides a dictionary) and **read aloud**,
  sentence by sentence, moving on through the pages.
- **Undo and redo.**

### Handwriting

- **Tools:** pen, pencil, highlighter and eraser, with **pen pressure** and pencil tilt.
- **Colours and sizes:** 36 ink and 18 highlighter colours, thickness and opacity.
- **Palm rejection:** fingers move the page while the pen writes. The pen also writes at any time.
- **Strokes stay exactly where you drew them** at any zoom.

### Light and safe

- **Light on low-end hardware:**
  - pdf.js loads only when a PDF is opened;
  - only the pages near the screen are drawn;
  - page images are size-capped and given back when they scroll away;
  - large files stream in parts when the server supports it.
- **Safe with any PDF:** PDF scripts never run, XFA forms are off, and fonts are never compiled
  with `eval`.
- **Wide support:** uses pdf.js's build for older browsers, so TV web views and older devices
  work.

## Install

```bash
npm install github:wittywilla/WittyPDF
```

TypeScript and React source, for **Vite** with `@vitejs/plugin-react` (the pdf.js worker is
included with Vite's `?url` import). Needs React 18 or newer. Load it lazily:

```tsx
import { lazy, Suspense } from 'react'
import { HostContext } from '@wittywilla/wittyink'
const WittyPDF = lazy(() => import('@wittywilla/wittypdf'))

<HostContext.Provider value={host}>
  <Suspense fallback={null}><WittyPDF /></Suspense>
</HostContext.Provider>
```

The host's `pdf` field is what to open: `{ url }`, or `{ data }` for a file already in memory.

## License

Apache-2.0, the same licence as [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).
Copyright 2026 WittyWilla and contributors.

You're welcome to use, change and share WittyPDF. If you redistribute it or build on it, the licence
asks you to keep the [NOTICE](NOTICE) file's attribution, *"WittyPDF, developed by WittyWilla"*, in
your own NOTICE file, documentation or About screen. The text-layer styles are adapted from pdf.js (Apache-2.0, Mozilla Foundation).
