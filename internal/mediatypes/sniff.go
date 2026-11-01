package mediatypes

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"slices"
	"strings"
)

// MaxSniff is the most of a file's start that is ever read to recognise it: enough
// to reach the ISO 9660 volume descriptor at 32 KiB, and no more. A hostile file
// can't make recognising it cost anything: nothing here allocates by what a file
// says, follows an offset out of what was read, or loops further than the bytes.
const MaxSniff = 64 << 10

const isoDescriptorAt = 0x8001

// SniffFile recognises a file by its content, reading at most MaxSniff bytes from its
// start and the few at its end that a Nero image keeps its signature in. It returns
// nil for content it doesn't recognise, or a file it can't read.
func SniffFile(path string) *Format {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil
	}
	head := make([]byte, min(int64(MaxSniff), st.Size()))
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil
	}
	if got := Sniff(head[:n]); got != nil {
		return got
	}
	if st.Size() >= 12 {
		tail := make([]byte, 12)
		if _, err := f.ReadAt(tail, st.Size()-12); err == nil && (string(tail[:4]) == "NER5" || string(tail[4:8]) == "NERO") {
			return byID["nrg"]
		}
	}
	return nil
}

// Sniff recognises a file by its first bytes, or returns nil. Give it up to MaxSniff.
func Sniff(b []byte) *Format {
	id := sniff(b)
	if id == "" {
		return nil
	}
	return byID[id]
}

// Verdict is what the content says about what a name claims.
type Verdict int

const (
	// Unknown: the content is nothing recognised, so the name stands.
	Unknown Verdict = iota
	// Agrees: the content is what the name says.
	Agrees
	// Differs: the content is clearly something else.
	Differs
)

// Check compares a name with its content. A format may look like others to Sniff
// (Format.SniffAs), and a name can belong to several formats.
func Check(name string, content *Format) Verdict {
	if content == nil {
		return Unknown
	}
	for _, f := range ByExt(name) {
		if f.ID == content.ID || slices.Contains(f.SniffAs, content.ID) {
			return Agrees
		}
	}
	return Differs
}

// Fits says a file's content suits one of these categories. Unrecognised content
// fits: FFmpeg reads many formats nobody can tell from their first bytes.
func Fits(content *Format, cats ...string) bool {
	return content == nil || slices.Contains(cats, content.Category)
}

func sniff(b []byte) string {
	if len(b) < 4 {
		return ""
	}
	has := func(sig string) bool { return bytes.HasPrefix(b, []byte(sig)) }
	at := func(off int, sig string) bool {
		return off >= 0 && off+len(sig) <= len(b) && string(b[off:off+len(sig)]) == sig
	}
	switch {
	// Containers with long signatures first.
	case has("\x1A\x45\xDF\xA3"):
		return ebmlDocType(b)
	case len(b) >= 12 && at(4, "ftyp"):
		return ftypBrand(b)
	case at(0, "\x00\x00\x00\x0cjP  \r\n\x87\n"), has("\xFF\x4F\xFF\x51"):
		return "jp2"
	case at(0, "\x00\x00\x00\x0cJXL \r\n\x87\n"), has("\xFF\x0A"):
		return "jxl"
	case at(4, "moov"), at(4, "mdat"), at(4, "free"), at(4, "wide"), at(4, "skip"), at(4, "pnot"):
		return "mov"
	case has("RIFF") && len(b) >= 12:
		switch string(b[8:12]) {
		case "WAVE":
			return "wav"
		case "AVI ":
			return "avi"
		case "AMV ":
			return "amv"
		case "WEBP":
			return "webp"
		case "RMID":
			return "midi"
		case "CDXA":
			return "vcd"
		}
		return ""
	case has("RF64") && at(8, "WAVE"):
		return "rf64"
	case has("riff\x2e\x91\xcf\x11"):
		return "w64"
	case has("FORM") && len(b) >= 12:
		switch string(b[8:12]) {
		case "AIFF", "AIFC":
			return "aiff"
		case "MMD0", "MMD1", "MMD2", "MMD3":
			return "tracker"
		}
		return ""
	case has("FRM8") && at(12, "DSD "):
		return "dff"
	case has("OggS"):
		return oggKind(b)
	case has("\x30\x26\xB2\x75\x8E\x66\xCF\x11\xA6\xD9\x00\xAA\x00\x62\xCE\x6C"):
		// A video stream's type marks it Windows Media Video, else it's sound.
		if bytes.Contains(b, []byte("\xC0\xEF\x19\xBC\x4D\x5B\xCF\x11\xA8\xFD\x00\x80\x5F\x5C\x44\x2B")) {
			return "wmv"
		}
		return "wma"
	case has("\xB7\xD8\x00\x20\x37\x49\xDA\x11\xA6\x4E\x00\x07\xE9\x5E\xAD\x8D"):
		return "wtv"
	case has(".RMF"):
		return "rm"
	case has(".ra\xFD"):
		return "ra"
	case has("FLV\x01"):
		return "flv"
	case has("\x06\x0E\x2B\x34\x02\x05\x01\x01"):
		return "mxf"
	case has("nut/multimedia container"):
		return "nut"
	case has("YUV4MPEG2"):
		return "y4m"
	case has("\x00\x00\x00\x00\x01\xBC"):
		return "gxf"
	case has("DKIF"):
		return "ivf"
	case has("BIKb"), has("BIKi"), has("KB2"):
		return "bink"
	case has("SMK2"), has("SMK4"):
		return "smacker"
	case has("NSVf"), has("NSVs"):
		return "nsv"
	case has("\x1F\x07\x00"):
		return "dv"
	case has("\x00\x00\x01\xBA"):
		return "mpg"
	case has("\x00\x00\x01\xB3"):
		return "mpeg-es"
	case rawH26x(b) != "":
		return rawH26x(b)
	case transportStream(b):
		return "ts"

	// Sound.
	case has("fLaC"):
		return "flac"
	case has("MAC "):
		return "ape"
	case has("wvpk"):
		return "wv"
	case has("TTA1"):
		return "tta"
	case has("MPCK"), has("MP+"):
		return "mpc"
	case has("tBaK"):
		return "tak"
	case has("ajkg"):
		return "shn"
	case has("DSD "):
		return "dsf"
	case has("MThd"):
		return "midi"
	case has("#!AMR"):
		return "amr"
	case has(".snd"):
		return "au"
	case has("caff"):
		return "caf"
	case has("Creative Voice File"):
		return "voc"
	case has("\x0B\x77") && len(b) > 5:
		if b[5]>>3 > 10 {
			return "eac3"
		}
		return "ac3"
	case has("\x7F\xFE\x80\x01"), has("\xFE\x7F\x01\x80"), has("\x1F\xFF\xE8\x00"), has("\xFF\x1F\x00\xE8"):
		return "dts"
	case at(4, "\xF8\x72\x6F\xBA"):
		return "truehd"
	case at(4, "\xF8\x72\x6F\xBB"):
		return "mlp"
	case has("Extended Module: "):
		return "xm"
	case has("IMPM"):
		return "it"
	case at(44, "SCRM"):
		return "s3m"
	case at(1080, "M.K."), at(1080, "M!K!"), at(1080, "FLT4"), at(1080, "FLT8"), at(1080, "4CHN"), at(1080, "6CHN"), at(1080, "8CHN"), at(1080, "CD81"), at(1080, "OCTA"):
		return "mod"
	case has("MTM\x10"), has("OKTASONG"), has("FAR\xFE"), has("DBM0"), has("PSM "), has("\xC1\x83\x2A\x9E"), has("DDMF"), has("MMD0"), has("MMD1"), has("MMD2"), has("MMD3"), has("if\x00"), has("JN\x00"):
		return "tracker"
	case has("NESM\x1A"), has("NSFE"):
		return "nsf"
	case has("SNES-SPC700"):
		return "spc"
	case has("GBS\x01"):
		return "gbs"
	case has("GYMX"):
		return "gym"
	case has("HESM"):
		return "hes"
	case has("KSCC"), has("KSSX"):
		return "kss"
	case has("SAP\r\n"):
		return "sap"
	case has("Vgm "):
		return "vgm"
	case has("ZXAY"):
		return "ay"

	// Pictures.
	case has("\xFF\xD8\xFF"):
		return "jpg"
	case has("\x89PNG\r\n\x1a\n"):
		if i := bytes.Index(b, []byte("acTL")); i > 0 {
			if j := bytes.Index(b, []byte("IDAT")); j < 0 || i < j {
				return "apng"
			}
		}
		return "png"
	case has("GIF87a"), has("GIF89a"):
		return "gif"
	case has("BM") && len(b) >= 18 && slices.Contains([]uint32{12, 40, 52, 56, 64, 108, 124}, binary.LittleEndian.Uint32(b[14:])):
		return "bmp"
	case has("\x00\x00\x01\x00") && len(b) >= 6 && b[4] != 0 && b[5] == 0, has("\x00\x00\x02\x00") && len(b) >= 6 && b[4] != 0 && b[5] == 0:
		return "ico"
	case has("IIRO"), has("IIRS"), has("MMOR"):
		return "orf"
	case has("IIU\x00"):
		return "rw2"
	case has("II*\x00") && at(8, "CR"):
		return "cr2"
	case has("II*\x00"), has("MM\x00*"), has("II+\x00"), has("MM\x00+"):
		return "tiff"
	case has("FUJIFILMCCD-RAW"):
		return "raf"
	case has("\x00MRM"), has("FOVb"), has("II\x1a\x00\x00\x00HEAPCCDR"):
		return "raw-other"
	case has("8BPS"):
		return "psd"
	case has("\x76\x2F\x31\x01"):
		return "exr"
	case has("qoif"):
		return "qoi"
	case has("#?RADIANCE"), has("#?RGBE"):
		return "hdr"
	case has("DDS "):
		return "dds"
	case has("\x01\xDA") && len(b) >= 4 && b[2] <= 1:
		return "sgi"
	case netpbm(b):
		return "pnm"
	case has("\x0A\x05\x01\x08"), has("\x0A\x03\x01\x08"):
		return "pcx"

	// Archives, which may be comics or books.
	case has("PK\x03\x04"):
		return zipKind(b)
	case has("Rar!\x1A\x07"):
		return "rar"
	case has("7z\xBC\xAF\x27\x1C"):
		return "7z"
	case at(257, "ustar"):
		return "tar"
	case has("\x1F\x8B"):
		return "gzip"
	case has("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"):
		return "ole2"
	case has("MComprHD"):
		return "chd"
	case at(60, "BOOKMOBI"):
		return "mobi"
	case at(60, "TEXtREAd"):
		return "pdb"
	case has("AT&TFORM") && (at(12, "DJVU") || at(12, "DJVM") || at(12, "DJVI")):
		return "djvu"
	case has("{\\rtf"):
		return "rtf"

	// Programs: never media, whatever they are called.
	case has("MZ") && !looksLikeText(b), has("\x7FELF"), has("\xCA\xFE\xBA\xBE"), has("\xFE\xED\xFA\xCE"), has("\xFE\xED\xFA\xCF"), has("\xCF\xFA\xED\xFE"), has("\xCE\xFA\xED\xFE"), has("#!"):
		return "program"

	// PGS subtitles: "PG", two timestamps, then a segment type.
	case has("PG") && len(b) >= 11 && slices.Contains([]byte{0x14, 0x15, 0x16, 0x17, 0x80}, b[10]):
		return "sup"
	}
	if id := discImage(b); id != "" {
		return id
	}
	if bytes.Contains(b[:min(len(b), 1024)], []byte("%PDF-")) {
		return "pdf"
	}
	if id := textKind(b); id != "" {
		return id
	}
	// Sync words are short: only once nothing longer has matched.
	return mpegAudio(b)
}

// discImage finds a disc's volume descriptor, in an image of bare sectors or of whole
// raw ones.
func discImage(b []byte) string {
	vol := func(off int) bool {
		if off+5 > len(b) {
			return false
		}
		switch string(b[off : off+5]) {
		case "CD001", "BEA01", "NSR02", "NSR03", "TEA01":
			return true
		}
		return false
	}
	if vol(isoDescriptorAt) {
		return "iso"
	}
	// 16 sectors of 2352 bytes, then a sync pattern and header ahead of the data.
	if vol(16*2352+16+1) || vol(16*2352+24+1) {
		return "bin"
	}
	if bytes.HasPrefix(b, []byte("\x00\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\x00")) {
		return "bin"
	}
	return ""
}

func ebmlDocType(b []byte) string {
	n := min(len(b), 64)
	i := bytes.Index(b[:n], []byte{0x42, 0x82})
	if i >= 0 && i+3 < n {
		size := int(b[i+2] &^ 0x80)
		if end := i + 3 + size; size > 0 && end <= n && string(b[i+3:end]) == "webm" {
			return "webm"
		}
	}
	return "mkv"
}

func ftypBrand(b []byte) string {
	size := int(binary.BigEndian.Uint32(b))
	end := min(max(size, 12), len(b), 256)
	var brands []string
	brands = append(brands, string(b[8:12]))
	for i := 16; i+4 <= end; i += 4 {
		brands = append(brands, string(b[i:i+4]))
	}
	hasBrand := func(want ...string) bool {
		for _, br := range brands {
			if slices.Contains(want, br) {
				return true
			}
		}
		return false
	}
	major := brands[0]
	switch {
	case major == "crx ":
		return "cr3"
	case major == "avif", major == "avis", major == "mif1" && hasBrand("avif"), major == "msf1" && hasBrand("avis", "avif"):
		return "avif"
	case hasBrand("heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs", "mif1", "msf1"):
		return "heic"
	case major == "qt  ":
		return "mov"
	case major == "M4A ", major == "M4B ", major == "M4P ":
		return "m4a"
	case strings.HasPrefix(major, "3gp"), strings.HasPrefix(major, "3gs"), strings.HasPrefix(major, "3gr"):
		return "3gp"
	case strings.HasPrefix(major, "3g2"):
		return "3g2"
	case major == "f4v ", major == "F4V ":
		return "f4v"
	case major == "jp2 ":
		return "jp2"
	}
	return "mp4"
}

func oggKind(b []byte) string {
	if len(b) < 28 {
		return "ogg"
	}
	segs := int(b[26])
	first := 27 + segs
	if first >= len(b) {
		return "ogg"
	}
	pkt := b[first:min(len(b), first+16)]
	has := func(s string) bool { return bytes.HasPrefix(pkt, []byte(s)) }
	switch {
	case has("\x80theora"):
		return "ogv"
	case has("OpusHead"):
		return "opus"
	case has("Speex   "):
		return "spx"
	case has("fishead"), has("\x01video"), has("OVP80"):
		return "ogv"
	}
	// Other streams may lead: look for a picture's header further in.
	if bytes.Contains(b, []byte("\x80theora")) || bytes.Contains(b, []byte("OVP80")) {
		return "ogv"
	}
	return "ogg"
}

// zipKind reads the entries that fit in what was read. Their names and sizes are
// bytes in the file: each is checked against what's here before it is used.
func zipKind(b []byte) string {
	off, images, entries := 0, 0, 0
	docx, odtEntry, epubEntry := false, false, false
	for off+30 <= len(b) && entries < 512 && string(b[off:off+4]) == "PK\x03\x04" {
		entries++
		method := binary.LittleEndian.Uint16(b[off+8:])
		csize := int(binary.LittleEndian.Uint32(b[off+18:]))
		nameLen := int(binary.LittleEndian.Uint16(b[off+26:]))
		extra := int(binary.LittleEndian.Uint16(b[off+28:]))
		start := off + 30
		if nameLen == 0 || start+nameLen > len(b) {
			break
		}
		name := string(b[start : start+nameLen])
		dataAt := start + nameLen + extra
		if name == "mimetype" && method == 0 && dataAt+csize <= len(b) && csize < 256 {
			content := string(b[dataAt : dataAt+csize])
			switch {
			case strings.HasPrefix(content, "application/epub+zip"):
				epubEntry = true
			case strings.HasPrefix(content, "application/vnd.oasis.opendocument.text"):
				odtEntry = true
			}
		}
		low := strings.ToLower(name)
		switch {
		case low == "[content_types].xml", strings.HasPrefix(low, "word/"):
			docx = docx || strings.HasPrefix(low, "word/")
		case strings.HasPrefix(low, "meta-inf/container.xml"):
			epubEntry = true
		case strings.HasSuffix(low, ".fb2"):
			return "fb2"
		case isPageName(low), low == "comicinfo.xml":
			images++
		}
		// An entry with a size only after its data (a data descriptor) can't be
		// stepped over from here.
		if flags := binary.LittleEndian.Uint16(b[off+6:]); flags&8 != 0 || csize < 0 {
			break
		}
		next := dataAt + csize
		if next <= off {
			break
		}
		off = next
	}
	switch {
	case epubEntry:
		return "epub"
	case odtEntry:
		return "odt"
	case docx:
		return "docx"
	case images > 0:
		return "cbz"
	}
	return "zip"
}

func isPageName(low string) bool {
	switch Ext(low) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif", ".bmp", ".jxl", ".tif", ".tiff":
		return true
	}
	return false
}

// rawH26x finds the parameter sets at the start of a raw H.264 or HEVC stream.
func rawH26x(b []byte) string {
	if len(b) < 6 {
		return ""
	}
	nal := -1
	switch {
	case b[0] == 0 && b[1] == 0 && b[2] == 0 && b[3] == 1:
		nal = 4
	case b[0] == 0 && b[1] == 0 && b[2] == 1:
		nal = 3
	}
	if nal < 0 {
		return ""
	}
	h := b[nal]
	if h&0x1F == 7 || h&0x1F == 9 { // sequence parameters, or an access unit delimiter
		if h&0x80 == 0 {
			return "h264"
		}
	}
	if t := (h >> 1) & 0x3F; h&0x80 == 0 && (t == 32 || t == 35) && b[nal+1] == 1 {
		return "hevc"
	}
	return ""
}

// transportStream looks for the sync byte at each packet's start, for packets of 188
// bytes and for 192 (Blu-ray) and 204 (broadcast).
func transportStream(b []byte) bool {
	type shape struct{ first, size int }
	for _, sh := range []shape{{0, 188}, {4, 192}, {0, 204}} {
		hits := 0
		for off := sh.first; off < len(b) && hits < 5; off += sh.size {
			if b[off] != 0x47 {
				hits = -1
				break
			}
			hits++
		}
		if hits >= 5 {
			return true
		}
	}
	return false
}

func netpbm(b []byte) bool {
	if len(b) < 4 || b[0] != 'P' || b[1] < '1' || b[1] > '7' {
		return false
	}
	c := b[2]
	return c == ' ' || c == '\n' || c == '\r' || c == '\t'
}

// mpegAudio checks a frame header at the start, after any ID3 tag, for what it
// really is: the tag may front AAC or FLAC as well as MP3.
func mpegAudio(b []byte) string {
	if len(b) < 4 {
		return ""
	}
	if string(b[:3]) == "ID3" && len(b) >= 10 {
		size := int(b[6]&0x7F)<<21 | int(b[7]&0x7F)<<14 | int(b[8]&0x7F)<<7 | int(b[9]&0x7F)
		rest := 10 + size
		if rest+4 > len(b) {
			return "mp3" // the tag runs past what was read, as a picture in it will
		}
		switch {
		case bytes.HasPrefix(b[rest:], []byte("fLaC")):
			return "flac"
		case bytes.HasPrefix(b[rest:], []byte("MAC ")):
			return "ape"
		}
		if id := mpegAudio(b[rest:]); id != "" {
			return id
		}
		return "mp3"
	}
	if b[0] != 0xFF || b[1]&0xE0 != 0xE0 {
		return ""
	}
	version := (b[1] >> 3) & 3
	layer := (b[1] >> 1) & 3
	if layer == 0 {
		if version >= 2 && (b[1]&0xF6) == 0xF0 { // ADTS
			return "aac"
		}
		return ""
	}
	bitrate, rate := b[2]>>4, (b[2]>>2)&3
	if version == 1 || bitrate == 15 || bitrate == 0 || rate == 3 {
		return ""
	}
	if layer == 2 { // layer II
		return "mp2"
	}
	return "mp3"
}

// textKind recognises formats that are text. It looks at the start only, past a byte
// order mark and the blank lines that editors leave.
func textKind(b []byte) string {
	if !looksLikeText(b) {
		return ""
	}
	t := bytes.TrimPrefix(b, []byte("\xEF\xBB\xBF"))
	t = bytes.TrimLeft(t, " \t\r\n")
	has := func(s string) bool { return bytes.HasPrefix(t, []byte(s)) }
	low := strings.ToLower(string(t[:min(len(t), 4096)]))
	lowHas := func(s string) bool { return strings.HasPrefix(low, s) }
	switch {
	case has("WEBVTT"):
		return "vtt"
	case lowHas("[script info]"):
		if strings.Contains(low, "v4.00+") {
			return "ass"
		}
		return "ssa"
	case has("# VobSub"):
		return "idx"
	case has("#EXTM3U"), has("#EXTINF"):
		return "m3u"
	case lowHas("[playlist]"):
		return "pls"
	case strings.Contains(low, "<playlist") && strings.Contains(low, "xspf"):
		return "xspf"
	case lowHas("<asx"):
		return "asx"
	case lowHas("<?wpl"), strings.Contains(low, "<smil") && strings.Contains(low, "<seq"):
		return "asx"
	case lowHas("<sami"):
		return "smi"
	case strings.Contains(low, "<fictionbook"):
		return "fb2"
	case strings.Contains(low, "<svg") && xmlStart(low):
		return "svg"
	case xmlStart(low) && (strings.Contains(low, "<tt ") || strings.Contains(low, "<tt:tt") || strings.Contains(low, "<tt>")):
		return "ttml"
	case lowHas("<!doctype html"), lowHas("<html"), lowHas("<!--") && strings.Contains(low, "<html"), xmlStart(low) && strings.Contains(low, "<html"):
		return "webpage"
	case cueSheet(low):
		return "cue"
	case microDVD(t):
		return "microdvd"
	case lowHas("[information]"), strings.Contains(low, "[br]") && subViewerTime(low):
		return "subviewer"
	case subRip(t):
		return "srt"
	case sbvTime(t):
		return "sbv"
	case lyricsTags(low):
		return "lrc"
	}
	return ""
}

func xmlStart(low string) bool {
	return strings.HasPrefix(low, "<?xml") || strings.HasPrefix(low, "<!--") || strings.HasPrefix(low, "<!doctype") || strings.HasPrefix(low, "<svg") || strings.HasPrefix(low, "<tt")
}

// looksLikeText: no control bytes other than the usual ones among the first bytes.
func looksLikeText(b []byte) bool {
	n := min(len(b), 2048)
	if n == 0 {
		return false
	}
	for _, c := range b[:n] {
		if c < 9 || (c > 13 && c < 32 && c != 27) {
			return false
		}
	}
	return true
}

func cueSheet(low string) bool {
	first := low
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	starts := false
	for _, w := range []string{"file ", "rem ", "title ", "performer ", "catalog ", "cdtextfile ", "songwriter "} {
		if strings.HasPrefix(first, w) {
			starts = true
		}
	}
	return starts && strings.Contains(low, "track ") && strings.Contains(low, "index ")
}

func microDVD(t []byte) bool {
	if len(t) < 6 || t[0] != '{' {
		return false
	}
	line := t
	if i := bytes.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	a := bytes.IndexByte(line, '}')
	if a < 2 || a+1 >= len(line) || line[a+1] != '{' {
		return false
	}
	return allDigits(line[1:a])
}

func allDigits(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// subRip: a cue number, then a time line with an arrow.
func subRip(t []byte) bool {
	line, rest, ok := bytes.Cut(t, []byte("\n"))
	if !ok || !allDigits(bytes.TrimSpace(line)) {
		return false
	}
	next, _, _ := bytes.Cut(rest, []byte("\n"))
	return bytes.Contains(next, []byte("-->"))
}

// sbvTime: 0:00:01.000,0:00:02.000 on the first line.
func sbvTime(t []byte) bool {
	line, _, _ := bytes.Cut(t, []byte("\n"))
	line = bytes.TrimSpace(line)
	a, b, ok := bytes.Cut(line, []byte(","))
	return ok && timeLike(a) && timeLike(b)
}

func timeLike(b []byte) bool {
	if len(b) < 7 || bytes.Count(b, []byte(":")) != 2 || bytes.Count(b, []byte(".")) != 1 {
		return false
	}
	for _, c := range b {
		if (c < '0' || c > '9') && c != ':' && c != '.' {
			return false
		}
	}
	return true
}

func subViewerTime(low string) bool {
	line, _, _ := strings.Cut(low, "\n")
	return strings.Count(line, ":") >= 4 && strings.Contains(line, ",")
}

func lyricsTags(low string) bool {
	if !strings.HasPrefix(low, "[") {
		return false
	}
	for _, tag := range []string{"[ar:", "[ti:", "[al:", "[by:", "[length:", "[offset:"} {
		if strings.HasPrefix(low, tag) {
			return true
		}
	}
	// A time at the start: [00:12.34].
	line, _, _ := strings.Cut(low, "]")
	if len(line) < 6 {
		return false
	}
	body := line[1:]
	if !strings.Contains(body, ":") {
		return false
	}
	for _, c := range body {
		if (c < '0' || c > '9') && c != ':' && c != '.' {
			return false
		}
	}
	return true
}
