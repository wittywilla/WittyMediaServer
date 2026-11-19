// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { t } from '../i18n'
import { getTvMode } from '../tvmode/mode'

// Editing scenes and skips is for a computer: the web app in a browser on a desktop or laptop,
// or the desktop app. TV mode and the TV apps don't offer it, and say so: a remote can't place
// a time to the frame, and a TV's hardware isn't made for scrubbing a film while editing.
// Files made on a computer play the same on every TV.

export const editorAllowed = () => !getTvMode()

export const desktopOnlyWords = () =>
  t('Editing scenes and skips is limited to desktop: open this title in the web app on a computer, or in the desktop app. A TV’s hardware and remote can’t place times precisely enough. What you make there plays here too.')
