# WittyMedia desktop client

Thin Electron shell around the WittyMediaServer web UI. Runs on Windows, macOS,
Linux and FreeBSD (`pkg install electron33 npm`, then `npm start` here).

    npm install
    npm start                # development
    npm run package:win      # or :mac / :linux  -> dist/

The shell remembers the server URL, forwards media keys to the player and opens
external links in the system browser. All features (DVD menus, transcoding,
libraries) live in the server, so the client stays small and update-free.
