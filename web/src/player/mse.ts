// Whether this browser can play HLS through Media Source: the question hls.js answers with its own
// isSupported, asked without loading hls.js, which is 177 KB compressed.
export function mediaSourceWorks(): boolean {
  const w = window as unknown as { MediaSource?: typeof MediaSource; ManagedMediaSource?: typeof MediaSource; WebKitMediaSource?: typeof MediaSource; SourceBuffer?: typeof SourceBuffer; WebKitSourceBuffer?: typeof SourceBuffer }
  const ms = w.MediaSource || w.ManagedMediaSource || w.WebKitMediaSource
  if (!ms || typeof ms.isTypeSupported !== 'function' || !ms.isTypeSupported('video/mp4; codecs="avc1.42E01E,mp4a.40.2"')) return false
  const sb = w.SourceBuffer || w.WebKitSourceBuffer
  return !sb || (!!sb.prototype && typeof sb.prototype.appendBuffer === 'function' && typeof sb.prototype.remove === 'function')
}
