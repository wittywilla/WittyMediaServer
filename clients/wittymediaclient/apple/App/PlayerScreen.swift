import AVKit
import SwiftUI
import VideoToolbox
import WittyKit

/// Drives AVPlayer: asks the server what to play, saves progress every ten
/// seconds and moves on to the next episode. The rules are in docs/CLIENT-API.md.
@MainActor
final class PlaybackModel: ObservableObject {
    let player = AVPlayer()
    @Published var item: Item?
    @Published var stream: Playback?
    @Published var error = ""
    @Published var filmSeconds = 0.0
    /// Seconds left before the next episode starts, while counting down.
    @Published var countdown: Int?

    private var timeObserver: Any?
    private var endObserver: NSObjectProtocol?
    private var failObserver: NSObjectProtocol?
    private var statusObserver: NSKeyValueObservation?
    private var failedItem: ObjectIdentifier?
    private var lastSave = 0.0
    private var client: WittyClient?
    private var id: Int64 = 0
    private var audio: Int?
    private var subtitle: Int?
    private var retries = 0
    private var countdownTask: Task<Void, Never>?

    var onFinished: ((Int64?) -> Void)?

    /// Apple TV HD and anything without HEVC hardware gets the safer profile.
    private var profile: String {
        VTIsHardwareDecodeSupported(kCMVideoCodecType_HEVC) ? "appletv" : "appletv-hd"
    }

    /// Inside the intro: where to skip to.
    var introEnd: Double? {
        guard let intro = stream?.intro, filmSeconds >= intro.startSec, filmSeconds < intro.endSec - 1 else { return nil }
        return intro.endSec
    }

    func start(id: Int64, client: WittyClient, startOver: Bool) async {
        self.id = id
        self.client = client
        timeObserver = player.addPeriodicTimeObserver(forInterval: CMTime(seconds: 1, preferredTimescale: 1), queue: .main) { [weak self] time in
            MainActor.assumeIsolated { self?.tick(time.seconds) }
        }
        do {
            item = try await client.item(id)
        } catch {
            self.error = error.localizedDescription
            return
        }
        // Left out, startSec lets the server resume where the viewer stopped.
        await open(startSec: startOver ? 0 : nil)
    }

    func choose(audio: Int? = nil, subtitle: Int? = nil) {
        if let audio { self.audio = audio }
        if let subtitle { self.subtitle = subtitle }
        let at = currentFilmSeconds
        Task { await open(startSec: at) }
    }

    private var currentFilmSeconds: Double {
        let inPlayer = player.currentTime().seconds
        return (inPlayer.isFinite ? inPlayer : 0) + (stream?.offset ?? 0)
    }

    private func open(startSec: Double?) async {
        guard let client else { return }
        do {
            let answer = try await client.playback(id, PlaybackRequest(profile: profile, audio: audio, subtitle: subtitle, startSec: startSec))
            let changed = answer.url != stream?.url
            stream = answer
            ClientLog.shared.session = answer.streamId ?? ""
            if changed {
                guard let url = client.mediaURL(answer.url) else { error = "The server sent an address that can't be used."; return }
                // The address already carries a short-lived stream token.
                let next = AVPlayerItem(url: url)
                observe(next)
                player.replaceCurrentItem(with: next)
                if let at = answer.startAtSec, at > 0 { await player.seek(to: CMTime(seconds: at, preferredTimescale: 600)) }
                player.play()
            }
            await pickSubtitleRendition(answer)
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// Text subtitles arrive as renditions in the playlist, and the player switches among them.
    private func pickSubtitleRendition(_ answer: Playback) async {
        guard let current = player.currentItem,
              let group = try? await current.asset.loadMediaSelectionGroup(for: .legible) else { return }
        let chosen = answer.subtitles?.first { $0.index == answer.subtitleIndex }
        guard answer.subtitleDelivery == "hls", let chosen else {
            current.select(nil, in: group)
            return
        }
        let wanted = group.options.first { $0.extendedLanguageTag == chosen.language || $0.locale?.identifier == chosen.language }
        current.select(wanted ?? group.options.first, in: group)
    }

    private func observe(_ next: AVPlayerItem) {
        if let endObserver { NotificationCenter.default.removeObserver(endObserver) }
        if let failObserver { NotificationCenter.default.removeObserver(failObserver) }
        endObserver = NotificationCenter.default.addObserver(forName: .AVPlayerItemDidPlayToEndTime, object: next, queue: .main) { [weak self] _ in
            MainActor.assumeIsolated { self?.finished() }
        }
        let itemId = ObjectIdentifier(next)
        failedItem = nil
        failObserver = NotificationCenter.default.addObserver(forName: .AVPlayerItemFailedToPlayToEndTime, object: next, queue: .main) { [weak self] note in
            let problem = note.userInfo?[AVPlayerItemFailedToPlayToEndTimeErrorKey] as? Error
            MainActor.assumeIsolated { self?.failed(problem, item: itemId) }
        }
        // A stream that never starts only shows up here, not in the notification above.
        statusObserver = next.observe(\.status) { [weak self] item, _ in
            guard item.status == .failed else { return }
            let problem = item.error
            Task { @MainActor [weak self] in self?.failed(problem, item: itemId) }
        }
    }

    /// An expired token or a stream that went away: ask again from the same place.
    /// One item can say it failed in both of the ways above, which counts as one.
    private func failed(_ problem: Error?, item: ObjectIdentifier) {
        guard failedItem != item else { return }
        failedItem = item
        let message = problem?.localizedDescription
        let detail = problem.map { problem -> String in
            let found = problem as NSError
            return "\(found.domain) \(found.code): \(found.localizedDescription)"
        } ?? "no details"
        ClientLog.shared.report(retries < 2 ? "warn" : "error", "player", "Playback failed: \(detail)", session: stream?.streamId ?? "")
        guard retries < 2 else { error = message ?? "This can't be played."; return }
        retries += 1
        stream = nil
        let at = filmSeconds
        Task { await open(startSec: at) }
    }

    private func finished() {
        guard stream?.nextId != nil else { onFinished?(nil); return }
        countdown = 8
        countdownTask = Task { [weak self] in
            while let left = self?.countdown, left > 0 {
                try? await Task.sleep(nanoseconds: 1_000_000_000)
                if Task.isCancelled { return }
                self?.countdown = left - 1
            }
            self?.onFinished?(self?.stream?.nextId)
        }
    }

    func cancelCountdown() {
        countdownTask?.cancel()
        countdown = nil
        onFinished?(nil)
    }

    private func tick(_ inPlayer: Double) {
        guard inPlayer.isFinite else { return }
        filmSeconds = inPlayer + (stream?.offset ?? 0)
        if player.timeControlStatus == .playing {
            retries = 0
            if abs(filmSeconds - lastSave) >= 10 {
                lastSave = filmSeconds
                save()
            }
        }
    }

    /// Goes past what a growing stream has so far by opening a new one.
    func seek(toFilm target: Double) {
        guard let current = stream else { return }
        let inPlayer = target - current.offset
        let known = player.currentItem?.seekableTimeRanges.last?.timeRangeValue.end.seconds ?? 0
        if !current.mustReopenToSeekFar || (inPlayer >= 0 && inPlayer <= known) {
            player.seek(to: CMTime(seconds: inPlayer, preferredTimescale: 600))
        } else {
            Task { await open(startSec: target) }
        }
    }

    func skipIntro() {
        if let end = introEnd { seek(toFilm: end) }
    }

    /// In its own task: the last save happens as the screen is leaving.
    private func save() {
        guard let client, let duration = stream?.durationSec, filmSeconds >= 5 else { return }
        let (id, at) = (id, filmSeconds)
        Task.detached { try? await client.saveProgress(id, positionSec: at, durationSec: duration) }
    }

    func stop() {
        save()
        player.pause()
        countdownTask?.cancel()
        if let timeObserver { player.removeTimeObserver(timeObserver) }
        timeObserver = nil
        if let endObserver { NotificationCenter.default.removeObserver(endObserver) }
        endObserver = nil
        if let failObserver { NotificationCenter.default.removeObserver(failObserver) }
        failObserver = nil
        statusObserver = nil
        ClientLog.shared.session = ""
        player.replaceCurrentItem(with: nil)
        // Stops the conversion straight away instead of letting it run on.
        if let client, let streamId = stream?.streamId, !streamId.isEmpty {
            Task.detached { try? await client.stopStream(streamId) }
        }
    }
}

#if os(macOS)
private struct PlayerHost: NSViewRepresentable {
    @ObservedObject var playback: PlaybackModel
    func makeNSView(context: Context) -> AVPlayerView {
        let view = AVPlayerView()
        view.player = playback.player
        view.controlsStyle = .floating
        return view
    }
    func updateNSView(_ view: AVPlayerView, context: Context) {}
}
#else
private struct PlayerHost: UIViewControllerRepresentable {
    @ObservedObject var playback: PlaybackModel

    func makeUIViewController(context: Context) -> AVPlayerViewController {
        let controller = AVPlayerViewController()
        controller.player = playback.player
        return controller
    }

    func updateUIViewController(_ controller: AVPlayerViewController, context: Context) {
        #if os(tvOS)
        // On Apple TV the choices belong in the system player's own bar, where the remote reaches them.
        var menus: [UIMenuElement] = []
        let stream = playback.stream
        if let audio = stream?.audio, audio.count > 1 {
            menus.append(UIMenu(title: "Audio", image: UIImage(systemName: "speaker.wave.2"), children: audio.map { track in
                UIAction(title: track.label, state: track.index == stream?.audioIndex ? .on : .off) { _ in playback.choose(audio: track.index) }
            }))
        }
        if let subtitles = stream?.subtitles, !subtitles.isEmpty {
            let off = UIAction(title: "Off", state: stream?.subtitleIndex == -1 ? .on : .off) { _ in playback.choose(subtitle: -1) }
            menus.append(UIMenu(title: "Subtitles", image: UIImage(systemName: "captions.bubble"), children: [off] + subtitles.map { track in
                UIAction(title: track.label, state: track.index == stream?.subtitleIndex ? .on : .off) { _ in playback.choose(subtitle: track.index) }
            }))
        }
        controller.transportBarCustomMenuItems = menus
        controller.contextualActions = playback.introEnd == nil ? [] : [UIAction(title: "Skip intro") { _ in playback.skipIntro() }]
        #endif
    }
}
#endif

struct PlayerScreen: View {
    @EnvironmentObject private var model: AppModel
    @StateObject private var playback = PlaybackModel()
    let id: Int64
    let startOver: Bool

    var body: some View {
        ZStack(alignment: .topTrailing) {
            Color.black.ignoresSafeArea()
            if playback.error.isEmpty {
                PlayerHost(playback: playback).ignoresSafeArea()
            } else {
                Text(playback.error).foregroundStyle(.red).padding(32).frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            if let left = playback.countdown {
                VStack(spacing: 16) {
                    Text("\(say("nextEpisodeIn")) \(left)").font(.title2)
                    Button(say("cancel")) { playback.cancelCountdown() }
                }
                .padding(32)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            #if !os(tvOS)
            // The system player has no place for these on iPhone, iPad and Mac.
            HStack(spacing: 12) {
                if playback.introEnd != nil {
                    Button("Skip intro") { playback.skipIntro() }
                }
                if let audio = playback.stream?.audio, audio.count > 1 {
                    Menu("Audio") {
                        ForEach(audio) { track in
                            Button(track.label) { playback.choose(audio: track.index) }
                        }
                    }
                }
                if let subtitles = playback.stream?.subtitles, !subtitles.isEmpty {
                    Menu("Subtitles") {
                        Button("Off") { playback.choose(subtitle: -1) }
                        ForEach(subtitles) { track in
                            Button(track.label) { playback.choose(subtitle: track.index) }
                        }
                    }
                }
            }
            .buttonStyle(.bordered)
            .padding()
            #endif
        }
        .task {
            playback.onFinished = { next in
                guard !model.path.isEmpty else { return }
                model.path.removeLast()
                if let next { model.path.append(.player(next, startOver: false)) }
            }
            await playback.start(id: id, client: model.client, startOver: startOver)
        }
        .onDisappear { playback.stop() }
    }
}
