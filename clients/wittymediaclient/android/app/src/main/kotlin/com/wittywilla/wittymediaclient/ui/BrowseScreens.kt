package com.wittywilla.wittymediaclient.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.wittywilla.wittymediaclient.net.Home
import com.wittywilla.wittymediaclient.net.Item
import com.wittywilla.wittymediaclient.net.Library
import com.wittywilla.wittymediaclient.net.Show
import com.wittywilla.wittymediaclient.net.ShowDetail
import com.wittywilla.wittymediaclient.net.isRigGame
import com.wittywilla.wittymediaclient.net.isRigGames

private fun posterWidth(nav: Nav) = if (nav.app.isTv) 150 else 112

// Only these kinds play here for now. Discs, music and books stay on the web
// client until their phases: see docs/DESIGN.md.
const val PLAYABLE = "video"
private val BROWSABLE_LIBRARIES = setOf("movies", "shows", "anime", "videos", "discs")

fun episodeLabel(it: Item): String {
    val season = it.season ?: return ""
    val episode = it.episode ?: return ""
    if (episode < 0) return ""
    val ep = if (it.episodeEnd != null && it.episodeEnd != 0) "E$episode–${it.episodeEnd}" else "E$episode"
    return if (season == 0) "Special · $ep" else "S$season · $ep"
}

private fun itemSubtitle(it: Item) = episodeLabel(it).ifEmpty { it.year?.toString().orEmpty() }

@Composable
private fun ItemCard(nav: Nav, it: Item, requestFocus: Boolean = false) {
    PosterCard(it.showTitle?.takeIf { s -> s.isNotEmpty() } ?: it.title, itemSubtitle(it), nav.app.api.posterUrl(it), { nav.push(Route.Item(it.id)) }, posterWidth(nav), requestFocus = requestFocus)
}

@Composable
private fun ShowCard(nav: Nav, s: Show, requestFocus: Boolean = false) {
    val label = if (s.unwatched > 0) "${s.unwatched} unwatched" else s.year?.toString().orEmpty()
    PosterCard(s.title, label, nav.app.api.showPosterUrl(s), { nav.push(Route.Show(s.id)) }, posterWidth(nav), requestFocus = requestFocus)
}

@Composable
private fun LoadFailed(nav: Nav, message: String, retry: () -> Unit) {
    Column(Modifier.fillMaxSize().padding(32.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        ErrorText(message)
        ActionButton("Try again", retry, requestFocus = true)
        ActionButton("Change server", nav::changeServer)
    }
}

@Composable
fun HomeScreen(nav: Nav, s: Map<String, String>) {
    var home by remember { mutableStateOf<Home?>(null) }
    var error by remember { mutableStateOf("") }
    var attempt by remember { mutableStateOf(0) }
    LaunchedEffect(attempt) {
        error = ""
        try {
            home = nav.app.api.home()
        } catch (e: Exception) {
            if (!nav.handleExpired(e)) error = s["errLost"].orEmpty()
        }
    }
    Screen {
        val h = home
        when {
            error.isNotEmpty() -> LoadFailed(nav, error) { attempt++ }
            h == null -> Loading()
            else -> LazyColumn(contentPadding = PaddingValues(24.dp), verticalArrangement = Arrangement.spacedBy(20.dp)) {
                item {
                    val libraries = h.libraries.filter { it.type in BROWSABLE_LIBRARIES || it.isRigGames }
                    LazyRow(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        items(libraries, key = { it.id }) { lib ->
                            ActionButton(lib.name, { nav.push(Route.Library(lib)) }, requestFocus = lib.id == libraries.first().id)
                        }
                        item(key = "signout") { ActionButton("Switch profile", nav::signOut) }
                    }
                }
                items(h.hubs.filter { it.items.isNotEmpty() || it.shows.isNotEmpty() }, key = { it.key }) { hub ->
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text(hub.title, color = MaterialTheme.colorScheme.onBackground, style = MaterialTheme.typography.titleMedium)
                        LazyRow(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                            items(hub.items, key = { "i${it.id}" }) { ItemCard(nav, it) }
                            items(hub.shows, key = { "s${it.id}" }) { ShowCard(nav, it) }
                        }
                    }
                }
            }
        }
    }
}

private const val PAGE = 60

@Composable
fun LibraryScreen(nav: Nav, library: Library) {
    val isShows = library.type == "shows" || library.type == "anime"
    val items = remember { mutableStateListOf<Item>() }
    val shows = remember { mutableStateListOf<Show>() }
    var total by remember { mutableStateOf(-1) }
    var error by remember { mutableStateOf("") }
    var loadedTo by remember { mutableStateOf(0) }

    LaunchedEffect(library.id, loadedTo) {
        try {
            if (isShows) {
                val page = nav.app.api.shows(library.id)
                shows.clear()
                shows.addAll(page.shows)
                total = page.total
            } else {
                val page = nav.app.api.items(library.id, items.size, PAGE)
                // Repeated keys crash a lazy grid, and the library can change between pages.
                val seen = items.mapTo(HashSet()) { it.id }
                items.addAll(page.items.filter { it.id !in seen })
                total = page.total
            }
        } catch (e: Exception) {
            if (!nav.handleExpired(e)) error = e.message.orEmpty()
        }
    }

    Screen {
        when {
            error.isNotEmpty() -> LoadFailed(nav, error) { error = ""; loadedTo++ }
            total < 0 -> Loading()
            else -> Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Heading(library.name)
                if (total == 0) Secondary("Nothing here yet.")
                LazyVerticalGrid(GridCells.Adaptive((posterWidth(nav) + 10).dp), horizontalArrangement = Arrangement.spacedBy(10.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                    if (isShows) {
                        items(shows, key = { it.id }) { ShowCard(nav, it, requestFocus = it.id == shows.first().id) }
                    } else {
                        items(items, key = { it.id }) {
                            ItemCard(nav, it, requestFocus = it.id == items.first().id)
                            // Near the end of what's loaded: fetch the next page.
                            if (it.id == items.lastOrNull()?.id && items.size < total) LaunchedEffect(items.size) { loadedTo = items.size }
                        }
                    }
                }
            }
        }
    }
}

@Composable
fun ShowScreen(nav: Nav, id: Long) {
    var detail by remember { mutableStateOf<ShowDetail?>(null) }
    var error by remember { mutableStateOf("") }
    var season by rememberSaveable { mutableStateOf<Int?>(null) }
    LaunchedEffect(id) {
        try {
            detail = nav.app.api.show(id)
        } catch (e: Exception) {
            if (!nav.handleExpired(e)) error = e.message.orEmpty()
        }
    }
    Screen {
        val d = detail
        when {
            error.isNotEmpty() -> ErrorText(error, Modifier.padding(32.dp))
            d == null -> Loading()
            else -> {
                val selected = season ?: d.seasons.firstOrNull()?.number
                LazyColumn(contentPadding = PaddingValues(24.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    item {
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Heading(d.show.title)
                            if (d.show.overview.isNotEmpty()) Secondary(d.show.overview, maxLines = 4)
                            d.nextUpId?.let { next ->
                                ActionButton("Next up", { nav.push(Route.Item(next)) }, primary = true, requestFocus = true)
                            }
                        }
                    }
                    item {
                        LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            items(d.seasons, key = { it.number }) { s ->
                                val title = s.title.ifEmpty { if (s.number == 0) "Specials" else "Season ${s.number}" }
                                ActionButton(title, { season = s.number }, primary = s.number == selected, requestFocus = d.nextUpId == null && s.number == selected)
                            }
                        }
                    }
                    items(d.episodes.filter { it.season == selected }, key = { it.id }) { ep ->
                        FocusBox({ nav.push(Route.Item(ep.id)) }, Modifier.fillMaxWidth(), label = "${episodeLabel(ep)}, ${ep.title}") {
                            Column(Modifier.padding(10.dp)) {
                                Text("${episodeLabel(ep)}  ${ep.title}", color = MaterialTheme.colorScheme.onBackground)
                                if (ep.progress?.watched == true) Secondary("Watched")
                            }
                        }
                    }
                }
            }
        }
    }
}

private fun runtimeText(sec: Double): String {
    if (sec <= 0) return ""
    val h = (sec / 3600).toInt()
    val m = ((sec % 3600) / 60).toInt()
    return if (h > 0) "$h h $m min" else "$m min"
}

@Composable
fun ItemScreen(nav: Nav, id: Long) {
    var item by remember { mutableStateOf<Item?>(null) }
    var error by remember { mutableStateOf("") }
    LaunchedEffect(id) {
        try {
            item = nav.app.api.item(id)
        } catch (e: Exception) {
            if (!nav.handleExpired(e)) error = e.message.orEmpty()
        }
    }
    Screen {
        val it = item
        when {
            error.isNotEmpty() -> ErrorText(error, Modifier.padding(32.dp))
            it == null -> Loading()
            else -> Row(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(32.dp), horizontalArrangement = Arrangement.spacedBy(24.dp)) {
                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    if (!it.showTitle.isNullOrEmpty()) Secondary("${it.showTitle} · ${episodeLabel(it)}")
                    Heading(it.title)
                    Secondary(listOfNotNull(it.year?.toString(), runtimeText(it.runtimeSec).ifEmpty { null }, it.contentRating.ifEmpty { null }).joinToString(" · "))
                    if (it.genres.isNotEmpty()) Secondary(it.genres.joinToString(", "))
                    if (it.isRigGame) {
                        if (it.tagline.isNotEmpty()) Secondary(it.tagline)
                        RigPlayPanel(nav, it)
                    } else if (it.kind == PLAYABLE) {
                        // The server decides where Resume goes; 0 means start over.
                        val resume = (it.progress?.resumeSec ?: 0.0) > 0
                        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                            ActionButton(if (resume) "Resume" else "Play", { nav.push(Route.Player(it.id, false)) }, primary = true, requestFocus = true)
                            if (resume) ActionButton("Start over", { nav.push(Route.Player(it.id, true)) })
                        }
                    } else {
                        Secondary("This kind of item can't play in this app yet. Open it in a web browser.")
                        ActionButton("Back", nav::pop, requestFocus = true)
                    }
                    if (it.overview.isNotEmpty()) Secondary(it.overview)
                }
            }
        }
    }
}
