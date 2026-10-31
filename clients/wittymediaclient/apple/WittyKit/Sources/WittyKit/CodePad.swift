import Foundation

/// The number pad's rules, apart from its screen, so they can be tested without one.
/// The focus engine on Apple TV moves between the keys by itself.
public struct CodeEntry: Equatable {
    public static let length = 6
    public private(set) var digits = ""
    public var busy = false

    public init() {}

    public var isComplete: Bool { digits.count == Self.length }

    public mutating func type(_ digit: Character) {
        guard !busy, !isComplete, digit.isASCII, digit.isNumber else { return }
        digits.append(digit)
    }

    public mutating func erase() {
        guard !busy, !digits.isEmpty else { return }
        digits.removeLast()
    }

    /// What's on screen: how many have been typed, not which.
    public var shown: String {
        String(repeating: "•", count: digits.count) + String(repeating: "–", count: Self.length - digits.count)
    }
}
