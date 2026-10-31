package com.wittywilla.wittymediaclient

import com.wittywilla.wittymediaclient.diag.ClientEvent
import com.wittywilla.wittymediaclient.diag.LogFile
import com.wittywilla.wittymediaclient.diag.Redact
import com.wittywilla.wittymediaclient.diag.ReportQueue
import com.wittywilla.wittymediaclient.diag.jsonString
import com.wittywilla.wittymediaclient.diag.reportBody
import com.wittywilla.wittymediaclient.net.json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.nio.file.Files

class ClientLogTest {
    private val dir: File = Files.createTempDirectory("wmc-log").toFile()

    @After
    fun cleanUp() {
        dir.deleteRecursively()
    }

    private fun event(message: String) = ClientEvent("error", "crash", message, time = "2026-01-01T00:00:00.000Z")

    // ------------------------------------------------------------ redaction

    @Test
    fun hidesEverySecretInTheSharedCases() {
        val cases = json.parseToJsonElement(File("../../shared/data/redaction-cases.json").readText()).jsonObject["cases"]!!.jsonArray
        assertTrue(cases.size > 10)
        for (c in cases) {
            val o = c.jsonObject
            val out = Redact.clean(o["input"]!!.jsonPrimitive.content)
            for (gone in o["gone"]!!.jsonArray) assertFalse("${gone.jsonPrimitive.content} survived in: $out", out.contains(gone.jsonPrimitive.content))
            for (kept in o["kept"]!!.jsonArray) assertTrue("${kept.jsonPrimitive.content} was lost from: $out", out.contains(kept.jsonPrimitive.content))
        }
    }

    @Test
    fun aKnownTokenIsHiddenWithNothingAroundIt() {
        Redact.addSecret("plainwords-that-are-a-token")
        assertFalse(Redact.clean("the value plainwords-that-are-a-token turned up").contains("plainwords"))
    }

    @Test
    fun eventsAreCleanedAndCutOnTheWayIn() {
        val e = ClientEvent.of("error", "player", "failed at /x?st=PLANTED-1 " + "m".repeat(5000), "at /y?token=PLANTED-2\n" + "s".repeat(50000), request = "r-1")
        assertFalse(e.toJson().contains("PLANTED"))
        assertTrue(e.message.length <= 1001)
        assertTrue(e.stack.length <= 4001)
    }

    @Test
    fun anUnwieldyStringIsCleanedQuickly() {
        val started = System.currentTimeMillis()
        for (s in listOf("a".repeat(1 shl 20), "a.".repeat(1 shl 19), "%41".repeat(1 shl 18), "\"a\":\"".repeat(1 shl 17))) Redact.clean(s)
        assertTrue(System.currentTimeMillis() - started < 5000)
    }

    // ------------------------------------------------------------ lines

    @Test
    fun aLineIsOneLineWithControlCharactersEscaped() {
        val raw = "two\nlines\r\u001b[31mred\u0007 ‮evil  \u0085end\"\\"
        val line = jsonString(raw)
        assertFalse(line.any { it < ' ' || it in '\u007f'..'\u009f' || it == ' ' || it == '‮' })
        assertEquals(raw, json.parseToJsonElement(line).jsonPrimitive.content)
    }

    @Test
    fun aSavedLineReadsBackAsTheEvent() {
        val e = ClientEvent("error", "crash", "said \"this\"\nand that", stack = "at x", page = "thread main", session = "v-1", request = "r-1", time = "2026-01-01T00:00:00.000Z")
        assertEquals(e, json.decodeFromString<ClientEvent>(e.toJson()))
    }

    @Test
    fun aReportHasTheShapeTheServerReads() {
        val body = json.parseToJsonElement(reportBody("app", "1.2", "TV", listOf(event("one"), event("two")))).jsonObject
        assertEquals("app", body["app"]!!.jsonPrimitive.content)
        assertEquals("TV", body["device"]!!.jsonPrimitive.content)
        val events = body["events"]!!.jsonArray
        assertEquals(2, events.size)
        assertEquals("one", events[0].jsonObject["message"]!!.jsonPrimitive.content)
        assertEquals("crash", events[0].jsonObject["kind"]!!.jsonPrimitive.content)
    }

    // ------------------------------------------------------------ the file

    @Test
    fun theFileRotatesAndKeepsOnlySoMany() {
        val file = LogFile(File(dir, "logs"), maxBytes = 1000, keep = 3, linesPerMinute = 100000)
        for (i in 0 until 500) file.append("line $i " + "x".repeat(40))
        val names = File(dir, "logs").list()!!.sorted()
        assertEquals(listOf("client.log", "client.log.1", "client.log.2"), names)
        for (n in names) assertTrue("$n is too big", File(dir, "logs/$n").length() <= 1000)
        assertTrue(File(dir, "logs/client.log").readText().contains("line 499 "))
    }

    @Test
    fun aFloodWritesALimitedNumberOfLinesAndSaysSo() {
        var clock = 1_000_000L
        val file = LogFile(File(dir, "logs"), linesPerMinute = 10, now = { clock })
        for (i in 0 until 1000) file.append("line $i")
        assertEquals(10, File(dir, "logs/client.log").readLines().size)
        clock += 61_000
        file.append("after")
        val lines = File(dir, "logs/client.log").readLines()
        assertTrue(lines[10].contains("990 lines left out"))
        assertEquals("after", lines[11])
    }

    @Test
    fun aFolderThatCantBeWrittenToIsNotAProblem() {
        val blocked = File(dir, "blocked")
        blocked.writeText("a file where the folder should go")
        LogFile(blocked).append("nothing happens")
    }

    // ------------------------------------------------------------ reports

    @Test
    fun repeatsWaitAsOneWithACount() {
        val q = ReportQueue(now = { 1_000_000L })
        repeat(40) { q.add(event("same")) }
        q.add(event("other"))
        val batch = q.take()
        assertEquals(2, batch.size)
        assertEquals(40, batch[0].count)
    }

    @Test
    fun aBatchKeepsToTheRoutesLimits() {
        val q = ReportQueue(now = { 1_000_000L })
        for (i in 0 until 100) q.add(ClientEvent.of("error", "crash", "$i " + "m".repeat(5000), "s".repeat(50000)))
        val batch = q.take()
        assertTrue(batch.size <= 25)
        assertTrue(reportBody("app", "1", "d", batch).toByteArray().size <= 64 * 1024)
    }

    @Test
    fun aFloodStaysBoundedAndCountsWhatItLost() {
        var clock = 1_000_000L
        val q = ReportQueue(now = { clock })
        for (i in 0 until 20000) q.add(event("flood $i"))
        val sent = ArrayList<ClientEvent>()
        repeat(10) { sent += q.take() }
        assertTrue("${sent.size} in a minute", sent.size <= 30)
        repeat(5) {
            clock += 61_000
            repeat(3) { sent += q.take() }
        }
        assertTrue(sent.count { it.message.startsWith("flood") } <= 100)
        assertTrue(sent.first().message.startsWith("${20000 - 100} earlier reports"))
    }

    @Test
    fun aFailedReportIsDroppedAndHeldOff() {
        var clock = 1_000_000L
        val q = ReportQueue(now = { clock })
        q.add(event("one"))
        q.failed(q.take())
        q.add(event("two"))
        assertTrue(q.take().isEmpty())
        assertNotNull(q.waitMs())
        assertTrue(q.waitMs()!! > 0)
        clock += 61_000
        val next = q.take()
        assertEquals(listOf("1 earlier reports from this app were left out", "two"), next.map { it.message })
        assertNull(q.waitMs())
    }
}
