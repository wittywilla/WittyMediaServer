# WittyMedia mobile client (iOS / Android)

A Capacitor shell that ships the WittyMediaServer web UI as a native app.
Native video playback uses the platform's HLS stack (AVPlayer on iOS via
Safari's native HLS; ExoPlayer-backed WebView on Android), so DVD menus,
transcoding and libraries all work exactly as in the browser.

    cd web && npm run build && cd ../clients/mobile
    npm install
    npm run add:ios        # needs Xcode on macOS
    npm run add:android    # needs Android Studio
    npm run sync           # copy web build + sync native projects
    npm run ios            # or: npm run android

Server address: the web client reads `window.WMS_SERVER` when present. Add a
small bootstrap in `www/index.html` (or use the Settings > Server screen) to
persist the URL with `@capacitor/preferences`; the API client sends the bearer
token on every request so cookies are not required.
