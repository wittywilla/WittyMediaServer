package libtranscode

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Output says where a run of FFmpeg writes.
type Output struct {
	Path string // the new file (a temporary name)
	// Exports are subtitle files to write as well, by step, in the plan's order.
	Exports []string
	// Covers are pictures already pulled out of the source, for an MKV to attach.
	Covers []string
}

// Args builds FFmpeg's command line. Everything from the file (track titles, languages)
// goes in as a separate argument, so nothing in a file can become an option, and only
// local files can be opened.
func (pl *Plan) Args(src *Source, out Output) []string {
	args := []string{"-hide_banner", "-nostdin", "-y", "-loglevel", "warning", "-nostats", "-progress", "pipe:1"}
	for _, st := range pl.Steps {
		if st.Kind == "video" && st.encoder != "" {
			args = append(args, st.inputArgs...)
		}
	}
	// Files that arrive with broken times get new ones instead of stopping the job.
	args = append(args, "-fflags", "+genpts", "-analyzeduration", "20M", "-probesize", "20M",
		"-protocol_whitelist", "file,pipe", "-i", src.Path)
	inputs := 1
	embeds := map[string]int{}
	for _, e := range pl.embeds {
		args = append(args, "-protocol_whitelist", "file,pipe", "-i", e.path)
		embeds[e.path] = inputs
		inputs++
	}
	// Burning a subtitle made of pictures needs the filter graph, which names its inputs.
	maps, vout := pl.filterGraph(src, &args)
	var vIdx, aIdx, sIdx, tIdx int
	var body []string
	add := func(a ...string) { body = append(body, a...) }
	for _, st := range pl.Steps {
		switch {
		case st.Action == ActDrop, st.Action == ActBurn, st.Action == ActExport, st.Action == ActLeave, st.Action == ActEmbed:
			continue
		}
		switch st.Kind {
		case "video":
			n := strconv.Itoa(vIdx)
			if st.encoder != "" && maps[st.index] != "" {
				add("-map", maps[st.index])
			} else {
				add("-map", "0:"+strconv.Itoa(st.index))
			}
			if st.encoder == "" {
				add("-c:v:"+n, "copy")
			} else {
				add("-c:v:"+n, st.encoder)
				add(streamArgs(st.args, "v:"+n)...)
				if len(st.filters) > 0 && maps[st.index] == "" {
					add("-filter:v:"+n, strings.Join(st.filters, ","))
				}
				if st.cfr > 0 {
					add("-fps_mode:v:"+n, "cfr", "-r:v:"+n, strconv.FormatFloat(st.cfr, 'f', 3, 64))
				} else {
					add("-fps_mode:v:"+n, "passthrough")
				}
			}
			if st.tag != "" {
				add("-tag:v:"+n, st.tag)
			}
			vIdx++
		case "cover":
			if pl.Container != "mp4" {
				continue // an MKV holds a cover as an attached file: see below
			}
			n := strconv.Itoa(vIdx)
			add("-map", "0:"+strconv.Itoa(st.index), "-c:v:"+n, "copy", "-disposition:v:"+n, "attached_pic")
			vIdx++
		case "audio":
			n := strconv.Itoa(aIdx)
			add("-map", "0:"+strconv.Itoa(st.index))
			if st.encoder == "" {
				add("-c:a:"+n, "copy")
			} else {
				add("-c:a:"+n, st.encoder, "-b:a:"+n, st.bitrate, "-ac:a:"+n, strconv.Itoa(st.channels))
				if st.encoder == "aac" || st.encoder == "libopus" {
					// Odd layouts such as 5.1(side) stop these encoders otherwise.
					add("-filter:a:"+n, "aformat=channel_layouts="+layoutFor(st.channels))
				}
			}
			if st.disposition != "" {
				add("-disposition:a:"+n, st.disposition)
			}
			if st.Action == ActAdd {
				add("-metadata:s:a:"+n, "title="+st.title)
			}
			if st.language != "" {
				add("-metadata:s:a:"+n, "language="+st.language)
			}
			aIdx++
		case "subtitle":
			n := strconv.Itoa(sIdx)
			add("-map", "0:"+strconv.Itoa(st.index))
			if st.encoder == "" {
				add("-c:s:"+n, "copy")
			} else {
				add("-c:s:"+n, st.encoder)
			}
			if st.disposition != "" {
				add("-disposition:s:"+n, st.disposition)
			}
			sIdx++
		case "attachment":
			add("-map", "0:"+strconv.Itoa(st.index), "-c:t", "copy")
			tIdx++
		}
	}
	for _, e := range pl.embeds {
		n := strconv.Itoa(sIdx)
		add("-map", strconv.Itoa(embeds[e.path])+":0")
		if e.encoder == "copy" {
			add("-c:s:"+n, "copy")
		} else {
			add("-c:s:"+n, e.encoder)
		}
		if e.language != "" {
			add("-metadata:s:s:"+n, "language="+e.language)
		}
		if e.disposition != "" {
			add("-disposition:s:"+n, e.disposition)
		}
		sIdx++
	}
	_ = vout
	for i, cover := range out.Covers {
		if i >= len(pl.coverSteps()) {
			break
		}
		st := pl.coverSteps()[i]
		n := strconv.Itoa(tIdx)
		add("-attach", cover, "-metadata:s:t:"+n, "mimetype="+coverMime(st.tag), "-metadata:s:t:"+n, "filename="+coverName(st))
		tIdx++
	}
	args = append(args, body...)
	args = append(args, "-map_metadata", "0", "-map_chapters", "0",
		"-max_muxing_queue_size", "4096", "-max_interleave_delta", "0", "-avoid_negative_ts", "make_zero")
	if pl.Container == "mp4" {
		args = append(args, "-movflags", "+faststart", "-f", "mp4", out.Path)
	} else {
		args = append(args, "-f", "matroska", out.Path)
	}
	for i, ex := range pl.exports {
		if i < len(out.Exports) {
			args = append(args, "-map", "0:"+strconv.Itoa(ex.index), "-c", "copy", "-f", "sup", out.Exports[i])
		}
	}
	return args
}

// filterGraph writes the graph for a burned-in subtitle, onto args, and says which picture
// stream comes out of it under which label.
func (pl *Plan) filterGraph(src *Source, args *[]string) (map[int]string, string) {
	maps := map[int]string{}
	if pl.burn == nil {
		return maps, ""
	}
	for i := range pl.Steps {
		st := &pl.Steps[i]
		if st.Kind != "video" || st.encoder == "" {
			continue
		}
		// The main picture is always the first kept one.
		chain := strings.Join(st.filters, ",")
		var graph string
		if pl.burn.text {
			burn := fmt.Sprintf("subtitles='%s':si=%d", escapeFilterPath(src.Path), pl.burn.subNumber)
			if chain != "" {
				chain = burn + "," + chain
			} else {
				chain = burn
			}
			graph = fmt.Sprintf("[0:%d]%s[vout]", st.index, chain)
		} else {
			// The picture subtitle goes over the source's own size, before any scaling.
			first, rest := splitBeforeScale(st.filters)
			pre := fmt.Sprintf("[0:%d]", st.index)
			if len(first) > 0 {
				graph = fmt.Sprintf("[0:%d]%s[base];", st.index, strings.Join(first, ","))
				pre = "[base]"
			}
			graph += fmt.Sprintf("%s[0:%d]overlay=eof_action=pass:format=auto", pre, pl.burn.index)
			if len(rest) > 0 {
				graph += "," + strings.Join(rest, ",")
			}
			graph += "[vout]"
		}
		*args = append(*args, "-filter_complex", graph)
		maps[st.index] = "[vout]"
		return maps, "[vout]"
	}
	return maps, ""
}

// splitBeforeScale parts the filters into those that run before a subtitle is drawn and
// those after: the size change and upload to the graphics card come last.
func splitBeforeScale(filters []string) (before, after []string) {
	for i, f := range filters {
		if strings.HasPrefix(f, "scale=-2") || strings.HasPrefix(f, "format=nv12") {
			return filters[:i], filters[i:]
		}
	}
	return filters, nil
}

// escapeFilterPath writes a path for a quoted value in a filter graph.
func escapeFilterPath(p string) string {
	return strings.NewReplacer(`\`, `\\\\`, `:`, `\:`, `'`, `'\\\''`).Replace(filepath.ToSlash(p))
}

// coverSteps lists the covers an MKV has to carry as attached files.
func (pl *Plan) coverSteps() []Step {
	if pl.Container == "mp4" {
		return nil
	}
	var out []Step
	for _, st := range pl.Steps {
		if st.Kind == "cover" && st.Action == ActKeep {
			out = append(out, st)
		}
	}
	return out
}

func coverMime(codec string) string {
	if codec == "png" {
		return "image/png"
	}
	return "image/jpeg"
}

func coverExt(codec string) string {
	if codec == "png" {
		return "png"
	}
	return "jpg"
}

func coverName(st Step) string {
	if st.title != "" {
		return filepath.Base(filepath.FromSlash(st.title))
	}
	return "cover." + coverExt(st.tag)
}

// CoverArgs pulls one cover out of a file, as it's stored there.
func CoverArgs(src string, st Step, out string) []string {
	args := []string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error", "-protocol_whitelist", "file,pipe", "-i", src,
		"-map", "0:" + strconv.Itoa(st.index), "-frames:v", "1", "-c", "copy", "-f", "image2", "-update", "1", out}
	return args
}

// streamArgs turns encoder options into ones that apply to one stream.
func streamArgs(args []string, spec string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i+1 < len(args); i += 2 {
		name := args[i]
		switch name {
		case "-profile:v":
			name = "-profile"
		case "-q:v":
			name = "-q"
		case "-b:v":
			name = "-b"
		}
		out = append(out, name+":"+spec, args[i+1])
	}
	return out
}

func layoutFor(ch int) string {
	switch {
	case ch <= 1:
		return "mono"
	case ch == 2:
		return "stereo"
	case ch == 3:
		return "2.1"
	case ch == 4:
		return "quad"
	case ch == 5:
		return "5.0"
	case ch == 6:
		return "5.1"
	case ch == 7:
		return "6.1"
	}
	return "7.1"
}
