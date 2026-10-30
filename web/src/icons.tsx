// Inline SVG icons (no icon-font dependency).
type P = { size?: number }
const S = ({ d, size = 20, fill = 'currentColor' }: { d: string; size?: number; fill?: string }) => (
  <svg viewBox="0 0 24 24" width={size} height={size} fill={fill} aria-hidden="true"><path d={d} /></svg>
)
export const IconHome = (p: P) => <S {...p} d="M12 3l9 8h-3v9h-5v-6h-2v6H6v-9H3z" />
export const IconFilm = (p: P) => <S {...p} d="M4 4h16v16H4zM6 6v2h2V6zm0 4v2h2v-2zm0 4v2h2v-2zm10-8v2h2V6zm0 4v2h2v-2zm0 4v2h2v-2zM10 6v12h4V6z" />
export const IconDisc = (p: P) => <S {...p} d="M12 2a10 10 0 100 20 10 10 0 000-20zm0 13a3 3 0 110-6 3 3 0 010 6zm0-4.5a1.5 1.5 0 100 3 1.5 1.5 0 000-3z" />
export const IconSettings = (p: P) => <S {...p} d="M19.4 13a7.9 7.9 0 000-2l2.1-1.6-2-3.5-2.5 1a8 8 0 00-1.7-1L15 3H9l-.3 2.9a8 8 0 00-1.7 1l-2.5-1-2 3.5L4.6 11a7.9 7.9 0 000 2l-2.1 1.6 2 3.5 2.5-1a8 8 0 001.7 1L9 21h6l.3-2.9a8 8 0 001.7-1l2.5 1 2-3.5zM12 15.5a3.5 3.5 0 110-7 3.5 3.5 0 010 7z" />
export const IconSearch = (p: P) => <S {...p} d="M15.5 14h-.8l-.3-.3A6.5 6.5 0 1014 15.5l.3.3v.8l5 5 1.5-1.5zm-6 0a4.5 4.5 0 110-9 4.5 4.5 0 010 9z" />
export const IconPlay = (p: P) => <S {...p} d="M8 5v14l11-7z" />
export const IconPause = (p: P) => <S {...p} d="M6 19h4V5H6zm8-14v14h4V5z" />
export const IconBack = (p: P) => <S {...p} d="M20 11H7.8l5.6-5.6L12 4l-8 8 8 8 1.4-1.4L7.8 13H20z" />
export const IconClose = (p: P) => <S {...p} d="M19 6.4L17.6 5 12 10.6 6.4 5 5 6.4l5.6 5.6L5 17.6 6.4 19l5.6-5.6 5.6 5.6 1.4-1.4-5.6-5.6z" />
export const IconFull = (p: P) => <S {...p} d="M7 14H5v5h5v-2H7zm-2-4h2V7h3V5H5zm12 7h-3v2h5v-5h-2zm-3-12v2h3v3h2V5z" />
export const IconVolume = (p: P) => <S {...p} d="M3 9v6h4l5 5V4L7 9zm13.5 3A4.5 4.5 0 0014 8v8a4.5 4.5 0 002.5-4zM14 3.2v2.1a7 7 0 010 13.4v2.1a9 9 0 000-17.6z" />
export const IconMute = (p: P) => <S {...p} d="M16.5 12A4.5 4.5 0 0014 8v2.2l2.5 2.5zM19 12a7 7 0 01-1.3 4l1.5 1.5A9 9 0 0021 12a9 9 0 00-7-8.8v2.1A7 7 0 0119 12zM4.3 3L3 4.3 7.7 9H3v6h4l5 5v-6.7l4.3 4.3a7 7 0 01-2.3 1.3v2.1a9 9 0 003.7-1.9L19.7 21l1.3-1.3zM12 4L9.9 6.1 12 8.2z" />
export const IconNext = (p: P) => <S {...p} d="M6 18l8.5-6L6 6zm10-12v12h2V6z" />
export const IconPrev = (p: P) => <S {...p} d="M6 6h2v12H6zm3.5 6l8.5 6V6z" />
export const IconMenu = (p: P) => <S {...p} d="M3 6h18v2H3zm0 5h18v2H3zm0 5h18v2H3z" />
export const IconSubs = (p: P) => <S {...p} d="M20 4H4a2 2 0 00-2 2v12a2 2 0 002 2h16a2 2 0 002-2V6a2 2 0 00-2-2zM4 12h4v2H4zm10 6H4v-2h10zm6 0h-4v-2h4zm0-4H10v-2h10z" />
export const IconAudio = (p: P) => <S {...p} d="M12 3v10.6A4 4 0 1014 17V7h4V3z" />
export const IconChapters = (p: P) => <S {...p} d="M3 5h2v14H3zm4 0h2v14H7zm4 0h10v14H11z" />
export const IconUp = (p: P) => <S {...p} d="M7.4 15.4L12 10.8l4.6 4.6L18 14l-6-6-6 6z" />
export const IconDown = (p: P) => <S {...p} d="M7.4 8.6L12 13.2l4.6-4.6L18 10l-6 6-6-6z" />
export const IconLeft = (p: P) => <S {...p} d="M15.4 7.4L10.8 12l4.6 4.6L14 18l-6-6 6-6z" />
export const IconRight = (p: P) => <S {...p} d="M8.6 7.4L13.2 12l-4.6 4.6L10 18l6-6-6-6z" />
export const IconRefresh = (p: P) => <S {...p} d="M17.6 6.4A8 8 0 004.3 10h2.1a6 6 0 019.8-2.2L13 11h7V4zm-11.2 11.2A8 8 0 0019.7 14h-2.1a6 6 0 01-9.8 2.2L11 13H4v7z" />
export const IconEdit = (p: P) => <S {...p} d="M3 17.3V21h3.7L17.8 9.9l-3.7-3.7zm17.7-10.2a1 1 0 000-1.4l-2.4-2.4a1 1 0 00-1.4 0l-1.8 1.8 3.7 3.7z" />
export const IconTrash = (p: P) => <S {...p} d="M6 19a2 2 0 002 2h8a2 2 0 002-2V7H6zm3-10h2v8H9zm4 0h2v8h-2zM15.5 4l-1-1h-5l-1 1H5v2h14V4z" />
export const IconCheck = (p: P) => <S {...p} d="M9 16.2l-3.5-3.5L4 14.2 9 19.2 20 8.2l-1.4-1.4z" />
export const IconFolder = (p: P) => <S {...p} d="M10 4H4a2 2 0 00-2 2v12a2 2 0 002 2h16a2 2 0 002-2V8a2 2 0 00-2-2h-8z" />
export const IconDrive = (p: P) => <S {...p} d="M20 13H4a1 1 0 00-1 1v5a1 1 0 001 1h16a1 1 0 001-1v-5a1 1 0 00-1-1zM7 18a1.5 1.5 0 110-3 1.5 1.5 0 010 3zM20 4H4a1 1 0 00-1 1v5a1 1 0 001 1h16a1 1 0 001-1V5a1 1 0 00-1-1zM7 9a1.5 1.5 0 110-3 1.5 1.5 0 010 3z" />
export const IconFile = (p: P) => <S {...p} d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8l-6-6zm4 18H6V4h7v5h5v11z" />
export const IconUsers = (p: P) => <S {...p} d="M16 11a3 3 0 100-6 3 3 0 000 6zm-8 0a3 3 0 100-6 3 3 0 000 6zm0 2c-2.3 0-7 1.2-7 3.5V19h14v-2.5C15 14.2 10.3 13 8 13zm8 0l-.9.1A4.4 4.4 0 0117 16.5V19h6v-2.5c0-2.3-4.7-3.5-7-3.5z" />
export const IconLogout = (p: P) => <S {...p} d="M17 7l-1.4 1.4 2.6 2.6H9v2h9.2l-2.6 2.6L17 17l5-5zM5 5h7V3H5a2 2 0 00-2 2v14a2 2 0 002 2h7v-2H5z" />
export const IconInfo = (p: P) => <S {...p} d="M11 7h2v2h-2zm0 4h2v6h-2zm1-9a10 10 0 100 20 10 10 0 000-20z" />
export const IconTv = (p: P) => <S {...p} d="M21 3H3a2 2 0 00-2 2v12a2 2 0 002 2h5v2h8v-2h5a2 2 0 002-2V5a2 2 0 00-2-2zm0 14H3V5h18z" />
export const IconImage = (p: P) => <S {...p} d="M21 19V5a2 2 0 00-2-2H5a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2zM8.5 13.5l2.5 3 3.5-4.5 4.5 6H5z" />
export const IconEject = (p: P) => <S {...p} d="M5 17h14v2H5zm7-12L5.3 15h13.4z" />
export const IconRew = (p: P) => <S {...p} d="M11 18V6l-8.5 6zm.5-6l8.5 6V6z" />
export const IconFf = (p: P) => <S {...p} d="M4 18l8.5-6L4 6zm9-12v12l8.5-6z" />
export const IconStop = (p: P) => <S {...p} d="M6 6h12v12H6z" />
export const IconBook = (p: P) => <S {...p} d="M18 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V4a2 2 0 00-2-2zM6 4h5v8l-2.5-1.5L6 12z" />
export const IconAnime = (p: P) => <S {...p} d="M12 2l2.4 5.6L20 9l-4.3 3.9L17 19l-5-3-5 3 1.3-6.1L4 9l5.6-1.4z" />
export const IconAngle = (p: P) => <S {...p} d="M17 10.5V7a1 1 0 00-1-1H4a1 1 0 00-1 1v10a1 1 0 001 1h12a1 1 0 001-1v-3.5l4 4v-11z" />
export const IconRemote = (p: P) => <S {...p} d="M15 2H9a3 3 0 00-3 3v14a3 3 0 003 3h6a3 3 0 003-3V5a3 3 0 00-3-3zm-3 3a2.5 2.5 0 110 5 2.5 2.5 0 010-5zM9.5 13h1.5v1.5H9.5zm3.5 0h1.5v1.5H13zm-3.5 3h1.5v1.5H9.5zm3.5 0h1.5v1.5H13z" />
export const IconMini = (p: P) => <S {...p} d="M19 11h-8v6h8zm4 8V5a2 2 0 00-2-2H3a2 2 0 00-2 2v14a2 2 0 002 2h18a2 2 0 002-2zm-2 0H3V5h18z" />
export const IconExpand = (p: P) => <S {...p} d="M4 4h7v2H7.4l4.3 4.3-1.4 1.4L6 7.4V11H4zm16 16h-7v-2h3.6l-4.3-4.3 1.4-1.4 4.3 4.3V13h2z" />
export const IconPalette = (p: P) => <S {...p} d="M12 3a9 9 0 000 18c.8 0 1.5-.7 1.5-1.5 0-.4-.2-.7-.4-1-.2-.3-.4-.6-.4-1 0-.8.7-1.5 1.5-1.5H16a5 5 0 005-5c0-4.4-4-8-9-8zm-5.5 9a1.5 1.5 0 110-3 1.5 1.5 0 010 3zm3-4a1.5 1.5 0 110-3 1.5 1.5 0 010 3zm5 0a1.5 1.5 0 110-3 1.5 1.5 0 010 3zm3 4a1.5 1.5 0 110-3 1.5 1.5 0 010 3z" />
export const IconShield = (p: P) => <S {...p} d="M12 2L4 5v6c0 5 3.4 9.7 8 11 4.6-1.3 8-6 8-11V5zm-1.2 14.2l-3.5-3.5 1.4-1.4 2.1 2.1 4.9-4.9 1.4 1.4z" />
export const IconTune = (p: P) => <S {...p} d="M3 17v2h6v-2zM3 5v2h10V5zm10 16v-2h8v-2h-8v-2h-2v6zM7 9v2H3v2h4v2h2V9zm14 4v-2H11v2zm-6-4h2V7h4V5h-4V3h-2z" />
export const IconTag = (p: P) => <S {...p} d="M21.4 11.6l-9-9A2 2 0 0011 2H4a2 2 0 00-2 2v7c0 .6.2 1.1.6 1.4l9 9c.4.4.9.6 1.4.6s1-.2 1.4-.6l7-7c.4-.4.6-.9.6-1.4s-.2-1-.6-1.4zM5.5 7A1.5 1.5 0 117 5.5 1.5 1.5 0 015.5 7z" />
export const IconPlus = (p: P) => <S {...p} d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6z" />
export const IconPlug = (p: P) => <S {...p} d="M16 7V3h-2v4h-4V3H8v4H6v6l4 4v4h4v-4l4-4V7z" />
export const IconLibrary = (p: P) => <S {...p} d="M4 6H2v14a2 2 0 002 2h14v-2H4zm16-4H8a2 2 0 00-2 2v12a2 2 0 002 2h12a2 2 0 002-2V4a2 2 0 00-2-2zm-1 9H9V9h10zm-4 4H9v-2h6zm4-8H9V5h10z" />
// The icon for a kind of library.
export const IconGamepad = (p: P) => <S {...p} d="M7 7h10a5 5 0 015 5v3.5a3.5 3.5 0 01-6.4 2L14 16h-4l-1.6 1.5A3.5 3.5 0 012 15.5V12a5 5 0 015-5zm0 2.5V11H5.5v1.5H7V14h1.5v-1.5H10V11H8.5V9.5zM16 10a1 1 0 100 2 1 1 0 000-2zm2.5 2.5a1 1 0 100 2 1 1 0 000-2z" />
export const IconShorts = (p: P) => <S {...p} d="M17 2H7a3 3 0 00-3 3v14a3 3 0 003 3h10a3 3 0 003-3V5a3 3 0 00-3-3zm-7 14V8l6 4z" />
export const IconChannels = (p: P) => <S {...p} d="M21 5H3a2 2 0 00-2 2v10a2 2 0 002 2h18a2 2 0 002-2V7a2 2 0 00-2-2zM10 15.5v-7l6 3.5z" />
export const IconRadio = (p: P) => <S {...p} d="M3.2 6.8A2 2 0 002 8.6V20a2 2 0 002 2h16a2 2 0 002-2V8a2 2 0 00-2-2H8.3l8.3-3.4L16 1zM7 20a3 3 0 110-6 3 3 0 010 6zm13-8h-2v-2h-2v2H4V8h16z" />
export const libIcon = (type: string) => (type === 'games' ? <IconGamepad /> : type === 'photos' ? <IconImage /> : type === 'shorts' ? <IconShorts /> : type === 'channels' || type === 'musicvideos' ? <IconChannels /> : type === 'radio' ? <IconRadio /> : type === 'movies' ? <IconFilm /> : type === 'shows' ? <IconTv /> : type === 'anime' ? <IconAnime /> : type === 'music' || type === 'podcasts' ? <IconAudio /> : type === 'audiobooks' || type === 'books' ? <IconBook /> : <IconDisc />)
export const IconGlobe = (p: P) => <S {...p} d="M12 2a10 10 0 100 20 10 10 0 000-20zm7.9 9h-3.4a15.7 15.7 0 00-1.4-6 8 8 0 014.8 6zM12 4c.9 1.2 2 3.7 2.4 7H9.6C10 7.7 11.1 5.2 12 4zM8.9 5a15.7 15.7 0 00-1.4 6H4.1a8 8 0 014.8-6zM4.1 13h3.4a15.7 15.7 0 001.4 6 8 8 0 01-4.8-6zm7.9 7c-.9-1.2-2-3.7-2.4-7h4.8c-.4 3.3-1.5 5.8-2.4 7zm3.1-1a15.7 15.7 0 001.4-6h3.4a8 8 0 01-4.8 6z" />
export const IconLock = (p: P) => <S {...p} d="M18 8h-1V6a5 5 0 00-10 0v2H6a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V10a2 2 0 00-2-2zM9 6a3 3 0 016 0v2H9zm3 11a2 2 0 110-4 2 2 0 010 4z" />
