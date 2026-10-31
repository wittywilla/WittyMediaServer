import Foundation
import XCTest
@testable import WittyKit

final class IdentityTests: XCTestCase {
    private struct Fixture: Decodable {
        let nonce: String
        let answer: IdentityAnswer
    }

    /// A real answer from a real server.
    private func fixture() throws -> Fixture {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        let data = try Data(contentsOf: root.appendingPathComponent("shared/fixtures/identity.json"))
        return try JSONDecoder().decode(Fixture.self, from: data)
    }

    private func flipFirstByte(_ text: String) throws -> String {
        var raw = try XCTUnwrap(Data(base64Encoded: text))
        raw[raw.startIndex] ^= 1
        return raw.base64EncodedString()
    }

    private func answer(_ real: IdentityAnswer, serverId: String? = nil, publicKey: String? = nil, algorithm: String? = nil, signature: String? = nil) -> IdentityAnswer {
        IdentityAnswer(
            serverId: serverId ?? real.serverId,
            publicKey: publicKey ?? real.publicKey,
            fingerprint: real.fingerprint,
            algorithm: algorithm ?? real.algorithm,
            signature: signature ?? real.signature
        )
    }

    #if canImport(CryptoKit)
    func testRealSignedAnswerVerifies() throws {
        let f = try fixture()
        XCTAssertTrue(Identity.signatureValid(f.answer, nonce: f.nonce))
    }

    func testTamperedAnswersFail() throws {
        let f = try fixture()
        XCTAssertFalse(Identity.signatureValid(answer(f.answer, signature: try flipFirstByte(f.answer.signature)), nonce: f.nonce), "signature")
        XCTAssertFalse(Identity.signatureValid(answer(f.answer, publicKey: try flipFirstByte(f.answer.publicKey)), nonce: f.nonce), "key")
        XCTAssertFalse(Identity.signatureValid(answer(f.answer, serverId: String(repeating: "0", count: 32)), nonce: f.nonce), "server ID")
        XCTAssertFalse(Identity.signatureValid(f.answer, nonce: try flipFirstByte(f.nonce)), "a replayed answer, to another nonce")
    }

    func testFingerprintIsWorkedOutFromTheKey() throws {
        let f = try fixture()
        XCTAssertEqual(Identity.fingerprint(f.answer.publicKey), f.answer.fingerprint)
        XCTAssertNotEqual(Identity.fingerprint(try flipFirstByte(f.answer.publicKey)), f.answer.fingerprint)
    }

    func testJudge() throws {
        let f = try fixture()
        let url = "http://192.168.1.20:9797"
        let mine = SavedServer(url: url, name: "Home", serverId: f.answer.serverId, publicKey: f.answer.publicKey)
        XCTAssertEqual(Identity.judge(f.answer, nonce: f.nonce, url: url, saved: []), .new)
        XCTAssertEqual(Identity.judge(f.answer, nonce: f.nonce, url: url, saved: [mine]), .same)
        let forged = answer(f.answer, signature: try flipFirstByte(f.answer.signature))
        XCTAssertEqual(Identity.judge(forged, nonce: f.nonce, url: url, saved: [mine]), .bad)
    }
    #endif

    func testMalformedAnswersFailWithoutThrowing() throws {
        let f = try fixture()
        XCTAssertFalse(Identity.signatureValid(answer(f.answer, algorithm: "rsa"), nonce: f.nonce))
        XCTAssertFalse(Identity.signatureValid(answer(f.answer, serverId: ""), nonce: f.nonce))
        XCTAssertFalse(Identity.signatureValid(answer(f.answer, publicKey: "not base64!"), nonce: f.nonce))
        XCTAssertFalse(Identity.signatureValid(answer(f.answer, signature: "not base64!"), nonce: f.nonce))
        XCTAssertFalse(Identity.signatureValid(answer(f.answer, publicKey: Data(count: 31).base64EncodedString()), nonce: f.nonce))
        XCTAssertFalse(Identity.signatureValid(answer(f.answer, signature: Data(count: 63).base64EncodedString()), nonce: f.nonce))
    }

    func testNoncesAreFreshAndAcceptableToTheServer() throws {
        let a = Identity.newNonce()
        XCTAssertNotEqual(a, Identity.newNonce())
        XCTAssertEqual(try XCTUnwrap(Data(base64Encoded: a)).count, 32) // the server accepts 16 to 64 bytes
    }

    private let url = "http://192.168.1.20:9797"
    private let keyA = "a2V5IEE="
    private let keyB = "a2V5IEI="
    private let idA = String(repeating: "a", count: 32)
    private let idB = String(repeating: "b", count: 32)

    private var mine: SavedServer { SavedServer(url: url, name: "Home", serverId: idA, publicKey: keyA) }

    func testComparingKeys() {
        XCTAssertEqual(Identity.compare(publicKey: keyA, serverId: idA, url: url, saved: []), .new)
        XCTAssertEqual(Identity.compare(publicKey: keyA, serverId: idA, url: url, saved: [SavedServer(url: url, name: "Home")]), .new, "a saved address without a key")
        XCTAssertEqual(Identity.compare(publicKey: keyA, serverId: idA, url: url, saved: [mine]), .same)
        XCTAssertEqual(Identity.compare(publicKey: keyA, serverId: idA, url: "http://10.0.0.9:9797", saved: [mine]), .same, "the same key wherever it answers")
        XCTAssertEqual(Identity.compare(publicKey: keyB, serverId: idB, url: url, saved: [mine]), .different, "another key at a remembered address")
        XCTAssertEqual(Identity.compare(publicKey: keyB, serverId: idA, url: "http://10.0.0.9:9797", saved: [mine]), .different, "announcing an ID proves nothing")
        XCTAssertEqual(Identity.compare(publicKey: keyB, serverId: idB, url: "http://10.0.0.9:9797", saved: [mine]), .new)
    }

    func testAnOlderServerIsOnlyAllowedWithNoKeyRemembered() {
        XCTAssertEqual(Identity.unanswered(url: url, saved: []), .older)
        XCTAssertEqual(Identity.unanswered(url: url, saved: [SavedServer(url: url, name: "Home")]), .older)
        XCTAssertEqual(Identity.unanswered(url: url, saved: [mine]), .bad)
    }

    func testTokensOnlyGoWithTheKeyTheyWereKeptWith() {
        let same = ServerProof(state: .same, serverId: idA, publicKey: keyA)
        XCTAssertTrue(Identity.fits(pin: keyA, proof: same))
        XCTAssertFalse(Identity.fits(pin: keyB, proof: same), "another remembered server")
        XCTAssertFalse(Identity.fits(pin: keyA, proof: ServerProof(state: .different, serverId: idA, publicKey: keyB)))
        XCTAssertFalse(Identity.fits(pin: keyA, proof: ServerProof(state: .bad)))
        XCTAssertFalse(Identity.fits(pin: keyA, proof: ServerProof(state: .unreachable)))
        XCTAssertFalse(Identity.fits(pin: keyA, proof: ServerProof(state: .older)), "no answer can't stand in for a key")
        XCTAssertTrue(Identity.fits(pin: "", proof: ServerProof(state: .new, serverId: idA, publicKey: keyA)), "nothing to compare with yet")
        XCTAssertFalse(Identity.fits(pin: "", proof: ServerProof(state: .bad)))
    }

    func testWhatHappensAfterChoosingAServer() {
        XCTAssertEqual(Identity.nextStep(.different, tokenFits: true), .warn)
        XCTAssertEqual(Identity.nextStep(.bad, tokenFits: true), .refuse)
        XCTAssertEqual(Identity.nextStep(.unreachable, tokenFits: true), .lost)
        XCTAssertEqual(Identity.nextStep(.new, tokenFits: false), .askCode)
        XCTAssertEqual(Identity.nextStep(.older, tokenFits: false), .signIn)
        XCTAssertEqual(Identity.nextStep(.same, tokenFits: false), .signIn)
        XCTAssertEqual(Identity.nextStep(.same, tokenFits: true), .open)
    }

    func testWhatHappensAtStart() {
        XCTAssertEqual(Identity.stepAtStart(.different, tokenFits: true), .warn)
        XCTAssertEqual(Identity.stepAtStart(.bad, tokenFits: true), .refuse)
        XCTAssertEqual(Identity.stepAtStart(.unreachable, tokenFits: true), .lost)
        XCTAssertEqual(Identity.stepAtStart(.same, tokenFits: true), .open)
        XCTAssertEqual(Identity.stepAtStart(.same, tokenFits: false), .signIn)
        XCTAssertEqual(Identity.stepAtStart(.new, tokenFits: false), .signIn)
    }

    func testRememberingKeepsOneEntryPerAddressAndPerServer() {
        let moved = mine.moved(to: "http://10.0.0.9:9797")
        XCTAssertEqual(SavedServer.remember(moved, in: [mine]), [moved])
        let atOldAddress = SavedServer(url: url, name: "New", serverId: idB, publicKey: keyB)
        XCTAssertEqual(SavedServer.remember(atOldAddress, in: [mine]), [atOldAddress])
        var list: [SavedServer] = []
        for i in 1...20 { list = SavedServer.remember(SavedServer(url: "http://10.0.0.\(i)", name: "\(i)", serverId: "\(i)"), in: list) }
        XCTAssertEqual(list.count, 8)
    }

    func testServersSavedBeforePinsExistedStillLoad() throws {
        let data = Data(#"[{"url":"http://10.0.0.1:9797","name":"Home"}]"#.utf8)
        XCTAssertEqual(try JSONDecoder().decode([SavedServer].self, from: data), [SavedServer(url: "http://10.0.0.1:9797", name: "Home")])
    }

    func testCapabilitiesNameTheApiVersions() throws {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        let data = try Data(contentsOf: root.appendingPathComponent("shared/fixtures/capabilities.json"))
        XCTAssertEqual(try JSONDecoder().decode(Capabilities.self, from: data).apiVersions, [1])
    }
}
