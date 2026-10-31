import Foundation

// What the server sends, keeping only the fields these apps use. Almost
// everything is optional, because a Go nil list arrives as null and older
// servers lack newer fields. Times are in seconds and may be fractional.

public struct ServerStatus: Decodable, Equatable {
    public let name: String?
    public let version: String?
    public let setupRequired: Bool?
    public let brandName: String?
    public let theme: String?
    public let user: User?

    public var displayName: String {
        if let brandName, !brandName.isEmpty { return brandName }
        return name ?? "WittyMediaServer"
    }
}

public struct User: Decodable, Equatable {
    public let id: Int64
    public let name: String
    public let isAdmin: Bool?
    public let kind: String?
}

public struct Profile: Decodable, Identifiable, Equatable {
    public let id: Int64
    public let name: String
    public let kind: String?
    public let hasPassword: Bool?
}

public struct Profiles: Decodable, Equatable {
    public let style: String?
    public let profiles: [Profile]?

    public init(style: String?, profiles: [Profile]?) {
        self.style = style
        self.profiles = profiles
    }
}

public struct LoginResult: Decodable {
    public let token: String
    public let user: User?
}

public struct Library: Decodable, Identifiable, Hashable {
    public let id: Int64
    public let name: String
    public let type: String
    public let itemCount: Int?

    public var isShows: Bool { type == "shows" || type == "anime" }
}

public struct Progress: Decodable, Equatable {
    public let positionSec: Double?
    public let durationSec: Double?
    public let watched: Bool?
    public let resumeSec: Double?
}

public struct Item: Decodable, Identifiable, Equatable {
    public let id: Int64
    public let libraryId: Int64?
    public let kind: String?
    public let title: String
    public let year: Int?
    public let overview: String?
    public let genres: [String]?
    public let contentRating: String?
    public let runtimeSec: Double?
    public let updatedAt: Int64?
    public let progress: Progress?
    public let showId: Int64?
    public let showTitle: String?
    public let season: Int?
    public let episode: Int?
    public let episodeEnd: Int?
    public let nextId: Int64?
    public let introStart: Double?
    public let introEnd: Double?

    /// Only video plays in these apps for now: see docs/DESIGN.md.
    public var isPlayable: Bool { kind == "video" }

    /// The server decides where Resume goes; nil means start over.
    public var resumeSeconds: Double? {
        guard let at = progress?.resumeSec, at > 0 else { return nil }
        return at
    }

    /// "S2 · E15–16", or "" for anything that isn't an episode.
    public var episodeLabel: String {
        guard let season, let episode, episode >= 0 else { return "" }
        var label = "E\(episode)"
        if let episodeEnd, episodeEnd != 0 { label += "–\(episodeEnd)" }
        return season == 0 ? "Special · \(label)" : "S\(season) · \(label)"
    }
}

public struct Show: Decodable, Identifiable, Equatable {
    public let id: Int64
    public let title: String
    public let year: Int?
    public let overview: String?
    public let updatedAt: Int64?
    public let episodeCount: Int?
    public let seasonCount: Int?
    public let unwatched: Int?
}

public struct Season: Decodable, Identifiable, Equatable {
    public let number: Int
    public let title: String?
    public let episodeCount: Int?
    public let unwatched: Int?

    public var id: Int { number }
    public var displayTitle: String {
        if let title, !title.isEmpty { return title }
        return number == 0 ? "Specials" : "Season \(number)"
    }
}

public struct ShowDetail: Decodable, Equatable {
    public let show: Show
    public let seasons: [Season]?
    public let episodes: [Item]?
    public let nextUpId: Int64?
}

public struct Hub: Decodable, Identifiable, Equatable {
    public let key: String
    public let title: String
    public let libraryId: Int64?
    public let items: [Item]?
    public let shows: [Show]?

    public var id: String { key }
    public var isEmpty: Bool { (items ?? []).isEmpty && (shows ?? []).isEmpty }
}

public struct Home: Decodable, Equatable {
    public let hubs: [Hub]?
    public let featured: [Item]?
    public let libraries: [Library]?
}

public struct ItemPage: Decodable {
    public let items: [Item]?
    public let total: Int?
}

public struct ShowPage: Decodable {
    public let shows: [Show]?
    public let total: Int?
}

public struct Track: Decodable, Identifiable, Equatable {
    public let index: Int
    public let language: String?
    public let languageName: String?
    public let title: String?
    public let codec: String?
    public let channels: Int?
    public let delivery: String?
    public let url: String?

    public var id: Int { index }

    public var label: String {
        var parts: [String] = []
        let code = language ?? ""
        let name = languageName ?? ""
        parts.append(!name.isEmpty ? name : code.isEmpty ? "Unknown" : (Locale.current.localizedString(forLanguageCode: code) ?? code))
        if let title, !title.isEmpty { parts.append(title) }
        if let codec, !codec.isEmpty { parts.append(codec.uppercased()) }
        if let channels, channels > 0 { parts.append("\(channels)ch") }
        return parts.joined(separator: " · ")
    }
}

public struct Intro: Decodable, Equatable {
    public let startSec: Double
    public let endSec: Double
}

/// What POST /items/{id}/playback answers: see docs/CLIENT-API.md. The film's
/// position is always the player's time plus timeOffsetSec.
public struct Playback: Decodable, Equatable {
    public let decision: String?
    public let protocolName: String?
    public let url: String
    public let streamId: String?
    public let durationSec: Double?
    public let seek: String?
    public let startAtSec: Double?
    public let timeOffsetSec: Double?
    public let audio: [Track]?
    public let subtitles: [Track]?
    public let audioIndex: Int?
    public let subtitleIndex: Int?
    public let subtitleDelivery: String?
    public let intro: Intro?
    public let nextId: Int64?

    private enum CodingKeys: String, CodingKey {
        case decision, url, streamId, durationSec, seek, startAtSec, timeOffsetSec, audio, subtitles
        case audioIndex, subtitleIndex, subtitleDelivery, intro, nextId
        case protocolName = "protocol"
    }

    public var offset: Double { timeOffsetSec ?? 0 }
    /// A growing stream has to be reopened to go past what exists so far.
    public var mustReopenToSeekFar: Bool { seek == "restart" }
}

/// Left out, a field lets the server decide: which tracks, and where to resume.
public struct PlaybackRequest: Encodable {
    public let profile: String
    public var audio: Int?
    public var subtitle: Int?
    public var startSec: Double?

    public init(profile: String, audio: Int? = nil, subtitle: Int? = nil, startSec: Double? = nil) {
        self.profile = profile
        self.audio = audio
        self.subtitle = subtitle
        self.startSec = startSec
    }
}

public struct Capabilities: Decodable, Equatable {
    public let server: String?
    public let version: String?
    public let apiVersion: Int?
    public let apiVersions: [Int]?
    public let setupRequired: Bool?
    public let features: [String]?
}

public struct PairStart: Decodable, Equatable {
    public let code: String
    public let deviceSecret: String
    public let expiresInSec: Int?
    public let intervalSec: Int?
    public let approvePath: String?
}

public struct PairPoll: Decodable, Equatable {
    public let status: String
    public let token: String?
    public let intervalSec: Int?
}
