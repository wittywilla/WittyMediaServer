package com.wittywilla.wittymediaclient.diag

import com.wittywilla.wittymediaclient.net.json
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.serialization.Serializable
import java.io.File
import java.io.FileOutputStream
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger

// The app's own log: a small file in its private storage, and errors reported to
// the server it is connected to, which is the only place they ever go. Nothing in
// here may throw or cause more errors of its own: a report that fails is dropped.

private const val MAX_FILE_BYTES = 1024L * 1024
private const val KEEP_FILES = 3
private const val LINES_PER_MINUTE = 120
private const val MAX_QUEUED_LINES = 200

// The route's limits are 64 KB and 25 events, and 60 events a minute for an account.
private const val FLUSH_MS = 5_000L
private const val MAX_EVENTS = 25
private const val MAX_BODY_BYTES = 60 * 1024
private const val SEND_PER_MINUTE = 30
private const val MAX_WAITING = 100
private const val HOLD_AFTER_FAILURE_MS = 60_000L
private const val MINUTE_MS = 60_000L

private const val MAX_MESSAGE = 1000
private const val MAX_STACK = 4000
private const val MAX_FIELD = 200
private const val MAX_INPUT = 16 * 1024 // looked at for secrets; anything longer isn't kept anyway
private const val HIDDEN = "[hidden]"

private const val CRASH_FILE = "crash-pending.json"

// The same rules as the server's log, and the same tests on every client
// (shared/data/redaction-cases.json).
object Redact {
    private val patterns: List<Pair<Regex, String>> = listOf(
        // In an address or a command line: token=..., ?st=..., password=...
        Regex("""(?i)(\b(?:token|access_token|refresh_token|id_token|auth|api[_\-]?key|apikey|password|passwd|pwd|pass|secret|pin|key|st|sig|signature|setup[_\-]?code|recovery[_\-]?code|pair(?:ing)?[_\-]?code|otp|totp)=)[^\&\s"'<>]+""") to "$1$HIDDEN",
        // The same with its name percent-encoded: %74oken=... signs in just the same.
        Regex("""(?i)([^\s\&?=;"'<>]{0,24}%[0-9a-f]{2}[^\s\&?=;"'<>]{0,24}=)[^\&\s"'<>]+""") to "$1$HIDDEN",
        // A header, as a line of text.
        Regex("""(?i)\b(proxy-authorization|authorization|set-cookie|cookie|x-wms-token|x-api-key)(\s*[:=]\s*)[^\r\n"']+""") to "$1$2$HIDDEN",
        Regex("""(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=\-]+""") to "$1$HIDDEN",
        Regex("""\b(Basic\s+)[A-Za-z0-9+/]*[A-Z0-9+/][A-Za-z0-9+/]{10,}={0,2}""") to "$1$HIDDEN",
        // JSON, finished or cut short: "password": "..."
        Regex("""(?i)("(?:[a-z_\-]{0,40}(?:token|secret)|password|passwd|api[_\-]?key|apikey|pin|code|setup[_\-]?code|recovery[_\-]?code|pair(?:ing)?[_\-]?code|cookie|authorization)"\s*:\s*)"[^"\\]*(?:\\.[^"\\]*)*(?:"|$)""") to "$1\"$HIDDEN\"",
        // A login in an address: rtsp://user:password@camera/
        Regex("""(?i)\b([a-z][a-z0-9+.\-]{0,20}://)[^/\s:@]+:[^/\s@]+@""") to "$1$HIDDEN@",
        // A share link's token.
        Regex("""(/(?:api/)?(?:s|share)/)[A-Za-z0-9_\-]{8,}""") to "$1$HIDDEN",
        // The sign-in cookie and stream tokens, however they got there.
        Regex("""(?i)\b(wms_token=|st1\.|st_)[A-Za-z0-9._~\-]{6,}""") to "$1$HIDDEN",
        // A long run of hex standing alone is what a session token looks like.
        Regex("""\b[a-fA-F0-9]{48,}\b""") to HIDDEN,
    )

    @Volatile
    private var secrets: List<String> = emptyList()

    // Makes sure this exact value never appears, wherever it turns up.
    fun addSecret(value: String) {
        if (value.length < 6 || value in secrets) return
        secrets = (secrets + value).takeLast(8)
    }

    // If cleaning goes wrong the text isn't kept at all.
    fun clean(text: String): String = try {
        var s = text.take(MAX_INPUT)
        for (secret in secrets) s = s.replace(secret, HIDDEN)
        for ((pattern, with) in patterns) s = pattern.replace(s, with)
        s
    } catch (_: Throwable) {
        "[left out: it could not be cleaned]"
    }
}

private fun cleaned(text: String, limit: Int): String {
    val s = Redact.clean(text)
    return if (s.length > limit) s.take(limit) + "…" else s
}

// JSON leaves some characters alone that a terminal or an editor acts on.
internal fun jsonString(s: String): String {
    val out = StringBuilder(s.length + 2).append('"')
    for (c in s) {
        when {
            c == '"' -> out.append("\\\"")
            c == '\\' -> out.append("\\\\")
            c == '\n' -> out.append("\\n")
            c == '\r' -> out.append("\\r")
            c == '\t' -> out.append("\\t")
            c < ' ' || c in '\u007f'..'\u009f' || c == ' ' || c == ' ' || c in '‪'..'‮' || c in '⁦'..'⁩' ->
                out.append(String.format(Locale.ROOT, "\\u%04x", c.code))
            else -> out.append(c)
        }
    }
    return out.append('"').toString()
}

private fun timeText(millis: Long): String =
    SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss.SSS'Z'", Locale.ROOT).apply { timeZone = TimeZone.getTimeZone("UTC") }.format(Date(millis))

// One thing that happened, in the form the server reads: docs/LOGGING.md.
@Serializable
data class ClientEvent(
    val level: String,
    val kind: String,
    val message: String,
    val stack: String = "",
    val page: String = "",
    val session: String = "",
    val request: String = "",
    val count: Int = 1,
    val time: String = "",
) {
    // One line of the log file, and one entry of a report.
    fun toJson(): String = buildString {
        append("{\"time\":").append(jsonString(time))
        append(",\"level\":").append(jsonString(level))
        append(",\"kind\":").append(jsonString(kind))
        append(",\"message\":").append(jsonString(message))
        if (stack.isNotEmpty()) append(",\"stack\":").append(jsonString(stack))
        if (page.isNotEmpty()) append(",\"page\":").append(jsonString(page))
        if (session.isNotEmpty()) append(",\"session\":").append(jsonString(session))
        if (request.isNotEmpty()) append(",\"request\":").append(jsonString(request))
        append(",\"count\":").append(count).append('}')
    }

    companion object {
        // Everything that goes in passes through here, so no call site has to remember.
        fun of(level: String, kind: String, message: String, stack: String = "", page: String = "", session: String = "", request: String = "", millis: Long = System.currentTimeMillis()) =
            ClientEvent(
                level = level,
                kind = kind,
                message = cleaned(message, MAX_MESSAGE),
                stack = if (stack.isEmpty()) "" else cleaned(stack, MAX_STACK),
                page = cleaned(page, MAX_FIELD),
                session = session.take(MAX_FIELD),
                request = request.take(MAX_FIELD),
                time = timeText(millis),
            )
    }
}

fun reportBody(app: String, version: String, device: String, events: List<ClientEvent>): String =
    "{\"app\":${jsonString(app)},\"version\":${jsonString(version)},\"device\":${jsonString(device)},\"events\":[${events.joinToString(",") { it.toJson() }}]}"

// The file, moved along by size and with only so many kept. A flood writes a limited
// number of lines a minute and then says how many it left out.
class LogFile(
    private val dir: File,
    private val maxBytes: Long = MAX_FILE_BYTES,
    private val keep: Int = KEEP_FILES,
    private val linesPerMinute: Int = LINES_PER_MINUTE,
    private val now: () -> Long = { System.currentTimeMillis() },
) {
    private val current = File(dir, "client.log")
    private var windowStart = 0L
    private var linesInWindow = 0
    private var leftOut = 0

    private fun numbered(i: Int) = File(dir, "client.log.$i")

    private fun rotate() {
        numbered(keep - 1).delete()
        for (i in keep - 2 downTo 1) numbered(i).renameTo(numbered(i + 1))
        // Something holding it open still can't make it grow for ever.
        if (!current.renameTo(numbered(1))) current.delete()
    }

    private fun write(line: String) {
        val bytes = (line + "\n").toByteArray()
        if (current.length() > 0 && current.length() + bytes.size > maxBytes) rotate()
        FileOutputStream(current, true).use { it.write(bytes) }
    }

    // Never throws: a line that can't be written is a line lost.
    @Synchronized
    fun append(line: String) {
        try {
            dir.mkdirs()
            val t = now()
            if (t - windowStart >= MINUTE_MS) {
                if (leftOut > 0) write(ClientEvent("warn", "log", "$leftOut lines left out: too many at once", time = timeText(t)).toJson())
                leftOut = 0
                windowStart = t
                linesInWindow = 0
            }
            if (linesInWindow >= linesPerMinute) {
                leftOut++
                return
            }
            linesInWindow++
            write(line)
        } catch (_: Throwable) {
            // see above
        }
    }
}

// What waits to go to the server. Repeats wait as one event with a count, and a full
// queue drops the newest: the first errors are usually the ones that explain the rest.
class ReportQueue(private val now: () -> Long = { System.currentTimeMillis() }) {
    private val waiting = LinkedHashMap<String, ClientEvent>()
    private var dropped = 0
    private var earlierInBatch = 0
    private var holdUntil = 0L
    private var windowStart = 0L
    private var sentInWindow = 0

    @Synchronized
    fun add(e: ClientEvent) {
        val key = listOf(e.level, e.kind, e.message, e.stack, e.page, e.session, e.request).joinToString("\u0000")
        val same = waiting[key]
        when {
            same != null -> waiting[key] = same.copy(count = same.count + 1)
            waiting.size >= MAX_WAITING -> dropped++
            else -> waiting[key] = e
        }
    }

    // The next batch, within the route's limits. Empty means not now.
    @Synchronized
    fun take(): List<ClientEvent> {
        val t = now()
        if (t < holdUntil) return emptyList()
        if (t - windowStart >= MINUTE_MS) {
            windowStart = t
            sentInWindow = 0
        }
        val room = minOf(MAX_EVENTS, SEND_PER_MINUTE - sentInWindow)
        if (room <= 0) return emptyList()
        val out = ArrayList<ClientEvent>()
        earlierInBatch = 0
        if (dropped > 0) {
            out += ClientEvent("warn", "network", "$dropped earlier reports from this app were left out", time = timeText(t))
            earlierInBatch = dropped
            dropped = 0
        }
        var bytes = out.sumOf { it.toJson().length * 2 } + 512 // the app, version and device around them
        val rows = waiting.entries.iterator()
        while (rows.hasNext() && out.size < room) {
            val event = rows.next().value
            val size = event.toJson().toByteArray().size
            if (bytes + size > MAX_BODY_BYTES) break
            bytes += size
            out += event
            rows.remove()
        }
        sentInWindow += out.size
        return out
    }

    // Whatever went wrong, what was being sent is gone. Nothing is sent again soon.
    @Synchronized
    fun failed(batch: List<ClientEvent>) {
        dropped += batch.size + earlierInBatch - (if (earlierInBatch > 0) 1 else 0)
        earlierInBatch = 0
        holdUntil = now() + HOLD_AFTER_FAILURE_MS
    }

    // How long before another batch can go, or null for nothing waiting.
    @Synchronized
    fun waitMs(): Long? {
        if (waiting.isEmpty() && dropped == 0) return null
        val t = now()
        val forWindow = if (sentInWindow >= SEND_PER_MINUTE) windowStart + MINUTE_MS - t else 0L
        return maxOf(holdUntil - t, forWindow, 0L)
    }
}

// Where reports can go now.
class ReportTarget(val token: String, val send: suspend (String) -> Int)

object ClientLog {
    // The stream in play. Whatever is reported meanwhile is filed under it on the server.
    @Volatile
    var session: String = ""

    private val queue = ReportQueue()
    private val scheduled = AtomicBoolean(false)
    private val queuedLines = AtomicInteger(0)
    private val writer = Executors.newSingleThreadExecutor { r -> Thread(r, "client-log").apply { isDaemon = true } }

    @Volatile
    private var file: LogFile? = null

    @Volatile
    private var directory: File? = null

    @Volatile
    private var scope: CoroutineScope? = null

    @Volatile
    private var target: (() -> ReportTarget?)? = null

    private var app = ""
    private var version = ""
    private var device = ""

    fun start(dir: File, app: String, version: String, device: String, scope: CoroutineScope, target: () -> ReportTarget?) {
        if (file != null) return
        this.app = app
        this.version = version
        this.device = device
        this.scope = scope
        this.target = target
        directory = dir
        file = LogFile(dir)
        // The crash goes in the file before the system's own handling, which is kept as it was.
        val previous = Thread.getDefaultUncaughtExceptionHandler()
        Thread.setDefaultUncaughtExceptionHandler { thread, e ->
            saveCrash(thread.name, e)
            previous?.uncaughtException(thread, e)
        }
        writer.execute { sendLastCrash() }
        info("app", "Started, version $version, $device")
    }

    fun error(kind: String, message: String, stack: String = "", request: String = "", session: String = this.session) =
        report("error", kind, message, stack, request, session)

    fun warn(kind: String, message: String, stack: String = "", request: String = "", session: String = this.session) =
        report("warn", kind, message, stack, request, session)

    // Only for the file on this device.
    fun info(kind: String, message: String) = report("info", kind, message, "", "", "")

    fun report(level: String, kind: String, message: String, stack: String = "", request: String = "", session: String = this.session) {
        try {
            val event = ClientEvent.of(level, kind, message, stack, session = session, request = request)
            writeLine(event)
            if (level != "info") {
                queue.add(event)
                schedule(FLUSH_MS)
            }
        } catch (_: Throwable) {
            // the log must never be why something else fails
        }
    }

    // Waiting reports can go now: someone signed in.
    fun kick() {
        try {
            if (queue.waitMs() != null) schedule(FLUSH_MS)
        } catch (_: Throwable) {
            // see report
        }
    }

    private fun writeLine(event: ClientEvent) {
        val to = file ?: return
        if (queuedLines.get() >= MAX_QUEUED_LINES) return
        queuedLines.incrementAndGet()
        val line = event.toJson()
        writer.execute {
            to.append(line)
            queuedLines.decrementAndGet()
        }
    }

    // On the thread that's about to die, so nothing here can wait for another.
    private fun saveCrash(thread: String, e: Throwable) {
        try {
            val dir = directory ?: return
            val line = ClientEvent.of("error", "crash", "The app stopped: ${e.javaClass.name}: ${e.message.orEmpty()}", e.stackTraceToString(), "thread $thread", session).toJson()
            file?.append(line)
            dir.mkdirs()
            File(dir, CRASH_FILE).writeText(line)
        } catch (_: Throwable) {
            // nothing more can be done from here
        }
    }

    private fun sendLastCrash() {
        try {
            val saved = File(directory ?: return, CRASH_FILE)
            if (!saved.exists()) return
            val text = saved.readText()
            saved.delete()
            queue.add(json.decodeFromString<ClientEvent>(text))
            schedule(FLUSH_MS)
        } catch (_: Throwable) {
            // an unreadable crash file is gone, and the file log still has it
        }
    }

    private fun schedule(ms: Long) {
        val where = scope ?: return
        if (!scheduled.compareAndSet(false, true)) return
        where.launch(Dispatchers.IO) {
            try {
                delay(ms)
            } finally {
                scheduled.set(false)
            }
            flush()
        }
    }

    private suspend fun flush() {
        try {
            val to = target?.invoke() ?: return // kept until there's a sign-in: see kick
            Redact.addSecret(to.token)
            val batch = queue.take()
            if (batch.isNotEmpty()) {
                val status = try {
                    to.send(reportBody(app, version, device, batch))
                } catch (_: Exception) {
                    0
                }
                if (status !in 200..299) {
                    queue.failed(batch)
                    file?.append(ClientEvent.of("info", "log", "Reports to the server were dropped: the answer was $status").toJson())
                }
            }
            queue.waitMs()?.let { schedule(maxOf(it, FLUSH_MS)) }
        } catch (_: Throwable) {
            // see report
        }
    }
}
