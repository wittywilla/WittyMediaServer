package com.wittywilla.wittymediaclient

import com.wittywilla.wittymediaclient.data.SavedServer
import com.wittywilla.wittymediaclient.net.Api
import com.wittywilla.wittymediaclient.net.ApiError
import com.wittywilla.wittymediaclient.net.IdentityState
import com.wittywilla.wittymediaclient.net.Probe
import com.wittywilla.wittymediaclient.net.identify
import com.wittywilla.wittymediaclient.net.json
import com.wittywilla.wittymediaclient.net.probeServer
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.bouncycastle.math.ec.rfc8032.Ed25519
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.File
import java.net.InetAddress
import java.net.ServerSocket
import java.security.SecureRandom
import kotlin.concurrent.thread
import kotlin.io.encoding.Base64

// The connect steps over real sockets, against a small stand-in for a server that
// really signs. Nothing here needs a screen or a device.
class ConnectOverHttpTest {
    private class Asked(val method: String, val path: String, val headers: Map<String, String>, val body: String)
    private class Answer(val status: Int, val body: String, val headers: String = "")

    private class Stub(private val handle: (Asked) -> Answer) {
        private val socket = ServerSocket(0, 16, InetAddress.getByName("127.0.0.1"))
        val url = "http://127.0.0.1:${socket.localPort}"
        val asked = java.util.concurrent.CopyOnWriteArrayList<Asked>()

        init {
            thread(isDaemon = true) {
                while (!socket.isClosed) {
                    val client = runCatching { socket.accept() }.getOrNull() ?: break
                    client.use {
                        val input = it.getInputStream().bufferedReader()
                        val first = input.readLine()?.split(" ") ?: return@use
                        val headers = HashMap<String, String>()
                        while (true) {
                            val line = input.readLine()
                            if (line.isNullOrEmpty()) break
                            val at = line.indexOf(':')
                            headers[line.substring(0, at).lowercase()] = line.substring(at + 1).trim()
                        }
                        val body = CharArray(headers["content-length"]?.toInt() ?: 0)
                        var read = 0
                        while (read < body.size) read += input.read(body, read, body.size - read)
                        val request = Asked(first[0], first[1], headers, String(body))
                        asked.add(request)
                        val answer = handle(request)
                        val bytes = answer.body.toByteArray()
                        val head = "HTTP/1.1 ${answer.status} X\r\nContent-Type: application/json\r\nContent-Length: ${bytes.size}\r\nConnection: close\r\n${answer.headers}\r\n"
                        it.getOutputStream().apply { write(head.toByteArray()); write(bytes); flush() }
                    }
                }
            }
        }

        fun close() = socket.close()
    }

    private class Keys(val serverId: String) {
        private val secret = ByteArray(32).also { SecureRandom().nextBytes(it) }
        val publicKey: String = Base64.encode(ByteArray(32).also { Ed25519.generatePublicKey(secret, 0, it, 0) })

        fun answer(nonce: String): String {
            val message = "WittyMediaServer identity v1\n$serverId\n$nonce".toByteArray()
            val signature = ByteArray(64).also { Ed25519.sign(secret, 0, message, 0, message.size, it, 0) }
            return """{"serverId":"$serverId","publicKey":"$publicKey","fingerprint":"AB12-CD34-EF56","algorithm":"ed25519","signature":"${Base64.encode(signature)}"}"""
        }
    }

    private val stubs = mutableListOf<Stub>()

    @After
    fun closeStubs() = stubs.forEach { it.close() }

    private fun stub(handle: (Asked) -> Answer) = Stub(handle).also { stubs.add(it) }

    private fun identityServer(keys: Keys) = stub { asked ->
        val nonce = json.parseToJsonElement(asked.body).jsonObject["nonce"]!!.jsonPrimitive.content
        Answer(200, keys.answer(nonce))
    }

    private val keys = Keys("0123456789abcdef0123456789abcdef")

    @Test
    fun firstConnectionProvesTheKey() = runBlocking {
        val server = identityServer(keys)
        val proof = identify(server.url, emptyList())
        assertEquals(IdentityState.New, proof.state)
        assertEquals(keys.publicKey, proof.publicKey)
        assertEquals(keys.serverId, proof.serverId)
    }

    @Test
    fun theFingerprintIsNotTakenFromTheAnswer() = runBlocking {
        val proof = identify(identityServer(keys).url, emptyList())
        assertNotEquals("AB12-CD34-EF56", proof.fingerprint)
    }

    @Test
    fun aFreshNonceEveryTimeAndNoTokenWithIt() = runBlocking {
        val server = identityServer(keys)
        identify(server.url, emptyList())
        identify(server.url, emptyList())
        assertEquals(2, server.asked.size)
        assertEquals("/api/v1/identity", server.asked[0].path)
        assertEquals("POST", server.asked[0].method)
        assertNotEquals(server.asked[0].body, server.asked[1].body)
        assertTrue(server.asked.none { "authorization" in it.headers })
    }

    @Test
    fun thePinnedKeyMustMatch() = runBlocking {
        val server = identityServer(keys)
        val saved = SavedServer(server.url, "Home", keys.serverId, keys.publicKey)
        assertEquals(IdentityState.Same, identify(server.url, listOf(saved)).state)
        val lookAlike = identityServer(Keys(keys.serverId))
        assertEquals(IdentityState.Different, identify(lookAlike.url, listOf(saved.copy(url = lookAlike.url))).state)
    }

    @Test
    fun aServerThatRepliesWithAnOldNonceIsBad() = runBlocking {
        val replay = stub { Answer(200, keys.answer("b2xkIG5vbmNlIHNlZW4gZWFybGllcg==")) }
        assertEquals(IdentityState.Bad, identify(replay.url, emptyList()).state)
    }

    @Test
    fun nonsenseIsBad() = runBlocking {
        assertEquals(IdentityState.Bad, identify(stub { Answer(200, "<html>hello</html>") }.url, emptyList()).state)
        assertEquals(IdentityState.Bad, identify(stub { Answer(500, """{"error":"broken"}""") }.url, emptyList()).state)
    }

    @Test
    fun aFourOhFourOnlyPassesWithNoKeyRemembered() = runBlocking {
        val old = stub { Answer(404, """{"error":"not found"}""") }
        assertEquals(IdentityState.None, identify(old.url, emptyList()).state)
        val pinned = SavedServer(old.url, "Home", keys.serverId, keys.publicKey)
        assertEquals(IdentityState.Bad, identify(old.url, listOf(pinned)).state)
    }

    @Test
    fun nothingListeningIsUnreachable() = runBlocking {
        val closed = stub { Answer(200, "") }
        closed.close()
        assertEquals(IdentityState.Unreachable, identify(closed.url, emptyList()).state)
    }

    @Test
    fun redirectsAreNotFollowed() = runBlocking {
        val elsewhere = identityServer(keys)
        val redirector = stub { Answer(302, "{}", "Location: ${elsewhere.url}/api/v1/identity\r\n") }
        assertEquals(IdentityState.Bad, identify(redirector.url, emptyList()).state)
        assertEquals(0, elsewhere.asked.size)
        assertEquals(Probe.Failed("not-server"), probeServer(redirector.url))
    }

    private fun fixture(name: String) = File("../../shared/fixtures/$name.json").readText()

    private fun capabilitiesServer(capabilities: String) = stub { asked ->
        when (asked.path) {
            "/api/v1/capabilities" -> Answer(200, capabilities)
            "/api/v1/status" -> Answer(200, fixture("status"))
            else -> Answer(404, "{}")
        }
    }

    @Test
    fun probeFollowsTheGuide() = runBlocking {
        assertTrue(probeServer(capabilitiesServer(fixture("capabilities")).url) is Probe.Found)
        assertEquals(Probe.Failed("not-server"), probeServer(capabilitiesServer("""{"server":"Something else"}""").url))
        assertEquals(Probe.Failed("app-old"), probeServer(capabilitiesServer("""{"server":"WittyMediaServer","apiVersions":[2]}""").url))
        assertEquals(Probe.Failed("not-set-up"), probeServer(capabilitiesServer("""{"server":"WittyMediaServer","apiVersions":[1],"setupRequired":true}""").url))
        assertEquals(Probe.Failed("not-server"), probeServer(stub { Answer(200, "<html></html>") }.url))
        assertEquals(Probe.Failed("not-server"), probeServer(stub { Answer(404, "{}") }.url))
        assertEquals(Probe.Failed("server-old"), probeServer(stub { asked ->
            if (asked.path == "/api/v1/status") Answer(200, fixture("status")) else Answer(404, "{}")
        }.url))
        val closed = stub { Answer(200, "") }
        closed.close()
        assertEquals(Probe.Failed("unreachable"), probeServer(closed.url))
    }

    @Test
    fun redeemSendsTheCodeAndGetsATokenWithoutOneBeingSent() = runBlocking {
        val server = stub { Answer(200, """{"status":"approved","token":"abc","user":{"id":1,"name":"Willow"}}""") }
        val result = Api(server.url, "", quick = true).pairRedeem("123456", "Living room TV", "Android TV")
        assertEquals("abc", result.token)
        assertEquals("/api/v1/pair/redeem", server.asked.single().path)
        val sent = json.parseToJsonElement(server.asked.single().body).jsonObject
        assertEquals("123456", sent["code"]?.jsonPrimitive?.content)
        assertEquals("Living room TV", sent["device"]?.jsonPrimitive?.content)
        assertEquals("Android TV", sent["platform"]?.jsonPrimitive?.content)
        assertFalse("authorization" in server.asked.single().headers)
    }

    @Test
    fun redeemFailuresCarryTheServersOwnWords() = runBlocking {
        val cases = listOf(
            400 to """{"error":"That isn't a code that works.","reason":"wrong-code"}""",
            410 to """{"error":"That code has run out.","reason":"expired"}""",
            429 to """{"error":"Too many wrong codes. Wait a little.","reason":"too-many-tries","retryAfterSec":30}""",
        )
        for ((status, body) in cases) {
            val server = stub { Answer(status, body) }
            try {
                Api(server.url, "", quick = true).pairRedeem("000000", "TV", "Android TV")
                fail("status $status should fail")
            } catch (e: ApiError) {
                assertEquals(status, e.status)
                assertEquals(json.parseToJsonElement(body).jsonObject["error"]?.jsonPrimitive?.content, e.message)
            }
        }
    }

    @Test
    fun anUntrustedServerCanNotSendAHugeAnswer() = runBlocking {
        val huge = stub { Answer(200, "x".repeat(2_000_000)) }
        assertEquals(IdentityState.Bad, identify(huge.url, emptyList()).state)
    }
}
