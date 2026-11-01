import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
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
import { installErrorReporter } from './errorReport'

installTVInput()
installErrorReporter()

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ErrorBoundary>
  </React.StrictMode>,
)
