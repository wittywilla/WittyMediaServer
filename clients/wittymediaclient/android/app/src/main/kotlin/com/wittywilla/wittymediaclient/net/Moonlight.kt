package com.wittywilla.wittymediaclient.net

// Getting from Play to a game on someone's own PC, which runs in the Moonlight app. Moonlight
// is a separate program under GPL-3.0: this app only starts it, and nothing of it is copied,
// linked or bundled. These are the decisions, kept apart from Android so they can be tested.

const val MOONLIGHT_PACKAGE = "com.limelight"
private const val MOONLIGHT_ACTIVITY = "com.limelight.ShortcutTrampoline"
private val MOONLIGHT_NEEDS = listOf("UUID", "AppId", "AppName")
private val MOONLIGHT_EXTRAS = MOONLIGHT_NEEDS + "Name"

val Item.isRigGame: Boolean get() = kind == "game" && console == "rig"
val Library.isRigGames: Boolean get() = type == "games" && console == "rig"

fun rigOsName(isTv: Boolean, isFireTv: Boolean) = when {
    isFireTv -> "firetv"
    isTv -> "androidtv"
    else -> "android"
}

// What to put in the intent that starts Moonlight.
data class MoonlightStart(val packageName: String, val activity: String, val extras: Map<String, String>)

// A server could send anything, and an explicit intent can start any app's exported screen,
// so this only ever names Moonlight's own, with the string extras it takes.
fun moonlightStart(method: RigMethod): MoonlightStart? {
    if (method.kind != "intent" || method.packageName != MOONLIGHT_PACKAGE || method.activity != MOONLIGHT_ACTIVITY) return null
    if (MOONLIGHT_NEEDS.any { method.extras[it].isNullOrEmpty() }) return null
    return MoonlightStart(MOONLIGHT_PACKAGE, MOONLIGHT_ACTIVITY, method.extras.filterKeys { it in MOONLIGHT_EXTRAS })
}

// Only web links leave for a browser: anything else could open other apps.
fun isWebLink(url: String) = url.startsWith("https://", ignoreCase = true) || url.startsWith("http://", ignoreCase = true)

sealed interface RigChoice {
    data class Open(val start: MoonlightStart, val note: String) : RigChoice
    data class Browser(val url: String, val note: String) : RigChoice
    data class Steps(val steps: List<String>) : RigChoice
    data object None : RigChoice
}

// What this device does with an answer that's ready. TVs mostly have no browser worth sending
// anyone to, and can't use a game streamed to one.
fun rigChoice(play: RigPlay, isTv: Boolean): RigChoice {
    val intent = play.methods.firstOrNull { it.kind == "intent" }
    val start = intent?.let(::moonlightStart)
    if (start != null) return RigChoice.Open(start, intent?.note.orEmpty())
    if (!isTv) {
        val bridge = play.methods.firstOrNull { it.kind == "bridge" && isWebLink(it.url) }
        if (bridge != null) return RigChoice.Browser(bridge.url, bridge.note)
    }
    val steps = play.methods.firstOrNull { it.kind == "steps" && it.steps.isNotEmpty() }
    return if (steps != null) RigChoice.Steps(steps.steps) else RigChoice.None
}

// Where this device gets Moonlight, best first. The store's own link, then the page the server named.
fun moonlightStoreLinks(isFireTv: Boolean, install: RigInstall?): List<String> =
    listOf(if (isFireTv) "amzn://apps/android?p=$MOONLIGHT_PACKAGE" else "market://details?id=$MOONLIGHT_PACKAGE") +
        listOfNotNull(install?.url?.takeIf(::isWebLink))
