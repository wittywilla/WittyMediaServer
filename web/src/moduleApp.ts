// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The app's own parts a module's web bundle may use (web host API 5): what WittyPlay's screens, moved
// out of the app, still import from it. A bundle imports them as @wms/app/<path>, which WittyModule's
// builder turns into a stand-in reading this map (moduleHost.ts, app). scripts/export-module-kit.mjs
// writes each one's exports for the builder. Removing one, or an export of one, breaks bundles already
// released: add, never take away, until the next web host API.

import * as m0 from './App'
import * as m1 from './a11y'
import * as m2 from './api'
import * as m3 from './audio/player'
import * as m4 from './audio/visualizer'
import * as m5 from './components/ArtworkPicker'
import * as m6 from './components/BrandLogo'
import * as m7 from './components/ConsoleMark'
import * as m8 from './components/HelpLink'
import * as m9 from './components/LegalNote'
import * as m10 from './components/LibraryHeader'
import * as m11 from './components/ModuleMissing'
import * as m12 from './components/ShareDialog'
import * as m13 from './components/UploadButton'
import * as m14 from './components/WebLink'
import * as m15 from './components/common'
import * as m16 from './components/edit/LogoChooser'
import * as m17 from './errorReport'
import * as m18 from './gallery'
import * as m19 from './icons'
import * as m20 from './moduleBundles'
import * as m21 from './myLimits'
import * as m22 from './pages/SettingsPage'
import * as m23 from './pages/games/VirtualGames'
import * as m24 from './pages/settings/ModuleSettings'
import * as m25 from './pages/settings/ui'
import * as m26 from './pages/setup/parts'
import * as m27 from './pages/setup/plan'
import * as m28 from './player/Controls'
import * as m29 from './player/InlinePlayer'
import * as m30 from './player/InsertDisc'
import * as m31 from './player/Remote'
import * as m32 from './player/TrackPicker'
import * as m33 from './player/ViewSettings'
import * as m34 from './player/chrome'
import * as m35 from './player/hooks'
import * as m36 from './player/remoteKeys'
import * as m37 from './player/speed'
import * as m38 from './player/viewPrefs'
import * as m39 from './remote/PlayOnButton'
import * as m40 from './remote/RemoteHost'
import * as m41 from './remote/pageTargets'
import * as m42 from './remote/playerStates'
import * as m43 from './remote/target'
import * as m44 from './sdui/SduiPage'
import * as m45 from './sdui/device'
import * as m46 from './sdui/regions'
import * as m47 from './sdui/render/cardRoute'
import * as m48 from './sdui/render/glyphs'
import * as m49 from './sdui/render/icons'
import * as m50 from './sdui/types'
import * as m51 from './shell/navModel'
import * as m52 from './shell/useNav'
import * as m53 from './spec/SpecPage'
import * as m54 from './spec/model'
import * as m55 from './spec/specs/albums'
import * as m56 from './spec/specs/consoleCores'
import * as m57 from './spec/specs/emulatorChoices'
import * as m58 from './spec/specs/fixgame'
import * as m59 from './spec/specs/gameLibrary'
import * as m60 from './spec/specs/gameReview'
import * as m61 from './spec/specs/manualEmulator'
import * as m62 from './spec/specs/media'
import * as m63 from './state'
import * as m64 from './theme'
import * as m65 from './tv'
import * as m66 from './tvmode/mode'
import * as m67 from './ui/Button'
import * as m68 from './ui/Overlay'
import * as m69 from './ui/Surface'
import * as m70 from './ui/gamepad'
import * as m71 from './ui/index'
import * as m72 from './ui/spatial'
import * as m73 from './ui/userPrefs'

export const appModules: Record<string, unknown> = {
  'App': m0,
  'a11y': m1,
  'api': m2,
  'audio/player': m3,
  'audio/visualizer': m4,
  'components/ArtworkPicker': m5,
  'components/BrandLogo': m6,
  'components/ConsoleMark': m7,
  'components/HelpLink': m8,
  'components/LegalNote': m9,
  'components/LibraryHeader': m10,
  'components/ModuleMissing': m11,
  'components/ShareDialog': m12,
  'components/UploadButton': m13,
  'components/WebLink': m14,
  'components/common': m15,
  'components/edit/LogoChooser': m16,
  'errorReport': m17,
  'gallery': m18,
  'icons': m19,
  'moduleBundles': m20,
  'myLimits': m21,
  'pages/SettingsPage': m22,
  'pages/games/VirtualGames': m23,
  'pages/settings/ModuleSettings': m24,
  'pages/settings/ui': m25,
  'pages/setup/parts': m26,
  'pages/setup/plan': m27,
  'player/Controls': m28,
  'player/InlinePlayer': m29,
  'player/InsertDisc': m30,
  'player/Remote': m31,
  'player/TrackPicker': m32,
  'player/ViewSettings': m33,
  'player/chrome': m34,
  'player/hooks': m35,
  'player/remoteKeys': m36,
  'player/speed': m37,
  'player/viewPrefs': m38,
  'remote/PlayOnButton': m39,
  'remote/RemoteHost': m40,
  'remote/pageTargets': m41,
  'remote/playerStates': m42,
  'remote/target': m43,
  'sdui/SduiPage': m44,
  'sdui/device': m45,
  'sdui/regions': m46,
  'sdui/render/cardRoute': m47,
  'sdui/render/glyphs': m48,
  'sdui/render/icons': m49,
  'sdui/types': m50,
  'shell/navModel': m51,
  'shell/useNav': m52,
  'spec/SpecPage': m53,
  'spec/model': m54,
  'spec/specs/albums': m55,
  'spec/specs/consoleCores': m56,
  'spec/specs/emulatorChoices': m57,
  'spec/specs/fixgame': m58,
  'spec/specs/gameLibrary': m59,
  'spec/specs/gameReview': m60,
  'spec/specs/manualEmulator': m61,
  'spec/specs/media': m62,
  'state': m63,
  'theme': m64,
  'tv': m65,
  'tvmode/mode': m66,
  'ui/Button': m67,
  'ui/Overlay': m68,
  'ui/Surface': m69,
  'ui/gamepad': m70,
  'ui/index': m71,
  'ui/spatial': m72,
  'ui/userPrefs': m73,
}
