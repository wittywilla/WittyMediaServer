import ImageIO
import SwiftUI
import WittyKit

/// Posters are decoded at the size shown, and only a bounded number stay in memory.
private final class PosterCache {
    static let shared: NSCache<NSURL, CGImage> = {
        let cache = NSCache<NSURL, CGImage>()
        cache.countLimit = 120
        return cache
    }()
}

private func decodePoster(_ data: Data, maxPixels: Int) -> CGImage? {
    guard let source = CGImageSourceCreateWithData(data as CFData, nil) else { return nil }
    let options: [CFString: Any] = [
        kCGImageSourceCreateThumbnailFromImageAlways: true,
        kCGImageSourceCreateThumbnailWithTransform: true,
        kCGImageSourceThumbnailMaxPixelSize: maxPixels,
    ]
    return CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary)
}

#if os(tvOS)
private let posterWidth: CGFloat = 220
#else
private let posterWidth: CGFloat = 120
#endif

struct PosterImage: View {
    @EnvironmentObject private var model: AppModel
    let url: URL?
    @State private var image: CGImage?

    var body: some View {
        ZStack {
            Rectangle().fill(Color.gray.opacity(0.2))
            if let image {
                Image(decorative: image, scale: 1).resizable().scaledToFill()
            }
        }
        .frame(width: posterWidth, height: posterWidth * 1.5)
        .clipped()
        .task(id: url) {
            guard let url else { return }
            if let cached = PosterCache.shared.object(forKey: url as NSURL) { image = cached; return }
            guard let data = try? await model.client.imageData(url) else { return }
            let decoded = await Task.detached(priority: .utility) { decodePoster(data, maxPixels: Int(posterWidth * 3)) }.value
            if let decoded { PosterCache.shared.setObject(decoded, forKey: url as NSURL) }
            image = decoded
        }
    }
}

private struct Tile: View {
    let title: String
    let subtitle: String
    let url: URL?
    let route: Route

    var body: some View {
        NavigationLink(value: route) {
            VStack(alignment: .leading, spacing: 4) {
                PosterImage(url: url)
                Text(title).lineLimit(1).font(.footnote)
                if !subtitle.isEmpty { Text(subtitle).lineLimit(1).font(.caption).foregroundStyle(.secondary) }
            }
            .frame(width: posterWidth)
        }
        .tileStyle()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(subtitle.isEmpty ? title : "\(title), \(subtitle)")
    }
}

private struct ItemTile: View {
    @EnvironmentObject private var model: AppModel
    let item: Item
    var body: some View {
        let title = (item.showTitle?.isEmpty == false ? item.showTitle : nil) ?? item.title
        let subtitle = item.episodeLabel.isEmpty ? (item.year.map(String.init) ?? "") : item.episodeLabel
        Tile(title: title, subtitle: subtitle, url: model.client.posterURL(item), route: .item(item.id))
    }
}

private struct ShowTile: View {
    @EnvironmentObject private var model: AppModel
    let show: Show
    var body: some View {
        let subtitle = (show.unwatched ?? 0) > 0 ? "\(show.unwatched!) unwatched" : (show.year.map(String.init) ?? "")
        Tile(title: show.title, subtitle: subtitle, url: model.client.posterURL(show), route: .show(show.id))
    }
}

/// Only these library types list things these apps can open for now.
private let browsableTypes: Set<String> = ["movies", "shows", "anime", "videos", "discs"]

struct HomeView: View {
    @EnvironmentObject private var model: AppModel
    @State private var home: Home?
    @State private var error = ""

    private func load() async {
        error = ""
        do { home = try await model.client.home() } catch { self.error = model.handle(error) }
    }

    var body: some View {
        Group {
            if !error.isEmpty {
                VStack(spacing: 16) {
                    Text(error).foregroundStyle(.red)
                    Button("Try again") { Task { await load() } }
                    Button(say("changeServer")) { model.changeServer() }
                }
            } else if let home {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 24) {
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(spacing: 12) {
                                ForEach((home.libraries ?? []).filter { browsableTypes.contains($0.type) }) { library in
                                    NavigationLink(library.name, value: Route.library(library))
                                }
                                Button("Switch profile") { model.signOut() }
                            }
                            .padding(.vertical, 8)
                        }
                        ForEach((home.hubs ?? []).filter { !$0.isEmpty }) { hub in
                            VStack(alignment: .leading, spacing: 8) {
                                Text(hub.title).font(.headline)
                                ScrollView(.horizontal, showsIndicators: false) {
                                    LazyHStack(alignment: .top, spacing: 16) {
                                        ForEach(hub.items ?? []) { ItemTile(item: $0) }
                                        ForEach(hub.shows ?? []) { ShowTile(show: $0) }
                                    }
                                    .padding(.vertical, 8)
                                }
                            }
                        }
                    }
                    .padding(24)
                }
            } else {
                ProgressView()
            }
        }
        .task { await load() }
    }
}

struct LibraryView: View {
    @EnvironmentObject private var model: AppModel
    let library: Library
    @State private var items: [Item] = []
    @State private var shows: [Show] = []
    @State private var total: Int?
    @State private var error = ""
    private let pageSize = 60

    private func loadMore() async {
        do {
            if library.isShows {
                let page = try await model.client.shows(library: library.id)
                shows = page.shows ?? []
                total = page.total ?? shows.count
            } else {
                let page = try await model.client.items(library: library.id, offset: items.count, limit: pageSize)
                let seen = Set(items.map(\.id))
                items += (page.items ?? []).filter { !seen.contains($0.id) }
                total = page.total ?? items.count
            }
        } catch {
            self.error = model.handle(error)
        }
    }

    var body: some View {
        Group {
            if !error.isEmpty {
                Text(error).foregroundStyle(.red)
            } else if total == nil {
                ProgressView()
            } else if total == 0 {
                Text("Nothing here yet.").foregroundStyle(.secondary)
            } else {
                ScrollView {
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: posterWidth + 16))], spacing: 20) {
                        ForEach(shows) { ShowTile(show: $0) }
                        ForEach(items) { item in
                            ItemTile(item: item).task {
                                if item.id == items.last?.id, items.count < (total ?? 0) { await loadMore() }
                            }
                        }
                    }
                    .padding(24)
                }
            }
        }
        .navigationTitle(library.name)
        .task { if total == nil { await loadMore() } }
    }
}

struct ShowView: View {
    @EnvironmentObject private var model: AppModel
    let id: Int64
    @State private var detail: ShowDetail?
    @State private var season: Int?
    @State private var error = ""

    var body: some View {
        Group {
            if !error.isEmpty {
                Text(error).foregroundStyle(.red)
            } else if let detail {
                let selected = season ?? detail.seasons?.first?.number
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 12) {
                        Text(detail.show.title).font(.title2).bold()
                        if let overview = detail.show.overview, !overview.isEmpty {
                            Text(overview).foregroundStyle(.secondary).lineLimit(4)
                        }
                        if let next = detail.nextUpId {
                            NavigationLink("Next up", value: Route.item(next))
                        }
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(spacing: 10) {
                                ForEach(detail.seasons ?? []) { s in
                                    Button(s.displayTitle) { season = s.number }
                                        .fontWeight(s.number == selected ? .bold : .regular)
                                }
                            }
                            .padding(.vertical, 8)
                        }
                        ForEach((detail.episodes ?? []).filter { $0.season == selected }) { episode in
                            NavigationLink(value: Route.item(episode.id)) {
                                VStack(alignment: .leading) {
                                    Text("\(episode.episodeLabel)  \(episode.title)")
                                    if episode.progress?.watched == true {
                                        Text("Watched").font(.caption).foregroundStyle(.secondary)
                                    }
                                }
                                .frame(maxWidth: .infinity, alignment: .leading)
                            }
                        }
                    }
                    .padding(24)
                }
            } else {
                ProgressView()
            }
        }
        .task(id: id) {
            do { detail = try await model.client.show(id) } catch { self.error = model.handle(error) }
        }
    }
}

private func runtimeText(_ seconds: Double?) -> String? {
    guard let seconds, seconds > 0 else { return nil }
    let hours = Int(seconds) / 3600
    let minutes = (Int(seconds) % 3600) / 60
    return hours > 0 ? "\(hours) h \(minutes) min" : "\(minutes) min"
}

struct ItemView: View {
    @EnvironmentObject private var model: AppModel
    let id: Int64
    @State private var item: Item?
    @State private var error = ""

    var body: some View {
        Group {
            if !error.isEmpty {
                Text(error).foregroundStyle(.red)
            } else if let item {
                ScrollView {
                    VStack(alignment: .leading, spacing: 12) {
                        if let show = item.showTitle, !show.isEmpty {
                            Text("\(show) · \(item.episodeLabel)").foregroundStyle(.secondary)
                        }
                        Text(item.title).font(.title2).bold()
                        Text([item.year.map(String.init), runtimeText(item.runtimeSec), item.contentRating?.isEmpty == false ? item.contentRating : nil]
                            .compactMap { $0 }.joined(separator: " · ")).foregroundStyle(.secondary)
                        if let genres = item.genres, !genres.isEmpty {
                            Text(genres.joined(separator: ", ")).foregroundStyle(.secondary)
                        }
                        if item.isPlayable {
                            HStack(spacing: 12) {
                                if item.resumeSeconds != nil {
                                    NavigationLink("Resume", value: Route.player(item.id, startOver: false))
                                    NavigationLink("Start over", value: Route.player(item.id, startOver: true))
                                } else {
                                    NavigationLink("Play", value: Route.player(item.id, startOver: true))
                                }
                            }
                        } else {
                            Text("This kind of item can't play in this app yet. Open it in a web browser.").foregroundStyle(.secondary)
                        }
                        if let overview = item.overview, !overview.isEmpty { Text(overview) }
                    }
                    .frame(maxWidth: 900, alignment: .leading)
                    .padding(32)
                }
            } else {
                ProgressView()
            }
        }
        .task(id: id) {
            do { item = try await model.client.item(id) } catch { self.error = model.handle(error) }
        }
    }
}
