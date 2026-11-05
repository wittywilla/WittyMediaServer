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
import './player.css'
import './oobe.css'
import './shell.css'
import './ui/ui.css'
import './audio.css'
import './pages/settings/settings.css'
import { installTVInput } from './tv'
import { installTenFoot } from './ui/tenFoot'
import { installErrorReporter } from './errorReport'

installTVInput()
installTenFoot()
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
