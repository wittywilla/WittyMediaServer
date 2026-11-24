// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
)

// publicRelay is an ordinary public relay, as found on the public network: libp2p's circuit relay
// with its default limits (a couple of minutes and a little data per connection), open to anyone,
// knowing nothing of WittyMediaServer. Not the owner's WittyRelay, which admits listed devices and
// carries whole connections. It prints its addresses, one per line, ending in /p2p/<id>, and runs
// until stopped.
func publicRelay(listen string) error {
	h, err := libp2p.New(libp2p.ListenAddrStrings(strings.Split(listen, ",")...), libp2p.ForceReachabilityPublic(),
		// As nodes on the public network do: they answer "can you reach me?" (AutoNAT), which is how a
		// server learns it is behind a router and turns to relays.
		libp2p.EnableNATService(), libp2p.EnableAutoNATv2(), libp2p.DisableRelay())
	if err != nil {
		return err
	}
	defer h.Close()
	r, err := relay.New(h)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, a := range h.Addrs() {
		fmt.Printf("relay address %s/p2p/%s\n", a, h.ID())
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	return nil
}
