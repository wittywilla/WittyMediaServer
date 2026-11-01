package library

import "github.com/wittywilla/wittymediaserver/internal/mediatypes"

// FileKind says what a file is to the server, from its name: video, audio, book,
// disc (an image of a DVD or Blu-ray), subtitle, image, playlist or other.
func FileKind(name string) string {
	if k := mediatypes.Kind(name); k != mediatypes.Other {
		return k
	}
	if photos().IsRaw(name) { // a newer WittyPhotos may know more cameras
		return "image"
	}
	return mediatypes.Other
}
