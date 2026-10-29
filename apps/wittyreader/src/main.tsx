import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@wittywilla/wittyink/kit.css'
import './app.css'
import App from './App'

// The service worker serves unpacked EPUBs and keeps the app working offline.
if ('serviceWorker' in navigator) navigator.serviceWorker.register('sw.js').catch(() => {})

createRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>)
