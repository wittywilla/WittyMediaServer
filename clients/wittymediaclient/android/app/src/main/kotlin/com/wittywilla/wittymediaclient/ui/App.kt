package com.wittywilla.wittymediaclient.ui

import androidx.activity.compose.BackHandler
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import com.wittywilla.wittymediaclient.WittyApp
import com.wittywilla.wittymediaclient.data.SavedServer
import com.wittywilla.wittymediaclient.diag.ClientLog
import com.wittywilla.wittymediaclient.net.Api
import com.wittywilla.wittymediaclient.net.ApiError
import com.wittywilla.wittymediaclient.net.Identity
import com.wittywilla.wittymediaclient.net.IdentityState
import com.wittywilla.wittymediaclient.net.NextStep
import com.wittywilla.wittymediaclient.net.ServerProof
import com.wittywilla.wittymediaclient.net.ServerStatus
import com.wittywilla.wittymediaclient.net.findServerById
import com.wittywilla.wittymediaclient.net.identify
import com.wittywilla.wittymediaclient.player.PlayerScreen
import kotlinx.coroutines.launch
import kotlinx.serialization.SerializationException
import java.io.IOException

enum class SignInWay { Usual, Password, ShownCode }

sealed interface Route {
    val key: String
    data object Starting : Route { override val key = "starting" }
    data class Connect(val message: String = "") : Route { override val key = "connect" }
    data class Code(val server: SavedServer) : Route { override val key = "code" }
    data class Mismatch(val server: SavedServer) : Route { override val key = "mismatch" }
    data class SignIn(val way: SignInWay = SignInWay.Usual) : Route { override val key = "signin-$way" }
    data object Home : Route { override val key = "home" }
    data class Library(val library: com.wittywilla.wittymediaclient.net.Library) : Route { override val key = "library-${library.id}" }
    data class Show(val id: Long) : Route { override val key = "show-$id" }
    data class Item(val id: Long) : Route { override val key = "item-$id" }
    data class Player(val id: Long, val startOver: Boolean) : Route { override val key = "player-$id" }
    data class Web(val path: String) : Route { override val key = "web-$path" }
}

// How long a saved server's ID is looked for when its address stops answering.
private const val FIND_AGAIN_MS = 6000L

// The app's state and its screen stack. Back pops one screen, and only leaves
// the app from the top level.
class Nav(val app: WittyApp, private val s: Map<String, String>) {
    val stack = mutableStateListOf<Route>(Route.Starting)
    val current: Route get() = stack.last()
    var themeId by mutableStateOf("default")

    // The server the sign-in screens talk to. Its key waits here for a token to be
    // kept with, unless it was already pinned.
    var server: SavedServer? = null
        private set

    fun push(r: Route) { stack.add(r) }
    fun replace(r: Route) { stack[stack.lastIndex] = r }
    fun reset(r: Route) { stack.clear(); stack.add(r) }
    fun pop() { if (stack.size > 1) stack.removeAt(stack.lastIndex) }

    // The warning, with the safe way out of it underneath.
    private fun warn(about: SavedServer) { reset(Route.Connect()); push(Route.Mismatch(about)) }

    // Only for a server whose identity has just been checked.
    private fun openServer(opening: SavedServer, token: String = "") {
        server = opening
        app.store.server = opening.url
        app.api = Api(opening.url, token)
        if (token.isNotEmpty()) ClientLog.kick()
    }

    // On to signing in, for a server that has been checked.
    fun signInTo(opening: SavedServer, way: SignInWay = SignInWay.Usual) {
        app.store.token = ""
        openServer(opening)
        reset(Route.SignIn(way))
    }

    // Opens a server with a token that was kept with its key.
    private suspend fun openWithToken(opening: SavedServer): String? = try {
        app.store.remember(opening)
        openServer(opening, app.store.token)
        val status = app.api.status()
        themeId = status.theme
        reset(if (status.user != null) Route.Home else Route.SignIn())
        null
    } catch (_: IOException) {
        s["errLost"].orEmpty()
    } catch (e: ApiError) {
        e.message.orEmpty()
    } catch (_: SerializationException) {
        s["errNotServer"].orEmpty()
    }

    private fun tokenFits(pin: String, proof: ServerProof) =
        app.store.token.isNotEmpty() && Identity.fits(pin, proof)

    // What the last run was using, if it still is who it was. Its token goes
    // nowhere until then.
    suspend fun startUp() {
        val url = app.store.server
        if (url.isEmpty()) { reset(Route.Connect()); return }
        var saved = app.store.find(url) ?: SavedServer(url, "WittyMediaServer")
        var proof = identify(url, listOf(saved))
        if (proof.state == IdentityState.Unreachable && saved.serverId.isNotEmpty()) {
            val found = findServerById(app, saved.serverId, FIND_AGAIN_MS)
            if (found != null) {
                saved = saved.copy(url = found.url)
                proof = identify(found.url, listOf(saved))
            }
        }
        val checked = proof.savedAs(saved.url, saved.name)
        when (Identity.stepAtStart(proof.state, tokenFits(saved.publicKey, proof))) {
            NextStep.Warn -> warn(checked)
            NextStep.Refuse -> reset(Route.Connect(s["identityBad"].orEmpty()))
            NextStep.Lost -> reset(Route.Connect(s["errLost"].orEmpty()))
            NextStep.Open -> openWithToken(checked)?.let { reset(Route.Connect(it)) }
            else -> {
                openServer(checked)
                try {
                    themeId = app.api.status().theme
                    reset(Route.SignIn())
                } catch (_: Exception) {
                    reset(Route.Connect(s["errLost"].orEmpty()))
                }
            }
        }
    }

    // Coming back to an app left running counts as starting it. Nothing answering is
    // left to the screens, as the network is often still waking up.
    suspend fun recheck() {
        val using = server ?: return
        if (app.api.token.isEmpty()) return
        val proof = identify(using.url, listOf(using))
        if (proof.state != IdentityState.Different && proof.state != IdentityState.Bad) return
        server = null
        app.api = Api("", "")
        if (proof.state == IdentityState.Bad) reset(Route.Connect(s["identityBad"].orEmpty()))
        else warn(proof.savedAs(using.url, using.name))
    }

    // Someone chose this server, and it says it's a WittyMediaServer. Its identity
    // decides what comes next. Answers words to show if that's nowhere.
    suspend fun arrive(url: String, status: ServerStatus): String? {
        val proof = identify(url, app.store.recent)
        val pin = app.store.find(app.store.server)?.publicKey.orEmpty()
        val checked = proof.savedAs(url, status.brandName.ifEmpty { status.name })
        val step = Identity.nextStep(proof.state, pin.isNotEmpty() && tokenFits(pin, proof))
        if (step != NextStep.Refuse && step != NextStep.Lost) themeId = status.theme
        return when (step) {
            NextStep.Warn -> { push(Route.Mismatch(checked)); null }
            NextStep.Refuse -> s["identityBad"].orEmpty()
            NextStep.Lost -> s["errUnreachable"].orEmpty()
            NextStep.AskCode -> { push(Route.Code(checked)); null }
            NextStep.SignIn -> { signInTo(checked); null }
            NextStep.Open -> openWithToken(checked)
        }
    }

    // "This is my server, trust it". The key pinned is the one the warning showed, or
    // it asks again, and the old sign-in isn't kept: docs/CLIENT-API.md says sign in again.
    suspend fun trust(shown: SavedServer): String? {
        val proof = identify(shown.url, emptyList())
        return when {
            proof.state == IdentityState.Unreachable -> s["errUnreachable"].orEmpty()
            proof.state != IdentityState.New -> s["identityBad"].orEmpty()
            proof.publicKey != shown.publicKey -> { replace(Route.Mismatch(proof.savedAs(shown.url, shown.name))); null }
            else -> {
                app.store.token = ""
                app.store.remember(shown)
                reset(Route.Connect())
                push(Route.Code(shown))
                null
            }
        }
    }

    // A code typed on this device. The server has to prove itself again first, as it
    // could have changed since the screen opened. Answers words to show on failure.
    suspend fun redeem(to: SavedServer, code: String): String? {
        val proof = identify(to.url, listOf(to))
        if (proof.state == IdentityState.Unreachable) return s["errUnreachable"]
        if (!Identity.fits(to.publicKey, proof)) return s["identityBad"]
        return try {
            val result = Api(to.url, "", quick = true).pairRedeem(code, app.deviceName, app.platformName)
            if (result.token.isEmpty()) return s["errInvalid"]
            openServer(to)
            signedIn(result.token)
            null
        } catch (e: ApiError) {
            e.message
        } catch (_: IOException) {
            s["errUnreachable"]
        } catch (_: SerializationException) {
            s["errInvalid"]
        }
    }

    fun signedIn(token: String) {
        val kept = server ?: SavedServer(app.api.base, "WittyMediaServer")
        app.store.signedIn(kept, token)
        app.api = Api(kept.url, token)
        ClientLog.kick()
        reset(Route.Home)
    }

    private fun forgetSignIn() {
        app.store.token = ""
        app.api = Api(app.api.base, "")
        reset(Route.SignIn())
    }

    // Ends the session on the server too, so no valid sign-in is left behind.
    private fun endSession() {
        val old = app.api
        if (old.token.isNotEmpty()) app.scope.launch { runCatching { old.logout() } }
    }

    fun signOut() {
        endSession()
        forgetSignIn()
    }

    fun changeServer() {
        endSession()
        server = null
        app.store.server = ""
        app.store.token = ""
        app.api = Api("", "")
        reset(Route.Connect())
    }

    // A sign-in the server no longer accepts means signing in again.
    fun handleExpired(e: Throwable): Boolean {
        if (e is ApiError && e.status == 401) { forgetSignIn(); return true }
        return false
    }
}

@Composable
fun AppRoot(app: WittyApp) {
    val context = LocalContext.current
    val themes = remember { Assets.themes(context) }
    val strings = remember { Assets.strings(context) }
    val nav = remember { Nav(app, strings) }
    val saveable = rememberSaveableStateHolder()
    val scope = rememberCoroutineScope()

    WittyTheme(pickTheme(themes, nav.themeId)) {
        LaunchedEffect(Unit) { nav.startUp() }
        LifecycleEventEffect(Lifecycle.Event.ON_START) { scope.launch { nav.recheck() } }
        BackHandler(enabled = nav.stack.size > 1) { nav.pop() }
        val route = nav.current
        saveable.SaveableStateProvider(route.key) {
            when (route) {
                Route.Starting -> Screen { Loading() }
                is Route.Connect -> ConnectScreen(nav, strings, route.message)
                is Route.Code -> CodeScreen(nav, strings, route.server)
                is Route.Mismatch -> MismatchScreen(nav, strings, route.server)
                is Route.SignIn -> SignInScreen(nav, strings, route.way)
                Route.Home -> HomeScreen(nav, strings)
                is Route.Library -> LibraryScreen(nav, route.library)
                is Route.Show -> ShowScreen(nav, route.id)
                is Route.Item -> ItemScreen(nav, route.id)
                is Route.Player -> PlayerScreen(nav, route.id, route.startOver)
                is Route.Web -> WebPlayerScreen(nav, route)
            }
        }
    }
}
