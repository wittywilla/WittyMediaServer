# WittyInk

What the Witty readers share: the **reader host** interface, annotations with undo and redo,
highlight colours and their meanings, the notes panel, definitions, citations, Markdown export,
and **handwriting with pen pressure** (pen, pencil, highlighter, eraser, palm rejection).

WittyInk is used by [WittyEbook](https://github.com/wittywilla/WittyEbook),
[WittyPDF](https://github.com/wittywilla/WittyPDF) and
[WittyComics](https://github.com/wittywilla/WittyComics). They run inside
[WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) and the
[WittyReader](https://github.com/wittywilla/WittyReader) app.

> This repository is a read-only mirror of `web/src/modules/wittyink` in WittyMediaServer, where
> development happens. Please open issues and pull requests there.

## Install

```bash
npm install github:wittywilla/WittyInk
```

It ships as TypeScript and React source and needs a bundler that compiles both, such as
[Vite](https://vite.dev) with `@vitejs/plugin-react`. React 18 or newer.

## The reader host

A reader never talks to your app directly. You give it a `ReaderHost`, which says where the book
comes from and where notes and the reading position are saved:

```tsx
import { HostContext, type ReaderHost } from '@wittywilla/wittyink'
import WittyEbook from '@wittywilla/wittyebook'

const host: ReaderHost = {
  key: 'moby-dick',                                  // stable id for this book
  book: { title: 'Moby Dick', author: 'Herman Melville', year: 1851 },
  format: 'epub',
  start: 0,                                          // saved position
  saveProgress: (position, total) => { /* save it */ },
  annotations: { list, add, update, remove },        // your storage
  epub: { info, chapterUrl, search },                // see WittyEbook
  define: undefined,                                 // optional dictionary
  toast: (message) => console.log(message),
  close: () => history.back(),
}

<HostContext.Provider value={host}><WittyEbook /></HostContext.Provider>
```

See `host.tsx` for every field.

## Styles

The readers use a handful of shared classes (`.btn`, `.ibtn`, `.input`, `.select`, `.seg`,
`.spinner`, `.muted`) and colour variables (`--bg`, `--fg`, `--accent`…). If your app does not
define them, import the small kit:

```ts
import '@wittywilla/wittyink/kit.css'
```

## Handwriting

Strokes are stored as compact `[x, y, pressure, …]` lists. Pen pressure sets the width, pencil
lines widen with tilt, and the highlighter can snap to text. A pen's eraser end or side button
erases. The first time a pen is used, fingers stop drawing so a resting palm leaves no marks.

## License

Apache-2.0
