package com.wittywilla.wittymediaclient.net

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Build
import android.os.Handler
import android.os.Looper
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull
import java.net.Inet4Address
import kotlin.coroutines.resume

// Browses for servers while a screen needs them. NsdManager calls back on its own
// threads and older Androids resolve one service at a time, so everything happens
// on the main thread, in turn. A browse that can't start just finds nothing.
class ServerFinder(context: Context, private val onChange: (List<FoundServer>) -> Unit) {
    private class Waiting(val info: NsdServiceInfo, val tries: Int)

    private val nsd = context.applicationContext.getSystemService(Context.NSD_SERVICE) as NsdManager
    private val main = Handler(Looper.getMainLooper())
    private val list = ServerList()
    private val waiting = ArrayDeque<Waiting>()
    private var resolving: Waiting? = null
    private var browsing = false

    private val browse = object : NsdManager.DiscoveryListener {
        override fun onDiscoveryStarted(serviceType: String) = Unit
        override fun onDiscoveryStopped(serviceType: String) = Unit
        override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) { main.post { browsing = false } }
        override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) = Unit
        override fun onServiceFound(info: NsdServiceInfo) { main.post { queue(Waiting(info, 0)) } }
        override fun onServiceLost(info: NsdServiceInfo) {
            main.post {
                waiting.removeAll { it.info.serviceName == info.serviceName }
                if (list.removeInstance(info.serviceName) && browsing) onChange(list.list())
            }
        }
    }

    private val giveUp = Runnable { resolving = null; resolveNext() }

    fun start() {
        main.post {
            browsing = true
            runCatching { nsd.discoverServices(SERVICE_TYPE, NsdManager.PROTOCOL_DNS_SD, browse) }.onFailure { browsing = false }
        }
    }

    fun stop() {
        main.post {
            if (browsing) runCatching { nsd.stopServiceDiscovery(browse) }
            browsing = false
            waiting.clear()
            main.removeCallbacks(giveUp)
        }
    }

    private fun queue(next: Waiting) {
        if (!browsing) return
        waiting.addLast(next)
        resolveNext()
    }

    private fun resolveNext() {
        if (!browsing || resolving != null) return
        val next = waiting.removeFirstOrNull() ?: return
        resolving = next
        main.postDelayed(giveUp, RESOLVE_WAIT_MS) // some devices never answer
        val listener = object : NsdManager.ResolveListener {
            override fun onResolveFailed(info: NsdServiceInfo, errorCode: Int) {
                main.post { finished(next) { if (next.tries < MAX_RETRIES) queue(Waiting(next.info, next.tries + 1)) } }
            }

            override fun onServiceResolved(info: NsdServiceInfo) {
                main.post {
                    finished(next) {
                        val server = Discovery.fromRecord(next.info.serviceName, info.attributes, ipv4(info), info.port)
                        if (server != null && list.add(server) && browsing) onChange(list.list())
                    }
                }
            }
        }
        @Suppress("DEPRECATION") // its replacement starts at Android 14, and this still works there
        runCatching { nsd.resolveService(next.info, listener) }.onFailure { finished(next) {} }
    }

    private fun finished(which: Waiting, then: () -> Unit) {
        if (resolving !== which) return // it was given up on
        main.removeCallbacks(giveUp)
        resolving = null
        then()
        resolveNext()
    }

    // The server only announces IPv4 addresses, but the system may offer others too.
    private fun ipv4(info: NsdServiceInfo): String? {
        @Suppress("DEPRECATION")
        val all = if (Build.VERSION.SDK_INT >= 34) info.hostAddresses else listOfNotNull(info.host)
        return all.firstOrNull { it is Inet4Address }?.hostAddress
    }

    private companion object {
        const val SERVICE_TYPE = "_wittymedia._tcp"
        const val RESOLVE_WAIT_MS = 5000L
        const val MAX_RETRIES = 2
    }
}

// Addresses change, so a saved server is looked for by its ID. The caller still
// has to check its identity: anyone can announce an ID.
suspend fun findServerById(context: Context, id: String, waitMs: Long): FoundServer? = withTimeoutOrNull(waitMs) {
    suspendCancellableCoroutine { waiter ->
        lateinit var finder: ServerFinder
        finder = ServerFinder(context) { servers ->
            val found = servers.find { it.id == id }
            if (found != null && waiter.isActive) {
                finder.stop()
                waiter.resume(found)
            }
        }
        waiter.invokeOnCancellation { finder.stop() }
        finder.start()
    }
}
