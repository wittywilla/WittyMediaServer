import Foundation
#if canImport(CryptoKit)
import CryptoKit
#endif

// Checks that an address is answered by the server we think it is: it has to
// sign a fresh random value with its Ed25519 key. See docs/CLIENT-API.md, "Server identity".

public struct IdentityAnswer: Decodable, Equatable {
    public let serverId: String
    public let publicKey: String
    public let fingerprint: String?
    public let algorithm: String
    public let signature: String
}

/// A server this device has used, with the identity key it proved.
public struct SavedServer: Codable, Identifiable, Equatable {
    public let url: String
    public let name: String
    public let serverId: String
    public let publicKey: String
    public let fingerprint: String
    public var id: String { url }

    private enum CodingKeys: String, CodingKey {
        case url, name, serverId, publicKey, fingerprint
    }

    public init(url: String, name: String, serverId: String = "", publicKey: String = "", fingerprint: String = "") {
        self.url = url
        self.name = name
        self.serverId = serverId
        self.publicKey = publicKey
        self.fingerprint = fingerprint
    }

    /// Servers saved before keys were pinned have none of the last three.
    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        url = try values.decode(String.self, forKey: .url)
        name = try values.decode(String.self, forKey: .name)
        serverId = try values.decodeIfPresent(String.self, forKey: .serverId) ?? ""
        publicKey = try values.decodeIfPresent(String.self, forKey: .publicKey) ?? ""
        fingerprint = try values.decodeIfPresent(String.self, forKey: .fingerprint) ?? ""
    }

    public func moved(to newURL: String) -> SavedServer {
        SavedServer(url: newURL, name: name, serverId: serverId, publicKey: publicKey, fingerprint: fingerprint)
    }

    /// The most recent first, once per address and once per server.
    public static func remember(_ server: SavedServer, in recent: [SavedServer]) -> [SavedServer] {
        let others = recent.filter { $0.url != server.url && (server.serverId.isEmpty || $0.serverId != server.serverId) }
        return Array(([server] + others).prefix(8))
    }
}

public enum IdentityState: Equatable {
    /// Nothing answered.
    case unreachable
    /// A server older than this check, and no key was remembered for it.
    case older
    /// The answer proves nothing, so nothing about it can be believed.
    case bad
    /// Never seen before.
    case new
    /// A key we remembered.
    case same
    /// We remembered another key here: possibly a look-alike.
    case different
}

public struct ServerProof: Equatable {
    public let state: IdentityState
    public let serverId: String
    public let publicKey: String
    public let fingerprint: String

    public init(state: IdentityState, serverId: String = "", publicKey: String = "", fingerprint: String = "") {
        self.state = state
        self.serverId = serverId
        self.publicKey = publicKey
        self.fingerprint = fingerprint
    }

    public func saved(url: String, name: String) -> SavedServer {
        SavedServer(url: url, name: name, serverId: serverId, publicKey: publicKey, fingerprint: fingerprint)
    }
}

public enum NextStep: Equatable {
    case warn, refuse, lost, askCode, signIn, open
}

public enum Identity {
    public static func newNonce() -> String {
        // The system's generator, which Swift documents as cryptographically secure.
        var generator = SystemRandomNumberGenerator()
        let bytes = (0..<32).map { _ in UInt8.random(in: .min ... .max, using: &generator) }
        return Data(bytes).base64EncodedString()
    }

    static func signedBytes(serverId: String, nonce: String) -> Data {
        Data("WittyMediaServer identity v1\n\(serverId)\n\(nonce)".utf8)
    }

    /// Is this answer really signed by the key it names? Linux has no CryptoKit,
    /// so nothing verifies there: the library builds, and refuses every answer.
    public static func signatureValid(_ answer: IdentityAnswer, nonce: String) -> Bool {
        guard answer.algorithm == "ed25519", !answer.serverId.isEmpty,
              let key = Data(base64Encoded: answer.publicKey), key.count == 32,
              let signature = Data(base64Encoded: answer.signature), signature.count == 64 else { return false }
        #if canImport(CryptoKit)
        guard let publicKey = try? Curve25519.Signing.PublicKey(rawRepresentation: key) else { return false }
        return publicKey.isValidSignature(signature, for: signedBytes(serverId: answer.serverId, nonce: nonce))
        #else
        return false
        #endif
    }

    /// A short form of the key for people to compare by eye. Worked out here, since
    /// a look-alike could claim another server's fingerprint in its answer.
    public static func fingerprint(_ publicKey: String) -> String {
        #if canImport(CryptoKit)
        guard let key = Data(base64Encoded: publicKey) else { return "" }
        let hex = SHA256.hash(data: key).prefix(6).map { String(format: "%02X", $0) }.joined()
        return "\(hex.prefix(4))-\(hex.dropFirst(4).prefix(4))-\(hex.dropFirst(8))"
        #else
        return ""
        #endif
    }

    /// What a key means, given the saved servers. Holding a remembered key proves who
    /// it is, wherever it answers. Otherwise a remembered address or ID answering with
    /// another key is what a look-alike would look like.
    public static func compare(publicKey: String, serverId: String, url: String, saved: [SavedServer]) -> IdentityState {
        let pinned = saved.filter { !$0.publicKey.isEmpty }
        if pinned.contains(where: { $0.publicKey == publicKey }) { return .same }
        if pinned.contains(where: { $0.url == url || $0.serverId == serverId }) { return .different }
        return .new
    }

    public static func judge(_ answer: IdentityAnswer, nonce: String, url: String, saved: [SavedServer]) -> IdentityState {
        guard signatureValid(answer, nonce: nonce) else { return .bad }
        return compare(publicKey: answer.publicKey, serverId: answer.serverId, url: url, saved: saved)
    }

    /// A 404 is an older server, unless we remember this address proving itself:
    /// then it's what a look-alike would say to avoid the question.
    public static func unanswered(url: String, saved: [SavedServer]) -> IdentityState {
        saved.contains { $0.url == url && !$0.publicKey.isEmpty } ? .bad : .older
    }

    /// May what belongs with this pin (a token, a code) go to the server that gave this proof?
    public static func fits(pin: String, proof: ServerProof) -> Bool {
        if pin.isEmpty { return proof.state == .new || proof.state == .older }
        return proof.state == .same && proof.publicKey == pin
    }

    /// After someone chooses a server.
    public static func nextStep(_ state: IdentityState, tokenFits: Bool) -> NextStep {
        switch state {
        case .different: return .warn
        case .bad: return .refuse
        case .unreachable: return .lost
        case .new: return .askCode
        case .older: return .signIn
        case .same: return tokenFits ? .open : .signIn
        }
    }

    /// When the app starts, with the server it was last using.
    public static func stepAtStart(_ state: IdentityState, tokenFits: Bool) -> NextStep {
        switch state {
        case .different: return .warn
        case .bad: return .refuse
        case .unreachable: return .lost
        case .new, .older, .same: return tokenFits ? .open : .signIn
        }
    }
}
