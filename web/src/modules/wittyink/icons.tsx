// The icons the readers use (inline SVG, so the module needs no icon set and
// every system draws them the same, unlike emoji). Filled icons for the bars;
// line icons for the handwriting tools.
type P = { size?: number }
const S = ({ d, size = 20 }: { d: string } & P) => (
  <svg viewBox="0 0 24 24" width={size} height={size} fill="currentColor" aria-hidden="true"><path d={d} /></svg>
)
const L = ({ d, size = 22 }: { d: string } & P) => (
  <svg viewBox="0 0 24 24" width={size} height={size} fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d={d} /></svg>
)
export const IconBack = (p: P) => <S {...p} d="M20 11H7.8l5.6-5.6L12 4l-8 8 8 8 1.4-1.4L7.8 13H20z" />
export const IconBook = (p: P) => <S {...p} d="M18 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V4a2 2 0 00-2-2zM6 4h5v8l-2.5-1.5L6 12z" />
export const IconContents = (p: P) => <S {...p} d="M3 9h14V7H3zm0 4h14v-2H3zm0 4h14v-2H3zm16 0h2v-2h-2zm0-10v2h2V7zm0 6h2v-2h-2z" />
export const IconSearch = (p: P) => <S {...p} d="M15.5 14h-.8l-.3-.3a6.5 6.5 0 10-.7.7l.3.3v.8l5 5 1.5-1.5zm-6 0a4.5 4.5 0 110-9 4.5 4.5 0 010 9z" />
export const IconSettings = (p: P) => <S {...p} d="M19.4 13a7.9 7.9 0 000-2l2.1-1.6-2-3.5-2.5 1a8 8 0 00-1.7-1L15 3H9l-.3 2.9a8 8 0 00-1.7 1l-2.5-1-2 3.5L4.6 11a7.9 7.9 0 000 2l-2.1 1.6 2 3.5 2.5-1a8 8 0 001.7 1L9 21h6l.3-2.9a8 8 0 001.7-1l2.5 1 2-3.5zM12 15.5a3.5 3.5 0 110-7 3.5 3.5 0 010 7z" />
export const IconNotes = (p: P) => <S {...p} d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8zm2 16H8v-2h8zm0-4H8v-2h8zm-3-5V3.5L18.5 9z" />
export const IconBookmark = ({ on, ...p }: P & { on?: boolean }) => (
  <S {...p} d={on ? 'M17 3H7a2 2 0 00-2 2v16l7-3 7 3V5a2 2 0 00-2-2z' : 'M17 3H7a2 2 0 00-2 2v16l7-3 7 3V5a2 2 0 00-2-2zm0 15l-5-2.2L7 18V5h10z'} />
)
export const IconSpeaker = (p: P) => <S {...p} d="M3 9v6h4l5 5V4L7 9zm13.5 3A4.5 4.5 0 0014 8v8a4.5 4.5 0 002.5-4zM14 3.2v2.1a7 7 0 010 13.4v2.1a9 9 0 000-17.6z" />
export const IconStop = (p: P) => <S {...p} d="M7 7h10v10H7z" />
export const IconClose = (p: P) => <S {...p} d="M19 6.4L17.6 5 12 10.6 6.4 5 5 6.4 10.6 12 5 17.6 6.4 19 12 13.4 17.6 19 19 17.6 13.4 12z" />
export const IconPrev = (p: P) => <S {...p} d="M15.4 7.4L14 6l-6 6 6 6 1.4-1.4-4.6-4.6z" />
export const IconNext = (p: P) => <S {...p} d="M10 6L8.6 7.4l4.6 4.6-4.6 4.6L10 18l6-6z" />
export const IconUndo = (p: P) => <S {...p} d="M12.5 8a10.4 10.4 0 00-6.9 2.6L2 7v9h9l-3.6-3.6a8 8 0 0112.7 3.6l2.4-.8A10.5 10.5 0 0012.5 8z" />
export const IconRedo = (p: P) => <S {...p} d="M18.4 10.6A10.4 10.4 0 0011.5 8a10.5 10.5 0 00-10 7.2l2.4.8a8 8 0 0112.7-3.6L13 16h9V7z" />
export const IconMore = (p: P) => <S {...p} d="M6 10a2 2 0 100 4 2 2 0 000-4zm12 0a2 2 0 100 4 2 2 0 000-4zm-6 0a2 2 0 100 4 2 2 0 000-4z" />
export const IconZoomIn = (p: P) => <S {...p} d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6z" />
export const IconZoomOut = (p: P) => <S {...p} d="M19 13H5v-2h14z" />
export const IconHandwriting = (p: P) => <S {...p} d="M3 17.3V21h3.8L17.8 9.9l-3.7-3.7zM20.7 7a1 1 0 000-1.4l-2.3-2.3a1 1 0 00-1.4 0l-1.8 1.8 3.7 3.7z" />
export const IconPen = (p: P) => <L {...p} d="M12 3l5.5 7L12 21 6.5 10zM12 21v-8.5M12 12.5a1.3 1.3 0 100-.01" />
export const IconPencil = (p: P) => <L {...p} d="M15.5 4.5l4 4L8 20l-4.5.5L4 16zM13 7l4 4M4 16l4 4" />
export const IconMarker = (p: P) => <L {...p} d="M14 4l6 6-8 8H7v-5zM7 13l5 5M4 21h8" />
export const IconEraser = (p: P) => <L {...p} d="M8 20h12M5.4 13.6l7.2-7.2a2 2 0 012.8 0l3.2 3.2a2 2 0 010 2.8L11 20H8.4l-3-3a2.4 2.4 0 010-3.4zM9 10l5.6 5.6" />
