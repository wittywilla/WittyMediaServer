import Foundation

// A game on someone's own PC. On iPhone and iPad it plays in the server's own page, in a web
// view. The separately installed Moonlight app is the fallback.
//
// Why these apps can't simply start Moonlight: no Moonlight client on an Apple device has a URL
// scheme, so nothing can open it or even tell whether it's installed. And Moonlight's code can
// never go inside this app, because GPL-3.0 and App Store terms are widely held to conflict.
//
// Apple TV has no web view at all, as tvOS doesn't offer WebKit to apps, so there the app can
// only start the game on the PC and say plainly what to do next. A separate Apple TV app under
// the GPL, published on its own terms, is the only way to do better: that's the owner's decision.

public struct RigInfo: Decodable, Equatable {
    public let id: Int64
    public let name: String
    public let canWake: Bool?
}

/// One way to play, best first. Which fields are filled depends on its kind.
public struct RigMethod: Decodable, Equatable {
    public let kind: String
    public let label: String?
    public let note: String?
    public let args: [String]?
    public let steps: [String]?
}

public struct RigInstall: Decodable, Equatable {
    public let name: String
    public let url: String?
    public let note: String?
    public let license: String?

    /// Where to get it, if that's a web page.
    public var webURL: URL? { url.flatMap(RigPlay.webLink) }
}

/// What POST /rigs/games/{id}/play answers.
public struct RigPlay: Decodable, Equatable {
    public let rig: RigInfo
    public let ready: Bool?
    public let message: String?
    public let methods: [RigMethod]?
    public let install: RigInstall?
    public let setup: [String]?

    /// Only web links leave for a browser: anything else could open other apps.
    static func webLink(_ text: String) -> URL? {
        guard let url = URL(string: text), let scheme = url.scheme?.lowercased(), scheme == "https" || scheme == "http" else { return nil }
        return url
    }

    public var isReady: Bool { ready == true }

    /// Whether the server's own page can play this here, which it offers first when it can.
    public var playsInPage: Bool { methods?.first?.kind == "client" }

    public var inPageNote: String? {
        methods?.first { $0.kind == "client" }?.note
    }

    /// What the desktop Moonlight would be run with, if the server offered that and it's safe.
    public var moonlightArgs: [String]? {
        guard let args = methods?.first(where: { $0.kind == "command" })?.args, MoonlightCommand.isSafe(args) else { return nil }
        return args
    }

    public var steps: [String] {
        methods?.first { $0.kind == "steps" }?.steps ?? []
    }
}

public struct RigStarted: Decodable, Equatable {
    public let ok: Bool?
    public let steps: [String]?
}

public struct RigWake: Decodable, Equatable {
    public let ok: Bool?
    public let note: String?
}

/// The server's own pages for playing, kept to that server in a web view.
public enum WebPlayer {
    public static let gamePCsPath = "/couch"
    public static func rigPath(_ gameId: Int64) -> String { "/couch-rig/\(gameId)" }

    /// Going anywhere else in the web interface means going back to this app's own screens.
    public static func staysInside(path: String) -> Bool {
        path == gamePCsPath || path.hasPrefix(gamePCsPath + "/") || path.hasPrefix("/couch-rig/")
    }

    private static func effectivePort(_ url: URL) -> Int? {
        if let port = url.port { return port }
        switch url.scheme?.lowercased() {
        case "https": return 443
        case "http": return 80
        default: return nil
        }
    }

    public static func sameServer(_ url: URL, base: String) -> Bool {
        guard let server = URL(string: base), let scheme = url.scheme?.lowercased(), scheme == "https" || scheme == "http",
              let host = url.host else { return false }
        return scheme == server.scheme?.lowercased() && host.lowercased() == server.host?.lowercased() && effectivePort(url) == effectivePort(server)
    }

    /// Only web links leave for a browser: anything else could open other apps.
    public static func isWebLink(_ url: URL) -> Bool { RigPlay.webLink(url.absoluteString) != nil }
}

/// Starting the desktop Moonlight program on a Mac. A server could send anything, so what it
/// sends is checked as strictly as the desktop app does, and it never names a program.
public enum MoonlightCommand {
    public static let macProgram = "/Applications/Moonlight.app/Contents/MacOS/Moonlight"

    private static let maxApp = 200
    private static let maxName = 253
    private static let alnum = Set("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
    private static let hexish = Set("abcdefABCDEF0123456789:.")

    private static func validPort(_ port: Substring) -> Bool {
        guard (1...5).contains(port.count), port.allSatisfy({ $0.isASCII && $0.isNumber }), let n = Int(port) else { return false }
        return (1...65535).contains(n)
    }

    private static func validName(_ name: Substring) -> Bool {
        guard !name.isEmpty, name.count <= maxName else { return false }
        return name.split(separator: ".", omittingEmptySubsequences: false).allSatisfy { label in
            guard let first = label.first, let last = label.last, alnum.contains(first), alnum.contains(last) else { return false }
            return label.allSatisfy { alnum.contains($0) || $0 == "-" }
        }
    }

    /// A plain name, an IPv4 address or [an IPv6 address], with a port if it needs one.
    public static func validHost(_ host: String) -> Bool {
        if host.hasPrefix("[") {
            guard let close = host.firstIndex(of: "]") else { return false }
            let inner = host[host.index(after: host.startIndex)..<close]
            guard inner.contains(":"), inner.allSatisfy({ hexish.contains($0) }) else { return false }
            let rest = host[host.index(after: close)...]
            return rest.isEmpty || (rest.first == ":" && validPort(rest.dropFirst()))
        }
        let parts = host.split(separator: ":", omittingEmptySubsequences: false)
        switch parts.count {
        case 1: return validName(parts[0])
        case 2: return validName(parts[0]) && validPort(parts[1])
        default: return false
        }
    }

    /// Game names are free text set on the PC. Nothing here may read as an option.
    public static func validApp(_ app: String) -> Bool {
        let trimmed = app.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty, app.utf16.count <= maxApp, !trimmed.hasPrefix("-") else { return false }
        return !app.unicodeScalars.contains { $0.properties.generalCategory == .control }
    }

    public static func isSafe(_ args: [String]) -> Bool {
        args.count == 3 && args[0] == "stream" && validHost(args[1]) && validApp(args[2])
    }
}
