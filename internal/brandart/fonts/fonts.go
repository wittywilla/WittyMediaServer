// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package fonts holds Inter 4.1 (https://github.com/rsms/inter, SIL Open Font License 1.1, in
// OFL.txt), the font the website and web client name first, so the words in generated pictures
// are the same letters people read on the site. Only scripts/genicon and the brand tests load it,
// so it adds nothing to the server or the tray.
package fonts

import _ "embed"

var (
	//go:embed Inter-Bold.ttf
	InterBold []byte
	//go:embed Inter-Regular.ttf
	InterRegular []byte
)
