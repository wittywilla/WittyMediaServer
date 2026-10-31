<h1 align="center">WittyEbook</h1>

<p align="center">
  <b>An ebook reader built for long and careful reading.</b><br>
  EPUB 2 and 3, a book-like page set to your taste, the study tools serious readers need, and a pen with pressure.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="React 18+" src="https://img.shields.io/badge/React-18%2B-61dafb.svg">
  <img alt="EPUB 2 and 3" src="https://img.shields.io/badge/EPUB-2%20·%203-8b6cff.svg">
</p>

<p align="center">
  <a href="#install">Install</a> ·
  <a href="#use">Use</a> ·
  <a href="wiki/Keyboard-shortcuts.md">Keyboard shortcuts</a> ·
  <a href="wiki/Home.md"><b>Full guide</b></a>
</p>

> Read-only mirror of `web/src/modules/wittyebook` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

Runs inside [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) and the
[WittyReader](https://github.com/wittywilla/WittyReader) app.

## Highlights

| | Reading | |
|---|---|---|
| 📖 | **Book view** | two pages side by side with a soft fold, one page, or scrolling; *Auto* picks for the screen |
| 🎯 | **Exact resume** | the place is a spot in the text, so the book reopens on the same words at any size, on any device |
| ⏱️ | **Time left** | in the chapter and the book, from your own reading speed |
| 🔤 | **Your typography** | five font choices, 60–250 % size, line spacing, margins, justification with hyphenation |
| 🌙 | **Comfortable at night** | light, sepia, dark and black pages, brightness, and a warm night light |
| 🦶 | **Footnotes in a pop-up** | read a note without leaving the page |

| | Study tools (from [WittyInk](https://github.com/wittywilla/WittyInk)) | |
|---|---|---|
| 🖍️ | **Highlights and notes** | tied to the words, so they survive layout changes; colour meanings; Markdown export |
| 🔎 | **Search the book** | snippets with the match marked; the passage flashes when you jump to it |
| ✍️ | **Pen with pressure** | pen, pencil, highlighter, eraser; strokes stay with their words; a highlighter line over text becomes a highlight |
| 🔊 | **Read aloud** | paragraph by paragraph, turning pages and chapters |

**Safe with any book:** chapters show in a sandbox where no script can run, under a strict content
policy, and links never navigate the page away.

## Install

```bash
npm install github:wittywilla/WittyEbook
```

TypeScript and React source, for a bundler such as Vite with `@vitejs/plugin-react`. Needs React 18
or newer. [WittyInk](https://github.com/wittywilla/WittyInk) is installed with it.

## Use

```tsx
import { HostContext } from '@wittywilla/wittyink'
import WittyEbook from '@wittywilla/wittyebook'

<HostContext.Provider value={host}><WittyEbook /></HostContext.Provider>
```

The host's `epub` field supplies the book: its chapters, contents and word counts, where each chapter
is loaded from, and search. See [Embedding](wiki/Embedding.md).

| Guide | |
|---|---|
| [Reading and layout](wiki/Reading-and-layout.md) | layouts, turning pages, progress, time left |
| [Text and display](wiki/Text-and-display.md) | fonts, sizes, page colours, brightness |
| [Study tools](wiki/Study-tools.md) | highlights, notes, search, definitions, handwriting |
| [Keyboard shortcuts](wiki/Keyboard-shortcuts.md) | every key |

## The Witty family

| Project | Role |
|---|---|
| [WittyInk](https://github.com/wittywilla/WittyInk) | what the readers share |
| **WittyEbook** · [WittyPDF](https://github.com/wittywilla/WittyPDF) · [WittyComics](https://github.com/wittywilla/WittyComics) | the book, PDF and comic readers |
| [WittyReader](https://github.com/wittywilla/WittyReader) · [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) | the apps they run in |

## Licence

**Apache-2.0**, the same licence as WittyMediaServer. © 2026 WittyWilla and contributors.
Use, change and share it; if you redistribute it or build on it, keep the [NOTICE](NOTICE)
attribution, *“WittyEbook, developed by WittyWilla”*, in your NOTICE file, documentation or About screen.

<!-- module-kit -->
## Releasing

A version tag builds the bundle for the browser, signs the checksum list, and publishes a release that servers can install.

Versions move in small steps as work lands. `go run github.com/wittywilla/wittymodule/cmd/wittymodule@v0.2.0 bump .` raises it
everywhere it is written (add `-part minor` for a bigger step). A bump isn't a release: nothing is
tagged or published until you tag it. CI fails when code changed since the last tag and the version
didn't, because tags never move.

To release:

1. Check that `version` in `module.json` is the one to publish, with the bump committed.
2. Tag that commit and push the tag: `git tag vX.Y.Z`, then `git push origin vX.Y.Z`. A tag with a
   suffix, such as `v1.2.0-beta`, is published as a prerelease.
3. Watch the *Actions* tab. The signing step uses the `WMS_SIGNING_KEY` secret of the `release` environment, and nothing is published without it.

To take a version back, run the *withdraw* workflow from the *Actions* tab. It adds the version to a signed list on the release tagged `withdrawals`, which servers read.

Licensed under Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
<!-- /module-kit -->
