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
    @EnvironmentObject private var model: AppModel
    let initialMessage: String
    @State private var address = ""
    @State private var error = ""
    @State private var busy = false
    @State private var showForm = false

    private func connect(_ url: String) async {
        guard !busy else { return }
        busy = true
        defer { busy = false }
        switch await WittyClient.probe(url) {
        case .found(let status):
            await model.use(server: url, status: status)
        case .failed(let reason):
            error = reasonText(reason)
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

    var body: some View {
        Panel {
            if showForm || model.recent.isEmpty {
                Text(say("addressTitle")).font(.title2).bold()
                Text(say("addressHint")).foregroundStyle(.secondary)
                TextField(say("addressLabel"), text: $address)
                    .plainEntry()
                    .onSubmit(submit)
                    .accessibilityLabel(say("addressLabel"))
                if !error.isEmpty { Text(error).foregroundStyle(.red) }
                Button(busy ? say("connecting") : say("connect"), action: submit).disabled(busy)
                if !model.recent.isEmpty {
                    Button(say("back")) { showForm = false; error = "" }
                }
            } else {
                Text(say("findTitle")).font(.title2).bold()
                if !error.isEmpty { Text(error).foregroundStyle(.red) }
                Text(say("recentServers")).foregroundStyle(.secondary)
                ForEach(model.recent) { server in
                    Button {
                        Task { await connect(server.url) }
                    } label: {
                        VStack(alignment: .leading) {
                            Text(server.name)
                            Text(server.url).font(.caption).foregroundStyle(.secondary)
                        }
                    }
                    .accessibilityLabel("\(server.name), \(server.url)")
                }
                Button(say("enterAddress")) { showForm = true }
            }
        }
        .onAppear { if error.isEmpty { error = initialMessage } }
    }
}

/// The TV shows a code; someone already signed in on a phone or computer approves
/// it, so nobody has to type a password with a remote. See docs/CLIENT-API.md.
private struct PairPanel: View {
    @EnvironmentObject private var model: AppModel
    let device: String
    let platform: String
    let onUsePassword: () -> Void
    @State private var round = 0
    @State private var code = ""
    @State private var approveAt = ""
    @State private var problem = ""

    private func run() async {
        code = ""
        problem = ""
        do {
            let start = try await model.client.pairStart(device: device, platform: platform)
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
            Button(say("usePassword"), action: onUsePassword)
            Button(say("changeServer")) { model.changeServer() }
        }
        .task(id: round) { await run() }
    }
}

struct SignInView: View {
    @EnvironmentObject private var model: AppModel
    #if os(tvOS)
    @State private var useCode = true // on a TV the code comes first
    #else
    @State private var useCode = false // everywhere else a keyboard is at hand
    #endif
    @State private var profiles: Profiles?
    @State private var chosen: Profile?
    @State private var useForm = false
    @State private var name = ""
    @State private var password = ""
    @State private var error = ""
    @State private var busy = false

    private var platform: String {
        #if os(tvOS)
        return "tvOS"
        #elseif os(macOS)
        return "macOS"
        #else
        return "iOS"
        #endif
    }

    private var deviceName: String {
        #if os(tvOS)
        return "Apple TV"
        #elseif os(macOS)
        return "Mac"
        #else
        return UIDevice.current.userInterfaceIdiom == .pad ? "iPad" : "iPhone"
        #endif
    }

    private func login(_ who: String, _ secret: String) {
        guard !busy else { return }
        busy = true
        error = ""
        Task {
            defer { busy = false }
            do {
                let result = try await model.client.login(name: who, password: secret, device: deviceName)
                model.signedIn(token: result.token)
            } catch {
                self.error = error.localizedDescription
            }
        }
    }

    var body: some View {
        Panel {
            if useCode {
                PairPanel(device: deviceName, platform: platform) { useCode = false }
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
                Button(say("useCode")) { useCode = true }
                Button(say("changeServer")) { model.changeServer() }
            } else if profiles != nil {
                Text("Sign in").font(.title2).bold()
                TextField("Name", text: $name).plainEntry()
                SecureField("Password", text: $password).onSubmit { login(name, password) }
                if !error.isEmpty { Text(error).foregroundStyle(.red) }
                Button("Sign in") { login(name, password) }.disabled(busy)
                if !(profiles?.profiles ?? []).isEmpty { Button(say("back")) { useForm = false; error = "" } }
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
