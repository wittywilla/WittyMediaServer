// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// A failure stays remembered, and the list of them can't grow without limit.
func TestThumbFailuresHaveALimit(t *testing.T) {
	thumbMu.Lock()
	defer thumbMu.Unlock()
	was := thumbFails
	defer func() { thumbFails = was }()
	thumbFails = map[string]thumbFailure{}
	bad := errors.New("can't")
	for i := range maxThumbFails + 100 {
		noteThumbFailure(fmt.Sprint(i), bad)
		time.Sleep(time.Nanosecond) // so they have an order
	}
	if len(thumbFails) > maxThumbFails {
		t.Fatalf("%d remembered", len(thumbFails))
	}
	if _, ok := thumbFails[fmt.Sprint(maxThumbFails+99)]; !ok {
		t.Fatal("the newest was forgotten")
	}
	if _, ok := thumbFails["0"]; ok {
		t.Fatal("the oldest stayed")
	}
}
