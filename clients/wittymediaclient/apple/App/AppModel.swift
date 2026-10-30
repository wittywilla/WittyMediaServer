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

struct SavedServer: Codable, Identifiable, Equatable {
    let url: String
    let name: String
    var id: String { url }
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

@MainActor
final class AppModel: ObservableObject {
    enum Stage: Equatable {
        case starting
        case connect(message: String)
        case signIn
        case browsing
    }

    @Published var stage: Stage = .starting
    @Published var path: [Route] = []
    @Published var themeId: String?
    private(set) var client = WittyClient(base: "")

    private let defaults = UserDefaults.standard

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

    func start() async {
        guard !server.isEmpty else { stage = .connect(message: ""); return }
        client = WittyClient(base: server, token: Keychain.token)
        do {
            let status = try await client.status()
            themeId = status.theme
            stage = status.user != nil ? .browsing : .signIn
        } catch {
            stage = .connect(message: say("errLost"))
        }
    }

    func use(server url: String, status: ServerStatus) async {
        server = url
        recent = [SavedServer(url: url, name: status.displayName)] + recent.filter { $0.url != url }.prefix(7)
        Keychain.token = ""
        client = WittyClient(base: url)
        themeId = status.theme
        stage = .signIn
    }

    func signedIn(token: String) {
        Keychain.token = token
        client = WittyClient(base: client.base, token: token)
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
        stage = .signIn
    }

    func changeServer() {
        endSession()
        Keychain.token = ""
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
            stage = .signIn
            return ""
        }
        return error.localizedDescription
    }
}
