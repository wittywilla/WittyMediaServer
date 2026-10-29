import { useEffect } from 'react'

// "Insert DVD": the tray opens, the disc drops in, the tray closes and the
// drive spins up. Sounds are synthesised with Web Audio (no audio files).

const DURATION_MS = 3000

function playDriveSounds() {
  const AC = (window as any).AudioContext || (window as any).webkitAudioContext
  if (!AC) return () => {}
  let ctx: AudioContext
  try { ctx = new AC() } catch { return () => {} }
  ctx.resume?.().catch(() => {})
  const t0 = ctx.currentTime + 0.05
  const master = ctx.createGain()
  master.gain.value = 0.35
  master.connect(ctx.destination)

  const noise = (len: number) => {
    const buf = ctx.createBuffer(1, Math.max(1, Math.floor(ctx.sampleRate * len)), ctx.sampleRate)
    const d = buf.getChannelData(0)
    for (let i = 0; i < d.length; i++) d[i] = Math.random() * 2 - 1
    const src = ctx.createBufferSource()
    src.buffer = buf
    return src
  }
  // Tray motor: band-passed noise with a low hum, twice (open, close).
  const motor = (at: number, dur: number) => {
    const n = noise(dur)
    const bp = ctx.createBiquadFilter()
    bp.type = 'bandpass'; bp.frequency.value = 900; bp.Q.value = 2.5
    const g = ctx.createGain()
    g.gain.setValueAtTime(0, at); g.gain.linearRampToValueAtTime(0.5, at + 0.05); g.gain.setValueAtTime(0.5, at + dur - 0.08); g.gain.linearRampToValueAtTime(0, at + dur)
    n.connect(bp).connect(g).connect(master)
    n.start(at); n.stop(at + dur)
    const o = ctx.createOscillator()
    o.type = 'square'; o.frequency.setValueAtTime(70, at)
    const og = ctx.createGain()
    og.gain.setValueAtTime(0, at); og.gain.linearRampToValueAtTime(0.05, at + 0.05); og.gain.linearRampToValueAtTime(0, at + dur)
    o.connect(og).connect(master)
    o.start(at); o.stop(at + dur)
  }
  // Mechanical click.
  const click = (at: number, level = 0.8) => {
    const n = noise(0.03)
    const hp = ctx.createBiquadFilter()
    hp.type = 'highpass'; hp.frequency.value = 1800
    const g = ctx.createGain()
    g.gain.setValueAtTime(level, at); g.gain.exponentialRampToValueAtTime(0.001, at + 0.03)
    n.connect(hp).connect(g).connect(master)
    n.start(at); n.stop(at + 0.04)
  }
  // Spin-up: rising whine with a little noise.
  const spin = (at: number, dur: number) => {
    const o = ctx.createOscillator()
    o.type = 'sawtooth'
    o.frequency.setValueAtTime(60, at); o.frequency.exponentialRampToValueAtTime(520, at + dur)
    const lp = ctx.createBiquadFilter()
    lp.type = 'lowpass'; lp.frequency.setValueAtTime(400, at); lp.frequency.linearRampToValueAtTime(1600, at + dur)
    const g = ctx.createGain()
    g.gain.setValueAtTime(0, at); g.gain.linearRampToValueAtTime(0.12, at + 0.3); g.gain.setValueAtTime(0.12, at + dur - 0.3); g.gain.linearRampToValueAtTime(0, at + dur)
    o.connect(lp).connect(g).connect(master)
    o.start(at); o.stop(at + dur)
  }
  click(t0, 0.5)
  motor(t0 + 0.02, 0.55)
  click(t0 + 0.6)
  click(t0 + 1.15, 0.6) // disc lands
  motor(t0 + 1.35, 0.55)
  click(t0 + 1.92)
  spin(t0 + 2.0, 1.0)
  const end = setTimeout(() => ctx.close().catch(() => {}), DURATION_MS + 800)
  return () => { clearTimeout(end); ctx.close().catch(() => {}) }
}

export default function InsertDisc({ poster, title, onDone }: { poster?: string; title: string; onDone: () => void }) {
  useEffect(() => {
    const stop = playDriveSounds()
    const t = setTimeout(onDone, DURATION_MS)
    return () => { clearTimeout(t); stop() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  // Drawn in three layers so the tray really slides out from under the
  // player's lid: lid (front-most) > tray with its recess and the disc >
  // front panel. All sizes are in cqw of the drive, so it scales anywhere.
  return (
    <div className="insert-disc" onClick={(e) => { e.stopPropagation(); onDone() }} title="Click to skip">
      <div className="drive">
        <div className="lid"><span className="lid-shine" /></div>
        <div className="tray">
          <div className="tray-top">
            <div className="recess" />
            <div className="disc">
              <div className="disc-label" style={poster ? { backgroundImage: `url(${poster})` } : undefined}>{!poster && <span>{title}</span>}</div>
              <div className="disc-hole" />
            </div>
          </div>
          <div className="tray-front" />
        </div>
        <div className="front">
          <span className="slot" />
          <span className="led" />
          <span className="front-label">DVD</span>
          <span className="front-display">{'LOADING'}</span>
        </div>
      </div>
      <div className="insert-caption">Loading {title}…</div>
    </div>
  )
}
