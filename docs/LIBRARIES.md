# Libraries

This guide helps you add your media, lay out and name your files so they're recognised, and decide who sees what. Libraries are managed under **Settings → Libraries**, by an administrator.

- [Add a library](#add-a-library)
- [Kinds of library](#kinds-of-library)
- [How to lay out and name your files](#how-to-lay-out-and-name-your-files)
- [Scanning](#scanning)
- [What happens to new, changed and deleted files](#what-happens-to-new-changed-and-deleted-files)
- [Library settings](#library-settings)
- [Read-only libraries](#read-only-libraries)
- [Network shares](#network-shares)
- [Hiding things, and who sees what](#hiding-things-and-who-sees-what)

## Add a library

A library is one or more folders of one kind of media. Your files stay where they are, and nothing is copied.

1. Go to **Settings → Libraries → Add library**.
2. Choose the kind of media. If a kind needs a module this server doesn't have, it's listed under **Add a module to use these**, with a button to add it. Adding a module downloads it.
3. If you already have a library of that kind, choose whether these folders go in it or make a separate one with its own name, such as "Kids' movies" beside "Movies".
4. Choose the folder:
   - On the computer running the server, **Choose folder…** opens your computer's own folder window.
   - **Browse folders** on that computer, or **Browse folder…** on any other device, shows the server's drives in the page. Press **Use this folder**, or tick several.
   - Or type the folder's full path.
5. Press the button that adds it, such as **Add Movies**.

Scanning starts right away. The folder has to be one the server can read, which isn't always the same as one you can read. If you're told *folder not found*, see [Troubleshooting](TROUBLESHOOTING.md#libraries-and-scanning).

**Folders of the same kind**, near the top of the Libraries page, sets which choice comes first in step 3: **Combine into one library** or **Keep separate, with own names**. You can always choose differently when adding.

## Kinds of library

| Kind | What goes in it | Folder |
|---|---|---|
| **Movies** | Film files. Disc images of films can go here too | Needed |
| **TV shows** | One folder per show, with season folders inside. Disc images of a series too | Needed |
| **DVDs, Blu-rays & CDs** | Only disc images and disc folders, sorted as films, series or audio CDs | Needed |
| **Anime** | Series with information from AniList. Also collects anime found in your TV shows | Needed |
| **Home videos** | Your own recordings. Nothing is looked up online | Needed |
| **Music videos & concerts** | Music videos and live recordings, by artist | Needed |
| **Creator videos** | An archive of creators' videos, by channel | Needed |
| **Vertical videos** | Short videos in a feed, with a For You page worked out on this server | Needed |
| **Music** | Albums by artist. Tags and covers are read from the files | Needed |
| **Audiobooks** | One folder per book | Needed |
| **Podcasts** | Shows you follow by feed, plus episode files you have | Optional, for downloads |
| **Internet radio** | Stations from a free directory, or by address | None |
| **Books & comics** | EPUB, PDF, comics and manga | Needed |
| **Photos** | Pictures and clips, RAW files too, with a timeline, albums and a map | Needed |
| **Games** | Your own game files, by console | Needed |
| **Commercials** | Your own ads and bumpers for TV channels | Needed |

An administrator can give a kind brought by a module a name of this server's own, with **Rename** under **Settings → Modules**. [What each media type supports](MEDIA-TYPES.md) goes into what each kind can do.

## How to lay out and name your files

Your files are never renamed or moved for you. These layouts get the best results. The file types each kind accepts are in [Supported formats](SUPPORTED-FORMATS.md).

**In every kind that has files,** folders whose names start with a dot are skipped.

**In film and TV libraries,** so are folders named `extras`, video files whose names start with `sample`, and video files under 5 MB. To keep a trailer or featurette with its film, use **It's a bonus feature** in its **Edit** dialog: see [Metadata](METADATA.md).

### Movies

Put the year in brackets after the title. Each film can have its own folder, or they can share one:

```
Movies/
  The Long Harbour (2001)/The Long Harbour (2001).mkv
  Paper Lanterns (2014).mp4
```

If a film keeps matching the wrong title, put its id in the name, and it wins over any search: `The Long Harbour (2001) [tmdbid-123].mkv`. Other forms work too, such as `{imdb-tt0000123}`. A second copy of a film, such as a DVD and a 4K file, can be joined to it with **This is another version of…** in its **Edit** dialog.

### TV shows

One folder per show. Season folders are optional, and these all work: `S02E01`, `s2.e1`, `2x01`, several episodes in a file (`S01E01E02`, `S01E01-E02`), and `05 - Title.mkv` inside a season folder.

```
TV Shows/
  Night Ferry/
    tvshow.nfo, poster.jpg, fanart.jpg        (optional)
    Season 02/
      Night Ferry - S02E01 - The Crossing.mkv
      Night Ferry - S02E15-E16 - Two Parter.mkv
    Specials/
```

A file that can't be numbered still appears under its show, as *Other videos*. More in [TV shows](TV.md).

### Anime

Lay it out as TV shows. Absolute episode numbers, as many releases use, are understood. Anime can live in an Anime library, in TV libraries, or both: **Settings → Metadata agents → Where anime from TV libraries appears** decides where it shows.

### Disc images

Use `.iso` or `.img` files, or folders holding `VIDEO_TS` or `BDMV`. They can go in Movies or TV shows, or in a **DVDs, Blu-rays & CDs** library of their own. You can mark each as a film or a series on its page.

Playing a disc takes a disc module. Encrypted discs are detected and refused. See [DVDs](DVD.md) and [Blu-rays](BLURAY.md).

### Home videos

Any layout. Nothing is looked up, and small files are kept, however short.

### Music

Any layout works if the files are tagged. Without tags, folders are used:

```
Music/
  Some Artist/An Album/01 First Song.mp3
  Some Artist - An Album/01 First Song.mp3
```

Covers and CD images are covered in [Music](MUSIC.md), and lyrics in [Lyrics and karaoke](LYRICS-KARAOKE.md).

### Audiobooks

Author, then book. Each file is a chapter, or one file can be a whole book:

```
Audiobooks/
  Jane Author/A Short Book/01 Chapter 1.m4b
  Jane Author/Another Book.m4b
```

### Podcasts

Open the library and press **Follow a podcast**, to search or paste a feed address. A folder is only needed for episodes you already have and episodes you keep. New episodes are looked for every 2 hours. See [Music, audiobooks and podcasts](MUSIC.md#podcasts).

### Internet radio

There are no files. Open the library and press **Add stations**. See [Internet radio](COLLECTIONS.md#internet-radio).

### Books and comics

Any layout. A comic named like `Harbour Tales 145 (2023).cbz` or `Harbour Tales v03.cbz` is grouped into its series by number. If a folder holds one book in two formats under the same name, it's listed once. See [Books, PDFs and comics](BOOKS.md).

### Photos

Any layout, and everything below the folder is picked up. Your files are never changed. Uploads are saved in an `Uploads` folder inside the library's first folder. See [Photos](PHOTOS.md).

### Vertical videos and creator videos

One folder per creator or channel. In creator videos, subfolders become playlists. Sidecar files beside a video can add its title, date and more. Nothing is downloaded or looked up. See [Vertical videos](SHORTS.md) and [Creator videos](CREATOR-VIDEOS.md).

### Music videos and concerts

Name files `Artist - Title.mp4`, or keep a folder per artist. Recordings of 20 minutes or more appear under **Concerts**, with their chapters as the setlist. See [Creator videos and music videos](CREATOR-VIDEOS.md).

### Games

Choose the console when you add the library, or **Many consoles in one folder**, which sorts by folder and file type. Nothing is bundled or downloaded, so these are your own files.

```
Games/
  Super Console/
    A Made-up Game.zip
    Two Disc Game.m3u
    BIOS/
```

- Zipped games work, and a `.cue` or `.m3u` keeps a multi-file game together.
- Some consoles need a BIOS file from a console you own. Put them in a folder named `BIOS` inside that console's games folder. It's never listed as a game.
- Each console needs a core, the program that plays its games. Install one under **Settings → Consoles & cores**.
- Encrypted game files are detected and refused.
- Anything the scan can't be sure of is listed under **Needs a look** on the library's page.

See [Games](GAMES.md).

### Commercials

Any layout, and short files are kept. These stay out of Home and search, and play on the [TV channels](CHANNELS.md) you choose them for.

## Scanning

A scan looks through a library's folders for new, changed and missing files, then looks up what it found. A scan runs:

- when you add a library or save changes to one;
- every 6 hours for every library, while the server is running;
- when you press **Scan now** under **Settings → Libraries**, or **Scan** on a library's page;
- when a module that a library was waiting for arrives.

The server doesn't watch your folders for changes as they happen, so a file you add waits for the next scan. Press **Scan now** if you don't want to wait. Photos and vertical videos uploaded through the app appear at once.

One library scans at a time and the others wait their turn, to go easy on a small machine. Files it has already seen and that haven't changed take almost no time.

The activity button at the top of the screen shows each scan's progress and any folder it had trouble with, and can pause or stop it. A scan you stop picks up where it left off next time. See [Background tasks](TASKS.md).

## What happens to new, changed and deleted files

| What you did | What happens at the next scan |
|---|---|
| Added a file | It's added and looked up |
| Replaced or changed a file | It's read again |
| Deleted a file | It leaves the lists |
| Put a deleted file back in the same place | It returns, with its progress |
| Moved or renamed a file | The old one leaves the lists and the new one is added as a new title |
| A whole folder is empty or can't be reached | Nothing is removed, and the activity button says so |

That last row is on purpose. A drive that's unplugged or a share that's offline looks like an empty folder, and that's not the same as you having deleted everything.

**Moved a whole drive?** Don't scan and hope. Use **Settings → Repair → Moved a drive?**, which keeps watch progress, matches and your edits. See [Repair](REPAIR.md#moved-a-drive).

Unless you've turned it off, the server saves NFO files and artwork beside your media: see [What's saved next to your media](METADATA.md#what-is-saved-next-to-your-media). To stop that everywhere, turn off **Settings → Metadata agents → Save information and artwork next to your media**. For one library, make it [read-only](#read-only-libraries).

## Library settings

Each library has a card under **Settings → Libraries**:

- **Scan now** looks through its folders again.
- **Edit** changes its name, its kind and its folders, and for games, its console. **Save and scan** starts a scan.
- The bin removes the library. Your files stay where they are, and the server forgets them.

For a **Commercials** library, the card also has **Show in the main menus and on Home**.

Converting files ahead of time is set per library under **Settings → Library transcoding**. See [Library transcoding](LIBRARY-TRANSCODING.md).

## Read-only libraries

Under **Settings → Library transcoding**, choose the library and turn on **This library is read-only**. The server then won't convert its files or save artwork and details beside them. Previews and dry runs still work.

It doesn't stop what people ask for directly: uploads to a Photos or Vertical videos library, which you can turn off under **Settings → Sharing & photos**, and podcast episodes kept in its folder. For a guarantee, mount the folder read-only as well, such as `:ro` in Docker.

## Network shares

A library can live on a NAS or another computer. How you add it depends on where you're sitting, because reaching a share makes the server sign in there with its own account, which on Windows is the machine's. A stolen session in a browser shouldn't be able to do that, so a path like `\\nas\media` is refused from another device. The message starts *That path is on another machine*.

You have three ways to add one:

1. **On the server itself.** Open the server's own address, `http://localhost:9797`, on the computer running it, and add the share there.
2. **As a mapped drive or a mount.** Give it a drive letter on Windows, or mount it on Linux, macOS or FreeBSD, then use that path. With Docker, pass it in with `-v`.
3. **By allowing it.** Set `allowNetworkPaths` in the server's config file, then restart. Only do this if you're happy with what's described above:

```json
{ "allowNetworkPaths": true }
```

It can also be set with `-allow-network-paths` or `WMS_ALLOW_NETWORK_PATHS=1`. `wittymediaserver config path` shows where the file is, and [Config file](CONFIG.md) says more. Libraries already on a share go on working whichever you choose.

**As a Windows service,** the server can't see mapped drive letters. Use `\\server\share` on a share that lets the `LOCAL SERVICE` account in, added on the server itself. See [Installing](INSTALL.md#windows).

**Keep the server's own data folder on a drive inside the computer.** Only the media should be on the share, because the database can slow down or be damaged on a network drive. **Settings → WittyDiagnoser → Storage** checks for this and other trouble: see [WittyDiagnoser](DIAGNOSER.md).

## Hiding things, and who sees what

**For one person:** go to **Settings → Profiles**, and press **Limits** beside their name. See [Accounts and profiles](ACCOUNTS.md#limits).

| In Limits | What it does |
|---|---|
| **What may they watch?** | An age limit, from *Little kids* to *Everything* |
| **Also show titles without a rating** | Home videos and unmatched files, which otherwise stay hidden under an age limit |
| **Libraries** | Only the ticked libraries are allowed. Leave all unticked to allow every library |
| **Hide genres** | Titles of these genres are hidden |
| **Exceptions** | **Always allow** or **Never allow** a film or show, whatever its rating |
| **Live TV** | Whether they get it, and which kinds of channel |

What someone can't see is hidden everywhere, in lists, search and counts alike, and asking for it directly gets "not found". Age limits depend on ratings, which can be missing or wrong, so they're a help and not a guarantee.

**For everyone:** there's no switch to hide a library from every person, except that a Commercials library stays out of menus unless you turn on **Show in the main menus and on Home**. To keep a library from most people, limit each of them to the others. **Categories** on a film or TV library's page lets an administrator rename, hide, reorder and merge that library's categories for everyone.

**For yourself:** **Customize home**, at the foot of the Home page, rearranges or hides its sections, and follows your account to every device. See [Metadata](METADATA.md#categories-studios-and-browsing) for shelves, filters and saved views.

---

<sub>WittyMediaServer documentation</sub>
