// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package datahome

import (
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"runtime"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/layout"
)

// Option is one of the places setup offers.
type Option struct {
	Choice    string `json:"choice"` // documents or install
	Path      string `json:"path"`
	Available bool   `json:"available"`
	Why       string `json:"why,omitempty"` // why it isn't available, in a sentence
	Free      int64  `json:"free"`
	// Synced names the program that keeps the folder in step with the cloud.
	Synced  string `json:"synced,omitempty"`
	Current bool   `json:"current,omitempty"`
}

// Options are the places setup offers, besides one someone types: Documents, then the install folder.
func Options(data string, need int64) []Option {
	return []Option{documentsOption(data, need), installOption(data, need)}
}

func documentsOption(data string, need int64) Option {
	o := Option{Choice: config.ChoiceDocuments, Path: config.DocumentsDataDir()}
	if o.Path == "" {
		o.Why = "This account has no Documents folder."
		return o
	}
	o.Free, o.Synced, o.Current = FreeBytes(o.Path), config.Synced(o.Path), Same(o.Path, data)
	if o.Current {
		o.Available = true
		return o
	}
	if p := CheckTarget(data, o.Path, need); p != "" {
		o.Why = p
		return o
	}
	o.Available = true
	return o
}

func installOption(data string, need int64) Option {
	o := Option{Choice: config.ChoiceInstall, Path: config.InstallDataDir()}
	o.Free, o.Current = FreeBytes(o.Path), Same(o.Path, data)
	if o.Current {
		o.Available = true
		return o
	}
	switch kind := layout.Current().Kind; {
	case runtime.GOOS == "darwin" && kind != layout.KindSource && kind != layout.KindArchive:
		o.Why = "A Mac app can't keep files inside itself: changing what's in it breaks its signature, and the Mac would refuse to open it."
		return o
	case kind == layout.KindAppImage || kind == layout.KindFlatpak || kind == layout.KindSnap:
		o.Why = "This copy of WittyMediaServer is one read-only file, so nothing can be kept inside it."
		return o
	case kind == layout.KindDeb || kind == layout.KindRPM || kind == layout.KindFreeBSD:
		o.Why = "The system's package manager looks after the install folder, and nothing else belongs in it."
		return o
	}
	if err := Writable(o.Path); err != nil {
		o.Why = i18n.F("This account can't make files in the install folder, {programDir}. A program installed for everyone, in Program Files, usually can't. Choose Documents or a folder of your own, or install WittyMediaServer for this account only.", "programDir", config.ProgramDir())
		return o
	}
	if p := CheckTarget(data, o.Path, need); p != "" {
		o.Why = p
		return o
	}
	o.Available = true
	return o
}

// Movable says whether the data folder can be moved from Settings, and if not, why: a folder
// set on the command line, in an environment variable or in a config file is changed there.
func Movable(from string) (bool, string) {
	switch from {
	case config.DataFromPointer, config.DataFromInstall, config.DataFromDefault:
		return true, ""
	case config.DataFromFlag:
		return false, i18n.T("The data folder is set with -data where the server is started (the service or a shortcut), so it's changed there.")
	case config.DataFromEnv:
		return false, "The data folder is set by the WMS_DATA_DIR environment variable, so it's changed there."
	case config.DataFromConfig, config.DataFromPortable:
		return false, i18n.T("The data folder is set in the config file, wms.json, so it's changed there.")
	case config.DataFromContainer:
		return false, "In a container the data folder is the /config volume: change it where the container is set up."
	case config.DataFromService:
		return false, i18n.T("This is the system service's data folder, which the package set up.")
	}
	return false, i18n.T("The data folder's place isn't known.")
}
