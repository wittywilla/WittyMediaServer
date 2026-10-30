package library

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// Blu-ray images and BDMV folders in a library, through WittyBluray. A
// disc's "titles" are its playlists (ttn = playlist number); the main one
// is the feature.

// probeBluray fills the item when its path is a Blu-ray; returns false
// when it is not one.
func (s *Scanner) probeBluray(ctx context.Context, it *Item) bool {
	d, err := wittybluray.Open(it.Path)
	if err != nil {
		return false
	}
	defer d.Close()
	if strings.HasPrefix(it.Kind, "dvd-") || it.Kind == "" {
		if strings.HasSuffix(it.Kind, "folder") {
			it.Kind = "bluray-folder"
		} else {
			it.Kind = "bluray-iso"
		}
	}
	info := &DiscInfo{Label: d.Label, Format: "bluray", HasMenus: d.Index.TopMenu.Kind == wittybluray.ObjectHDMV, Scrambled: d.Encrypted}
	if d.Label != "" && it.Title == "" {
		it.Title, _ = ParseName(d.Label)
	}
	main := d.MainPlaylist()
	nums := make([]int, 0, len(d.Playlists))
	for n := range d.Playlists {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for _, n := range nums {
		pl := d.Playlists[n]
		dur := d.PlaylistDuration(pl)
		if len(pl.Items) == 0 || len(pl.Items[0].Video) == 0 || (dur < 30 && pl != main) {
			continue
		}
		it0 := pl.Items[0]
		dt := DiscTitle{Number: n, DurationSec: dur, Angles: len(it0.Angles), Wide: true, Standard: standardName(it0.Video[0]), Chapters: []float64{}, Audio: []string{}, Subtitles: []string{}}
		var t0 float64
		starts := make([]float64, len(pl.Items))
		for i, pi := range pl.Items {
			starts[i] = t0
			t0 += pi.Duration()
		}
		for _, m := range pl.Marks {
			if m.Type == 1 && m.Item < len(pl.Items) {
				dt.Chapters = append(dt.Chapters, starts[m.Item]+float64(int64(m.Time)-int64(pl.Items[m.Item].In))/45000)
			}
		}
		for _, a := range it0.Audio {
			dt.Audio = append(dt.Audio, strings.TrimSpace(fmt.Sprintf("%s %s %dch", orUnd(a.Lang), a.CodingName(), channelsOf(a.Format))))
		}
		for _, sub := range it0.PG {
			dt.Subtitles = append(dt.Subtitles, orUnd(sub.Lang))
		}
		info.Titles = append(info.Titles, dt)
	}
	if main != nil && len(main.Items) > 0 {
		info.MainTitle = main.Number
		it.RuntimeSec = d.PlaylistDuration(main)
		v := main.Items[0].Video[0]
		it.Width, it.Height = frameSize(v.Format)
		it.VideoCodec = codecName(v.Coding)
		for _, a := range main.Items[0].Audio {
			it.AudioLangs = append(it.AudioLangs, orUnd(a.Lang))
		}
		for _, sub := range main.Items[0].PG {
			it.SubLangs = append(it.SubLangs, orUnd(sub.Lang))
		}
	}
	if d.Stereo3D {
		it.Genres = appendUnique(it.Genres, "3D")
	}
	it.Container = "bluray"
	it.Disc = info
	return true
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return list
		}
	}
	return append(list, v)
}

func standardName(v wittybluray.Stream) string {
	base := resolutionName(v.Format)
	switch v.DynamicRange {
	case 1:
		if v.HDR10Plus {
			return base + " HDR10+"
		}
		return base + " HDR10"
	case 2:
		return base + " Dolby Vision"
	}
	return base
}

func resolutionName(format int) string {
	switch format {
	case 1:
		return "480i"
	case 2:
		return "576i"
	case 3:
		return "480p"
	case 4:
		return "1080i"
	case 5:
		return "720p"
	case 6:
		return "1080p"
	case 7:
		return "576p"
	case 8:
		return "2160p"
	}
	return ""
}

func frameSize(format int) (int, int) {
	switch format {
	case 1, 3:
		return 720, 480
	case 2, 7:
		return 720, 576
	case 5:
		return 1280, 720
	case 8:
		return 3840, 2160
	}
	return 1920, 1080
}

func codecName(coding byte) string {
	switch coding {
	case 0x02:
		return "mpeg2video"
	case 0x1b, 0x20:
		return "h264"
	case 0x24:
		return "hevc"
	case 0xea:
		return "vc1"
	}
	return ""
}

func channelsOf(format int) int {
	switch format {
	case 1:
		return 1
	case 6, 12:
		return 6
	}
	return 2
}

// blurayThumbInput is the FFmpeg input for a frame about 20% into the
// feature, and the seek within it.
func (s *Scanner) blurayThumbInput(it *Item, h [20]byte) ([]string, float64, error) {
	d, err := wittybluray.Open(it.Path)
	if err != nil {
		return nil, 0, err
	}
	defer d.Close()
	pl := d.MainPlaylist()
	if pl == nil || len(pl.Items) == 0 {
		return nil, 0, fmt.Errorf("no main playlist")
	}
	target := d.PlaylistDuration(pl) * 0.2
	item, t := 0, 0.0
	for i, pi := range pl.Items {
		if t+pi.Duration() > target {
			item = i
			break
		}
		t += pi.Duration()
	}
	pi := pl.Items[item]
	at := pi.In + uint32((target-t)*45000)
	c := d.Clip(pi.Clip)
	if c == nil {
		return nil, 0, fmt.Errorf("clip missing")
	}
	e, ok := c.SPNAt(c.VideoPID(), at)
	if !ok {
		return nil, 0, fmt.Errorf("no entry point")
	}
	ext, err := d.ClipExtents(pi.Clip)
	if err != nil {
		return nil, 0, err
	}
	from := int64(e.SPN) * 192
	to := from + int64(c.TSRate/8)*20
	var cut []wittydvd.FileExtent
	var off int64
	for _, x := range ext {
		a, b := off, off+x.Length
		off = b
		lo, hi := a, b
		if from > lo {
			lo = from
		}
		if to < hi {
			hi = to
		}
		if hi > lo {
			cut = append(cut, wittydvd.FileExtent{File: x.Path, Offset: x.Offset + (lo - a), Length: hi - lo})
		}
	}
	if len(cut) == 0 {
		return nil, 0, fmt.Errorf("cannot read the feature")
	}
	list, err := ffmpeg.WriteConcat(filepath.Join(s.CacheDir, "lists"), "thumb-"+hex.EncodeToString(h[:8]), []ffmpeg.Segment{{Extents: cut}}, int(c.VideoPID()))
	if err != nil {
		return nil, 0, err
	}
	return ffmpeg.ConcatInputArgs(list), float64(int64(at)-int64(e.PTS)) / 45000, nil
}
