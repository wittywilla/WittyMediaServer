// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// The guide key (Advanced settings) is masked when settings are read back, so whoever holds an
// administrator's sign-in can replace it but not read it. A link with "{key}" in it would undo
// that if it could go anywhere: the key would be sent to whatever site the link names. So the key
// goes to one site only: the first one a "{key}" link is read from after the key was saved
// (entering the key again lets it go to another), over HTTPS, never as part of the site's name,
// and never on to another site by a redirect.

// guideKeyHostSetting is the site the guide key was first used with. It isn't one of
// settingKeys, so the settings API can neither show nor set it.
const guideKeyHostSetting = "guideKeyHost"

// guideKeyMark stands in for "{key}" while a link's parts are checked.
const guideKeyMark = "wmsguidekeymark"

// guideKeyHost checks a "{key}" link and answers the site the key may be sent to: the one it was
// first used with, or this link's own, which is then kept.
func (s *Server) guideKeyHost(ctx context.Context, link string) (string, error) {
	u, err := url.Parse(strings.ReplaceAll(link, "{key}", guideKeyMark))
	if err != nil || u.Host == "" {
		return "", errors.New("a guide link starts with https://")
	}
	if u.Scheme != "https" {
		return "", errors.New("a link with your guide key in it must start with https://, so the key isn't sent in the clear")
	}
	host := strings.ToLower(u.Host)
	if strings.Contains(host, guideKeyMark) || (u.User != nil && strings.Contains(strings.ToLower(u.User.String()), guideKeyMark)) {
		return "", errors.New("your guide key can go in a link's path or query, not in the site's name")
	}
	bound := s.DB.GetSetting(ctx, guideKeyHostSetting, "")
	if bound == "" {
		if err := s.DB.SetSetting(ctx, guideKeyHostSetting, host); err != nil {
			return "", err
		}
		return host, nil
	}
	if bound != host {
		return "", errors.New("your guide key is used with " + bound + ". To use it with another site, enter the key again under Advanced")
	}
	return host, nil
}

// keyedGuideClient is SafeClient, refusing redirects to any site but host: the link, and so the
// key, would go along as the Referer.
func keyedGuideClient(host string) *http.Client {
	c := *library.SafeClient
	next := c.CheckRedirect
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !strings.EqualFold(req.URL.Host, host) || req.URL.Scheme != "https" {
			return errors.New("the guide's site sent the request to another site")
		}
		if next != nil {
			return next(req, via)
		}
		return nil
	}
	return &c
}
