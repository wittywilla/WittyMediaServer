package mediatypes

// The kinds of library each category belongs to.
var (
	videoLibs    = []string{"movies", "shows", "anime", "videos", "shorts", "channels", "musicvideos", "commercials", "photos"}
	audioLibs    = []string{"music", "audiobooks", "podcasts"}
	subtitleLibs = []string{"movies", "shows", "anime", "videos", "shorts", "channels", "musicvideos"}
	photoLibs    = []string{"photos"}
	bookLibs     = []string{"books"}
	discLibs     = []string{"movies", "shows", "anime", "discs", "music"}
	sidecarLibs  = []string{"movies", "shows", "anime", "videos", "music", "audiobooks", "photos", "books"}
)

const (
	needFFmpeg     = "FFmpeg"
	needNothing    = "Nothing extra"
	needOpenMPT    = "An FFmpeg built with libopenmpt"
	needGME        = "An FFmpeg built with libgme"
	needDjVuLibre  = "The DjVuLibre tools on the server (ddjvu), to open it"
	needFFmpegHEIF = "An FFmpeg of version 7 or later"
)

func v(id, name string, exts []string, mime string, ffmpeg []string, plays string, extra ...func(*Format)) Format {
	return build(Format{ID: id, Name: name, Category: Video, Exts: exts, MIME: mime, FFmpeg: ffmpeg, Plays: plays, Libraries: videoLibs, Needs: needFFmpeg}, extra)
}

func a(id, name string, exts []string, mime string, ffmpeg []string, plays string, extra ...func(*Format)) Format {
	return build(Format{ID: id, Name: name, Category: Audio, Exts: exts, MIME: mime, FFmpeg: ffmpeg, Plays: plays, Libraries: audioLibs, Needs: needFFmpeg}, extra)
}

func s(id, name string, exts []string, plays string, extra ...func(*Format)) Format {
	return build(Format{ID: id, Name: name, Category: Subtitle, Exts: exts, MIME: "application/octet-stream", Plays: plays, Libraries: subtitleLibs, Needs: needFFmpeg}, extra)
}

func p(id, name string, exts []string, mime, plays string, extra ...func(*Format)) Format {
	return build(Format{ID: id, Name: name, Category: Photo, Exts: exts, MIME: mime, Plays: plays, Libraries: photoLibs, Needs: needFFmpeg}, extra)
}

func b(id, name, cat string, exts []string, mime, plays string, extra ...func(*Format)) Format {
	return build(Format{ID: id, Name: name, Category: cat, Exts: exts, MIME: mime, Plays: plays, Libraries: bookLibs, Needs: needNothing}, extra)
}

func build(f Format, extra []func(*Format)) Format {
	for _, e := range extra {
		e(&f)
	}
	return f
}

func confirm(f *Format) { f.Confirm = true }

func note(s string) func(*Format)  { return func(f *Format) { f.Notes = s } }
func needs(s string) func(*Format) { return func(f *Format) { f.Needs = s } }
func looksLike(ids ...string) func(*Format) {
	return func(f *Format) { f.SniffAs = ids }
}
func demuxer(n string) func(*Format) { return func(f *Format) { f.Demuxer = n } }
func libs(l ...string) func(*Format) { return func(f *Format) { f.Libraries = l } }

func exts(e ...string) []string { return e }

// The registry. Order matters only within a category: the first format that uses an
// extension answers for it.
var formats = []Format{
	// ---- Video: the containers browsers can open themselves.
	v("mp4", "MPEG-4", exts(".mp4", ".m4v", ".mp4v"), "video/mp4", []string{"mov", "mp4"}, PlayNative, looksLike("mov", "3gp", "3g2", "f4v", "heic", "avif", "m4a")),
	v("mov", "QuickTime", exts(".mov", ".qt"), "video/quicktime", []string{"mov"}, PlayNative, looksLike("mp4", "m4a"), note("Sent to browsers as MPEG-4, which is what they play it as.")),
	v("webm", "WebM", exts(".webm"), "video/webm", []string{"webm"}, PlayNative, looksLike("mkv")),
	v("3gp", "3GPP (phones)", exts(".3gp", ".3gpp"), "video/mp4", []string{"mov", "3gp"}, PlayNative, looksLike("mp4", "3g2")),
	v("3g2", "3GPP2 (phones)", exts(".3g2", ".3gp2"), "video/mp4", []string{"mov", "3g2"}, PlayNative, looksLike("mp4", "3gp")),
	v("f4v", "Flash MP4", exts(".f4v"), "video/mp4", []string{"mov"}, PlayNative, looksLike("mp4")),

	// ---- Video: rewrapped for browsers.
	v("mkv", "Matroska", exts(".mkv", ".mk3d"), "video/x-matroska", []string{"matroska"}, PlayRewrap, looksLike("webm")),
	v("ts", "MPEG transport stream", exts(".ts", ".m2ts", ".mts", ".m2t", ".tp", ".trp"), "video/mp2t", []string{"mpegts"}, PlayRewrap, confirm),
	v("flv", "Flash Video", exts(".flv"), "video/x-flv", []string{"flv"}, PlayRewrap),
	v("mpg", "MPEG program stream", exts(".mpg", ".mpeg", ".mpe", ".m2p", ".vob", ".evo", ".vro"), "video/mpeg", []string{"mpeg", "mpegvideo", "vob"}, PlayRewrap, looksLike("mpeg-es", "mp3")),
	v("camcorder", "Camcorder MPEG", exts(".mod", ".tod"), "video/mpeg", []string{"mpeg"}, PlayRewrap, confirm, looksLike("mpg", "ts"), note("MOD and TOD files from camcorders. A .mod in a music library is a tracker song instead.")),
	v("vcd", "Video CD track", exts(".dat"), "video/mpeg", []string{"mpeg"}, PlayRewrap, confirm, looksLike("mpg"), note("Only when its content really is an MPEG stream.")),

	// ---- Video: always converted.
	v("avi", "AVI", exts(".avi", ".divx", ".xvid"), "video/x-msvideo", []string{"avi"}, PlayRewrap),
	v("wmv", "Windows Media Video", exts(".wmv"), "video/x-ms-wmv", []string{"asf"}, PlayConvert, looksLike("asf", "wma")),
	v("asf", "Advanced Systems Format", exts(".asf"), "video/x-ms-asf", []string{"asf"}, PlayConvert, looksLike("wmv", "wma")),
	v("dvr-ms", "Windows Media Center recording", exts(".dvr-ms"), "video/x-ms-asf", []string{"asf"}, PlayConvert, looksLike("wmv", "asf", "wma")),
	v("wtv", "Windows TV recording", exts(".wtv"), "video/x-ms-wtv", []string{"wtv"}, PlayConvert),
	v("rm", "RealMedia", exts(".rm", ".rmvb", ".rv"), "application/vnd.rn-realmedia", []string{"rm"}, PlayConvert),
	v("ogv", "Ogg video", exts(".ogv", ".ogm", ".ogx"), "video/ogg", []string{"ogg"}, PlayConvert, looksLike("ogg", "opus", "spx")),
	v("mxf", "Material Exchange Format", exts(".mxf"), "application/mxf", []string{"mxf", "mxf_d10", "mxf_opatom"}, PlayConvert),
	v("dv", "Digital Video", exts(".dv", ".dif"), "video/x-dv", []string{"dv"}, PlayConvert),
	v("nut", "NUT", exts(".nut"), "application/octet-stream", []string{"nut"}, PlayConvert),
	v("y4m", "YUV4MPEG", exts(".y4m"), "application/octet-stream", []string{"yuv4mpegpipe"}, PlayConvert),
	v("gxf", "General Exchange Format", exts(".gxf"), "application/gxf", []string{"gxf"}, PlayConvert),
	v("amv", "AMV", exts(".amv"), "application/octet-stream", []string{"amv"}, PlayConvert, looksLike("avi")),
	v("mjpeg", "Motion JPEG", exts(".mjpeg", ".mjpg"), "video/x-motion-jpeg", []string{"mjpeg"}, PlayConvert, looksLike("jpg")),
	v("ivf", "IVF", exts(".ivf"), "application/octet-stream", []string{"ivf"}, PlayConvert),
	v("mpeg-es", "MPEG video stream", exts(".m1v", ".m2v", ".mpv"), "video/mpeg", []string{"mpegvideo"}, PlayConvert, looksLike("mpg")),
	v("h264", "Raw H.264", exts(".h264", ".264", ".avc"), "application/octet-stream", []string{"h264"}, PlayRewrap),
	v("hevc", "Raw HEVC", exts(".h265", ".265", ".hevc"), "application/octet-stream", []string{"hevc"}, PlayRewrap),
	v("vvc", "Raw VVC", exts(".h266", ".266", ".vvc"), "application/octet-stream", []string{"vvc"}, PlayConvert, note("Needs an FFmpeg that decodes VVC.")),
	v("obu", "Raw AV1", exts(".obu"), "application/octet-stream", []string{"obu", "av1"}, PlayRewrap),
	v("bink", "Bink Video", exts(".bik", ".bk2"), "application/octet-stream", []string{"bink"}, PlayConvert),
	v("smacker", "Smacker Video", exts(".smk"), "application/octet-stream", []string{"smk"}, PlayConvert),
	v("nsv", "Nullsoft Streaming Video", exts(".nsv"), "application/octet-stream", []string{"nsv"}, PlayConvert),
	v("flic", "FLIC animation", exts(".fli", ".flc"), "application/octet-stream", []string{"flic"}, PlayConvert),
	v("ty", "TiVo recording", exts(".ty"), "application/octet-stream", []string{"ty"}, PlayConvert, confirm, note("Only recordings that aren't encrypted. Encrypted ones are refused.")),

	// ---- Audio.
	a("mp3", "MP3", exts(".mp3"), "audio/mpeg", []string{"mp3"}, PlayNative, looksLike("aac", "mp2")),
	a("mp2", "MPEG audio layer II", exts(".mp2", ".mpa", ".m2a"), "audio/mpeg", []string{"mp3"}, PlayConvert, looksLike("mp3")),
	a("aac", "AAC", exts(".aac", ".adts"), "audio/aac", []string{"aac"}, PlayNative, looksLike("mp3")),
	a("m4a", "MPEG-4 audio", exts(".m4a", ".m4b", ".m4r", ".alac"), "audio/mp4", []string{"mov", "m4a"}, PlayNative, looksLike("mp4", "mov"), note("Plays in a browser with AAC inside. ALAC is converted.")),
	a("flac", "FLAC", exts(".flac", ".fla"), "audio/flac", []string{"flac"}, PlayNative),
	a("ogg", "Ogg audio", exts(".ogg", ".oga"), "audio/ogg", []string{"ogg"}, PlayNative, looksLike("opus", "ogv", "spx")),
	a("opus", "Opus", exts(".opus"), "audio/ogg", []string{"ogg", "opus"}, PlayNative, looksLike("ogg")),
	a("spx", "Speex", exts(".spx"), "audio/ogg", []string{"ogg"}, PlayConvert, looksLike("ogg", "opus")),
	a("wav", "WAV", exts(".wav", ".wave", ".bwf"), "audio/wav", []string{"wav"}, PlayNative),
	a("w64", "Sony Wave64", exts(".w64"), "application/octet-stream", []string{"w64"}, PlayConvert),
	a("rf64", "RF64", exts(".rf64"), "application/octet-stream", []string{"rf64"}, PlayConvert),
	a("aiff", "AIFF", exts(".aiff", ".aif", ".aifc"), "audio/aiff", []string{"aiff"}, PlayConvert),
	a("wma", "Windows Media Audio", exts(".wma"), "audio/x-ms-wma", []string{"asf"}, PlayConvert, looksLike("asf", "wmv")),
	a("ape", "Monkey's Audio", exts(".ape"), "application/octet-stream", []string{"ape"}, PlayConvert),
	a("wv", "WavPack", exts(".wv"), "application/octet-stream", []string{"wv"}, PlayConvert),
	a("tta", "True Audio", exts(".tta"), "application/octet-stream", []string{"tta"}, PlayConvert),
	a("mpc", "Musepack", exts(".mpc", ".mp+", ".mpp"), "application/octet-stream", []string{"mpc", "mpc8"}, PlayConvert),
	a("tak", "TAK", exts(".tak"), "application/octet-stream", []string{"tak"}, PlayConvert),
	a("shn", "Shorten", exts(".shn"), "application/octet-stream", []string{"shn"}, PlayConvert),
	a("dsf", "DSD (DSF)", exts(".dsf"), "application/octet-stream", []string{"dsf"}, PlayConvert),
	a("dff", "DSD (DFF)", exts(".dff"), "application/octet-stream", []string{"iff"}, PlayConvert),
	a("ac3", "Dolby Digital", exts(".ac3"), "audio/ac3", []string{"ac3"}, PlayConvert, looksLike("eac3")),
	a("eac3", "Dolby Digital Plus", exts(".eac3", ".ec3"), "audio/eac3", []string{"eac3"}, PlayConvert, looksLike("ac3")),
	a("dts", "DTS", exts(".dts", ".dtshd", ".dtsma"), "audio/vnd.dts", []string{"dts", "dtshd"}, PlayConvert),
	a("truehd", "Dolby TrueHD", exts(".truehd", ".thd"), "application/octet-stream", []string{"truehd"}, PlayConvert, looksLike("mlp")),
	a("mlp", "Meridian Lossless", exts(".mlp"), "application/octet-stream", []string{"mlp"}, PlayConvert, looksLike("truehd")),
	a("mka", "Matroska audio", exts(".mka"), "audio/x-matroska", []string{"matroska"}, PlayConvert, looksLike("mkv", "webm")),
	a("weba", "WebM audio", exts(".weba"), "audio/webm", []string{"webm", "matroska"}, PlayNative, looksLike("mkv", "webm")),
	a("amr", "AMR (voice)", exts(".amr", ".awb"), "audio/amr", []string{"amr", "amrnb", "amrwb"}, PlayConvert),
	a("au", "Sun audio", exts(".au", ".snd"), "audio/basic", []string{"au"}, PlayConvert, confirm),
	a("caf", "Core Audio", exts(".caf"), "audio/x-caf", []string{"caf"}, PlayConvert),
	a("ra", "RealAudio", exts(".ra"), "audio/x-pn-realaudio", []string{"rm"}, PlayConvert, looksLike("rm")),
	a("voc", "Creative Voice", exts(".voc"), "application/octet-stream", []string{"voc"}, PlayConvert),
	a("gsm", "GSM voice", exts(".gsm"), "application/octet-stream", []string{"gsm"}, PlayConvert, confirm),
	a("midi", "MIDI", exts(".mid", ".midi", ".kar", ".rmi"), "audio/midi", nil, PlayConvert, needs("Nothing extra: the server has its own synthesiser"), note("Played by the server's built-in synthesiser, which is simple but needs no sound font.")),
	a("mod", "Tracker: ProTracker", exts(".mod"), "application/octet-stream", []string{"libopenmpt"}, PlayConvert, needs(needOpenMPT), demuxer("libopenmpt"), confirm),
	a("xm", "Tracker: FastTracker", exts(".xm"), "application/octet-stream", []string{"libopenmpt"}, PlayConvert, needs(needOpenMPT), demuxer("libopenmpt")),
	a("s3m", "Tracker: Scream Tracker", exts(".s3m", ".stm"), "application/octet-stream", []string{"libopenmpt"}, PlayConvert, needs(needOpenMPT), demuxer("libopenmpt")),
	a("it", "Tracker: Impulse Tracker", exts(".it", ".mptm"), "application/octet-stream", []string{"libopenmpt"}, PlayConvert, needs(needOpenMPT), demuxer("libopenmpt")),
	a("tracker", "Tracker: other formats", exts(".669", ".mtm", ".med", ".okt", ".ult", ".far", ".dbm", ".ams", ".dmf", ".psm", ".umx", ".j2b", ".mdl", ".ptm", ".gdm"), "application/octet-stream", []string{"libopenmpt"}, PlayConvert, needs(needOpenMPT), demuxer("libopenmpt"), confirm),
	a("nsf", "Game music: Nintendo", exts(".nsf", ".nsfe"), "application/octet-stream", []string{"libgme"}, PlayConvert, needs(needGME), demuxer("libgme")),
	a("spc", "Game music: Super Nintendo", exts(".spc"), "application/octet-stream", []string{"libgme"}, PlayConvert, needs(needGME), demuxer("libgme")),
	a("gbs", "Game music: Game Boy", exts(".gbs"), "application/octet-stream", []string{"libgme"}, PlayConvert, needs(needGME), demuxer("libgme")),
	a("vgm", "Game music: Video Game Music", exts(".vgm", ".vgz"), "application/octet-stream", []string{"libgme"}, PlayConvert, needs(needGME), demuxer("libgme"), looksLike("gzip")),
	a("gym", "Game music: Genesis", exts(".gym"), "application/octet-stream", []string{"libgme"}, PlayConvert, needs(needGME), demuxer("libgme")),
	a("hes", "Game music: PC Engine", exts(".hes"), "application/octet-stream", []string{"libgme"}, PlayConvert, needs(needGME), demuxer("libgme")),
	a("kss", "Game music: MSX", exts(".kss"), "application/octet-stream", []string{"libgme"}, PlayConvert, needs(needGME), demuxer("libgme")),
	a("sap", "Game music: Atari", exts(".sap"), "application/octet-stream", []string{"libgme"}, PlayConvert, needs(needGME), demuxer("libgme")),
	a("ay", "Game music: ZX Spectrum", exts(".ay"), "application/octet-stream", []string{"libgme"}, PlayConvert, needs(needGME), demuxer("libgme"), confirm),

	// ---- Subtitles.
	s("srt", "SubRip", exts(".srt"), PlayTrack),
	s("ass", "Advanced SubStation", exts(".ass"), PlayTrack, looksLike("ssa"), note("Shown as plain text by default, or burned in to keep its styling.")),
	s("ssa", "SubStation Alpha", exts(".ssa"), PlayTrack, looksLike("ass"), note("Shown as plain text by default, or burned in to keep its styling.")),
	s("vtt", "WebVTT", exts(".vtt", ".webvtt"), PlayTrack),
	s("sub", "MicroDVD, SubViewer or VobSub", exts(".sub"), PlayTrack, looksLike("mpg", "microdvd", "subviewer", "srt"), note("A .sub beside an .idx is VobSub, a picture format, and is burned in. Other .sub files are text.")),
	s("idx", "VobSub index", exts(".idx"), PlayBurn, note("Pictures from a DVD, read with the .sub beside it, and burned into the picture.")),
	s("sup", "Blu-ray PGS", exts(".sup", ".pgs"), PlayBurn, note("Pictures from a Blu-ray, burned into the picture.")),
	s("sbv", "YouTube SubViewer", exts(".sbv"), PlayTrack, needs(needNothing)),
	s("ttml", "TTML", exts(".ttml", ".ttml2"), PlayTrack, needs(needNothing), looksLike("dfxp")),
	s("dfxp", "DFXP", exts(".dfxp"), PlayTrack, needs(needNothing), looksLike("ttml")),
	s("smi", "SAMI", exts(".smi", ".sami"), PlayTrack),
	s("lrc", "Timed lyrics", exts(".lrc"), PlayTrack, needs(needNothing), libs(append(append([]string{}, subtitleLibs...), "music")...), note("Beside a song, these are its lyrics. Beside a video, a subtitle track.")),
	s("mks", "Matroska subtitles", exts(".mks"), PlayTrack, looksLike("mkv")),
	s("scc", "Scenarist closed captions", exts(".scc"), PlayTrack),
	s("mpl2", "MPL2", exts(".mpl"), PlayTrack, confirm),
	s("stl", "Spruce subtitles", exts(".stl"), PlayTrack, confirm),
	s("jss", "JACOsub", exts(".jss"), PlayTrack),
	s("realtext", "RealText", exts(".rt"), PlayTrack, confirm),

	// ---- Photos a browser shows as they are.
	p("jpg", "JPEG", exts(".jpg", ".jpeg", ".jpe", ".jfif"), "image/jpeg", PlayImage, note("A motion photo's clip, stored after the picture, plays with it.")),
	p("png", "PNG", exts(".png"), "image/png", PlayImage, looksLike("apng")),
	p("apng", "Animated PNG", exts(".apng"), "image/apng", PlayImage, looksLike("png")),
	p("gif", "GIF", exts(".gif"), "image/gif", PlayImage),
	p("webp", "WebP", exts(".webp"), "image/webp", PlayImage),
	p("avif", "AVIF", exts(".avif"), "image/avif", PlayImage, looksLike("heic", "mp4")),
	p("bmp", "BMP", exts(".bmp", ".dib"), "image/bmp", PlayImage),
	p("ico", "Icon", exts(".ico", ".cur"), "image/x-icon", PlayImage),
	p("svg", "SVG", exts(".svg", ".svgz"), "image/svg+xml", PlayImage, needs(needNothing), looksLike("gzip"), note("Scripts, outside links and active content are removed before it is sent. FFmpeg with librsvg makes smaller copies when it has it.")),

	// ---- Photos that reach the viewer as a JPEG.
	p("heic", "HEIC and HEIF", exts(".heic", ".heif", ".hif"), "image/heif", PlayThumb, needs(needFFmpegHEIF), looksLike("avif", "mp4")),
	p("jxl", "JPEG XL", exts(".jxl"), "image/jxl", PlayThumb, needs("An FFmpeg built with libjxl"), confirm, note("A file with this name and no JPEG XL signature isn't listed: FFmpeg would work at one for minutes and find nothing.")),
	p("tiff", "TIFF", exts(".tif", ".tiff"), "image/tiff", PlayThumb, looksLike("cr2", "nef")),
	p("jp2", "JPEG 2000", exts(".jp2", ".j2k", ".jpf", ".jpx"), "image/jp2", PlayThumb),
	p("psd", "Photoshop", exts(".psd"), "application/octet-stream", PlayThumb),
	p("tga", "Targa", exts(".tga"), "application/octet-stream", PlayThumb, confirm),
	p("exr", "OpenEXR", exts(".exr"), "application/octet-stream", PlayThumb),
	p("hdr", "Radiance HDR", exts(".hdr"), "application/octet-stream", PlayThumb, confirm),
	p("dds", "DirectDraw Surface", exts(".dds"), "application/octet-stream", PlayThumb),
	p("pcx", "PCX", exts(".pcx"), "application/octet-stream", PlayThumb),
	p("pnm", "Netpbm", exts(".ppm", ".pgm", ".pbm", ".pnm", ".pam"), "application/octet-stream", PlayThumb),
	p("qoi", "QOI", exts(".qoi"), "application/octet-stream", PlayThumb),
	p("sgi", "SGI", exts(".sgi", ".rgb"), "application/octet-stream", PlayThumb, confirm),
	// Camera RAW: shown through the JPEG every camera stores inside the file.
	p("cr2", "Canon RAW", exts(".cr2", ".crw"), "image/x-canon-cr2", PlayThumb, looksLike("tiff"), needs(needNothing)),
	p("cr3", "Canon RAW 3", exts(".cr3"), "image/x-canon-cr3", PlayThumb, looksLike("mp4", "mov"), needs(needNothing)),
	p("nef", "Nikon RAW", exts(".nef", ".nrw"), "image/x-nikon-nef", PlayThumb, looksLike("tiff"), needs(needNothing)),
	p("arw", "Sony RAW", exts(".arw", ".srf", ".sr2"), "image/x-sony-arw", PlayThumb, looksLike("tiff"), needs(needNothing)),
	p("dng", "Adobe DNG", exts(".dng"), "image/x-adobe-dng", PlayThumb, looksLike("tiff"), needs(needNothing)),
	p("raf", "Fujifilm RAW", exts(".raf"), "image/x-fuji-raf", PlayThumb, needs(needNothing)),
	p("orf", "Olympus RAW", exts(".orf"), "image/x-olympus-orf", PlayThumb, looksLike("tiff"), needs(needNothing)),
	p("rw2", "Panasonic RAW", exts(".rw2"), "image/x-panasonic-rw2", PlayThumb, looksLike("tiff"), needs(needNothing)),
	p("pef", "Pentax RAW", exts(".pef"), "image/x-pentax-pef", PlayThumb, looksLike("tiff"), needs(needNothing)),
	p("srw", "Samsung RAW", exts(".srw"), "image/x-samsung-srw", PlayThumb, looksLike("tiff"), needs(needNothing)),
	p("raw-other", "Other camera RAW", exts(".3fr", ".fff", ".erf", ".kdc", ".dcr", ".mef", ".mos", ".mrw", ".iiq", ".x3f", ".rwl"), "application/octet-stream", PlayThumb, looksLike("tiff"), needs(needNothing)),

	// ---- Comics: page images in an archive, whatever the archive's name says.
	b("cbz", "Comic (ZIP)", Comic, exts(".cbz"), "application/vnd.comicbook+zip", PlayReader, looksLike("zip", "rar", "7z", "tar")),
	b("cbr", "Comic (RAR)", Comic, exts(".cbr"), "application/vnd.comicbook-rar", PlayReader, looksLike("rar", "zip", "7z", "tar")),
	b("cb7", "Comic (7-Zip)", Comic, exts(".cb7"), "application/x-cb7", PlayReader, looksLike("7z", "zip", "rar", "tar")),
	b("cbt", "Comic (TAR)", Comic, exts(".cbt"), "application/x-cbt", PlayReader, looksLike("tar", "zip", "rar", "7z")),

	// ---- Books.
	b("epub", "EPUB", Book, exts(".epub", ".kepub"), "application/epub+zip", PlayReader, looksLike("zip")),
	b("pdf", "PDF", Book, exts(".pdf"), "application/pdf", PlayReader),
	b("mobi", "Kindle (MOBI)", Book, exts(".mobi", ".prc"), "application/x-mobipocket-ebook", PlayReader, looksLike("pdb"), note("Copy-protected Kindle books are refused.")),
	b("azw", "Kindle (AZW)", Book, exts(".azw", ".azw1"), "application/vnd.amazon.ebook", PlayReader, looksLike("mobi", "pdb"), note("Copy-protected Kindle books are refused.")),
	b("azw3", "Kindle (KF8)", Book, exts(".azw3", ".kf8"), "application/vnd.amazon.ebook", PlayReader, looksLike("mobi", "pdb"), note("Copy-protected Kindle books are refused.")),
	b("pdb", "Palm eBook", Book, exts(".pdb"), "application/vnd.palm", PlayReader, confirm, looksLike("mobi")),
	b("fb2", "FictionBook", Book, exts(".fb2", ".fbz"), "application/x-fictionbook+xml", PlayReader, looksLike("zip")),
	b("docx", "Word document", Book, exts(".docx"), "application/vnd.openxmlformats-officedocument.wordprocessingml.document", PlayReader, looksLike("zip")),
	b("odt", "OpenDocument text", Book, exts(".odt"), "application/vnd.oasis.opendocument.text", PlayReader, looksLike("zip")),
	b("doc", "Word 97-2003 document", Book, exts(".doc"), "application/msword", PlayReader, looksLike("ole2"), note("Its text is shown, without pictures or layout.")),
	b("rtf", "Rich Text", Book, exts(".rtf"), "application/rtf", PlayReader),
	b("md", "Markdown", Book, exts(".md", ".markdown"), "text/plain; charset=utf-8", PlayReader, confirm, looksLike("webpage")),
	b("html", "Web page", Book, exts(".html", ".htm", ".xhtml"), "text/html", PlayReader, confirm, looksLike("webpage"), note("Scripts and outside links are removed before it is shown.")),
	b("txt", "Plain text", Book, exts(".txt"), "text/plain; charset=utf-8", PlayReader, confirm, looksLike("webpage", "srt", "m3u", "cue", "lrc", "sbv", "microdvd")),
	b("djvu", "DjVu", Book, exts(".djvu", ".djv"), "image/vnd.djvu", PlayReader, needs(needDjVuLibre), note("Pages are drawn by DjVuLibre when it is installed. Without it the book is still listed, and can be downloaded.")),

	// ---- Discs. Video discs and audio CDs; encrypted discs are detected and refused.
	{ID: "iso", Name: "Disc image", Category: Disc, Exts: exts(".iso", ".udf"), MIME: "application/x-iso9660-image", Plays: PlayDiscMenu, Libraries: discLibs, Needs: needFFmpeg, Notes: "DVD, Blu-ray or audio CD. Encrypted discs are refused."},
	{ID: "img", Name: "Disc image", Category: Disc, Exts: exts(".img"), MIME: "application/octet-stream", Plays: PlayDiscMenu, Libraries: discLibs, Needs: needFFmpeg, Confirm: true, SniffAs: []string{"iso", "bin"}, Notes: "DVD, Blu-ray or audio CD."},
	{ID: "bin", Name: "Raw disc image", Category: Disc, Exts: exts(".bin"), MIME: "application/octet-stream", Plays: PlayDiscMenu, Libraries: discLibs, Needs: needFFmpeg, Confirm: true, SniffAs: []string{"iso"}, Notes: "Read through the .cue beside it. An audio CD, or a data image of a DVD or Blu-ray."},
	{ID: "cue", Name: "CUE sheet", Category: Disc, Exts: exts(".cue"), MIME: "text/plain; charset=utf-8", Plays: PlayDiscMenu, Libraries: discLibs, Needs: needNothing, Notes: "Splits one long sound file, or a raw image, into tracks. The tracks of the sheet appear as one album."},
	{ID: "nrg", Name: "Nero image", Category: Disc, Exts: exts(".nrg"), MIME: "application/octet-stream", Plays: PlayDiscMenu, Libraries: discLibs, Needs: needFFmpeg},
	{ID: "mdf", Name: "Alcohol image", Category: Disc, Exts: exts(".mdf"), MIME: "application/octet-stream", Plays: PlayDiscMenu, Libraries: discLibs, Needs: needFFmpeg, SniffAs: []string{"iso", "bin"}, Notes: "Images that keep whole sectors of a DVD. The .mds beside it isn't needed."},
	{ID: "ccd", Name: "CloneCD sheet", Category: Disc, Exts: exts(".ccd"), MIME: "text/plain; charset=utf-8", Plays: PlayDiscMenu, Libraries: []string{"music", "discs"}, Needs: needNothing},
	{ID: "toc", Name: "cdrdao sheet", Category: Disc, Exts: exts(".toc"), MIME: "text/plain; charset=utf-8", Plays: PlayDiscMenu, Libraries: []string{"music", "discs"}, Needs: needNothing, Confirm: true},
	{ID: "chd", Name: "Compressed disc", Category: Disc, Exts: exts(".chd"), MIME: "application/octet-stream", Plays: PlayDiscMenu, Libraries: []string{"music", "discs"}, Needs: needNothing},
	{ID: "video-ts", Name: "DVD folder (VIDEO_TS)", Category: Disc, Exts: nil, Folder: true, Plays: PlayDiscMenu, Libraries: discLibs, Needs: needFFmpeg, Notes: "A folder that holds VIDEO_TS is one disc."},
	{ID: "bdmv", Name: "Blu-ray folder (BDMV)", Category: Disc, Exts: nil, Folder: true, Plays: PlayDiscMenu, Libraries: discLibs, Needs: needFFmpeg, Notes: "A folder that holds BDMV is one disc."},

	// ---- Playlists.
	{ID: "m3u", Name: "M3U playlist", Category: Playlist, Exts: exts(".m3u", ".m3u8"), MIME: "audio/x-mpegurl", Plays: PlaySheet, Libraries: []string{"music"}, Needs: needNothing, Notes: "Becomes a playlist of the songs it names that are in the library. Web addresses in it are ignored."},
	{ID: "pls", Name: "PLS playlist", Category: Playlist, Exts: exts(".pls"), MIME: "audio/x-scpls", Plays: PlaySheet, Libraries: []string{"music"}, Needs: needNothing, Notes: "As M3U."},
	{ID: "xspf", Name: "XSPF playlist", Category: Playlist, Exts: exts(".xspf"), MIME: "application/xspf+xml", Plays: PlaySheet, Libraries: []string{"music"}, Needs: needNothing, Notes: "As M3U."},
	{ID: "asx", Name: "Windows Media playlist", Category: Playlist, Exts: exts(".asx", ".wax", ".wvx", ".wpl"), MIME: "video/x-ms-asf", Plays: PlaySheet, Libraries: []string{"music"}, Needs: needNothing, Notes: "As M3U."},

	// ---- Files read beside the media.
	{ID: "nfo", Name: "NFO details", Category: Sidecar, Exts: exts(".nfo"), MIME: "text/plain; charset=utf-8", Plays: PlaySheet, Libraries: sidecarLibs, Needs: needNothing, Notes: "Titles and details saved beside a film, show or album."},
	{ID: "tbn", Name: "Thumbnail beside a file", Category: Sidecar, Exts: exts(".tbn"), MIME: "image/jpeg", Plays: PlaySheet, Libraries: sidecarLibs, Needs: needNothing, Notes: "Artwork beside a file. So is any picture named poster, fanart, folder, cover or after the file."},
	{ID: "json-meta", Name: "Details in JSON", Category: Sidecar, Exts: exts(".json"), MIME: "application/json", Plays: PlaySheet, Libraries: []string{"movies", "shows", "anime", "videos", "shorts", "channels", "musicvideos"}, Needs: needNothing, Notes: "name.metadata.json, and the .info.json video archivers write."},

	// ---- Recognised so that a file with the wrong name can be told apart.
	{ID: "zip", Name: "ZIP archive", Category: Other, Exts: nil},
	{ID: "rar", Name: "RAR archive", Category: Other, Exts: nil},
	{ID: "7z", Name: "7-Zip archive", Category: Other, Exts: nil},
	{ID: "tar", Name: "TAR archive", Category: Other, Exts: nil},
	{ID: "gzip", Name: "gzip archive", Category: Other, Exts: nil},
	{ID: "ole2", Name: "Microsoft Office file", Category: Other, Exts: nil},
	{ID: "program", Name: "Program", Category: Other, Exts: nil},
	{ID: "webpage", Name: "Web page", Category: Other, Exts: nil},
	{ID: "microdvd", Name: "MicroDVD subtitles", Category: Other, Exts: nil},
	{ID: "subviewer", Name: "SubViewer subtitles", Category: Other, Exts: nil},
}

// codecs: what can be inside. Browser is what a client that reports nothing is assumed
// to decode; the web app reports what its browser really does.
var codecs = []Codec{
	{ID: "h264", Name: "H.264 / AVC", Kind: Video, Browser: true},
	{ID: "hevc", Name: "HEVC / H.265", Kind: Video, Notes: "Plays directly where the browser and machine decode it, otherwise converted."},
	{ID: "av1", Name: "AV1", Kind: Video, Browser: true},
	{ID: "vp9", Name: "VP9", Kind: Video, Browser: true},
	{ID: "vp8", Name: "VP8", Kind: Video, Browser: true},
	{ID: "vvc", Name: "VVC / H.266", Kind: Video, Notes: "Needs an FFmpeg that decodes it."},
	{ID: "mpeg1video", Name: "MPEG-1", Kind: Video},
	{ID: "mpeg2video", Name: "MPEG-2", Kind: Video},
	{ID: "mpeg4", Name: "MPEG-4 ASP (DivX, Xvid)", Kind: Video},
	{ID: "msmpeg4v3", Name: "MS MPEG-4", Kind: Video},
	{ID: "h263", Name: "H.263", Kind: Video},
	{ID: "vc1", Name: "VC-1", Kind: Video},
	{ID: "wmv3", Name: "Windows Media Video 9", Kind: Video},
	{ID: "wmv2", Name: "Windows Media Video 8", Kind: Video},
	{ID: "prores", Name: "ProRes", Kind: Video},
	{ID: "dnxhd", Name: "DNxHD / DNxHR", Kind: Video},
	{ID: "theora", Name: "Theora", Kind: Video},
	{ID: "rv30", Name: "RealVideo 3", Kind: Video},
	{ID: "rv40", Name: "RealVideo 4", Kind: Video},
	{ID: "mjpeg", Name: "Motion JPEG", Kind: Video},
	{ID: "dvvideo", Name: "DV", Kind: Video},
	{ID: "ffv1", Name: "FFV1", Kind: Video},
	{ID: "rawvideo", Name: "Uncompressed", Kind: Video},
	{ID: "flv1", Name: "Sorenson Spark", Kind: Video},

	{ID: "aac", Name: "AAC", Kind: Audio, Browser: true},
	{ID: "mp3", Name: "MP3", Kind: Audio, Browser: true},
	{ID: "mp3float", Name: "MP3", Kind: Audio, Browser: true},
	{ID: "mp2", Name: "MPEG audio layer II", Kind: Audio},
	{ID: "opus", Name: "Opus", Kind: Audio, Browser: true},
	{ID: "vorbis", Name: "Vorbis", Kind: Audio, Browser: true},
	{ID: "flac", Name: "FLAC", Kind: Audio, Browser: true},
	{ID: "alac", Name: "Apple Lossless", Kind: Audio},
	{ID: "ac3", Name: "Dolby Digital", Kind: Audio},
	{ID: "eac3", Name: "Dolby Digital Plus", Kind: Audio},
	{ID: "dts", Name: "DTS and DTS-HD", Kind: Audio},
	{ID: "truehd", Name: "Dolby TrueHD", Kind: Audio},
	{ID: "mlp", Name: "Meridian Lossless", Kind: Audio},
	{ID: "pcm_s16le", Name: "PCM 16-bit", Kind: Audio, Browser: true},
	{ID: "pcm_s24le", Name: "PCM 24-bit", Kind: Audio, Browser: true},
	{ID: "pcm_u8", Name: "PCM 8-bit", Kind: Audio, Browser: true},
	{ID: "pcm_f32le", Name: "PCM 32-bit float", Kind: Audio, Browser: true},
	{ID: "pcm_s16be", Name: "PCM 16-bit, big-endian", Kind: Audio},
	{ID: "pcm_s24be", Name: "PCM 24-bit, big-endian", Kind: Audio},
	{ID: "pcm_bluray", Name: "Blu-ray PCM", Kind: Audio},
	{ID: "pcm_dvd", Name: "DVD PCM", Kind: Audio},
	{ID: "wmav2", Name: "Windows Media Audio", Kind: Audio},
	{ID: "wmapro", Name: "Windows Media Audio Pro", Kind: Audio},
	{ID: "wmalossless", Name: "Windows Media Audio Lossless", Kind: Audio},
	{ID: "ape", Name: "Monkey's Audio", Kind: Audio},
	{ID: "wavpack", Name: "WavPack", Kind: Audio},
	{ID: "tta", Name: "True Audio", Kind: Audio},
	{ID: "musepack7", Name: "Musepack SV7", Kind: Audio},
	{ID: "musepack8", Name: "Musepack SV8", Kind: Audio},
	{ID: "dsd_lsbf", Name: "DSD", Kind: Audio},
	{ID: "dsd_lsbf_planar", Name: "DSD", Kind: Audio},
	{ID: "dsd_msbf", Name: "DSD", Kind: Audio},
	{ID: "dsd_msbf_planar", Name: "DSD", Kind: Audio},
	{ID: "cook", Name: "RealAudio Cooker", Kind: Audio},
	{ID: "amr_nb", Name: "AMR", Kind: Audio},
	{ID: "amr_wb", Name: "AMR wideband", Kind: Audio},
	{ID: "speex", Name: "Speex", Kind: Audio},
	{ID: "midi", Name: "MIDI", Kind: Audio},

	{ID: "subrip", Name: "SubRip", Kind: Subtitle, Text: true},
	{ID: "srt", Name: "SubRip", Kind: Subtitle, Text: true},
	{ID: "ass", Name: "Advanced SubStation", Kind: Subtitle, Text: true},
	{ID: "ssa", Name: "SubStation Alpha", Kind: Subtitle, Text: true},
	{ID: "webvtt", Name: "WebVTT", Kind: Subtitle, Text: true},
	{ID: "mov_text", Name: "MP4 timed text", Kind: Subtitle, Text: true},
	{ID: "text", Name: "Plain text", Kind: Subtitle, Text: true},
	{ID: "microdvd", Name: "MicroDVD", Kind: Subtitle, Text: true},
	{ID: "subviewer", Name: "SubViewer", Kind: Subtitle, Text: true},
	{ID: "subviewer1", Name: "SubViewer", Kind: Subtitle, Text: true},
	{ID: "sami", Name: "SAMI", Kind: Subtitle, Text: true},
	{ID: "realtext", Name: "RealText", Kind: Subtitle, Text: true},
	{ID: "stl", Name: "Spruce subtitles", Kind: Subtitle, Text: true},
	{ID: "vplayer", Name: "VPlayer", Kind: Subtitle, Text: true},
	{ID: "mpl2", Name: "MPL2", Kind: Subtitle, Text: true},
	{ID: "pjs", Name: "Phoenix", Kind: Subtitle, Text: true},
	{ID: "jacosub", Name: "JACOsub", Kind: Subtitle, Text: true},
	{ID: "ttml", Name: "TTML", Kind: Subtitle, Text: true},
	{ID: "dfxp", Name: "DFXP", Kind: Subtitle, Text: true},
	{ID: "eia_608", Name: "CEA-608 captions", Kind: Subtitle, Text: true},
	{ID: "cea608", Name: "CEA-608 captions", Kind: Subtitle, Text: true},
	{ID: "hdmv_pgs_subtitle", Name: "Blu-ray PGS", Kind: Subtitle},
	{ID: "dvd_subtitle", Name: "DVD subtitles", Kind: Subtitle},
	{ID: "dvb_subtitle", Name: "DVB subtitles", Kind: Subtitle},
	{ID: "xsub", Name: "DivX subtitles", Kind: Subtitle},
}
