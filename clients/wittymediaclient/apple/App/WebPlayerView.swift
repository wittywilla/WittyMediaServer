#if os(iOS)
import SwiftUI
import WebKit
import WittyKit

/// The server's own page in a web view, kept to that server, with no script handlers, so the
/// page can reach nothing of this app or this device. No Moonlight code is involved on this side:
/// see WittyKit's Rigs.swift. WebRTC works in a web view since iOS 14.3. On plain http a browser
/// hides game controllers from a page.
private struct WebPlayerPage: UIViewRepresentable {
    let base: String
    let token: String
    let url: URL
    let onLeave: () -> Void
    let onFail: () -> Void

    final class Coordinator: NSObject, WKNavigationDelegate {
        let parent: WebPlayerPage
        var watching: NSKeyValueObservation?

        init(_ parent: WebPlayerPage) { self.parent = parent }

        func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
            guard let url = action.request.url else { return decisionHandler(.cancel) }
            if WebPlayer.sameServer(url, base: parent.base) { return decisionHandler(.allow) }
            if action.targetFrame?.isMainFrame != false, WebPlayer.isWebLink(url) { UIApplication.shared.open(url) }
            decisionHandler(.cancel)
        }

        func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
            parent.onFail()
        }
    }

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.allowsInlineMediaPlayback = true
        config.mediaTypesRequiringUserActionForPlayback = []
        // Kept in memory only, so the sign-in goes when this screen does.
        config.websiteDataStore = .nonPersistent()
        let web = WKWebView(frame: .zero, configuration: config)
        web.isOpaque = false
        web.backgroundColor = .black
        web.navigationDelegate = context.coordinator
        // This fires when the page changes its own address, which is how it closes its player.
        context.coordinator.watching = web.observe(\.url, options: [.new]) { [onLeave] view, _ in
            guard let path = view.url?.path, !path.isEmpty else { return }
            if !WebPlayer.staysInside(path: path) { DispatchQueue.main.async { onLeave() } }
        }
        // The sign-in reaches the page as the server's own cookie, set only for its address.
        var properties: [HTTPCookiePropertyKey: Any] = [.originURL: url, .path: "/", .name: "wms_token", .value: token, .sameSitePolicy: HTTPCookieStringPolicy.sameSiteLax]
        properties[HTTPCookiePropertyKey("HttpOnly")] = "TRUE"
        if url.scheme?.lowercased() == "https" { properties[.secure] = "TRUE" }
        if let cookie = HTTPCookie(properties: properties) {
            config.websiteDataStore.httpCookieStore.setCookie(cookie) { [weak web, url] in
                web?.load(URLRequest(url: url))
            }
        } else {
            DispatchQueue.main.async { onFail() }
        }
        return web
    }

    func updateUIView(_ web: WKWebView, context: Context) {}

    static func dismantleUIView(_ web: WKWebView, coordinator: Coordinator) {
        coordinator.watching = nil
        web.navigationDelegate = nil
        web.stopLoading() // the web view going away closes the stream's connection
    }
}

struct WebPlayerView: View {
    @EnvironmentObject private var model: AppModel
    let path: String
    @State private var failed = false

    /// Leaving is only for this screen, however many times the page says it left.
    private func leave() {
        if model.path.last == Route.web(path) { model.path.removeLast() }
    }

    var body: some View {
        Group {
            if failed || model.client.token.isEmpty {
                VStack(spacing: 16) {
                    Text("The game player couldn't open. Check the server is on, then try again.").foregroundStyle(.red)
                    Button("Back") { leave() }
                }
            } else if let url = model.client.mediaURL(path) {
                WebPlayerPage(base: model.client.base, token: model.client.token, url: url, onLeave: leave, onFail: { failed = true })
                    .ignoresSafeArea(.container, edges: .bottom)
            }
        }
        .background(Color.black)
        .navigationBarTitleDisplayMode(.inline)
        // A game played on a controller never touches the screen.
        .onAppear { UIApplication.shared.isIdleTimerDisabled = true }
        .onDisappear { UIApplication.shared.isIdleTimerDisabled = false }
    }
}
#endif
