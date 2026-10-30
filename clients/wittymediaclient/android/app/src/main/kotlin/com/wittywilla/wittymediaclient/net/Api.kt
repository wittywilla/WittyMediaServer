package com.wittywilla.wittymediaclient.net

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
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

class ApiError(val status: Int, message: String) : Exception(message)

val json = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true // a Go nil list arrives as null
    explicitNulls = false
}

private val JSON_TYPE = "application/json".toMediaType()
private val API_PATH = Regex("^/api(/v[0-9]+)?$")

// One connection to one server. The token only ever goes to that server, in a
// header, so it stays out of URLs and logs. Redirects from https to http are refused.
class Api(val base: String, val token: String) {
    val http: OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(8, TimeUnit.SECONDS)
        .readTimeout(60, TimeUnit.SECONDS) // the first play request probes the file
        .followSslRedirects(false)
        .addInterceptor(Interceptor { chain ->
            val req = chain.request()
            val server = Request.Builder().url(base).build().url
            val same = req.url.host == server.host && req.url.port == server.port && req.url.scheme == server.scheme
            chain.proceed(if (same && token.isNotEmpty()) req.newBuilder().header("Authorization", "Bearer $token").build() else req)
        })
        .build()

    // Where this server's API lives. Newer servers name a versioned path; older
    // ones don't know the question and keep the plain one.
    @Volatile
    private var apiPath = "/api"

    suspend fun resolveApiPath() {
        val named = runCatching {
            json.parseToJsonElement(call("GET", "/capabilities")).jsonObject["apiBase"]?.jsonPrimitive?.content
        }.getOrNull()
        if (named != null && API_PATH.matches(named)) apiPath = named
    }

    private suspend fun call(method: String, path: String, body: String? = null): String = withContext(Dispatchers.IO) {
        val request = Request.Builder()
            .url("$base$apiPath$path")
            .method(method, body?.toRequestBody(JSON_TYPE))
            .build()
        http.newCall(request).execute().use { res ->
            val text = res.body.string()
            if (!res.isSuccessful) {
                val message = runCatching { json.parseToJsonElement(text).jsonObject["error"]?.jsonPrimitive?.content }.getOrNull()
                throw ApiError(res.code, message ?: "The server answered with an error (${res.code}).")
            }
            text
        }
    }

    private suspend inline fun <reified T> get(path: String): T = json.decodeFromString(call("GET", path))

    suspend fun status(): ServerStatus = get("/status")
    suspend fun profiles(): Profiles = get("/profiles")
    suspend fun home(): Home = get("/home")
    suspend fun item(id: Long): Item = get("/items/$id")
    suspend fun show(id: Long): ShowDetail = get("/shows/$id")
    suspend fun shows(library: Long): ShowPage = get("/shows?library=$library&sort=title")
    suspend fun items(library: Long, offset: Int, limit: Int): ItemPage =
        get("/items?library=$library&offset=$offset&limit=$limit&sort=title")
    suspend fun play(id: Long, audio: Int, subtitle: Int): PlayInfo = get("/items/$id/play?audio=$audio&subtitle=$subtitle")

    suspend fun login(name: String, password: String, device: String): LoginResult =
        json.decodeFromString(call("POST", "/login", json.encodeToString(Credentials(name, password, device))))

    suspend fun saveProgress(id: Long, positionSec: Double, durationSec: Double) {
        call("POST", "/items/$id/progress", json.encodeToString(ProgressReport(positionSec, durationSec)))
    }

    suspend fun logout() {
        call("POST", "/logout", "{}")
    }

    fun posterUrl(it: Item) = "$base$apiPath/items/${it.id}/poster?v=${it.updatedAt}"
    fun showPosterUrl(s: Show) = "$base$apiPath/shows/${s.id}/art/poster?v=${s.updatedAt}"
    fun mediaUrl(path: String) = if (path.startsWith("http")) path else base + path
}

// Asks an address whether it is a WittyMediaServer, in words for the connect screen.
sealed interface Probe {
    data class Found(val status: ServerStatus) : Probe
    data class Failed(val reason: String) : Probe
}

suspend fun probeServer(url: String): Probe = try {
    val status = Api(url, "").status()
    if (status.name == "WittyMediaServer") Probe.Found(status) else Probe.Failed("not-server")
} catch (_: IOException) {
    Probe.Failed("unreachable")
} catch (_: ApiError) {
    Probe.Failed("not-server")
} catch (_: kotlinx.serialization.SerializationException) {
    Probe.Failed("not-server")
}
