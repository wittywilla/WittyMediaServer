<h1 align="center">WittyInk</h1>

<p align="center">
  <b>The study and handwriting toolkit behind the Witty readers.</b><br>
  Highlights, notes, bookmarks, definitions and citations, a pen with pressure, and the panels and pop-ups every reader shares.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="React 18+" src="https://img.shields.io/badge/React-18%2B-61dafb.svg">
  <img alt="TypeScript" src="https://img.shields.io/badge/TypeScript-source-3178c6.svg">
</p>

<p align="center">
  <a href="#install">Install</a> ·
  <a href="#quick-start">Quick start</a> ·
  <a href="#building-blocks">Building blocks</a> ·
  <a href="wiki/Home.md"><b>Full guide</b></a>
</p>

> Read-only mirror of `web/src/modules/wittyink` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

WittyInk is what [WittyEbook](https://github.com/wittywilla/WittyEbook), [WittyPDF](https://github.com/wittywilla/WittyPDF)
and [WittyComics](https://github.com/wittywilla/WittyComics) share. The readers run inside
[WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) and the
[WittyReader](https://github.com/wittywilla/WittyReader) app.

## Highlights

| | For readers | |
|---|---|---|
| 🖍️ | **Highlights in any colour** | four quick colours, 18 more, or any you pick; edit by clicking a highlight |
| 🏷️ | **Colour meanings** | name what each colour means (“Definition”, “Question”); shown in filters and exports, the same in every book |
| 📝 | **Notes panel** | every highlight, note, bookmark and handwritten page, filtered by kind, colour or text |
| 📖 | **Definitions & citations** | pronunciation and examples; citations as *“quote” (Author, Title (Year), p. 12)* |
| 📤 | **Markdown export** | all notes, grouped by chapter or page, with your colour meanings |
| ✍️ | **Pen with pressure** | pen, pencil (tilt widens it, with grain), highlighter, eraser; 36 ink and 18 highlighter colours |
| 🖐️ | **Palm rejection** | after the first pen touch, fingers turn pages instead of drawing |
| ↩️ | **Undo and redo** | Ctrl+Z / Ctrl+Y, up to 200 steps, for highlights and handwriting |

The pen's eraser end and side button erase; every tool's colour, thickness and opacity is remembered
on each device.

## Install

```bash
npm install github:wittywilla/WittyInk
```

TypeScript and React source, for a bundler that compiles both, such as [Vite](https://vite.dev) with
`@vitejs/plugin-react`. Needs React 18 or newer.

## Quick start

A reader talks to the app around it through one interface, the `ReaderHost`:

```tsx
import { HostContext, type ReaderHost } from '@wittywilla/wittyink'
import WittyEbook from '@wittywilla/wittyebook'

const host: ReaderHost = {
  key: 'moby-dick',
  book: { title: 'Moby Dick', author: 'Herman Melville', year: 1851 },
  format: 'epub',
  start: 0,
  saveProgress: (position, total) => { /* remember it */ },
  annotations: { list, add, update, remove },   // your storage
  epub: { info, chapterUrl, search },           // see WittyEbook
  toast: (message) => showMessage(message),
  close: () => history.back(),
}

<HostContext.Provider value={host}><WittyEbook /></HostContext.Provider>
```

The same reader saves to a server in one app and to the device in another.
[Reader host](wiki/Reader-host.md) explains every field.

## Building blocks

| Part | What it is |
|---|---|
| `useAnnotations` | the book's notes, with undo and redo |
| `Sheet`, `Field`, `Range` | the side panel (header, scrolling body, backdrop), a settings row, a filled slider |
| `NotesPanel`, `SelectionBar`, `HighlightEditor`, `Define` | the notes list, the toolbar over selected text, the highlight editor, definitions |
| `InkBar`, `InkStroke`, stroke geometry | the pen toolbar and strokes with pressure |
| `cite`, `exportNotes` | citations and Markdown export |
| icons | one consistent icon set, drawn the same on every system |
| `reader.css`, `kit.css` | the shared reader chrome; an optional small kit (colours, buttons, inputs) for apps without their own |

[Building blocks](wiki/Building-blocks.md) and [Styling](wiki/Styling.md) have the details.

## The Witty family

| Project | Role |
|---|---|
| **WittyInk** | what the readers share |
| [WittyEbook](https://github.com/wittywilla/WittyEbook) · [WittyPDF](https://github.com/wittywilla/WittyPDF) · [WittyComics](https://github.com/wittywilla/WittyComics) | the book, PDF and comic readers |
| [WittyReader](https://github.com/wittywilla/WittyReader) · [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) | the apps they run in |

## Licence

**Apache-2.0**, the same licence as WittyMediaServer. © 2026 WittyWilla and contributors.
Use, change and share it; if you redistribute it or build on it, keep the [NOTICE](NOTICE)
attribution, *“WittyInk, developed by WittyWilla”*, in your NOTICE file, documentation or About screen.

## Legal

WittyInk is an independent open-source project. It isn't associated with, sponsored by or endorsed
by any publisher, rights holder, service or other media software project. It doesn't provide or
check the books you open with it. Any third-party names here only describe what it works with, and
belong to their owners. It's provided “as is”, with no warranty; see [LICENSE](LICENSE). Local law
applies to how you use it. This isn't legal advice.
