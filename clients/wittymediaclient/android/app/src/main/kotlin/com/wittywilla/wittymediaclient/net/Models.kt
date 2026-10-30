package com.wittywilla.wittymediaclient.net

import kotlinx.serialization.Serializable

// What the server sends, keeping only the fields this app uses. Times are in
// seconds and may be fractional. Lists may arrive as null.

@Serializable
data class ServerStatus(
    val name: String = "",
    val version: String = "",
    val setupRequired: Boolean = false,
    val brandName: String = "",
    val theme: String = "default",
    val user: User? = null,
)

@Serializable
data class User(
    val id: Long = 0,
    val name: String = "",
    val isAdmin: Boolean = false,
    val kind: String = "adult",
)

@Serializable
data class Profile(
    val id: Long = 0,
    val name: String = "",
    val kind: String = "adult",
    val hasPassword: Boolean = true,
)

@Serializable
data class Profiles(val style: String = "classic", val profiles: List<Profile> = emptyList())

@Serializable
data class LoginResult(val token: String = "", val user: User? = null)

@Serializable
data class Library(
    val id: Long = 0,
    val name: String = "",
    val type: String = "",
    val itemCount: Int = 0,
)

@Serializable
data class Progress(
    val positionSec: Double = 0.0,
    val durationSec: Double = 0.0,
    val watched: Boolean = false,
)

@Serializable
data class Item(
    val id: Long = 0,
    val libraryId: Long = 0,
    val kind: String = "video",
    val title: String = "",
    val year: Int? = null,
    val overview: String = "",
    val genres: List<String> = emptyList(),
    val contentRating: String = "",
    val runtimeSec: Double = 0.0,
    val updatedAt: Long = 0,
    val progress: Progress? = null,
    val showId: Long? = null,
    val showTitle: String? = null,
    val season: Int? = null,
    val episode: Int? = null,
    val episodeEnd: Int? = null,
    val nextId: Long? = null,
    val introStart: Double? = null,
    val introEnd: Double? = null,
)

@Serializable
data class Show(
    val id: Long = 0,
    val title: String = "",
    val year: Int? = null,
    val overview: String = "",
    val updatedAt: Long = 0,
    val episodeCount: Int = 0,
    val seasonCount: Int = 0,
    val unwatched: Int = 0,
)

@Serializable
data class Season(
    val number: Int = 0,
    val title: String = "",
    val episodeCount: Int = 0,
    val unwatched: Int = 0,
)

@Serializable
data class ShowDetail(
    val show: Show = Show(),
    val seasons: List<Season> = emptyList(),
    val episodes: List<Item> = emptyList(),
    val nextUpId: Long? = null,
)

@Serializable
data class Hub(
    val key: String = "",
    val title: String = "",
    val libraryId: Long? = null,
    val items: List<Item> = emptyList(),
    val shows: List<Show> = emptyList(),
)

@Serializable
data class Home(
    val hubs: List<Hub> = emptyList(),
    val featured: List<Item> = emptyList(),
    val libraries: List<Library> = emptyList(),
)

@Serializable
data class ItemPage(val items: List<Item> = emptyList(), val total: Int = 0)

@Serializable
data class ShowPage(val shows: List<Show> = emptyList(), val total: Int = 0)

@Serializable
data class Track(
    val index: Int = 0,
    val language: String = "",
    val title: String = "",
    val codec: String = "",
    val channels: Int = 0,
)

@Serializable
data class PlayInfo(
    val type: String = "hls",
    val url: String = "",
    val durationSec: Double = 0.0,
    val audio: List<Track> = emptyList(),
    val subtitles: List<Track> = emptyList(),
    val audioIndex: Int = 0,
    val subtitleIndex: Int = -1,
)

@Serializable
data class ProgressReport(val positionSec: Double, val durationSec: Double)

@Serializable
data class Credentials(val name: String, val password: String, val device: String)
