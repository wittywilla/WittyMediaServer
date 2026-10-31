package library

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Metadata is saved next to the media in the standard NFO/artwork layout that
// other media software reads, so a library can move between servers:
//
//	Movie in its own folder:    movie folder/<name>.nfo, poster.jpg, fanart.jpg
//	Movies sharing a folder:    <name>.nfo, <name>-poster.jpg, <name>-fanart.jpg
//	VIDEO_TS folder:            <folder>/movie.nfo, poster.jpg, fanart.jpg
//	TV show:                    <show>/tvshow.nfo, poster.jpg, fanart.jpg, seasonNN-poster.jpg
//	Episode:                    <name>.nfo (episodedetails), <name>-thumb.jpg
//
// NFOs we did not write are kept as <name>.nfo.bak before being replaced.

const nfoMarker = "<!-- written by WittyMediaServer -->"

// ArtPaths returns where an item's poster, backdrop and NFO live.
func ArtPaths(it *Item) (poster, backdrop, nfo string) {
	if it.Kind == "dvd-folder" {
		return filepath.Join(it.Path, "poster.jpg"), filepath.Join(it.Path, "fanart.jpg"), filepath.Join(it.Path, "movie.nfo")
	}
	dir := filepath.Dir(it.Path)
	if it.Kind == "game" {
		// A folder of games has no one game to call its poster.jpg after.
		base := gameBaseName(filepath.Base(it.Path))
		return filepath.Join(dir, base+"-poster.jpg"), filepath.Join(dir, base+"-fanart.jpg"), filepath.Join(dir, base+".nfo")
	}
	base := strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path))
	nfo = filepath.Join(dir, base+".nfo")
	if it.ShowID != nil {
		return filepath.Join(dir, base+"-thumb.jpg"), "", nfo
	}
	if soleMedia(dir, it.Path) {
		return filepath.Join(dir, "poster.jpg"), filepath.Join(dir, "fanart.jpg"), nfo
	}
	return filepath.Join(dir, base+"-poster.jpg"), filepath.Join(dir, base+"-fanart.jpg"), nfo
}

// soleMedia reports whether p is the only video/disc file in dir.
func soleMedia(dir, p string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || strings.EqualFold(filepath.Join(dir, e.Name()), p) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if videoExts[ext] || ext == ".iso" || ext == ".img" {
			return false
		}
	}
	return true
}

// LoadImage reads an artwork reference: a URL, a "cache:" name or a path.
// References can come from sidecar files anyone with access to a media folder
// wrote, and what is read here gets saved beside the media, so it only ever
// reads pictures: from the internet, the artwork cache or a library folder.
func (s *Scanner) LoadImage(ctx context.Context, ref string) ([]byte, error) {
	var b []byte
	var err error
	switch {
	case ref == "":
		return nil, errors.New("no image")
	case strings.HasPrefix(ref, "cache:"):
		name := strings.TrimPrefix(ref, "cache:")
		if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\:`) {
			return nil, errors.New("not an artwork name")
		}
		b, err = readPlain(filepath.Join(s.ImagesDir, name))
	case isRemote(ref):
		b, err = fetchPicture(ctx, ref)
	default:
		if !s.Store.LibraryFile(ctx, ref) {
			return nil, errors.New("artwork outside the libraries")
		}
		if st, serr := os.Stat(ref); serr == nil && st.Size() > 20<<20 { // the same limit as a download
			return nil, errors.New("image too large")
		}
		b, err = os.ReadFile(ref)
	}
	if err != nil {
		return nil, err
	}
	if !isPicture(b) {
		return nil, errors.New("not a picture")
	}
	return b, nil
}

// fetchPicture downloads an image from the internet.
func fetchPicture(ctx context.Context, ref string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, "GET", ref, nil)
	if err != nil {
		return nil, err
	}
	res, err := SafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("image fetch: %s", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 20<<20))
}

// writeArt stores image bytes at dst (unless dst already is the source).
func (s *Scanner) writeArt(ctx context.Context, ref, dst string) error {
	if dst == "" || ref == "" || filepath.Clean(ref) == filepath.Clean(dst) {
		return nil
	}
	b, err := s.LoadImage(ctx, ref)
	if err != nil {
		return err
	}
	return writeBeside(dst, b)
}

// writeNFO replaces a file beside the media in one step, so a crash or a full
// disk can't leave someone's NFO cut short.
func writeNFO(path string, doc any) error {
	if old, err := readPlain(path); err == nil && !bytes.Contains(old, []byte(nfoMarker)) {
		if _, err := os.Lstat(path + ".bak"); os.IsNotExist(err) {
			_ = writeBeside(path+".bak", old)
		}
	}
	b, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	out := []byte(xml.Header + nfoMarker + "\n")
	out = append(out, b...)
	out = append(out, '\n')
	return writeBeside(path, out)
}

type nfoUniqueID struct {
	Type    string `xml:"type,attr"`
	Default bool   `xml:"default,attr,omitempty"`
	ID      string `xml:",chardata"`
}

type nfoMovieOut struct {
	XMLName   xml.Name      `xml:"movie"`
	Title     string        `xml:"title"`
	SortTitle string        `xml:"sorttitle,omitempty"`
	Year      int           `xml:"year,omitempty"`
	Plot      string        `xml:"plot,omitempty"`
	Tagline   string        `xml:"tagline,omitempty"`
	Runtime   int           `xml:"runtime,omitempty"`
	MPAA      string        `xml:"mpaa,omitempty"`
	Rating    float64       `xml:"rating,omitempty"`
	Genres    []string      `xml:"genre"`
	Studio    string        `xml:"studio,omitempty"`
	UniqueIDs []nfoUniqueID `xml:"uniqueid"`
	Thumb     string        `xml:"thumb,omitempty"`
	Fanart    *struct {
		Thumb string `xml:"thumb"`
	} `xml:"fanart,omitempty"`
}

type nfoEpisodeOut struct {
	XMLName   xml.Name      `xml:"episodedetails"`
	Title     string        `xml:"title"`
	ShowTitle string        `xml:"showtitle,omitempty"`
	Season    int           `xml:"season"`
	Episode   int           `xml:"episode"`
	Plot      string        `xml:"plot,omitempty"`
	Aired     string        `xml:"aired,omitempty"`
	Runtime   int           `xml:"runtime,omitempty"`
	Rating    float64       `xml:"rating,omitempty"`
	UniqueIDs []nfoUniqueID `xml:"uniqueid"`
}

type nfoShowOut struct {
	XMLName      xml.Name      `xml:"tvshow"`
	Title        string        `xml:"title"`
	SortTitle    string        `xml:"sorttitle,omitempty"`
	Year         int           `xml:"year,omitempty"`
	Plot         string        `xml:"plot,omitempty"`
	MPAA         string        `xml:"mpaa,omitempty"`
	Rating       float64       `xml:"rating,omitempty"`
	Genres       []string      `xml:"genre"`
	Studio       string        `xml:"studio,omitempty"`
	Status       string        `xml:"status,omitempty"`
	UniqueIDs    []nfoUniqueID `xml:"uniqueid"`
	TVDBIDCompat string        `xml:"tvdbid,omitempty"`
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefF(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// SaveItemMetadata writes the item's NFO and artwork beside it. Artwork is
// copied to the standard names and the item is pointed at the local files,
// so other servers (and a rescan) see the same pictures.
func (s *Scanner) SaveItemMetadata(ctx context.Context, it *Item) error {
	posterPath, backdropPath, nfoPath := ArtPaths(it)
	var errs []error
	if it.Poster != "" && !strings.HasPrefix(it.Poster, "cache:thumb-") {
		if err := s.writeArt(ctx, it.Poster, posterPath); err != nil {
			errs = append(errs, err)
		} else {
			it.Poster = posterPath
		}
	}
	if backdropPath != "" && it.Backdrop != "" {
		if err := s.writeArt(ctx, it.Backdrop, backdropPath); err != nil {
			errs = append(errs, err)
		} else {
			it.Backdrop = backdropPath
		}
	}
	var ids []nfoUniqueID
	if it.ProviderID != "" && it.Provider != "" {
		ids = append(ids, nfoUniqueID{Type: it.Provider, Default: true, ID: it.ProviderID})
	}
	var doc any
	if it.ShowID != nil {
		doc = nfoEpisodeOut{Title: it.Title, ShowTitle: it.ShowTitle, Season: deref(it.Season), Episode: deref(it.Episode),
			Plot: it.Overview, Aired: it.Aired, Runtime: int(it.RuntimeSec / 60), Rating: derefF(it.Rating), UniqueIDs: ids}
	} else {
		m := nfoMovieOut{Title: it.Title, Year: deref(it.Year), Plot: it.Overview, Tagline: it.Tagline, Runtime: int(it.RuntimeSec / 60),
			MPAA: it.ContentRating, Rating: derefF(it.Rating), Genres: it.Genres, Studio: it.Studio, UniqueIDs: ids}
		if it.SortTitle != "" && it.SortTitle != it.Title {
			m.SortTitle = it.SortTitle
		}
		if isRemote(it.Poster) {
			m.Thumb = it.Poster
		}
		doc = m
	}
	if err := writeNFO(nfoPath, doc); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// SaveShowMetadata writes tvshow.nfo and show/season artwork into the show folder.
func (s *Scanner) SaveShowMetadata(ctx context.Context, sh *Show) error {
	var errs []error
	if sh.Poster != "" {
		dst := filepath.Join(sh.Path, "poster.jpg")
		if err := s.writeArt(ctx, sh.Poster, dst); err != nil {
			errs = append(errs, err)
		} else {
			sh.Poster = dst
		}
	}
	if sh.Backdrop != "" {
		dst := filepath.Join(sh.Path, "fanart.jpg")
		if err := s.writeArt(ctx, sh.Backdrop, dst); err != nil {
			errs = append(errs, err)
		} else {
			sh.Backdrop = dst
		}
	}
	if seasons, err := s.Store.Seasons(ctx, sh.ID, 0); err == nil {
		for _, se := range seasons {
			if se.Poster == "" {
				continue
			}
			name := fmt.Sprintf("season%02d-poster.jpg", se.Number)
			if se.Number == 0 {
				name = "season-specials-poster.jpg"
			}
			dst := filepath.Join(sh.Path, name)
			if err := s.writeArt(ctx, se.Poster, dst); err == nil {
				_ = s.Store.SetSeasonPoster(ctx, sh.ID, se.Number, dst, true)
			}
		}
	}
	doc := nfoShowOut{Title: sh.Title, Year: deref(sh.Year), Plot: sh.Overview, MPAA: sh.ContentRating, Rating: derefF(sh.Rating),
		Genres: sh.Genres, Studio: sh.Network, Status: sh.Status}
	if sh.SortTitle != "" && sh.SortTitle != sh.Title {
		doc.SortTitle = sh.SortTitle
	}
	if sh.Provider != "" && sh.ProviderID != "" {
		doc.UniqueIDs = []nfoUniqueID{{Type: sh.Provider, Default: true, ID: sh.ProviderID}}
		if sh.Provider == "tvdb" {
			doc.TVDBIDCompat = sh.ProviderID
		}
	}
	if err := writeNFO(filepath.Join(sh.Path, "tvshow.nfo"), doc); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// saveOnScan writes metadata for newly indexed items when enabled and the
// item has no NFO yet (existing NFOs are the user's and are never touched
// by a scan).
func (s *Scanner) saveOnScan(ctx context.Context, it *Item) {
	if s.SaveMeta == nil || !s.SaveMeta() {
		return
	}
	_, _, nfo := ArtPaths(it)
	if _, err := os.Stat(nfo); err == nil {
		return
	}
	if err := s.SaveItemMetadata(ctx, it); err != nil {
		s.Log.Debug("save metadata", "path", it.Path, "err", err)
	}
}

func (s *Scanner) saveShowOnScan(ctx context.Context, sh *Show) {
	if s.SaveMeta == nil || !s.SaveMeta() {
		return
	}
	if _, err := os.Stat(filepath.Join(sh.Path, "tvshow.nfo")); err == nil {
		return
	}
	if err := s.SaveShowMetadata(ctx, sh); err != nil {
		s.Log.Debug("save show metadata", "path", sh.Path, "err", err)
	}
	_ = s.Store.UpsertShow(ctx, sh)
}
