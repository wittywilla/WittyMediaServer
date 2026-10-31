<h1 align="center">WittyPDF</h1>

<p align="center">
  <b>A PDF reader for papers, textbooks and scans.</b><br>
  Selectable text, zoom and night pages, with the same highlights, notes and pen as the other Witty readers.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="React 18+" src="https://img.shields.io/badge/React-18%2B-61dafb.svg">
  <img alt="pdf.js" src="https://img.shields.io/badge/renders%20with-pdf.js-e44d26.svg">
</p>

<p align="center">
  <a href="#install">Install</a> ·
  <a href="#use">Use</a> ·
  <a href="wiki/Performance-and-security.md">Performance & security</a> ·
  <a href="wiki/Home.md"><b>Full guide</b></a>
</p>

> Read-only mirror of `web/src/modules/wittypdf` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

Runs inside [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) and the
[WittyReader](https://github.com/wittywilla/WittyReader) app. Pages are drawn with
[pdf.js](https://mozilla.github.io/pdf.js/).

## Highlights

| | Viewing and getting around | |
|---|---|---|
| 📄 | **Continuous pages** | one or two across, fit width, fit page, or 50–400 % (Ctrl + scroll, + and −) |
| 🌙 | **Dark pages** | light text on dark pages for night reading, or sepia |
| 🗂️ | **Outline and thumbnails** | the document's own contents, nested, and page thumbnails |
| 🔎 | **Search** | the whole document, with results arriving while long files are searched |
| 🎯 | **Exact resume** | the page and how far down it, synced across devices in WittyMediaServer |

| | Study tools (from [WittyInk](https://github.com/wittywilla/WittyInk)) | |
|---|---|---|
| 🖍️ | **Highlights and notes** | boxes over the text, line by line, even across pages; colour meanings; Markdown export by page |
| 📚 | **Citations with page numbers** | *“quote” (Author, Title (Year), p. 12)* |
| ✍️ | **Pen with pressure** | pen, pencil, highlighter, eraser; strokes stay exactly where drawn at any zoom |
| 🔊 | **Read aloud** | moving on through the pages |

| | Light and safe | |
|---|---|---|
| 🪶 | **Low-end friendly** | pdf.js loads only when needed; only nearby pages are drawn; page images are released as they scroll away; large files stream in parts |
| 🔒 | **Safe with any PDF** | PDF scripts never run, XFA forms are off, fonts are never compiled with `eval` |
| 📺 | **Older devices** | uses pdf.js's build for older browsers, so TV web views work |

## Install

```bash
npm install github:wittywilla/WittyPDF
```

TypeScript and React source, for **Vite** with `@vitejs/plugin-react` (the pdf.js worker comes with
Vite's `?url` import). Needs React 18 or newer.

## Use

Load it lazily, so pdf.js is only downloaded when a PDF is opened:

```tsx
import { lazy, Suspense } from 'react'
import { HostContext } from '@wittywilla/wittyink'
const WittyPDF = lazy(() => import('@wittywilla/wittypdf'))

<HostContext.Provider value={host}>
  <Suspense fallback={null}><WittyPDF /></Suspense>
</HostContext.Provider>
```

The host's `pdf` field is what to open: `{ url }`, or `{ data }` for a file already in memory. See
[Embedding](wiki/Embedding.md).

## The Witty family

| Project | Role |
|---|---|
| [WittyInk](https://github.com/wittywilla/WittyInk) | what the readers share |
| [WittyEbook](https://github.com/wittywilla/WittyEbook) · **WittyPDF** · [WittyComics](https://github.com/wittywilla/WittyComics) | the book, PDF and comic readers |
| [WittyReader](https://github.com/wittywilla/WittyReader) · [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) | the apps they run in |

## Licence

**Apache-2.0**, the same licence as WittyMediaServer. © 2026 WittyWilla and contributors.
Use, change and share it; if you redistribute it or build on it, keep the [NOTICE](NOTICE)
attribution, *“WittyPDF, developed by WittyWilla”*, in your NOTICE file, documentation or About screen.
The text-layer styles are adapted from pdf.js (Apache-2.0, Mozilla Foundation).

## Legal

WittyPDF is an independent open-source project. It isn't associated with, sponsored by or endorsed
by any publisher, rights holder, service or other media software project. No documents come with it,
and it doesn't check the ones you open: you're responsible for having the rights to them. Any
third-party names here only describe what it works with, and belong to their owners. It's provided
“as is”, with no warranty; see [LICENSE](LICENSE). Local law applies to how you use it. This isn't
legal advice.
