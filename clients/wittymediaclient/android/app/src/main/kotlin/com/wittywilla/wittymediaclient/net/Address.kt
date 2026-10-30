package com.wittywilla.wittymediaclient.net

import java.net.URI
import java.net.URISyntaxException

// Turns what someone typed into a server address, or says why it can't be one.
// Every client follows the same rules: see shared/data/connection-cases.json.
object Address {
    const val DEFAULT_PORT = 9797

    sealed interface Result {
        data class Ok(val url: String) : Result
        data class Bad(val reason: String) : Result
    }

    private val SCHEME = Regex("^([a-zA-Z][a-zA-Z0-9+.-]*)://")
    private val BARE_SCHEME = Regex("^(javascript|file|data|mailto):", RegexOption.IGNORE_CASE)
    private val HAS_PORT = Regex("(\\]|^[^\\[\\]:]+):\\d*$")
    private val IPV4 = Regex("^(\\d{1,3})\\.(\\d{1,3})\\.(\\d{1,3})\\.(\\d{1,3})$")

    fun normalize(input: String): Result {
        var text = input.trim()
        if (text.isEmpty()) return Result.Bad("empty")
        val scheme = SCHEME.find(text)?.groupValues?.get(1) ?: BARE_SCHEME.find(text)?.groupValues?.get(1)
        var addPort = false
        if (scheme != null) {
            if (!scheme.equals("http", true) && !scheme.equals("https", true)) return Result.Bad("scheme")
        } else {
            // A bare IPv6 address needs its brackets before it can be parsed.
            if (text.count { it == ':' } >= 2 && !text.startsWith("[")) text = "[$text]"
            addPort = !HAS_PORT.containsMatchIn(text)
            text = "http://$text"
        }
        val uri = try {
            URI(text)
        } catch (_: URISyntaxException) {
            return Result.Bad("invalid")
        }
        if (uri.rawUserInfo != null) return Result.Bad("credentials")
        val host = uri.host?.lowercase() ?: return Result.Bad("invalid")
        val secure = uri.scheme.equals("https", true)
        val port = if (addPort) DEFAULT_PORT else uri.port
        if (port > 65535) return Result.Bad("invalid")
        if (!secure && !isPrivateHost(host)) return Result.Bad("public-http")
        val defaultPort = if (secure) 443 else 80
        val suffix = if (port == -1 || port == defaultPort) "" else ":$port"
        return Result.Ok("${if (secure) "https" else "http"}://$host$suffix")
    }

    // Is this address on the local network? A name with no dots or ending in
    // .local can only be resolved locally; anything else may be public.
    fun isPrivateHost(host: String): Boolean {
        IPV4.matchEntire(host)?.let { m ->
            val a = m.groupValues[1].toInt()
            val b = m.groupValues[2].toInt()
            return a == 127 || a == 10 || (a == 172 && b in 16..31) || (a == 192 && b == 168) ||
                (a == 169 && b == 254) || (a == 100 && b in 64..127)
        }
        if (host.startsWith("[")) {
            val h = host.substring(1, host.length - 1)
            return h == "::1" || h.startsWith("fc") || h.startsWith("fd") ||
                h.startsWith("fe8") || h.startsWith("fe9") || h.startsWith("fea") || h.startsWith("feb")
        }
        return host == "localhost" || !host.contains('.') || host.endsWith(".local")
    }
}
