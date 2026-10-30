import Foundation
import Security
import SwiftUI
import WittyKit

/// The sign-in lives in the Keychain, the server's address in plain settings.
enum Keychain {
    private static let account = "session-token"
    private static let service = "com.wittywilla.wittymediaclient"

    private static var query: [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: account]
    }

    static var token: String {
        get {
            var q = query
            q[kSecReturnData as String] = true
            q[kSecMatchLimit as String] = kSecMatchLimitOne
            var out: AnyObject?
            guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess, let data = out as? Data else { return "" }
            return String(data: data, encoding: .utf8) ?? ""
        }
        set {
            SecItemDelete(query as CFDictionary)
            guard !newValue.isEmpty else { return }
            var q = query
            q[kSecValueData as String] = Data(newValue.utf8)
            q[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            SecItemAdd(q as CFDictionary, nil)
        }
    }
}

/// Themes and wording are shared data: see shared/data/, bundled into the app.
enum SharedData {
    private static func load(_ name: String) -> Data? {
        guard let url = Bundle.main.url(forResource: name, withExtension: "json", subdirectory: "data") else { return nil }
        return try? Data(contentsOf: url)
    }

    static let strings: [String: String] = load("strings.en").flatMap { try? JSONDecoder().decode([String: String].self, from: $0) } ?? [:]

    struct ThemeColours: Decodable {
        let background, panel, card, border, text, textSecondary, accent, onAccent, danger: String
    }
    struct Theme: Decodable {
        let id: String
        let scheme: String
        let colours: ThemeColours
    }
    private struct ThemeFile: Decodable {
        let defaultTheme: String
        let themes: [Theme]
    }
    private static let themeFile = load("themes").flatMap { try? JSONDecoder().decode(ThemeFile.self, from: $0) }

    /// A theme uploaded to the server has no entry here, and gets the default look.
    static func theme(_ id: String?) -> Theme? {
        guard let file = themeFile else { return nil }
        return file.themes.first { $0.id == id } ?? file.themes.first { $0.id == file.defaultTheme }
    }
}

func say(_ key: String) -> String { SharedData.strings[key] ?? "" }

extension Color {
    init(hex: String) {
        let value = UInt32(hex.dropFirst(), radix: 16) ?? 0
        self.init(.sRGB, red: Double((value >> 16) & 0xff) / 255, green: Double((value >> 8) & 0xff) / 255, blue: Double(value & 0xff) / 255)
    }
}

enum Route: Hashable {
    case library(Library)
    case show(Int64)
    case item(Int64)
    case player(Int64, startOver: Bool)
}

enum SignInWay: Hashable {
    case usual, password, shownCode
}

@MainActor
final class AppModel: ObservableObject {
    enum Stage: Equatable {
        case starting
        case connect(message: String)
        case code(SavedServer, fromSignIn: Bool)
        case mismatch(SavedServer)
        case signIn(SignInWay)
        case browsing
    }

    @Published var stage: Stage = .starting
    @Published var path: [Route] = []
    @Published var themeId: String?
    private(set) var client = WittyClient(base: "")

    /// The server the sign-in screens talk to. Its key waits here for a token to be
    /// kept with, unless it was already pinned.
    private(set) var current: SavedServer?

    private let defaults = UserDefaults.standard

    var platformName: String {
        #if os(tvOS)
        return "tvOS"
        #elseif os(macOS)
        return "macOS"
        #else
        return "iOS"
        #endif
    }

    var deviceName: String {
        #if os(tvOS)
        return "Apple TV"
        #elseif os(macOS)
        return "Mac"
        #else
        return UIDevice.current.userInterfaceIdiom == .pad ? "iPad" : "iPhone"
        #endif
    }

    var recent: [SavedServer] {
        get {
            guard let data = defaults.data(forKey: "recent") else { return [] }
            return (try? JSONDecoder().decode([SavedServer].self, from: data)) ?? []
        }
        set { defaults.set(try? JSONEncoder().encode(newValue), forKey: "recent") }
    }

    private var server: String {
        get { defaults.string(forKey: "server") ?? "" }
        set { defaults.set(newValue, forKey: "server") }
    }

    var colours: SharedData.ThemeColours? { SharedData.theme(themeId)?.colours }
    var scheme: ColorScheme { SharedData.theme(themeId)?.scheme == "light" ? .light : .dark }

    /// Only for a server whose identity has just been checked.
    private func openServer(_ opening: SavedServer, token: String = "") {
        current = opening
        server = opening.url
        client = WittyClient(base: opening.url, token: token)
    }

    /// On to signing in, for a server that has been checked.
    func signIn(to opening: SavedServer, way: SignInWay = .usual) {
        Keychain.token = ""
        openServer(opening)
        stage = .signIn(way)
    }

    /// Opens a server with a token that was kept with its key. Answers words to show if it can't.
    private func openWithToken(_ opening: SavedServer) async -> String? {
        recent = SavedServer.remember(opening, in: recent)
        openServer(opening, token: Keychain.token)
        do {
            let status = try await client.status()
            themeId = status.theme
            path = []
            stage = status.user != nil ? .browsing : .signIn(.usual)
            return nil
        } catch let error as APIError {
            return error.message
        } catch is URLError {
            return say("errLost")
        } catch {
            return say("errNotServer")
        }
    }

    private func tokenFits(pin: String, proof: ServerProof) -> Bool {
        !Keychain.token.isEmpty && Identity.fits(pin: pin, proof: proof)
    }

    /// What the last run was using, if it still is who it was. Its token goes
    /// nowhere until then.
    func start() async {
        let url = server
        guard !url.isEmpty else { stage = .connect(message: ""); return }
        var saved = recent.first { $0.url == url } ?? SavedServer(url: url, name: "WittyMediaServer")
        var proof = await WittyClient.identify(url, saved: [saved])
        if proof.state == .unreachable, !saved.serverId.isEmpty,
           let found = await ServerFinder.find(id: saved.serverId, waitSeconds: 6) {
            saved = saved.moved(to: found.url)
            proof = await WittyClient.identify(found.url, saved: [saved])
        }
        let checked = proof.saved(url: saved.url, name: saved.name)
        switch Identity.stepAtStart(proof.state, tokenFits: tokenFits(pin: saved.publicKey, proof: proof)) {
        case .warn:
            stage = .mismatch(checked)
        case .refuse:
            stage = .connect(message: say("identityBad"))
        case .lost:
            stage = .connect(message: say("errLost"))
        case .open:
            if let problem = await openWithToken(checked) { stage = .connect(message: problem) }
        case .askCode, .signIn:
            openServer(checked)
            do {
                themeId = try await client.status().theme
                stage = .signIn(.usual)
            } catch {
                stage = .connect(message: say("errLost"))
            }
        }
    }

    /// Coming back to an app left running counts as starting it. Nothing answering is
    /// left to the screens, as the network is often still waking up.
    func recheck() async {
        guard let using = current, !client.token.isEmpty else { return }
        let proof = await WittyClient.identify(using.url, saved: [using])
        guard proof.state == .different || proof.state == .bad else { return }
        current = nil
        client = WittyClient(base: "")
        path = []
        stage = proof.state == .bad ? .connect(message: say("identityBad")) : .mismatch(proof.saved(url: using.url, name: using.name))
    }

    /// Someone chose this server, and it says it's a WittyMediaServer. Its identity
    /// decides what comes next. Answers words to show if that's nowhere.
    func arrive(url: String, status: ServerStatus) async -> String? {
        let proof = await WittyClient.identify(url, saved: recent)
        let pin = recent.first { $0.url == server }?.publicKey ?? ""
        let checked = proof.saved(url: url, name: status.displayName)
        let step = Identity.nextStep(proof.state, tokenFits: !pin.isEmpty && tokenFits(pin: pin, proof: proof))
        if step != .refuse && step != .lost { themeId = status.theme }
        switch step {
        case .warn:
            stage = .mismatch(checked)
        case .refuse:
            return say("identityBad")
        case .lost:
            return say("errUnreachable")
        case .askCode:
            stage = .code(checked, fromSignIn: false)
        case .signIn:
            signIn(to: checked)
        case .open:
            return await openWithToken(checked)
        }
        return nil
    }

    /// "This is my server, trust it". The key pinned is the one the warning showed, or
    /// it asks again, and the old sign-in isn't kept: docs/CLIENT-API.md says sign in again.
    func trust(_ shown: SavedServer) async -> String? {
        let proof = await WittyClient.identify(shown.url, saved: [])
        switch proof.state {
        case .unreachable:
            return say("errUnreachable")
        case .new where proof.publicKey == shown.publicKey:
            Keychain.token = ""
            recent = SavedServer.remember(shown, in: recent)
            stage = .code(shown, fromSignIn: false)
            return nil
        case .new:
            stage = .mismatch(proof.saved(url: shown.url, name: shown.name))
            return nil
        default:
            return say("identityBad")
        }
    }

    /// Typing a code instead, from the sign-in screens. Only a server that has proved
    /// its identity can be sent one.
    var canTypeCode: Bool { !(current?.publicKey ?? "").isEmpty }

    func askForCode() {
        if let current { stage = .code(current, fromSignIn: true) }
    }

    /// A code typed on this device. The server has to prove itself again first, as it
    /// could have changed since the screen opened. Answers words to show on failure.
    func redeem(_ to: SavedServer, code: String) async -> String? {
        let proof = await WittyClient.identify(to.url, saved: [to])
        if proof.state == .unreachable { return say("errUnreachable") }
        guard Identity.fits(pin: to.publicKey, proof: proof) else { return say("identityBad") }
        do {
            let result = try await WittyClient(base: to.url, quick: true).pairRedeem(code: code, device: deviceName, platform: platformName)
            openServer(to)
            signedIn(token: result.token)
            return nil
        } catch let error as APIError {
            return error.message
        } catch is URLError {
            return say("errUnreachable")
        } catch {
            return say("errInvalid")
        }
    }

    /// The address, the key it proved and the token are kept together.
    func signedIn(token: String) {
        let kept = current ?? SavedServer(url: client.base, name: "WittyMediaServer")
        Keychain.token = token
        server = kept.url
        recent = SavedServer.remember(kept, in: recent)
        client = WittyClient(base: kept.url, token: token)
        path = []
        stage = .browsing
    }

    private func endSession() {
        let old = client
        guard !old.token.isEmpty else { return }
        Task { try? await old.logout() }
    }

    func signOut() {
        endSession()
        Keychain.token = ""
        client = WittyClient(base: client.base)
        path = []
        stage = .signIn(.usual)
    }

    func changeServer() {
        endSession()
        Keychain.token = ""
        current = nil
        server = ""
        client = WittyClient(base: "")
        path = []
        themeId = nil
        stage = .connect(message: "")
    }

    /// A sign-in the server no longer accepts means signing in again.
    func handle(_ error: Error) -> String {
        if let api = error as? APIError, api.isExpiredSignIn {
            Keychain.token = ""
            client = WittyClient(base: client.base)
            path = []
            stage = .signIn(.usual)
            return ""
        }
        return error.localizedDescription
    }
}
