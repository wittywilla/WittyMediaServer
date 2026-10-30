import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public struct APIError: LocalizedError {
    public let status: Int
    public let message: String
    public var errorDescription: String? { message }
    public var isExpiredSignIn: Bool { status == 401 }
}

/// Refuses redirects that drop from https to http, and never carries the
/// sign-in to another host.
private final class RedirectGuard: NSObject, URLSessionTaskDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        let from = task.originalRequest?.url
        let to = request.url
        let sameServer = from?.host == to?.host && from?.port == to?.port && from?.scheme == to?.scheme
        completionHandler(sameServer ? request : nil)
    }
}

/// One connection to one server. The token goes in a header, so it stays out
/// of URLs and logs.
public final class WittyClient: @unchecked Sendable {
    public let base: String
    public let token: String
    private let apiPath = "/api/v1" // the contract: docs/CLIENT-API.md
    private let session: URLSession
    private let decoder = JSONDecoder()

    public init(base: String, token: String = "") {
        self.base = base
        self.token = token
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 60 // the first play request probes the file
        config.httpCookieStorage = nil
        config.urlCache = nil
        session = URLSession(configuration: config, delegate: RedirectGuard(), delegateQueue: nil)
    }

    /// The headers an API or artwork request to this server needs. Video doesn't
    /// use these: its addresses carry a short-lived stream token instead.
    public var authHeaders: [String: String] {
        token.isEmpty ? [:] : ["Authorization": "Bearer \(token)"]
    }

    private func call(_ method: String, _ path: String, body: Data? = nil) async throws -> Data {
        guard let url = URL(string: base + apiPath + path) else {
            throw APIError(status: 0, message: "That server address can't be used.")
        }
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.httpBody = body
        if body != nil { request.setValue("application/json", forHTTPHeaderField: "Content-Type") }
        for (name, value) in authHeaders { request.setValue(value, forHTTPHeaderField: name) }
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw APIError(status: 0, message: "The server sent an answer that can't be read.")
        }
        guard (200..<300).contains(http.statusCode) else {
            struct Failure: Decodable { let error: String? }
            let message = (try? decoder.decode(Failure.self, from: data))?.error
            throw APIError(status: http.statusCode, message: message ?? "The server answered with an error (\(http.statusCode)).")
        }
        return data
    }

    private func get<T: Decodable>(_ path: String) async throws -> T {
        try decoder.decode(T.self, from: try await call("GET", path))
    }

    public func capabilities() async throws -> Capabilities { try await get("/capabilities") }
    public func status() async throws -> ServerStatus { try await get("/status") }
    public func profiles() async throws -> Profiles { try await get("/profiles") }
    public func home() async throws -> Home { try await get("/home") }
    public func item(_ id: Int64) async throws -> Item { try await get("/items/\(id)") }
    public func show(_ id: Int64) async throws -> ShowDetail { try await get("/shows/\(id)") }
    public func shows(library: Int64) async throws -> ShowPage { try await get("/shows?library=\(library)&sort=title") }
    public func items(library: Int64, offset: Int, limit: Int) async throws -> ItemPage {
        try await get("/items?library=\(library)&offset=\(offset)&limit=\(limit)&sort=title")
    }
    public func playback(_ id: Int64, _ request: PlaybackRequest) async throws -> Playback {
        let body = try JSONEncoder().encode(request)
        return try decoder.decode(Playback.self, from: try await call("POST", "/items/\(id)/playback", body: body))
    }

    /// Stops the conversion at once when the viewer leaves.
    public func stopStream(_ streamId: String) async throws {
        _ = try await call("DELETE", "/hls/\(streamId)")
    }

    public func pairStart(device: String, platform: String) async throws -> PairStart {
        let body = try JSONEncoder().encode(["device": device, "platform": platform])
        return try decoder.decode(PairStart.self, from: try await call("POST", "/pair/start", body: body))
    }

    /// Denied, expired and slow-down are normal answers here, not errors.
    public func pairPoll(deviceSecret: String) async throws -> PairPoll {
        let body = try JSONEncoder().encode(["deviceSecret": deviceSecret])
        do {
            return try decoder.decode(PairPoll.self, from: try await call("POST", "/pair/poll", body: body))
        } catch let error as APIError where [403, 410, 429].contains(error.status) {
            switch error.status {
            case 403: return PairPoll(status: "denied", token: nil, intervalSec: nil)
            case 410: return PairPoll(status: "expired", token: nil, intervalSec: nil)
            default: return PairPoll(status: "slow-down", token: nil, intervalSec: nil)
            }
        }
    }

    public func login(name: String, password: String, device: String) async throws -> LoginResult {
        let body = try JSONEncoder().encode(["name": name, "password": password, "device": device])
        return try decoder.decode(LoginResult.self, from: try await call("POST", "/login", body: body))
    }

    public func saveProgress(_ id: Int64, positionSec: Double, durationSec: Double) async throws {
        let body = try JSONEncoder().encode(["positionSec": positionSec, "durationSec": durationSec])
        _ = try await call("POST", "/items/\(id)/progress", body: body)
    }

    public func logout() async throws {
        _ = try await call("POST", "/logout", body: Data("{}".utf8))
    }

    public func imageData(_ url: URL) async throws -> Data {
        var request = URLRequest(url: url)
        for (name, value) in authHeaders { request.setValue(value, forHTTPHeaderField: name) }
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError(status: (response as? HTTPURLResponse)?.statusCode ?? 0, message: "No picture.")
        }
        return data
    }

    public func posterURL(_ item: Item) -> URL? { URL(string: "\(base)\(apiPath)/items/\(item.id)/poster?v=\(item.updatedAt ?? 0)") }
    public func posterURL(_ show: Show) -> URL? { URL(string: "\(base)\(apiPath)/shows/\(show.id)/art/poster?v=\(show.updatedAt ?? 0)") }
    /// Addresses in answers start with /api/: the server's address goes in front, nothing else changes.
    public func mediaURL(_ path: String) -> URL? { URL(string: base + path) }

    public enum Probe: Equatable {
        case found(ServerStatus)
        case failed(String)
    }

    /// Asks an address whether it is a WittyMediaServer this app can use.
    public static func probe(_ url: String) async -> Probe {
        let client = WittyClient(base: url)
        do {
            let caps = try await client.capabilities()
            guard caps.server == "WittyMediaServer" else { return .failed("not-server") }
            if caps.setupRequired == true { return .failed("not-set-up") }
            return .found(try await client.status())
        } catch is URLError {
            return .failed("unreachable")
        } catch let error as APIError where error.status == 404 {
            // No /capabilities: an older server, if it says it is one at all.
            let old = (try? await client.status())?.name == "WittyMediaServer"
            return .failed(old ? "server-old" : "not-server")
        } catch {
            return .failed("not-server")
        }
    }
}
