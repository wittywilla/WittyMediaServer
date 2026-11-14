// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The app's own parts a module's web bundle may use (web host API 5): what WittyPlay's screens, moved
// out of the app, still import from it. A bundle imports them as @wms/app/<path>, which WittyModule's
// builder turns into a stand-in reading this map (moduleHost.ts, app). scripts/export-module-kit.mjs
// writes each one's exports for the builder. Removing one, or an export of one, breaks a bundle that
// still imports it: a module takes its own copy of a part that is its alone (as WittyPlay did of the
// game parts) before the part goes from here.

import * as m0 from './App'
import * as m1 from './a11y'
import * as m2 from './api'
import * as m3 from './audio/player'
import * as m4 from './audio/visualizer'
import * as m5 from './components/ArtworkPicker'
import * as m6 from './components/BrandLogo'
import * as m7 from './components/HelpLink'
import * as m8 from './components/LegalNote'
import * as m9 from './components/LibraryHeader'
import * as m10 from './components/ModuleMissing'
import * as m11 from './components/ShareDialog'
import * as m12 from './components/UploadButton'
import * as m13 from './components/WebLink'
import * as m14 from './components/common'
import * as m15 from './components/edit/LogoChooser'
import * as m16 from './errorReport'
import * as m17 from './gallery'
import * as m18 from './icons'
import * as m19 from './moduleBundles'
import * as m20 from './myLimits'
import * as m21 from './pages/SettingsPage'
import * as m22 from './pages/settings/ModuleSettings'
import * as m23 from './pages/settings/ui'
import * as m24 from './pages/setup/parts'
import * as m25 from './pages/setup/plan'
import * as m26 from './player/Controls'
import * as m27 from './player/InlinePlayer'
import * as m28 from './player/InsertDisc'
import * as m29 from './player/Remote'
import * as m30 from './player/TrackPicker'
import * as m31 from './player/ViewSettings'
import * as m32 from './player/chrome'
import * as m33 from './player/hooks'
import * as m34 from './player/remoteKeys'
import * as m35 from './player/speed'
import * as m36 from './player/viewPrefs'
import * as m37 from './remote/PlayOnButton'
import * as m38 from './remote/RemoteHost'
import * as m39 from './remote/pageTargets'
import * as m40 from './remote/playerStates'
import * as m41 from './remote/target'
import * as m42 from './sdui/SduiPage'
import * as m43 from './sdui/device'
import * as m44 from './sdui/regions'
import * as m45 from './sdui/render/cardRoute'
import * as m46 from './sdui/render/glyphs'
import * as m47 from './sdui/render/icons'
import * as m48 from './sdui/types'
import * as m49 from './shell/navModel'
import * as m50 from './shell/useNav'
import * as m51 from './spec/SpecPage'
import * as m52 from './spec/model'
import * as m53 from './spec/specs/albums'
import * as m54 from './spec/specs/media'
import * as m55 from './state'
import * as m56 from './theme'
import * as m57 from './tv'
import * as m58 from './tvmode/mode'
import * as m59 from './ui/Button'
import * as m60 from './ui/Overlay'
import * as m61 from './ui/Surface'
import * as m62 from './ui/gamepad'
import * as m63 from './ui/index'
import * as m64 from './ui/spatial'
import * as m65 from './ui/userPrefs'

export const appModules: Record<string, unknown> = {
  'App': m0,
  'a11y': m1,
  'api': m2,
  'audio/player': m3,
  'audio/visualizer': m4,
  'components/ArtworkPicker': m5,
  'components/BrandLogo': m6,
  'components/HelpLink': m7,
  'components/LegalNote': m8,
  'components/LibraryHeader': m9,
  'components/ModuleMissing': m10,
  'components/ShareDialog': m11,
  'components/UploadButton': m12,
  'components/WebLink': m13,
  'components/common': m14,
  'components/edit/LogoChooser': m15,
  'errorReport': m16,
  'gallery': m17,
  'icons': m18,
  'moduleBundles': m19,
  'myLimits': m20,
  'pages/SettingsPage': m21,
  'pages/settings/ModuleSettings': m22,
  'pages/settings/ui': m23,
  'pages/setup/parts': m24,
  'pages/setup/plan': m25,
  'player/Controls': m26,
  'player/InlinePlayer': m27,
  'player/InsertDisc': m28,
  'player/Remote': m29,
  'player/TrackPicker': m30,
  'player/ViewSettings': m31,
  'player/chrome': m32,
  'player/hooks': m33,
  'player/remoteKeys': m34,
  'player/speed': m35,
  'player/viewPrefs': m36,
  'remote/PlayOnButton': m37,
  'remote/RemoteHost': m38,
  'remote/pageTargets': m39,
  'remote/playerStates': m40,
  'remote/target': m41,
  'sdui/SduiPage': m42,
  'sdui/device': m43,
  'sdui/regions': m44,
  'sdui/render/cardRoute': m45,
  'sdui/render/glyphs': m46,
  'sdui/render/icons': m47,
  'sdui/types': m48,
  'shell/navModel': m49,
  'shell/useNav': m50,
  'spec/SpecPage': m51,
  'spec/model': m52,
  'spec/specs/albums': m53,
  'spec/specs/media': m54,
  'state': m55,
  'theme': m56,
  'tv': m57,
  'tvmode/mode': m58,
  'ui/Button': m59,
  'ui/Overlay': m60,
  'ui/Surface': m61,
  'ui/gamepad': m62,
  'ui/index': m63,
  'ui/spatial': m64,
  'ui/userPrefs': m65,
}
