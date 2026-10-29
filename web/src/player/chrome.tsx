import { createContext, useContext } from 'react'

// PlayerChrome connects a player to the dock that hosts it: whether it is
// shown as the small mini player, and how to shrink, restore or stop it.
export type PlayerChrome = {
  mini: boolean
  minimize: () => void // back to the app, playback continues in the corner
  expand: () => void   // full screen player again
  close: () => void    // stop playback
}

export const PlayerChromeContext = createContext<PlayerChrome>({
  mini: false,
  minimize: () => window.history.back(),
  expand: () => {},
  close: () => window.history.back(),
})

export const usePlayerChrome = () => useContext(PlayerChromeContext)
