package com.wittywilla.wittymediaclient.net

// Reads servers announced on the local network: docs/CLIENT-API.md, "Finding a server".
// All of this comes from the network and is treated as untrusted: being announced
// opens nothing, and the app checks the server's identity afterwards.

data class FoundServer(val id: String, val name: String, val url: String, val instance: String = "")

object Discovery {
    const val MAX_SERVERS = 32
    private const val MAX_NAME = 80
    private val ID = Regex("^[0-9a-f]{32}$")
    private val IPV4 = Regex("^\\d{1,3}(\\.\\d{1,3}){3}$")
    private val CONTROL = Regex("[\\u0000-\\u001f\\u007f]")

    fun cleanName(name: String): String = name.replace(CONTROL, "").trim().take(MAX_NAME).ifEmpty { "WittyMediaServer" }

    // A server as the connect screen may show it, or null if any part is unusable.
    fun makeServer(id: String, name: String, host: String, port: Int, tls: Boolean, instance: String = ""): FoundServer? {
        if (!ID.matches(id) || port !in 1..65535) return null
        if (!IPV4.matches(host) || !Address.isPrivateHost(host)) return null
        val r = Address.normalize("${if (tls) "https" else "http"}://$host:$port")
        return if (r is Address.Result.Ok) FoundServer(id, cleanName(name), r.url, instance) else null
    }

    // One resolved mDNS instance, with its TXT record.
    fun fromRecord(instance: String, txt: Map<String, ByteArray?>, host: String?, port: Int): FoundServer? {
        fun value(key: String) = txt[key]?.toString(Charsets.UTF_8).orEmpty()
        return makeServer(value("id"), value("name"), host.orEmpty(), port, value("tls") == "1", instance)
    }
}

// Keeps the list. Servers drop out when the network says they have gone, and the
// oldest goes when too many turn up, so a noisy network can't grow it without limit.
class ServerList {
    private val servers = LinkedHashMap<String, FoundServer>()

    fun add(server: FoundServer): Boolean {
        val before = servers[server.id]
        if (before == null && servers.size >= Discovery.MAX_SERVERS) servers.remove(servers.keys.first())
        servers[server.id] = server
        return before != server
    }

    fun removeInstance(instance: String): Boolean = servers.values.removeAll { it.instance == instance }

    fun list(): List<FoundServer> = servers.values.sortedBy { it.name.lowercase() }
}
