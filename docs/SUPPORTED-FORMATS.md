# Supported formats

This page is made from the server's own list of formats (`internal/mediatypes`), so it always matches what
the server does. The same list is in the app, under **Settings, About, Supported formats**, where it also says what
your own FFmpeg can do, and at `GET /api/media-types` for modules and apps.

**A file's content decides, not its name.** A file with a missing or wrong extension is recognised from its first
bytes. A name that other things also use, such as `.ts`, is checked the same way. A file whose content clearly isn't
media, like a program named `film.mkv`, is left out of the library and the log says why.

**FFmpeg does the decoding.** Patent-encumbered codecs such as H.264, HEVC and AAC are handled by the FFmpeg on your
machine, as with any program that uses it. Which ones you can use depends on your FFmpeg and where you live.

## Video

A file plays directly when your player can open its container and decode what's inside. If only the container is wrong, the server rewraps it without touching the picture. If only the sound is wrong, it converts the sound alone. The picture is only converted when nothing simpler would play.

| Format | Extensions | How it plays | Needs |
|---|---|---|---|
| MPEG-4 | .mp4, .m4v, .mp4v | As it is, when the codecs inside allow | FFmpeg |
| QuickTime (Sent to browsers as MPEG-4, which is what they play it as) | .mov, .qt | As it is, when the codecs inside allow | FFmpeg |
| WebM | .webm | As it is, when the codecs inside allow | FFmpeg |
| 3GPP (phones) | .3gp, .3gpp | As it is, when the codecs inside allow | FFmpeg |
| 3GPP2 (phones) | .3g2, .3gp2 | As it is, when the codecs inside allow | FFmpeg |
| Flash MP4 | .f4v | As it is, when the codecs inside allow | FFmpeg |
| Matroska | .mkv, .mk3d | Rewrapped, or converted if it has to be | FFmpeg |
| MPEG transport stream | .ts, .m2ts, .mts, .m2t, .tp, .trp | Rewrapped, or converted if it has to be | FFmpeg |
| Flash Video | .flv | Rewrapped, or converted if it has to be | FFmpeg |
| MPEG program stream | .mpg, .mpeg, .mpe, .m2p, .vob, .evo, .vro | Rewrapped, or converted if it has to be | FFmpeg |
| Camcorder MPEG (MOD and TOD files from camcorders. A .mod in a music library is a tracker song instead) | .mod, .tod | Rewrapped, or converted if it has to be | FFmpeg |
| Video CD track (Only when its content really is an MPEG stream) | .dat | Rewrapped, or converted if it has to be | FFmpeg |
| AVI | .avi, .divx, .xvid | Rewrapped, or converted if it has to be | FFmpeg |
| Windows Media Video | .wmv | Converted as it plays | FFmpeg |
| Advanced Systems Format | .asf | Converted as it plays | FFmpeg |
| Windows Media Center recording | .dvr-ms | Converted as it plays | FFmpeg |
| Windows TV recording | .wtv | Converted as it plays | FFmpeg |
| RealMedia | .rm, .rmvb, .rv | Converted as it plays | FFmpeg |
| Ogg video | .ogv, .ogm, .ogx | Converted as it plays | FFmpeg |
| Material Exchange Format | .mxf | Converted as it plays | FFmpeg |
| Digital Video | .dv, .dif | Converted as it plays | FFmpeg |
| NUT | .nut | Converted as it plays | FFmpeg |
| YUV4MPEG | .y4m | Converted as it plays | FFmpeg |
| General Exchange Format | .gxf | Converted as it plays | FFmpeg |
| AMV | .amv | Converted as it plays | FFmpeg |
| Motion JPEG | .mjpeg, .mjpg | Converted as it plays | FFmpeg |
| IVF | .ivf | Converted as it plays | FFmpeg |
| MPEG video stream | .m1v, .m2v, .mpv | Converted as it plays | FFmpeg |
| Raw H.264 | .h264, .264, .avc | Rewrapped, or converted if it has to be | FFmpeg |
| Raw HEVC | .h265, .265, .hevc | Rewrapped, or converted if it has to be | FFmpeg |
| Raw VVC (Needs an FFmpeg that decodes VVC) | .h266, .266, .vvc | Converted as it plays | FFmpeg |
| Raw AV1 | .obu | Rewrapped, or converted if it has to be | FFmpeg |
| Bink Video | .bik, .bk2 | Converted as it plays | FFmpeg |
| Smacker Video | .smk | Converted as it plays | FFmpeg |
| Nullsoft Streaming Video | .nsv | Converted as it plays | FFmpeg |
| FLIC animation | .fli, .flc | Converted as it plays | FFmpeg |
| TiVo recording (Only recordings that aren't encrypted. Encrypted ones are refused) | .ty | Converted as it plays | FFmpeg |

Video codecs the server knows by name: H.264 / AVC, HEVC / H.265, AV1, VP9, VP8, VVC / H.266, MPEG-1, MPEG-2, MPEG-4 ASP (DivX, Xvid), MS MPEG-4, H.263, VC-1, Windows Media Video 9, Windows Media Video 8, ProRes, DNxHD / DNxHR, Theora, RealVideo 3, RealVideo 4, Motion JPEG, DV, FFV1, Uncompressed, Sorenson Spark.

## Music and audio

A browser plays the formats marked "As it is". Everything else is converted to AAC as it plays, for all your devices, with the original left alone.

| Format | Extensions | How it plays | Needs |
|---|---|---|---|
| MP3 | .mp3 | As it is, when the codecs inside allow | FFmpeg |
| MPEG audio layer II | .mp2, .mpa, .m2a | Converted as it plays | FFmpeg |
| AAC | .aac, .adts | As it is, when the codecs inside allow | FFmpeg |
| MPEG-4 audio (Plays in a browser with AAC inside. ALAC is converted) | .m4a, .m4b, .m4r, .alac | As it is, when the codecs inside allow | FFmpeg |
| FLAC | .flac, .fla | As it is, when the codecs inside allow | FFmpeg |
| Ogg audio | .ogg, .oga | As it is, when the codecs inside allow | FFmpeg |
| Opus | .opus | As it is, when the codecs inside allow | FFmpeg |
| Speex | .spx | Converted as it plays | FFmpeg |
| WAV | .wav, .wave, .bwf | As it is, when the codecs inside allow | FFmpeg |
| Sony Wave64 | .w64 | Converted as it plays | FFmpeg |
| RF64 | .rf64 | Converted as it plays | FFmpeg |
| AIFF | .aiff, .aif, .aifc | Converted as it plays | FFmpeg |
| Windows Media Audio | .wma | Converted as it plays | FFmpeg |
| Monkey's Audio | .ape | Converted as it plays | FFmpeg |
| WavPack | .wv | Converted as it plays | FFmpeg |
| True Audio | .tta | Converted as it plays | FFmpeg |
| Musepack | .mpc, .mp+, .mpp | Converted as it plays | FFmpeg |
| TAK | .tak | Converted as it plays | FFmpeg |
| Shorten | .shn | Converted as it plays | FFmpeg |
| DSD (DSF) | .dsf | Converted as it plays | FFmpeg |
| DSD (DFF) | .dff | Converted as it plays | FFmpeg |
| Dolby Digital | .ac3 | Converted as it plays | FFmpeg |
| Dolby Digital Plus | .eac3, .ec3 | Converted as it plays | FFmpeg |
| DTS | .dts, .dtshd, .dtsma | Converted as it plays | FFmpeg |
| Dolby TrueHD | .truehd, .thd | Converted as it plays | FFmpeg |
| Meridian Lossless | .mlp | Converted as it plays | FFmpeg |
| Matroska audio | .mka | Converted as it plays | FFmpeg |
| WebM audio | .weba | As it is, when the codecs inside allow | FFmpeg |
| AMR (voice) | .amr, .awb | Converted as it plays | FFmpeg |
| Sun audio | .au, .snd | Converted as it plays | FFmpeg |
| Core Audio | .caf | Converted as it plays | FFmpeg |
| RealAudio | .ra | Converted as it plays | FFmpeg |
| Creative Voice | .voc | Converted as it plays | FFmpeg |
| GSM voice | .gsm | Converted as it plays | FFmpeg |
| MIDI (Played by the server's built-in synthesiser, which is simple but needs no sound font) | .mid, .midi, .kar, .rmi | Converted as it plays | Nothing extra: the server has its own synthesiser |
| Tracker: ProTracker | .mod | Converted as it plays | An FFmpeg built with libopenmpt |
| Tracker: FastTracker | .xm | Converted as it plays | An FFmpeg built with libopenmpt |
| Tracker: Scream Tracker | .s3m, .stm | Converted as it plays | An FFmpeg built with libopenmpt |
| Tracker: Impulse Tracker | .it, .mptm | Converted as it plays | An FFmpeg built with libopenmpt |
| Tracker: other formats | .669, .mtm, .med, .okt, .ult, .far, .dbm, .ams, .dmf, .psm, .umx, .j2b, .mdl, .ptm, .gdm | Converted as it plays | An FFmpeg built with libopenmpt |
| Game music: Nintendo | .nsf, .nsfe | Converted as it plays | An FFmpeg built with libgme |
| Game music: Super Nintendo | .spc | Converted as it plays | An FFmpeg built with libgme |
| Game music: Game Boy | .gbs | Converted as it plays | An FFmpeg built with libgme |
| Game music: Video Game Music | .vgm, .vgz | Converted as it plays | An FFmpeg built with libgme |
| Game music: Genesis | .gym | Converted as it plays | An FFmpeg built with libgme |
| Game music: PC Engine | .hes | Converted as it plays | An FFmpeg built with libgme |
| Game music: MSX | .kss | Converted as it plays | An FFmpeg built with libgme |
| Game music: Atari | .sap | Converted as it plays | An FFmpeg built with libgme |
| Game music: ZX Spectrum | .ay | Converted as it plays | An FFmpeg built with libgme |

Audio codecs the server knows by name: AAC, MP3, MPEG audio layer II, Opus, Vorbis, FLAC, Apple Lossless, Dolby Digital, Dolby Digital Plus, DTS and DTS-HD, Dolby TrueHD, Meridian Lossless, PCM 16-bit, PCM 24-bit, PCM 8-bit, PCM 32-bit float, PCM 16-bit, big-endian, PCM 24-bit, big-endian, Blu-ray PCM, DVD PCM, Windows Media Audio, Windows Media Audio Pro, Windows Media Audio Lossless, Monkey's Audio, WavPack, True Audio, Musepack SV7, Musepack SV8, DSD, RealAudio Cooker, AMR, AMR wideband, Speex, MIDI.

## Subtitles

Keep subtitle files beside a video with the same name, such as `Film.en.srt` or `Film.en.forced.srt`, or in a `Subs` folder. Text is shown by your player. Pictures from discs are drawn into the video by the server.

| Format | Extensions | How it plays | Needs |
|---|---|---|---|
| SubRip | .srt | As text | FFmpeg |
| Advanced SubStation (Shown as plain text by default, or burned in to keep its styling) | .ass | As text | FFmpeg |
| SubStation Alpha (Shown as plain text by default, or burned in to keep its styling) | .ssa | As text | FFmpeg |
| WebVTT | .vtt, .webvtt | As text | FFmpeg |
| MicroDVD, SubViewer or VobSub (A .sub beside an .idx is VobSub, a picture format, and is burned in. Other .sub files are text) | .sub | As text | FFmpeg |
| VobSub index (Pictures from a DVD, read with the .sub beside it, and burned into the picture) | .idx | Drawn into the picture | FFmpeg |
| Blu-ray PGS (Pictures from a Blu-ray, burned into the picture) | .sup, .pgs | Drawn into the picture | FFmpeg |
| YouTube SubViewer | .sbv | As text | Nothing extra |
| TTML | .ttml, .ttml2 | As text | Nothing extra |
| DFXP | .dfxp | As text | Nothing extra |
| SAMI | .smi, .sami | As text | FFmpeg |
| Timed lyrics (Beside a song, these are its lyrics. Beside a video, a subtitle track) | .lrc | As text | Nothing extra |
| Matroska subtitles | .mks | As text | FFmpeg |
| Scenarist closed captions | .scc | As text | FFmpeg |
| MPL2 | .mpl | As text | FFmpeg |
| Spruce subtitles | .stl | As text | FFmpeg |
| JACOsub | .jss | As text | FFmpeg |
| RealText | .rt | As text | FFmpeg |

Subtitle tracks inside files the server knows by name: SubRip, Advanced SubStation, SubStation Alpha, WebVTT, MP4 timed text, Plain text, MicroDVD, SubViewer, SAMI, RealText, Spruce subtitles, VPlayer, MPL2, Phoenix, JACOsub, TTML, DFXP, CEA-608 captions, Blu-ray PGS, DVD subtitles, DVB subtitles, DivX subtitles.

## Photos

Pictures a browser can't show are sent as a JPEG the server makes, and the original stays untouched. Camera RAW files show the picture the camera stored inside them, so nothing has to be developed.

| Format | Extensions | How it plays | Needs |
|---|---|---|---|
| JPEG (A motion photo's clip, stored after the picture, plays with it) | .jpg, .jpeg, .jpe, .jfif | As it is | FFmpeg |
| PNG | .png | As it is | FFmpeg |
| Animated PNG | .apng | As it is | FFmpeg |
| GIF | .gif | As it is | FFmpeg |
| WebP | .webp | As it is | FFmpeg |
| AVIF | .avif | As it is | FFmpeg |
| BMP | .bmp, .dib | As it is | FFmpeg |
| Icon | .ico, .cur | As it is | FFmpeg |
| SVG (Scripts, outside links and active content are removed before it is sent. FFmpeg with librsvg makes smaller copies when it has it) | .svg, .svgz | As it is | Nothing extra |
| HEIC and HEIF | .heic, .heif, .hif | As a JPEG | An FFmpeg of version 7 or later |
| JPEG XL | .jxl | As a JPEG | An FFmpeg built with libjxl |
| TIFF | .tif, .tiff | As a JPEG | FFmpeg |
| JPEG 2000 | .jp2, .j2k, .jpf, .jpx | As a JPEG | FFmpeg |
| Photoshop | .psd | As a JPEG | FFmpeg |
| Targa | .tga | As a JPEG | FFmpeg |
| OpenEXR | .exr | As a JPEG | FFmpeg |
| Radiance HDR | .hdr | As a JPEG | FFmpeg |
| DirectDraw Surface | .dds | As a JPEG | FFmpeg |
| PCX | .pcx | As a JPEG | FFmpeg |
| Netpbm | .ppm, .pgm, .pbm, .pnm, .pam | As a JPEG | FFmpeg |
| QOI | .qoi | As a JPEG | FFmpeg |
| SGI | .sgi, .rgb | As a JPEG | FFmpeg |
| Canon RAW | .cr2, .crw | As a JPEG | Nothing extra |
| Canon RAW 3 | .cr3 | As a JPEG | Nothing extra |
| Nikon RAW | .nef, .nrw | As a JPEG | Nothing extra |
| Sony RAW | .arw, .srf, .sr2 | As a JPEG | Nothing extra |
| Adobe DNG | .dng | As a JPEG | Nothing extra |
| Fujifilm RAW | .raf | As a JPEG | Nothing extra |
| Olympus RAW | .orf | As a JPEG | Nothing extra |
| Panasonic RAW | .rw2 | As a JPEG | Nothing extra |
| Pentax RAW | .pef | As a JPEG | Nothing extra |
| Samsung RAW | .srw | As a JPEG | Nothing extra |
| Other camera RAW | .3fr, .fff, .erf, .kdc, .dcr, .mef, .mos, .mrw, .iiq, .x3f, .rwl | As a JPEG | Nothing extra |

## Books

Everything here opens in the built-in reader. Formats other than EPUB and PDF are converted to EPUB once, on the server, the first time they're needed, and the original is left alone.

| Format | Extensions | How it plays | Needs |
|---|---|---|---|
| EPUB | .epub, .kepub | In the reader | Nothing extra |
| PDF | .pdf | In the reader | Nothing extra |
| Kindle (MOBI) (Copy-protected Kindle books are refused) | .mobi, .prc | In the reader | Nothing extra |
| Kindle (AZW) (Copy-protected Kindle books are refused) | .azw, .azw1 | In the reader | Nothing extra |
| Kindle (KF8) (Copy-protected Kindle books are refused) | .azw3, .kf8 | In the reader | Nothing extra |
| Palm eBook | .pdb | In the reader | Nothing extra |
| FictionBook | .fb2, .fbz | In the reader | Nothing extra |
| Word document | .docx | In the reader | Nothing extra |
| OpenDocument text | .odt | In the reader | Nothing extra |
| Word 97-2003 document (Its text is shown, without pictures or layout) | .doc | In the reader | Nothing extra |
| Rich Text | .rtf | In the reader | Nothing extra |
| Markdown | .md, .markdown | In the reader | Nothing extra |
| Web page (Scripts and outside links are removed before it is shown) | .html, .htm, .xhtml | In the reader | Nothing extra |
| Plain text | .txt | In the reader | Nothing extra |
| DjVu (Pages are drawn by DjVuLibre when it is installed. Without it the book is still listed, and can be downloaded) | .djvu, .djv | In the reader | The DjVuLibre tools on the server (ddjvu), to open it |

## Comics

Pages can be in any common picture format. The archive is recognised by its contents, so a RAR file named .cbz still opens.

| Format | Extensions | How it plays | Needs |
|---|---|---|---|
| Comic (ZIP) | .cbz | In the reader | Nothing extra |
| Comic (RAR) | .cbr | In the reader | Nothing extra |
| Comic (7-Zip) | .cb7 | In the reader | Nothing extra |
| Comic (TAR) | .cbt | In the reader | Nothing extra |

## Discs

Disc images and folders play with their menus. Encrypted discs are detected and refused with a clear message: nothing here removes copy protection.

| Format | Extensions | How it plays | Needs |
|---|---|---|---|
| Disc image (DVD, Blu-ray or audio CD. Encrypted discs are refused) | .iso, .udf | With its menus | FFmpeg |
| Disc image (DVD, Blu-ray or audio CD) | .img | With its menus | FFmpeg |
| Raw disc image (Read through the .cue beside it. An audio CD, or a data image of a DVD or Blu-ray) | .bin | With its menus | FFmpeg |
| CUE sheet (Splits one long sound file, or a raw image, into tracks. The tracks of the sheet appear as one album) | .cue | With its menus | Nothing extra |
| Nero image | .nrg | With its menus | FFmpeg |
| Alcohol image (Images that keep whole sectors of a DVD. The .mds beside it isn't needed) | .mdf | With its menus | FFmpeg |
| CloneCD sheet | .ccd | With its menus | Nothing extra |
| cdrdao sheet | .toc | With its menus | Nothing extra |
| Compressed disc | .chd | With its menus | Nothing extra |
| DVD folder (VIDEO_TS) (A folder that holds VIDEO_TS is one disc) | a folder | With its menus | FFmpeg |
| Blu-ray folder (BDMV) (A folder that holds BDMV is one disc) | a folder | With its menus | FFmpeg |

## Playlists

Playlists in a music library become collections of the songs they name. Web addresses in them are ignored.

| Format | Extensions | How it plays | Needs |
|---|---|---|---|
| M3U playlist (Becomes a playlist of the songs it names that are in the library. Web addresses in it are ignored) | .m3u, .m3u8 | Read by the scan | Nothing extra |
| PLS playlist (As M3U) | .pls | Read by the scan | Nothing extra |
| XSPF playlist (As M3U) | .xspf | Read by the scan | Nothing extra |
| Windows Media playlist (As M3U) | .asx, .wax, .wvx, .wpl | Read by the scan | Nothing extra |

## Files beside your media

These aren't played. They give titles, details or artwork to what's beside them.

| Format | Extensions | How it plays | Needs |
|---|---|---|---|
| NFO details (Titles and details saved beside a film, show or album) | .nfo | Read by the scan | Nothing extra |
| Thumbnail beside a file (Artwork beside a file. So is any picture named poster, fanart, folder, cover or after the file) | .tbn | Read by the scan | Nothing extra |
| Details in JSON (name.metadata.json, and the .info.json video archivers write) | .json | Read by the scan | Nothing extra |

## Special cases

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
- **Films split across files** (`Film CD1.avi`, `Film CD2.avi`, or `Film.part1.mkv`) are one title that plays straight through.
- **CUE sheets** split one long sound file into the tracks of an album.
- **Live photos and motion photos** play their clip, either beside the picture or inside its file.
- **SVG pictures** lose anything active before they are sent.

---

<sub>WittyMediaServer documentation</sub>
