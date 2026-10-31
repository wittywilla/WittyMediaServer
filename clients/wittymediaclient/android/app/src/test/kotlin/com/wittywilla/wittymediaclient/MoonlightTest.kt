package com.wittywilla.wittymediaclient

import com.wittywilla.wittymediaclient.net.GAME_PCS_PATH
import com.wittywilla.wittymediaclient.net.Item
import com.wittywilla.wittymediaclient.net.MoonlightStart
import com.wittywilla.wittymediaclient.net.RigChoice
import com.wittywilla.wittymediaclient.net.RigInstall
import com.wittywilla.wittymediaclient.net.RigMethod
import com.wittywilla.wittymediaclient.net.RigPlay
import com.wittywilla.wittymediaclient.net.RigPlayRequest
import com.wittywilla.wittymediaclient.net.isRigGame
import com.wittywilla.wittymediaclient.net.json
import com.wittywilla.wittymediaclient.net.moonlightStart
import com.wittywilla.wittymediaclient.net.moonlightStoreLinks
import com.wittywilla.wittymediaclient.net.rigChoice
import com.wittywilla.wittymediaclient.net.rigOtherChoice
import com.wittywilla.wittymediaclient.net.rigPlayerPath
import com.wittywilla.wittymediaclient.net.sameServer
import com.wittywilla.wittymediaclient.net.sessionCookie
import com.wittywilla.wittymediaclient.net.staysInWebPlayer
import com.wittywilla.wittymediaclient.net.rigOsName
import kotlinx.serialization.encodeToString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class MoonlightTest {
    private val extras = mapOf("UUID" to "ABC-123", "AppId" to "42", "AppName" to "Space Game")
    private val intent = RigMethod(kind = "intent", packageName = "com.limelight", activity = "com.limelight.ShortcutTrampoline", extras = extras, note = "Pair first.")
    private val client = RigMethod(kind = "client", label = "Play here", note = "Keyboard only.")
    private val steps = RigMethod(kind = "steps", steps = listOf("Open the Moonlight app."))

    @Test
    fun readsTheServersAnswer() {
        val answer = json.decodeFromString<RigPlay>(
            """{"rig":{"id":3,"libraryId":9,"name":"Den PC","paired":true,"state":"online","message":"Ready to play.","canWake":false,"canQuit":false,"browserPlay":false},
            "game":{"id":7,"title":"Space Game"},"away":false,"ready":true,
            "methods":[{"kind":"intent","label":"Open in Moonlight","oneClick":true,"package":"com.limelight","activity":"com.limelight.ShortcutTrampoline",
            "extras":{"UUID":"ABC-123","AppId":"42","AppName":"Space Game"},"note":"Pair first."},{"kind":"steps","label":"Open it yourself","oneClick":false,"steps":["Open the Moonlight app."]}],
            "install":{"name":"Moonlight for Android","url":"https://play.google.com/store/apps/details?id=com.limelight","license":"GPL-3.0","source":"https://example.com"},"setup":null}""",
        )
        assertEquals("Den PC", answer.rig.name)
        assertEquals(intent.packageName, answer.methods.first().packageName)
        assertEquals(extras, answer.methods.first().extras)
        assertEquals("Moonlight for Android", answer.install?.name)
        assertTrue(answer.setup.isEmpty())
    }

    @Test
    fun onlyAsksForWhatItIs() {
        assertEquals("""{"client":"android","os":"firetv"}""", json.encodeToString(RigPlayRequest("android", rigOsName(isTv = true, isFireTv = true))))
        assertEquals("androidtv", rigOsName(isTv = true, isFireTv = false))
        assertEquals("android", rigOsName(isTv = false, isFireTv = false))
    }

    @Test
    fun knowsARigGame() {
        assertTrue(Item(kind = "game", console = "rig").isRigGame)
        assertFalse(Item(kind = "game", console = "snes").isRigGame)
        assertFalse(Item(kind = "video").isRigGame)
    }

    @Test
    fun buildsMoonlightsIntent() {
        val start = moonlightStart(intent.copy(extras = extras + ("Name" to "Den PC")))
        assertEquals(MoonlightStart("com.limelight", "com.limelight.ShortcutTrampoline", extras + ("Name" to "Den PC")), start)
    }

    @Test
    fun aServerCantNameAnyOtherApp() {
        assertNull(moonlightStart(intent.copy(packageName = "com.example.bank")))
        assertNull(moonlightStart(intent.copy(activity = "com.limelight.Game")))
        assertNull(moonlightStart(intent.copy(kind = "steps")))
    }

    @Test
    fun anythingMissingOrExtraIsDealtWith() {
        assertNull(moonlightStart(intent.copy(extras = extras - "UUID")))
        assertNull(moonlightStart(intent.copy(extras = extras + ("AppId" to ""))))
        assertEquals(extras, moonlightStart(intent.copy(extras = extras + ("Other" to "x")))?.extras)
    }

    private fun play(vararg methods: RigMethod) = RigPlay(ready = true, methods = methods.toList())

    @Test
    fun phonesPlayInThePageWithMoonlightSecond() {
        val answer = play(client, intent, steps)
        assertEquals(RigChoice.InPage("Keyboard only."), rigChoice(answer, isTv = false))
        assertEquals("Pair first.", (rigOtherChoice(answer, isTv = false) as RigChoice.Open).note)
    }

    @Test
    fun tvsGoToMoonlightWithThePageSecond() {
        val answer = play(client, intent, steps)
        assertEquals("Pair first.", (rigChoice(answer, isTv = true) as RigChoice.Open).note)
        assertEquals(RigChoice.InPage("Keyboard only."), rigOtherChoice(answer, isTv = true))
    }

    @Test
    fun aTvWithNoMoonlightRouteStillHasThePage() {
        assertEquals(RigChoice.InPage("Keyboard only."), rigChoice(play(client), isTv = true))
        assertNull(rigOtherChoice(play(client), isTv = true))
    }

    @Test
    fun withoutThePageMoonlightOrStepsAreLeft() {
        assertEquals("Pair first.", (rigChoice(play(intent, steps), isTv = false) as RigChoice.Open).note)
        assertEquals(RigChoice.Steps(steps.steps), rigChoice(play(steps), isTv = false))
        assertNull(rigOtherChoice(play(steps), isTv = false))
    }

    @Test
    fun theWebPlayerKeepsToItsOwnPages() {
        assertEquals("/moonlight-rig/7", rigPlayerPath(7))
        for (path in listOf(GAME_PCS_PATH, "/moonlight/3", "/moonlight/3/play/9", "/moonlight-rig/7")) assertTrue(path, staysInWebPlayer(path))
        for (path in listOf("/", "/item/7", "/moonlights", "/settings", "/login")) assertFalse(path, staysInWebPlayer(path))
    }

    @Test
    fun onlyTheSameServerStaysInTheWebView() {
        val base = "http://192.168.1.5:9797"
        assertTrue(sameServer("http://192.168.1.5:9797/moonlight-rig/7", base))
        assertFalse(sameServer("http://192.168.1.5:9798/", base))
        assertFalse(sameServer("https://192.168.1.5:9797/", base))
        assertFalse(sameServer("http://192.168.1.5.example.com:9797/", base))
        assertFalse(sameServer("https://github.com/moonlight-stream", base))
        assertFalse(sameServer("intent://x#Intent;end", base))
        assertFalse(sameServer("file:///etc/passwd", base))
        assertTrue(sameServer("https://home.example/x", "https://home.example:443"))
    }

    @Test
    fun theSignInCookieIsTheServersOwnShape() {
        assertEquals("wms_token=abc123; Path=/; HttpOnly; SameSite=Lax", sessionCookie("abc123", https = false))
        assertEquals("wms_token=abc123; Path=/; HttpOnly; SameSite=Lax; Secure", sessionCookie("abc123", https = true))
        assertNull(sessionCookie("", https = false))
        assertNull(sessionCookie("abc; Domain=example.com", https = false))
        assertNull(sessionCookie("abc\r\nX: y", https = false))
    }

    @Test
    fun anIntentForSomethingElseIsNotFollowed() {
        assertEquals(RigChoice.Steps(steps.steps), rigChoice(play(intent.copy(packageName = "com.example.bank"), steps), isTv = false))
    }

    @Test
    fun nothingLeftIsSaidPlainly() {
        assertEquals(RigChoice.None, rigChoice(play(), isTv = false))
    }

    @Test
    fun theStoreFitsTheDevice() {
        val install = RigInstall(url = "https://play.google.com/store/apps/details?id=com.limelight")
        assertEquals(listOf("market://details?id=com.limelight", install.url), moonlightStoreLinks(false, install))
        assertEquals("amzn://apps/android?p=com.limelight", moonlightStoreLinks(true, install).first())
        assertEquals(listOf("market://details?id=com.limelight"), moonlightStoreLinks(false, RigInstall(url = "file:///etc/passwd")))
        assertEquals(listOf("market://details?id=com.limelight"), moonlightStoreLinks(false, null))
    }
}
