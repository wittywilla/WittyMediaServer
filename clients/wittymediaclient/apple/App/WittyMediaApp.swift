import SwiftUI
import WittyKit

@main
struct WittyMediaApp: App {
    @StateObject private var model = AppModel()

    init() {
        let version = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? ""
        ClientLog.shared.start(app: "WittyMedia Apple", version: version)
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(model)
                .preferredColorScheme(model.scheme)
                .tint(model.colours.map { Color(hex: $0.accent) })
                .task { await model.start() }
        }
    }
}

struct RootView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        ZStack {
            (model.colours.map { Color(hex: $0.background) } ?? Color.black).ignoresSafeArea()
            switch model.stage {
            case .starting:
                ProgressView()
            case .connect(let message):
                ConnectView(initialMessage: message)
            case .code(let server, let fromSignIn):
                CodeView(server: server, fromSignIn: fromSignIn)
            case .mismatch(let server):
                MismatchView(server: server)
            case .signIn(let way):
                SignInView(way: way).id(way)
            case .browsing:
                NavigationStack(path: $model.path) {
                    HomeView()
                        .navigationDestination(for: Route.self) { route in
                            switch route {
                            case .library(let library): LibraryView(library: library)
                            case .show(let id): ShowView(id: id)
                            case .item(let id): ItemView(id: id)
                            case .player(let id, let startOver): PlayerScreen(id: id, startOver: startOver)
                            case .web(let path):
                                #if os(iOS)
                                WebPlayerView(path: path)
                                #else
                                EmptyView()
                                #endif
                            }
                        }
                }
            }
        }
        .onChange(of: scenePhase) { phase in
            if phase == .active { Task { await model.recheck() } }
        }
    }
}

extension View {
    /// The Back button on the Apple TV remote. Without an action it leaves the app as usual.
    @ViewBuilder func onRemoteBack(_ action: (() -> Void)?) -> some View {
        #if os(tvOS)
        self.onExitCommand(perform: action)
        #else
        self
        #endif
    }

    /// Keeps focus able to move into and out of a group, wherever its buttons line up.
    @ViewBuilder func remoteSection() -> some View {
        #if os(tvOS)
        self.focusSection()
        #else
        self
        #endif
    }

    /// Cards on Apple TV lift when focused, which is how focus is shown there.
    @ViewBuilder func tileStyle() -> some View {
        #if os(tvOS)
        self.buttonStyle(.card)
        #else
        self.buttonStyle(.plain)
        #endif
    }

    /// A text box that doesn't try to correct a server address or a name.
    @ViewBuilder func plainEntry() -> some View {
        #if os(macOS)
        self.autocorrectionDisabled()
        #else
        self.textInputAutocapitalization(.never).autocorrectionDisabled()
        #endif
    }
}
