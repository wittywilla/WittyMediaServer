// WittyPlay's web half: the game player, controllers and touch editor. The host
// app provides the API through PlayHost; the server half is the Go module of
// the same name.
export { default } from './Player'
export type { PlayHost, SaveSlot, SessionInfo } from './Player'
export type { ControlsData, SaveControl } from './consoles'
export type { SettingsHost } from './settings'
export { GameSettingsScreen } from './Screen'
