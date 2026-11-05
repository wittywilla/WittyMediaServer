package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/themes"
)

// The format table: every format, from a sample made here, through scan, probe,
// artwork, the playback decision and real playback. It's the proof that the registry's
// promises hold on this machine, and it says what happens at each stage.
//
// Set WMS_FORMAT_TABLE to a file to get the table as Markdown. Formats no tool here can
// make a sample of say so rather than pass.

type stage struct{ scan, probe, art, decision, play string }

const (
	sOK = "ok"
	sNA = "-"
)

type formatRow struct {
	id   string // the row's name in the table
	file string // relative to its library's folder
	make func(st *sampleTools, path string) error
	// want is the playback decision for a browser that says nothing about itself.
	want string
	// extra files are checked to have become part of something else.
	check func(p *ffmpeg.Probe) string
	res   stage
	note  string
	made  bool // its sample was made
}

func av(args ...string) func(st *sampleTools, p string) error {
	return func(st *sampleTools, p string) error { return st.ff(p, args...) }
}

func vonly(args ...string) func(st *sampleTools, p string) error {
	return func(st *sampleTools, p string) error { return st.ffvideo(p, args...) }
}

func aonly(args ...string) func(st *sampleTools, p string) error {
	return func(st *sampleTools, p string) error { return st.ffaudio(p, args...) }
}

func still(args ...string) func(st *sampleTools, p string) error {
	return func(st *sampleTools, p string) error { return st.ffimage(p, args...) }
}

func bytesOf(f func() []byte) func(st *sampleTools, p string) error {
	return func(st *sampleTools, p string) error { return os.WriteFile(p, f(), 0o644) }
}

func notSDR(p *ffmpeg.Probe) string {
	if v := p.Video(); v != nil && (v.ColorTransfer == "smpte2084" || v.ColorTransfer == "arib-std-b67") {
		return "the picture is still HDR after tone mapping"
	}
	return ""
}

func eightBit(p *ffmpeg.Probe) string {
	if v := p.Video(); v != nil && strings.Contains(v.PixFmt, "10") {
		return "the picture is still 10-bit"
	}
	return ""
}

func h264v() []string { return []string{"-map", "0:v", "-map", "1:a", "-shortest"} }

var x264 = []string{"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p"}
var x265 = []string{"-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=none"}

func join(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func videoRows() []*formatRow {
	aac := []string{"-c:a", "aac", "-b:a", "64k"}
	mp3 := []string{"-c:a", "libmp3lame", "-b:a", "64k"}
	return []*formatRow{
		{id: "mp4 H.264 AAC", file: "a.mp4", make: av(join(h264v(), x264, aac)...), want: "direct"},
		{id: "m4v H.264 AAC", file: "a.m4v", make: av(join(h264v(), x264, aac, []string{"-f", "mp4"})...), want: "direct"},
		{id: "mov H.264 AAC", file: "a.mov", make: av(join(h264v(), x264, aac)...), want: "direct"},
		{id: "qt H.264 AAC", file: "a.qt", make: av(join(h264v(), x264, aac, []string{"-f", "mov"})...), want: "direct"},
		{id: "webm VP9 Opus", file: "a.webm", make: av(join(h264v(), []string{"-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-c:a", "libopus", "-b:a", "48k"})...), want: "direct"},
		{id: "webm VP8 Vorbis", file: "b.webm", make: av(join(h264v(), []string{"-c:v", "libvpx", "-deadline", "realtime", "-cpu-used", "8", "-c:a", "libvorbis"})...), want: "direct"},
		{id: "webm AV1 Opus", file: "c.webm", make: av(join(h264v(), []string{"-c:v", "libsvtav1", "-preset", "12", "-c:a", "libopus", "-b:a", "48k"})...), want: "direct"},
		{id: "f4v H.264 AAC", file: "a.f4v", make: av(join(h264v(), x264, aac, []string{"-f", "mp4"})...), want: "direct"},
		{id: "3gp H.264 AAC", file: "a.3gp", make: av(join(h264v(), x264, aac, []string{"-ar", "48000"})...), want: "direct"},
		{id: "3g2 H.264 AAC", file: "a.3g2", make: av(join(h264v(), x264, aac)...), want: "direct"},
		{id: "mp4 H.264 AC-3 (audio only converted)", file: "ac3.mp4", make: av(join(h264v(), x264, []string{"-c:a", "ac3"})...), want: "remux+audio"},
		{id: "mkv H.264 AAC (rewrapped)", file: "a.mkv", make: av(join(h264v(), x264, aac)...), want: "remux"},
		{id: "mkv H.264 DTS (audio only converted)", file: "dts.mkv", make: av(join(h264v(), x264, []string{"-c:a", "dca", "-strict", "-2"})...), want: "remux+audio"},
		{id: "mkv H.264 FLAC (audio only converted)", file: "flac.mkv", make: av(join(h264v(), x264, []string{"-c:a", "flac"})...), want: "remux+audio"},
		{id: "mkv H.264 TrueHD (audio only converted)", file: "thd.mkv", make: av(join(h264v(), x264, []string{"-c:a", "truehd", "-strict", "-2"})...), want: "remux+audio"},
		{id: "avi H.264 MP3 (rewrapped)", file: "h264.avi", make: av(join(h264v(), x264, mp3)...), want: "remux"},
		{id: "avi MPEG-4 ASP MP3", file: "a.avi", make: av(join(h264v(), []string{"-c:v", "mpeg4"}, mp3)...), want: "transcode"},
		{id: "divx MPEG-4 ASP MP3", file: "a.divx", make: av(join(h264v(), []string{"-c:v", "mpeg4", "-vtag", "DX50"}, mp3, []string{"-f", "avi"})...), want: "transcode"},
		{id: "xvid MPEG-4 ASP MP3", file: "a.xvid", make: av(join(h264v(), []string{"-c:v", "mpeg4", "-vtag", "XVID"}, mp3, []string{"-f", "avi"})...), want: "transcode"},
		{id: "wmv WMV2 WMA", file: "a.wmv", make: av(join(h264v(), []string{"-c:v", "wmv2", "-c:a", "wmav2"})...), want: "transcode"},
		{id: "asf WMV1 WMA", file: "a.asf", make: av(join(h264v(), []string{"-c:v", "wmv1", "-c:a", "wmav2"})...), want: "transcode"},
		{id: "dvr-ms WMV2 WMA", file: "a.dvr-ms", make: av(join(h264v(), []string{"-c:v", "wmv2", "-c:a", "wmav2", "-f", "asf"})...), want: "transcode"},
		{id: "flv H.264 AAC (rewrapped)", file: "a.flv", make: av(join(h264v(), x264, aac)...), want: "remux"},
		{id: "flv Sorenson MP3", file: "s.flv", make: av(join(h264v(), []string{"-c:v", "flv"}, mp3, []string{"-ar", "44100"})...), want: "transcode"},
		{id: "mpg MPEG-2 MP2", file: "a.mpg", make: av(join(h264v(), []string{"-c:v", "mpeg2video", "-c:a", "mp2"})...), want: "transcode"},
		{id: "mpeg MPEG-1 MP2", file: "a.mpeg", make: av(join(h264v(), []string{"-c:v", "mpeg1video", "-c:a", "mp2", "-f", "mpeg"})...), want: "transcode"},
		{id: "mpe MPEG-2 MP2", file: "a.mpe", make: av(join(h264v(), []string{"-c:v", "mpeg2video", "-c:a", "mp2", "-f", "mpeg"})...), want: "transcode"},
		{id: "m2v raw MPEG-2", file: "a.m2v", make: vonly("-c:v", "mpeg2video", "-f", "mpeg2video"), want: "transcode"},
		{id: "vob MPEG-2 AC-3", file: "a.vob", make: av(join(h264v(), []string{"-c:v", "mpeg2video", "-c:a", "ac3", "-f", "vob"})...), want: "transcode"},
		{id: "ts H.264 AAC (rewrapped)", file: "a.ts", make: av(join(h264v(), x264, aac)...), want: "remux"},
		{id: "m2ts H.264 AC-3 (audio only converted)", file: "a.m2ts", make: av(join(h264v(), x264, []string{"-c:a", "ac3", "-f", "mpegts", "-mpegts_m2ts_mode", "1"})...), want: "remux+audio"},
		{id: "mts H.264 AC-3 (audio only converted)", file: "a.mts", make: av(join(h264v(), x264, []string{"-c:a", "ac3", "-f", "mpegts"})...), want: "remux+audio"},
		{id: "tp H.264 AAC (rewrapped)", file: "a.tp", make: av(join(h264v(), x264, aac, []string{"-f", "mpegts"})...), want: "remux"},
		{id: "trp MPEG-2 MP2", file: "a.trp", make: av(join(h264v(), []string{"-c:v", "mpeg2video", "-c:a", "mp2", "-f", "mpegts"})...), want: "transcode"},
		{id: "ogv Theora Vorbis", file: "a.ogv", make: av(join(h264v(), []string{"-c:v", "libtheora", "-c:a", "libvorbis"})...), want: "transcode"},
		{id: "ogm Theora Vorbis", file: "a.ogm", make: av(join(h264v(), []string{"-c:v", "libtheora", "-c:a", "libvorbis", "-f", "ogg"})...), want: "transcode"},
		{id: "rm RealVideo 2 AC-3", file: "a.rm", make: av(join(h264v(), []string{"-c:v", "rv20", "-c:a", "ac3", "-ar", "44100"})...), want: "transcode"},
		{id: "rmvb RealVideo 1 AC-3", file: "a.rmvb", make: av(join(h264v(), []string{"-c:v", "rv10", "-c:a", "ac3", "-ar", "44100", "-f", "rm"})...), want: "transcode"},
		{id: "mxf MPEG-2 PCM", file: "a.mxf", make: av(join(h264v(), []string{"-c:v", "mpeg2video", "-c:a", "pcm_s16le"})...), want: "transcode"},
		{id: "dv DV video", file: "a.dv", make: func(st *sampleTools, p string) error {
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=720x480:rate=30000/1001:duration=1", "-f", "lavfi", "-i", "sine=frequency=440:duration=1:sample_rate=48000", "-shortest", "-target", "ntsc-dv", p)
		}, want: "transcode"},
		{id: "nut MPEG-4 MP3", file: "a.nut", make: av(join(h264v(), []string{"-c:v", "mpeg4"}, mp3)...), want: "transcode"},
		{id: "y4m uncompressed", file: "a.y4m", make: vonly("-c:v", "wrapped_avframe", "-pix_fmt", "yuv420p", "-f", "yuv4mpegpipe"), want: "transcode"},
		{id: "gxf MPEG-2", file: "a.gxf", make: func(st *sampleTools, p string) error {
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=720x576:rate=25:duration=1", "-c:v", "mpeg2video", "-pix_fmt", "yuv420p", p)
		}, want: "transcode"},
		{id: "wtv MPEG-2 AC-3", file: "a.wtv", make: av(join(h264v(), []string{"-c:v", "mpeg2video", "-c:a", "ac3"})...), want: "transcode"},
		{id: "amv AMV", file: "a.amv", make: func(st *sampleTools, p string) error {
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=160x120:rate=15:duration=1", "-f", "lavfi", "-i", "sine=frequency=440:duration=1:sample_rate=22050", "-shortest", "-c:v", "amv", "-pix_fmt", "yuvj420p", "-c:a", "adpcm_ima_amv", "-ac", "1", "-ar", "22050", "-block_size", "1470", "-strict", "-1", "-f", "amv", p)
		}, want: "transcode"},
		{id: "mjpeg raw", file: "a.mjpeg", make: vonly("-c:v", "mjpeg", "-pix_fmt", "yuvj420p", "-f", "mjpeg"), want: "transcode"},
		{id: "ivf VP9", file: "a.ivf", make: vonly("-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-f", "ivf"), want: "remux"},
		{id: "h264 raw stream", file: "a.h264", make: vonly(join(x264, []string{"-f", "h264"})...), want: "remux"},
		{id: "hevc raw stream", file: "a.hevc", make: vonly(join(x265, []string{"-pix_fmt", "yuv420p", "-f", "hevc"})...), want: "transcode"},
		{id: "obu raw AV1", file: "a.obu", make: vonly("-c:v", "libsvtav1", "-preset", "12", "-f", "obu"), want: "remux"},
		{id: "mkv HEVC AAC", file: "hevc.mkv", make: av(join(h264v(), x265, []string{"-pix_fmt", "yuv420p"}, aac)...), want: "transcode"},
		{id: "mp4 HEVC hvc1 AAC", file: "hevc.mp4", make: av(join(h264v(), x265, []string{"-pix_fmt", "yuv420p", "-tag:v", "hvc1"}, aac)...), want: "transcode"},
		{id: "mkv AV1 Opus (rewrapped)", file: "av1.mkv", make: av(join(h264v(), []string{"-c:v", "libsvtav1", "-preset", "12", "-c:a", "libopus", "-b:a", "48k"})...), want: "remux+audio"},
		{id: "mkv VP9 Opus (rewrapped)", file: "vp9.mkv", make: av(join(h264v(), []string{"-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-c:a", "libopus", "-b:a", "48k"})...), want: "remux+audio"},
		{id: "mov ProRes PCM", file: "prores.mov", make: av(join(h264v(), []string{"-c:v", "prores_ks", "-profile:v", "0", "-pix_fmt", "yuv422p10le", "-c:a", "pcm_s16le"})...), want: "transcode", check: eightBit},
		{id: "mov DNxHD PCM", file: "dnxhd.mov", make: func(st *sampleTools, p string) error {
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=30000/1001:duration=0.3", "-c:v", "dnxhd", "-profile:v", "dnxhr_lb", "-pix_fmt", "yuv422p", p)
		}, want: "transcode"},
		{id: "mkv MPEG-2 AC-3", file: "mpeg2.mkv", make: av(join(h264v(), []string{"-c:v", "mpeg2video", "-c:a", "ac3"})...), want: "transcode"},
		{id: "mkv FFV1 FLAC", file: "ffv1.mkv", make: av(join(h264v(), []string{"-c:v", "ffv1", "-c:a", "flac"})...), want: "transcode"},
		{id: "mkv VVC", file: "vvc.mkv", make: vonly("-c:v", "libvvenc"), want: "transcode"},

		// Edge cases.
		{id: "HDR10 (HEVC 10-bit PQ)", file: "hdr10.mkv", make: vonly(join(x265, []string{"-pix_fmt", "yuv420p10le", "-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc"})...), want: "transcode", check: notSDR},
		{id: "HLG (HEVC 10-bit)", file: "hlg.mkv", make: vonly(join(x265, []string{"-pix_fmt", "yuv420p10le", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc"})...), want: "transcode", check: notSDR},
		{id: "HEVC 12-bit", file: "12bit.mkv", make: vonly(join(x265, []string{"-pix_fmt", "yuv420p12le"})...), want: "transcode", check: eightBit},
		{id: "H.264 10-bit", file: "hi10.mkv", make: vonly("-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le"), want: "transcode", check: eightBit},
		{id: "VP9 10-bit", file: "vp910.webm", make: vonly("-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-pix_fmt", "yuv420p10le", "-profile:v", "2"), want: "direct"},
		{id: "Interlaced H.264", file: "interlaced.mkv", make: vonly("-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-flags", "+ilme+ildct", "-x264-params", "interlaced=1", "-top", "1"), want: "transcode"},
		{id: "Interlaced MPEG-2", file: "interlaced.mpg", make: vonly("-c:v", "mpeg2video", "-flags", "+ilme+ildct", "-top", "1", "-f", "mpeg"), want: "transcode"},
		{id: "Variable frame rate", file: "vfr.mkv", make: vonly(join(x264, []string{"-vf", `setpts=N/25/TB+if(gte(N\,10)\,0.5\,0)/TB`, "-fps_mode", "vfr"})...), want: "remux"},
		{id: "Several video streams", file: "twovideo.mkv", make: func(st *sampleTools, p string) error {
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=1", "-f", "lavfi", "-i", "smptebars=size=320x240:rate=25:duration=1", "-map", "0:v", "-map", "1:v", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", p)
		}, want: "remux"},
		{id: "Attached picture ahead of the video", file: "cover.mkv", make: func(st *sampleTools, p string) error {
			src := p + ".src.mkv"
			if err := st.ff(src, join(h264v(), x264, aac)...); err != nil {
				return err
			}
			defer os.Remove(src)
			art := p + ".art.jpg"
			if err := st.ffimage(art); err != nil {
				return err
			}
			defer os.Remove(art)
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", src, "-attach", art, "-metadata:s:t", "mimetype=image/jpeg", "-metadata:s:t", "filename=cover.jpg", "-map", "0", "-c", "copy", p)
		}, want: "remux"},
		{id: "Cover art stream first (MP4, plays as it is)", file: "cover.mp4", make: func(st *sampleTools, p string) error {
			src := p + ".src.mp4"
			if err := st.ff(src, join(h264v(), x264, aac)...); err != nil {
				return err
			}
			defer os.Remove(src)
			art := p + ".art.png"
			if err := st.ffimage(art); err != nil {
				return err
			}
			defer os.Remove(art)
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", art, "-i", src, "-map", "0", "-map", "1", "-c", "copy", "-disposition:v:0", "attached_pic", p)
		}, want: "direct"},
		{id: "Broken timestamps (joined TS)", file: "broken.ts", make: func(st *sampleTools, p string) error {
			a, b := p+".a.ts", p+".b.ts"
			defer os.Remove(a)
			defer os.Remove(b)
			if err := st.ff(a, join(h264v(), x264, aac)...); err != nil {
				return err
			}
			if err := st.ff(b, join(h264v(), x264, aac)...); err != nil {
				return err
			}
			x, _ := os.ReadFile(a)
			y, _ := os.ReadFile(b)
			return os.WriteFile(p, append(x, y...), 0o644)
		}, want: "remux"},
		{id: "No timestamps at all (raw stream)", file: "raw.m1v", make: vonly("-c:v", "mpeg1video", "-f", "mpeg1video"), want: "transcode"},
		{id: "3D side by side, by its name", file: "Film.3D.HSBS.mkv", make: av(join(h264v(), x264, aac)...), want: "remux"},
		{id: "3D, by its stream's tag", file: "tagged3d.mkv", make: av(join(h264v(), x264, aac, []string{"-metadata:s:v", "stereo_mode=top_bottom"})...), want: "remux"},
		{id: "HDR10+ (plays its HDR10 layer)", file: "hdr10plus.mkv", make: func(st *sampleTools, p string) error {
			meta := p + ".json"
			if err := os.WriteFile(meta, []byte(`{"SceneInfo":[{"BezierCurveData":{"Anchors":[1,2,3,4,5,6,7,8,9],"KneePointX":1,"KneePointY":1},"LuminanceParameters":{"AverageRGB":1,"LuminanceDistributions":{"DistributionIndex":[1,5,10,25,50,75,90,95,99],"DistributionValues":[1,1,1,1,1,1,1,1,1]},"MaxScl":[1,1,1]},"NumberOfWindows":1,"TargetedSystemDisplayMaximumLuminance":1000,"SceneFrameIndex":0,"SceneId":0,"SequenceFrameIndex":0}],"SeqInfo":{"JSONInfo":{"HDR10plusProfile":"B","Version":"1.0"},"NumberOfScenes":1}}`), 0o644); err != nil {
				return err
			}
			defer os.Remove(meta)
			return st.ffvideo(p, "-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-x265-params", "log-level=error:hdr10=1:dhdr10-info="+meta+":repeat-headers=1", "-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc")
		}, want: "transcode", check: notSDR},
		{id: "Dolby Vision", file: "dv.mkv", make: func(st *sampleTools, p string) error {
			return errors.New("no encoder here writes its side data; decisions are checked in TestDolbyVisionRange")
		}, want: "transcode"},
		{id: "VC-1", file: "vc1.mkv", make: func(st *sampleTools, p string) error {
			return errors.New("no VC-1 encoder exists in FFmpeg; its decoder is checked in TestCodecsDecoders")
		}, want: "transcode"},
		{id: "RealVideo 3 and 4", file: "rv40.rm", make: func(st *sampleTools, p string) error {
			return errors.New("no encoder exists in FFmpeg; its decoders are checked in TestCodecsDecoders")
		}, want: "transcode"},
		{id: "Sound only, in a video library", file: "soundonly.mp4", make: aonly("-c:a", "aac"), want: "transcode"},
		{id: "Name lies: a Matroska file called .mp4", file: "lies.mp4", make: av(join(h264v(), x264, aac, []string{"-f", "matroska"})...), want: "remux"},
		{id: "No extension at all", file: "noextension", make: av(join(h264v(), x264, aac, []string{"-f", "matroska"})...), want: "remux"},
		{id: "Unknown extension", file: "video.xyz", make: av(join(h264v(), x264, aac, []string{"-f", "mp4"})...), want: "direct"},
		{id: "Split film, CD1 and CD2", file: "Split Film CD1.avi", make: func(st *sampleTools, p string) error {
			if err := st.ff(p, join(h264v(), []string{"-c:v", "mpeg4", "-c:a", "libmp3lame"})...); err != nil {
				return err
			}
			return st.ff(strings.Replace(p, "CD1", "CD2", 1), join(h264v(), []string{"-c:v", "mpeg4", "-c:a", "libmp3lame"})...)
		}, want: "transcode"},
		{id: "Split film, part1 and part2", file: "Other Film.part1.mkv", make: func(st *sampleTools, p string) error {
			if err := st.ff(p, join(h264v(), x264, aac)...); err != nil {
				return err
			}
			return st.ff(strings.Replace(p, "part1", "part2", 1), join(h264v(), x264, aac)...)
		}, want: "remux"},
	}
}

func audioRows() []*formatRow {
	return []*formatRow{
		{id: "mp3", file: "a.mp3", make: aonly("-c:a", "libmp3lame"), want: "audio-direct"},
		{id: "mp3 with cover art", file: "Cover Album/a.mp3", make: func(st *sampleTools, p string) error {
			art := p + ".png"
			if err := st.ffimage(art); err != nil {
				return err
			}
			defer os.Remove(art)
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-i", art, "-map", "0", "-map", "1", "-c:a", "libmp3lame", "-c:v", "mjpeg", "-disposition:v", "attached_pic", p)
		}, want: "audio-direct"},
		{id: "m4a AAC", file: "a.m4a", make: aonly("-c:a", "aac"), want: "audio-direct"},
		{id: "m4b AAC", file: "a.m4b", make: aonly("-c:a", "aac", "-f", "ipod"), want: "audio-direct"},
		{id: "aac ADTS", file: "a.aac", make: aonly("-c:a", "aac", "-f", "adts"), want: "audio-direct"},
		{id: "flac", file: "a.flac", make: aonly("-c:a", "flac"), want: "audio-direct"},
		{id: "alac in m4a", file: "alac.m4a", make: aonly("-c:a", "alac"), want: "audio-transcode"},
		{id: "ogg Vorbis", file: "a.ogg", make: aonly("-c:a", "libvorbis"), want: "audio-direct"},
		{id: "oga FLAC", file: "a.oga", make: aonly("-c:a", "flac", "-f", "ogg"), want: "audio-direct"},
		{id: "opus", file: "a.opus", make: aonly("-c:a", "libopus"), want: "audio-direct"},
		{id: "spx Speex", file: "a.spx", make: aonly("-c:a", "libspeex", "-ar", "16000", "-f", "ogg"), want: "audio-transcode"},
		{id: "wav 16-bit", file: "a.wav", make: aonly("-c:a", "pcm_s16le"), want: "audio-direct"},
		{id: "wav 24-bit", file: "b.wav", make: aonly("-c:a", "pcm_s24le"), want: "audio-direct"},
		{id: "aiff", file: "a.aiff", make: aonly("-c:a", "pcm_s16be"), want: "audio-transcode"},
		{id: "aif", file: "a.aif", make: aonly("-c:a", "pcm_s16be", "-f", "aiff"), want: "audio-transcode"},
		{id: "wma", file: "a.wma", make: aonly("-c:a", "wmav2", "-f", "asf"), want: "audio-transcode"},
		{id: "ape Monkey's Audio", file: "a.ape", make: func(st *sampleTools, p string) error { return errors.New("no encoder for it here; FFmpeg decodes it") }, want: "audio-transcode"},
		{id: "wv WavPack", file: "a.wv", make: aonly("-c:a", "wavpack"), want: "audio-transcode"},
		{id: "tta True Audio", file: "a.tta", make: aonly("-c:a", "tta"), want: "audio-transcode"},
		{id: "mpc Musepack", file: "a.mpc", make: func(st *sampleTools, p string) error {
			wav := p + ".wav"
			if err := st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=1:sample_rate=44100", "-ac", "2", wav); err != nil {
				return err
			}
			defer os.Remove(wav)
			return st.run("mpcenc", "--silent", "--overwrite", wav, p)
		}, want: "audio-transcode"},
		{id: "dsf DSD", file: "a.dsf", make: bytesOf(dsfBytes), want: "audio-transcode"},
		{id: "dff DSD", file: "a.dff", make: bytesOf(dffBytes), want: "audio-transcode"},
		{id: "ac3", file: "a.ac3", make: aonly("-c:a", "ac3"), want: "audio-transcode"},
		{id: "eac3", file: "a.eac3", make: aonly("-c:a", "eac3"), want: "audio-transcode"},
		{id: "dts", file: "a.dts", make: aonly("-c:a", "dca", "-strict", "-2", "-f", "dts"), want: "audio-transcode"},
		{id: "truehd", file: "a.truehd", make: aonly("-c:a", "truehd", "-strict", "-2", "-f", "truehd"), want: "audio-transcode"},
		{id: "mlp", file: "a.mlp", make: aonly("-c:a", "mlp", "-strict", "-2", "-f", "mlp"), want: "audio-transcode"},
		{id: "mka FLAC", file: "a.mka", make: aonly("-c:a", "flac"), want: "audio-transcode"},
		{id: "weba Opus", file: "a.weba", make: aonly("-c:a", "libopus", "-f", "webm"), want: "audio-direct"},
		{id: "mp2", file: "a.mp2", make: aonly("-c:a", "mp2"), want: "audio-transcode"},
		{id: "au", file: "a.au", make: aonly("-c:a", "pcm_mulaw", "-ar", "8000"), want: "audio-transcode"},
		{id: "caf", file: "a.caf", make: aonly("-c:a", "pcm_s16le"), want: "audio-transcode"},
		{id: "voc", file: "a.voc", make: aonly("-c:a", "pcm_u8", "-ar", "22050", "-ac", "1"), want: "audio-transcode"},
		{id: "gsm", file: "a.gsm", make: aonly("-c:a", "libgsm", "-ar", "8000", "-ac", "1", "-f", "gsm"), want: "audio-transcode"},
		{id: "w64", file: "a.w64", make: aonly("-c:a", "pcm_s16le"), want: "audio-transcode"},
		{id: "rf64", file: "a.rf64", make: aonly("-c:a", "pcm_s16le", "-rf64", "always", "-f", "wav"), want: "audio-transcode"},
		{id: "mid MIDI", file: "a.mid", make: bytesOf(midiBytes), want: "audio-transcode"},
		{id: "midi MIDI", file: "a.midi", make: bytesOf(midiBytes), want: "audio-transcode"},
		{id: "mod tracker", file: "a.mod", make: bytesOf(modBytes), want: "audio-transcode"},
		{id: "xm tracker", file: "a.xm", make: bytesOf(xmBytes), want: "audio-transcode"},
		{id: "s3m tracker", file: "a.s3m", make: bytesOf(s3mBytes), want: "audio-transcode"},
		{id: "it tracker", file: "a.it", make: bytesOf(itBytes), want: "audio-transcode"},
		{id: "spc game music", file: "a.spc", make: bytesOf(spcBytes), want: "audio-transcode"},
		{id: "vgm game music", file: "a.vgm", make: bytesOf(vgmBytes), want: "audio-transcode"},
		{id: "nsf game music", file: "a.nsf", make: bytesOf(nsfBytes), want: "audio-transcode"},
		{id: "CUE sheet over one FLAC", file: "cueflac/Album.cue", make: func(st *sampleTools, p string) error {
			audio := strings.TrimSuffix(p, ".cue") + ".flac"
			if err := st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=2:sample_rate=44100", "-ac", "2", "-sample_fmt", "s16", audio); err != nil {
				return err
			}
			return cueFor(p, audio, "WAVE")
		}, want: "cd"},
		{id: "CUE sheet over one WAV", file: "cuewav/Album.cue", make: func(st *sampleTools, p string) error {
			audio := strings.TrimSuffix(p, ".cue") + ".wav"
			if err := st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=2:sample_rate=44100", "-ac", "2", "-c:a", "pcm_s16le", audio); err != nil {
				return err
			}
			return cueFor(p, audio, "WAVE")
		}, want: "cd"},
		{id: "CUE sheet over one MP3", file: "cuemp3/Album.cue", make: func(st *sampleTools, p string) error {
			audio := strings.TrimSuffix(p, ".cue") + ".mp3"
			if err := st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=2:sample_rate=44100", "-ac", "2", "-c:a", "libmp3lame", audio); err != nil {
				return err
			}
			return cueFor(p, audio, "MP3")
		}, want: "cd"},
		{id: "BIN and CUE audio CD", file: "cuebin/Album.cue", make: func(st *sampleTools, p string) error {
			audio := strings.TrimSuffix(p, ".cue") + ".bin"
			if err := st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=2.4:sample_rate=44100", "-ac", "2", "-f", "s16le", "-c:a", "pcm_s16le", audio); err != nil {
				return err
			}
			return cueFor(p, audio, "BINARY")
		}, want: "cd"},
		{id: "Sound with no extension", file: "soundfile", make: aonly("-c:a", "libmp3lame", "-b:a", "128k", "-f", "mp3"), want: "audio-direct"},
	}
}

func photoRows() []*formatRow {
	tiffRaw := func(name string, magic []byte, extra []byte) *formatRow {
		return &formatRow{id: "RAW " + strings.TrimPrefix(filepath.Ext(name), "."), file: name, make: func(st *sampleTools, p string) error {
			return os.WriteFile(p, rawTIFF(st.t, magic, extra), 0o644)
		}, want: "jpeg"}
	}
	le := []byte("II*\x00")
	return []*formatRow{
		{id: "jpg", file: "jpg.jpg", make: still(), want: "original"},
		{id: "jpeg", file: "sjpeg.jpeg", make: still("-f", "image2", "-c:v", "mjpeg"), want: "original"},
		{id: "png", file: "spng.png", make: still(), want: "original"},
		{id: "apng", file: "sapng.apng", make: still("-f", "apng"), want: "original"},
		{id: "gif", file: "sgif.gif", make: still(), want: "original"},
		{id: "webp", file: "swebp.webp", make: still("-c:v", "libwebp"), want: "original"},
		{id: "avif", file: "savif.avif", make: still("-c:v", "libaom-av1", "-still-picture", "1", "-cpu-used", "8", "-pix_fmt", "yuv420p"), want: "original"},
		{id: "bmp", file: "sbmp.bmp", make: still(), want: "original"},
		{id: "ico", file: "sico.ico", make: func(st *sampleTools, p string) error {
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=1:duration=1", "-frames:v", "1", "-pix_fmt", "bgra", p)
		}, want: "original"},
		{id: "heic", file: "sheic.heic", make: func(st *sampleTools, p string) error {
			png := p + ".png"
			if err := st.ffimage(png); err != nil {
				return err
			}
			defer os.Remove(png)
			return st.run("heif-enc", "-q", "50", "-o", p, png)
		}, want: "jpeg"},
		{id: "heif", file: "sheif.heif", make: func(st *sampleTools, p string) error {
			png := p + ".png"
			if err := st.ffimage(png); err != nil {
				return err
			}
			defer os.Remove(png)
			return st.run("heif-enc", "-q", "50", "-o", p, png)
		}, want: "jpeg"},
		{id: "jxl", file: "sjxl.jxl", make: still("-c:v", "libjxl"), want: "jpeg"},
		{id: "tiff", file: "stiff.tiff", make: still(), want: "jpeg"},
		{id: "tif", file: "stif.tif", make: still("-f", "image2", "-c:v", "tiff"), want: "jpeg"},
		{id: "jp2", file: "sjp2.jp2", make: still(), want: "jpeg"},
		{id: "tga", file: "stga.tga", make: still("-c:v", "targa"), want: "jpeg"},
		{id: "exr", file: "sexr.exr", make: still("-pix_fmt", "gbrpf32le"), want: "jpeg"},
		{id: "hdr", file: "shdr.hdr", make: still("-c:v", "hdr", "-pix_fmt", "gbrpf32le"), want: "jpeg"},
		{id: "psd", file: "spsd.psd", make: func(st *sampleTools, p string) error {
			png := p + ".png"
			if err := st.ffimage(png); err != nil {
				return err
			}
			defer os.Remove(png)
			return st.run("convert", png, "-compress", "none", p)
		}, want: "jpeg"},
		{id: "dds", file: "sdds.dds", make: func(st *sampleTools, p string) error {
			png := p + ".png"
			if err := st.ffimage(png); err != nil {
				return err
			}
			defer os.Remove(png)
			return st.run("convert", png, p)
		}, want: "jpeg"},
		{id: "pcx", file: "spcx.pcx", make: still("-c:v", "pcx", "-pix_fmt", "rgb24"), want: "jpeg"},
		{id: "ppm", file: "sppm.ppm", make: still(), want: "jpeg"},
		{id: "qoi", file: "sqoi.qoi", make: still("-c:v", "qoi", "-f", "image2"), want: "jpeg"},
		{id: "sgi", file: "ssgi.sgi", make: still("-c:v", "sgi", "-f", "image2"), want: "jpeg"},
		{id: "svg (sanitised)", file: "ssvg.svg", make: func(st *sampleTools, p string) error {
			return os.WriteFile(p, []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="64" height="48" onload="alert(1)"><script>alert(2)</script><rect width="64" height="48" fill="red"/><image href="http://evil.test/x.png"/></svg>`), 0o644)
		}, want: "svg"},
		{id: "svgz (sanitised)", file: "b.svgz", make: func(st *sampleTools, p string) error {
			src := p + ".svg"
			if err := os.WriteFile(src, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 48"><circle cx="32" cy="24" r="10" fill="blue"/></svg>`), 0o644); err != nil {
				return err
			}
			defer os.Remove(src)
			if err := st.run("gzip", "-kf", src); err != nil {
				return err
			}
			return os.Rename(src+".gz", p)
		}, want: "svg"},
		tiffRaw("raw-cr2.cr2", le, []byte("CR\x02\x00\x00\x00\x00\x00")),
		tiffRaw("raw-nef.nef", le, nil),
		tiffRaw("raw-arw.arw", le, nil),
		tiffRaw("raw-dng.dng", le, nil),
		tiffRaw("raw-orf.orf", []byte("IIRO"), nil),
		tiffRaw("raw-rw2.rw2", []byte("IIU\x00"), nil),
		tiffRaw("raw-pef.pef", le, nil),
		tiffRaw("raw-srw.srw", le, nil),
		{id: "RAW raf", file: "sraf.raf", make: func(st *sampleTools, p string) error { return os.WriteFile(p, rawRAF(st.t), 0o644) }, want: "jpeg"},
		{id: "RAW cr3", file: "scr3.cr3", make: func(st *sampleTools, p string) error { return os.WriteFile(p, rawCR3(st.t), 0o644) }, want: "jpeg"},
		{id: "Live photo (HEIC and MOV)", file: "live/Live.heic", make: func(st *sampleTools, p string) error {
			png := p + ".png"
			if err := st.ffimage(png); err != nil {
				return err
			}
			defer os.Remove(png)
			if err := st.run("heif-enc", "-q", "50", "-o", p, png); err != nil {
				return err
			}
			return st.ff(strings.TrimSuffix(p, ".heic")+".mov", join(h264v(), x264, []string{"-c:a", "aac"})...)
		}, want: "motion"},
		{id: "Motion photo (clip inside the JPEG)", file: "motion/Motion.jpg", make: func(st *sampleTools, p string) error {
			clip := p + ".mp4"
			if err := st.ff(clip, join(h264v(), x264, []string{"-c:a", "aac"})...); err != nil {
				return err
			}
			defer os.Remove(clip)
			c, err := os.ReadFile(clip)
			if err != nil {
				return err
			}
			// A comment segment says what it is, as a phone's XMP would.
			j := smallJPEGBytes(st.t)
			mark := []byte("GCamera:MicroVideo")
			seg := append([]byte{0xFF, 0xFE, 0, byte(len(mark) + 2)}, mark...)
			out := append(append(append([]byte{}, j[:2]...), seg...), j[2:]...)
			return os.WriteFile(p, append(out, c...), 0o644)
		}, want: "motion"},
		{id: "Picture with no extension", file: "picturefile", make: still("-f", "image2", "-c:v", "png", "-update", "1"), want: "jpeg"},
	}
}

func bookRows() []*formatRow {
	return []*formatRow{
		{id: "cbz", file: "scbz.cbz", make: func(st *sampleTools, p string) error { return st.cbz(p) }, want: "cbz"},
		{id: "cbr", file: "scbr.cbr", make: func(st *sampleTools, p string) error { return st.cbr(p) }, want: "cbz"},
		{id: "cb7", file: "scb7.cb7", make: func(st *sampleTools, p string) error { return st.cb7(p) }, want: "cbz"},
		{id: "cbt", file: "scbt.cbt", make: func(st *sampleTools, p string) error { return st.cbt(p) }, want: "cbz"},
		{id: "Name lies: a RAR called .cbz", file: "rar.cbz", make: func(st *sampleTools, p string) error { return st.cbr(p) }, want: "cbz"},
		{id: "Name lies: a ZIP called .cbr", file: "zip.cbr", make: func(st *sampleTools, p string) error { return st.cbz(p) }, want: "cbz"},
		{id: "zip of pictures", file: "pages.zip", make: func(st *sampleTools, p string) error { return st.cbz(p) }, want: "cbz"},
		{id: "epub", file: "sepub.epub", make: func(st *sampleTools, p string) error { return st.pandoc(p) }, want: "epub"},
		{id: "mobi", file: "smobi.mobi", make: func(st *sampleTools, p string) error { return st.calibre(p) }, want: "epub"},
		{id: "azw", file: "b.azw", make: func(st *sampleTools, p string) error {
			m := p + ".mobi"
			if err := st.calibre(m); err != nil {
				return err
			}
			return os.Rename(m, p)
		}, want: "epub"},
		{id: "azw3", file: "sazw3.azw3", make: func(st *sampleTools, p string) error { return st.calibre(p) }, want: "epub"},
		{id: "kf8", file: "c.kf8", make: func(st *sampleTools, p string) error {
			m := p + ".azw3"
			if err := st.calibre(m); err != nil {
				return err
			}
			return os.Rename(m, p)
		}, want: "epub"},
		{id: "prc", file: "sprc.prc", make: func(st *sampleTools, p string) error {
			m := p + ".mobi"
			if err := st.calibre(m); err != nil {
				return err
			}
			return os.Rename(m, p)
		}, want: "epub"},
		{id: "pdb Palm", file: "spdb.pdb", make: bytesOf(palmDocBytes), want: "epub"},
		{id: "fb2", file: "sfb2.fb2", make: func(st *sampleTools, p string) error { return st.pandoc(p) }, want: "epub"},
		{id: "docx", file: "sdocx.docx", make: func(st *sampleTools, p string) error { return st.pandoc(p) }, want: "epub"},
		{id: "odt", file: "sodt.odt", make: func(st *sampleTools, p string) error { return st.pandoc(p) }, want: "epub"},
		{id: "doc", file: "sdoc.doc", make: func(st *sampleTools, p string) error { return st.soffice(p, "doc") }, want: "epub"},
		{id: "rtf", file: "srtf.rtf", make: func(st *sampleTools, p string) error { return st.pandoc(p) }, want: "epub"},
		{id: "html", file: "shtml.html", make: func(st *sampleTools, p string) error { return st.pandoc(p, "--to", "html") }, want: "epub"},
		{id: "md", file: "smd.md", make: func(st *sampleTools, p string) error {
			return os.WriteFile(p, []byte(strings.Repeat(bookSource(), 8)), 0o644)
		}, want: "epub"},
		{id: "txt", file: "stxt.txt", make: func(st *sampleTools, p string) error {
			return os.WriteFile(p, []byte(strings.Repeat(sampleBook, 40)), 0o644)
		}, want: "epub"},
		{id: "pdf", file: "spdf.pdf", make: func(st *sampleTools, p string) error { return st.calibre(p) }, want: "pdf"},
		{id: "djvu", file: "sdjvu.djvu", make: func(st *sampleTools, p string) error { return st.djvu(p) }, want: "cbz"},
	}
}

func discRows() []*formatRow {
	return []*formatRow{
		{id: "VIDEO_TS folder", file: "Folder Disc", make: func(st *sampleTools, p string) error {
			src, err := st.dvdFolder("dvdsrc-folder")
			if err != nil {
				return err
			}
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			return os.Rename(filepath.Join(src, "VIDEO_TS"), filepath.Join(p, "VIDEO_TS"))
		}, want: "disc"},
		{id: "iso DVD", file: "a.iso", make: func(st *sampleTools, p string) error { return st.dvdISO(p) }, want: "disc"},
		{id: "img DVD", file: "b.img", make: func(st *sampleTools, p string) error { return st.dvdISO(p) }, want: "disc"},
		{id: "mdf DVD", file: "c.mdf", make: func(st *sampleTools, p string) error { return st.dvdISO(p) }, want: "disc"},
		{id: "nrg DVD", file: "d.nrg", make: func(st *sampleTools, p string) error { return st.dvdISO(p) }, want: "disc"},
		{id: "bin DVD (data sectors)", file: "e.bin", make: func(st *sampleTools, p string) error { return st.dvdISO(p) }, want: "disc"},
		{id: "Encrypted DVD (refused)", file: "scrambled.iso", make: func(st *sampleTools, p string) error {
			if err := st.dvdISO(p); err != nil {
				return err
			}
			return scrambleVOBs(p)
		}, want: "refused"},
	}
}

// A subtitle row names one video and one subtitle file beside it.
type subtitleRow struct {
	id, name string
	make     func(st *sampleTools, path string) error
	burn     bool // pictures: the server draws it
	res      stage
	note     string
	video    bool // a file beside a video, rather than a track inside one
	made     bool // its sample was made
}

func text(s string) func(st *sampleTools, p string) error {
	return func(st *sampleTools, p string) error { return os.WriteFile(p, []byte(s), 0o644) }
}

func subtitleRows() []*subtitleRow {
	srt := "1\n00:00:00,100 --> 00:00:00,900\nHello from a made-up film\n\n"
	return []*subtitleRow{
		{id: "srt", name: "Clip.en.srt", make: text(srt)},
		{id: "srt in Windows-1252", name: "Clip.fr.srt", make: text("1\n00:00:00,100 --> 00:00:00,900\nCaf\xE9 cr\xE8me\n\n")},
		{id: "srt in UTF-16", name: "Clip.de.srt", make: func(st *sampleTools, p string) error {
			out := []byte{0xFF, 0xFE}
			for _, r := range srt {
				out = append(out, byte(r), byte(r>>8))
			}
			return os.WriteFile(p, out, 0o644)
		}},
		{id: "ass", name: "Clip.ass", make: text("[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize\nStyle: Default,Arial,20\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:00.10,0:00:00.90,Default,,0,0,0,,Hello from a made-up film\n")},
		{id: "ssa", name: "Clip.ssa", make: text("[Script Info]\nScriptType: v4.00\n\n[V4 Styles]\nFormat: Name, Fontname, Fontsize\nStyle: Default,Arial,20\n\n[Events]\nFormat: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: Marked=0,0:00:00.10,0:00:00.90,Default,,0,0,0,,Hello from a made-up film\n")},
		{id: "vtt", name: "Clip.vtt", make: text("WEBVTT\n\n00:00.100 --> 00:00.900\nHello from a made-up film\n")},
		{id: "sub (MicroDVD)", name: "Clip.sub", make: text("{1}{1}25.000\n{3}{22}Hello from a made-up film\n{25}{40}And a second line\n{50}{60}And a third\n{70}{80}And a fourth\n")},
		{id: "sub (SubViewer)", name: "Clip2.sub", make: text("[INFORMATION]\n[END INFORMATION]\n00:00:00.10,00:00:00.90\nHello from a made-up film\n")},
		{id: "sbv", name: "Clip.sbv", make: text("0:00:00.100,0:00:00.900\nHello from a made-up film\n\n")},
		{id: "ttml", name: "Clip.ttml", make: text(`<?xml version="1.0"?><tt xmlns="http://www.w3.org/ns/ttml"><body><div><p begin="0.1s" end="0.9s">Hello from a made-up film</p></div></body></tt>`)},
		{id: "dfxp", name: "Clip.dfxp", make: text(`<tt xmlns="http://www.w3.org/2006/04/ttaf1"><body><div><p begin="00:00:00.10" end="00:00:00.90">Hello from a <br/>made-up film</p></div></body></tt>`)},
		{id: "smi", name: "Clip.smi", make: text("<SAMI><BODY><SYNC Start=100><P Class=ENCC>Hello from a made-up film<SYNC Start=900><P Class=ENCC>&nbsp;</BODY></SAMI>\n")},
		{id: "lrc", name: "Clip.lrc", make: text("[ti:Made Up]\n[00:00.10]Hello from a made-up film\n[00:00.90]\n")},
		{id: "mks", name: "Clip.mks", make: func(st *sampleTools, p string) error {
			src := p + ".srt"
			if err := os.WriteFile(src, []byte(srt), 0o644); err != nil {
				return err
			}
			defer os.Remove(src)
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", src, "-c:s", "srt", "-f", "matroska", p)
		}},
		{id: "scc", name: "Clip.scc", make: text("Scenarist_SCC V1.0\n\n00:00:00:03\t9420 9420 94ae 94ae 9470 9470 c8e5 ecec 6f80 942c 942c 942f 942f\n\n")},
		{id: "mpl", name: "Clip.mpl", make: text("[1][9]Hello from a made-up film\n[10][19]Second\n[20][29]Third\n[30][39]Fourth\n")},
		{id: "stl", name: "Clip.stl", make: text("$FontName = Arial\n00:00:00:03 , 00:00:00:22 , Hello from a made-up film\n")},
		{id: "jss", name: "Clip.jss", make: text("#TITLE made up\n0:00:00.10 0:00:00.90 D Hello from a made-up film\n")},
		{id: "rt", name: "Clip.rt", make: text(`<window><time begin="00:00:00.10" end="00:00:00.90"/>Hello from a made-up film</window>` + "\n")},
		{id: "sup PGS", name: "Clip.sup", make: bytesOf(supBytes), burn: true},
		{id: "idx and sub VobSub", name: "Clip.idx", make: func(st *sampleTools, p string) error {
			return vobSub(p, strings.TrimSuffix(p, ".idx")+".sub")
		}, burn: true},
	}
}

// embeddedRows are subtitle tracks inside a video.
func embeddedRows() []*subtitleRow {
	base := func(st *sampleTools, p string, codec string, burnedSource string) error {
		src := p + ".src" + burnedSource
		var data string
		switch burnedSource {
		case ".srt":
			data = "1\n00:00:00,100 --> 00:00:00,900\nHello from a made-up film\n\n"
		case ".ass":
			data = "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize\nStyle: Default,Arial,20\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:00.10,0:00:00.90,Default,,0,0,0,,Hello\n"
		}
		if err := os.WriteFile(src, []byte(data), 0o644); err != nil {
			return err
		}
		defer os.Remove(src)
		return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=1", "-i", src, "-map", "0:v", "-map", "1:s", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:s", codec, p)
	}
	return []*subtitleRow{
		{id: "embedded SubRip in mkv", name: "text.mkv", make: func(st *sampleTools, p string) error { return base(st, p, "srt", ".srt") }},
		{id: "embedded ASS in mkv", name: "ass.mkv", make: func(st *sampleTools, p string) error { return base(st, p, "ass", ".ass") }},
		{id: "embedded MP4 timed text", name: "text.mp4", make: func(st *sampleTools, p string) error { return base(st, p, "mov_text", ".srt") }},
		{id: "embedded WebVTT in webm", name: "text.webm", make: func(st *sampleTools, p string) error {
			src := p + ".src.vtt"
			if err := os.WriteFile(src, []byte("WEBVTT\n\n00:00.100 --> 00:00.900\nHello\n"), 0o644); err != nil {
				return err
			}
			defer os.Remove(src)
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=1", "-i", src, "-map", "0:v", "-map", "1:s", "-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-c:s", "webvtt", p)
		}},
		{id: "embedded PGS in mkv", name: "pgs.mkv", burn: true, make: func(st *sampleTools, p string) error {
			src := p + ".src.sup"
			if err := os.WriteFile(src, supBytes(), 0o644); err != nil {
				return err
			}
			defer os.Remove(src)
			return st.run("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=1", "-i", src, "-map", "0:v", "-map", "1:s", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:s", "copy", p)
		}},
	}
}

// formatRowsAtOnce is how many rows are played at once: fewer than hls.maxGrowing, which ends the
// oldest copied stream to make room for another.
const formatRowsAtOnce = 6

// eachAtOnce runs f on every row in a subtest of its own, up to most at once (0: as many as
// -parallel says), and returns when they've all finished.
func eachAtOnce[R any](t *testing.T, name string, most int, rows []R, label func(R) string, f func(t *testing.T, r R)) {
	t.Helper()
	var slots chan struct{}
	if most > 0 {
		slots = make(chan struct{}, most)
	}
	t.Run(name, func(t *testing.T) {
		for _, r := range rows {
			t.Run(label(r), func(t *testing.T) {
				t.Parallel()
				if slots != nil {
					slots <- struct{}{}
					defer func() { <-slots }()
				}
				f(t, r)
			})
		}
	})
}

// forRow is the sample tools for one row, with a folder of its own, as rows are made at once.
func (st *sampleTools) forRow(t *testing.T, id string) *sampleTools {
	t.Helper()
	dir := filepath.Join(st.dir, safeRowName(id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &sampleTools{t: t, ffmpeg: st.ffmpeg, dir: dir, office: st.office}
}

// formatServer is a server like newTestServer's, with the machine's own FFmpeg.
func formatServer(t *testing.T, tools *ffmpeg.Tools) *testServer {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	var logTo io.Writer = io.Discard
	if os.Getenv("WMS_FORMAT_LOG") != "" {
		logTo = os.Stderr
	}
	log := slog.New(slog.NewTextHandler(logTo, nil))
	h := hls.NewManager(tools, filepath.Join(dir, "transcode"), formatRowsAtOnce, log)
	store := &library.Store{DB: d}
	cache := filepath.Join(dir, "cache")
	s := &Server{
		Cfg: &config.Config{DataDir: dir, ImagesDir: filepath.Join(dir, "images"), CacheDir: cache, MaxTranscode: formatRowsAtOnce}, DB: d, Log: log, Tools: tools, Store: store,
		Scanner: library.NewScanner(store, tools, cache, func() string { return "" }, log),
		Auth:    &auth.Service{DB: d}, HLS: h, DVD: dvdsession.NewManager(h, tools, dir, "/api", log), BD: bdsession.NewManager(h, tools, dir, "/api", log),
		Themes: &themes.Manager{Dir: filepath.Join(dir, "themes")}, Web: fstest.MapFS{"index.html": {Data: []byte("<html></html>")}}, Version: "test",
		audioSlots: make(chan struct{}, 2*formatRowsAtOnce),
	}
	if err := os.MkdirAll(s.Scanner.ImagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return &testServer{Server: s, h: s.Handler()}
}

func TestFormatTable(t *testing.T) {
	modtest.Need(t, "wittycd", "wittydvd", "wittybluray", "wittyphotos")
	if testing.Short() {
		t.Skip("makes real samples and plays them")
	}
	// The slowest tests here, each with a server and an FFmpeg of its own: together they take
	// the time of the longest, not the sum.
	t.Parallel()
	tools, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("no FFmpeg here")
	}
	ts := formatServer(t, tools)
	_, tok := ts.user(t, "tester", auth.KindAdult, true, "")
	root := t.TempDir()
	st := &sampleTools{t: t, ffmpeg: tools.FFmpeg, dir: filepath.Join(root, "work"), office: &officeProfile{dir: filepath.Join(root, "office-profile")}}
	if err := os.MkdirAll(st.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	groups := []struct {
		title string
		lib   string
		rows  []*formatRow
	}{
		{"Video", "videos", videoRows()},
		{"Audio", "music", audioRows()},
		{"Photos", "photos", photoRows()},
		{"Books and comics", "books", bookRows()},
		{"Discs", "discs", discRows()},
	}
	only := os.Getenv("WMS_FORMAT_ONLY") // a pattern, to work on a few rows
	var filter *regexp.Regexp
	if only != "" {
		filter = regexp.MustCompile(only)
	}
	var failures []string
	var table strings.Builder
	for _, g := range groups {
		folder := filepath.Join(root, g.lib)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		// A small film alone in the folder keeps the "folder is empty" check from firing
		// if a run makes nothing else.
		var wanted, made []*formatRow
		for _, row := range g.rows {
			if filter == nil || filter.MatchString(row.id) {
				wanted = append(wanted, row)
			}
		}
		rowID := func(r *formatRow) string { return r.id }
		eachAtOnce(t, g.title+" samples", 0, wanted, rowID, func(t *testing.T, row *formatRow) {
			p := filepath.Join(folder, filepath.FromSlash(row.file))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := row.make(st.forRow(t, row.id), p); err != nil {
				row.res = stage{scan: "no sample", probe: sNA, art: sNA, decision: sNA, play: sNA}
				row.note = strings.ReplaceAll(strings.TrimSpace(err.Error()), "\n", " ")
				return
			}
			row.made = true
		})
		for _, row := range wanted {
			if row.made {
				made = append(made, row)
			}
		}
		if len(made) > 0 {
			id := ts.library(t, g.lib, folder)
			scanAndWait(t, ts.Server, id)
			items, _, err := ts.Store.Items(context.Background(), library.ItemQuery{LibraryID: id, NoTotal: true, Limit: 100000})
			if err != nil {
				t.Fatal(err)
			}
			byPath := map[string]*library.Item{}
			for _, it := range items {
				byPath[it.Path] = it
			}
			eachAtOnce(t, g.title+" results", formatRowsAtOnce, made, rowID, func(t *testing.T, row *formatRow) {
				p := filepath.Join(folder, filepath.FromSlash(row.file))
				if it := byPath[p]; it == nil {
					row.res = stage{scan: "FAIL: not in the library", probe: sNA, art: sNA, decision: sNA, play: sNA}
				} else {
					checkItem(t, ts, tok, g.lib, row, it, tools)
				}
			})
		}
		fmt.Fprintf(&table, "\n### %s\n\n| Format | Scan | Probe | Artwork | Decision | Playback | Note |\n|---|---|---|---|---|---|---|\n", g.title)
		for _, row := range g.rows {
			if filter != nil && !filter.MatchString(row.id) {
				continue
			}
			r := row.res
			fmt.Fprintf(&table, "| %s | %s | %s | %s | %s | %s | %s |\n", row.id, r.scan, r.probe, r.art, r.decision, r.play, row.note)
			for _, c := range []string{r.scan, r.probe, r.art, r.decision, r.play} {
				if strings.HasPrefix(c, "FAIL") {
					failures = append(failures, row.id+": "+c)
					break
				}
			}
		}
	}
	if subs := checkSubtitles(t, ts, tok, tools, st, root, filter); subs != "" {
		table.WriteString(subs)
	}
	if extra := checkSheets(t, ts, tok, st, root, filter); extra != "" {
		table.WriteString(extra)
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		// Older FFmpegs don't read everything (HEIC before 7, for one), and that's what
		// a format's Needs says. What's enforced is this FFmpeg and later.
		if ffmpegMajor(tools) >= 8 {
			t.Errorf("%d formats failed:\n%s", len(failures), strings.Join(failures, "\n"))
		} else {
			t.Logf("%d formats didn't work with %s:\n%s", len(failures), tools.Version, strings.Join(failures, "\n"))
		}
	}
	t.Log(table.String())
	if out := os.Getenv("WMS_FORMAT_TABLE"); out != "" {
		if err := os.WriteFile(out, []byte(table.String()), 0o644); err != nil {
			t.Error(err)
		}
	}
}

var ffmpegVersionRe = regexp.MustCompile(`version (N-|n?)(\d*)`)

// ffmpegMajor reads a version; a build from the development branch counts as new.
func ffmpegMajor(t *ffmpeg.Tools) int {
	m := ffmpegVersionRe.FindStringSubmatch(t.Version)
	if m == nil {
		return 0
	}
	if m[1] == "N-" {
		return 99
	}
	n := 0
	fmt.Sscanf(m[2], "%d", &n)
	return n
}

// checkSheets covers the files read beside media: playlists become collections, and
// details and artwork reach the items they belong to.
func checkSheets(t *testing.T, ts *testServer, tok string, st *sampleTools, root string, filter *regexp.Regexp) string {
	t.Helper()
	var rows []string
	row := func(id, what, result string) {
		rows = append(rows, fmt.Sprintf("| %s | %s | %s |\n", id, what, result))
		if strings.HasPrefix(result, "FAIL") {
			t.Errorf("%s: %s", id, result)
		}
	}
	if filter != nil && !filter.MatchString("sheets") {
		return ""
	}
	music := filepath.Join(root, "playlist-music")
	songs := []string{"One.mp3", "sub/Two.flac", "Three.ogg"}
	for _, s := range songs {
		p := filepath.Join(music, filepath.FromSlash(s))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		args := map[string][]string{".mp3": {"-c:a", "libmp3lame"}, ".flac": {"-c:a", "flac"}, ".ogg": {"-c:a", "libvorbis"}}[filepath.Ext(s)]
		if err := st.ffaudio(p, args...); err != nil {
			t.Fatal(err)
		}
	}
	lists := map[string]string{
		"list m3u.m3u":   "#EXTM3U\n#EXTINF:1,One\nOne.mp3\nsub/Two.flac\nhttp://evil.test/x.mp3\n../../../etc/passwd\nThree.ogg\n",
		"list m3u8.m3u8": "#EXTM3U\nThree.ogg\nOne.mp3\n",
		"list pls.pls":   "[playlist]\nNumberOfEntries=2\nFile2=sub/Two.flac\nFile1=One.mp3\n",
		"list xspf.xspf": `<?xml version="1.0"?><playlist xmlns="http://xspf.org/ns/0/"><trackList><track><location>One.mp3</location></track><track><location>Three.ogg</location></track></trackList></playlist>`,
		"list asx.asx":   `<asx version="3.0"><entry><ref href="sub/Two.flac"/></entry><entry><ref href="One.mp3"/></entry></asx>`,
		"none.m3u":       "http://evil.test/only.mp3\n",
	}
	for n, body := range lists {
		if err := os.WriteFile(filepath.Join(music, n), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	id := ts.library(t, "music", music)
	scanAndWait(t, ts.Server, id)
	cols := ts.do(t, "GET", "/api/collections", tok, nil)
	var named []struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	json.Unmarshal(cols.Body.Bytes(), &named)
	want := map[string]int{"list m3u": 3, "list m3u8": 2, "list pls": 2, "list xspf": 2, "list asx": 2}
	got := map[string]int{}
	for _, c := range named {
		got[c.Name] = c.Count
	}
	for name, n := range want {
		res := "ok"
		if got[name] != n {
			res = fmt.Sprintf("FAIL: %d songs, wanted %d (%v)", got[name], n, got)
		}
		row(name, "a collection of the songs it names", res)
	}
	if _, made := got["none"]; made {
		row("A playlist of web addresses", "makes nothing", "FAIL: it made a collection")
	} else {
		row("A playlist of web addresses", "makes nothing", "ok")
	}
	// NFO and artwork.
	films := filepath.Join(root, "sidecar-films")
	if err := os.MkdirAll(filepath.Join(films, "Odd Name"), 0o755); err != nil {
		t.Fatal(err)
	}
	film := filepath.Join(films, "Odd Name", "odd.mp4")
	if err := st.ff(film, join(h264v(), x264, []string{"-c:a", "aac"})...); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(films, "Odd Name", "odd.nfo"), []byte(`<movie><title>The Made-Up Film</title><year>1999</year></movie>`), 0o644); err != nil {
		t.Fatal(err)
	}
	j := smallJPEGBytes(t)
	if err := os.WriteFile(filepath.Join(films, "Odd Name", "poster.tbn"), j, 0o644); err != nil {
		t.Fatal(err)
	}
	album := filepath.Join(root, "sidecar-album")
	if err := st.ffaudio(filepath.Join(album+"-make.mp3"), "-c:a", "libmp3lame"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(album, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(filepath.Join(album, "Song.mp3"), album+"-make.mp3"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(album, "folder.webp"), []byte("RIFF0000WEBP"), 0o644); err != nil {
		t.Fatal(err)
	}
	lid := ts.library(t, "videos", films)
	scanAndWait(t, ts.Server, lid)
	items, _, err := ts.Store.Items(context.Background(), library.ItemQuery{LibraryID: lid, NoTotal: true, Limit: 100})
	if err != nil || len(items) != 1 {
		t.Fatalf("%v items, %v", len(items), err)
	}
	res := "ok"
	if items[0].Title != "The Made-Up Film" {
		res = "FAIL: titled " + items[0].Title
	}
	row("nfo", "gives the title and details", res)
	res = "ok"
	if !strings.HasSuffix(items[0].Poster, "poster.tbn") {
		res = "FAIL: poster is " + items[0].Poster
	}
	row("tbn and other pictures beside a file", "become its artwork", res)
	aid := ts.library(t, "music", album)
	scanAndWait(t, ts.Server, aid)
	songsOf, _, err := ts.Store.Items(context.Background(), library.ItemQuery{LibraryID: aid, NoTotal: true, Limit: 100})
	if err != nil || len(songsOf) != 1 {
		t.Fatalf("%v items, %v", len(songsOf), err)
	}
	res = "ok"
	if !strings.HasSuffix(songsOf[0].Poster, "folder.webp") {
		res = "FAIL: cover is " + songsOf[0].Poster
	}
	row("Album art in the folder", "becomes the cover", res)
	return "\n### Playlists and files beside media\n\n| Format | What should happen | Result |\n|---|---|---|\n" + strings.Join(rows, "")
}

func scanAndWait(t *testing.T, s *Server, id int64) {
	t.Helper()
	if !s.Scanner.Start(id) {
		t.Fatal("the scan didn't start")
	}
	deadline := time.Now().Add(10 * time.Minute)
	for s.Scanner.Snapshot(id) != nil {
		if time.Now().After(deadline) {
			t.Fatal("the scan didn't finish")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

type playbackAnswer struct {
	Decision         string `json:"decision"`
	Protocol         string `json:"protocol"`
	URL              string `json:"url"`
	CDURL            string `json:"cdUrl"`
	StreamID         string `json:"streamId"`
	DurationSec      float64
	Seek             string `json:"seek"`
	SubtitleDelivery string `json:"subtitleDelivery"`
	Disc             *struct {
		Type      string `json:"type"`
		HasMenus  bool   `json:"hasMenus"`
		OpenURL   string `json:"openUrl"`
		Scrambled bool   `json:"scrambled"`
	} `json:"disc"`
	Subtitles []struct {
		Index    int    `json:"index"`
		Codec    string `json:"codec"`
		Delivery string `json:"delivery"`
		URL      string `json:"url"`
		Text     bool   `json:"text"`
	} `json:"subtitles"`
	Explain *struct {
		Summary string `json:"summary"`
		Sound   string `json:"sound"`
	} `json:"explain"`
	Videos []struct {
		Index int `json:"index"`
	} `json:"videos"`
	Stereo string `json:"stereo"`
}

func askPlayback(t *testing.T, ts *testServer, tok string, id int64, body map[string]any) (playbackAnswer, string) {
	t.Helper()
	if body == nil {
		body = map[string]any{}
	}
	body["profile"] = "web"
	w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/playback", id), tok, body)
	var a playbackAnswer
	if w.Code != 200 {
		return a, fmt.Sprintf("FAIL: playback %d: %s", w.Code, trimBody(w.Body.String()))
	}
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		return a, "FAIL: " + err.Error()
	}
	return a, ""
}

func trimBody(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

func checkItem(t *testing.T, ts *testServer, tok, lib string, row *formatRow, it *library.Item, tools *ffmpeg.Tools) {
	t.Helper()
	row.res = stage{scan: sOK, probe: sOK, art: sNA, decision: sNA, play: sNA}
	switch {
	case it.Kind == "video":
		if it.Container == "" || it.RuntimeSec <= 0 {
			row.res.probe = fmt.Sprintf("FAIL: container %q, length %.2f", it.Container, it.RuntimeSec)
		}
		if it.Poster != "" {
			row.res.art = sOK
		} else if it.VideoCodec != "" {
			row.res.art = "FAIL: no picture"
		}
		checkVideo(t, ts, tok, row, it, tools)
	case it.Kind == "audio":
		if it.VideoCodec == "" || it.RuntimeSec <= 0 {
			row.res.probe = fmt.Sprintf("FAIL: codec %q, length %.2f", it.VideoCodec, it.RuntimeSec)
		}
		if strings.Contains(row.id, "cover") {
			row.res.art = sOK
			if it.Poster == "" {
				row.res.art = "FAIL: no cover"
			}
		}
		checkAudio(t, ts, tok, row, it)
	case it.Kind == library.CDKind:
		row.res.probe = sOK
		a, bad := askPlayback(t, ts, tok, it.ID, nil)
		if bad != "" {
			row.res.decision = bad
			return
		}
		row.res.decision = a.Decision
		if a.Decision != row.want {
			row.res.decision = "FAIL: " + a.Decision + ", wanted " + row.want
		}
		row.res.play = fetchSome(t, ts, tok, a.URL)
	case it.Kind == "photo":
		checkPhoto(t, ts, tok, row, it)
	case it.Kind == "book":
		checkBook(t, ts, tok, row, it)
	case library.IsDiscKind(it.Kind):
		a, bad := askPlayback(t, ts, tok, it.ID, nil)
		if bad != "" {
			row.res.decision = bad
			return
		}
		row.res.decision = a.Decision
		if row.want == "refused" {
			row.res.probe, row.res.art = a.Disc.Type, sNA
			w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/dvd", it.ID), tok, map[string]any{"mode": "title"})
			row.res.play = "FAIL: it plays: " + trimBody(w.Body.String())
			if !a.Disc.Scrambled {
				row.res.decision = "FAIL: not noticed"
			} else if w.Code >= 400 || strings.Contains(w.Body.String(), "scrambled") {
				row.res.play = "refused"
			}
			return
		}
		if a.Decision != row.want || a.Disc == nil {
			row.res.decision = "FAIL: " + a.Decision + ", wanted " + row.want
			return
		}
		row.res.probe = a.Disc.Type
		if it.Poster != "" {
			row.res.art = sOK
		} else {
			row.res.art = "FAIL: no picture"
		}
		w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/dvd", it.ID), tok, map[string]any{"mode": "title"})
		row.res.play = sOK
		if w.Code != 200 && w.Code != 201 {
			row.res.play = fmt.Sprintf("FAIL: the disc session answers %d: %s", w.Code, trimBody(w.Body.String()))
			return
		}
		var sess struct {
			ID string `json:"id"`
		}
		json.Unmarshal(w.Body.Bytes(), &sess)
		if sess.ID != "" {
			ts.do(t, "DELETE", "/api/dvd/"+sess.ID, tok, nil)
		}
	default:
		row.res.scan = "FAIL: indexed as " + it.Kind
	}
}

func checkVideo(t *testing.T, ts *testServer, tok string, row *formatRow, it *library.Item, tools *ffmpeg.Tools) {
	t.Helper()
	a, bad := askPlayback(t, ts, tok, it.ID, nil)
	if bad != "" {
		row.res.decision = bad
		return
	}
	got := a.Decision
	if got == "remux" && a.Explain != nil && strings.Contains(a.Explain.Sound, "Converted") {
		got = "remux+audio"
	}
	row.res.decision = got
	if got != row.want {
		row.res.decision = "FAIL: " + got + ", wanted " + row.want
	}
	if strings.Contains(row.id, "Several video") && len(a.Videos) != 2 {
		row.res.decision = fmt.Sprintf("FAIL: lists %d video streams", len(a.Videos))
	}
	if strings.Contains(row.id, "3D") && a.Stereo == "" {
		row.res.decision = "FAIL: 3D not noticed"
	}
	switch a.Protocol {
	case "file":
		w := ts.do(t, "GET", a.URL, tok, nil, "Range", "bytes=0-99")
		row.res.play = sOK
		if w.Code != 206 {
			row.res.play = fmt.Sprintf("FAIL: the file answers %d", w.Code)
		} else if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "video/") && !strings.HasPrefix(ct, "audio/") {
			row.res.play = "FAIL: sent as " + ct
		}
	case "hls":
		row.res.play = playHLS(t, ts, tok, a, tools, row.check)
		ts.do(t, "DELETE", "/api/hls/"+a.StreamID, tok, nil)
	default:
		row.res.play = "FAIL: protocol " + a.Protocol
	}
}

var segLineRe = regexp.MustCompile(`(?m)^seg/([^\s?]+)`)

// playHLS asks for the playlist and the first segment, as a player would, and looks at
// what really came out.
func playHLS(t *testing.T, ts *testServer, tok string, a playbackAnswer, tools *ffmpeg.Tools, check func(*ffmpeg.Probe) string) string {
	t.Helper()
	w := ts.do(t, "GET", a.URL, tok, nil)
	if w.Code != 200 {
		return fmt.Sprintf("FAIL: playlist %d: %s", w.Code, trimBody(w.Body.String()))
	}
	m := segLineRe.FindStringSubmatch(w.Body.String())
	if m == nil {
		return "FAIL: no segments in the playlist: " + trimBody(w.Body.String())
	}
	base := "/api/hls/" + a.StreamID + "/seg/"
	w = ts.do(t, "GET", base+m[1], tok, nil)
	if w.Code != 200 {
		return fmt.Sprintf("FAIL: first segment %d: %s", w.Code, trimBody(w.Body.String()))
	}
	var data []byte
	if strings.HasSuffix(m[1], ".m4s") {
		init := ts.do(t, "GET", base+"init.mp4", tok, nil)
		if init.Code != 200 {
			return fmt.Sprintf("FAIL: init segment %d", init.Code)
		}
		data = append(data, init.Body.Bytes()...)
	}
	data = append(data, w.Body.Bytes()...)
	if len(data) == 0 {
		return "FAIL: empty segment"
	}
	tmp := filepath.Join(t.TempDir(), "segment.ts")
	if strings.HasSuffix(m[1], ".m4s") {
		tmp = filepath.Join(t.TempDir(), "segment.mp4")
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "FAIL: " + err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, err := tools.ProbeFile(ctx, tmp)
	if err != nil {
		return "FAIL: the segment doesn't read: " + trimBody(err.Error())
	}
	if len(p.Streams) == 0 {
		return "FAIL: the segment holds no streams"
	}
	if check != nil {
		if bad := check(p); bad != "" {
			return "FAIL: " + bad
		}
	}
	return sOK
}

func fetchSome(t *testing.T, ts *testServer, tok, url string) string {
	t.Helper()
	w := ts.do(t, "GET", url, tok, nil)
	if w.Code != 200 && w.Code != 206 {
		return fmt.Sprintf("FAIL: %d: %s", w.Code, trimBody(w.Body.String()))
	}
	if w.Body.Len() == 0 {
		return "FAIL: nothing came back"
	}
	return sOK
}

func checkAudio(t *testing.T, ts *testServer, tok string, row *formatRow, it *library.Item) {
	t.Helper()
	a, bad := askPlayback(t, ts, tok, it.ID, nil)
	if bad != "" {
		row.res.decision = bad
		return
	}
	row.res.decision = a.Decision
	if a.Decision != row.want {
		row.res.decision = "FAIL: " + a.Decision + ", wanted " + row.want
	}
	row.res.play = fetchSome(t, ts, tok, a.URL)
}

func checkPhoto(t *testing.T, ts *testServer, tok string, row *formatRow, it *library.Item) {
	t.Helper()
	if it.Poster == "" {
		row.res.art = "FAIL: no picture"
	} else {
		row.res.art = sOK
	}
	row.res.decision = row.want
	w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/image?w=320", it.ID), tok, nil)
	ct := w.Header().Get("Content-Type")
	if w.Code != 200 {
		row.res.play = fmt.Sprintf("FAIL: thumbnail %d: %s", w.Code, trimBody(w.Body.String()))
		return
	}
	if row.want == "svg" {
		body := w.Body.String()
		if ct != "image/svg+xml" || strings.Contains(body, "script") || strings.Contains(body, "onload") || strings.Contains(body, "evil.test") {
			row.res.play = "FAIL: not sanitised: " + trimBody(body)
			return
		}
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
			row.res.play = "FAIL: not sandboxed"
			return
		}
		row.res.play = sOK
		return
	}
	if ct != "image/jpeg" {
		row.res.play = "FAIL: thumbnail sent as " + ct
		return
	}
	full := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/image", it.ID), tok, nil)
	row.res.play = sOK
	switch {
	case full.Code != 200:
		row.res.play = fmt.Sprintf("FAIL: the picture answers %d", full.Code)
	case row.want == "original" && full.Header().Get("Content-Type") == "image/jpeg" && !strings.HasPrefix(row.id, "jp"):
		row.res.play = "FAIL: the original wasn't sent as it is"
	case row.want == "jpeg" && full.Header().Get("Content-Type") != "image/jpeg":
		row.res.play = "FAIL: sent as " + full.Header().Get("Content-Type")
	case row.want == "motion":
		m := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/motion", it.ID), tok, nil)
		if m.Code != 200 || m.Body.Len() < 100 {
			row.res.play = fmt.Sprintf("FAIL: the clip answers %d", m.Code)
		}
	}
}

func checkBook(t *testing.T, ts *testServer, tok string, row *formatRow, it *library.Item) {
	t.Helper()
	row.res.decision = it.Reader
	if it.Reader != row.want {
		row.res.decision = fmt.Sprintf("FAIL: opens as %q, wanted %s", it.Reader, row.want)
	}
	if it.Poster != "" {
		row.res.art = sOK
	}
	w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/book", it.ID), tok, nil)
	if w.Code != 200 {
		row.res.play = fmt.Sprintf("FAIL: %d: %s", w.Code, trimBody(w.Body.String()))
		return
	}
	var info struct {
		Pages int      `json:"pages"`
		Spine []string `json:"spine"`
	}
	json.Unmarshal(w.Body.Bytes(), &info)
	switch it.Reader {
	case "cbz":
		if info.Pages == 0 {
			row.res.play = "FAIL: no pages"
			return
		}
		pg := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/page/0", it.ID), tok, nil)
		row.res.play = sOK
		if pg.Code != 200 || !strings.HasPrefix(pg.Header().Get("Content-Type"), "image/") {
			row.res.play = fmt.Sprintf("FAIL: first page %d, %s: %s", pg.Code, pg.Header().Get("Content-Type"), trimBody(pg.Body.String()))
		}
	case "epub":
		if len(info.Spine) == 0 {
			row.res.play = "FAIL: no chapters"
			return
		}
		ch := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/epub/%s", it.ID, info.Spine[0]), tok, nil)
		row.res.play = sOK
		if ch.Code != 200 {
			row.res.play = fmt.Sprintf("FAIL: first chapter %d", ch.Code)
			return
		}
		all := ""
		for _, c := range info.Spine {
			all += ts.do(t, "GET", fmt.Sprintf("/api/items/%d/epub/%s", it.ID, c), tok, nil).Body.String()
		}
		if !strings.Contains(all, "lamp") && !strings.Contains(all, "boats") {
			row.res.play = "FAIL: the book's words aren't there: " + trimBody(all)
		}
		if strings.Contains(strings.ToLower(all), "<script") {
			row.res.play = "FAIL: a script got through"
		}
	case "pdf":
		f := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/file", it.ID), tok, nil)
		row.res.play = sOK
		if f.Code != 200 || f.Header().Get("Content-Type") != "application/pdf" {
			row.res.play = fmt.Sprintf("FAIL: the file answers %d, %s", f.Code, f.Header().Get("Content-Type"))
		}
	}
}

// checkSubtitles plays a small video with each subtitle file beside it, and each kind
// of track inside it.
func checkSubtitles(t *testing.T, ts *testServer, tok string, tools *ffmpeg.Tools, st *sampleTools, root string, filter *regexp.Regexp) string {
	t.Helper()
	folder := filepath.Join(root, "subtitles")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	rows := subtitleRows()
	embedded := embeddedRows()
	var wanted, made []*subtitleRow
	for _, row := range rows {
		row.video = true
	}
	for _, row := range append(append([]*subtitleRow{}, rows...), embedded...) {
		if filter == nil || filter.MatchString(row.id) {
			wanted = append(wanted, row)
		}
	}
	rowID := func(r *subtitleRow) string { return r.id }
	eachAtOnce(t, "Subtitle samples", 0, wanted, rowID, func(t *testing.T, row *subtitleRow) {
		st := st.forRow(t, row.id)
		dir := filepath.Join(folder, safeRowName(row.id))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if row.video { // beside a video of its own
			stem := strings.SplitN(row.name, ".", 2)[0]
			if err := st.ff(filepath.Join(dir, stem+".mkv"), join(h264v(), x264, []string{"-c:a", "aac"})...); err != nil {
				t.Fatal(err)
			}
		}
		if err := row.make(st, filepath.Join(dir, row.name)); err != nil {
			row.res = stage{scan: "no sample", probe: sNA, art: sNA, decision: sNA, play: sNA}
			row.note = err.Error()
			return
		}
		row.made = true
	})
	for _, row := range wanted {
		if row.made {
			made = append(made, row)
		}
	}
	if len(made) == 0 {
		return ""
	}
	id := ts.library(t, "videos", folder)
	scanAndWait(t, ts.Server, id)
	items, _, err := ts.Store.Items(context.Background(), library.ItemQuery{LibraryID: id, NoTotal: true, Limit: 100000})
	if err != nil {
		t.Fatal(err)
	}
	byDir := map[string]*library.Item{}
	for _, it := range items {
		byDir[filepath.Base(filepath.Dir(it.Path))] = it
	}
	eachAtOnce(t, "Subtitle results", formatRowsAtOnce, made, rowID, func(t *testing.T, row *subtitleRow) {
		row.res = stage{scan: sOK, probe: sNA, art: sNA, decision: sNA, play: sNA}
		it := byDir[safeRowName(row.id)]
		if it == nil {
			row.res.scan = "FAIL: the video isn't in the library"
			return
		}
		a, bad := askPlayback(t, ts, tok, it.ID, map[string]any{"subtitle": 0})
		if bad != "" {
			row.res.decision = bad
			return
		}
		if len(a.Subtitles) == 0 {
			row.res.probe = "FAIL: no subtitle track found"
			return
		}
		row.res.probe = sOK
		row.res.decision = a.SubtitleDelivery
		want := "sidecar"
		if row.burn {
			want = "burn"
		}
		if a.SubtitleDelivery != want {
			row.res.decision = "FAIL: delivered by " + a.SubtitleDelivery + ", wanted " + want
		}
		if row.burn {
			row.res.play = playHLS(t, ts, tok, a, tools, nil)
			ts.do(t, "DELETE", "/api/hls/"+a.StreamID, tok, nil)
			return
		}
		vtt := ts.do(t, "GET", a.Subtitles[0].URL, tok, nil)
		body := vtt.Body.String()
		switch {
		case vtt.Code != 200:
			row.res.play = fmt.Sprintf("FAIL: WebVTT %d: %s", vtt.Code, trimBody(body))
			return
		case !strings.HasPrefix(body, "WEBVTT") || !strings.Contains(body, "-->"):
			row.res.play = "FAIL: not WebVTT: " + trimBody(body)
			return
		}
		row.res.play = sOK
		if strings.Contains(row.id, "1252") && !strings.Contains(body, "Café crème") {
			row.res.play = "FAIL: its encoding came out wrong: " + trimBody(body)
			return
		}
		// Burned in as well, for a player that wants its styling.
		b, bad := askPlayback(t, ts, tok, it.ID, map[string]any{"subtitle": 0, "burnSubtitle": true})
		if bad != "" {
			row.res.play = bad
			return
		}
		row.res.play = playHLS(t, ts, tok, b, tools, nil)
		ts.do(t, "DELETE", "/api/hls/"+b.StreamID, tok, nil)
	})
	var out strings.Builder
	out.WriteString("\n### Subtitles\n\n| Format | Scan | Found | Artwork | Delivery | Text, and burned in | Note |\n|---|---|---|---|---|---|---|\n")
	all := append(rows, embedded...)
	for _, row := range all {
		if filter != nil && !filter.MatchString(row.id) {
			continue
		}
		r := row.res
		fmt.Fprintf(&out, "| %s | %s | %s | %s | %s | %s | %s |\n", row.id, r.scan, r.probe, r.art, r.decision, r.play, row.note)
		for _, c := range []string{r.scan, r.probe, r.decision, r.play} {
			if strings.HasPrefix(c, "FAIL") {
				t.Errorf("%s: %s", row.id, c)
				break
			}
		}
	}
	return out.String()
}

func safeRowName(id string) string {
	return regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(id, "-")
}

func TestDolbyVisionRange(t *testing.T) {
	dv := func(profile, compat float64, transfer string) *ffmpeg.Stream {
		return &ffmpeg.Stream{ColorTransfer: transfer, SideData: []map[string]any{{"side_data_type": "DOVI configuration record", "dv_profile": profile, "dv_bl_signal_compatibility_id": compat}}}
	}
	for _, c := range []struct {
		s    *ffmpeg.Stream
		want string
	}{
		{dv(5, 0, "smpte2084"), "dv5"},
		{dv(8, 1, "smpte2084"), "hdr10"},
		{dv(7, 6, "smpte2084"), "hdr10"},
		{dv(8, 4, "arib-std-b67"), "hlg"},
		{dv(8, 2, ""), "sdr"},
		{dv(4, 0, "arib-std-b67"), "hlg"},
		{&ffmpeg.Stream{ColorTransfer: "smpte2084"}, "hdr10"},
		{&ffmpeg.Stream{ColorTransfer: "arib-std-b67"}, "hlg"},
		{&ffmpeg.Stream{ColorTransfer: "bt709"}, "sdr"},
		{&ffmpeg.Stream{}, "sdr"},
	} {
		if got := c.s.Range(); got != c.want {
			t.Errorf("%+v: %s, want %s", c.s, got, c.want)
		}
	}
	if got := dv(8, 1, "smpte2084").DynamicRange(); got != "Dolby Vision profile 8.1" {
		t.Errorf("named %q", got)
	}
	plus := &ffmpeg.Stream{SideData: []map[string]any{{"side_data_type": "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)"}}}
	if got := plus.DynamicRange(); got != "HDR10+" {
		t.Errorf("named %q", got)
	}
}

func TestStereoNames(t *testing.T) {
	plain := &ffmpeg.Stream{Tags: map[string]string{}}
	for name, want := range map[string]string{
		"Film.3D.HSBS.mkv": "sbs", "Film 3D SBS.mkv": "sbs", "Film.3D.H-SBS.1080p.mkv": "sbs", "Film.HOU.mkv": "tab", "Film.3D.TAB.mkv": "tab",
		"Film.mkv": "", "Tabby.mkv": "", "Sbsomething.mkv": "", "Out of Time.mkv": "",
	} {
		if got := plain.StereoMode(name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	if got := (&ffmpeg.Stream{Tags: map[string]string{"stereo_mode": "top_bottom"}}).StereoMode("a.mkv"); got != "tab" {
		t.Errorf("by its tag: %q", got)
	}
	if got := (&ffmpeg.Stream{Tags: map[string]string{"stereo_mode": "mono"}}).StereoMode("a.mkv"); got != "" {
		t.Errorf("mono: %q", got)
	}
	if got := ffmpeg.FlattenFilter("sbs", 1920, 1080); got != "crop=iw/2:ih:0:0,scale=iw*2:ih,setsar=1" {
		t.Errorf("half width: %q", got)
	}
	if got := ffmpeg.FlattenFilter("sbs", 3840, 1080); got != "crop=iw/2:ih:0:0" {
		t.Errorf("full width: %q", got)
	}
	if got := ffmpeg.FlattenFilter("tab", 1920, 1080); got != "crop=iw:ih/2:0:0,scale=iw:ih*2,setsar=1" {
		t.Errorf("half height: %q", got)
	}
}

// Codecs nothing here can encode are checked by asking FFmpeg whether it can decode them.
func TestCodecsDecoders(t *testing.T) {
	tools, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("no FFmpeg here")
	}
	for _, c := range []string{"vc1", "rv30", "rv40", "ape", "tak", "wmapro", "prores", "dnxhd", "dca", "truehd", "cook", "hevc", "av1", "vp9"} {
		if !tools.CanDecode(c) && !tools.CanDecode("lib"+c) && !tools.CanDecode("lib"+c+"1") && c != "av1" {
			t.Errorf("this FFmpeg can't decode %s", c)
		}
	}
}

// Damaged, empty and made-up files of every kind: a scan gets through them, and asking to
// play or open each gets an answer rather than a crash.
func TestHostileFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("runs FFmpeg on bad files")
	}
	t.Parallel() // as TestFormatTable
	tools, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("no FFmpeg here")
	}
	ts := formatServer(t, tools)
	_, tok := ts.user(t, "tester", auth.KindAdult, true, "")
	root := t.TempDir()
	noise := make([]byte, 1<<16)
	for i := range noise {
		noise[i] = byte(i*2654435761>>13) ^ byte(i>>3)
	}
	bodies := map[string][]byte{
		"noise": noise, "zeros": make([]byte, 1<<16), "tiny": []byte("x"),
		"html":     []byte("<!DOCTYPE html><html><script>alert(1)</script></html>" + strings.Repeat(" ", 2000)),
		"prog":     append([]byte("MZ\x90\x00\x03\x00\x00\x00"), noise...),
		"cut-mkv":  append([]byte("\x1A\x45\xDF\xA3\x01\x42\x82\x88matroska"), noise[:512]...),
		"cut-mp4":  append([]byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00isom"), noise[:512]...),
		"cut-zip":  append([]byte("PK\x03\x04\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF"), noise[:512]...),
		"cut-rar":  append([]byte("Rar!\x1A\x07\x01\x00"), noise[:512]...),
		"cut-7z":   append([]byte("7z\xBC\xAF\x27\x1C\x00\x04"), noise[:512]...),
		"cut-pdb":  append(append(make([]byte, 60), "BOOKMOBI"...), noise[:512]...),
		"cut-midi": append([]byte("MThd\x00\x00\x00\x06\x00\x01\x7F\xFF\x00\x01MTrk\x7F\xFF\xFF\xFF"), noise[:512]...),
		"cut-svg":  []byte(strings.Repeat("<svg><g>", 4000)),
		"cut-ole":  append([]byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"), noise[:2048]...),
		"cut-iso":  append(append(make([]byte, 0x8001), "CD001"...), noise[:4096]...),
	}
	order := make([]string, 0, len(bodies))
	for name := range bodies {
		order = append(order, name)
	}
	sort.Strings(order)
	for lib, exts := range map[string]string{
		"videos": ".mkv .mp4 .avi .ts .m2ts .wmv .rm .mpg .webm .mxf .y4m .ivf .h264",
		"music":  ".mp3 .flac .m4a .ogg .wav .mid .mod .xm .nsf .spc .dsf .ape .mka .cue .m3u .pls .xspf .asx",
		"photos": ".jpg .png .heic .avif .jxl .svg .svgz .tiff .cr2 .cr3 .nef .ico .psd",
		"books":  ".epub .mobi .azw3 .fb2 .docx .odt .doc .rtf .pdf .djvu .cbz .cbr .cb7 .cbt .txt .md .html",
		"discs":  ".iso .img .nrg .mdf .bin",
	} {
		dir := filepath.Join(root, lib)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for i, e := range strings.Fields(exts) {
			// Every kind of damage for each extension takes minutes. The full run is for
			// when the parsers change: WMS_HOSTILE_FULL=1.
			every := []string{"noise", "html", "prog"}
			every = append(every, order[i%len(order)])
			if os.Getenv("WMS_HOSTILE_FULL") != "" {
				every = order
			}
			for _, name := range every {
				if err := os.WriteFile(filepath.Join(dir, name+"-"+strings.TrimPrefix(e, ".")+e), bodies[name], 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		id := ts.library(t, lib, dir)
		scanAndWait(t, ts.Server, id)
		items, _, err := ts.Store.Items(context.Background(), library.ItemQuery{LibraryID: id, NoTotal: true, Limit: 100000})
		if err != nil {
			t.Fatal(err)
		}
		eachAtOnce(t, lib, formatRowsAtOnce, items, func(it *library.Item) string { return filepath.Base(it.Path) }, func(t *testing.T, it *library.Item) {
			for _, u := range map[string][]string{"book": {"/book", "/page/0", "/file"}, "photo": {"/image?w=320", "/motion"}, "video": {"/file"}, "audio": {"/stream"}}[it.Kind] {
				w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d%s", it.ID, u), tok, nil, "Range", "bytes=0-99")
				if w.Code >= 500 && w.Code != 503 && strings.Contains(w.Body.String(), "panic") {
					t.Errorf("%s %s: %d %s", it.Path, u, w.Code, trimBody(w.Body.String()))
				}
			}
			w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/playback", it.ID), tok, map[string]any{"profile": "web"})
			if w.Code == 200 {
				var a playbackAnswer
				if json.Unmarshal(w.Body.Bytes(), &a) == nil && a.StreamID != "" {
					ts.do(t, "DELETE", "/api/hls/"+a.StreamID, tok, nil)
				}
			}
			if strings.Contains(strings.ToLower(it.Path), "html-") || strings.Contains(it.Path, "prog-") {
				switch lib {
				case "videos", "music", "photos":
					t.Errorf("%s: a web page or a program got into a %s library", it.Path, lib)
				}
			}
		})
	}
}
