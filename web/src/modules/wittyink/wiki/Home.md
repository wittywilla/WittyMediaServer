# WittyInk

WittyInk is the toolkit the Witty readers share. It contains:

- the **reader host** interface that connects a reader to the app around it;
- the **study tools**: highlights, notes, bookmarks, definitions, citations and export;
- **handwriting with pen pressure**.

| Reader | Reads | Uses WittyInk for |
|---|---|---|
| [WittyEbook](https://github.com/wittywilla/WittyEbook/wiki) | EPUB books | study tools, handwriting, host |
| [WittyPDF](https://github.com/wittywilla/WittyPDF/wiki) | PDFs | study tools, handwriting, host |
| [WittyComics](https://github.com/wittywilla/WittyComics/wiki) | Comics and manga | host |

The readers run in two apps:

- **[WittyMediaServer](https://github.com/wittywilla/WittyMediaServer)**: notes and reading
  positions are kept per account on your server and follow you to every device.
- **[WittyReader](https://github.com/wittywilla/WittyReader/wiki)**: everything stays on one
  device.

## For readers

- [Highlights and notes](Highlights-and-notes): selecting text, colours and their meanings,
  notes, bookmarks, citations, definitions and export.
- [Handwriting](Handwriting): pens, pencils, highlighters, the eraser, pressure, palm rejection
  and shortcuts.

## For developers

- [Reader host](Reader-host): connect a reader to your app.
- [Building blocks](Building-blocks): the components and functions you can reuse.
- [Styling](Styling): the classes and colours the readers expect, and `kit.css`.

Development happens in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer)
(`web/src/modules/wittyink`). Licensed under Apache-2.0.
