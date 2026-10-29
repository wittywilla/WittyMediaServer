// The few icons the reader uses (inline SVG, so the module needs no icon set).
type P = { size?: number }
const S = ({ d, size = 20 }: { d: string; size?: number }) => (
  <svg viewBox="0 0 24 24" width={size} height={size} fill="currentColor" aria-hidden="true"><path d={d} /></svg>
)
export const IconBack = (p: P) => <S {...p} d="M20 11H7.8l5.6-5.6L12 4l-8 8 8 8 1.4-1.4L7.8 13H20z" />
export const IconBook = (p: P) => <S {...p} d="M18 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V4a2 2 0 00-2-2zM6 4h5v8l-2.5-1.5L6 12z" />
export const IconSearch = (p: P) => <S {...p} d="M15.5 14h-.8l-.3-.3A6.5 6.5 0 1014 15.5l.3.3v.8l5 5 1.5-1.5zm-6 0a4.5 4.5 0 110-9 4.5 4.5 0 010 9z" />
export const IconSettings = (p: P) => <S {...p} d="M19.4 13a7.9 7.9 0 000-2l2.1-1.6-2-3.5-2.5 1a8 8 0 00-1.7-1L15 3H9l-.3 2.9a8 8 0 00-1.7 1l-2.5-1-2 3.5L4.6 11a7.9 7.9 0 000 2l-2.1 1.6 2 3.5 2.5-1a8 8 0 001.7 1L9 21h6l.3-2.9a8 8 0 001.7-1l2.5 1 2-3.5zM12 15.5a3.5 3.5 0 110-7 3.5 3.5 0 010 7z" />
