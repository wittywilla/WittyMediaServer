package com.wittywilla.wittymediaclient.player

import android.view.KeyEvent
import androidx.annotation.OptIn
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.focusable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.HttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.PlayerView
import com.wittywilla.wittymediaclient.net.Item
import com.wittywilla.wittymediaclient.net.Playback
import com.wittywilla.wittymediaclient.net.PlaybackRequest
import com.wittywilla.wittymediaclient.net.Track
import com.wittywilla.wittymediaclient.ui.ActionButton
import com.wittywilla.wittymediaclient.ui.ErrorText
import com.wittywilla.wittymediaclient.ui.Nav
import com.wittywilla.wittymediaclient.ui.Route
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.util.Locale

private const val SEEK_SEC = 10.0
private const val PROGRESS_EVERY_SEC = 10
private const val PROGRESS_NOT_BEFORE_SEC = 5.0
private const val CONTROLS_HIDE_MS = 5_000L
private const val NEXT_COUNTDOWN_SEC = 8
private const val MAX_STREAM_RETRIES = 2
private const val PROFILE = "androidtv"

private fun clock(sec: Double): String {
    val total = sec.toLong().coerceAtLeast(0)
    val h = total / 3600
    val m = (total % 3600) / 60
    val s = total % 60
    return if (h > 0) String.format(Locale.ROOT, "%d:%02d:%02d", h, m, s) else String.format(Locale.ROOT, "%d:%02d", m, s)
}

private fun trackName(t: Track): String {
    val language = t.languageName.ifEmpty { if (t.language.isEmpty()) "Unknown" else Locale.forLanguageTag(t.language).displayLanguage.ifEmpty { t.language } }
    return listOf(language, t.title, t.codec.uppercase(Locale.ROOT), if (t.channels > 0) "${t.channels}ch" else "")
        .filter { it.isNotEmpty() }.joinToString(" · ")
}

private enum class Picker { None, Audio, Subtitles }

// What to ask the server for. A new value of this opens or changes the stream.
private data class Ask(val audio: Int? = null, val subtitle: Int? = null, val startSec: Double? = null, val attempt: Int = 0)

@OptIn(UnstableApi::class)
@Composable
fun PlayerScreen(nav: Nav, id: Long, startOver: Boolean) {
    val context = LocalContext.current
    val api = nav.app.api

    var item by remember { mutableStateOf<Item?>(null) }
    var stream by remember { mutableStateOf<Playback?>(null) }
    var ask by remember { mutableStateOf(Ask(startSec = if (startOver) 0.0 else null)) }
    var error by remember { mutableStateOf("") }
    var controls by remember { mutableStateOf(true) }
    var picker by remember { mutableStateOf(Picker.None) }
    var playing by remember { mutableStateOf(false) }
    var filmSec by remember { mutableStateOf(0.0) }
    var interactions by remember { mutableStateOf(0) }
    var countdown by remember { mutableStateOf<Int?>(null) }
    var retries by remember { mutableStateOf(0) }
    val rootFocus = remember { FocusRequester() }

    // The address from the server already carries a short-lived stream token,
    // which is safer to hand a player than the sign-in.
    val player = remember { ExoPlayer.Builder(context).build() }

    fun currentFilmSec() = player.currentPosition / 1000.0 + (stream?.timeOffsetSec ?: 0.0)

    fun saveProgress() {
        val duration = stream?.durationSec ?: return
        val at = currentFilmSec()
        if (at < PROGRESS_NOT_BEFORE_SEC) return
        // In the app's scope: the last save happens as this screen is leaving.
        nav.app.scope.launch { runCatching { api.saveProgress(id, at, duration) } }
    }

    DisposableEffect(player) {
        val listener = object : Player.Listener {
            override fun onIsPlayingChanged(isPlaying: Boolean) { playing = isPlaying }
            override fun onPlayerError(e: PlaybackException) {
                // 401 means a new token and 404 means the stream went away: ask again.
                val code = (e.cause as? HttpDataSource.InvalidResponseCodeException)?.responseCode
                if ((code == 401 || code == 404) && retries < MAX_STREAM_RETRIES) {
                    retries++
                    ask = ask.copy(startSec = currentFilmSec(), attempt = ask.attempt + 1)
                } else {
                    error = "Can't play this: ${e.errorCodeName}"
                }
            }
            override fun onPlaybackStateChanged(state: Int) {
                if (state == Player.STATE_READY) retries = 0
                if (state == Player.STATE_ENDED) {
                    if (stream?.nextId != null) countdown = NEXT_COUNTDOWN_SEC else nav.pop()
                }
            }
        }
        player.addListener(listener)
        onDispose {
            player.removeListener(listener)
            player.release()
        }
    }

    // Ask the server what to play, and switch only if the answer is another address.
    LaunchedEffect(ask) {
        try {
            if (item == null) item = api.item(id)
            val answer = api.playback(id, PlaybackRequest(PROFILE, ask.audio, ask.subtitle, ask.startSec))
            val changed = answer.url != stream?.url
            stream = answer
            if (changed) {
                val media = MediaItem.Builder().setUri(api.mediaUrl(answer.url))
                if (answer.protocol == "hls") media.setMimeType(MimeTypes.APPLICATION_M3U8)
                val chosen = answer.subtitles.firstOrNull { it.index == answer.subtitleIndex }
                if (chosen != null && chosen.delivery == "sidecar" && chosen.url.isNotEmpty()) {
                    media.setSubtitleConfigurations(
                        listOf(
                            MediaItem.SubtitleConfiguration.Builder(android.net.Uri.parse(api.mediaUrl(chosen.url)))
                                .setMimeType(MimeTypes.TEXT_VTT)
                                .setLanguage(chosen.language)
                                .setSelectionFlags(C.SELECTION_FLAG_DEFAULT)
                                .build(),
                        ),
                    )
                }
                player.setMediaItem(media.build(), (answer.startAtSec * 1000).toLong())
                player.prepare()
                player.playWhenReady = true
            }
            // Tracks the player can switch itself: embedded, renditions and sidecars.
            val audio = answer.audio.firstOrNull { it.index == answer.audioIndex }
            val text = answer.subtitles.firstOrNull { it.index == answer.subtitleIndex }
            player.trackSelectionParameters = player.trackSelectionParameters.buildUpon()
                .setPreferredAudioLanguage(audio?.language?.ifEmpty { null })
                .setPreferredTextLanguage(text?.language?.ifEmpty { null })
                .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, text == null || answer.subtitleDelivery == "burn")
                .build()
        } catch (e: Exception) {
            if (!nav.handleExpired(e)) error = e.message.orEmpty()
        }
    }

    LaunchedEffect(player) {
        var sinceSave = 0
        while (true) {
            delay(1000)
            filmSec = currentFilmSec()
            if (++sinceSave >= PROGRESS_EVERY_SEC && player.isPlaying) {
                sinceSave = 0
                saveProgress()
            }
        }
    }

    // Leaving: save the place and stop the conversion straight away. This runs
    // before the player's own effect above, so the player can still be read.
    DisposableEffect(Unit) {
        onDispose {
            saveProgress()
            val streamId = stream?.streamId.orEmpty()
            if (streamId.isNotEmpty()) nav.app.scope.launch { runCatching { api.stopStream(streamId) } }
        }
    }

    LaunchedEffect(countdown) {
        val left = countdown ?: return@LaunchedEffect
        val next = stream?.nextId ?: return@LaunchedEffect
        if (left <= 0) {
            nav.replace(Route.Player(next, false))
        } else {
            delay(1000)
            countdown = left - 1
        }
    }

    LaunchedEffect(controls, interactions, playing) {
        if (controls && playing && picker == Picker.None) {
            delay(CONTROLS_HIDE_MS)
            controls = false
        }
    }
    LaunchedEffect(controls) { if (!controls) runCatching { rootFocus.requestFocus() } }

    // How far can be reached depends on the stream: a growing one has to be
    // reopened to go past what exists so far (see `seek` in docs/CLIENT-API.md).
    fun seekFilm(targetSec: Double) {
        val current = stream ?: return
        val target = targetSec.coerceIn(0.0, current.durationSec.takeIf { it > 0 } ?: Double.MAX_VALUE)
        val inPlayer = target - current.timeOffsetSec
        val known = player.duration.takeIf { it != C.TIME_UNSET }?.div(1000.0)
        val reachable = current.seek != "restart" || (inPlayer >= 0 && known != null && inPlayer <= known)
        if (reachable) {
            player.seekTo((inPlayer * 1000).toLong())
        } else {
            ask = ask.copy(startSec = target, attempt = ask.attempt + 1)
        }
        filmSec = target
    }
    fun playPause() { if (player.isPlaying) player.pause() else player.play() }

    // Buttons from shared/data/input-map.json. With the controls hidden the arrows
    // seek or show the controls. With them showing, focus moves among the buttons.
    fun onKey(code: Int): Boolean {
        interactions++
        when (code) {
            KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE, KeyEvent.KEYCODE_BUTTON_X, KeyEvent.KEYCODE_SPACE -> playPause()
            KeyEvent.KEYCODE_MEDIA_PLAY -> player.play()
            KeyEvent.KEYCODE_MEDIA_PAUSE -> player.pause()
            KeyEvent.KEYCODE_MEDIA_REWIND, KeyEvent.KEYCODE_BUTTON_L1 -> seekFilm(currentFilmSec() - SEEK_SEC)
            KeyEvent.KEYCODE_MEDIA_FAST_FORWARD, KeyEvent.KEYCODE_BUTTON_R1 -> seekFilm(currentFilmSec() + SEEK_SEC)
            KeyEvent.KEYCODE_BACK, KeyEvent.KEYCODE_BUTTON_B, KeyEvent.KEYCODE_ESCAPE -> when {
                countdown != null -> countdown = null
                picker != Picker.None -> picker = Picker.None
                controls && playing -> controls = false
                else -> nav.pop()
            }
            KeyEvent.KEYCODE_DPAD_LEFT -> if (controls) return false else seekFilm(currentFilmSec() - SEEK_SEC)
            KeyEvent.KEYCODE_DPAD_RIGHT -> if (controls) return false else seekFilm(currentFilmSec() + SEEK_SEC)
            KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_DPAD_DOWN -> if (controls) return false else controls = true
            KeyEvent.KEYCODE_DPAD_CENTER, KeyEvent.KEYCODE_ENTER, KeyEvent.KEYCODE_BUTTON_A -> if (controls) return false else controls = true
            else -> return false
        }
        return true
    }

    Box(
        Modifier
            .fillMaxSize()
            .background(Color.Black)
            // Before focusable: key handlers only hear the focus target and what wraps it.
            .onPreviewKeyEvent { if (it.type == KeyEventType.KeyDown) onKey(it.key.nativeKeyCode) else false }
            .focusRequester(rootFocus)
            .focusable()
            .clickable(interactionSource = remember { MutableInteractionSource() }, indication = null) {
                interactions++
                controls = !controls
            },
    ) {
        AndroidView(
            factory = { PlayerView(it).apply { useController = false; this.player = player } },
            modifier = Modifier.fillMaxSize(),
        )
        val left = countdown
        if (error.isNotEmpty()) {
            Column(Modifier.align(Alignment.Center).padding(32.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                ErrorText(error)
                ActionButton("Back", nav::pop, requestFocus = true)
            }
        } else if (left != null) {
            Column(Modifier.align(Alignment.Center).padding(32.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text("Next episode in $left", color = Color.White)
                ActionButton("Cancel", { countdown = null; nav.pop() }, requestFocus = true)
            }
        } else if (controls) {
            val intro = stream?.intro
            Controls(
                title = item?.title.orEmpty(),
                playing = playing,
                filmSec = filmSec,
                durationSec = stream?.durationSec ?: 0.0,
                introEndSec = intro?.takeIf { filmSec >= it.startSec && filmSec < it.endSec - 1 }?.endSec,
                hasAudioChoice = (stream?.audio?.size ?: 0) > 1,
                hasSubtitles = (stream?.subtitles?.size ?: 0) > 0,
                onPlayPause = { interactions++; playPause() },
                onSeekBack = { interactions++; seekFilm(currentFilmSec() - SEEK_SEC) },
                onSeekForward = { interactions++; seekFilm(currentFilmSec() + SEEK_SEC) },
                onSeekTo = { interactions++; seekFilm(it) },
                onSkipIntro = { interactions++; seekFilm(it) },
                onAudio = { picker = Picker.Audio },
                onSubtitles = { picker = Picker.Subtitles },
                modifier = Modifier.align(Alignment.BottomCenter),
            )
        }
        val current = stream
        if (picker != Picker.None && current != null) {
            TrackList(
                tracks = if (picker == Picker.Audio) current.audio else current.subtitles,
                selected = if (picker == Picker.Audio) current.audioIndex else current.subtitleIndex,
                allowOff = picker == Picker.Subtitles,
                onPick = { index ->
                    val at = currentFilmSec()
                    ask = if (picker == Picker.Audio) {
                        ask.copy(audio = index, startSec = at, attempt = ask.attempt + 1)
                    } else {
                        ask.copy(subtitle = index, startSec = at, attempt = ask.attempt + 1)
                    }
                    picker = Picker.None
                },
                modifier = Modifier.align(Alignment.Center),
            )
        }
    }
}

@Composable
private fun Controls(
    title: String,
    playing: Boolean,
    filmSec: Double,
    durationSec: Double,
    introEndSec: Double?,
    hasAudioChoice: Boolean,
    hasSubtitles: Boolean,
    onPlayPause: () -> Unit,
    onSeekBack: () -> Unit,
    onSeekForward: () -> Unit,
    onSeekTo: (Double) -> Unit,
    onSkipIntro: (Double) -> Unit,
    onAudio: () -> Unit,
    onSubtitles: () -> Unit,
    modifier: Modifier = Modifier,
) {
    // The bar follows a finger or the remote, and only seeks once it is let go:
    // on a growing stream each seek can mean opening a new one.
    var dragging by remember { mutableStateOf<Float?>(null) }
    Column(
        modifier.fillMaxWidth().background(Color(0xCC000000)).padding(20.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Text(title, color = Color.White)
        if (introEndSec != null) ActionButton("Skip intro", { onSkipIntro(introEndSec) }, primary = true)
        if (durationSec > 0) {
            Slider(
                value = dragging ?: filmSec.coerceIn(0.0, durationSec).toFloat(),
                onValueChange = { dragging = it },
                onValueChangeFinished = {
                    dragging?.let { onSeekTo(it.toDouble()) }
                    dragging = null
                },
                valueRange = 0f..durationSec.toFloat(),
                modifier = Modifier.fillMaxWidth(),
            )
        }
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
            ActionButton(if (playing) "Pause" else "Play", onPlayPause, primary = true, requestFocus = true)
            ActionButton("Back 10 s", onSeekBack)
            ActionButton("Forward 10 s", onSeekForward)
            if (hasAudioChoice) ActionButton("Audio", onAudio)
            if (hasSubtitles) ActionButton("Subtitles", onSubtitles)
            Text("${clock(filmSec)} / ${clock(durationSec)}", color = Color.White)
        }
    }
}

@Composable
private fun TrackList(
    tracks: List<Track>,
    selected: Int,
    allowOff: Boolean,
    onPick: (Int) -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier
            .widthIn(max = 520.dp)
            .background(Color(0xEE101114))
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        if (allowOff) ActionButton("Off", { onPick(-1) }, Modifier.fillMaxWidth(), primary = selected == -1, requestFocus = selected == -1)
        tracks.forEach { t ->
            ActionButton(trackName(t), { onPick(t.index) }, Modifier.fillMaxWidth(), primary = t.index == selected, requestFocus = t.index == selected)
        }
    }
}
