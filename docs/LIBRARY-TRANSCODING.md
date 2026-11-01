# Library transcoding

Library transcoding converts the files in a library ahead of time, so they play without the server having to convert them
while someone watches. You'll find it in **Settings, Library transcoding**.

It's a different job from [live transcoding](TRANSCODING.md). Live transcoding happens while someone is watching, leaves your
files alone and stops when they do. Library transcoding **rewrites your files**, so it starts switched off.

- [The short version](#the-short-version)
- [The three presets](#the-three-presets)
- [What it does to your files](#what-it-does-to-your-files)
- [How it affects people watching](#how-it-affects-people-watching)
- [Converting new files by themselves](#converting-new-files-by-themselves)
- [Sound tracks](#sound-tracks)
- [Subtitles](#subtitles)
- [Unusual files, decision by decision](#unusual-files-decision-by-decision)
- [Connecting Tdarr](#connecting-tdarr)
- [Tdarr's licence](#tdarrs-licence)
- [Fixing things](#fixing-things)

## The short version

1. Read the warning at the top of the page and turn it on. Until you do, nothing can be changed.
2. Choose a library, and the preset that sounds like what you want.
3. Under **Try it on one file**, choose a film and press **Show what would happen**. Each track is listed with what will become of it.
4. Press **Check this library**. That's a dry run: it looks at every file, changes nothing, and tells you how many files it would
   change, roughly how much room you'd end up with and how long it would take.
5. Press **Start converting**. It shows in the activity panel at the top of every page, where you can pause or cancel it.

## The three presets

| Preset | What you get | The cost |
|---|---|---|
| **Make everything play on any device** | MP4 with H.264 picture and AAC sound, at most 1080p, with no HDR. Every phone, TV, tablet and browser plays it with no help from the server. | Heavy for the processor. Files may grow if they were very efficient to begin with. |
| **Save space** | H.265 picture, usually 30 to 60% smaller. Lossless sound becomes Dolby Digital Plus. HDR stays HDR and every track stays. A file that doesn't come out at least 10% smaller is left as it was. | The slowest choice. Older devices may need the server to convert these files while they play. |
| **Keep the best quality** | Picture and sound aren't touched. Files are tidied into MKV, tracks are in the order you want, and a stereo track is added where there's only surround. | Very little: mostly copying. |

Under each preset, **What this does** says exactly what it does, and the **details** let you change any of it. Whatever you
change, a file that already matches is left alone, and running the same recipe twice does nothing the second time.

Save space needs an FFmpeg with H.265 in it. Release builds for Windows, Linux and macOS have one. If yours doesn't, it uses a
stronger H.264 setting instead, which saves less, and tells you so. HDR files can't fall back on H.264, so those are left alone.

## What it does to your files

Nothing is replaced until the new file has been checked. For each file:

1. It checks there's room, keeping free the amount you chose (10 GB unless you change it).
2. It writes the new file into a hidden folder beside the old one (`.wittymedia-work`), on the same drive.
3. It checks the new file: it opens, it's as long as the old one, the tracks and chapters are all there, and it really does
   decode at the start, middle and end. If not, the new file is thrown away and the old one is untouched.
4. It checks the old file hasn't changed meanwhile, that nobody started playing it, and, if you asked for a saving, that there is one.
5. Only then does it swap them, doing what you chose for the old file:

| Choice | What happens to the old file | Good for |
|---|---|---|
| **Keep the original** (the default) | Moves to a hidden folder beside it, `.wittymedia-originals`. **Put the original back** undoes it. **Delete the original** frees the room. | Being sure |
| **Replace the original** | Deleted for good, once the new file is in place and the library knows. | When you need the room and you've tested your recipe |
| **Write it beside the original** | Stays where it is, in use. The new file is `Film.wms-converted.mp4`, which a scan ignores. **Use the copy** puts it into use. | Comparing them yourself |

The item in your library stays the same item, even if the file goes from `.avi` to `.mp4`. What people have watched, ratings,
your edits and its match all stay. If the server stops at any moment, it tidies up when it starts again: temporary files are
removed, and a swap caught halfway is finished or put back.

**Two more safety switches.** A library set to **read-only** is never changed, by this or by saved artwork and details. And if
**Save next to media** is off in Metadata settings, conversions in a library need one more switch, **Allow conversions in this
library**, and even then nothing but the video file is written.

## How it affects people watching

Converting competes with watching for the same processor and the same drive. People may see stutters, or a film that takes
longer to start, especially if their device needs the server to convert while it plays.

| Setting | What it does | Unless you change it |
|---|---|---|
| Pause while people are streaming | Holds the conversion still, using no processor, while anyone is playing something or a game. It carries on where it was. | On |
| Low priority | Everything else gets the processor first | On |
| Only at certain hours | Holds still outside the hours you choose, which can run past midnight | Off |
| How much of the processor | Threads for each file | Half of them |
| Files at once | | 1 |
| Use hardware encoders | Uses the ones ticked under Live transcoding, for ordinary 8-bit pictures. Far faster, with slightly larger files. HDR and 10-bit pictures always use software, which can be trusted to keep them right. | On |

**Time a 20-second test** really converts a stretch from the middle of the file you chose, and tells you how fast this machine
did it and what the whole film would come to. The figures from **Check this library** are rougher, and say so.

## Converting new files by themselves

Each library can have **Convert new files by themselves** turned on. It's off unless you turn it on, and it needs the recipe saved and library transcoding switched on.

- **What counts as new.** Files a scan finds after you turned it on, and files that change after that. What's already in the library is left alone: **Start converting** is how to do those.
- **When it starts.** After a scan finds something, it waits until each file has stopped changing, 2 minutes unless you change it, so that a file still being copied or downloaded is left to finish. Then it starts an ordinary conversion of those files.
- **Its limits.** All the same ones: the hours, how much of the processor, files at once, low priority and pausing while people are streaming. Outside its hours or while people are watching, it doesn't start.
- **Where to see it.** In the activity panel, first as **New files in** and the library's name, saying what it's waiting for, then as **Converting new files in** and the library's name, with Pause and Cancel. Files with nothing to do leave no trace.
- **What it won't do again.** Each file's outcome is remembered against its size and date and the recipe. A file that fails, comes out no smaller, already matches, is put back, or whose run you cancelled, isn't tried again unless the file or the recipe changes. **Try again** in the activity panel, or **Start converting**, still does it when you ask.
- **What it does try again.** A file someone was playing and a file that didn't fit on the drive are looked at after the next scan, because neither says anything against the file. A file that changed while it was being converted is looked at again too. If the server stopped partway, what was left is picked up when it starts again.

## Sound tracks

Every track is either kept as it is, converted, or removed. The preview says which, and why.

- **Languages.** Keep them all, or a list, in the order you want. **Also keep the original language** keeps what the film was made
  in: what the file marks as original, otherwise its default track, otherwise its first.
- **Plays first.** The language chosen when a viewer hasn't said what they want.
- **No language.** Keep or remove them, or label them with a language if you know what they are.
- **Commentary and audio description** are recognised by the file's flags and by words in their titles, and can be kept or removed.
  They go after the plain tracks of their language.
- **Stereo beside surround.** A language with only surround gets a stereo AAC track, for phones, headphones and TVs without a sound system.
- **Lossless sound** (TrueHD, DTS-HD MA, FLAC, PCM) can stay or be converted. **Object sound** (Atmos, DTS:X) survives only by staying
  as it is, so converting says plainly that it will be lost.
- **The last track.** A film never ends up silent: if your choices would remove every track, one stays and the preview says so.

## Subtitles

- **Languages.** Keep them all, a list in order, or none.
- **Forced** (only the lines in another language): keep as a track, remove, or burn into the picture. Burning means converting the
  picture, and it can't be turned off afterwards.
- **SDH** (with sound descriptions) and **untagged**: keep or remove.
- **Stored as pictures** (Blu-ray PGS, DVD VobSub, DVB): they can't become text here, which would need character recognition.
  An MP4 can't hold them. So you choose: keep them, with the file staying MKV; save Blu-ray ones as a `.sup` file beside the video;
  or remove them. Anything that would be lost is listed in the preview.
- **Text** stays as it is, or becomes plain SRT. An MP4 holds only its own kind, so going to MP4 loses ASS styling, and the preview
  says so. Going to MKV turns MP4 text into SRT.
- **Files beside the video** (`Film.en.srt`) keep working if left where they are, because the name stays the same. They can instead be
  moved inside the new file.

## Unusual files, decision by decision

| The file | What happens |
|---|---|
| Already matches | Skipped, whichever recipe it is. Running a recipe again finds nothing to do. |
| Several picture streams | Kept and each converted if needed, or just the main one. The main one is a later stream marked default, otherwise the largest, and it goes first. |
| Cover pictures | Always kept, as they are, in MKV and MP4. An MP4 only holds JPEG and PNG: anything else is listed as lost. |
| Chapters and metadata | Kept. A new file that lost its chapters is thrown away. |
| Attached fonts | Kept in MKV. An MP4 can't hold them, and the preview says so. |
| HDR10, HLG | Your choice: stay HDR (H.265 or AV1 only, with a software encoder, keeping the mastering information), become an ordinary picture, or leave those files alone. |
| HDR10+ | Kept if the picture isn't converted. Converted, it stays HDR10, and the preview says the scene-by-scene information is lost. |
| Dolby Vision | Kept only if the picture isn't touched. If it would have to be, your choice: leave the file alone, or convert from its ordinary layer, losing Dolby Vision. Profile 5 has no ordinary layer, so it's always left alone rather than come out with the wrong colours. |
| 10-bit | Stays 10-bit in H.265 and AV1. "Any device" makes it 8-bit. |
| Interlaced | Deinterlaced if it's being converted and you asked. |
| Variable frame rate | Stays as it is, so sound stays in step. "Any device" makes it constant. |
| 3D | Keep both eyes as stored, keep one eye, or leave those files alone. The kind that stores its second eye separately (MVC) can't survive conversion, so it's left alone unless you choose one eye. |
| Broken timestamps | New ones are made. If the file's stated length is wrong, what's really in it decides. If it can't be made right, the old file stays. |
| A split film (`CD1`, `part2`) | Its parts stay separate files, are converted together with the same settings, and swap together. If one part fails, none is replaced. If one part fits and another doesn't, both are converted, so they still join. |
| Fails partway | The temporary file is removed, the old file is untouched, and it's listed as a problem you can retry. |
| Sound-only | Left alone, since these recipes are about pictures. |
| Changed during the job | The new version is thrown away, and it's looked at again next time. |
| Being played | Left until the end of the run. If it's still being played then, it's left for next time. If someone starts playing it just as the new file is ready, the swap waits for them. |
| Disc images and folders | Not converted: they'd lose their menus. They play as they are. |
| Copy-protected or encrypted | Refused, and never decrypted. |
| A link leading out of the library | Refused. Nothing outside a library's folders is ever touched. |

## Connecting Tdarr

Tdarr is a separate program, made by other people, that does this same kind of work across several computers. If you already use it,
or want spare computers to do the work, this server can watch it and hand it files. Everything above works without it.

1. Install Tdarr from [tdarr.io](https://tdarr.io) on any machine, and agree to its terms. It's not included here.
2. In Tdarr, make an API key under **Tools, API Keys**.
3. Here, under **Connect your own Tdarr**, enter its address with its server port (`http://192.168.1.20:8266`) and the key.
   Press **Test the connection**. You'll see its version, its nodes and what they're working on.
4. Turn on **Use this connection**. What Tdarr is doing now shows in the activity panel. Nothing contacts Tdarr while this is off.
5. To hand it a library, choose which of Tdarr's libraries it goes to, and press **Ask Tdarr to look through this library**. If the
   two machines call the folder by different names, fill those in too.

**About nodes.** A Tdarr node connects to a Tdarr server, and how they talk isn't documented for anything else to use, so nodes can't
connect directly to this server. Attach them to your Tdarr server as usual, and connect that here.

**Who decides what.** Tdarr's own plugins and flows decide what happens to a file, not the recipes on this page. Tdarr's safety rules are
its own, too: nothing in [What it does to your files](#what-it-does-to-your-files) applies. If it replaces a file under the same name,
the next scan picks up the new details and the item stays the same. If it changes the extension, this server sees a new file, and what's been
watched doesn't carry over to it.

To check this against your Tdarr, developers can run `WMS_TDARR_URL=http://... WMS_TDARR_KEY=... go test ./internal/libtranscode/tdarr -run TestLive -v`.

## Tdarr's licence

Tdarr's server and node aren't open source. They come under an [end user licence agreement](https://github.com/HaveAGitGat/Tdarr/blob/master/LICENSE.md)
with free and paid tiers, which forbids redistributing or reverse engineering them, and whoever installs Tdarr agrees to it. Its source
isn't published, so there's nothing current to fork. Its [plugins](https://github.com/HaveAGitGat/Tdarr_Plugins) are GPL-3.0.

So WittyMediaServer's own engine is written from scratch on FFmpeg, and is complete by itself. The connection uses only Tdarr's documented
HTTP API. No Tdarr code or plugin is included, it isn't downloaded for you, and Tdarr's name is used only to say what it connects to.

## Fixing things

| What you see | What to do |
|---|---|
| **Start converting** is greyed out | The line under it says why: turned off, read-only, unsaved changes, or Save next to media. |
| A file failed | Its old file is untouched. Open it in the list to see why. **Try again** in the activity panel's history retries what's left. WittyLog has FFmpeg's own words. |
| "Not enough free space" | Free some room, delete kept originals, or choose to replace originals. |
| It says it's waiting | People are watching, or it's outside its hours. The activity panel says which. |
| A folder called `.wittymedia-originals` | Your originals. Delete them from this page, or by hand once you're happy. If you delete them by hand, **Put the original back** will say they've gone. |
| A folder called `.wittymedia-work` | Temporary files. It removes them when it starts up, and it's safe to delete if the server isn't converting anything. |
| You want to go back to a file by hand | Move it out of `.wittymedia-originals` to where it was, delete the converted one, and scan the library. |
