// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Voice search where the browser offers speech recognition of its own. The server
// never hears the voice: the browser turns it into text (its maker's speech service,
// as for any other site), and only the text is searched for.

type Recognition = {
  lang: string
  interimResults: boolean
  maxAlternatives: number
  continuous: boolean
  start(): void
  abort(): void
  onresult: ((e: { results: ArrayLike<ArrayLike<{ transcript: string }> & { isFinal: boolean }> }) => void) | null
  onerror: ((e: { error: string }) => void) | null
  onend: (() => void) | null
}

type RecognitionClass = new () => Recognition

let broken = false // it failed in a way that won't mend (no service, or no microphone allowed)

function recognition(): RecognitionClass | null {
  if (typeof window === 'undefined' || broken) return null
  // The desktop app's browser has the class but no service behind it.
  if (/Electron\//.test(navigator.userAgent)) return null
  const w = window as unknown as { SpeechRecognition?: RecognitionClass; webkitSpeechRecognition?: RecognitionClass }
  return w.SpeechRecognition || w.webkitSpeechRecognition || null
}

export function voiceSearchAvailable(): boolean {
  return recognition() !== null
}

// listen hears one search: onText gets what was heard so far and, at the end, the
// whole of it with final set. The function it returns stops listening.
export function listen(onText: (text: string, final: boolean) => void, onEnd: (failed?: string) => void): () => void {
  const R = recognition()
  if (!R) {
    onEnd('unavailable')
    return () => {}
  }
  const rec = new R()
  rec.lang = document.documentElement.lang || navigator.language || 'en'
  rec.interimResults = true
  rec.maxAlternatives = 1
  rec.continuous = false
  let failed: string | undefined
  rec.onresult = (e) => {
    let text = ''
    let final = false
    for (let i = 0; i < e.results.length; i++) {
      text += e.results[i][0]?.transcript || ''
      final = e.results[i].isFinal
    }
    onText(text.trim(), final)
  }
  rec.onerror = (e) => {
    failed = e.error
    if (e.error === 'network' || e.error === 'not-allowed' || e.error === 'service-not-allowed' || e.error === 'audio-capture') broken = true
  }
  rec.onend = () => onEnd(failed)
  try {
    rec.start()
  } catch {
    onEnd('start')
  }
  return () => {
    try { rec.abort() } catch { /* already stopped */ }
  }
}
