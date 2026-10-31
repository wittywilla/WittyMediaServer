import Foundation
import XCTest
@testable import WittyKit

/// Sample responses, kept true to real servers by shared/scripts/check-fixtures.mjs.
final class ModelTests: XCTestCase {
    private func fixture<T: Decodable>(_ name: String) throws -> T {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        let data = try Data(contentsOf: root.appendingPathComponent("shared/fixtures/\(name).json"))
        return try JSONDecoder().decode(T.self, from: data)
    }

    func testRigPlay() throws {
        let json = """
        {"rig":{"id":3,"libraryId":9,"name":"Den PC","paired":true,"state":"online","message":"Ready to play.","canWake":false},
        "game":{"id":7,"title":"Space Game"},"away":false,"ready":true,
        "methods":[{"kind":"client","label":"Play here","oneClick":true,"note":"Keyboard only."},
        {"kind":"command","label":"Open in Moonlight","oneClick":true,"args":["stream","192.168.1.20","Space Game"]},
        {"kind":"steps","label":"Open it yourself","oneClick":false,"steps":["Open the Moonlight app."]}],
        "install":{"name":"Moonlight for Apple TV","url":"https://apps.apple.com/x","license":"GPL-3.0","source":"https://example.com"},"setup":null}
        """
        let play = try JSONDecoder().decode(RigPlay.self, from: Data(json.utf8))
        XCTAssertTrue(play.isReady)
        XCTAssertEqual(play.rig.name, "Den PC")
        XCTAssertTrue(play.playsInPage)
        XCTAssertEqual(play.inPageNote, "Keyboard only.")
        XCTAssertEqual(play.moonlightArgs, ["stream", "192.168.1.20", "Space Game"])
        XCTAssertEqual(play.steps, ["Open the Moonlight app."])
        XCTAssertNotNil(play.install?.webURL)
    }

    func testRigAnswersAreCheckedBeforeUse() throws {
        let json = #"{"rig":{"id":1,"name":"PC"},"install":{"name":"M","url":"file:///etc/passwd"},"methods":[{"kind":"command","args":["stream","pc","--quit-after"]},{"kind":"client"}]}"#
        let play = try JSONDecoder().decode(RigPlay.self, from: Data(json.utf8))
        XCTAssertNil(play.install?.webURL)
        XCTAssertNil(play.moonlightArgs)
        XCTAssertFalse(play.isReady)
        XCTAssertFalse(play.playsInPage, "only when it's what the server puts first")
    }

    func testWebPlayerKeepsToItsOwnPages() throws {
        XCTAssertEqual(WebPlayer.rigPath(7), "/moonlight-rig/7")
        for path in ["/moonlight", "/moonlight/3", "/moonlight/3/play/9", "/moonlight-rig/7"] { XCTAssertTrue(WebPlayer.staysInside(path: path), path) }
        for path in ["/", "/item/7", "/moonlights", "/settings", "/login"] { XCTAssertFalse(WebPlayer.staysInside(path: path), path) }
    }

    func testOnlyTheSameServerStaysInTheWebView() throws {
        let base = "http://192.168.1.5:9797"
        func same(_ text: String) throws -> Bool { WebPlayer.sameServer(try XCTUnwrap(URL(string: text)), base: base) }
        XCTAssertTrue(try same("http://192.168.1.5:9797/moonlight-rig/7"))
        XCTAssertFalse(try same("http://192.168.1.5:9798/"))
        XCTAssertFalse(try same("https://192.168.1.5:9797/"))
        XCTAssertFalse(try same("http://192.168.1.5.example.com:9797/"))
        XCTAssertFalse(try same("https://github.com/moonlight-stream"))
        XCTAssertFalse(try same("file:///etc/passwd"))
        XCTAssertTrue(WebPlayer.sameServer(try XCTUnwrap(URL(string: "https://home.example/x")), base: "https://home.example:443"))
        XCTAssertTrue(WebPlayer.isWebLink(try XCTUnwrap(URL(string: "https://github.com/x"))))
        XCTAssertFalse(WebPlayer.isWebLink(try XCTUnwrap(URL(string: "tel:123"))))
    }

    func testMoonlightArguments() {
        for good in [["stream", "192.168.1.20", "Space Game"], ["stream", "pc.local:48010", "A & B; C"], ["stream", "[fd00::20]:48010", "Game"]] {
            XCTAssertTrue(MoonlightCommand.isSafe(good), good.joined(separator: " "))
        }
        let bad = [
            [], ["stream"], ["stream", "pc"], ["stream", "pc", "Game", "--extra"], ["list", "pc", "Game"], ["pair", "pc", "Game"],
            ["stream", "-pc", "Game"], ["stream", "pc name", "Game"], ["stream", "pc:", "Game"], ["stream", "pc:70000", "Game"], ["stream", "[nope]", "Game"],
            ["stream", "pc", ""], ["stream", "pc", "--quit-after"], ["stream", "pc", " -x"], ["stream", "pc", "a\nb"], ["stream", "pc", String(repeating: "x", count: 201)],
        ]
        for args in bad { XCTAssertFalse(MoonlightCommand.isSafe(args), args.joined(separator: " ")) }
    }

    func testStatus() throws {
        let status: ServerStatus = try fixture("status")
        XCTAssertEqual(status.name, "WittyMediaServer")
        XCTAssertNil(status.user)
        XCTAssertEqual(status.displayName, "WittyMediaServer")
    }

    func testProfiles() throws {
        let profiles: Profiles = try fixture("profiles")
        XCTAssertEqual(profiles.style, "profiles")
        XCTAssertEqual(profiles.profiles?.last?.hasPassword, false)
    }

    func testHomeAcceptsNullLists() throws {
        let home: Home = try fixture("home")
        XCTAssertEqual(home.libraries?.last?.isShows, true)
        let continuing = try XCTUnwrap(home.hubs?.first?.items?.first)
        XCTAssertEqual(continuing.progress?.positionSec, 912.25)
        XCTAssertNil(home.hubs?.last?.items)
        XCTAssertEqual(home.hubs?.last?.isEmpty, false)
    }

    func testShow() throws {
        let detail: ShowDetail = try fixture("show")
        XCTAssertEqual(detail.nextUpId, 22)
        let episodes = try XCTUnwrap(detail.episodes)
        XCTAssertEqual(episodes[0].episodeLabel, "S1 · E1")
        XCTAssertEqual(episodes[1].episodeLabel, "S1 · E2–3")
        XCTAssertNil(episodes[0].resumeSeconds, "with no resume point from the server, it starts again")
    }

    func testPlayback() throws {
        let play: Playback = try fixture("playback")
        XCTAssertEqual(play.protocolName, "hls")
        XCTAssertTrue(play.mustReopenToSeekFar)
        XCTAssertEqual(play.offset, 600)
        XCTAssertEqual(play.audio?.count, 2)
        XCTAssertEqual(play.subtitles?.first?.delivery, "hls")
        XCTAssertEqual(play.intro?.endSec, 95)
        XCTAssertEqual(play.nextId, 43)
    }

    func testPlaybackRequestLeavesOutWhatTheServerDecides() throws {
        let data = try JSONEncoder().encode(PlaybackRequest(profile: "appletv"))
        XCTAssertEqual(String(data: data, encoding: .utf8), #"{"profile":"appletv"}"#)
    }

    func testCapabilities() throws {
        let caps: Capabilities = try fixture("capabilities")
        XCTAssertEqual(caps.server, "WittyMediaServer")
        XCTAssertEqual(caps.features?.contains("pairing"), true)
        XCTAssertEqual(caps.apiVersions, [1])
    }

    func testPairStart() throws {
        let start: PairStart = try fixture("pair-start")
        XCTAssertEqual(start.code, "BCDF-GHJK")
        XCTAssertEqual(start.intervalSec, 3)
    }
}
