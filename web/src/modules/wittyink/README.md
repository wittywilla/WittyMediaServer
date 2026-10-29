# WittyInk

**The study and handwriting toolkit behind the Witty readers.** WittyInk is what
[WittyEbook](https://github.com/wittywilla/WittyEbook), [WittyPDF](https://github.com/wittywilla/WittyPDF)
and [WittyComics](https://github.com/wittywilla/WittyComics) share. It contains:

- the **reader host** interface that connects a reader to the app around it;
- highlights, notes, bookmarks, definitions, citations and export;
- **handwriting with pen pressure**.

The readers run inside [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) and the
[WittyReader](https://github.com/wittywilla/WittyReader) app.

📖 **[Read the full guide](wiki/Home.md)** in the `wiki` folder.

> This repository is a read-only mirror of `web/src/modules/wittyink` in
> [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer), where development happens.
> Please open issues and pull requests there.

## What it does

### Highlights and notes

- **Select text** to get a toolbar with:
  - four quick highlight colours, and **+** for 18 more or any colour you pick;
  - **Note**, **Define**, **Copy**, **Cite**, **Search** and **Read aloud**.
- **Colour meanings:** name what each colour means to you ("Definition", "Question",
  "Quote for essay"). The names show on highlights, in filters and in exports, and are the same
  in every book.
- **Edit a highlight** by clicking it: change its colour, write or edit its note, copy a
  citation, or remove it.
- **Notes panel:**
  - lists every highlight, note, bookmark and handwritten page, in reading order;
  - filters by kind (all, highlights, notes, bookmarks, handwriting), by colour or by any text;
  - jumps straight to any entry.
- **Citations** are copied in the form *"quote" (Author, Title (Year), Chapter or p. 12)*.
- **Definitions** show pronunciation, parts of speech and examples, with a link to Wiktionary.
  The app hosting the reader decides where they come from.
- **Export** all notes as Markdown, grouped by chapter or page, including your colour meanings.
- **Undo and redo** (Ctrl+Z, Ctrl+Y) for highlights and handwriting, up to 200 steps.

### Handwriting

- **Tools:** pen, pencil, highlighter and eraser.
- **Pen pressure** sets the line width as you write. **Pencil** lines widen when the pen is
  tilted and have a light grain.
- **Colours:** 36 ink colours, 18 highlighter colours, any custom colour, and your recent colours.
- **Thickness and opacity:** six thickness presets per tool, plus a slider for exact thickness
  and opacity.
- **Palm rejection:** the first time a pen touches the page, fingers stop drawing, so a resting
  hand leaves no marks. Fingers still turn and move pages.
- **Write any time:** a pen can write without opening the tools first.
- **Erasing:** use the eraser tool, the pen's eraser end, or its side button.
- **Highlighter that snaps to text**, where the reader supports it (WittyEbook).
- **Remembered per device:** every tool's colour, thickness and opacity.

### For developers

- **`ReaderHost`:** the single interface between a reader and the app. It covers book
  information, the saved position, notes storage, file locations, an optional dictionary,
  messages and closing. The same reader saves to a server in one app and to the device in
  another.
- **Building blocks** you can use in your own reader: `useAnnotations` (notes with undo),
  `NotesPanel`, `SelectionBar`, `HighlightEditor`, `Define`, `InkBar`, `InkStroke`, the stroke
  geometry, `cite` and `exportNotes`.
- **`kit.css`:** an optional small stylesheet (colours, buttons, inputs) for apps without their
  own design system.

## Install

```bash
npm install github:wittywilla/WittyInk
```

WittyInk ships as TypeScript and React source, for a bundler that compiles both, such as
[Vite](https://vite.dev) with `@vitejs/plugin-react`. Needs React 18 or newer.

## Quick start

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

The guide's [Reader host](wiki/Reader-host.md) page
explains every field.

## License

Apache-2.0, the same licence as [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).
Copyright 2026 WittyWilla and contributors.

You're welcome to use, change and share WittyInk. If you redistribute it or build on it, the licence
asks you to keep the [NOTICE](NOTICE) file's attribution, *"WittyInk, developed by WittyWilla"*, in
your own NOTICE file, documentation or About screen.
