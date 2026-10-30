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
