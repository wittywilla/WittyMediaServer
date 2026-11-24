// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
)

// Joining by an invite through the server's own WittyRelay (#614). A relay
// admits only the keys the server lists with it, and a device that has never
// met the server isn't listed: its key is made on the device, and the server
// can't know it before the device shows the invite. So an invite has a key of
// its own, made from its secret: the enrol key. While the invite is open, the
// server lists that key with its relays too; the device connects with it,
// shows the invite, and names its own key, signed by it. The server lists the
// device's own key, stops listing the enrol key, and tells the relay at once.
//
// What this lets through: whoever holds the invite can reach the server
// through its relay, for at most as long as the invite lasts and only until
// it is used, and the server answers an unlisted key nothing but the invite
// itself. That is no more than the invite gives them anyway. The relay's rule
// is unchanged: it admits the keys the server lists, and no others.

// EnrolSeed is the enrol key's seed, made from an invite's secret.
func EnrolSeed(secret string) []byte {
	sum := sha256.Sum256([]byte("wittymedia relay enrol v1\n" + secret))
	return sum[:]
}

// EnrolID is the enrol key's peer ID, as the server lists it.
func EnrolID(secret string) (string, error) { return IDFromSeed(EnrolSeed(secret)) }

// enrolBinding is what a device signs with its own key: that this enrol key,
// for this server, may put it on the list. A signature made for one invite or
// one server is no use for another.
func enrolBinding(serverID, enrolID string) []byte {
	return []byte("wittymedia enrol for v1\n" + serverID + "\n" + enrolID)
}

// SignEnrolFor is the device's signature over enrolBinding, with its own key.
func SignEnrolFor(seed []byte, serverID, enrolID string) (string, error) {
	k, err := keyFromSeed(seed)
	if err != nil {
		return "", err
	}
	sig, err := k.Sign(enrolBinding(serverID, enrolID))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// ErrEnrolFor: an invite shown on behalf of another key that doesn't check out.
var ErrEnrolFor = errors.New("the invite named a device it couldn't speak for")

// CheckEnrolFor says whether a request shown by connID may put req.Device on
// the list instead: only the invite's own enrol key may, and only with the
// named device's signature over this server and that enrol key. It answers
// the ID to list.
func CheckEnrolFor(req EnrolRequest, connID, serverID string) (string, error) {
	if req.Device == "" {
		return connID, nil
	}
	enrolID, err := EnrolID(req.Secret)
	if err != nil || enrolID != connID || req.Device == connID {
		return "", ErrEnrolFor
	}
	dev, err := peer.Decode(req.Device)
	if err != nil {
		return "", ErrEnrolFor
	}
	pub, err := dev.ExtractPublicKey()
	if err != nil || pub.Type() != crypto.Ed25519 {
		return "", ErrEnrolFor
	}
	sig, err := base64.StdEncoding.DecodeString(req.DeviceSig)
	if err != nil {
		return "", ErrEnrolFor
	}
	if ok, err := pub.Verify(enrolBinding(serverID, connID), sig); err != nil || !ok {
		return "", ErrEnrolFor
	}
	return dev.String(), nil
}

// EnrolThroughRelay shows the server an invite from a node of the invite's own
// enrol key, for a device the server can't yet be reached by through its
// relay. cfg is the device's: its seed is what is put on the list, and its
// hints are where to look. The node is gone when this returns; the device then
// connects with its own key as usual.
func EnrolThroughRelay(ctx context.Context, cfg ClientConfig, secret, name, platform string) (EnrolResponse, error) {
	own, err := IDFromSeed(cfg.Seed)
	if err != nil {
		return EnrolResponse{}, err
	}
	enrolID, err := EnrolID(secret)
	if err != nil {
		return EnrolResponse{}, err
	}
	sig, err := SignEnrolFor(cfg.Seed, cfg.ServerID, enrolID)
	if err != nil {
		return EnrolResponse{}, err
	}
	ec := cfg
	ec.Seed = EnrolSeed(secret)
	ec.OnState = nil
	c, err := NewClient(ec)
	if err != nil {
		return EnrolResponse{}, err
	}
	defer c.Close()
	wctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if s, err := c.keeper.wait(wctx, State.Connected); err != nil {
		if s.Error != "" {
			return EnrolResponse{}, errors.New(s.Error)
		}
		return EnrolResponse{}, errors.New("the server wasn't found")
	}
	s, err := c.host.NewStream(c.allow(wctx), c.server, ProtoEnrol)
	if err != nil {
		return EnrolResponse{}, errors.New("the server takes no invites")
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(20 * time.Second))
	if err := writeMessage(s, EnrolRequest{V: 1, Secret: secret, Name: name, Platform: platform, Device: own, DeviceSig: sig}); err != nil {
		return EnrolResponse{}, err
	}
	var res EnrolResponse
	if err := readMessage(s, &res); err != nil {
		return EnrolResponse{}, errors.New("the server didn't answer the invite")
	}
	if res.OK {
		c.host.Peerstore().AddAddrs(c.server, parseAddrs(res.Addrs, c.server), peerstore.PermanentAddrTTL)
	}
	return res, nil
}
