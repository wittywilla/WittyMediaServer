// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package config

import "golang.org/x/sys/windows"

// systemDocuments asks Windows where Documents is: OneDrive's folder when Known Folder Move put it
// there, or wherever someone moved it in its Properties.
func systemDocuments() string {
	p, err := windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DONT_VERIFY)
	if err != nil {
		return ""
	}
	return p
}
