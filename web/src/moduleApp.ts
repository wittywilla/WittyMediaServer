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
import * as m9 from './components/WebLink'
import * as m10 from './components/common'
import * as m11 from './components/edit/LogoChooser'
import * as m12 from './errorReport'
import * as m13 from './icons'
import * as m14 from './moduleBundles'
import * as m15 from './myLimits'
import * as m16 from './pages/SettingsPage'
import * as m17 from './pages/games/VirtualGames'
import * as m19 from './pages/settings/ModuleSettings'
import * as m21 from './pages/settings/ui'
import * as m22 from './pages/setup/parts'
import * as m23 from './pages/setup/plan'
import * as m24 from './remote/RemoteHost'
import * as m25 from './remote/pageTargets'
import * as m26 from './sdui/SduiPage'
import * as m27 from './sdui/device'
import * as m28 from './sdui/regions'
import * as m29 from './sdui/render/cardRoute'
import * as m30 from './sdui/render/glyphs'
import * as m31 from './sdui/render/icons'
import * as m32 from './sdui/types'
import * as m33 from './shell/navModel'
import * as m34 from './shell/useNav'
import * as m35 from './spec/SpecPage'
import * as m36 from './spec/model'
import * as m37 from './spec/specs/consoleCores'
import * as m38 from './spec/specs/emulatorChoices'
import * as m39 from './spec/specs/fixgame'
import * as m40 from './spec/specs/gameLibrary'
import * as m41 from './spec/specs/gameReview'
import * as m42 from './spec/specs/manualEmulator'
import * as m43 from './state'
import * as m44 from './theme'
import * as m45 from './tv'
import * as m46 from './ui/Overlay'
import * as m47 from './ui/Surface'
import * as m48 from './ui/gamepad'
import * as m49 from './ui/index'
import * as m50 from './ui/spatial'
import * as m51 from './ui/userPrefs'

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
  'components/WebLink': m9,
  'components/common': m10,
  'components/edit/LogoChooser': m11,
  'errorReport': m12,
  'icons': m13,
  'moduleBundles': m14,
  'myLimits': m15,
  'pages/SettingsPage': m16,
  'pages/games/VirtualGames': m17,
  'pages/settings/ModuleSettings': m19,
  'pages/settings/ui': m21,
  'pages/setup/parts': m22,
  'pages/setup/plan': m23,
  'remote/RemoteHost': m24,
  'remote/pageTargets': m25,
  'sdui/SduiPage': m26,
  'sdui/device': m27,
  'sdui/regions': m28,
  'sdui/render/cardRoute': m29,
  'sdui/render/glyphs': m30,
  'sdui/render/icons': m31,
  'sdui/types': m32,
  'shell/navModel': m33,
  'shell/useNav': m34,
  'spec/SpecPage': m35,
  'spec/model': m36,
  'spec/specs/consoleCores': m37,
  'spec/specs/emulatorChoices': m38,
  'spec/specs/fixgame': m39,
  'spec/specs/gameLibrary': m40,
  'spec/specs/gameReview': m41,
  'spec/specs/manualEmulator': m42,
  'state': m43,
  'theme': m44,
  'tv': m45,
  'ui/Overlay': m46,
  'ui/Surface': m47,
  'ui/gamepad': m48,
  'ui/index': m49,
  'ui/spatial': m50,
  'ui/userPrefs': m51,
}
