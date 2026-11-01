package logging

import (
	"regexp"
	"strings"
)

// Explanation is a failure in plain words, with what to do about it.
type Explanation struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	What  string `json:"what"`
	Fix   string `json:"fix"`
}

type explainRule struct {
	match *regexp.Regexp
	Explanation
}

// The failures people run into most. First match wins, so the specific ones come
// first. Keep them short: this is what someone reads when something is broken.
var explainRules = []explainRule{
	{regexp.MustCompile(`(?i)CSS[- ]scrambled|\bAACS\b|\bBD\+|encrypted|copy protect`), Explanation{"encrypted", "This disc is copy-protected",
		"The image or folder is still encrypted. WittyMediaServer never decrypts anything, so it can't play it.",
		"Encrypted media isn't supported, and nothing here can change that. A disc or file without copy protection will play."}},
	{regexp.MustCompile(`(?i)no space left|not enough space|disk full|ENOSPC|quota exceeded`), Explanation{"disk-full", "A disk is full",
		"Something couldn't be written because the disk has no room left.",
		"Free some space, or move the transcode folder to a bigger disk. Settings, WittyDiagnoser shows every folder's free space."}},
	{regexp.MustCompile(`(?i)moov atom not found|invalid data found|could not find codec parameters|end of file.*header|truncat|corrupt|EBML header parsing failed|file format not recognized`), Explanation{"damaged-file", "This file looks damaged",
		"FFmpeg couldn't make sense of the file. It may be incomplete, damaged, or not really a video.",
		"Try the file in another program. If it fails there too, replace it. WittyDiagnoser can check the drive it is on."}},
	{regexp.MustCompile(`(?i)(cannot load|failed to load|error loading).*(nvcuvid|nvenc|amf|libmfx|qsv|vaapi)|no nvenc capable|no capable devices|device creation failed|driver does not support|nvenc.*(error|failed)|hw(?:accel| encoder).*failed|h264_(?:nvenc|qsv|amf|vaapi)`), Explanation{"hw-encoder", "The graphics card's encoder failed",
		"Hardware transcoding was asked for, but the graphics card or its driver wouldn't do it.",
		"Update the graphics driver, or turn off this encoder in Settings, Transcoding. Software transcoding still works."}},
	{regexp.MustCompile(`(?i)unknown encoder|encoder .* not found|decoder .* not found|no decoder|unsupported codec`), Explanation{"missing-codec", "FFmpeg can't handle this format",
		"This build of FFmpeg doesn't include what the file needs.",
		"Use a fuller FFmpeg: Settings, About shows which one is in use and WMS_FFMPEG picks another."}},
	{regexp.MustCompile(`(?i)ffmpeg not found|ffprobe not found|executable file not found|the program was not found`), Explanation{"missing-program", "A program it needs wasn't found",
		"FFmpeg, or another program the server runs, isn't where it was expected.",
		"Keep bin/ffmpeg next to the server, install it on the PATH, or set WMS_FFMPEG."}},
	{regexp.MustCompile(`(?i)database is locked|sqlite_busy`), Explanation{"db-busy", "The database was busy",
		"Too much was writing to the database at once, or another program has it open.",
		"Usually this passes. If it keeps happening, make sure the data folder is on a local disk, not a network drive, and no backup tool is holding it."}},
	{regexp.MustCompile(`(?i)database disk image is malformed|disk i/o error|sqlite_corrupt|sqlite_ioerr`), Explanation{"db-damaged", "The database has a problem",
		"The disk holding the database failed a read or write, or the database file is damaged.",
		"Stop the server and copy the data folder somewhere safe, then run WittyDiagnoser on the data folder."}},
	{regexp.MustCompile(`(?i)out of memory|cannot allocate memory|memory limit|ran out of memory`), Explanation{"memory", "It ran out of memory",
		"The computer didn't have enough free memory for this.",
		"Close other programs, or lower how many transcodes run at once in Settings, Transcoding."}},
	{regexp.MustCompile(`(?i)too many open files|EMFILE`), Explanation{"file-limit", "Too many files open",
		"The system's limit on open files was reached.",
		"Raise the limit (ulimit -n, or LimitNOFILE in the service file) and restart the server."}},
	{regexp.MustCompile(`(?i)address already in use|only one usage of each socket address|bind: `), Explanation{"port-busy", "The port is taken",
		"Another program, or another copy of the server, is already using this port.",
		"Stop the other program, or start this one on another port with -listen."}},
	{regexp.MustCompile(`(?i)signature|files have changed since they were installed|not signed|verification failed|checksum`), Explanation{"verify-failed", "A download or module didn't pass its check",
		"What was downloaded or installed doesn't match its signature, so it wasn't used.",
		"Try again. If it keeps failing, check the connection, and reinstall the module from Settings, Modules."}},
	{regexp.MustCompile(`(?i)(core|cores?\b).*(load|missing|not found|failed|no such)|the runner failed|game runner|wittyplay-host|runner .*(crash|exit|invalid)|libretro|retro_load`), Explanation{"game-runner", "The game couldn't run",
		"The emulator core is missing or stopped, or it doesn't like this game file.",
		"Check the core is installed in Settings, Consoles & cores, and that the game file works in another emulator. The lines above show what the core said."}},
	{regexp.MustCompile(`(?i)tuner|no signal|tune failed|no tuner|channel lock`), Explanation{"tuner", "The tuner had a problem",
		"A tuner didn't answer, is in use, or found no signal on this channel.",
		"Check the tuner is on and reachable, and that another program isn't using it. Settings, Live TV can scan again."}},
	{regexp.MustCompile(`(?i)acme|certificate|x509|tls handshake|lets ?encrypt`), Explanation{"tls", "An HTTPS certificate problem",
		"A secure connection couldn't be made, or the certificate couldn't be renewed.",
		"Check the server's address and that the port is reachable from outside: Settings, Remote Access can test it."}},
	{regexp.MustCompile(`(?i)upnp|nat-?pmp|pcp|port ?map|router`), Explanation{"router", "Your router didn't open the port",
		"Automatic port mapping failed, so the server may not be reachable from outside.",
		"Turn on UPnP on the router or forward the port by hand. Settings, Remote Access has a check."}},
	{regexp.MustCompile(`(?i)account locked|too many (?:failed|attempts|sign)|locked out`), Explanation{"lockout", "Too many failed sign-ins",
		"Someone kept typing the wrong password, so sign-in waits for a while.",
		"Wait a few minutes. If it wasn't you, check who can reach this server."}},
	{regexp.MustCompile(`(?i)permission denied|access is denied|operation not permitted|EACCES|EPERM`), Explanation{"permission", "The server wasn't allowed",
		"The account running the server doesn't have the rights for this file or folder.",
		"Give that account access to the folder. On a NAS or Docker, check the mount's user. WittyDiagnoser checks this for every library."}},
	{regexp.MustCompile(`(?i)no such file|cannot find the (?:file|path)|file not found|not found on disk|ENOENT|path not found`), Explanation{"missing-file", "A file is missing",
		"The file or folder isn't there any more. A disk may be unplugged, or the file moved.",
		"Check the drive is connected, then scan the library again."}},
	{regexp.MustCompile(`(?i)input/output error|i/o error|EIO|data error|cyclic redundancy|bad sector|unreadable`), Explanation{"io-error", "The disk couldn't read this",
		"The drive reported an error reading the data. This can mean it is failing.",
		"Back up what is on that drive soon, and run WittyDiagnoser's storage check on it."}},
	{regexp.MustCompile(`(?i)MEDIA_ERR|media error|hls.*(fatal|error)|manifest(?:load|parsing)|bufferstall|player error|decode error`), Explanation{"player", "The player gave up",
		"The app couldn't load or decode the stream.",
		"Try again, or try lowering the quality. The lines with the same session show what the server was doing at that moment."}},
	{regexp.MustCompile(`(?i)connection refused|no such host|i/o timeout|network is unreachable|connection reset|timeout|deadline exceeded|tls: |EOF$`), Explanation{"network", "A connection failed",
		"Something on the network didn't answer: a website, a tuner or another device.",
		"Check that device and the internet connection. This often passes by itself."}},
	{regexp.MustCompile(`(?i)a part of the server crashed|panic`), Explanation{"crash", "Part of the server crashed",
		"A bug stopped one task. The server caught it and carried on.",
		"Send the support bundle with your bug report. It is in Settings, WittyLog."}},
}

// Explain finds a plain-language explanation for a record, if there is one. It
// looks at the message, and at what an error or a process said.
func Explain(rec *Record) *Explanation {
	var b strings.Builder
	b.WriteString(rec.Msg)
	for i := range rec.Attrs {
		switch rec.Attrs[i].K {
		case "err", "error", "stderr", "reason", "panic", "output", "kind":
			b.WriteByte('\n')
			v := rec.Attrs[i].V
			if len(v) > 8<<10 {
				v = v[len(v)-8<<10:]
			}
			b.WriteString(v)
		}
	}
	text := b.String()
	for i := range explainRules {
		if explainRules[i].match.MatchString(text) {
			e := explainRules[i].Explanation
			return &e
		}
	}
	return nil
}
