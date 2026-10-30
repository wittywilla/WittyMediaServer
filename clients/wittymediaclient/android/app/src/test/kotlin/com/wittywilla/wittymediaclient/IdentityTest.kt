package com.wittywilla.wittymediaclient

import com.wittywilla.wittymediaclient.data.SavedServer
import com.wittywilla.wittymediaclient.data.rememberServer
import com.wittywilla.wittymediaclient.net.Identity
import com.wittywilla.wittymediaclient.net.IdentityAnswer
import com.wittywilla.wittymediaclient.net.IdentityState
import com.wittywilla.wittymediaclient.net.NextStep
import com.wittywilla.wittymediaclient.net.ServerProof
import com.wittywilla.wittymediaclient.net.json
import kotlinx.serialization.Serializable
import kotlinx.serialization.decodeFromString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import kotlin.io.encoding.Base64

class IdentityTest {
    @Serializable
    private data class Fixture(val nonce: String, val answer: IdentityAnswer)

    // A real answer from a real server.
    private val fixture = json.decodeFromString<Fixture>(File("../../shared/fixtures/identity.json").readText())
    private val nonce = fixture.nonce
    private val real = fixture.answer

    private fun flipFirstByte(text: String): String {
        val raw = Base64.decode(text)
        raw[0] = (raw[0].toInt() xor 1).toByte()
        return Base64.encode(raw)
    }

    @Test
    fun realSignedAnswerVerifies() {
        assertTrue(Identity.signatureValid(real, nonce))
    }

    @Test
    fun tamperedAnswersFail() {
        assertFalse("signature", Identity.signatureValid(real.copy(signature = flipFirstByte(real.signature)), nonce))
        assertFalse("key", Identity.signatureValid(real.copy(publicKey = flipFirstByte(real.publicKey)), nonce))
        assertFalse("server ID", Identity.signatureValid(real.copy(serverId = "00" + real.serverId.drop(2)), nonce))
        assertFalse("a replayed answer, to another nonce", Identity.signatureValid(real, flipFirstByte(nonce)))
    }

    @Test
    fun malformedAnswersFailWithoutThrowing() {
        assertFalse(Identity.signatureValid(real.copy(algorithm = "rsa"), nonce))
        assertFalse(Identity.signatureValid(real.copy(algorithm = ""), nonce))
        assertFalse(Identity.signatureValid(real.copy(serverId = ""), nonce))
        assertFalse(Identity.signatureValid(real.copy(publicKey = "not base64!"), nonce))
        assertFalse(Identity.signatureValid(real.copy(signature = "not base64!"), nonce))
        assertFalse(Identity.signatureValid(real.copy(publicKey = Base64.encode(ByteArray(31))), nonce))
        assertFalse(Identity.signatureValid(real.copy(signature = Base64.encode(ByteArray(63))), nonce))
        assertFalse(Identity.signatureValid(IdentityAnswer(), nonce))
    }

    @Test
    fun fingerprintIsWorkedOutFromTheKey() {
        assertEquals(real.fingerprint, Identity.fingerprint(real.publicKey))
        assertNotEquals(real.fingerprint, Identity.fingerprint(flipFirstByte(real.publicKey)))
    }

    @Test
    fun noncesAreFreshAndAcceptableToTheServer() {
        val a = Identity.newNonce()
        assertNotEquals(a, Identity.newNonce())
        assertEquals(32, Base64.decode(a).size) // the server accepts 16 to 64 bytes
    }

    private val url = "http://192.168.1.20:9797"
    private val mine = SavedServer(url, "Home", real.serverId, real.publicKey, real.fingerprint)
    private val otherKey = "ETNweKcsBs/GVCsvdOUSvznsUL93EKstZelyfa67ltB="

    @Test
    fun aNewServerIsNew() {
        assertEquals(IdentityState.New, Identity.judge(real, nonce, url, emptyList()))
        val elsewhere = SavedServer("http://10.0.0.1:9797", "Other", "ff".repeat(16), otherKey)
        assertEquals(IdentityState.New, Identity.judge(real, nonce, url, listOf(elsewhere)))
    }

    @Test
    fun aSavedAddressWithoutAKeyIsStillNew() {
        assertEquals(IdentityState.New, Identity.judge(real, nonce, url, listOf(SavedServer(url, "Home"))))
    }

    @Test
    fun theRememberedKeyIsTheSameWhereverItAnswers() {
        assertEquals(IdentityState.Same, Identity.judge(real, nonce, url, listOf(mine)))
        assertEquals(IdentityState.Same, Identity.judge(real, nonce, "http://192.168.1.99:9797", listOf(mine)))
    }

    @Test
    fun anotherKeyAtARememberedAddressOrIdIsDifferent() {
        val atSameAddress = mine.copy(serverId = "ff".repeat(16), publicKey = otherKey)
        assertEquals(IdentityState.Different, Identity.judge(real, nonce, url, listOf(atSameAddress)))
        val withSameId = mine.copy(url = "http://192.168.1.99:9797", publicKey = otherKey)
        assertEquals(IdentityState.Different, Identity.judge(real, nonce, url, listOf(withSameId)))
    }

    @Test
    fun aBadSignatureIsBadEvenWithTheRightKey() {
        val forged = real.copy(signature = flipFirstByte(real.signature))
        assertEquals(IdentityState.Bad, Identity.judge(forged, nonce, url, listOf(mine)))
    }

    @Test
    fun announcingAnIdProvesNothing() {
        // A look-alike can copy the ID, and sign with its own key.
        val saved = mine.copy(url = "http://10.0.0.5:9797", publicKey = otherKey)
        assertEquals(IdentityState.Different, Identity.judge(real, nonce, "http://10.0.0.9:9797", listOf(saved)))
    }

    @Test
    fun anOlderServerIsOnlyAllowedWithNoKeyRemembered() {
        assertEquals(IdentityState.None, Identity.unanswered(url, emptyList()))
        assertEquals(IdentityState.None, Identity.unanswered(url, listOf(SavedServer(url, "Home"))))
        assertEquals(IdentityState.Bad, Identity.unanswered(url, listOf(mine)))
    }

    @Test
    fun tokensOnlyGoWithTheKeyTheyWereKeptWith() {
        val same = ServerProof(IdentityState.Same, real.serverId, real.publicKey)
        assertTrue(Identity.fits(real.publicKey, same))
        assertFalse("another remembered server", Identity.fits(otherKey, same))
        assertFalse(Identity.fits(real.publicKey, ServerProof(IdentityState.Different, real.serverId, otherKey)))
        assertFalse(Identity.fits(real.publicKey, ServerProof(IdentityState.Bad)))
        assertFalse(Identity.fits(real.publicKey, ServerProof(IdentityState.Unreachable)))
        assertFalse("no answer can't stand in for a key", Identity.fits(real.publicKey, ServerProof(IdentityState.None)))
        assertTrue("nothing to compare with yet", Identity.fits("", ServerProof(IdentityState.New, real.serverId, real.publicKey)))
        assertFalse(Identity.fits("", ServerProof(IdentityState.Bad)))
    }

    @Test
    fun whatHappensAfterChoosingAServer() {
        assertEquals(NextStep.Warn, Identity.nextStep(IdentityState.Different, true))
        assertEquals(NextStep.Refuse, Identity.nextStep(IdentityState.Bad, true))
        assertEquals(NextStep.Lost, Identity.nextStep(IdentityState.Unreachable, true))
        assertEquals(NextStep.AskCode, Identity.nextStep(IdentityState.New, false))
        assertEquals(NextStep.SignIn, Identity.nextStep(IdentityState.None, false))
        assertEquals(NextStep.SignIn, Identity.nextStep(IdentityState.Same, false))
        assertEquals(NextStep.Open, Identity.nextStep(IdentityState.Same, true))
    }

    @Test
    fun whatHappensAtStart() {
        assertEquals(NextStep.Warn, Identity.stepAtStart(IdentityState.Different, true))
        assertEquals(NextStep.Refuse, Identity.stepAtStart(IdentityState.Bad, true))
        assertEquals(NextStep.Lost, Identity.stepAtStart(IdentityState.Unreachable, true))
        assertEquals(NextStep.Open, Identity.stepAtStart(IdentityState.Same, true))
        assertEquals(NextStep.SignIn, Identity.stepAtStart(IdentityState.Same, false))
        assertEquals(NextStep.SignIn, Identity.stepAtStart(IdentityState.New, false))
    }

    @Test
    fun rememberingKeepsOneEntryPerAddressAndPerServer() {
        val moved = mine.copy(url = "http://192.168.1.99:9797")
        assertEquals(listOf(moved), rememberServer(listOf(mine), moved))
        val atOldAddress = SavedServer(url, "New", "ff".repeat(16), otherKey)
        assertEquals(listOf(atOldAddress), rememberServer(listOf(mine), atOldAddress))
        val noId = SavedServer("http://10.0.0.1", "Old")
        assertEquals(listOf(noId, SavedServer("http://10.0.0.2", "Older")), rememberServer(listOf(SavedServer("http://10.0.0.2", "Older")), noId))
        val many = (1..20).map { SavedServer("http://10.0.0.$it", "$it", "$it") }
        assertEquals(8, many.fold(emptyList<SavedServer>(), ::rememberServer).size)
    }

    @Test
    fun serversSavedBeforePinsExistedStillLoad() {
        val old = json.decodeFromString<List<SavedServer>>("""[{"url":"http://10.0.0.1:9797","name":"Home"}]""")
        assertEquals(SavedServer("http://10.0.0.1:9797", "Home"), old.single())
    }
}
