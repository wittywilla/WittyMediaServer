# The metadata folder

Look inside the folder of plain text files the server can write for your titles, fix what's wrong with any text editor, and know what's safe to change. You don't need to know any programming, and nothing you do in this folder can break your server or your media.

It's best for lots of small fixes at once, for fixing things when the server won't start, and for anyone who'd rather work in a text editor. For one or two titles, the **Edit** button is usually quicker: see [Fixing wrong details](FIXING-METADATA.md).

- [Where it is](#where-it-is)
- [Fixing something, step by step](#fixing-something-step-by-step)
- [What's in the folder](#whats-in-the-folder)
- [What's safe to edit or delete](#whats-safe-to-edit-or-delete)
- [What a file looks like](#what-a-file-looks-like)
- [Examples](#examples)
- [When it says something is wrong](#when-it-says-something-is-wrong)
- [Taking it back](#taking-it-back)
- [What can go in a file](#what-can-go-in-a-file)
- [Files beside your media](#files-beside-your-media)
- [How it's kept safe](#how-its-kept-safe)
- [On the command line](#on-the-command-line)

## Where it is

It's the `metadata` folder inside the [data folder](CONFIG.md#folders-the-server-uses). **Settings → Repair**, on the **Metadata files** tab, shows the exact path under **Where it is on the server**.

It's empty until you export it. The server doesn't read it until you apply it. Your titles are in the server's database, and these files are only a copy to edit.

There are two other things that sound similar. Artwork the server has saved is somewhere else (the `metadataDir` setting), and files beside your media are [different again](#files-beside-your-media).

## Fixing something, step by step

1. Open **Settings → Repair**, on the **Metadata files** tab, and press **Export the files**. You can leave the page while it runs. Only an administrator can do this.
2. Open the file for the title in Notepad, TextEdit or any text editor. Change what's wrong, and save it.
3. Press **Check what would change**. It lists each change with what's there now beside what's coming, and the file and line of anything that's wrong. Nothing has changed yet.
4. Press **Apply to 1 title**. The button says how many.

What you change this way is locked, so no scan or lookup changes it back. Press **Undo all of it** if you change your mind.

## What's in the folder

```
metadata/
  README.txt
  .exported.json
  Movies/
    My Film (2001) [item-123].json
    My Film (2001) [item-123]-poster.jpg
  Shows/
    A Show [show-22]/
      show.json
      poster.jpg
      S01E01 Pilot [item-311].json
```

| What | What it is |
|---|---|
| `README.txt` | A short version of this guide. The server ignores it |
| `.exported.json` | A hidden record of what the server last wrote or applied. It's how the server knows which files you've changed |
| `Movies/`, `Shows/` | One folder for each library, with the library's name. If two names can't be told apart, one gets `[library-5]` added |
| `My Film (2001) [item-123].json` | One film, disc, episode, home video, music video, book or game |
| `A Show [show-22]/` | One show's folder |
| `show.json` | The show itself |
| `S01E01 Pilot [item-311].json` | One episode |
| `...-poster.jpg`, `poster.jpg` | Only if you put one there. See [A wrong poster](#a-wrong-poster) |

The number in square brackets is how the server knows which title a file is for. Everything else in the name is only there to help you find it, so you can rename a file as long as the brackets stay. A file stays where it is if you rename the title, so its name may be out of date. That does no harm.

Films, shows, episodes, discs, home videos, music videos, books and games get files. Music, audiobooks, podcasts, photos and radio don't, because these fields don't describe them.

You can also put a hand-written `.nfo` here, if its name has the number in square brackets. It's read the same way, and its pictures have to be web addresses.

## What's safe to edit or delete

Anything in this folder is safe. Here's what happens with each.

| If you | What happens |
|---|---|
| **Edit a title's file** | Nothing, until you check and apply. Then what you changed is locked |
| **Delete one line from a file** | That thing stays as it is. A file can be cut down to the one thing it changes |
| **Empty a value** (`""` or `[]`) | That thing is cleared when you apply, except a title, which can't be empty |
| **Delete a title's file** | Nothing happens to the title. **Export again** writes a new file |
| **Delete the whole folder** | Nothing happens to your titles, and anything you've applied stays. Export again to start over. This is the way out of any muddle |
| **Rename a file** | Fine, as long as the part in square brackets stays |
| **Delete `README.txt`** | The next export writes it again |
| **Delete `.exported.json`** | The server can't tell which files you changed, so an old file can count as an edit, and every difference from the server is listed when you check. Read the list before you apply. Deleting the whole folder is cleaner |
| **Leave other files here** | They're ignored, with a note. Backup files your editor leaves, such as `.bak`, `.tmp` and `~` files, get no note at all |

Leave the first four lines of a file alone (`wittyMetadata`, `target`, `id` and `path`). They say what the file is for, and the server uses them to catch mix-ups.

**The rest of the data folder is different.** Don't edit `wms.db`, which is the server's database, or the folders beside this one. Use [Repair](REPAIR.md) instead, and make a [backup](BACKUP.md) before anything risky. A backup includes this folder too.

## What a file looks like

```json
{
  "wittyMetadata": 1,
  "target": "item",
  "id": 123,
  "path": "D:\\Media\\Movies\\My Film (2001).mkv",
  "title": "My Film",
  "sortTitle": "My Film",
  "year": 2001,
  "director": "",
  "edition": [],
  "tagline": "",
  "overview": "A lighthouse keeper finds a boat.",
  "genres": ["Drama", "Mystery"],
  "tags": [],
  "contentRating": "PG",
  "rating": 7.5,
  "studios": ["Lantern Works"],
  "poster": "https://example.com/my-film.jpg",
  "locked": []
}
```

The rules are few. Text goes in `"quotes"`. A number has none. A list goes in `[square brackets]`. Every line but the last has a comma at its end.

## Examples

### A wrong title and year

Before:

```json
  "title": "Harbour Ligts",
  "sortTitle": "Harbour Ligts",
  "year": 1998,
```

After:

```json
  "title": "Harbour Lights",
  "sortTitle": "Harbour Lights",
  "year": 2004,
```

### A wrong poster

Either put a picture beside the file, or give a web address.

**A picture beside the file.** If the file is `My Film (2001) [item-123].json`, call the picture `My Film (2001) [item-123]-poster.jpg`. Use `-backdrop`, `-banner` or `-logo` for those. Beside a `show.json`, it's just `poster.jpg`, `backdrop.jpg`, `banner.jpg` or `logo.jpg`. A picture there comes before a web address in the file. When you apply it, it's copied into the server's own artwork.

**A web address**, in the file:

```json
  "poster": "https://example.com/my-poster.jpg",
```

**Or the plain name of a picture** in the same folder, whatever it's called:

```json
  "poster": "cover.png",
```

Pictures have to be JPEG, PNG, WebP or GIF, up to 20 MB. If a title has a picture only the server can find, such as a frame from the video, the file has no `poster` line, and you can add one.

### Wrong genres

```json
  "genres": ["Drama", "Mystery"],
```

### A summary with several lines

A line break in text is written `\n`, and a quote inside text is written `\"`:

```json
  "overview": "First paragraph.\n\nThe \"second\" paragraph.",
```

### The wrong match

To say which entry on a service a title is, give both lines. The services are `tmdb`, `tvdb`, `tvmaze`, `anilist`, `wikidata` and `imdb`:

```json
  "provider": "tmdb",
  "providerId": "12345",
```

Nothing is looked up when you apply this. To choose from a list of results with a preview, use [Fix matches](FIXING-METADATA.md#the-fix-matches-page). Empty both lines to remove a match.

### Letting a field follow its source again

`"locked"` lists the fields no scan or lookup may change. Anything you change in a file locks itself. To unlock something, take it out of the list.

```json
  "locked": ["title", "year"]
```

## When it says something is wrong

Every message names the file and the line, like this:

```
Movies/Quiet Pier (2011) [item-2].json:9: A comma is missing at the end of the line before this one
```

That means line 9 of that file. Look at that line and the one before it. A file with a problem is left out, and the others still apply.

| What it says | What to do |
|---|---|
| **A comma is missing at the end of the line before this one** | Add a comma at the end of the line above |
| **Something is wrong just before here. Often it's a comma after the last line** | Take the comma off the last line before the closing `}` |
| **This line has something that isn't a value** | Text needs `"quotes"`. A number doesn't |
| **There's something a text can't hold as it is** | Write a quote as `\"`, a backslash as `\\`, and a new line as `\n` |
| **The file ends too soon** | The closing `}` or a `"` has gone missing from the end |
| **"year" has to be a year such as 2001** | A number with no quotes, between 1800 and 3000 |
| **"rating" has to be a number from 0 to 10** | The same, from 0 to 10 |
| **"genres" has to be a list in [square brackets]** | `["Drama"]`, even for only one |
| **"x" isn't something this file can set. Did you mean "y"?** | A spelling slip. It's left out, and the rest of the file is used. Names are case sensitive |
| **"poster": only http and https addresses are accepted** | Pictures are only web addresses or file names, and an address can't have a login in it |
| **This picture can't be used** | The file isn't really a JPEG, PNG, WebP or GIF. Convert it, or use another |
| **This file was written for one thing, but this number is now another** | The number means another title now, perhaps after a restore. Delete the file and export again |
| **There's no title with the number…** | That title has gone from the server. Delete the file |
| **This file and another describe the same title** | Delete one of them |
| **This file is saved as UTF-16** | Save it again as UTF-8. In Notepad it's in the **Save As** window, beside **Encoding** |
| **HTML or hidden characters were taken out** | Only a note. They were removed, and the rest was used |

**Starting a file over:** delete it, and press **Export again**.

**Starting the whole folder over:** delete it and export again. Anything you've applied stays.

## Taking it back

Right after applying, press **Undo all of it**. Later, open **Fix matches**, choose **History**, and undo it there. Everything from one apply is one entry, and it goes back as one. The last 5,000 changes are kept.

After undoing, press **Export again** to bring the files back into line.

Some things worth knowing:

- **A file you haven't touched changes nothing.** So if you change a title in the app after exporting, its old file can't change it back.
- **An export never writes over your work.** A file you've edited and not applied stays as it is, and the export says how many it kept. It also never deletes anything.
- **A file you've changed says everything in it.** If you edit a file exported a while ago, everything in it that differs from the server is listed as a change. Read the list: it's why it's there. Or delete the file and export again before you edit.

## What can go in a file

| Name | What it is | Limit |
|---|---|---|
| `title` | The name. Can't be empty | 300 characters |
| `sortTitle` | How it sorts. If you change the title and leave this out, it's worked out for you | 300 |
| `year` | A number | 1800 to 3000 |
| `overview` | The summary | 20,000 |
| `tagline` | One line | 500 |
| `genres`, `tags` | Lists | 30 of 60 characters; 50 of 60 |
| `contentRating` | `PG`, `TV-14` and so on. Kids and Guest profiles go by this | 20 |
| `rating` | A score | 0 to 10 |
| `studios` | A list | 12 of 120 |
| `network`, `status` | Shows only | 120; 40 |
| `director`, `edition` | Films | 300; a list of 8 of 60 |
| `aired`, `season`, `episode`, `episodeEnd` | Episodes only | |
| `introStart`, `introEnd` | Where the intro is, in seconds | Up to 86,400 |
| `poster`, `backdrop`, `banner`, `logo` | A web address, or the name of a picture beside the file | 20 MB a picture |
| `locked` | A list of the names above | |
| `provider`, `providerId` | Which entry on which service | |

A field that kind of title doesn't have, such as `network` on a film, gets a note and is left out.

## Files beside your media

These are separate from the metadata folder. They live next to your films and shows, and other media software can read them too.

### What the server writes: saveMetadata

The setting is **Save information and artwork next to your media**, under **Settings → Metadata agents**. Its name on the command line and in the API is `saveMetadata`. It's on by default.

With it on, the server writes the files listed in [What is saved next to your media](METADATA.md#what-is-saved-next-to-your-media): NFO files, `poster.jpg`, `fanart.jpg` and so on. With it off, nothing is written next to your media, your details stay in the server's database, and the metadata folder still works as usual. Turn it off if your media is read-only or you'd rather the server stayed out of those folders.

### What's safe to do with them

| If you | What happens |
|---|---|
| **Edit an NFO the server wrote** | A scan reads your edit. But the file still has the line `<!-- written by WittyMediaServer -->`, so the server treats it as its own, and may write over it. Delete that line to make it yours |
| **Write your own NFO** | A scan reads it, and never writes over it. If you edit that title in the app with saving on, your original is kept as `<name>.nfo.bak` |
| **Delete an NFO or picture** | With saving on, the server writes a new one at the next scan. With it off, it stays gone. What the server knows stays in its database |
| **Delete a `.nfo.bak`** | Nothing. It's only a copy of your old file |

Your media files are never changed by any of this.

### What the server reads

| File | What it is | Largest it reads |
|---|---|---|
| `<name>.nfo`, `movie.nfo`, `tvshow.nfo` | Standard details, from this server or other software | 1 MB |
| `<file>.metadata.json`, `<name>.metadata.json`, `metadata.json` | Details in this project's own simple form | 256 KB |
| `.info.json`, `<name>.json` | What video archivers leave beside a video, in creator libraries | 48 MB |
| `<name>.wittycd.json` | Names and release saved for a CD image | 1 MB |

A `.metadata.json` looks like this, and every line is optional:

```json
{ "title": "My Film", "year": 2001, "overview": "A lighthouse keeper finds a boat.", "genres": ["Drama"], "poster": "https://example.com/my-film.jpg" }
```

A file that can't be read is left out, and the rest of the title still works. To see which:

- **Settings → Repair** lists them under `nfo-files`, with the file and the line.
- **Settings → WittyLog**, in the **Library scans** area, names them too.

Most often it's a stray `&`, which has to be written `&amp;`, or a tag that was never closed.

## How it's kept safe

This folder can hold anything, including something you were sent or downloaded. So nothing in it is believed:

- **Text is only text.** HTML, scripts, comments, control characters and tricks that reverse how text reads are taken out, and you're told. The app also shows these fields as text, never as markup.
- **A web address has to be one.** Only `http` and `https`, with no login in it. Not `javascript:`, `data:` or `file:`.
- **A picture has to be one.** It's judged by what's in the file and not its name. SVG isn't accepted, because it can hold scripts.
- **Nothing leads out of the folder.** A file name can't hold a folder or `..`. Links, shortcuts and junctions in the folder aren't followed, and nothing is written through them.
- **Sizes are limited.** 256 KB for a file, 1 MB for an NFO, 20 MB for a picture, 500,000 files in all, and six folders deep.
- **NFOs can't declare anything.** A `DOCTYPE` or `ENTITY` is refused, because that's how an XML file can be made to read others.
- **Only an administrator can apply.** It goes through the same checks as the edit dialog, and every change is recorded in **History**.

The same cleaning is done to anything read from beside your media: `.nfo`, `.metadata.json`, `.info.json`, `.wittycd.json`, and the details inside a book or comic. A link beside your media may be an ordinary part of your folders, so it can be followed there, but it must end at an ordinary file within the size limit.

## On the command line

These work on any system, with the server stopped, for instance if it won't start. Add `-data <folder>` if yours isn't the default.

```
wittymediaserver metadata export
wittymediaserver metadata export -library 3
wittymediaserver metadata check
wittymediaserver metadata apply
wittymediaserver metadata apply --apply
wittymediaserver metadata undo <batch>
```

| Command | What it does |
|---|---|
| `metadata export` | Writes the files. `-library` takes a library's number, for only that one |
| `metadata check` | Says what's wrong with them, by file and line. It needs no database |
| `metadata apply` | Says what they would change. Nothing changes |
| `metadata apply --apply` | Changes it, and tells you the batch |
| `metadata undo <batch>` | Takes back everything that apply changed |

`check` and `apply` end with a non-zero exit code if a file has a problem, so they can be used in a script. If a server is running on the same folder, `apply --apply` and `undo` refuse and point you to Settings, because the server is the one holding those files.

---

<sub>WittyMediaServer documentation</sub>
