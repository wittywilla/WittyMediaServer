import Foundation

// A game on someone's own PC, which plays in the separately installed Moonlight app.
//
// Why these apps can't simply start it: no Moonlight client on an Apple device has a URL
// scheme, so nothing can open it or even tell whether it's installed. And Moonlight's code
// can never go inside this app, because GPL-3.0 and App Store terms don't mix. So on Apple
// TV, iPhone and iPad the app starts the game on the PC and says plainly what to do next.

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
    public let url: String?
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

    /// Browser play. The link is good once, for two minutes, so ask again just before using it.
    public var bridgeURL: URL? {
        methods?.first { $0.kind == "bridge" }?.url.flatMap(RigPlay.webLink)
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
