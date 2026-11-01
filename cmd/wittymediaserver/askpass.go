package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// errCantHide: what's typed here would show on the screen, so it isn't asked for.
var errCantHide = errors.New("this terminal can't hide what's typed, so it won't ask for a password here. Use --password-stdin to pipe it in instead")

// restoreOnSignal runs restore when stop is called, or first if the program is told to end,
// which would otherwise leave the terminal the way it was set for the prompt.
func restoreOnSignal(restore func()) (stop func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	var once sync.Once
	done := make(chan struct{})
	go func() {
		select {
		case <-sig:
			once.Do(restore)
			fmt.Fprintln(os.Stderr)
			os.Exit(130)
		case <-done:
		}
	}()
	return func() {
		signal.Stop(sig)
		close(done)
		once.Do(restore)
	}
}
