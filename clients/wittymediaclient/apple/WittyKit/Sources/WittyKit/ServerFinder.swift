#if canImport(Network)
import Foundation
import Network

/// Browses for servers while a screen needs them. Bonjour gives the name and TXT
/// record of each one; a short connection gives its address, which is then checked
/// like any other. Everything happens on one queue, and answers arrive on the main
/// one. The first use makes the system ask for local network access.
public final class ServerFinder: @unchecked Sendable {
    private static let serviceType = "_wittymedia._tcp"
    private static let resolveSeconds = 5.0

    private let queue = DispatchQueue(label: "com.wittywilla.wittymediaclient.finder")
    private let onChange: ([FoundServer]) -> Void
    private var browser: NWBrowser?
    private var list = ServerList()
    private var seen: [String: [String: String]] = [:] // instance to TXT record
    private var resolving: [String: NWConnection] = [:]

    public init(onChange: @escaping ([FoundServer]) -> Void) {
        self.onChange = onChange
    }

    public func start() {
        queue.async { [self] in
            guard browser == nil else { return }
            let found = NWBrowser(for: .bonjourWithTXTRecord(type: Self.serviceType, domain: "local."), using: NWParameters())
            found.browseResultsChangedHandler = { [weak self] results, _ in self?.update(results) }
            found.start(queue: queue)
            browser = found
        }
    }

    public func stop() {
        queue.async { [self] in
            browser?.cancel()
            browser = nil
            for connection in resolving.values { connection.cancel() }
            resolving = [:]
            seen = [:]
        }
    }

    private func publish() {
        let servers = list.list
        let tell = onChange
        DispatchQueue.main.async { tell(servers) }
    }

    private func update(_ results: Set<NWBrowser.Result>) {
        var here = Set<String>()
        for result in results {
            guard case let .service(name, _, _, _) = result.endpoint else { continue }
            here.insert(name)
            guard case let .bonjour(record) = result.metadata else { continue }
            let txt = record.dictionary
            if seen[name] != txt {
                seen[name] = txt
                resolve(result.endpoint, instance: name, txt: txt)
            }
        }
        var changed = false
        for gone in Set(seen.keys).subtracting(here) {
            seen[gone] = nil
            resolving.removeValue(forKey: gone)?.cancel()
            if list.remove(instance: gone) { changed = true }
        }
        if changed { publish() }
    }

    /// The server only announces IPv4 addresses, so this only asks for one.
    private func resolve(_ endpoint: NWEndpoint, instance: String, txt: [String: String]) {
        resolving.removeValue(forKey: instance)?.cancel()
        let parameters = NWParameters.tcp
        if let ip = parameters.defaultProtocolStack.internetProtocol as? NWProtocolIP.Options { ip.version = .v4 }
        let connection = NWConnection(to: endpoint, using: parameters)
        resolving[instance] = connection
        connection.stateUpdateHandler = { [weak self, weak connection] state in
            guard let self, let connection else { return }
            switch state {
            case .ready:
                if case let .hostPort(host, port)? = connection.currentPath?.remoteEndpoint, case let .ipv4(address) = host {
                    let text = address.rawValue.map { String($0) }.joined(separator: ".")
                    if let server = Discovery.fromRecord(instance: instance, txt: txt, host: text, port: Int(port.rawValue)), self.list.add(server) {
                        self.publish()
                    }
                }
                connection.cancel()
            case .failed, .waiting:
                connection.cancel()
            case .cancelled:
                connection.stateUpdateHandler = nil
                if self.resolving[instance] === connection { self.resolving[instance] = nil }
            default:
                break
            }
        }
        connection.start(queue: queue)
        queue.asyncAfter(deadline: .now() + Self.resolveSeconds) { [weak connection] in connection?.cancel() }
    }

    /// Addresses change, so a saved server is looked for by its ID. The caller still
    /// has to check its identity: anyone can announce an ID.
    @MainActor
    public static func find(id: String, waitSeconds: Double) async -> FoundServer? {
        await withCheckedContinuation { (waiter: CheckedContinuation<FoundServer?, Never>) in
            let search = OneSearch(waiter)
            let finder = ServerFinder { servers in
                if let found = servers.first(where: { $0.id == id }) { search.finish(found) }
            }
            search.finder = finder
            finder.start()
            DispatchQueue.main.asyncAfter(deadline: .now() + waitSeconds) { search.finish(nil) }
        }
    }
}

/// Answers once, on the main queue, whichever comes first: the server or the wait running out.
private final class OneSearch: @unchecked Sendable {
    private var waiter: CheckedContinuation<FoundServer?, Never>?
    var finder: ServerFinder?

    init(_ waiter: CheckedContinuation<FoundServer?, Never>) {
        self.waiter = waiter
    }

    func finish(_ found: FoundServer?) {
        guard let waiting = waiter else { return }
        waiter = nil
        finder?.stop()
        finder = nil
        waiting.resume(returning: found)
    }
}
#endif
