package library

import (
	"path/filepath"
	"strings"
)

var subtitleExts = map[string]bool{".srt": true, ".ass": true, ".ssa": true, ".vtt": true, ".sub": true, ".idx": true, ".sup": true}
var imageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}

func isImageName(name string) bool {
	return imageExts[strings.ToLower(filepath.Ext(name))] || IsPhotoFile(name)
}

// FileKind says what a file is to the server, from its name: video, audio,
// book, disc (an image of a DVD or Blu-ray), subtitle, image or other.
func FileKind(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case videoExts[ext]:
		return "video"
	case audioExts[ext]:
		return "audio"
	case bookExts[ext] > 0:
		return "book"
	case ext == ".iso" || ext == ".img":
		return "disc"
	case subtitleExts[ext]:
		return "subtitle"
	case isImageName(name):
		return "image"
	}
	return "other"
}
