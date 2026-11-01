import { useEffect, useRef } from 'react'
import { useLocation } from 'react-router-dom'
import { LookEditor } from './LookPanel'
import { useLook } from './look'
import './tv.css'

// WittyTV's look in Settings, Appearance: the same choices as the guide's Look panel.
export default function TvLookSettings() {
  const look = useLook()
  const box = useRef<HTMLDivElement | null>(null)
  const { hash } = useLocation()
  useEffect(() => { if (hash === '#wittytv') box.current?.scrollIntoView({ block: 'start' }) }, [hash])
  return (
    <div className="panel" id="wittytv" ref={box}>
      <h2>WittyTV</h2>
      <p className="muted">How the TV guide, its player, recordings and channel pages look for you, apart from the app's theme. It follows you to your other devices.</p>
      <LookEditor look={look} inSettings />
    </div>
  )
}
