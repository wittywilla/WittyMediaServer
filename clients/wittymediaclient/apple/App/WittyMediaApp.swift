import SwiftUI
import WittyKit

@main
struct WittyMediaApp: App {
    @StateObject private var model = AppModel()

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

    var body: some View {
        ZStack {
            (model.colours.map { Color(hex: $0.background) } ?? Color.black).ignoresSafeArea()
            switch model.stage {
            case .starting:
                ProgressView()
            case .connect(let message):
                ConnectView(initialMessage: message)
            case .signIn:
                SignInView()
            case .browsing:
                NavigationStack(path: $model.path) {
                    HomeView()
                        .navigationDestination(for: Route.self) { route in
                            switch route {
                            case .library(let library): LibraryView(library: library)
                            case .show(let id): ShowView(id: id)
                            case .item(let id): ItemView(id: id)
                            case .player(let id, let startOver): PlayerScreen(id: id, startOver: startOver)
                            }
                        }
                }
            }
        }
    }
}

extension View {
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
