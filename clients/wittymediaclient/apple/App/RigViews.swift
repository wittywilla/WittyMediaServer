import SwiftUI
import WittyKit

// A game on someone's own PC. Why iPhone and iPad use the server's page, why Apple TV can't, why
// nothing here can open Moonlight, and why Moonlight is never inside this app (GPL-3.0 and the App
// Store) are all at the top of WittyKit's Rigs.swift.

#if os(tvOS)
private let rigOS = "tvos"
#elseif os(macOS)
private let rigOS = "mac"
#else
private let rigOS = "ios"
#endif

// The server only offers a command to run for the desktop kind of client. A Mac is the one
// Apple device that has that kind of Moonlight, so it asks as one, on purpose.
#if os(macOS)
private let rigClient = "desktop"
#else
private let rigClient = "apple"
#endif

#if os(macOS)
/// Runs the desktop Moonlight, if it's installed where it installs. This app is sandboxed and
/// a program it starts stays inside that sandbox, so this may not be enough on a real Mac: it
/// has never been tried. Starting the game on the PC, below it, always remains.
private func runMoonlight(_ args: [String]) -> Bool {
    guard MoonlightCommand.isSafe(args), FileManager.default.isExecutableFile(atPath: MoonlightCommand.macProgram) else { return false }
    let process = Process()
    process.executableURL = URL(fileURLWithPath: MoonlightCommand.macProgram)
    process.arguments = args
    process.standardOutput = FileHandle.nullDevice
    process.standardError = FileHandle.nullDevice
    do {
        try process.run()
        return true
    } catch {
        return false
    }
}
#endif

/// Nothing here is ever disabled, because on Apple TV a disabled button loses the focus.
struct RigPlayPanel: View {
    @EnvironmentObject private var model: AppModel
    let item: Item
    @State private var play: RigPlay?
    @State private var steps: [String] = []
    @State private var said = ""
    @State private var error = ""
    @State private var busy = false

    private func work(_ job: @escaping () async throws -> Void) {
        guard !busy else { return }
        busy = true
        error = ""
        said = ""
        Task {
            do { try await job() } catch { self.error = model.handle(error) }
            busy = false
        }
    }

    private func ask() async throws {
        play = try await model.client.rigPlay(item.id, client: rigClient, os: rigOS)
    }

    @ViewBuilder private func lines(_ list: [String]) -> some View {
        ForEach(Array(list.enumerated()), id: \.offset) { _, line in Text(line) }
    }

    @ViewBuilder private func playing(_ play: RigPlay) -> some View {
        #if os(iOS)
        if play.playsInPage {
            if let note = play.inPageNote, !note.isEmpty { Text(note).foregroundStyle(.secondary) }
            NavigationLink("Play here", value: Route.web(WebPlayer.rigPath(item.id)))
        }
        #endif
        #if os(macOS)
        if play.playsInPage, let url = model.client.mediaURL(WebPlayer.rigPath(item.id)) {
            // No web view here, as a Mac has real browsers. It may need signing in there.
            Link("Play in your browser", destination: url)
        }
        if let args = play.moonlightArgs, FileManager.default.isExecutableFile(atPath: MoonlightCommand.macProgram) {
            Button("Open in Moonlight") {
                if !runMoonlight(args) { error = "Moonlight couldn't be started. Start the game on the PC instead." }
            }
        }
        #endif
        Button("Start on \(play.rig.name)") {
            work {
                let started = try await model.client.rigStart(item.id)
                steps = started.steps ?? []
            }
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if !error.isEmpty { Text(error).foregroundStyle(.red) }
            if !said.isEmpty { Text(said) }
            if let play {
                if let message = play.message, !message.isEmpty { Text(message).foregroundStyle(.secondary) }
                if play.isReady {
                    playing(play)
                    if !steps.isEmpty {
                        lines(steps)
                    } else {
                        lines(play.steps).foregroundStyle(.secondary)
                    }
                } else if play.rig.canWake == true {
                    Button("Wake \(play.rig.name)") {
                        work {
                            let woken = try await model.client.rigWake(play.rig.id)
                            said = woken.note ?? "A wake-up signal was sent."
                        }
                    }
                }
                Button("Check again") { work { steps = []; try await ask() } }
                if let install = play.install {
                    let license = install.license.map { " (\($0))" } ?? ""
                    Text("This needs \(install.name), a separate, free app\(license).").foregroundStyle(.secondary)
                    if let note = install.note, !note.isEmpty { Text(note).foregroundStyle(.secondary) }
                    #if !os(tvOS)
                    if let url = install.webURL { Link("Get \(install.name)", destination: url) }
                    #endif
                }
                if let setup = play.setup, !setup.isEmpty {
                    Text("Setting up Moonlight").font(.headline)
                    lines(setup).foregroundStyle(.secondary)
                }
            } else if error.isEmpty {
                ProgressView()
            } else {
                Button("Try again") { work { try await ask() } }
            }
        }
        .task(id: item.id) {
            do { try await ask() } catch { self.error = model.handle(error) }
        }
    }
}
