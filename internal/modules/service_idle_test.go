// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"testing"
	"time"
)

// A service answering a call, however long it takes, isn't idle: a scan's last call can group a
// whole library for longer than serviceIdle, and stopping the service would end it.
func TestServiceInUseIsNotIdle(t *testing.T) {
	m := &Manager{services: map[string]*runningService{}}
	long := time.Now().Add(-2 * serviceIdle)
	m.services["wittyplay"] = &runningService{last: long}
	m.services["wittyother"] = &runningService{last: long}
	done := m.Using("wittyplay")
	if idle := m.idleServices(time.Now()); idle["wittyplay"] != nil || idle["wittyother"] == nil {
		t.Fatalf("idle while a call is under way: %v", idle)
	}
	done()
	if idle := m.idleServices(time.Now()); idle["wittyplay"] != nil {
		t.Fatal("idle the moment its call ended")
	}
	if idle := m.idleServices(time.Now().Add(2 * serviceIdle)); idle["wittyplay"] == nil {
		t.Fatal("never idle once its call ended")
	}
	m.Using("not-running")() // nothing to mark
}
