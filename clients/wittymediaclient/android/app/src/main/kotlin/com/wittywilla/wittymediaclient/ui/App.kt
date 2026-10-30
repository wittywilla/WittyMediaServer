package com.wittywilla.wittymediaclient.ui

import androidx.activity.compose.BackHandler
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import com.wittywilla.wittymediaclient.WittyApp
import com.wittywilla.wittymediaclient.data.SavedServer
import com.wittywilla.wittymediaclient.net.Api
import com.wittywilla.wittymediaclient.net.ApiError
import com.wittywilla.wittymediaclient.player.PlayerScreen
import kotlinx.coroutines.launch

sealed interface Route {
    val key: String
    data object Starting : Route { override val key = "starting" }
    data class Connect(val message: String = "") : Route { override val key = "connect" }
    data object SignIn : Route { override val key = "signin" }
    data object Home : Route { override val key = "home" }
    data class Library(val library: com.wittywilla.wittymediaclient.net.Library) : Route { override val key = "library-${library.id}" }
    data class Show(val id: Long) : Route { override val key = "show-$id" }
    data class Item(val id: Long) : Route { override val key = "item-$id" }
    data class Player(val id: Long, val startOver: Boolean) : Route { override val key = "player-$id" }
}

// The app's state and its screen stack. Back pops one screen, and only leaves
// the app from the top level.
class Nav(val app: WittyApp) {
    val stack = mutableStateListOf<Route>(Route.Starting)
    val current: Route get() = stack.last()
    var themeId by mutableStateOf("default")

    fun push(r: Route) { stack.add(r) }
    fun replace(r: Route) { stack[stack.lastIndex] = r }
    fun reset(r: Route) { stack.clear(); stack.add(r) }
    fun pop() { if (stack.size > 1) stack.removeAt(stack.lastIndex) }

    fun openServer(server: SavedServer, token: String = "") {
        app.store.server = server.url
        app.store.remember(server)
        app.store.token = token
        app.api = Api(server.url, token)
    }

    fun signedIn(token: String) {
        app.store.token = token
        app.api = Api(app.api.base, token)
        reset(Route.Home)
    }

    private fun forgetSignIn() {
        app.store.token = ""
        app.api = Api(app.api.base, "")
        reset(Route.SignIn)
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
    val nav = remember { Nav(app) }
    val themes = remember { Assets.themes(context) }
    val strings = remember { Assets.strings(context) }
    val saveable = rememberSaveableStateHolder()

    WittyTheme(pickTheme(themes, nav.themeId)) {
        LaunchedEffect(Unit) {
            val server = app.store.server
            if (server.isEmpty()) { nav.reset(Route.Connect()); return@LaunchedEffect }
            val token = app.store.token
            app.api = Api(server, token)
            try {
                app.api.resolveApiPath()
                val status = app.api.status()
                nav.themeId = status.theme
                nav.reset(if (status.user != null) Route.Home else Route.SignIn)
            } catch (_: Exception) {
                nav.reset(Route.Connect(strings["errLost"].orEmpty()))
            }
        }
        BackHandler(enabled = nav.stack.size > 1) { nav.pop() }
        val route = nav.current
        saveable.SaveableStateProvider(route.key) {
            when (route) {
                Route.Starting -> Screen { Loading() }
                is Route.Connect -> ConnectScreen(nav, strings, route.message)
                Route.SignIn -> SignInScreen(nav, strings)
                Route.Home -> HomeScreen(nav, strings)
                is Route.Library -> LibraryScreen(nav, route.library)
                is Route.Show -> ShowScreen(nav, route.id)
                is Route.Item -> ItemScreen(nav, route.id)
                is Route.Player -> PlayerScreen(nav, route.id, route.startOver)
            }
        }
    }
}
