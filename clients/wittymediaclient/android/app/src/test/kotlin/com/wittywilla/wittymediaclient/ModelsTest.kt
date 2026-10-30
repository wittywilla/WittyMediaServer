package com.wittywilla.wittymediaclient

import com.wittywilla.wittymediaclient.net.Capabilities
import com.wittywilla.wittymediaclient.net.Home
import com.wittywilla.wittymediaclient.net.PairStart
import com.wittywilla.wittymediaclient.net.Playback
import com.wittywilla.wittymediaclient.net.PlaybackRequest
import com.wittywilla.wittymediaclient.net.Profiles
import com.wittywilla.wittymediaclient.net.ServerStatus
import com.wittywilla.wittymediaclient.net.ShowDetail
import com.wittywilla.wittymediaclient.net.json
import kotlinx.serialization.encodeToString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

// Sample responses, kept true to real servers by shared/scripts/check-fixtures.mjs.
class ModelsTest {
    private fun fixture(name: String) = File("../../shared/fixtures/$name.json").readText()

    @Test
    fun status() {
        val status = json.decodeFromString<ServerStatus>(fixture("status"))
        assertEquals("WittyMediaServer", status.name)
        assertNull(status.user)
    }

    @Test
    fun capabilities() {
        val caps = json.decodeFromString<Capabilities>(fixture("capabilities"))
        assertEquals("WittyMediaServer", caps.server)
        assertTrue("pairing" in caps.features)
    }

    @Test
    fun profiles() {
        val profiles = json.decodeFromString<Profiles>(fixture("profiles"))
        assertEquals("profiles", profiles.style)
        assertEquals(false, profiles.profiles.last().hasPassword)
    }

    @Test
    fun homeAcceptsNullLists() {
        val home = json.decodeFromString<Home>(fixture("home"))
        assertEquals(912.25, home.hubs.first().items.first().progress?.positionSec ?: 0.0, 0.001)
        assertTrue(home.hubs.last().items.isEmpty())
        assertEquals(1, home.hubs.last().shows.size)
        assertEquals("shows", home.libraries.last().type)
    }

    @Test
    fun show() {
        val detail = json.decodeFromString<ShowDetail>(fixture("show"))
        assertEquals(22L, detail.nextUpId)
        assertEquals(3, detail.episodes[1].episodeEnd)
        assertEquals(65.5, detail.episodes[1].introEnd ?: 0.0, 0.001)
    }

    @Test
    fun playback() {
        val play = json.decodeFromString<Playback>(fixture("playback"))
        assertEquals("hls", play.protocol)
        assertEquals("restart", play.seek)
        assertEquals(600.0, play.timeOffsetSec, 0.001)
        assertEquals(2, play.audio.size)
        assertEquals("hls", play.subtitles.first().delivery)
        assertEquals(95.0, play.intro?.endSec ?: 0.0, 0.001)
        assertEquals(43L, play.nextId)
    }

    @Test
    fun playbackRequestLeavesOutWhatTheServerDecides() {
        assertEquals("""{"profile":"androidtv"}""", json.encodeToString(PlaybackRequest("androidtv")))
    }

    @Test
    fun pairStart() {
        val start = json.decodeFromString<PairStart>(fixture("pair-start"))
        assertEquals("BCDF-GHJK", start.code)
        assertEquals(3, start.intervalSec)
    }
}
