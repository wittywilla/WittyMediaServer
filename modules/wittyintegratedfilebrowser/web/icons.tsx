// The browser's own icons (inline SVG paths), so the module needs nothing
// from the app it runs in.
type P = { size?: number }
const S = ({ d, size = 20 }: { d: string; size?: number }) => (
  <svg viewBox="0 0 24 24" width={size} height={size} fill="currentColor" aria-hidden="true" focusable="false"><path d={d} /></svg>
)
export const IconAudio = (p: P) => <S {...p} d="M12 3v10.6A4 4 0 1014 17V7h4V3z" />
export const IconBook = (p: P) => <S {...p} d="M18 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V4a2 2 0 00-2-2zM6 4h5v8l-2.5-1.5L6 12z" />
export const IconCheck = (p: P) => <S {...p} d="M9 16.2l-3.5-3.5L4 14.2 9 19.2 20 8.2l-1.4-1.4z" />
export const IconClose = (p: P) => <S {...p} d="M19 6.4L17.6 5 12 10.6 6.4 5 5 6.4l5.6 5.6L5 17.6 6.4 19l5.6-5.6 5.6 5.6 1.4-1.4-5.6-5.6z" />
export const IconDisc = (p: P) => <S {...p} d="M12 2a10 10 0 100 20 10 10 0 000-20zm0 13a3 3 0 110-6 3 3 0 010 6zm0-4.5a1.5 1.5 0 100 3 1.5 1.5 0 000-3z" />
export const IconDrive = (p: P) => <S {...p} d="M20 13H4a1 1 0 00-1 1v5a1 1 0 001 1h16a1 1 0 001-1v-5a1 1 0 00-1-1zM7 18a1.5 1.5 0 110-3 1.5 1.5 0 010 3zM20 4H4a1 1 0 00-1 1v5a1 1 0 001 1h16a1 1 0 001-1V5a1 1 0 00-1-1zM7 9a1.5 1.5 0 110-3 1.5 1.5 0 010 3z" />
export const IconFile = (p: P) => <S {...p} d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8l-6-6zm4 18H6V4h7v5h5v11z" />
export const IconFilm = (p: P) => <S {...p} d="M4 4h16v16H4zM6 6v2h2V6zm0 4v2h2v-2zm0 4v2h2v-2zm10-8v2h2V6zm0 4v2h2v-2zm0 4v2h2v-2zM10 6v12h4V6z" />
export const IconFolder = (p: P) => <S {...p} d="M10 4H4a2 2 0 00-2 2v12a2 2 0 002 2h16a2 2 0 002-2V8a2 2 0 00-2-2h-8z" />
export const IconHome = (p: P) => <S {...p} d="M12 3l9 8h-3v9h-5v-6h-2v6H6v-9H3z" />
export const IconImage = (p: P) => <S {...p} d="M21 19V5a2 2 0 00-2-2H5a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2zM8.5 13.5l2.5 3 3.5-4.5 4.5 6H5z" />
export const IconLeft = (p: P) => <S {...p} d="M15.4 7.4L10.8 12l4.6 4.6L14 18l-6-6 6-6z" />
export const IconLibrary = (p: P) => <S {...p} d="M4 6H2v14a2 2 0 002 2h14v-2H4zm16-4H8a2 2 0 00-2 2v12a2 2 0 002 2h12a2 2 0 002-2V4a2 2 0 00-2-2zm-1 9H9V9h10zm-4 4H9v-2h6zm4-8H9V5h10z" />
export const IconRefresh = (p: P) => <S {...p} d="M17.6 6.4A8 8 0 004.3 10h2.1a6 6 0 019.8-2.2L13 11h7V4zm-11.2 11.2A8 8 0 0019.7 14h-2.1a6 6 0 01-9.8 2.2L11 13H4v7z" />
export const IconRight = (p: P) => <S {...p} d="M8.6 7.4L13.2 12l-4.6 4.6L10 18l6-6-6-6z" />
export const IconSearch = (p: P) => <S {...p} d="M15.5 14h-.8l-.3-.3A6.5 6.5 0 1014 15.5l.3.3v.8l5 5 1.5-1.5zm-6 0a4.5 4.5 0 110-9 4.5 4.5 0 010 9z" />
export const IconSubs = (p: P) => <S {...p} d="M20 4H4a2 2 0 00-2 2v12a2 2 0 002 2h16a2 2 0 002-2V6a2 2 0 00-2-2zM4 12h4v2H4zm10 6H4v-2h10zm6 0h-4v-2h4zm0-4H10v-2h10z" />
export const IconUp = (p: P) => <S {...p} d="M7.4 15.4L12 10.8l4.6 4.6L18 14l-6-6-6 6z" />
