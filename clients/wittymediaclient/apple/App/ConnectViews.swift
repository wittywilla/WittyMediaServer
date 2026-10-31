import SwiftUI
import WittyKit

private func reasonText(_ reason: String) -> String {
    switch reason {
    case "empty": return say("errEmpty")
    case "scheme": return say("errScheme")
    case "credentials": return say("errCredentials")
    case "public-http": return say("errPublicHttp")
    case "unreachable": return say("errUnreachable")
    case "not-server": return say("errNotServer")
    case "not-set-up": return say("errNotSetUp")
    case "server-old": return say("errServerOld")
    case "app-old": return say("errAppOld")
    case "certificate": return say("errCertificate")
    default: return say("errInvalid")
    }
}

private struct Panel<Content: View>: View {
    @ViewBuilder let content: Content
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) { content }
                .frame(maxWidth: 640, alignment: .leading)
                .padding(32)
                .frame(maxWidth: .infinity)
        }
    }
}

struct ConnectView: View {
    private enum Spot: Hashable {
        case found(String)
        case recent(String)
        case enterAddress
    }

    @EnvironmentObject private var model: AppModel
    let initialMessage: String
    @State private var address = ""
    @State private var error = ""
    @State private var busy = false
    @State private var showForm = false
    @State private var found: [FoundServer] = []
    @State private var finder: ServerFinder?
    @State private var searchedLong = false
    @State private var moved = false
    @State private var offered: Spot?
    @State private var lastSpot: Spot?
    @FocusState private var spot: Spot?

    private func connect(_ url: String) async {
        guard !busy else { return }
        busy = true
        error = ""
        defer { busy = false }
        let problem: String?
        switch await WittyClient.probe(url) {
        case .found(let status):
            problem = await model.arrive(url: url, status: status)
        case .failed(let reason):
            problem = reasonText(reason)
        }
        if let problem {
            error = problem
            address = url
            showForm = true
        }
    }

    private func submit() {
        switch Address.normalize(address) {
        case .ok(let url): Task { await connect(url) }
        case .bad(let reason): error = reasonText(reason)
        }
    }

    /// The first server found takes focus, until the person has moved themselves. The
    /// row only exists after this update, so the focus has to wait for the next one.
    private func show(_ servers: [FoundServer]) {
        found = servers
        guard !moved, let first = servers.first else { return }
        let target = Spot.found(first.id)
        offered = target
        DispatchQueue.main.async { spot = target }
    }

    /// The system picking the first thing to focus isn't the person moving.
    private func focusChanged(_ new: Spot?) {
        if lastSpot != nil, new != nil, new != offered { moved = true }
        if new != nil { lastSpot = new }
    }

    private func row(name: String, url: String) -> some View {
        Button {
            Task { await connect(url) }
        } label: {
            VStack(alignment: .leading) {
                Text(name)
                Text(url).font(.caption).foregroundStyle(.secondary)
            }
        }
        .accessibilityLabel("\(name), \(url)")
    }

    var body: some View {
        let others = model.recent.filter { saved in !found.contains { $0.url == saved.url || $0.id == saved.serverId } }
        Panel {
            if showForm {
                Text(say("addressTitle")).font(.title2).bold()
                Text(say("addressHint")).foregroundStyle(.secondary)
                Text(say("legalConnect")).font(.footnote).foregroundStyle(.secondary)
                TextField(say("addressLabel"), text: $address)
                    .plainEntry()
                    .onSubmit(submit)
                    .accessibilityLabel(say("addressLabel"))
                if !error.isEmpty { Text(error).foregroundStyle(.red) }
                Button(busy ? say("connecting") : say("connect"), action: submit).disabled(busy)
                Button(say("back")) { showForm = false; error = "" }
            } else {
                Text(say("findTitle")).font(.title2).bold()
                Text(say("legalConnect")).font(.footnote).foregroundStyle(.secondary)
                if !error.isEmpty { Text(error).foregroundStyle(.red) }
                if busy { Text(say("connecting")).foregroundStyle(.secondary) }
                Text(say("foundServers")).foregroundStyle(.secondary)
                if found.isEmpty {
                    Text(searchedLong ? say("findNone") : say("findSearching")).foregroundStyle(.secondary)
                }
                ForEach(found) { server in
                    row(name: server.name, url: server.url).focused($spot, equals: .found(server.id))
                }
                if !others.isEmpty {
                    Text(say("recentServers")).foregroundStyle(.secondary)
                    ForEach(others) { server in
                        row(name: server.name, url: server.url).focused($spot, equals: .recent(server.url))
                    }
                }
                Button(say("enterAddress")) { showForm = true; error = "" }
                    .focused($spot, equals: .enterAddress)
            }
        }
        .onChange(of: spot) { focusChanged($0) }
        .onRemoteBack(showForm ? { showForm = false; error = "" } : nil)
        .onAppear {
            if error.isEmpty { error = initialMessage }
            // Only looks while this screen is open.
            let looking = ServerFinder { show($0) }
            looking.start()
            finder = looking
        }
        .onDisappear {
            finder?.stop()
            finder = nil
        }
        .task {
            try? await Task.sleep(nanoseconds: 6_000_000_000)
            searchedLong = true
        }
    }
}

/// A new server is opened with a six-digit code from someone already signed in. See
/// docs/CLIENT-API.md, "The code is typed on the device". Works with only a remote:
/// the zero is twice as wide, so the focus engine finds something below every digit.
struct CodeView: View {
    @EnvironmentObject private var model: AppModel
    let server: SavedServer
    let fromSignIn: Bool
    @State private var entry = CodeEntry()
    @State private var problem = ""

    private func press(_ digit: Character) {
        let before = entry
        entry.type(digit)
        guard entry != before else { return }
        problem = ""
        guard entry.isComplete else { return }
        entry.busy = true
        let code = entry.digits
        Task {
            if let failed = await model.redeem(server, code: code) {
                entry = CodeEntry()
                problem = failed
            }
        }
    }

    private func back() {
        model.stage = fromSignIn ? .signIn(.usual) : .connect(message: "")
    }

    private func key(_ digit: Character) -> some View {
        Button {
            press(digit)
        } label: {
            Text(String(digit)).font(.title2.weight(.semibold)).frame(maxWidth: .infinity)
        }
        .buttonStyle(.bordered)
    }

    var body: some View {
        Panel {
            Text(say("codeTitle")).font(.title2).bold()
            Text(say("codeHint")).foregroundStyle(.secondary)
            Text(entry.shown)
                .font(.system(size: 44, weight: .bold, design: .monospaced))
                .accessibilityLabel("\(say("codeLabel")): \(entry.digits.count) \(say("codeEntered"))")
            if entry.busy {
                Text(say("connecting")).foregroundStyle(.secondary)
            } else if !problem.isEmpty {
                Text(problem).foregroundStyle(.red)
            }
            Grid(horizontalSpacing: 12, verticalSpacing: 12) {
                GridRow { key("1"); key("2"); key("3") }
                GridRow { key("4"); key("5"); key("6") }
                GridRow { key("7"); key("8"); key("9") }
                GridRow {
                    Button {
                        entry.erase()
                    } label: {
                        Text(say("codeDelete")).frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.bordered)
                    key("0").gridCellColumns(2)
                }
            }
            .remoteSection()
            VStack(alignment: .leading, spacing: 16) {
                Button(say("usePassword")) { model.signIn(to: server, way: .password) }
                Button(say("showCodeInstead")) { model.signIn(to: server, way: .shownCode) }
                Button(say("back"), action: back)
            }
            .remoteSection()
            Text("\(say("fingerprintLabel")): \(server.fingerprint)").font(.caption).foregroundStyle(.secondary)
        }
        .onRemoteBack(back)
    }
}

/// Another machine is answering for a server we know. Only the person can decide, so
/// the safe choice comes first and has focus, and nothing has been sent to it.
struct MismatchView: View {
    @EnvironmentObject private var model: AppModel
    let server: SavedServer
    @State private var problem = ""
    @State private var busy = false
    @FocusState private var onSafeChoice: Bool

    private func back() {
        model.stage = .connect(message: "")
    }

    var body: some View {
        Panel {
            Text(say("identityTitle")).font(.title2).bold()
            Text(say("identityHint")).foregroundStyle(.secondary)
            Text("\(server.url) · \(say("fingerprintLabel")): \(server.fingerprint)").font(.caption).foregroundStyle(.secondary)
            if !problem.isEmpty { Text(problem).foregroundStyle(.red) }
            Button(say("back"), action: back).focused($onSafeChoice)
            Button(say("identityTrust")) {
                busy = true
                Task {
                    problem = await model.trust(server) ?? ""
                    busy = false
                }
            }
            .disabled(busy)
        }
        .onRemoteBack(back)
        .task { onSafeChoice = true }
    }
}

/// The TV shows a code; someone already signed in on a phone or computer approves
/// it, so nobody has to type a password with a remote. See docs/CLIENT-API.md.
private struct PairPanel: View {
    @EnvironmentObject private var model: AppModel
    let onUsePassword: () -> Void
    @State private var round = 0
    @State private var code = ""
    @State private var approveAt = ""
    @State private var problem = ""

    private func run() async {
        code = ""
        problem = ""
        do {
            let start = try await model.client.pairStart(device: model.deviceName, platform: model.platformName)
            code = start.code
            approveAt = model.client.base + (start.approvePath ?? "/pair")
            var wait = max(start.intervalSec ?? 3, 1)
            while !Task.isCancelled {
                try await Task.sleep(nanoseconds: UInt64(wait) * 1_000_000_000)
                let poll = try await model.client.pairPoll(deviceSecret: start.deviceSecret)
                switch poll.status {
                case "approved":
                    if let token = poll.token { model.signedIn(token: token) }
                    return
                case "denied":
                    problem = say("pairDenied"); code = ""; return
                case "expired":
                    problem = say("pairExpired"); code = ""; return
                case "slow-down":
                    wait = max(poll.intervalSec ?? wait, wait) + 1
                default:
                    break
                }
            }
        } catch is CancellationError {
            return
        } catch {
            problem = error.localizedDescription
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text(say("pairTitle")).font(.title2).bold()
            if !code.isEmpty {
                Text(say("pairHint")).foregroundStyle(.secondary)
                Text(approveAt)
                Text(code).font(.system(size: 56, weight: .bold, design: .monospaced))
                    .accessibilityLabel(code.map(String.init).joined(separator: " "))
                Text(say("pairWaiting")).foregroundStyle(.secondary)
            }
            if !problem.isEmpty { Text(problem).foregroundStyle(.red) }
            if code.isEmpty && !problem.isEmpty { Button(say("pairNew")) { round += 1 } }
            if model.canTypeCode { Button(say("codeTitle")) { model.askForCode() } }
            Button(say("usePassword"), action: onUsePassword)
            Button(say("changeServer")) { model.changeServer() }
        }
        .task(id: round) { await run() }
    }
}

struct SignInView: View {
    @EnvironmentObject private var model: AppModel
    @State private var useCode: Bool
    @State private var profiles: Profiles?
    @State private var chosen: Profile?
    @State private var useForm = false
    @State private var name = ""
    @State private var password = ""
    @State private var error = ""
    @State private var busy = false

    init(way: SignInWay) {
        switch way {
        case .shownCode:
            _useCode = State(initialValue: true)
        case .password:
            _useCode = State(initialValue: false)
        case .usual:
            #if os(tvOS)
            _useCode = State(initialValue: true) // on a TV the code comes first
            #else
            _useCode = State(initialValue: false) // everywhere else a keyboard is at hand
            #endif
        }
    }

    private func login(_ who: String, _ secret: String) {
        guard !busy else { return }
        busy = true
        error = ""
        Task {
            defer { busy = false }
            do {
                let result = try await model.client.login(name: who, password: secret, device: model.deviceName)
                model.signedIn(token: result.token)
            } catch {
                self.error = error.localizedDescription
            }
        }
    }

    var body: some View {
        Panel {
            if useCode {
                PairPanel { useCode = false }
            } else if let chosen {
                Text(chosen.name).font(.title2).bold()
                SecureField("Password", text: $password).onSubmit { login(chosen.name, password) }
                if !error.isEmpty { Text(error).foregroundStyle(.red) }
                Button("Sign in") { login(chosen.name, password) }.disabled(busy)
                Button(say("back")) { self.chosen = nil; password = ""; error = "" }
            } else if let list = profiles?.profiles, profiles?.style == "profiles", !useForm, !list.isEmpty {
                Text("Who's watching?").font(.title2).bold()
                if !error.isEmpty { Text(error).foregroundStyle(.red) }
                ForEach(list) { profile in
                    Button(profile.name) {
                        if profile.hasPassword == false { login(profile.name, "") } else { chosen = profile }
                    }
                }
                Button("Sign in with another account") { useForm = true }
                if model.canTypeCode { Button(say("codeTitle")) { model.askForCode() } }
                Button(say("useCode")) { useCode = true }
                Button(say("changeServer")) { model.changeServer() }
            } else if profiles != nil {
                Text("Sign in").font(.title2).bold()
                TextField("Name", text: $name).plainEntry()
                SecureField("Password", text: $password).onSubmit { login(name, password) }
                if !error.isEmpty { Text(error).foregroundStyle(.red) }
                Button("Sign in") { login(name, password) }.disabled(busy)
                if !(profiles?.profiles ?? []).isEmpty { Button(say("back")) { useForm = false; error = "" } }
                if model.canTypeCode { Button(say("codeTitle")) { model.askForCode() } }
                Button(say("useCode")) { useCode = true }
                Button(say("changeServer")) { model.changeServer() }
            } else {
                ProgressView()
            }
        }
        .task {
            profiles = (try? await model.client.profiles()) ?? Profiles(style: "classic", profiles: nil)
        }
    }
}
