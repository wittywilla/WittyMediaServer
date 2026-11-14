// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The app's own parts a module's web bundle may use (web host API 5): what WittyPlay's screens, moved
// out of the app, still import from it. A bundle imports them as @wms/app/<path>, which WittyModule's
// builder turns into a stand-in reading this map (moduleHost.ts, app). scripts/export-module-kit.mjs
// writes each one's exports for the builder. Removing one, or an export of one, breaks bundles already
// released: add, never take away, until the next web host API.

import * as m0 from './App'
import * as m1 from './api'
import * as m2 from './components/ArtworkPicker'
import * as m3 from './components/BrandLogo'
import * as m4 from './components/ConsoleMark'
import * as m5 from './components/HelpLink'
import * as m6 from './components/LegalNote'
import * as m7 from './components/LibraryHeader'
import * as m8 from './components/ModuleMissing'
import * as m9 from './components/ShareDialog'
import * as m10 from './components/UploadButton'
import * as m11 from './components/WebLink'
import * as m12 from './components/common'
import * as m13 from './components/edit/LogoChooser'
import * as m14 from './errorReport'
import * as m15 from './gallery'
import * as m16 from './icons'
import * as m17 from './moduleBundles'
import * as m18 from './myLimits'
import * as m19 from './pages/SettingsPage'
import * as m20 from './pages/games/VirtualGames'
import * as m21 from './pages/settings/ModuleSettings'
import * as m22 from './pages/settings/ui'
import * as m23 from './pages/setup/parts'
import * as m24 from './pages/setup/plan'
import * as m25 from './player/InlinePlayer'
import * as m26 from './remote/PlayOnButton'
import * as m27 from './remote/RemoteHost'
import * as m28 from './remote/pageTargets'
import * as m29 from './sdui/SduiPage'
import * as m30 from './sdui/device'
import * as m31 from './sdui/regions'
import * as m32 from './sdui/render/cardRoute'
import * as m33 from './sdui/render/glyphs'
import * as m34 from './sdui/render/icons'
import * as m35 from './sdui/types'
import * as m36 from './shell/navModel'
import * as m37 from './shell/useNav'
import * as m38 from './spec/SpecPage'
import * as m39 from './spec/model'
import * as m40 from './spec/specs/albums'
import * as m41 from './spec/specs/consoleCores'
import * as m42 from './spec/specs/emulatorChoices'
import * as m43 from './spec/specs/fixgame'
import * as m44 from './spec/specs/gameLibrary'
import * as m45 from './spec/specs/gameReview'
import * as m46 from './spec/specs/manualEmulator'
import * as m47 from './spec/specs/media'
import * as m48 from './state'
import * as m49 from './theme'
import * as m50 from './tv'
import * as m51 from './ui/Button'
import * as m52 from './ui/Overlay'
import * as m53 from './ui/Surface'
import * as m54 from './ui/gamepad'
import * as m55 from './ui/index'
import * as m56 from './ui/spatial'
import * as m57 from './ui/userPrefs'

export const appModules: Record<string, unknown> = {
  'App': m0,
  'api': m1,
  'components/ArtworkPicker': m2,
  'components/BrandLogo': m3,
  'components/ConsoleMark': m4,
  'components/HelpLink': m5,
  'components/LegalNote': m6,
  'components/LibraryHeader': m7,
  'components/ModuleMissing': m8,
  'components/ShareDialog': m9,
  'components/UploadButton': m10,
  'components/WebLink': m11,
  'components/common': m12,
  'components/edit/LogoChooser': m13,
  'errorReport': m14,
  'gallery': m15,
  'icons': m16,
  'moduleBundles': m17,
  'myLimits': m18,
  'pages/SettingsPage': m19,
  'pages/games/VirtualGames': m20,
  'pages/settings/ModuleSettings': m21,
  'pages/settings/ui': m22,
  'pages/setup/parts': m23,
  'pages/setup/plan': m24,
  'player/InlinePlayer': m25,
  'remote/PlayOnButton': m26,
  'remote/RemoteHost': m27,
  'remote/pageTargets': m28,
  'sdui/SduiPage': m29,
  'sdui/device': m30,
  'sdui/regions': m31,
  'sdui/render/cardRoute': m32,
  'sdui/render/glyphs': m33,
  'sdui/render/icons': m34,
  'sdui/types': m35,
  'shell/navModel': m36,
  'shell/useNav': m37,
  'spec/SpecPage': m38,
  'spec/model': m39,
  'spec/specs/albums': m40,
  'spec/specs/consoleCores': m41,
  'spec/specs/emulatorChoices': m42,
  'spec/specs/fixgame': m43,
  'spec/specs/gameLibrary': m44,
  'spec/specs/gameReview': m45,
  'spec/specs/manualEmulator': m46,
  'spec/specs/media': m47,
  'state': m48,
  'theme': m49,
  'tv': m50,
  'ui/Button': m51,
  'ui/Overlay': m52,
  'ui/Surface': m53,
  'ui/gamepad': m54,
  'ui/index': m55,
  'ui/spatial': m56,
  'ui/userPrefs': m57,
}
