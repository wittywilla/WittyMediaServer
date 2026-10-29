# Notes and backups

## Where your notes live

Highlights, notes, bookmarks, handwriting and reading positions are saved **on this device, in
this browser**, the moment you make them. There is no account and no cloud copy.

This means:

- your notes are private to this device;
- they're lost if the browser's data for the site is cleared, the browser is uninstalled, or
  the device is reset;
- another browser on the same computer has its own separate library.

**Export the notes you care about**, especially before clearing browser data or changing devices.

## Export a book's notes (to keep or move)

In the library, choose **⋯ → Export notes** on a book. WittyReader saves
`Title - notes.json`. It contains everything you made in that book:

- highlights with their colours and notes;
- bookmarks;
- handwriting.

It also records which book they belong to.

## Import notes

1. Add the same book to WittyReader on the new device (or after clearing data).
2. Open **Settings → Import notes…** and choose the `.json` file.

WittyReader matches the notes to the book by its file fingerprint, or by its title if you're
using a different copy. Notes already in the book aren't added twice, so importing the same file
again is safe.

## A readable summary (Markdown)

Inside a book, open the notes panel **☰** and choose **Export**. This saves `Title - notes.md`,
a readable document that works well in note-taking apps:

- one heading per chapter or page;
- your highlights as quotes, with their colour meanings and notes;
- bookmarks;
- a count of handwritten strokes.

It's for reading and writing up. To move notes back into WittyReader, use the `.json` export.

## Moving to WittyMediaServer

[WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) uses the same readers and
keeps notes per account, synced to every device. Importing WittyReader's notes files into the
server is planned.
