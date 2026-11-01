# Fixing metadata by hand

The server can write what it knows about each film, show and episode into a folder of plain text files. You can
open one in any text editor, fix what's wrong, and apply it. You don't need to know anything about programming, and
nothing you do in this folder can break your server or your media.

There are other ways to fix a title, and they're usually quicker for one or two: **Edit** on a title's page, and
**Fix matches** for the wrong film or show ([Metadata](METADATA.md#editing-and-fixing-matches)). The folder is
better for lots of small fixes at once, for fixing things when the server won't start, and for anyone who'd
rather work in a text editor.

- [The short version](#the-short-version)
- [What's in the folder](#whats-in-the-folder)
- [Examples](#examples)
- [When it says something is wrong](#when-it-says-something-is-wrong)
- [Taking it back](#taking-it-back)
- [What can go in a file](#what-can-go-in-a-file)
- [How it's kept safe](#how-its-kept-safe)
- [On the command line](#on-the-command-line)

## The short version

1. Open *Settings → Repair → Metadata files* and press **Export the files**. The page says where the folder is. It's
   `metadata` inside the [data folder](CONFIG.md#folders-the-server-uses).
2. Open the file for the title in Notepad, TextEdit or any text editor. Change what's wrong, and save.
3. Press **Check what would change**. It lists each change with what's there now beside what's coming, and the
   file and line of anything that's wrong. Nothing has changed yet.
4. Press **Apply**.

What you change this way is locked: no scan or lookup changes it back.

## What's in the folder

```
metadata/
  README.txt                                     what you're reading now, in brief
  Movies/
    My Film (2001) [item-123].json               one film
    My Film (2001) [item-123]-poster.jpg         a picture for it, if you put one there
  Shows/
    A Show [show-22]/
      show.json                                  the show
      poster.jpg                                 a picture for the show, if you put one there
      S01E01 Pilot [item-311].json               one episode
```

- Each library has a folder with its name. Two libraries whose names a file system can't tell apart get the library's
  number as well.
- The number in square brackets is how the server knows which title a file is for. Everything else in the name is
  only there to help you find it, so you can rename a file as long as the brackets stay.
- A file stays where it is if you rename the title, so its name may be out of date. It does no harm.
- Films, shows, episodes, discs, home videos, music videos, books and games get files. Music, audiobooks, podcasts,
  photos and radio don't, because these fields don't describe them.

A file looks like this:

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

Leave the first four lines alone. They say what the file is for, and the server uses them to catch mix-ups.

The rules are few. Text goes in `"quotes"`. A number has none. A list goes in `[square brackets]`. Every line but the
last has a comma at its end.

## Examples

### A wrong title and year

```json
  "title": "Harbour Ligts",
  "sortTitle": "Harbour Ligts",
  "year": 1998,
```

becomes

```json
  "title": "Harbour Lights",
  "sortTitle": "Harbour Lights",
  "year": 2004,
```

### A wrong poster

Either put a picture beside the file, or give a web address.

**A picture beside the file.** If the file is `My Film (2001) [item-123].json`, call the picture
`My Film (2001) [item-123]-poster.jpg`. Use `-backdrop`, `-banner` or `-logo` for those. Beside a `show.json`, it's just `poster.jpg`,
`backdrop.jpg`, `banner.jpg` or `logo.jpg`. A picture there comes before a web address in the file. When you apply it, it's
copied into the server's own image cache.

**A web address**, in the file:

```json
  "poster": "https://example.com/my-poster.jpg",
```

Or the plain name of a picture in the same folder, whatever it's called:

```json
  "poster": "cover.png",
```

Pictures have to be JPEG, PNG, WebP or GIF, up to 20 MB. If a title has a picture only the server can find, such as a
frame from the video, the file has no `poster` line, and you can add one.

### Wrong genres

```json
  "genres": ["Drama", "Mystery"],
```

To clear something, empty its quotes or brackets: `"tagline": ""` or `"genres": []`. To leave something as it is,
you can delete its line, and a file can be cut down to just the one thing it changes. A title can't be emptied.

### A summary with several lines

A line break in text is written `\n`, and a quote inside text is written `\"`:

```json
  "overview": "First paragraph.\n\nThe \"second\" paragraph.",
```

### The wrong match

To say which entry on a service a title is, give both lines. The services are `tmdb`, `tvdb`, `tvmaze`, `anilist`, `wikidata`
and `imdb`:

```json
  "provider": "tmdb",
  "providerId": "12345",
```

Nothing is looked up when you apply this. For choosing from a list of results, with a preview, use
[Fix matches](METADATA.md#fix-matches-the-review-list). Empty both lines to remove a match.

### Letting a field follow its source again

`"locked"` lists the fields no scan or lookup may change. Anything you change in a file locks itself. To unlock something,
take it out of the list.

```json
  "locked": ["title", "year"]
```

## When it says something is wrong

Every message names the file and the line, like this:

```
Movies/Quiet Pier (2011) [item-2].json:9: A comma is missing at the end of the line before this one
```

That means line 9 of that file. Look at that line and the one before it. A file with a problem is left out, and the
others still apply.

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
| **It isn't a picture, whatever its name says** | The file isn't really a JPEG, PNG, WebP or GIF. Convert it, or use another |
| **This file was written for one thing, but this number is now another** | The number means another title now, perhaps after a restore. Delete the file and export again |
| **There's no title with the number…** | That title has gone from the server. Delete the file |
| **This file and another describe the same title** | Delete one of them |
| **This file is saved as UTF-16** | Save it again as UTF-8. In Notepad it's in the *Save As* window, beside *Encoding* |
| **HTML or hidden characters were taken out** | Only a note. They were removed, and the rest was used. See [below](#how-its-kept-safe) |

**Starting a file over:** delete it, and press **Export again**.

**Starting the whole folder over:** delete it and export again. Your titles are in the server's database, and these files are
only a copy to edit. Anything you've applied stays.

## Taking it back

Right after applying, press **Undo all of it**. Later, open *Fix matches → History*. Everything from one apply is one
entry there, and it goes back as one. The last 5,000 changes are kept.

After undoing, press **Export again** to bring the files back into line.

Some things worth knowing:

- **A file you haven't touched changes nothing.** The server remembers what it last wrote or applied, in a hidden file called
  `.exported.json`. So if you change a title in the app after exporting, its old file can't change it back.
- **An export never writes over your work.** A file you've edited and not applied stays as it is, and the export says how many it left.
- **A file you've changed says everything in it.** If you edit a file exported a while ago, everything in it that differs from the server
  is listed as a change. Read the list: it's why it's there. Or delete the file and export again before you edit.

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

A field that kind of title doesn't have, such as `network` on a film, is a note, and it's left out.

## How it's kept safe

This folder can hold anything, including something you were sent or downloaded. So nothing in it is believed:

- **Text is only text.** HTML, scripts, comments, control characters and tricks that reverse how text reads are taken out, and you're told.
  The web app also shows these fields as text, never as markup.
- **A web address has to be one.** Only `http` and `https`, with no login in it. Not `javascript:`, `data:` or `file:`.
- **A picture has to be one.** It's judged by what's in the file and not its name. SVG isn't accepted, because it can hold scripts.
- **Nothing leads out of the folder.** A file name can't hold a folder or `..`. Links, shortcuts and junctions in the folder aren't followed, and
  nothing is written through them.
- **Sizes are limited.** 256 KB for a file, 1 MB for an NFO, 20 MB for a picture, 500,000 files in all.
- **NFOs can't declare anything.** A `DOCTYPE` or `ENTITY` is refused, because that's how an XML file can be made to read others.
- **Only an administrator can apply.** It goes through the same checks as the edit dialog, and every change is recorded in History.

The same cleaning is done to anything read from beside your media: `.nfo`, `.metadata.json`, `.info.json`, `.wittycd.json`, and the details inside
a book or comic. A problem there is in *Settings → WittyLog*, in the `library` category, with the file and the line.

You can also put a hand-written `.nfo` in this folder, if its name has the number in square brackets. It's read in the same way, and its
pictures have to be web addresses.

## On the command line

These work on any system, with the server stopped, for instance if it won't start. Add `-data <folder>` if yours isn't the default.

```
wms metadata export                write the files. -library <number> for only one library
wms metadata check                 what's wrong with them, by file and line. It needs no database
wms metadata apply                 what they would change. Nothing changes
wms metadata apply --apply         change it
wms metadata undo <batch>          take it back. Apply tells you the batch
```

`check` and `apply` end with a non-zero exit code if a file has a problem, so they can be used in a script. If a server is running on the same
folder, `apply --apply` refuses and points you to Settings, because the server is the one holding those files.

---
<sub>WittyMediaServer documentation · [Back to the README](../README.md)</sub>
