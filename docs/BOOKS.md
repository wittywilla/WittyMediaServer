# Books, PDFs and comics

Use this guide to set up a library of books, PDFs, comics and manga, and read them with your place, bookmarks, highlights and notes kept on your server.

The reader runs in a browser and in the desktop WittyMediaClient, which shows the same pages. The Android and Apple apps can't open these libraries yet. [WittyReader](#wittyreader) is a separate app for books kept on your own device.

## Setting it up

1. Go to **Settings → Libraries**, or use first-time setup, and choose **Books & comics**.
2. Choose your folders. Any layout works.

If **Books & comics** is listed under **Add a module to use these**, press the button for the reader you want first.

### What needs a module

Each reader is a module an admin can add or remove in **Settings → Modules**. A book whose reader isn't on the server says so when you open it, and an admin can add it from there, with no restart.

| Module | What it reads |
|---|---|
| **WittyEbook** | EPUB, and everything converted to EPUB: Kindle, FictionBook, Word and the rest below |
| **WittyPDF** | PDFs |
| **WittyComics** | Comics and manga, and DjVu books |
| **WittyInk** | What the readers share: notes, definitions and handwriting. It comes along with any of them |

## What it can read

| Kind | Formats | How it opens |
|---|---|---|
| Books | EPUB, KEPUB | As they are |
| PDFs | PDF | As they are |
| Comics | CBZ, CBR, CB7, CBT | Page by page |
| Kindle and Palm | MOBI, PRC, AZW, AZW3, KF8, PDB | Converted to EPUB |
| Other documents | FictionBook, DOCX, ODT, DOC, RTF, Markdown, web pages, plain text | Converted to EPUB |
| DjVu | DJVU, DJV | Page by page, with the DjVuLibre tools on the server |

Conversion happens once on the server, the first time it's needed. The copy goes in the cache, and your original is left alone. A `.doc` file gives its text only, without pictures or layout. Web pages have their scripts and outside links removed first.

DjVu pages need the `ddjvu` program from DjVuLibre on the server. Without it the book is still listed, and you can download it.

Every format is in [Supported formats](SUPPORTED-FORMATS.md#books) and [comics](SUPPORTED-FORMATS.md#comics).

### How files are picked up

- **Several formats of one book:** files with the same name in a folder count as one book, and the best format is used: CBZ first, then EPUB, then PDF.
- **Text files:** plain text, Markdown and web pages under 2 KB are skipped, and so are files named like `readme`, `notes` or `license`.
- **Details:** titles, authors, covers and descriptions come from the book itself: EPUB details, `ComicInfo.xml` in a comic, or the PDF. Otherwise they come from the file name.
- **Series:** a name such as `Series Name 12 (2023)` or `Series v03` gives the series, number and year. EPUB series details are read too. The **Series** tab groups them.
- **Manga:** `ComicInfo.xml` marked as manga, or a folder with "manga" in its name, opens right to left.
- **More details:** a [custom agent](METADATA.md#custom-agents) can add details for books.

## Copy-protected and password-protected books

Nothing here removes protection.

- **Copy-protected Kindle books** are refused, with the message "this book has copy protection, so it can't be opened here".
- **Password-protected Word, OpenDocument and zipped FictionBook files** are refused with a message saying the file has a password.
- **PDFs with a password** are refused with a message saying so. The reader doesn't ask for the password. You can download the file and open it in another program.
- **Copy-protected EPUB files** are refused when you open them, with the same message as a Kindle book. A locked EPUB stays on your shelf under its title and author, with no cover from inside it, and it can still be downloaded. If there's another copy with the same name in the same folder, such as a PDF, that copy is used instead. Files whose names end in `_lcp.epub` are ignored by the scan.
- **EPUBs with a password on the file itself** are refused with a message saying so.
- **EPUBs whose embedded fonts are scrambled** aren't protected books, and open as usual. The scrambling only stops a font being copied out for other uses, so the text shows in the reader's font instead.
- **Books too large to convert** are refused: the limit is 256 MB, or 64 MB for text formats.

**Download** on the book's page gets you the file as it is.

## Reading

Open a book and press **Read**, **Continue reading** or **Read again**. Esc or the back button closes panels first, then the book.

**Your place** is saved for your account, so any device carries on where you stopped. The library shows your progress on each cover. Your layout, text and colour choices are kept on each device.

### Books (EPUB and converted formats)

**Layout,** under **Text and display:** **Auto**, **One page**, **Book view** (two pages side by side) or **Scroll**.

**Getting around:**

- Swipe, or use the arrow keys, Space and Page Up or Down. The slider moves through the whole book.
- **Contents** lists the chapters. If a book has none, its sections are listed.
- The bottom bar shows how far you are, and the **time left** in the chapter and book, from your own reading speed.
- Footnotes open in a pop-up, so you don't lose your place.
- A book reopens on the same words after you change the size of the text or the window.

**Text and display:**

- **Page:** light, sepia, dark or black, with **Brightness** and **Warmth (night light)**.
- **Font:** **Publisher's font**, **Serif**, **Sans-serif**, **Extra readable** or **Monospace**.
- **Text size** from 60% to 250%, **Line spacing**, **Margins**, and **Justify and hyphenate**.

### PDFs

- **Zoom:** **Fit width**, **Fit page**, or 50% to 400%. Ctrl and scroll, or + and −, zoom too.
- **Pages across:** **One page** or **Two pages**.
- **Page colour:** **As printed**, **Sepia** or **Dark**. Dark shows light text on dark pages, and pictures change colour too.
- **Contents:** **Outline**, the document's own contents, and **Pages**, with thumbnails.
- **Search** looks through the whole document.
- Type a number at the bottom to go to that page.

PDF scripts never run, and XFA forms are off.

### Comics and manga

The settings are behind the gear button, and are remembered for each comic on that device.

- **Layout:** **Page**, **Spread** (two pages, with the cover alone so pairs line up as printed) or **Scroll**, one long strip.
- **Reading direction:** **Left to right** or **Right to left (manga)**.
- **Fit:** **Fit height** or **Fit width**.
- **Turning pages:** tap the sides or swipe, and tap the middle to show or hide the bars. Arrow keys follow the reading direction. Space and Page Up or Down work too, and Home and End go to the first and last page.

Highlights, notes, bookmarks and handwriting aren't part of the comic reader, and comics have no text to search.

## Bookmarks, highlights and notes

These work in books and PDFs. They belong to your account on the server, and each person sees only their own.

**Bookmarks:** press the bookmark button or **b**.

**Select text** for its toolbar:

- A colour highlights it. Four colours are one press away, more are behind the next button, and any colour can be picked.
- **Note** highlights it and adds a note. Press a highlight later to change its colour or note, or to **Remove** it.
- **Define** appears for up to three words. See [what goes online](#what-goes-online).
- **Copy**, and **Cite**, which copies it with the author, title and place.
- **Search** looks for it in the book.
- **Read aloud** starts from there.

**Notes & highlights** lists everything, and can show only **Highlights**, **Notes**, **Bookmarks** or **Handwriting**, or filter by text or colour.

- **Colours** lets you name what each colour means to you, such as "Question". The names are kept on that device.
- **Export** saves your notes as a Markdown file, grouped by chapter or page. Handwriting isn't exported: it counts the strokes and they stay in the reader.

**Undo and redo:** Ctrl+Z and Ctrl+Y.

### Handwriting

Press the pen button or **p** for pen, pencil, highlighter and eraser. With a pen, you can write on the page at any time. Line width follows pen pressure, and strokes stay with their words in a book, or where you drew them in a PDF. A highlighter line across text in a book becomes a normal highlight.

### Read aloud

The speaker button reads from where you are, turning pages as it goes. It uses your device's own voices, so the sound and any online voices depend on your device.

## What goes online

Reading stays on your server, with one exception: **Define**.

When you press **Define**, your server asks the [Free Dictionary API](https://dictionaryapi.dev) about that word, and nothing else. It's in English, it's a third-party service, and you should check a definition before relying on it. Nothing is sent if you don't press it. The **More on Wiktionary** link opens that site in your browser.

## Keys

| Key | What it does |
|---|---|
| → ← | Next or previous page. In a comic, these follow the reading direction. In a PDF at **Fit width**, use Page Up and Page Down |
| Space, Page Down, Page Up | Next page, or previous with Page Up |
| b | Bookmark (books and PDFs) |
| / | Search (books and PDFs) |
| p | Handwriting tools (books and PDFs) |
| e | Eraser, with the handwriting tools open |
| Ctrl+Z, Ctrl+Y | Undo, redo |
| Esc | Close a panel, then the book |

## Who can read what

Each library can be limited per account, as everywhere. A comic's `AgeRating` in `ComicInfo.xml` sets its rating, and books without a rating are hidden from Kids and Guest profiles unless you allow titles without a rating, under **Settings → Profiles**.

## WittyReader

**WittyReader** is a separate app for reading on your own device, with no server. It's built from the same readers, so books, PDFs and comics read the same way, with highlights, notes and a pen.

- Books and notes stay in that app on that device. Nothing is uploaded, and it has no account.
- **Define** is off there until you turn it on in its settings.
- It opens EPUB, PDF and CBZ files only. The other formats here are converted by the server, so they aren't available in it.
- It doesn't connect to your server, so notes and places made in one aren't in the other. You can export a book's notes in WittyReader and import them into another copy of WittyReader.

Its installers for Windows, macOS and Linux are on its own releases page. The Windows installer has been built and tested under Wine, not on real Windows, and the Mac version hasn't been run yet.

## For developers

The routes are under Books in [Client API](CLIENT-API.md#every-route). How a reader fits into an app is in [Making a module](MODULES.md).

---

<sub>WittyMediaServer documentation</sub>
