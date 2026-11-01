package mediatypes

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Registry is the whole list, as served by GET /api/media-types and written to
// docs/media-types.json for modules that want it when they're built.
type Registry struct {
	Version int      `json:"version"`
	Formats []Format `json:"formats"`
	Codecs  []Codec  `json:"codecs"`
}

// All returns the registry. Bump the version when a field changes meaning.
func All() Registry { return Registry{Version: 1, Formats: formats, Codecs: codecs} }

// JSON is the registry as a file: stable, so that a generated copy can be compared.
func JSON() []byte {
	b, err := json.MarshalIndent(All(), "", "  ")
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}

var categoryOrder = []struct{ id, title, intro string }{
	{Video, "Video", "A file plays directly when your player can open its container and decode what's inside. If only the container is wrong, the server rewraps it without touching the picture. If only the sound is wrong, it converts the sound alone. The picture is only converted when nothing simpler would play."},
	{Audio, "Music and audio", "A browser plays the formats marked \"As it is\". Everything else is converted to AAC as it plays, for all your devices, with the original left alone."},
	{Subtitle, "Subtitles", "Keep subtitle files beside a video with the same name, such as `Film.en.srt` or `Film.en.forced.srt`, or in a `Subs` folder. Text is shown by your player. Pictures from discs are drawn into the video by the server."},
	{Photo, "Photos", "Pictures a browser can't show are sent as a JPEG the server makes, and the original stays untouched. Camera RAW files show the picture the camera stored inside them, so nothing has to be developed."},
	{Book, "Books", "Everything here opens in the built-in reader. Formats other than EPUB and PDF are converted to EPUB once, on the server, the first time they're needed, and the original is left alone."},
	{Comic, "Comics", "Pages can be in any common picture format. The archive is recognised by its contents, so a RAR file named .cbz still opens."},
	{Disc, "Discs", "Disc images and folders play with their menus. Encrypted discs are detected and refused with a clear message: nothing here removes copy protection."},
	{Playlist, "Playlists", "Playlists in a music library become collections of the songs they name. Web addresses in them are ignored."},
	{Sidecar, "Files beside your media", "These aren't played. They give titles, details or artwork to what's beside them."},
}

var playsWords = map[string]string{
	PlayNative:   "As it is, when the codecs inside allow",
	PlayRewrap:   "Rewrapped, or converted if it has to be",
	PlayConvert:  "Converted as it plays",
	PlayImage:    "As it is",
	PlayThumb:    "As a JPEG",
	PlayReader:   "In the reader",
	PlayDownload: "Download",
	PlayDiscMenu: "With its menus",
	PlayTrack:    "As text",
	PlayBurn:     "Drawn into the picture",
	PlaySheet:    "Read by the scan",
}

// PlaysWords puts how a format plays in words.
func PlaysWords(plays string) string { return playsWords[plays] }

func extList(f Format) string {
	if f.Folder {
		return "a folder"
	}
	return strings.Join(f.Exts, ", ")
}

// Markdown is the Supported formats page in docs/, made from the registry so that it
// can't say anything the server doesn't do. A test fails if the file differs.
func Markdown() string {
	var b strings.Builder
	b.WriteString(`# Supported formats

This page is made from the server's own list of formats (` + "`internal/mediatypes`" + `), so it always matches what
the server does. The same list is in the app, under **Settings, About, Supported formats**, where it also says what
your own FFmpeg can do, and at ` + "`GET /api/media-types`" + ` for modules and apps.

**A file's content decides, not its name.** A file with a missing or wrong extension is recognised from its first
bytes. A name that other things also use, such as ` + "`.ts`" + `, is checked the same way. A file whose content clearly isn't
media, like a program named ` + "`film.mkv`" + `, is left out of the library and the log says why.

**FFmpeg does the decoding.** Patent-encumbered codecs such as H.264, HEVC and AAC are handled by the FFmpeg on your
machine, as with any program that uses it. Which ones you can use depends on your FFmpeg and where you live.

`)
	for _, cat := range categoryOrder {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", cat.title, cat.intro)
		b.WriteString("| Format | Extensions | How it plays | Needs |\n|---|---|---|---|\n")
		for _, f := range formats {
			if f.Category != cat.id {
				continue
			}
			name := f.Name
			if f.Notes != "" {
				name += " (" + strings.TrimSuffix(f.Notes, ".") + ")"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", name, extList(f), PlaysWords(f.Plays), f.Needs)
		}
		b.WriteString("\n")
		if cat.id == Video {
			b.WriteString(codecTable(Video))
		}
		if cat.id == Audio {
			b.WriteString(codecTable(Audio))
		}
		if cat.id == Subtitle {
			b.WriteString(codecTable(Subtitle))
		}
	}
	b.WriteString(`## Special cases

- **HDR10, HDR10+, HLG and Dolby Vision** play as they are on a player and screen that show them. Anything else is
  converted to fit an ordinary screen. HDR10+ plays its HDR10 layer unless the player handles more. Dolby Vision
  profile 5 has no plain HDR layer, so its colours may look a little off once converted.
- **10-bit and 12-bit video** plays directly where the player decodes it, and is converted elsewhere.
- **Interlaced video** is deinterlaced as it's converted.
- **Variable frame rates and damaged timestamps** play: the stream is rebuilt as it's converted.
- **3D films** in a frame (side by side, or one above the other) play as they are. The player's information offers to
  show them in 2D.
- **Files with several video streams** play the first, and the player's information offers the others.
- **Cover art inside a video file** is used as artwork and is never mistaken for the picture.
- **Films split across files** (` + "`Film CD1.avi`" + `, ` + "`Film CD2.avi`" + `, or ` + "`Film.part1.mkv`" + `) are one title that plays straight through.
- **CUE sheets** split one long sound file into the tracks of an album.
- **Live photos and motion photos** play their clip, either beside the picture or inside its file.
- **SVG pictures** lose anything active before they are sent.

---

<sub>WittyMediaServer documentation</sub>
`)
	return b.String()
}

func codecTable(kind string) string {
	var b strings.Builder
	var names []string
	for _, c := range codecs {
		if c.Kind == kind && !slices.Contains(names, c.Name) {
			names = append(names, c.Name)
		}
	}
	word := map[string]string{Video: "Video codecs", Audio: "Audio codecs", Subtitle: "Subtitle tracks inside files"}[kind]
	fmt.Fprintf(&b, "%s the server knows by name: %s.\n\n", word, strings.Join(names, ", "))
	return b.String()
}

// TypeScript is the small part of the registry the web app can't ask for first: names
// for book formats and the file types its upload boxes accept.
func TypeScript() string {
	var b strings.Builder
	b.WriteString("// Made from internal/mediatypes by `go run ./internal/mediatypes/gen`. A test fails if it differs: don't edit it.\n\n")
	b.WriteString("export const BOOK_FORMATS: Record<string, string> = {\n")
	for _, f := range formats {
		if f.Category == Book || f.Category == Comic {
			fmt.Fprintf(&b, "  %q: %q,\n", f.ID, f.Name)
		}
	}
	b.WriteString("}\n\n")
	list := func(name string, cats ...string) {
		fmt.Fprintf(&b, "export const %s = %q\n", name, acceptList(cats...))
	}
	list("ACCEPT_VIDEO", Video)
	list("ACCEPT_PHOTO", Photo)
	list("ACCEPT_GALLERY", Photo, Video)
	return b.String()
}

func acceptList(cats ...string) string {
	var out []string
	for _, f := range formats {
		if !slices.Contains(cats, f.Category) {
			continue
		}
		out = append(out, f.Exts...)
	}
	return strings.Join(out, ",")
}
