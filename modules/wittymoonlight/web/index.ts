// WittyMoonlight's web half: a player for game streams from any PC running Sunshine. The host app
// provides the API through MoonlightHost; the server half is the Go module of the same name.
export { default } from './Player'
export type { MoonlightHost, StreamInfo, StreamRequest } from './Player'
