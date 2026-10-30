import Foundation
import XCTest
@testable import WittyKit

/// The same cases every client must pass.
final class AddressTests: XCTestCase {
    private struct Cases: Decodable {
        struct Case: Decodable {
            let input: String
            let url: String?
            let error: String?
        }
        let cases: [Case]
    }

    func testSharedCases() throws {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent() // WittyKitTests
            .deletingLastPathComponent() // Tests
            .deletingLastPathComponent() // WittyKit
            .deletingLastPathComponent() // apple
            .deletingLastPathComponent() // wittymediaclient
        let data = try Data(contentsOf: root.appendingPathComponent("shared/data/connection-cases.json"))
        for c in try JSONDecoder().decode(Cases.self, from: data).cases {
            let expected: Address.Result = c.url.map { .ok($0) } ?? .bad(c.error ?? "")
            XCTAssertEqual(Address.normalize(c.input), expected, "address \"\(c.input)\"")
        }
    }
}
