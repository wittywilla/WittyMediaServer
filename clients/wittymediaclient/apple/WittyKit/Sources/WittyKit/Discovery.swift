import Foundation

// Reads servers announced on the local network: docs/CLIENT-API.md, "Finding a server".
// All of this comes from the network and is treated as untrusted: being announced
// opens nothing, and the app checks the server's identity afterwards.

public struct FoundServer: Identifiable, Equatable {
    public let id: String
    public let name: String
    public let url: String
    public let instance: String

    public init(id: String, name: String, url: String, instance: String = "") {
        self.id = id
        self.name = name
        self.url = url
        self.instance = instance
    }
}

public enum Discovery {
    public static let maxServers = 32
    private static let maxName = 80

    public static func cleanName(_ name: String) -> String {
        let shown = String(name.unicodeScalars.filter { $0.value >= 0x20 && $0.value != 0x7f })
        let clean = String(shown.trimmingCharacters(in: .whitespacesAndNewlines).prefix(maxName))
        return clean.isEmpty ? "WittyMediaServer" : clean
    }

    /// A server as the connect screen may show it, or nil if any part is unusable.
    public static func makeServer(id: String, name: String, host: String, port: Int, tls: Bool, instance: String = "") -> FoundServer? {
        guard id.range(of: "^[0-9a-f]{32}$", options: .regularExpression) != nil, (1...65535).contains(port) else { return nil }
        guard host.range(of: "^[0-9]{1,3}(\\.[0-9]{1,3}){3}$", options: .regularExpression) != nil, Address.isPrivateHost(host) else { return nil }
        guard case .ok(let url) = Address.normalize("\(tls ? "https" : "http")://\(host):\(port)") else { return nil }
        return FoundServer(id: id, name: cleanName(name), url: url, instance: instance)
    }

    /// One resolved mDNS instance, with its TXT record.
    public static func fromRecord(instance: String, txt: [String: String], host: String?, port: Int) -> FoundServer? {
        makeServer(id: txt["id"] ?? "", name: txt["name"] ?? "", host: host ?? "", port: port, tls: txt["tls"] == "1", instance: instance)
    }
}

/// Keeps the list. Servers drop out when the network says they have gone, and the
/// oldest goes when too many turn up, so a noisy network can't grow it without limit.
public struct ServerList {
    private var servers: [FoundServer] = []

    public init() {}

    @discardableResult
    public mutating func add(_ server: FoundServer) -> Bool {
        if let at = servers.firstIndex(where: { $0.id == server.id }) {
            if servers[at] == server { return false }
            servers[at] = server
            return true
        }
        if servers.count >= Discovery.maxServers { servers.removeFirst() }
        servers.append(server)
        return true
    }

    @discardableResult
    public mutating func remove(instance: String) -> Bool {
        let before = servers.count
        servers.removeAll { $0.instance == instance }
        return servers.count != before
    }

    public var list: [FoundServer] {
        servers.sorted { $0.name.lowercased() < $1.name.lowercased() }
    }
}
