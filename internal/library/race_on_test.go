//go:build race

package library

const raceEnabled = true // the race detector runs code many times slower: timings mean nothing
