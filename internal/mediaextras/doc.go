// Package mediaextras finds and keeps the things that go with a media file without being
// part of it: a short looping video shown behind a song or a title (a "canvas"), the
// separated parts of a song for karaoke ("stems"), and where the intro, recap, credits and
// adverts are in a video ("markers").
//
// Everything here is plain functions over paths and bytes. It never decides who may see
// what: the server checks that before asking, and opens files beside media only through
// its own library checks. What an administrator uploads is kept in the server's own
// folder, never beside the media.
package mediaextras
