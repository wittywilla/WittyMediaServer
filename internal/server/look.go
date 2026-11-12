// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// ThemeLook is the finished look a client draws: the experience in force with what it
// brings (finish, type set, shape, motion), the theme, and who required which part.
type ThemeLook struct {
	Experience string `json:"experience"`
	Theme      string `json:"theme"`
	// RequiredExperience and RequiredTheme are set when an administrator requires the part:
	// who, when, and for whom (a profile, the kids or everyone). A client shows the part
	// locked: "Required by Willow".
	RequiredExperience *sdui.RequiredPart `json:"requiredExperience,omitempty"`
	RequiredTheme      *sdui.RequiredPart `json:"requiredTheme,omitempty"`
	// TVTheme is the WittyTV look the experience suggests.
	TVTheme string `json:"tvTheme"`
	// What the experience brings: see sdui.Experience.
	Effects sdui.Effects          `json:"effects"`
	Shape   sdui.ExperienceShape  `json:"shape"`
	Type    string                `json:"type"`
	Density string                `json:"density"`
	Motion  sdui.ExperienceMotion `json:"motion"`
	// Finish and Scene are the theme's glass and gloss recipes and its background scene.
	Finish themes.Finish `json:"finish"`
	Scene  themes.Scene  `json:"scene"`
	// ProfileThemes: this profile may import themes of its own.
	ProfileThemes bool `json:"profileThemes"`
}

// resolvedLook is a look and where its theme came from.
type resolvedLook struct {
	ThemeLook
	ThemeSource string
}

// lookFor works the look out for a request: the experience (required, the one asked for as a
// trial, the person's own, the server's, Lumen) and the theme (required, the trial asked
// for, the person's own, the server's own if it chose one, else the experience's default).
func (s *Server) lookFor(r *http.Request, cfg *sduistore.Config, choices themeChoices) resolvedLook {
	ctx := &sdui.RenderContext{}
	ff := sdui.FormWeb
	if s.sduiDevice(r, ctx) == nil && ctx.Profile != nil {
		ff = ctx.Profile.FormFactor
	}
	exp, err := s.sduiExperience(r, cfg, ff)
	if err != nil || exp == nil {
		exp = sdui.ExperienceByID(sdui.DefaultExperience(ff))
	}
	reqExp, reqTheme := s.requiredLook(r, cfg)
	out := resolvedLook{}
	out.Experience, out.RequiredExperience, out.RequiredTheme = exp.ID, reqExp, reqTheme
	// The theme.
	switch asked := strings.ToLower(strings.TrimSpace(sduiPick(r, "theme", "X-WMS-Theme"))); {
	case reqTheme != nil:
		out.Theme, out.ThemeSource = reqTheme.Value, "required"
	case asked != "" && s.themeKnown(asked):
		out.Theme, out.ThemeSource = asked, "trial"
	case choices.Theme != "":
		out.Theme, out.ThemeSource = choices.Theme, "user"
	case s.DB.GetSetting(r.Context(), "theme", "") != "":
		out.Theme, out.ThemeSource = s.serverTheme(r.Context()), "server"
	default:
		out.Theme, out.ThemeSource = exp.Theme, "server"
		if !s.themeKnown(out.Theme) {
			out.Theme = "default"
		}
	}
	out.TVTheme, out.Effects, out.Shape, out.Type, out.Density, out.Motion = exp.TVTheme, exp.Effects, exp.Shape, exp.Type, exp.Density, exp.Motion
	out.Finish, out.Scene = s.Themes.LookOf(out.Theme)
	out.ProfileThemes = cfg.Experiences != nil && cfg.Experiences.ProfileThemes
	return out
}
