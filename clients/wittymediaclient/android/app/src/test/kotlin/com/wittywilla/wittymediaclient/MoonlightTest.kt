package com.wittywilla.wittymediaclient

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
    private val bridge = RigMethod(kind = "bridge", url = "https://home.example:47900/wms-ticket/x", note = "Keyboard only.")
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
    fun theIntentComesFirst() {
        val choice = rigChoice(play(bridge, intent, steps), isTv = false)
        assertEquals("Pair first.", (choice as RigChoice.Open).note)
    }

    @Test
    fun aBrowserIsOnlyOfferedOffATv() {
        assertEquals(RigChoice.Browser(bridge.url, bridge.note), rigChoice(play(bridge, steps), isTv = false))
        assertEquals(RigChoice.Steps(steps.steps), rigChoice(play(bridge, steps), isTv = true))
    }

    @Test
    fun onlyWebLinksGoToABrowser() {
        assertEquals(RigChoice.Steps(steps.steps), rigChoice(play(bridge.copy(url = "intent://x#Intent;end"), steps), isTv = false))
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
