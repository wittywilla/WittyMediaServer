// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"slices"
)

// WittyTV's look belongs to WittyTuner (docs/SETUP-FLOW.md, "WittyTV after setup"). Whenever
// WittyTuner is freshly installed (not updated), the server asks for the guide's look once: in
// first-run setup that is a page of the Looks step; afterwards it is a "Finish setting up
// WittyTuner" notice on Home, in every app, until it is done or dismissed. Taking WittyTuner off
// forgets the question was answered, so installing it again asks again.

const (
	settingWittyTVLookAsk = "wittytvLookAsk"
	wittyTVLookStep       = "wittytv-look"
)

// noteModulesInstalling is told which modules are about to be installed: a WittyTuner that isn't
// here yet is a fresh install, and its look is to be asked about.
func (s *Server) noteModulesInstalling(ctx context.Context, ids []string) {
	if s.Modules == nil || !slices.Contains(ids, tunerModule) || s.Modules.Installed(tunerModule) {
		return
	}
	_ = s.DB.SetSetting(ctx, settingWittyTVLookAsk, "1")
}

// forgetWittyTVLook: WittyTuner was taken off, so whatever was chosen is asked again next time.
func (s *Server) forgetWittyTVLook(ctx context.Context) {
	_ = s.DB.SetSetting(ctx, settingWittyTVLookAsk, "")
}

// wittyTVLookAnswered: chosen, or put off, in setup or from the notice.
func (s *Server) wittyTVLookAnswered(ctx context.Context) error {
	return s.DB.SetSetting(ctx, settingWittyTVLookAsk, "0")
}

// wittyTVLookPending is the step while WittyTuner is in use and its look hasn't been asked.
func (s *Server) wittyTVLookPending(ctx context.Context) []pendingModuleStep {
	if s.Modules == nil || s.DB.GetSetting(ctx, settingWittyTVLookAsk, "") != "1" || !s.Modules.Installed(tunerModule) {
		return nil
	}
	return []pendingModuleStep{wittyTVLookNotice()}
}

func wittyTVLookNotice() pendingModuleStep {
	return pendingModuleStep{Module: tunerModule, Name: "WittyTuner", ID: wittyTVLookStep, Title: "Choosing how the TV guide looks",
		Later: "Settings, Appearance, under WittyTV", To: "/settings/appearance#wittytv", Open: "/settings/appearance#wittytv"}
}
