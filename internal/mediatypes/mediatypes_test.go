package mediatypes

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRegistryIsWellFormed(t *testing.T) {
	extRe := regexp.MustCompile(`^\.[a-z0-9+_-]+$`)
	for _, f := range formats {
		if f.ID == "" || f.Name == "" || f.Category == "" {
			t.Errorf("%+v: needs an id, a name and a category", f)
		}
		if f.Category != Other {
			if f.Plays == "" || len(f.Libraries) == 0 || f.Needs == "" {
				t.Errorf("%s: needs to say how it plays, where it goes and what it needs", f.ID)
			}
			if PlaysWords(f.Plays) == "" {
				t.Errorf("%s: %q isn't a way of playing", f.ID, f.Plays)
			}
			if len(f.Exts) == 0 && !f.Folder {
				t.Errorf("%s: no extensions", f.ID)
			}
		}
		for _, e := range f.Exts {
			if !extRe.MatchString(e) {
				t.Errorf("%s: %q isn't an extension as this list writes them", f.ID, e)
			}
		}
		for _, other := range f.SniffAs {
			if byID[other] == nil {
				t.Errorf("%s: looks like %q, which isn't in the list", f.ID, other)
			}
		}
		if scriptable(f.MIME) && f.ID != "svg" && f.ID != "html" {
			t.Errorf("%s: a type a browser would run", f.ID)
		}
	}
	for _, f := range formats {
		if f.Category == Other {
			continue
		}
		seen := map[string]string{}
		for _, e := range f.Exts {
			if seen[e] != "" {
				t.Errorf("%s lists %s twice", f.ID, e)
			}
			seen[e] = f.ID
		}
	}
	// One extension may mean two categories, never two formats of one category.
	type pair struct{ ext, cat string }
	owner := map[pair]string{}
	for _, f := range formats {
		for _, e := range f.Exts {
			k := pair{e, f.Category}
			if owner[k] != "" {
				t.Errorf("%s belongs to both %s and %s", e, owner[k], f.ID)
			}
			owner[k] = f.ID
		}
	}
}

// What the brief asks for by name: if one of these disappears, a test says so.
func TestEverythingAskedForIsListed(t *testing.T) {
	for cat, list := range map[string]string{
		Video:    "mp4 m4v mkv webm avi mov qt wmv asf flv f4v mpg mpeg m2v mpe vob ts m2ts mts tp trp ogv ogm 3gp 3g2 rm rmvb divx xvid mxf dv nut y4m gxf wtv dvr-ms amv mjpeg ivf",
		Audio:    "mp3 m4a m4b aac flac alac ogg oga opus wav aiff wma ape wv tta mpc dsf dff ac3 eac3 dts truehd mka mid midi mod xm s3m it nsf spc vgm",
		Subtitle: "srt ass ssa vtt sub idx sup sbv ttml dfxp smi lrc mks",
		Photo:    "jpg png gif webp avif heic heif jxl tiff bmp ico svg cr2 cr3 nef arw dng raf orf rw2",
		Book:     "epub mobi azw azw3 kf8 fb2 pdf djvu txt rtf md html doc docx odt",
		Comic:    "cbz cbr cb7 cbt",
		Disc:     "iso img bin cue nrg mdf",
		Playlist: "m3u m3u8 pls xspf asx",
		Sidecar:  "nfo",
	} {
		for _, e := range strings.Fields(list) {
			if Find("x."+e, cat) == nil {
				t.Errorf(".%s isn't listed as %s", e, cat)
			}
		}
	}
	for _, c := range strings.Fields("h264 hevc av1 vp8 vp9 vvc mpeg2video mpeg4 vc1 prores dnxhd theora rv40") {
		if CodecByID(c) == nil {
			t.Errorf("codec %s isn't listed", c)
		}
	}
}

func TestKinds(t *testing.T) {
	for name, want := range map[string]string{
		"a.mkv": "video", "A.MKV": "video", "a.flac": "audio", "a.cbr": "book", "a.epub": "book", "a.iso": "disc", "a.srt": "subtitle", "a.heic": "image", "a.CR3": "image",
		"a.m3u": "playlist", "a.nfo": "other", "a.xyz": "other", "noext": "other", "a.mod": "video",
	} {
		if got := Kind(name); got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
}

func TestMIMENeverRuns(t *testing.T) {
	for name, want := range map[string]string{
		"a.mp4": "video/mp4", "a.svg": "application/octet-stream", "a.html": "application/octet-stream", "a.xhtml": "application/octet-stream", "a.exe": "application/octet-stream",
		"a.pdf": "application/pdf", "a.flac": "audio/flac", "a.jpg": "image/jpeg",
	} {
		if got := MIME(name); got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
}

func zipOf(names ...string) []byte {
	var b bytes.Buffer
	for _, n := range names {
		h := make([]byte, 30)
		copy(h, "PK\x03\x04")
		binary.LittleEndian.PutUint16(h[26:], uint16(len(n)))
		b.Write(h)
		b.WriteString(n)
	}
	return b.Bytes()
}

func oggPage(first string) []byte {
	b := make([]byte, 28)
	copy(b, "OggS")
	b[26] = 1
	b[27] = byte(len(first))
	return append(b, first...)
}

func tsPackets(size, first int) []byte {
	b := make([]byte, size*6+first)
	for i := first; i < len(b); i += size {
		b[i] = 0x47
	}
	return b
}

func TestSniff(t *testing.T) {
	iso := make([]byte, MaxSniff)
	copy(iso[isoDescriptorAt:], "CD001")
	udf := make([]byte, MaxSniff)
	copy(udf[isoDescriptorAt:], "BEA01")
	raw := make([]byte, MaxSniff)
	copy(raw, "\x00\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\x00")
	mod := make([]byte, 1084)
	copy(mod[1080:], "M.K.")
	s3m := make([]byte, 96)
	copy(s3m[44:], "SCRM")
	tar := make([]byte, 512)
	copy(tar[257:], "ustar")
	asf := "\x30\x26\xB2\x75\x8E\x66\xCF\x11\xA6\xD9\x00\xAA\x00\x62\xCE\x6C"
	pdb := make([]byte, 80)
	copy(pdb[60:], "BOOKMOBI")
	bmp := make([]byte, 20)
	copy(bmp, "BM")
	bmp[14] = 40
	id3 := append([]byte("ID3\x03\x00\x00\x00\x00\x00\x02\x00\x00"), []byte("\xFF\xFB\x90\x00")...)
	ac3 := []byte("\x0B\x77\x00\x00\x00\x40")
	eac3 := []byte("\x0B\x77\x00\x00\x00\x80")
	for _, c := range []struct {
		name string
		in   []byte
		want string
	}{
		{"matroska", []byte("\x1A\x45\xDF\xA3\x01\x42\x82\x88matroska"), "mkv"},
		{"webm", []byte("\x1A\x45\xDF\xA3\x01\x42\x82\x84webm"), "webm"},
		{"mp4", []byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00isom"), "mp4"},
		{"quicktime", []byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00"), "mov"},
		{"old quicktime", []byte("\x00\x00\x00\x08moov\x00\x00\x00\x00"), "mov"},
		{"m4a", []byte("\x00\x00\x00\x18ftypM4A \x00\x00\x00\x00M4A "), "m4a"},
		{"3gp", []byte("\x00\x00\x00\x18ftyp3gp4\x00\x00\x00\x00isom"), "3gp"},
		{"heic", []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1"), "heic"},
		{"avif", []byte("\x00\x00\x00\x18ftypavif\x00\x00\x00\x00mif1"), "avif"},
		{"cr3", []byte("\x00\x00\x00\x18ftypcrx \x00\x00\x00\x00crx "), "cr3"},
		{"avi", []byte("RIFF\x00\x00\x00\x00AVI LIST"), "avi"},
		{"amv", []byte("RIFF\x00\x00\x00\x00AMV LIST"), "amv"},
		{"wav", []byte("RIFF\x00\x00\x00\x00WAVEfmt "), "wav"},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "webp"},
		{"aiff", []byte("FORM\x00\x00\x00\x00AIFFCOMM"), "aiff"},
		{"dff", []byte("FRM8\x00\x00\x00\x00\x00\x00\x00\x00DSD "), "dff"},
		{"theora", oggPage("\x80theora"), "ogv"},
		{"vorbis", oggPage("\x01vorbis"), "ogg"},
		{"opus", oggPage("OpusHead"), "opus"},
		{"wma", []byte(asf + "audio only"), "wma"},
		{"wmv", []byte(asf + "\xC0\xEF\x19\xBC\x4D\x5B\xCF\x11\xA8\xFD\x00\x80\x5F\x5C\x44\x2B"), "wmv"},
		{"wtv", []byte("\xB7\xD8\x00\x20\x37\x49\xDA\x11\xA6\x4E\x00\x07\xE9\x5E\xAD\x8D"), "wtv"},
		{"rm", []byte(".RMF\x00\x00"), "rm"},
		{"flv", []byte("FLV\x01\x05"), "flv"},
		{"mxf", []byte("\x06\x0E\x2B\x34\x02\x05\x01\x01\x0D\x01"), "mxf"},
		{"nut", []byte("nut/multimedia container\x00"), "nut"},
		{"y4m", []byte("YUV4MPEG2 W320"), "y4m"},
		{"gxf", []byte("\x00\x00\x00\x00\x01\xBC\x00"), "gxf"},
		{"ivf", []byte("DKIF\x00\x00"), "ivf"},
		{"dv", []byte("\x1F\x07\x00\x3F"), "dv"},
		{"ps", []byte("\x00\x00\x01\xBA\x44"), "mpg"},
		{"es", []byte("\x00\x00\x01\xB3\x14"), "mpeg-es"},
		{"h264", []byte("\x00\x00\x00\x01\x67\x64\x00"), "h264"},
		{"hevc", []byte("\x00\x00\x00\x01\x40\x01\x0C"), "hevc"},
		{"ts", tsPackets(188, 0), "ts"},
		{"m2ts", tsPackets(192, 4), "ts"},
		{"flac", []byte("fLaC\x00\x00"), "flac"},
		{"ape", []byte("MAC \x96\x0F"), "ape"},
		{"wavpack", []byte("wvpk\x00\x00"), "wv"},
		{"tta", []byte("TTA1\x01\x00"), "tta"},
		{"mpc", []byte("MPCK\x00\x00"), "mpc"},
		{"dsf", []byte("DSD \x1C\x00"), "dsf"},
		{"midi", []byte("MThd\x00\x00\x00\x06"), "midi"},
		{"xm", []byte("Extended Module: x"), "xm"},
		{"it", []byte("IMPM\x00\x00"), "it"},
		{"mod", mod, "mod"},
		{"s3m", s3m, "s3m"},
		{"nsf", []byte("NESM\x1A\x01"), "nsf"},
		{"spc", []byte("SNES-SPC700 Sound"), "spc"},
		{"vgm", []byte("Vgm \x00\x00"), "vgm"},
		{"ac3", ac3, "ac3"},
		{"eac3", eac3, "eac3"},
		{"dts", []byte("\x7F\xFE\x80\x01\x00"), "dts"},
		{"truehd", []byte("\x00\x00\x00\x00\xF8\x72\x6F\xBA\x00"), "truehd"},
		{"mp3 with a tag", id3, "mp3"},
		{"mp3", []byte("\xFF\xFB\x90\x00\x00"), "mp3"},
		{"adts", []byte("\xFF\xF1\x50\x80\x00"), "aac"},
		{"jpeg", []byte("\xFF\xD8\xFF\xE0\x00"), "jpg"},
		{"png", []byte("\x89PNG\r\n\x1a\n\x00"), "png"},
		{"gif", []byte("GIF89a\x00"), "gif"},
		{"bmp", bmp, "bmp"},
		{"ico", []byte("\x00\x00\x01\x00\x01\x00\x10\x10"), "ico"},
		{"tiff", []byte("II*\x00\x08\x00"), "tiff"},
		{"cr2", []byte("II*\x00\x10\x00\x00\x00CR\x02"), "cr2"},
		{"orf", []byte("IIRO\x08\x00"), "orf"},
		{"rw2", []byte("IIU\x00\x08\x00"), "rw2"},
		{"raf", []byte("FUJIFILMCCD-RAW 0201"), "raf"},
		{"jxl", []byte("\x00\x00\x00\x0cJXL \r\n\x87\n"), "jxl"},
		{"jp2", []byte("\x00\x00\x00\x0cjP  \r\n\x87\n"), "jp2"},
		{"psd", []byte("8BPS\x00\x01"), "psd"},
		{"svg", []byte(`<?xml version="1.0"?><svg xmlns="x">`), "svg"},
		{"pdf", []byte("%PDF-1.7\n"), "pdf"},
		{"djvu", []byte("AT&TFORM\x00\x00\x00\x00DJVM"), "djvu"},
		{"rtf", []byte(`{\rtf1\ansi `), "rtf"},
		{"mobi", pdb, "mobi"},
		{"fb2", []byte(`<?xml version="1.0"?><FictionBook xmlns="x">`), "fb2"},
		{"epub", zipOf("META-INF/container.xml", "OEBPS/a.html"), "epub"},
		{"docx", zipOf("[Content_Types].xml", "word/document.xml"), "docx"},
		{"zip of pictures", zipOf("001.jpg", "002.jpg"), "cbz"},
		{"plain zip", zipOf("readme.txt"), "zip"},
		{"rar", []byte("Rar!\x1A\x07\x01\x00"), "rar"},
		{"7z", []byte("7z\xBC\xAF\x27\x1C\x00"), "7z"},
		{"tar", tar, "tar"},
		{"iso", iso, "iso"},
		{"udf", udf, "iso"},
		{"raw sectors", raw, "bin"},
		{"chd", []byte("MComprHD\x00"), "chd"},
		{"program", []byte("\x7FELF\x02\x01"), "program"},
		{"windows program", []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xFF\xFF"), "program"},
		{"web page", []byte("<!DOCTYPE html><html>"), "webpage"},
		{"webvtt", []byte("WEBVTT\n\n"), "vtt"},
		{"srt", []byte("1\n00:00:01,000 --> 00:00:02,000\nHi\n"), "srt"},
		{"srt with a mark", []byte("\xEF\xBB\xBF1\r\n00:00:01,000 --> 00:00:02,000\r\nHi\r\n"), "srt"},
		{"ass", []byte("[Script Info]\nScriptType: v4.00+\n"), "ass"},
		{"ssa", []byte("[Script Info]\nScriptType: v4.00\n"), "ssa"},
		{"idx", []byte("# VobSub index file, v7\n"), "idx"},
		{"sup", []byte("PG\x00\x00\x00\x00\x00\x00\x00\x00\x16\x00"), "sup"},
		{"sami", []byte("<SAMI><BODY>"), "smi"},
		{"ttml", []byte(`<?xml version="1.0"?><tt xmlns="x">`), "ttml"},
		{"sbv", []byte("0:00:01.000,0:00:02.000\nHi\n"), "sbv"},
		{"lrc", []byte("[00:12.34]A line\n"), "lrc"},
		{"m3u", []byte("#EXTM3U\n#EXTINF:1,x\na.mp3\n"), "m3u"},
		{"pls", []byte("[playlist]\nFile1=a.mp3\n"), "pls"},
		{"cue", []byte("FILE \"a.flac\" WAVE\n  TRACK 01 AUDIO\n    INDEX 01 00:00:00\n"), "cue"},
	} {
		got := Sniff(c.in)
		switch {
		case got == nil:
			t.Errorf("%s: nothing recognised, want %s", c.name, c.want)
		case got.ID != c.want:
			t.Errorf("%s: %s, want %s", c.name, got.ID, c.want)
		}
	}
	for _, none := range [][]byte{nil, {}, []byte("a"), []byte("plain words of no format"), make([]byte, 4096), []byte("export const x: number = 1\n")} {
		if got := Sniff(none); got != nil {
			t.Errorf("%q: recognised as %s", none, got.ID)
		}
	}
}

func TestCheckAndConfirm(t *testing.T) {
	matroska := byID["mkv"]
	if Check("film.mkv", matroska) != Agrees || Check("film.webm", matroska) != Agrees {
		t.Error("Matroska and WebM read alike")
	}
	if Check("film.mkv", byID["program"]) != Differs {
		t.Error("a program called .mkv")
	}
	if Check("film.mkv", nil) != Unknown {
		t.Error("nothing recognised leaves the name standing")
	}
	if Check("raw.nef", byID["tiff"]) != Agrees {
		t.Error("RAW files read as TIFF")
	}
	if Check("comic.cbz", byID["rar"]) != Agrees || Check("comic.cbr", byID["zip"]) != Agrees {
		t.Error("comics are often not what their names say")
	}
	ts := byID["ts"]
	if Confirms(ts, []byte("export const x: number = 1\nconsole.log(x)\n")) {
		t.Error("TypeScript isn't a transport stream")
	}
	if !Confirms(ts, tsPackets(188, 0)) {
		t.Error("a transport stream is one")
	}
	if !Confirms(ts, append([]byte{1, 2, 3, 0xFF}, make([]byte, 100)...)) {
		t.Error("binary nothing recognises: FFmpeg gets to look")
	}
	if Confirms(byID["bin"], []byte{1, 2, 3, 4}) {
		t.Error(".bin is too common a name to believe")
	}
}

func TestCanonical(t *testing.T) {
	for _, c := range []struct{ format, name, v, a, want string }{
		{"matroska,webm", "a.mkv", "h264", "aac", "mkv"},
		{"matroska,webm", "a.webm", "vp9", "opus", "webm"},
		{"matroska,webm", "a.webm", "h264", "aac", "mkv"}, // no browser takes that as WebM
		{"matroska,webm", "a.mp4", "h264", "aac", "mkv"},  // the name lies
		{"mov,mp4,m4a,3gp,3g2,mj2", "a.mp4", "h264", "aac", "mp4"},
		{"mov,mp4,m4a,3gp,3g2,mj2", "a.mov", "h264", "aac", "mov"},
		{"mov,mp4,m4a,3gp,3g2,mj2", "a.mkv", "h264", "aac", "mp4"}, // the name lies
		{"mov,mp4,m4a,3gp,3g2,mj2", "a.3gp", "h264", "aac", "mp4"},
		{"mov,mp4,m4a,3gp,3g2,mj2", "a.f4v", "h264", "aac", "mp4"},
		{"mov,mp4,m4a,3gp,3g2,mj2", "a.m4a", "", "aac", "m4a"},
		{"mpegts", "a.m2ts", "h264", "ac3", "ts"},
		{"mpeg", "a.vob", "mpeg2video", "ac3", "mpg"},
		{"ogg", "a.ogv", "theora", "vorbis", "ogg"},
		{"avi", "a.divx", "mpeg4", "mp3", "avi"},
		{"asf", "a.wmv", "wmv2", "wmav2", "wmv"},
		{"flac", "a.flac", "", "flac", "flac"},
		{"wav", "a.rf64", "", "pcm_s16le", "rf64"},
		{"somethingnew", "a.x", "", "", "somethingnew"},
	} {
		if got := Canonical(c.format, c.name, c.v, c.a); got != c.want {
			t.Errorf("%s %s: %s, want %s", c.format, c.name, got, c.want)
		}
	}
}

func TestAudioPlaysDirect(t *testing.T) {
	for _, c := range []struct {
		container, codec string
		want             bool
	}{
		{"mp3", "mp3", true}, {"m4a", "aac", true}, {"m4a", "alac", false}, {"flac", "flac", true}, {"ogg", "vorbis", true}, {"ogg", "opus", true},
		{"wav", "pcm_s16le", true}, {"wav", "pcm_s24le", true}, {"webm", "opus", true}, {"mkv", "flac", false}, {"mkv", "aac", false}, {"wma", "wmav2", false},
		{"ape", "ape", false}, {"aiff", "pcm_s16be", false}, {"rf64", "pcm_s16le", false}, {"midi", "midi", false},
	} {
		if got := AudioPlaysDirect(c.container, c.codec); got != c.want {
			t.Errorf("%s %s: %v", c.container, c.codec, got)
		}
	}
}

// The generated files can't drift from the registry: this is what keeps the docs true.
// Run `go run ./internal/mediatypes/gen` after changing it.
func TestGeneratedFilesAreCurrent(t *testing.T) {
	root := filepath.Join("..", "..")
	for path, want := range map[string]string{
		"docs/SUPPORTED-FORMATS.md":       Markdown(),
		"docs/media-types.json":           string(JSON()),
		"web/src/mediaTypes.generated.ts": TypeScript(),
	} {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.ReplaceAll(string(got), "\r\n", "\n") != strings.ReplaceAll(want, "\r\n", "\n") {
			t.Errorf("%s is out of date: run go run ./internal/mediatypes/gen", path)
		}
	}
}

func TestSniffFileReadsLittle(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(1 << 40) // sparse: reading it all would never end
	f.WriteAt([]byte("fLaC\x00\x00\x00"), 0)
	f.Close()
	if got := SniffFile(p); got == nil || got.ID != "flac" {
		t.Errorf("got %v", got)
	}
	if SniffFile(dir) != nil {
		t.Error("a folder isn't media")
	}
	if SniffFile(filepath.Join(dir, "missing")) != nil {
		t.Error("a missing file isn't media")
	}
	nero := filepath.Join(dir, "disc.nrg")
	tail := append(make([]byte, 1024), "NER5\x00\x00\x00\x00\x00\x00\x00\x00"...)
	if err := os.WriteFile(nero, tail, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := SniffFile(nero); got == nil || got.ID != "nrg" {
		t.Errorf("a Nero image's mark is at its end: got %v", got)
	}
}

func FuzzSniff(f *testing.F) {
	for _, s := range [][]byte{
		[]byte("PK\x03\x04\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF"),
		[]byte("\x1A\x45\xDF\xA3\x42\x82\xFF"), []byte("\x00\x00\xFF\xFFftypheic"), []byte("OggS"), []byte("ID3\x03\x00\x00\xFF\xFF\xFF\xFF"), []byte("RIFF"), zipOf("a.jpg"),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if got := Sniff(b); got != nil && byID[got.ID] != got {
			t.Errorf("recognised as something not in the list: %v", got)
		}
	})
}
