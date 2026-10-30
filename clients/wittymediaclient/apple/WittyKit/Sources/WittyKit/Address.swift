import Foundation

/// Turns what someone typed into a server address, or says why it can't be one.
/// Every client follows the same rules: see shared/data/connection-cases.json.
public enum Address {
    public static let defaultPort = 9797

    public enum Result: Equatable {
        case ok(String)
        case bad(String)
    }

    private static func matches(_ text: String, _ pattern: String, caseInsensitive: Bool = false) -> Bool {
        var options: String.CompareOptions = [.regularExpression]
        if caseInsensitive { options.insert(.caseInsensitive) }
        return text.range(of: pattern, options: options) != nil
    }

    public static func normalize(_ input: String) -> Result {
        var text = input.trimmingCharacters(in: .whitespacesAndNewlines)
        if text.isEmpty { return .bad("empty") }
        // Newer Foundation quietly encodes spaces instead of refusing them.
        if text.contains(where: \.isWhitespace) { return .bad("invalid") }
        let hasScheme = matches(text, "^[a-zA-Z][a-zA-Z0-9+.-]*://")
        var addPort = false
        if hasScheme {
            if !matches(text, "^https?://", caseInsensitive: true) { return .bad("scheme") }
        } else if matches(text, "^(javascript|file|data|mailto):", caseInsensitive: true) {
            return .bad("scheme")
        } else {
            // A bare IPv6 address needs its brackets before it can be parsed.
            if text.filter({ $0 == ":" }).count >= 2 && !text.hasPrefix("[") { text = "[\(text)]" }
            addPort = !matches(text, "(\\]|^[^\\[\\]:]+):[0-9]*$")
            text = "http://" + text
        }
        guard let parts = URLComponents(string: text), let rawHost = parts.host, !rawHost.isEmpty else {
            return .bad("invalid")
        }
        if parts.user != nil || parts.password != nil { return .bad("credentials") }
        // Foundation may or may not keep an IPv6 address's brackets.
        let bare = rawHost.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        let host = bare.contains(":") ? "[\(bare)]" : bare
        let secure = parts.scheme?.lowercased() == "https"
        let port = addPort ? defaultPort : parts.port
        if let port, port > 65535 || port < 0 { return .bad("invalid") }
        if !secure && !isPrivateHost(host) { return .bad("public-http") }
        let standard = secure ? 443 : 80
        let suffix = (port == nil || port == standard) ? "" : ":\(port!)"
        return .ok("\(secure ? "https" : "http")://\(host)\(suffix)")
    }

    /// Is this address on the local network? A name with no dots or ending in
    /// .local can only be resolved locally; anything else may be public.
    public static func isPrivateHost(_ host: String) -> Bool {
        let pieces = host.split(separator: ".", omittingEmptySubsequences: false)
        if pieces.count == 4, pieces.allSatisfy({ !$0.isEmpty && $0.count <= 3 && $0.allSatisfy(\.isNumber) }) {
            let a = Int(pieces[0]) ?? 0
            let b = Int(pieces[1]) ?? 0
            return a == 127 || a == 10 || (a == 172 && (16...31).contains(b)) || (a == 192 && b == 168)
                || (a == 169 && b == 254) || (a == 100 && (64...127).contains(b))
        }
        if host.hasPrefix("[") {
            let h = host.dropFirst().dropLast()
            return h == "::1" || ["fc", "fd", "fe8", "fe9", "fea", "feb"].contains { h.hasPrefix($0) }
        }
        return host == "localhost" || !host.contains(".") || host.hasSuffix(".local")
    }
}
