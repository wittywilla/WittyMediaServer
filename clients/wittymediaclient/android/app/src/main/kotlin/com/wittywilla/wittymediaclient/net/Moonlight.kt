package com.wittywilla.wittymediaclient.net

import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

// Getting from Play to a game on someone's own PC. The server's own page can play it, so that
// comes first, shown in this app. The Moonlight app is the fallback: a separate program under
// GPL-3.0 that this app only starts, with nothing of it copied, linked or bundled. These are
// the decisions, kept apart from Android so they can be tested.

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

// The server's own pages for playing.
const val GAME_PCS_PATH = "/moonlight"
fun rigPlayerPath(gameId: Long) = "/moonlight-rig/$gameId"

// Going anywhere else in the web interface means going back to this app's own screens.
fun staysInWebPlayer(path: String) = path == GAME_PCS_PATH || path.startsWith("$GAME_PCS_PATH/") || path.startsWith("/moonlight-rig/")

fun sameServer(url: String, base: String): Boolean {
    val a = url.toHttpUrlOrNull() ?: return false
    val b = base.toHttpUrlOrNull() ?: return false
    return a.scheme == b.scheme && a.host == b.host && a.port == b.port
}

// The server's own sign-in cookie, as it would set it, for this app's web view only.
fun sessionCookie(token: String, https: Boolean): String? {
    if (token.isEmpty() || token.any { it == ';' || it == ',' || it.isWhitespace() || it.isISOControl() }) return null
    return "wms_token=$token; Path=/; HttpOnly; SameSite=Lax" + if (https) "; Secure" else ""
}

sealed interface RigChoice {
    data class InPage(val note: String) : RigChoice
    data class Open(val start: MoonlightStart, val note: String) : RigChoice
    data class Steps(val steps: List<String>) : RigChoice
    data object None : RigChoice
}

private fun inPage(play: RigPlay) = play.methods.firstOrNull { it.kind == "client" }?.let { RigChoice.InPage(it.note) }

private fun inMoonlight(play: RigPlay): RigChoice.Open? {
    val intent = play.methods.firstOrNull { it.kind == "intent" } ?: return null
    return moonlightStart(intent)?.let { RigChoice.Open(it, intent.note) }
}

// What this device does with an answer that's ready. Phones and tablets play in the page. TVs
// go to Moonlight first: whether a TV's web view gives a page the Gamepad API at all is doubtful,
// and a remote alone can't work the player's mouse and keyboard, while Moonlight is built for it.
fun rigChoice(play: RigPlay, isTv: Boolean): RigChoice {
    val first = if (isTv) inMoonlight(play) ?: inPage(play) else inPage(play) ?: inMoonlight(play)
    if (first != null) return first
    val steps = play.methods.firstOrNull { it.kind == "steps" && it.steps.isNotEmpty() }
    return if (steps != null) RigChoice.Steps(steps.steps) else RigChoice.None
}

// The other way to play, if there is one, as a second button.
fun rigOtherChoice(play: RigPlay, isTv: Boolean): RigChoice? =
    when (rigChoice(play, isTv)) {
        is RigChoice.InPage -> inMoonlight(play)
        is RigChoice.Open -> inPage(play)
        else -> null
    }

// Where this device gets Moonlight, best first. The store's own link, then the page the server named.
fun moonlightStoreLinks(isFireTv: Boolean, install: RigInstall?): List<String> =
    listOf(if (isFireTv) "amzn://apps/android?p=$MOONLIGHT_PACKAGE" else "market://details?id=$MOONLIGHT_PACKAGE") +
        listOfNotNull(install?.url?.takeIf(::isWebLink))
