package main

import "testing"

// A misspelt command must not start a server: in a container, that's a second
// server on the same data.
func TestUnknownCommandDoesntServe(t *testing.T) {
	for _, word := range []string{"hardwar", "helth", "start"} {
		if got := run([]string{word}); got != 2 {
			t.Errorf("%s: exit %d", word, got)
		}
	}
}
