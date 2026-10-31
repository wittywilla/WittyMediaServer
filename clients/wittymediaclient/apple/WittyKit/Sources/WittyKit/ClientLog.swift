import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

// The app's own log: a small file in its caches, and errors reported to the server
// it is connected to, which is the only place they ever go. Nothing in here may
// cause more errors of its own: a report that fails is dropped.

private enum Limit {
    static let fileBytes = 1024 * 1024
    static let keepFiles = 3
    static let linesPerMinute = 120
    static let queuedLines = 200

    // The route's limits are 64 KB and 25 events, and 60 events a minute for an account.
    static let flushSeconds = 5.0
    static let events = 25
    static let bodyBytes = 60 * 1024
    static let sendPerMinute = 30
    static let waiting = 100
    static let holdAfterFailure = 60.0

    static let message = 1000
    static let stack = 4000
    static let field = 200
    static let input = 16 * 1024 // looked at for secrets; anything longer isn't kept anyway
}

private let hidden = "[hidden]"

/// The same rules as the server's log, and the same tests on every client
/// (shared/data/redaction-cases.json).
public enum Redact {
    private static func rule(_ pattern: String, _ with: String, ignoreCase: Bool = true) -> (NSRegularExpression, String)? {
        guard let regex = try? NSRegularExpression(pattern: pattern, options: ignoreCase ? [.caseInsensitive] : []) else { return nil }
        return (regex, with)
    }

    private static let rules: [(NSRegularExpression, String)] = [
        // In an address or a command line: token=..., ?st=..., password=...
        rule(#"(\b(?:token|access_token|refresh_token|id_token|auth|api[_\-]?key|apikey|password|passwd|pwd|pass|secret|pin|key|st|sig|signature|setup[_\-]?code|recovery[_\-]?code|pair(?:ing)?[_\-]?code|otp|totp)=)[^\&\s"'<>]+"#, "$1\(hidden)"),
        // The same with its name percent-encoded: %74oken=... signs in just the same.
        rule(#"([^\s\&?=;"'<>]{0,24}%[0-9a-f]{2}[^\s\&?=;"'<>]{0,24}=)[^\&\s"'<>]+"#, "$1\(hidden)"),
        // A header, as a line of text.
        rule(#"\b(proxy-authorization|authorization|set-cookie|cookie|x-wms-token|x-api-key)(\s*[:=]\s*)[^\r\n"']+"#, "$1$2\(hidden)"),
        rule(#"\b(Bearer\s+)[A-Za-z0-9._~+/=\-]+"#, "$1\(hidden)"),
        rule(#"\b(Basic\s+)[A-Za-z0-9+/]*[A-Z0-9+/][A-Za-z0-9+/]{10,}={0,2}"#, "$1\(hidden)", ignoreCase: false),
        // JSON, finished or cut short: "password": "..."
        rule(#"("(?:[a-z_\-]{0,40}(?:token|secret)|password|passwd|api[_\-]?key|apikey|pin|code|setup[_\-]?code|recovery[_\-]?code|pair(?:ing)?[_\-]?code|cookie|authorization)"\s*:\s*)"[^"\\]*(?:\\.[^"\\]*)*(?:"|$)"#, "$1\"\(hidden)\""),
        // A login in an address: rtsp://user:password@camera/
        rule(#"\b([a-z][a-z0-9+.\-]{0,20}://)[^/\s:@]+:[^/\s@]+@"#, "$1\(hidden)@"),
        // A share link's token.
        rule(#"(/(?:api/)?(?:s|share)/)[A-Za-z0-9_\-]{8,}"#, "$1\(hidden)", ignoreCase: false),
        // The sign-in cookie and stream tokens, however they got there.
        rule(#"\b(wms_token=|st1\.|st_)[A-Za-z0-9._~\-]{6,}"#, "$1\(hidden)"),
        // A long run of hex standing alone is what a session token looks like.
        rule(#"\b[a-fA-F0-9]{48,}\b"#, hidden, ignoreCase: false),
    ].compactMap { $0 }

    private static let lock = NSLock()
    private static var secrets: [String] = []

    /// Makes sure this exact value never appears, wherever it turns up.
    public static func addSecret(_ value: String) {
        guard value.count >= 6 else { return }
        lock.lock()
        defer { lock.unlock() }
        if !secrets.contains(value) { secrets = Array((secrets + [value]).suffix(8)) }
    }

    public static func clean(_ text: String) -> String {
        var s = String(text.prefix(Limit.input))
        lock.lock()
        let known = secrets
        lock.unlock()
        for secret in known { s = s.replacingOccurrences(of: secret, with: hidden) }
        for (regex, with) in rules {
            s = regex.stringByReplacingMatches(in: s, options: [], range: NSRange(s.startIndex..., in: s), withTemplate: with)
        }
        return s
    }
}

private func cleaned(_ text: String, limit: Int) -> String {
    let s = Redact.clean(text)
    return s.count > limit ? String(s.prefix(limit)) + "…" : s
}

/// JSON's string form, plus what it leaves alone but a terminal or an editor acts on.
func jsonString(_ s: String) -> String {
    var out = "\""
    for scalar in s.unicodeScalars {
        switch scalar.value {
        case 0x22: out += "\\\""
        case 0x5C: out += "\\\\"
        case 0x0A: out += "\\n"
        case 0x0D: out += "\\r"
        case 0x09: out += "\\t"
        case 0..<0x20, 0x7F...0x9F, 0x2028, 0x2029, 0x202A...0x202E, 0x2066...0x2069:
            out += String(format: "\\u%04x", scalar.value)
        default:
            out.unicodeScalars.append(scalar)
        }
    }
    return out + "\""
}

private func timeText(_ date: Date) -> String {
    let format = ISO8601DateFormatter()
    format.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return format.string(from: date)
}

/// One thing that happened, in the form the server reads: docs/LOGGING.md.
public struct ClientEvent: Codable, Equatable {
    public var level: String
    public var kind: String
    public var message: String
    public var stack: String = ""
    public var page: String = ""
    public var session: String = ""
    public var request: String = ""
    public var count: Int = 1
    public var time: String = ""

    init(level: String, kind: String, message: String, stack: String = "", page: String = "", session: String = "", request: String = "", count: Int = 1, time: String = "") {
        self.level = level
        self.kind = kind
        self.message = message
        self.stack = stack
        self.page = page
        self.session = session
        self.request = request
        self.count = count
        self.time = time
    }

    /// Everything that goes in passes through here, so no call site has to remember.
    static func cleanedUp(level: String, kind: String, message: String, stack: String = "", page: String = "", session: String = "", request: String = "", at date: Date = Date()) -> ClientEvent {
        ClientEvent(
            level: level, kind: kind, message: cleaned(message, limit: Limit.message),
            stack: stack.isEmpty ? "" : cleaned(stack, limit: Limit.stack),
            page: cleaned(page, limit: Limit.field),
            session: String(session.prefix(Limit.field)), request: String(request.prefix(Limit.field)),
            time: timeText(date))
    }

    /// One line of the log file, and one entry of a report.
    func json() -> String {
        var fields = ["\"time\":\(jsonString(time))", "\"level\":\(jsonString(level))", "\"kind\":\(jsonString(kind))", "\"message\":\(jsonString(message))"]
        if !stack.isEmpty { fields.append("\"stack\":\(jsonString(stack))") }
        if !page.isEmpty { fields.append("\"page\":\(jsonString(page))") }
        if !session.isEmpty { fields.append("\"session\":\(jsonString(session))") }
        if !request.isEmpty { fields.append("\"request\":\(jsonString(request))") }
        fields.append("\"count\":\(count)")
        return "{" + fields.joined(separator: ",") + "}"
    }

    /// Saved lines leave out what's empty.
    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        level = try c.decode(String.self, forKey: .level)
        kind = try c.decode(String.self, forKey: .kind)
        message = try c.decode(String.self, forKey: .message)
        stack = try c.decodeIfPresent(String.self, forKey: .stack) ?? ""
        page = try c.decodeIfPresent(String.self, forKey: .page) ?? ""
        session = try c.decodeIfPresent(String.self, forKey: .session) ?? ""
        request = try c.decodeIfPresent(String.self, forKey: .request) ?? ""
        count = try c.decodeIfPresent(Int.self, forKey: .count) ?? 1
        time = try c.decodeIfPresent(String.self, forKey: .time) ?? ""
    }
}

func reportBody(app: String, version: String, device: String, events: [ClientEvent]) -> Data {
    let list = events.map { $0.json() }.joined(separator: ",")
    return Data("{\"app\":\(jsonString(app)),\"version\":\(jsonString(version)),\"device\":\(jsonString(device)),\"events\":[\(list)]}".utf8)
}

/// The file, moved along by size and with only so many kept. A flood writes a limited
/// number of lines a minute and then says how many it left out. Only ever called on
/// one queue, or from a crash when nothing else is running.
final class LogFile: @unchecked Sendable {
    private let directory: URL
    private let maxBytes: Int
    private let keep: Int
    private let linesPerMinute: Int
    private let now: () -> Date
    private var windowStart = Date.distantPast
    private var linesInWindow = 0
    private var leftOut = 0

    init(directory: URL, maxBytes: Int = Limit.fileBytes, keep: Int = Limit.keepFiles, linesPerMinute: Int = Limit.linesPerMinute, now: @escaping () -> Date = Date.init) {
        self.directory = directory
        self.maxBytes = maxBytes
        self.keep = keep
        self.linesPerMinute = linesPerMinute
        self.now = now
    }

    private var current: URL { directory.appendingPathComponent("client.log") }
    private func numbered(_ i: Int) -> URL { directory.appendingPathComponent("client.log.\(i)") }

    private func size() -> Int {
        ((try? FileManager.default.attributesOfItem(atPath: current.path))?[.size] as? NSNumber)?.intValue ?? 0
    }

    private func rotate() {
        let files = FileManager.default
        try? files.removeItem(at: numbered(keep - 1))
        if keep > 2 {
            for i in stride(from: keep - 2, through: 1, by: -1) { try? files.moveItem(at: numbered(i), to: numbered(i + 1)) }
        }
        // Something holding it open still can't make it grow for ever.
        do { try files.moveItem(at: current, to: numbered(1)) } catch { try? files.removeItem(at: current) }
    }

    private func write(_ line: String) {
        let bytes = Data((line + "\n").utf8)
        let have = size()
        if have > 0 && have + bytes.count > maxBytes { rotate() }
        if let handle = try? FileHandle(forWritingTo: current) {
            defer { try? handle.close() }
            _ = try? handle.seekToEnd()
            try? handle.write(contentsOf: bytes)
        } else {
            try? bytes.write(to: current)
        }
    }

    /// A line that can't be written is a line lost.
    func append(_ line: String) {
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let t = now()
        if t.timeIntervalSince(windowStart) >= 60 {
            if leftOut > 0 {
                write(ClientEvent(level: "warn", kind: "log", message: "\(leftOut) lines left out: too many at once", time: timeText(t)).json())
            }
            leftOut = 0
            windowStart = t
            linesInWindow = 0
        }
        if linesInWindow >= linesPerMinute {
            leftOut += 1
            return
        }
        linesInWindow += 1
        write(line)
    }
}

/// What waits to go to the server. Repeats wait as one event with a count, and a full
/// queue drops the newest: the first errors are usually the ones that explain the rest.
/// Not locked itself: ClientLog holds its lock around every call.
final class ReportQueue {
    private var order: [String] = []
    private var waiting: [String: ClientEvent] = [:]
    private var dropped = 0
    private var earlierInBatch = 0
    private var holdUntil = Date.distantPast
    private var windowStart = Date.distantPast
    private var sentInWindow = 0
    private let now: () -> Date

    init(now: @escaping () -> Date = Date.init) { self.now = now }

    func add(_ e: ClientEvent) {
        let key = [e.level, e.kind, e.message, e.stack, e.page, e.session, e.request].joined(separator: "\u{0}")
        if waiting[key] != nil {
            waiting[key]?.count += 1
        } else if waiting.count >= Limit.waiting {
            dropped += 1
        } else {
            waiting[key] = e
            order.append(key)
        }
    }

    /// The next batch, within the route's limits. Empty means not now.
    func take() -> [ClientEvent] {
        let t = now()
        if t < holdUntil { return [] }
        if t.timeIntervalSince(windowStart) >= 60 {
            windowStart = t
            sentInWindow = 0
        }
        let room = min(Limit.events, Limit.sendPerMinute - sentInWindow)
        if room <= 0 { return [] }
        var out: [ClientEvent] = []
        earlierInBatch = 0
        if dropped > 0 {
            out.append(ClientEvent(level: "warn", kind: "network", message: "\(dropped) earlier reports from this app were left out", time: timeText(t)))
            earlierInBatch = dropped
            dropped = 0
        }
        var bytes = 1024 // the app, version and device around them
        while let key = order.first, out.count < room, let event = waiting[key] {
            let size = event.json().utf8.count
            if bytes + size > Limit.bodyBytes { break }
            bytes += size
            out.append(event)
            waiting[key] = nil
            order.removeFirst()
        }
        sentInWindow += out.count
        return out
    }

    /// Whatever went wrong, what was being sent is gone. Nothing is sent again soon.
    func failed(_ batch: [ClientEvent]) {
        dropped += batch.count + earlierInBatch - (earlierInBatch > 0 ? 1 : 0)
        earlierInBatch = 0
        holdUntil = now().addingTimeInterval(Limit.holdAfterFailure)
    }

    /// How long before another batch can go, or nil for nothing waiting.
    func waitSeconds() -> TimeInterval? {
        if waiting.isEmpty && dropped == 0 { return nil }
        let t = now()
        let forWindow = sentInWindow >= Limit.sendPerMinute ? windowStart.addingTimeInterval(60).timeIntervalSince(t) : 0
        return max(holdUntil.timeIntervalSince(t), forWindow, 0)
    }
}

public final class ClientLog: @unchecked Sendable {
    public static let shared = ClientLog()

    private let lock = NSLock()
    private let writes = DispatchQueue(label: "wittymedia.clientlog", qos: .utility)
    private let queue = ReportQueue()
    private var file: LogFile?
    private var directory: URL?
    private var client: WittyClient?
    private var app = ""
    private var version = ""
    private var device = ""
    private var currentSession = ""
    private var scheduled = false
    private var queuedLines = 0

    private static let crashFile = "crash-pending.json"

    /// The stream in play. Whatever is reported meanwhile is filed under it on the server.
    public var session: String {
        get {
            lock.lock()
            defer { lock.unlock() }
            return currentSession
        }
        set {
            lock.lock()
            defer { lock.unlock() }
            currentSession = newValue
        }
    }

    /// In caches: Apple TV keeps nothing else for an app like this one.
    public static var defaultDirectory: URL {
        let base = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first ?? FileManager.default.temporaryDirectory
        return base.appendingPathComponent("logs", isDirectory: true)
    }

    private static var systemName: String {
        #if os(tvOS)
        return "tvOS"
        #elseif os(iOS)
        return "iOS"
        #elseif os(macOS)
        return "macOS"
        #else
        return "Linux"
        #endif
    }

    /// Call it once, as the app starts.
    public func start(app: String, version: String, directory: URL = ClientLog.defaultDirectory) {
        lock.lock()
        if file != nil {
            lock.unlock()
            return
        }
        self.app = app
        self.version = version
        device = "\(ClientLog.systemName) \(ProcessInfo.processInfo.operatingSystemVersionString)"
        self.directory = directory
        file = LogFile(directory: directory)
        lock.unlock()
        ClientLog.catchCrashes()
        writes.async { self.sendLastCrash() }
        info("app", "Started, version \(version), \(device)")
    }

    /// The connection now in use. Reports go only to this server, with this sign-in.
    public func use(_ client: WittyClient) {
        lock.lock()
        self.client = client
        lock.unlock()
        if !client.token.isEmpty {
            Redact.addSecret(client.token)
            kick()
        }
    }

    public func error(_ kind: String, _ message: String, stack: String = "", request: String = "", session: String? = nil) {
        report("error", kind, message, stack: stack, request: request, session: session)
    }

    public func warn(_ kind: String, _ message: String, stack: String = "", request: String = "", session: String? = nil) {
        report("warn", kind, message, stack: stack, request: request, session: session)
    }

    /// Only for the file on this device.
    public func info(_ kind: String, _ message: String) {
        report("info", kind, message, stack: "", request: "", session: "")
    }

    public func report(_ level: String, _ kind: String, _ message: String, stack: String = "", request: String = "", session: String? = nil) {
        let event = ClientEvent.cleanedUp(level: level, kind: kind, message: message, stack: stack, session: session ?? self.session, request: request)
        writeLine(event)
        guard level != "info" else { return }
        lock.lock()
        queue.add(event)
        lock.unlock()
        schedule(after: Limit.flushSeconds)
    }

    private func writeLine(_ event: ClientEvent) {
        lock.lock()
        guard let to = file, queuedLines < Limit.queuedLines else {
            lock.unlock()
            return
        }
        queuedLines += 1
        lock.unlock()
        let line = event.json()
        writes.async {
            to.append(line)
            self.lock.lock()
            self.queuedLines -= 1
            self.lock.unlock()
        }
    }

    private func kick() {
        lock.lock()
        let waiting = queue.waitSeconds() != nil
        lock.unlock()
        if waiting { schedule(after: Limit.flushSeconds) }
    }

    private func schedule(after seconds: TimeInterval) {
        lock.lock()
        if scheduled {
            lock.unlock()
            return
        }
        scheduled = true
        lock.unlock()
        writes.asyncAfter(deadline: .now() + seconds) {
            self.lock.lock()
            self.scheduled = false
            self.lock.unlock()
            Task.detached(priority: .utility) { await self.flush() }
        }
    }

    /// Who to send to and what, if there's a sign-in. If not, it all stays until use(_:).
    private func nextBatch() -> (to: WittyClient, batch: [ClientEvent], body: Data)? {
        lock.lock()
        defer { lock.unlock() }
        guard let to = client, !to.token.isEmpty else { return nil }
        let batch = queue.take()
        return (to, batch, reportBody(app: app, version: version, device: device, events: batch))
    }

    private func dropped(_ batch: [ClientEvent]) {
        lock.lock()
        queue.failed(batch)
        lock.unlock()
    }

    private func scheduleWhatsLeft() {
        lock.lock()
        let wait = queue.waitSeconds()
        lock.unlock()
        if let wait { schedule(after: max(wait, Limit.flushSeconds)) }
    }

    private func flush() async {
        guard let next = nextBatch() else { return }
        if !next.batch.isEmpty {
            let status = (try? await next.to.sendClientLog(next.body)) ?? 0
            if !(200..<300).contains(status) {
                dropped(next.batch)
                info("log", "Reports to the server were dropped: the answer was \(status)")
            }
        }
        scheduleWhatsLeft()
    }

    // MARK: Crashes

    /// Saved for the next start, as the app can't be relied on to do more now.
    fileprivate func saveCrash(name: String, reason: String, stack: String) {
        lock.lock()
        let (to, dir, session) = (file, directory, currentSession)
        lock.unlock()
        let line = ClientEvent.cleanedUp(level: "error", kind: "crash", message: "The app stopped: \(name): \(reason)", stack: stack, session: session).json()
        to?.append(line)
        if let dir {
            try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
            try? Data(line.utf8).write(to: dir.appendingPathComponent(ClientLog.crashFile))
        }
    }

    private func sendLastCrash() {
        lock.lock()
        let dir = directory
        lock.unlock()
        guard let saved = dir?.appendingPathComponent(ClientLog.crashFile), let data = try? Data(contentsOf: saved) else { return }
        try? FileManager.default.removeItem(at: saved)
        guard let event = try? JSONDecoder().decode(ClientEvent.self, from: data) else { return }
        lock.lock()
        queue.add(event)
        lock.unlock()
        schedule(after: Limit.flushSeconds)
    }

    private static func catchCrashes() {
        #if canImport(ObjectiveC)
        previousExceptionHandler = NSGetUncaughtExceptionHandler()
        NSSetUncaughtExceptionHandler { exception in
            ClientLog.shared.saveCrash(name: exception.name.rawValue, reason: exception.reason ?? "", stack: exception.callStackSymbols.joined(separator: "\n"))
            previousExceptionHandler?(exception)
        }
        #endif
    }
}

#if canImport(ObjectiveC)
/// What was there before, kept so that its handling still happens. A plain global,
/// as the handler can't capture anything.
private var previousExceptionHandler: (@convention(c) (NSException) -> Void)?
#endif
