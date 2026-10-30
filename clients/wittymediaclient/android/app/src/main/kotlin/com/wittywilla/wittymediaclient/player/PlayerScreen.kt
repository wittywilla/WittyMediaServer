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
import androidx.media3.datasource.DefaultDataSource
import androidx.media3.datasource.DefaultHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.ui.PlayerView
import com.wittywilla.wittymediaclient.net.Item
import com.wittywilla.wittymediaclient.net.PlayInfo
import com.wittywilla.wittymediaclient.net.Track
import com.wittywilla.wittymediaclient.ui.ActionButton
import com.wittywilla.wittymediaclient.ui.ErrorText
import com.wittywilla.wittymediaclient.ui.Nav
import com.wittywilla.wittymediaclient.ui.RESUME_MIN_SEC
import com.wittywilla.wittymediaclient.ui.Route
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.util.Locale

private const val SEEK_MS = 10_000L
private const val PROGRESS_EVERY_MS = 10_000L
private const val CONTROLS_HIDE_MS = 5_000L

private fun clock(ms: Long): String {
    val sec = (ms / 1000).coerceAtLeast(0)
    val h = sec / 3600
    val m = (sec % 3600) / 60
    val s = sec % 60
    return if (h > 0) String.format(Locale.ROOT, "%d:%02d:%02d", h, m, s) else String.format(Locale.ROOT, "%d:%02d", m, s)
}

private fun trackName(t: Track): String {
    val language = if (t.language.isEmpty()) "Unknown" else Locale.forLanguageTag(t.language).displayLanguage.ifEmpty { t.language }
    return listOf(language, t.title, t.codec.uppercase(Locale.ROOT), if (t.channels > 0) "${t.channels}ch" else "")
        .filter { it.isNotEmpty() }.joinToString(" · ")
}

private enum class Picker { None, Audio, Subtitles }

@OptIn(UnstableApi::class)
@Composable
fun PlayerScreen(nav: Nav, id: Long, startOver: Boolean) {
    val context = LocalContext.current
    val api = nav.app.api

    var item by remember { mutableStateOf<Item?>(null) }
    var info by remember { mutableStateOf<PlayInfo?>(null) }
    var audio by remember { mutableStateOf(0) }
    var subtitle by remember { mutableStateOf(-1) }
    var error by remember { mutableStateOf("") }
    var controls by remember { mutableStateOf(true) }
    var picker by remember { mutableStateOf(Picker.None) }
    var playing by remember { mutableStateOf(false) }
    var position by remember { mutableStateOf(0L) }
    var interactions by remember { mutableStateOf(0) }
    val rootFocus = remember { FocusRequester() }

    // The sign-in travels as a header on every playlist and segment request, so
    // it never appears in a URL.
    val player = remember {
        val http = DefaultHttpDataSource.Factory()
            .setAllowCrossProtocolRedirects(false)
            .setConnectTimeoutMs(8_000)
            .setReadTimeoutMs(60_000) // a transcoded segment can take a while to appear
            .setDefaultRequestProperties(if (api.token.isEmpty()) emptyMap() else mapOf("Authorization" to "Bearer ${api.token}"))
        ExoPlayer.Builder(context)
            .setMediaSourceFactory(DefaultMediaSourceFactory(DefaultDataSource.Factory(context, http)))
            .build()
    }

    fun saveProgress() {
        val duration = info?.durationSec ?: return
        val at = player.currentPosition / 1000.0
        if (at <= 0) return
        // In the app's scope: the last save happens as this screen is leaving.
        nav.app.scope.launch { runCatching { api.saveProgress(id, at, duration) } }
    }

    DisposableEffect(player) {
        val listener = object : Player.Listener {
            override fun onIsPlayingChanged(isPlaying: Boolean) { playing = isPlaying }
            override fun onPlayerError(e: PlaybackException) { error = "Can't play this: ${e.errorCodeName}" }
            override fun onPlaybackStateChanged(state: Int) {
                if (state == Player.STATE_ENDED) {
                    val next = item?.nextId
                    if (next != null) nav.replace(Route.Player(next, false)) else nav.pop()
                }
            }
        }
        player.addListener(listener)
        onDispose {
            player.removeListener(listener)
            player.release()
        }
    }

    // Asking again with other tracks gives a new stream: carry on from the same place.
    LaunchedEffect(id, audio, subtitle) {
        try {
            val first = item == null
            val it = item ?: api.item(id).also { fetched -> item = fetched }
            val play = api.play(id, audio, subtitle)
            info = play
            val saved = it.progress
            val start = when {
                !first -> player.currentPosition
                startOver || saved == null || saved.watched || saved.positionSec <= RESUME_MIN_SEC -> 0L
                else -> (saved.positionSec * 1000).toLong()
            }
            val media = MediaItem.Builder().setUri(api.mediaUrl(play.url))
            if (play.type == "hls") media.setMimeType(MimeTypes.APPLICATION_M3U8)
            player.setMediaItem(media.build(), start)
            player.prepare()
            player.playWhenReady = true
        } catch (e: Exception) {
            if (!nav.handleExpired(e)) error = e.message.orEmpty()
        }
    }

    LaunchedEffect(player) {
        var sinceSave = 0L
        while (true) {
            delay(1000)
            position = player.currentPosition
            sinceSave += 1000
            if (sinceSave >= PROGRESS_EVERY_MS && player.isPlaying) {
                sinceSave = 0
                saveProgress()
            }
        }
    }
    DisposableEffect(Unit) { onDispose { saveProgress() } }

    LaunchedEffect(controls, interactions, playing) {
        if (controls && playing && picker == Picker.None) {
            delay(CONTROLS_HIDE_MS)
            controls = false
        }
    }
    LaunchedEffect(controls) { if (!controls) runCatching { rootFocus.requestFocus() } }

    fun seekBy(ms: Long) {
        val duration = player.duration.takeIf { it != C.TIME_UNSET } ?: Long.MAX_VALUE
        player.seekTo((player.currentPosition + ms).coerceIn(0, duration))
        position = player.currentPosition
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
            KeyEvent.KEYCODE_MEDIA_REWIND, KeyEvent.KEYCODE_BUTTON_L1 -> seekBy(-SEEK_MS)
            KeyEvent.KEYCODE_MEDIA_FAST_FORWARD, KeyEvent.KEYCODE_BUTTON_R1 -> seekBy(SEEK_MS)
            KeyEvent.KEYCODE_BACK, KeyEvent.KEYCODE_BUTTON_B, KeyEvent.KEYCODE_ESCAPE -> when {
                picker != Picker.None -> picker = Picker.None
                controls && playing -> controls = false
                else -> nav.pop()
            }
            KeyEvent.KEYCODE_DPAD_LEFT -> if (controls) return false else seekBy(-SEEK_MS)
            KeyEvent.KEYCODE_DPAD_RIGHT -> if (controls) return false else seekBy(SEEK_MS)
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
        if (error.isNotEmpty()) {
            Column(Modifier.align(Alignment.Center).padding(32.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                ErrorText(error)
                ActionButton("Back", nav::pop, requestFocus = true)
            }
        } else if (controls) {
            Controls(
                title = item?.title.orEmpty(),
                playing = playing,
                position = position,
                duration = ((info?.durationSec ?: 0.0) * 1000).toLong(),
                intro = item?.let { i ->
                    val end = i.introEnd
                    val start = i.introStart ?: 0.0
                    if (end != null && position / 1000.0 >= start && position / 1000.0 < end - 1) end else null
                },
                hasAudioChoice = (info?.audio?.size ?: 0) > 1,
                hasSubtitles = (info?.subtitles?.size ?: 0) > 0,
                onPlayPause = { interactions++; playPause() },
                onSeekBack = { interactions++; seekBy(-SEEK_MS) },
                onSeekForward = { interactions++; seekBy(SEEK_MS) },
                onSeekTo = { interactions++; player.seekTo(it) },
                onSkipIntro = { end -> interactions++; player.seekTo((end * 1000).toLong()) },
                onAudio = { picker = Picker.Audio },
                onSubtitles = { picker = Picker.Subtitles },
                modifier = Modifier.align(Alignment.BottomCenter),
            )
        }
        val current = info
        if (picker != Picker.None && current != null) {
            TrackList(
                tracks = if (picker == Picker.Audio) current.audio else current.subtitles,
                selected = if (picker == Picker.Audio) audio else subtitle,
                allowOff = picker == Picker.Subtitles,
                onPick = { index ->
                    if (picker == Picker.Audio) audio = index else subtitle = index
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
    position: Long,
    duration: Long,
    intro: Double?,
    hasAudioChoice: Boolean,
    hasSubtitles: Boolean,
    onPlayPause: () -> Unit,
    onSeekBack: () -> Unit,
    onSeekForward: () -> Unit,
    onSeekTo: (Long) -> Unit,
    onSkipIntro: (Double) -> Unit,
    onAudio: () -> Unit,
    onSubtitles: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier.fillMaxWidth().background(Color(0xCC000000)).padding(20.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Text(title, color = Color.White)
        if (intro != null) ActionButton("Skip intro", { onSkipIntro(intro) }, primary = true)
        if (duration > 0) {
            Slider(
                value = position.coerceIn(0, duration).toFloat(),
                onValueChange = { onSeekTo(it.toLong()) },
                valueRange = 0f..duration.toFloat(),
                modifier = Modifier.fillMaxWidth(),
            )
        }
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
            ActionButton(if (playing) "Pause" else "Play", onPlayPause, primary = true, requestFocus = true)
            ActionButton("Back 10 s", onSeekBack)
            ActionButton("Forward 10 s", onSeekForward)
            if (hasAudioChoice) ActionButton("Audio", onAudio)
            if (hasSubtitles) ActionButton("Subtitles", onSubtitles)
            Text("${clock(position)} / ${clock(duration)}", color = Color.White)
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
