import Foundation
import XCTest
@testable import WittyKit

final class ClientLogTests: XCTestCase {
    private var dir: URL!

    override func setUpWithError() throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("wmc-log-\(UUID().uuidString)", isDirectory: true)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: dir)
    }

    private func event(_ message: String) -> ClientEvent {
        ClientEvent(level: "error", kind: "crash", message: message, time: "2026-01-01T00:00:00.000Z")
    }

    // MARK: Redaction

    private struct Cases: Decodable {
        struct Case: Decodable {
            let input: String
            let gone: [String]
            let kept: [String]
        }
        let cases: [Case]
    }

    func testHidesEverySecretInTheSharedCases() throws {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent() // WittyKitTests
            .deletingLastPathComponent() // Tests
            .deletingLastPathComponent() // WittyKit
            .deletingLastPathComponent() // apple
            .deletingLastPathComponent() // wittymediaclient
        let data = try Data(contentsOf: root.appendingPathComponent("shared/data/redaction-cases.json"))
        let cases = try JSONDecoder().decode(Cases.self, from: data).cases
        XCTAssertGreaterThan(cases.count, 10)
        for c in cases {
            let out = Redact.clean(c.input)
            for gone in c.gone { XCTAssertFalse(out.contains(gone), "\(gone) survived in: \(out)") }
            for kept in c.kept { XCTAssertTrue(out.contains(kept), "\(kept) was lost from: \(out)") }
        }
    }

    func testAKnownTokenIsHiddenWithNothingAroundIt() {
        Redact.addSecret("plainwords-that-are-a-token")
        XCTAssertFalse(Redact.clean("the value plainwords-that-are-a-token turned up").contains("plainwords"))
    }

    func testEventsAreCleanedAndCutOnTheWayIn() {
        let e = ClientEvent.cleanedUp(level: "error", kind: "player", message: "failed at /x?st=PLANTED-1 " + String(repeating: "m", count: 5000), stack: "at /y?token=PLANTED-2\n" + String(repeating: "s", count: 50000))
        XCTAssertFalse(e.json().contains("PLANTED"))
        XCTAssertLessThanOrEqual(e.message.count, 1001)
        XCTAssertLessThanOrEqual(e.stack.count, 4001)
    }

    // MARK: Lines

    func testALineIsOneLineWithControlCharactersEscaped() throws {
        let raw = "two\nlines\r\u{1b}[31mred\u{07} \u{202e}evil\u{2028} \u{85}end\"\\"
        let line = jsonString(raw)
        for scalar in line.unicodeScalars {
            XCTAssertFalse(scalar.value < 0x20 || (0x7F...0x9F).contains(scalar.value) || scalar.value == 0x2028 || scalar.value == 0x202E)
        }
        let back = try JSONDecoder().decode([String].self, from: Data("[\(line)]".utf8))
        XCTAssertEqual(back, [raw])
    }

    func testASavedLineReadsBackAsTheEvent() throws {
        let e = ClientEvent(level: "error", kind: "crash", message: "said \"this\"\nand that", stack: "at x", page: "main", session: "v-1", request: "r-1", time: "2026-01-01T00:00:00.000Z")
        XCTAssertEqual(try JSONDecoder().decode(ClientEvent.self, from: Data(e.json().utf8)), e)
        let plain = event("short")
        XCTAssertEqual(try JSONDecoder().decode(ClientEvent.self, from: Data(plain.json().utf8)), plain)
    }

    func testAReportHasTheShapeTheServerReads() throws {
        struct Report: Decodable {
            let app, version, device: String
            let events: [ClientEvent]
        }
        let body = reportBody(app: "app", version: "1.2", device: "TV", events: [event("one"), event("two")])
        let report = try JSONDecoder().decode(Report.self, from: body)
        XCTAssertEqual(report.app, "app")
        XCTAssertEqual(report.device, "TV")
        XCTAssertEqual(report.events.map(\.message), ["one", "two"])
    }

    // MARK: The file

    func testTheFileRotatesAndKeepsOnlySoMany() throws {
        let file = LogFile(directory: dir, maxBytes: 1000, keep: 3, linesPerMinute: 100_000)
        for i in 0..<500 { file.append("line \(i) " + String(repeating: "x", count: 40)) }
        let names = try FileManager.default.contentsOfDirectory(atPath: dir.path).sorted()
        XCTAssertEqual(names, ["client.log", "client.log.1", "client.log.2"])
        for name in names {
            XCTAssertLessThanOrEqual(try Data(contentsOf: dir.appendingPathComponent(name)).count, 1000, name)
        }
        let last = try String(contentsOf: dir.appendingPathComponent("client.log"), encoding: .utf8)
        XCTAssertTrue(last.contains("line 499 "))
    }

    func testAFloodWritesALimitedNumberOfLinesAndSaysSo() throws {
        var clock = Date(timeIntervalSince1970: 1_000_000)
        let file = LogFile(directory: dir, linesPerMinute: 10, now: { clock })
        for i in 0..<1000 { file.append("line \(i)") }
        func lines() throws -> [String] {
            try String(contentsOf: dir.appendingPathComponent("client.log"), encoding: .utf8).split(separator: "\n").map(String.init)
        }
        XCTAssertEqual(try lines().count, 10)
        clock.addTimeInterval(61)
        file.append("after")
        let all = try lines()
        XCTAssertTrue(all[10].contains("990 lines left out"))
        XCTAssertEqual(all[11], "after")
    }

    func testAFolderThatCantBeWrittenToIsNotAProblem() throws {
        try Data("a file where the folder should go".utf8).write(to: dir)
        LogFile(directory: dir).append("nothing happens")
    }

    // MARK: Reports

    func testRepeatsWaitAsOneWithACount() {
        let q = ReportQueue(now: { Date(timeIntervalSince1970: 1_000_000) })
        for _ in 0..<40 { q.add(event("same")) }
        q.add(event("other"))
        let batch = q.take()
        XCTAssertEqual(batch.count, 2)
        XCTAssertEqual(batch[0].count, 40)
    }

    func testABatchKeepsToTheRoutesLimits() {
        let q = ReportQueue(now: { Date(timeIntervalSince1970: 1_000_000) })
        for i in 0..<100 {
            q.add(ClientEvent.cleanedUp(level: "error", kind: "crash", message: "\(i) " + String(repeating: "m", count: 5000), stack: String(repeating: "s", count: 50000)))
        }
        let batch = q.take()
        XCTAssertLessThanOrEqual(batch.count, 25)
        XCTAssertLessThanOrEqual(reportBody(app: "app", version: "1", device: "d", events: batch).count, 64 * 1024)
    }

    func testAFloodStaysBoundedAndCountsWhatItLost() {
        var clock = Date(timeIntervalSince1970: 1_000_000)
        let q = ReportQueue(now: { clock })
        for i in 0..<20000 { q.add(event("flood \(i)")) }
        var sent: [ClientEvent] = []
        for _ in 0..<10 { sent += q.take() }
        XCTAssertLessThanOrEqual(sent.count, 30)
        for _ in 0..<5 {
            clock.addTimeInterval(61)
            for _ in 0..<3 { sent += q.take() }
        }
        XCTAssertLessThanOrEqual(sent.filter { $0.message.hasPrefix("flood") }.count, 100)
        XCTAssertTrue(sent[0].message.hasPrefix("\(20000 - 100) earlier reports"))
    }

    func testAFailedReportIsDroppedAndHeldOff() {
        var clock = Date(timeIntervalSince1970: 1_000_000)
        let q = ReportQueue(now: { clock })
        q.add(event("one"))
        q.failed(q.take())
        q.add(event("two"))
        XCTAssertTrue(q.take().isEmpty)
        XCTAssertGreaterThan(q.waitSeconds() ?? 0, 0)
        clock.addTimeInterval(61)
        XCTAssertEqual(q.take().map(\.message), ["1 earlier reports from this app were left out", "two"])
        XCTAssertNil(q.waitSeconds())
    }
}
