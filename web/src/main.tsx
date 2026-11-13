// SPDX-License-Identifier: AGPL-3.0-or-later
//
// WittyMediaServer - A self-hosted media server that plays DVDs and Blu-rays with their real menus, in any browser.
// Copyright (C) 2026 WittyWilla and contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

import React from 'react'
import ReactDOM from 'react-dom/client'
import { createBrowserRouter, RouterProvider } from 'react-router-dom'
// Pages all over the app use these, and they come first so that styles.css has the last word.
import './pages/games.css'
import './pages/setup/setup.css'
import './pages/pair.css'
import App from './App'
import ErrorBoundary from './components/ErrorBoundary'
import './styles.css'
import './design2.css'
import './finish.css'

// After the server is updated, a page that was already open asks for parts of the old app that are gone. Load the new app
// instead, once (a second failure within a minute is a real fault, and the error page says so).
window.addEventListener('vite:preloadError', (e) => {
  try {
    const last = Number(sessionStorage.getItem('wms.staleReload') || 0)
    if (Date.now() - last < 60_000) return
    sessionStorage.setItem('wms.staleReload', String(Date.now()))
  } catch { /* storage off: still reload once for this event */ }
  e.preventDefault()
  window.location.reload()
})
import './player.css'
import './player/bar.css'
import './oobe.css'
import './shell.css'
import './ui/ui.css'
import './audio.css'
import './pages/settings/settings.css'
import { installTVInput } from './tv'
import { installTvMode } from './tvmode/mode'
import { installDensity } from './ui/density'
import './tvmode/tvmode.css'
import { installErrorReporter } from './errorReport'

installTVInput()
installDensity()
installTvMode()
installErrorReporter()

// One route that hands everything to App's own routes. A data router is what lets a page
// with unsaved changes ask before it is left (useBlocker); the boundary inside it keeps a
// page that fails showing this app's own error screen, not the router's.
const router = createBrowserRouter([{ path: '*', element: <ErrorBoundary><App /></ErrorBoundary> }])

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <RouterProvider router={router} />
    </ErrorBoundary>
  </React.StrictMode>,
)
