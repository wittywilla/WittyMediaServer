// Package mediatypes is the one list of the file types, containers and codecs the
// server knows: what each is called, how to recognise it, which libraries take it,
// how it plays and what it needs. Scanning, playback, serving, the file browser and
// the Supported formats page all read this list, so a format added here is known
// everywhere and can't be forgotten in one of them.
//
// It is plain data and pure functions. Sniff never reads more than it is given, and
// SniffFile never reads more than MaxSniff, so a hostile file costs next to nothing.
// GET /api/media-types serves the same data as JSON for modules and apps.
package mediatypes

import (
	"path/filepath"
	"slices"
	"strings"
)

// Categories of file.
const (
	Video    = "video"
	Audio    = "audio"
	Subtitle = "subtitle"
	Photo    = "photo"
	Book     = "book"
	Comic    = "comic"
	Disc     = "disc"
	Playlist = "playlist"
	Sidecar  = "sidecar"
	// Other is known content that is not media: archives, programs, web pages.
	// Recognising it lets a scan leave out a "film" that is really a program.
	Other = "other"
)

// How a format reaches the viewer.
const (
	PlayNative   = "native"   // a browser plays it as it is, when the codecs inside allow
	PlayRewrap   = "rewrap"   // the picture is passed on in a new wrapper, or converted if it has to be
	PlayConvert  = "convert"  // always converted on the way to the player
	PlayImage    = "image"    // shown as it is
	PlayThumb    = "picture"  // shown as a JPEG the server makes
	PlayReader   = "reader"   // opens in the built-in reader
	PlayDownload = "download" // listed and kept; opening it needs another program
	PlayDiscMenu = "disc"     // plays with its menus through the disc player
	PlayTrack    = "track"    // a subtitle track shown over the picture, or burned in
	PlayBurn     = "burn"     // a picture-based subtitle, burned into the picture
	PlaySheet    = "sheet"    // read by the scan: gives titles, tracks or artwork to something else
)

// Format is one file type.
type Format struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Exts     []string `json:"extensions"`
	MIME     string   `json:"mime"`
	// FFmpeg lists the demuxer names ffprobe reports for this format.
	FFmpeg []string `json:"ffmpegFormats,omitempty"`
	// Confirm: the extension alone proves nothing (.ts is also TypeScript, .bin is
	// anything), so the scan looks at the content before believing it.
	Confirm bool `json:"confirmByContent,omitempty"`
	// SniffAs: what the content of such a file looks like to Sniff when that isn't
	// this format's own id: a Nikon RAW file is a TIFF to anything that only reads
	// its first bytes.
	SniffAs []string `json:"sniffAs,omitempty"`
	// Folder: a folder laid out in a known way, not a file.
	Folder bool   `json:"folder,omitempty"`
	Plays  string `json:"plays"`
	// Libraries: the kinds of library that take it.
	Libraries []string `json:"libraries"`
	// Needs says what has to be present for it to work, in words for people.
	Needs string `json:"needs"`
	// Demuxer is the FFmpeg input it depends on when not every build has it.
	Demuxer string `json:"demuxer,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

// Codec is one way of storing picture, sound or subtitles inside a container.
type Codec struct {
	ID    string `json:"id"` // FFmpeg's name
	Name  string `json:"name"`
	Kind  string `json:"kind"` // video | audio | subtitle
	Notes string `json:"notes,omitempty"`
	// Text: a subtitle that converts to WebVTT (the rest are pictures).
	Text bool `json:"text,omitempty"`
	// Browser: common browsers decode it without help. Clients report what they
	// really can; this is only what a client that says nothing is assumed to do.
	Browser bool `json:"browser,omitempty"`
}

var (
	byID  = map[string]*Format{}
	byExt = map[string][]*Format{}
	codec = map[string]*Codec{}
)

func init() {
	for i := range formats {
		f := &formats[i]
		if _, dup := byID[f.ID]; dup {
			panic("mediatypes: two formats called " + f.ID)
		}
		byID[f.ID] = f
		for _, e := range f.Exts {
			byExt[e] = append(byExt[e], f)
		}
	}
	for i := range codecs {
		codec[codecs[i].ID] = &codecs[i]
	}
}

// Codecs returns every codec. Don't change them.
func Codecs() []Codec { return codecs }

// ByID finds a format by its id.
func ByID(id string) *Format { return byID[id] }

// CodecByID finds a codec by FFmpeg's name for it.
func CodecByID(id string) *Codec { return codec[id] }

// Ext is a file name's extension in the form the registry uses.
func Ext(name string) string { return strings.ToLower(filepath.Ext(name)) }

// ByExt lists the formats that use a file name's extension.
func ByExt(name string) []*Format { return byExt[Ext(name)] }

// ofCategory is the first format of a category that uses this extension.
func ofCategory(name string, cats ...string) *Format {
	for _, f := range byExt[Ext(name)] {
		if slices.Contains(cats, f.Category) {
			return f
		}
	}
	return nil
}

// Find is the format of a category that a file name says it is, or nil.
func Find(name string, cats ...string) *Format { return ofCategory(name, cats...) }

// IsVideo: the name is that of a video file.
func IsVideo(name string) bool { return ofCategory(name, Video) != nil }

// IsAudio: the name is that of a sound file.
func IsAudio(name string) bool { return ofCategory(name, Audio) != nil }

// IsPhoto: the name is that of a picture or camera RAW file.
func IsPhoto(name string) bool { return ofCategory(name, Photo) != nil }

// IsSubtitle: the name is that of a subtitle file.
func IsSubtitle(name string) bool { return ofCategory(name, Subtitle) != nil }

// IsPlaylist: the name is that of a playlist.
func IsPlaylist(name string) bool { return ofCategory(name, Playlist) != nil }

// IsDiscImage: the name is that of a disc image.
func IsDiscImage(name string) bool { return ofCategory(name, Disc) != nil }

// IsWebImage: every current browser shows this kind of picture itself, so the
// original can be sent as it is.
func IsWebImage(name string) bool {
	f := ofCategory(name, Photo)
	return f != nil && f.Plays == PlayImage
}

// Kind says what a file is to the file browser, from its name: video, audio, book,
// disc, subtitle, image, playlist or other.
func Kind(name string) string {
	// The order settles extensions that mean two things.
	switch {
	case ofCategory(name, Video) != nil:
		return Video
	case ofCategory(name, Audio) != nil:
		return Audio
	case ofCategory(name, Book, Comic) != nil:
		return Book
	case ofCategory(name, Disc) != nil:
		return Disc
	case ofCategory(name, Subtitle) != nil:
		return Subtitle
	case ofCategory(name, Photo) != nil:
		return "image"
	case ofCategory(name, Playlist) != nil:
		return Playlist
	}
	return Other
}

// MIME is the Content-Type a file is sent with, from this list only and never from
// what the file says about itself: a "film" that is really a web page stays a
// download. Types a browser would run are never returned.
func MIME(name string) string {
	for _, f := range byExt[Ext(name)] {
		if f.MIME != "" && !scriptable(f.MIME) {
			return f.MIME
		}
	}
	return "application/octet-stream"
}

// MIMEOf is the type of a format, by id.
func MIMEOf(id string) string {
	if f := byID[id]; f != nil && f.MIME != "" && !scriptable(f.MIME) {
		return f.MIME
	}
	return "application/octet-stream"
}

// scriptable types run in the browser when opened on their own. They are served
// only by handlers that sanitise them first and say so themselves.
func scriptable(m string) bool {
	return m == "image/svg+xml" || m == "text/html" || m == "application/xhtml+xml"
}

// IsTextSubtitle: this subtitle codec is text and converts to WebVTT.
func IsTextSubtitle(id string) bool {
	c := codec[id]
	return c != nil && c.Kind == Subtitle && c.Text
}

// Canonical says what a file really is from what ffprobe reported about it: a name
// the playback decision can compare with what a client plays. The extension only
// picks between the names of one demuxer ("matroska,webm" is either), so a file
// whose extension lies is still judged by its contents.
//
// videoCodec and audioCodec settle WebM, whose rules are stricter than Matroska's:
// a ".webm" holding H.264 is a Matroska file to a browser.
func Canonical(formatName, name, videoCodec, audioCodec string) string {
	names := strings.Split(formatName, ",")
	ext := strings.TrimPrefix(Ext(name), ".")
	has := func(want ...string) bool {
		for _, n := range names {
			if slices.Contains(want, n) {
				return true
			}
		}
		return false
	}
	switch {
	case has("matroska", "webm"):
		if (ext == "webm" || ext == "weba") && webmVideo(videoCodec) && webmAudio(audioCodec) {
			return "webm"
		}
		return "mkv"
	case has("mov", "mp4"):
		// One demuxer reads the whole family; these are the names clients list.
		switch ext {
		case "mov", "qt":
			return "mov"
		case "m4v":
			return "m4v"
		case "m4a", "m4b", "m4r":
			return "m4a"
		}
		return "mp4"
	case has("mpegts"):
		return "ts"
	case has("mpeg", "vob"):
		return "mpg"
	case has("ogg"):
		return "ogg"
	case has("wav") && ext == "rf64":
		return "rf64" // read like WAV; no browser reads it
	}
	for _, n := range names {
		for i := range formats {
			if slices.Contains(formats[i].FFmpeg, n) {
				return formats[i].ID
			}
		}
	}
	if len(names) > 0 && names[0] != "" {
		return names[0]
	}
	return ext
}

func webmVideo(c string) bool { return c == "" || c == "vp8" || c == "vp9" || c == "av1" }
func webmAudio(c string) bool { return c == "" || c == "opus" || c == "vorbis" }

// AudioPlaysDirect says a browser plays this sound file as it is: both the codec and
// the container around it have to be ones a browser takes, so FLAC in Matroska, which
// nothing opens, is converted though plain FLAC isn't.
func AudioPlaysDirect(container, audioCodec string) bool {
	c := codec[audioCodec]
	if c == nil || c.Kind != Audio || !c.Browser {
		return false
	}
	switch container {
	case "mp3", "mp2", "mpa":
		return audioCodec == "mp3"
	case "aac", "adts":
		return audioCodec == "aac"
	case "flac":
		return audioCodec == "flac"
	case "wav":
		return strings.HasPrefix(audioCodec, "pcm_")
	case "ogg", "oga", "opus":
		return audioCodec == "vorbis" || audioCodec == "opus" || audioCodec == "flac"
	case "webm", "weba":
		return audioCodec == "vorbis" || audioCodec == "opus"
	case "m4a", "mp4", "m4v", "mov", "m4b":
		return audioCodec == "aac" || audioCodec == "mp3" || audioCodec == "flac" || audioCodec == "opus"
	}
	return false
}

// ReaderFor says which reader opens a book of this format: epub (real EPUB, and what
// the server converts to it), cbz (page pictures) or pdf. "" means none does.
func ReaderFor(id string) string {
	switch id {
	case "pdf":
		return "pdf"
	case "cbz", "cbr", "cb7", "cbt", "djvu":
		return "cbz"
	case "epub", "mobi", "azw", "azw3", "pdb", "fb2", "docx", "odt", "doc", "rtf", "md", "html", "txt":
		return "epub"
	}
	return ""
}

// Confirms says a file's content doesn't contradict a format that has to be confirmed
// (Format.Confirm). Content that's recognised as something of the same category
// agrees. Content nothing recognises is judged by whether it's text: TypeScript named
// .ts is text, a transport stream isn't.
func Confirms(f *Format, head []byte) bool {
	content := Sniff(head)
	if content != nil {
		if content.ID == f.ID || slices.Contains(f.SniffAs, content.ID) {
			return true
		}
		return content.Category == f.Category
	}
	switch f.ID {
	case "vcd", "bin", "ty", "toc":
		return false // too many other things use these names
	case "txt", "md", "html", "mpl2", "realtext", "stl":
		return looksLikeText(head)
	case "sub":
		return true
	}
	return !looksLikeText(head)
}

// ArtworkExts are the pictures looked for beside media: poster.jpg, Film-fanart.png and
// so on. A browser shows every one of them, and .tbn is a JPEG by an older name.
var ArtworkExts = []string{".jpg", ".jpeg", ".jfif", ".png", ".webp", ".avif", ".gif", ".bmp", ".tbn"}

// IsArtwork says a name is one of the pictures looked for beside media.
func IsArtwork(name string) bool { return slices.Contains(ArtworkExts, Ext(name)) }
