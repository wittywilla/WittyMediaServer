package com.wittywilla.wittymediaclient.net

import com.wittywilla.wittymediaclient.data.SavedServer
import com.wittywilla.wittymediaclient.diag.ClientLog
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerializationException
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.IOException
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLException

class ApiError(val status: Int, message: String) : Exception(message)

val json = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true // a Go nil list arrives as null
    explicitNulls = false
}

private val JSON_TYPE = "application/json".toMediaType()
private const val API = "/api/v1" // the contract: docs/CLIENT-API.md
private const val QUICK_SECONDS = 5L
private const val QUICK_MAX_BYTES = 64L * 1024
private val shared = OkHttpClient() // connections and threads are reused across servers

private data class Answer(val status: Int, val text: String, val requestId: String)

// One connection to one server. The token only ever goes to that server, in a
// header, so it stays out of URLs and logs. Redirects are never followed. Quick is
// for questions to an address not yet trusted: short waits and small answers.
class Api(val base: String, val token: String, private val quick: Boolean = false) {
    val http: OkHttpClient = shared.newBuilder()
        .connectTimeout(8, TimeUnit.SECONDS)
        .readTimeout(60, TimeUnit.SECONDS) // the first playback request probes the file
        .callTimeout(if (quick) QUICK_SECONDS else 0, TimeUnit.SECONDS)
        .followRedirects(false)
        .followSslRedirects(false)
        .addInterceptor(Interceptor { chain ->
            val req = chain.request()
            val server = Request.Builder().url(base).build().url
            val same = req.url.host == server.host && req.url.port == server.port && req.url.scheme == server.scheme
            chain.proceed(if (same && token.isNotEmpty()) req.newBuilder().header("Authorization", "Bearer $token").build() else req)
        })
        .build()

    private fun errorText(status: Int, text: String) =
        runCatching { json.parseToJsonElement(text).jsonObject["error"]?.jsonPrimitive?.content }.getOrNull()
            ?: "The server answered with an error ($status)."

    private suspend fun raw(method: String, path: String, body: String?): Answer = withContext(Dispatchers.IO) {
        val request = Request.Builder()
            .url("$base$API$path")
            .method(method, body?.toRequestBody(JSON_TYPE))
            .build()
        http.newCall(request).execute().use { res ->
            Answer(res.code, if (quick) res.peekBody(QUICK_MAX_BYTES).string() else res.body.string(), res.header("X-Request-Id").orEmpty())
        }
    }

    // Only for a signed-in connection: an address being tried has its own words on
    // screen. A sign-in that has run out and a stream already gone are normal.
    private fun reportsFailure(method: String, status: Int) =
        !quick && token.isNotEmpty() && status != 401 && !(method == "DELETE" && status == 404)

    private suspend fun call(method: String, path: String, body: String? = null): String {
        val where = "$method ${path.substringBefore('?')}"
        val answer = try {
            raw(method, path, body)
        } catch (e: IOException) {
            if (reportsFailure(method, 0)) ClientLog.warn("network", "$where failed: ${e.javaClass.simpleName}")
            throw e
        }
        if (answer.status !in 200..299) {
            val message = errorText(answer.status, answer.text)
            if (reportsFailure(method, answer.status)) {
                val words = "$where answered ${answer.status}: $message"
                if (answer.status >= 500) ClientLog.error("request", words, request = answer.requestId)
                else ClientLog.warn("request", words, request = answer.requestId)
            }
            throw ApiError(answer.status, message)
        }
        return answer.text
    }

    private suspend inline fun <reified T> get(path: String): T = json.decodeFromString(call("GET", path))

    suspend fun capabilities(): Capabilities = get("/capabilities")
    suspend fun status(): ServerStatus = get("/status")
    suspend fun profiles(): Profiles = get("/profiles")
    suspend fun home(): Home = get("/home")
    suspend fun item(id: Long): Item = get("/items/$id")
    suspend fun show(id: Long): ShowDetail = get("/shows/$id")
    suspend fun shows(library: Long): ShowPage = get("/shows?library=$library&sort=title")
    suspend fun items(library: Long, offset: Int, limit: Int): ItemPage =
        get("/items?library=$library&offset=$offset&limit=$limit&sort=title")

    suspend fun playback(id: Long, request: PlaybackRequest): Playback =
        json.decodeFromString(call("POST", "/items/$id/playback", json.encodeToString(request)))

    // Stops the conversion at once when the viewer leaves.
    suspend fun stopStream(streamId: String) {
        call("DELETE", "/hls/$streamId")
    }

    suspend fun login(name: String, password: String, device: String): LoginResult =
        json.decodeFromString(call("POST", "/login", json.encodeToString(Credentials(name, password, device))))

    suspend fun pairStart(device: String, platform: String): PairStart =
        json.decodeFromString(call("POST", "/pair/start", json.encodeToString(PairStartRequest(device, platform))))

    // Denied, expired and slow-down are normal answers here, not errors.
    suspend fun pairPoll(deviceSecret: String): PairPoll {
        val (status, text) = raw("POST", "/pair/poll", json.encodeToString(PairPollRequest(deviceSecret)))
        if (status in 200..299 || status == 403 || status == 410 || status == 429) return json.decodeFromString(text)
        throw ApiError(status, errorText(status, text))
    }

    // Errors carry the server's own plain words: a wrong, expired or used code, or too many tries.
    suspend fun pairRedeem(code: String, device: String, platform: String): LoginResult =
        json.decodeFromString(call("POST", "/pair/redeem", json.encodeToString(PairRedeemRequest(code, device, platform))))

    suspend fun identity(nonce: String): IdentityAnswer =
        json.decodeFromString(call("POST", "/identity", json.encodeToString(IdentityRequest(nonce))))

    suspend fun saveProgress(id: Long, positionSec: Double, durationSec: Double) {
        call("POST", "/items/$id/progress", json.encodeToString(ProgressReport(positionSec, durationSec)))
    }

    // What this device can do to play a game on someone's own PC. It starts nothing there.
    suspend fun rigPlay(gameId: Long, request: RigPlayRequest): RigPlay =
        json.decodeFromString(call("POST", "/rigs/games/$gameId/play", json.encodeToString(request)))

    suspend fun rigWake(rigId: Long): RigWake =
        json.decodeFromString(call("POST", "/rigs/$rigId/wake", "{}"))

    suspend fun couchStatus(): CouchStatus = get("/couch")

    suspend fun logout() {
        call("POST", "/logout", "{}")
    }

    // This app's own errors, for the server's log. The answer is all that comes back:
    // it skips `call`, so a report that fails can't become a report of its own.
    suspend fun sendClientLog(body: String): Int = raw("POST", "/logs/client", body).status

    fun posterUrl(it: Item) = "$base$API/items/${it.id}/poster?v=${it.updatedAt}"
    fun showPosterUrl(s: Show) = "$base$API/shows/${s.id}/art/poster?v=${s.updatedAt}"

    // Addresses in answers start with /api/: the server's address goes in front, nothing else changes.
    fun mediaUrl(path: String) = base + path
}

// Asks an address whether it is a WittyMediaServer this app can use.
sealed interface Probe {
    data class Found(val status: ServerStatus) : Probe
    data class Failed(val reason: String) : Probe
}

suspend fun probeServer(url: String): Probe {
    val api = Api(url, "", quick = true)
    return try {
        val caps = api.capabilities()
        when {
            caps.server != "WittyMediaServer" -> Probe.Failed("not-server")
            1 !in caps.apiVersions -> Probe.Failed("app-old")
            caps.setupRequired -> Probe.Failed("not-set-up")
            else -> Probe.Found(api.status())
        }
    } catch (_: SSLException) {
        Probe.Failed("certificate")
    } catch (_: IOException) {
        Probe.Failed("unreachable")
    } catch (e: ApiError) {
        // No /capabilities: an older server, if it says it is one at all.
        val old = e.status == 404 && runCatching { api.status().name == "WittyMediaServer" }.getOrDefault(false)
        Probe.Failed(if (old) "server-old" else "not-server")
    } catch (_: SerializationException) {
        Probe.Failed("not-server")
    }
}

// Has the server at this address proved it holds its key, and is that a key we
// remember? A new nonce every time, and no token goes with the question.
suspend fun identify(url: String, saved: List<SavedServer>): ServerProof {
    val nonce = Identity.newNonce()
    return try {
        val answer = Api(url, "", quick = true).identity(nonce)
        val state = withContext(Dispatchers.Default) { Identity.judge(answer, nonce, url, saved) }
        if (state == IdentityState.Bad) ServerProof(state)
        else ServerProof(state, answer.serverId, answer.publicKey, Identity.fingerprint(answer.publicKey))
    } catch (_: IOException) {
        ServerProof(IdentityState.Unreachable)
    } catch (e: ApiError) {
        ServerProof(if (e.status == 404) Identity.unanswered(url, saved) else IdentityState.Bad)
    } catch (_: SerializationException) {
        ServerProof(IdentityState.Bad)
    }
}
