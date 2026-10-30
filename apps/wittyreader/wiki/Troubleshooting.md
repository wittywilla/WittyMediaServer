# Troubleshooting

### "This browser cannot show EPUB books here"

EPUBs are shown through the app's service worker, which browsers only allow on **https** or
`localhost` addresses, and not in some private-browsing modes. Open WittyReader from an https
address, or leave private browsing. PDFs and comics still work either way.

### A book won't open: "is not a book WittyReader can open"

WittyReader opens EPUB, PDF and CBZ files. Other formats, `.cbr` and `.cb7` comics, and books
with copy protection (DRM) aren't supported yet. A file can also be damaged; try downloading it
again.

### A PDF says it cannot be opened

Password-protected PDFs aren't supported yet. Remove the password with the tool that made the
PDF, or open it elsewhere.

### My library is empty after clearing browser data

The library lives in the browser's storage for the site, so clearing it removes books and notes.
Add the books again, then import any notes you exported (**Settings → Import notes…**). See
[Notes and backups](Notes-and-backups).

### The Define button is missing

Turn on **Settings → Look up words online**. It appears for selections of up to three words.
English definitions only, for now.

### Read aloud doesn't speak

Read aloud uses your device's voices. Check that your system has a voice installed for the
book's language and that the sound is up. Some browsers need you to click on the page once first.

### My pen draws when I want to select text

A pen writes at any time by default. To make it select text instead, open the ink toolbar (the **pen** button),
choose **⋯**, and turn off **Draw with a pen any time**.

### My hand leaves marks / my finger won't draw

**Only a pen draws** turns itself on the first time a pen touches the page, so fingers turn pages
instead of drawing. To draw with a finger, turn it off in the ink toolbar's **⋯** settings.

### The app looks out of date after an update

Close all WittyReader windows and open it again while online.
