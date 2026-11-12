// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'

// Thumb loads a scene picture. The server already tries two moments of the
// scene; a failed load (busy server, timeout) is retried twice before the
// tile says "Preview not available".
export function Thumb({ src }: { src: string }) {
  const [tries, setTries] = useState(0)
  const [state, setState] = useState<'loading' | 'ok' | 'failed'>('loading')
  const onError = () => {
    if (tries >= 2) { setState('failed'); return }
    setTimeout(() => setTries((t) => t + 1), 3000 * (tries + 1))
  }
  if (state === 'failed') return <span className="thumb-na">Preview not available</span>
  return <img key={tries} src={tries ? `${src}&retry=${tries}` : src} alt="" loading="lazy" onLoad={() => setState('ok')} onError={onError} className={state === 'ok' ? '' : 'loading'} />
}
