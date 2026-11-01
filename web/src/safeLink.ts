// Addresses that came from somewhere else: a metadata source, a station, a feed, a module.
// Only web addresses become links. The app's policy already stops a javascript: address from
// running, but not one that opens another program on the machine (search-ms:, ms-msdt:, and
// whatever else is installed), or a data: page. '' for anything else, relative addresses
// included: nothing from outside has a reason to point into this app.
export function safeLink(u?: string | null): string {
  if (!u) return ''
  try {
    const p = new URL(u) // reads it as the browser will: no tabs or odd capitals to hide behind
    return p.protocol === 'https:' || p.protocol === 'http:' ? p.href : ''
  } catch {
    return ''
  }
}
