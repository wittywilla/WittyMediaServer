# WittyMedia desktop client

Thin Electron shell around the WittyMediaServer web UI. Runs on Windows, macOS,
Linux and FreeBSD (`pkg install electron33 npm`, then `npm start` here).

    npm install
    npm start                # development
    npm run package:win      # or :mac / :linux  -> dist/

It works at home or through Remote Access: enter the address from the server's
Settings → Remote Access (**Server → Change server…** switches). The shell warns
before using an unencrypted address outside a private network.

The shell remembers the server URL, forwards media keys to the player and opens
external links in the system browser. All features (DVD menus, transcoding,
libraries) live in the server, so the client stays small and update-free.
