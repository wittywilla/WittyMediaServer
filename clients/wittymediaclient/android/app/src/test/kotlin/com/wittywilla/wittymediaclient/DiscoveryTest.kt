package com.wittywilla.wittymediaclient

import com.wittywilla.wittymediaclient.net.Discovery
import com.wittywilla.wittymediaclient.net.FoundServer
import com.wittywilla.wittymediaclient.net.ServerList
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

// The same rules as the desktop client's desktop/test/connect.test.js.
class DiscoveryTest {
    private val id = "0123456789abcdef0123456789abcdef"

    private fun txt(id: String = this.id, name: String? = "Home", tls: String = "0"): Map<String, ByteArray?> =
        mapOf("id" to id.toByteArray(), "name" to name?.toByteArray(), "v" to "0.4.0".toByteArray(), "api" to "1".toByteArray(), "tls" to tls.toByteArray())

    @Test
    fun readsAResolvedInstance() {
        assertEquals(FoundServer(id, "Home", "http://192.168.1.20:9797", "Home (0123)"), Discovery.fromRecord("Home (0123)", txt(), "192.168.1.20", 9797))
    }

    @Test
    fun tlsOneMeansHttps() {
        assertEquals("https://192.168.1.20:9797", Discovery.fromRecord("x", txt(tls = "1"), "192.168.1.20", 9797)?.url)
    }

    @Test
    fun refusesAddressesOutsideTheLocalNetwork() {
        assertNull(Discovery.fromRecord("x", txt(), "8.8.8.8", 9797))
        assertNull(Discovery.fromRecord("x", txt(), null, 9797))
        assertNull(Discovery.fromRecord("x", txt(), "evil.example.com", 9797))
    }

    @Test
    fun refusesABadIdOrPort() {
        assertNull(Discovery.fromRecord("x", txt(id = "not-an-id"), "192.168.1.20", 9797))
        assertNull(Discovery.fromRecord("x", emptyMap(), "192.168.1.20", 9797))
        assertNull(Discovery.makeServer(id, "x", "192.168.1.2", 70000, false))
        assertNull(Discovery.makeServer(id, "x", "192.168.1.2", 0, false))
    }

    @Test
    fun cleansNames() {
        assertEquals("AB", Discovery.makeServer(id, "A\u0007B\n", "10.0.0.2", 9797, false)?.name)
        assertEquals(80, Discovery.makeServer(id, "x".repeat(500), "10.0.0.2", 9797, false)?.name?.length)
        assertEquals("WittyMediaServer", Discovery.makeServer(id, "", "10.0.0.2", 9797, false)?.name)
        assertEquals("WittyMediaServer", Discovery.fromRecord("x", txt(name = null), "10.0.0.2", 9797)?.name)
    }

    @Test
    fun theListNoticesChangesAndForgetsWhatIsLost() {
        val list = ServerList()
        val home = FoundServer(id, "Home", "http://10.0.0.2:9797", "Home (0123)")
        assertTrue(list.add(home))
        assertFalse("the same answer again is no change", list.add(home))
        assertTrue("a new address is", list.add(home.copy(url = "http://10.0.0.3:9797")))
        assertEquals(1, list.list().size)
        assertFalse(list.removeInstance("Someone else"))
        assertTrue(list.removeInstance("Home (0123)"))
        assertTrue(list.list().isEmpty())
    }

    @Test
    fun theListIsSortedAndCanNotGrowWithoutLimit() {
        val list = ServerList()
        list.add(FoundServer("2", "beta", "http://10.0.0.2"))
        list.add(FoundServer("1", "Alpha", "http://10.0.0.1"))
        assertEquals(listOf("Alpha", "beta"), list.list().map { it.name })
        for (i in 0 until Discovery.MAX_SERVERS + 10) list.add(FoundServer("n$i", "n$i", "http://10.0.0.2"))
        assertEquals(Discovery.MAX_SERVERS, list.list().size)
    }
}
