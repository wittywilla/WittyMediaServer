import Foundation
import XCTest
@testable import WittyKit

/// The number pad and the list of found servers, with the same rules as the other clients.
final class ConnectLogicTests: XCTestCase {
    private func typed(_ text: String) -> CodeEntry {
        var entry = CodeEntry()
        for digit in text { entry.type(digit) }
        return entry
    }

    func testSixDigitsCompleteIt() {
        XCTAssertFalse(typed("12345").isComplete)
        XCTAssertTrue(typed("123456").isComplete)
        XCTAssertEqual(typed("123456").digits, "123456")
    }

    func testASeventhDigitIsIgnored() {
        XCTAssertEqual(typed("1234567").digits, "123456")
    }

    func testOnlyDigitsCount() {
        XCTAssertEqual(typed("1a-2 ٣").digits, "12")
    }

    func testDeleteRemovesTheLast() {
        var entry = typed("123")
        entry.erase()
        XCTAssertEqual(entry.digits, "12")
        var empty = CodeEntry()
        empty.erase()
        XCTAssertEqual(empty.digits, "")
    }

    func testNothingChangesWhileSending() {
        var entry = typed("123456")
        entry.busy = true
        let sending = entry
        entry.type("7")
        entry.erase()
        XCTAssertEqual(entry, sending)
    }

    func testTheScreenShowsHowManyNotWhich() {
        XCTAssertEqual(typed("42").shown, "••––––")
        XCTAssertEqual(CodeEntry().shown, "––––––")
    }

    private let id = "0123456789abcdef0123456789abcdef"

    func testReadsAResolvedInstance() {
        let txt = ["id": id, "name": "Home", "v": "0.4.0", "api": "1", "tls": "0"]
        XCTAssertEqual(Discovery.fromRecord(instance: "Home (0123)", txt: txt, host: "192.168.1.20", port: 9797),
                       FoundServer(id: id, name: "Home", url: "http://192.168.1.20:9797", instance: "Home (0123)"))
        var secure = txt
        secure["tls"] = "1"
        XCTAssertEqual(Discovery.fromRecord(instance: "x", txt: secure, host: "192.168.1.20", port: 9797)?.url, "https://192.168.1.20:9797")
    }

    func testRefusesBadAnnouncements() {
        let txt = ["id": id, "name": "Home", "tls": "0"]
        XCTAssertNil(Discovery.fromRecord(instance: "x", txt: txt, host: "8.8.8.8", port: 9797))
        XCTAssertNil(Discovery.fromRecord(instance: "x", txt: txt, host: nil, port: 9797))
        XCTAssertNil(Discovery.fromRecord(instance: "x", txt: ["id": "not-an-id"], host: "192.168.1.20", port: 9797))
        XCTAssertNil(Discovery.makeServer(id: id, name: "x", host: "192.168.1.2", port: 70000, tls: false))
        XCTAssertNil(Discovery.makeServer(id: id, name: "x", host: "192.168.1.2", port: 0, tls: false))
    }

    func testCleansNames() {
        XCTAssertEqual(Discovery.makeServer(id: id, name: "A\u{07}B\n", host: "10.0.0.2", port: 9797, tls: false)?.name, "AB")
        XCTAssertEqual(Discovery.makeServer(id: id, name: String(repeating: "x", count: 500), host: "10.0.0.2", port: 9797, tls: false)?.name.count, 80)
        XCTAssertEqual(Discovery.makeServer(id: id, name: "", host: "10.0.0.2", port: 9797, tls: false)?.name, "WittyMediaServer")
    }

    func testTheListNoticesChangesAndForgetsWhatIsLost() {
        var list = ServerList()
        let home = FoundServer(id: id, name: "Home", url: "http://10.0.0.2:9797", instance: "Home (0123)")
        XCTAssertTrue(list.add(home))
        XCTAssertFalse(list.add(home), "the same answer again is no change")
        XCTAssertTrue(list.add(FoundServer(id: id, name: "Home", url: "http://10.0.0.3:9797", instance: "Home (0123)")))
        XCTAssertEqual(list.list.count, 1)
        XCTAssertFalse(list.remove(instance: "Someone else"))
        XCTAssertTrue(list.remove(instance: "Home (0123)"))
        XCTAssertTrue(list.list.isEmpty)
    }

    func testTheListIsSortedAndCanNotGrowWithoutLimit() {
        var list = ServerList()
        list.add(FoundServer(id: "2", name: "beta", url: "http://10.0.0.2"))
        list.add(FoundServer(id: "1", name: "Alpha", url: "http://10.0.0.1"))
        XCTAssertEqual(list.list.map(\.name), ["Alpha", "beta"])
        for i in 0..<(Discovery.maxServers + 10) { list.add(FoundServer(id: "n\(i)", name: "n\(i)", url: "http://10.0.0.2")) }
        XCTAssertEqual(list.list.count, Discovery.maxServers)
    }
}
