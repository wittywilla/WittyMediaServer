# Privacy and security

## What stays on your device

**Everything.** Your books, covers, reading positions, highlights, notes, bookmarks and
handwriting are stored in your browser's own storage for WittyReader, on this device. WittyReader
has no account, no analytics and no server of its own. Books are read and unpacked on the device
itself.

The one exception is something you choose: **Settings → Look up words online**. When it's on and
you press **Define**, the word (and only the word) is sent to the free dictionary at
dictionaryapi.dev. It's off until you turn it on, and the **Define** button is hidden while it's
off.

**Read aloud** uses your device's own voices. Some systems use online voices; that's set in your
system's speech settings, not by WittyReader.

## Books are untrusted

A book file can contain things you don't want running, such as scripts in an EPUB, or scripts
and forms in a PDF. WittyReader keeps them in check:

- **EPUB chapters** are shown in a sandboxed frame that can't run scripts. Every chapter is
  served with a strict Content-Security-Policy that blocks scripts, plugins and forms. Links in a
  book never navigate the frame; web links open in a new tab.
- **PDFs** are drawn by pdf.js with PDF scripting off, XFA forms off, and no code compiled from
  fonts.
- **Comics** are just images.
- **Damaged files:** archives are unpacked with a library patched against a known flaw where a
  damaged ZIP file could freeze the app.

## The app itself

- WittyReader runs under a strict Content-Security-Policy:
  - only its own scripts run;
  - it can't load plugins or submit forms;
  - the only outside address it may contact is the dictionary.
- The offline service worker only answers for WittyReader's own files and the open book. It
  never contacts other sites.
- Installing WittyReader as an app gives it no extra access to your files. It only sees the files
  you open with it.
