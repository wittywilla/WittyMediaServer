// WittyCouch's web half: a player for game streams from any PC running Sunshine. The host app
// provides the API through CouchHost; the server half is the Go module of the same name.
export { default } from './Player'
export type { CouchHost, StreamInfo, StreamRequest } from './Player'
