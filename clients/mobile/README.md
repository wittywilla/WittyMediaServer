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

Server address: the first time it starts, the app asks for the server's address
(at home, or the one from Settings → Remote Access), and the sign-in screen has a
**Change server** button. It is kept on the device. A build can also set it in
advance with `window.WMS_SERVER`. The API client sends the bearer token on every
request so cookies are not required.
