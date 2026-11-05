// The colour of a name: a profile's avatar, and the placeholder for a title with no
// picture. The same rule in every app (WittyMediaClient's docs/APP-DESIGN.md, Avatar),
// so each person, and each title, keeps its colour on every device.
// All dark enough for white words (4.5:1).
export const AVATAR_COLORS = ['#c9302f', '#b8500d', '#99690a', '#2e7d3c', '#0e7c6f', '#0a6fb8', '#3e5bd0', '#7f46b8', '#bd2f85', '#8d6444']

export function avatarColor(name: string) {
  let h = 0
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) >>> 0
  return AVATAR_COLORS[h % AVATAR_COLORS.length]
}
