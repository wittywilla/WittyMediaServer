# Highlights and notes

These tools work the same way in [WittyEbook](https://github.com/wittywilla/WittyEbook/wiki)
and [WittyPDF](https://github.com/wittywilla/WittyPDF/wiki). Where your notes are kept depends
on the app:

- In **WittyMediaServer**, notes belong to your account and appear on every device you sign in
  on.
- In **WittyReader**, notes stay on the device. You can [export them](https://github.com/wittywilla/WittyReader/wiki/Notes-and-backups).

## Selecting text

Select a word or passage with the mouse, a finger (press and hold) or a pen. A toolbar appears
above it:

| Button | What it does |
|---|---|
| **Four coloured dots** | Highlights in yellow, green, blue or pink. |
| **+** | Opens 18 more colours and a picker for any colour. |
| **Note** | Highlights in yellow and opens the note editor at once. |
| **Define** | Looks the word up. Shown for selections of up to three words, when the app offers a dictionary. |
| **Copy** | Copies the text. |
| **Cite** | Copies the text as a citation (see below). |
| **Search** | Searches the whole book for the selected text. |
| **Read aloud** | Reads from that point on, with your device's voices. |

## Editing a highlight

Click or tap a highlight to open its editor:

- **Colour:** pick another from the palette.
- **Note:** type in the box. It's saved when you click away or press **Done**.
  Highlights with a note show a dotted underline (in PDFs, a line under the box).
- **Cite:** copies a citation of the highlighted text.
- **Remove:** deletes the highlight and its note. **Ctrl+Z** brings it back.

## Colour meanings

Colours mean more when you name them. Open the **notes panel** (☰), expand **Colours**, and give
each colour you have used a name, for example:

- yellow: *Key idea*
- green: *Definition*
- blue: *Question*
- pink: *Quote for essay*

The names appear when you point at a highlight, in the notes list, in the colour filter and in
exported notes. They're the same in every book on that device.

## The notes panel

**☰** in the top bar opens a list of everything you have marked in this book, in reading order:

- **Highlights:** the quoted text, its chapter or page, and the colour's meaning.
- **Notes:** shown under their highlight.
- **Bookmarks:** with the first words of the page.
- **Handwriting:** one entry per chapter or page that has some, with how many strokes.

Choose an entry to jump to it; the passage flashes so you can find it. **✕** deletes an entry.

**Filters:**

- **All / Highlights / Notes / Bookmarks / Handwriting:** show one kind of entry.
- **Filter notes:** finds entries by their text, note or colour meaning.
- **Colours:** pick a colour to show only that one; pick it again to show all.

**Export** saves all notes as a Markdown file (`Title - notes.md`):

- one heading per chapter or page;
- each highlight as a quote, followed by its colour meaning and note;
- bookmarks as a list;
- a count of handwritten strokes (handwriting itself stays in the reader).

## Bookmarks

**🔖** in the top bar (or the **b** key) bookmarks the page you are on, and a red ribbon shows in
the corner. Press it again to remove the bookmark. Bookmarks are listed in the notes panel.

## Citations

**Cite** copies the selection in this form:

> “Call me Ishmael.” (Herman Melville, Moby Dick; Or, The Whale (1851), CHAPTER 1. Loomings)

- In PDFs the place is a page number, for example *p. 12*.
- The author and year are included when the book has them.

## Definitions

**Define** shows up to two dictionary entries: the pronunciation, each part of speech with up to
three meanings, and examples. **More on Wiktionary ↗** opens the full entry.

Where definitions come from is up to the app:

- **WittyMediaServer** fetches them on the server, so your devices don't contact anyone.
- **WittyReader** only looks words up after you turn it on in its settings.

## Undo and redo

**Ctrl+Z** undoes and **Ctrl+Y** (or **Ctrl+Shift+Z**) redoes, up to 200 steps. Undo covers:

- adding highlights and handwriting;
- removing highlights and handwriting.

Changing a colour, editing a note, and adding or removing a bookmark are saved directly and are
not part of the undo history.
