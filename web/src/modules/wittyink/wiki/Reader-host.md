# Reader host

A Witty reader never talks to your app directly. You give it a **`ReaderHost`** through
`HostContext`, and the reader asks the host for everything else: the book, where to save the
reading position and notes, and where pages come from. That is how the same reader saves to a
server in WittyMediaServer and to the device in WittyReader.

```tsx
import { HostContext, type ReaderHost } from '@wittywilla/wittyink'
import WittyEbook from '@wittywilla/wittyebook'

<HostContext.Provider value={host}>
  <WittyEbook />
</HostContext.Provider>
```

Pick the reader by `host.format`: WittyEbook for `epub`, WittyPDF for `pdf` (load it lazily;
pdf.js is large), WittyComics for `cbz`.

## Fields

| Field | Type | What it is |
|---|---|---|
| `key` | `string` | A stable id for the book. Used for per-book settings (for example a comic's reading direction). |
| `book` | `{ title, author?, year? }` | Shown in the top bar and used in citations and exports. |
| `format` | `'epub' \| 'pdf' \| 'cbz'` | Which reader the book needs. |
| `reading` | `string?` | Comics only: `ltr`, `rtl` (manga) or `webtoon` (vertical strip), the first layout. |
| `start` | `number` | The saved position to open at (see *Positions*). |
| `saveProgress(position, total)` | function | Called as the reader moves. Readers already throttle it, so saving each call is fine. |
| `annotations` | object | Your storage for notes (see *Annotations*). |
| `epub` | object | EPUB books: see [WittyEbook](https://github.com/wittywilla/WittyEbook/wiki/Embedding). |
| `pdf` | `{ url?, data? }` | PDFs: what pdf.js opens, an address or the file's bytes. |
| `comic` | `{ pages, pageUrl(n) }` | Comics: page count and each page's image address. |
| `define(word)` | function, optional | Returns dictionary entries. Leave it out and the **Define** button is hidden. |
| `toast(message)` | function | Shows a short message ("Bookmarked", errors). |
| `close()` | function | Leaves the reader (the back arrow and **Esc**). |

## Positions

`start` and `saveProgress` use one number whose meaning depends on the reader:

- **WittyEbook:** chapter index plus how far into the chapter's *text* (0 to 1). For example
  `12.5` is halfway through the 13th spine item. Because it's a place in the text rather than a
  page, the same words open on any screen or font size. `total` is the number of chapters.
- **WittyPDF:** page index plus how far down the page. `total` is the page count.
- **WittyComics:** the page index. `total` is the page count.

`position / total` is a fair "percent read" for a library view.

## Annotations

```ts
annotations: {
  list(): Promise<Ann[]>                          // this book's notes, any order
  add(a: Partial<Ann>): Promise<Ann>              // save a new one; return it with its id and created time
  update(a: Ann, patch: Partial<Ann>): Promise<void>  // colour or note changed
  remove(a: Ann): Promise<void>
}
```

An `Ann` is a bookmark, highlight or handwritten stroke:

| Field | Meaning |
|---|---|
| `id` | Your id for it (a number). |
| `kind` | `bookmark`, `highlight` or `ink`. |
| `chapter` | The chapter (WittyEbook) or page (WittyPDF), 0-based. |
| `pos` | Where it is, in the same units as positions (for sorting and jumping). |
| `quote` | The highlighted text; for bookmarks the page's first words; for ink nearby words. |
| `prefix`, `suffix` | Up to 40 characters of text either side of a highlight, so it is found again if the same words appear twice. |
| `color` | `yellow`, `green`, `blue`, `pink` or `#rrggbb`. |
| `note` | The note, if any. |
| `data` | JSON (see below). |
| `created` | Unix time in seconds. |

What `data` holds:

- **Highlights in WittyEbook:** nothing. A highlight is found by its quote and context.
- **Highlights in WittyPDF:** the boxes it covers, as JSON
  `[[page, x, y, width, height], …]`, each a fraction of the page.
- **Ink:** the stroke as JSON `{ t, w, o, a, pts }`:
  - `t`: the tool (`pen`, `pencil` or `marker`);
  - `w`: the width;
  - `o`: the opacity;
  - `a`: the anchor;
  - `pts`: a flat `[x, y, pressure, …]` list.

  In WittyEbook, `a` is the character offset the stroke is tied to, and units are ems from that
  character. In WittyPDF, `a` is 0 and units are PDF points from the page's top-left corner.

Treat `data` as opaque: store it and return it unchanged. Keep strokes up to about 256 KB each
(WittyMediaServer's limit).

## Examples

- **A server host** (WittyMediaServer):
  [`web/src/pages/Reader.tsx`](https://github.com/wittywilla/WittyMediaServer/blob/master/web/src/pages/Reader.tsx).
  It uses the HTTP API with notes per account.
- **A device host** (WittyReader):
  [`src/books.ts`](https://github.com/wittywilla/WittyReader/blob/main/src/books.ts). It uses
  IndexedDB, with EPUBs unpacked on the device.
