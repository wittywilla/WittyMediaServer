package com.wittywilla.wittymediaclient.ui

import android.annotation.SuppressLint
import android.app.Activity
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.graphics.Color
import android.webkit.CookieManager
import android.webkit.WebChromeClient
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.net.toUri
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import com.wittywilla.wittymediaclient.net.isWebLink
import com.wittywilla.wittymediaclient.net.sameServer
import com.wittywilla.wittymediaclient.net.sessionCookie
import com.wittywilla.wittymediaclient.net.staysInWebPlayer
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

private const val CANT_OPEN = "The game player couldn't open. Check the server is on, then try again."
private const val NO_WEB_VIEW = "This device has no working web view, which the game player needs."

// The server's own page in a web view, kept to that server. Only web links leave, for the
// browser. There's no script bridge, so the page can reach nothing of this app or this device.
private class PlayerClient(
    private val base: String,
    private val onLeave: () -> Unit,
    private val onFail: () -> Unit,
) : WebViewClient() {
    override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
        val url = request.url.toString()
        if (sameServer(url, base)) return false
        if (request.isForMainFrame && isWebLink(url)) {
            try {
                view.context.startActivity(Intent(Intent.ACTION_VIEW, url.toUri()))
            } catch (_: ActivityNotFoundException) {
                // Nowhere to send it. The page stays as it is.
            }
        }
        return true
    }

    // Also runs for the page changing its own address, which is how it closes its player.
    override fun doUpdateVisitedHistory(view: WebView, url: String, isReload: Boolean) {
        val path = url.toHttpUrlOrNull()?.encodedPath ?: return
        if (!staysInWebPlayer(path)) onLeave()
    }

    override fun onReceivedError(view: WebView, request: WebResourceRequest, error: WebResourceError) {
        if (request.isForMainFrame) onFail()
    }
}

// JavaScript is what the player is. It only ever runs the signed-in server's own page.
@SuppressLint("SetJavaScriptEnabled")
private fun playerWebView(context: Context): WebView = WebView(context).apply {
    setBackgroundColor(Color.BLACK)
    keepScreenOn = true // a game played on a controller never touches the screen
    with(settings) {
        javaScriptEnabled = true
        domStorageEnabled = true
        mediaPlaybackRequiresUserGesture = false
        allowFileAccess = false
        allowContentAccess = false
        setGeolocationEnabled(false)
        setSupportMultipleWindows(false)
    }
    // WebRTC needs one of these to exist. Its default refuses every permission a page asks
    // for, and the player only receives a stream, so it never needs one granted.
    webChromeClient = WebChromeClient()
}

// The sign-in reaches the page as the server's own cookie, set only for that server's address,
// and is gone from the web view's storage when this screen closes. On plain http a browser hides
// game controllers from a page, and the Gamepad API here has never been tried on a real device.
@Composable
fun WebPlayerScreen(nav: Nav, route: Route.Web) {
    val context = LocalContext.current
    val api = nav.app.api
    var failed by remember { mutableStateOf("") }
    // Some devices have a missing or broken web view, and creating one is where that shows.
    val web = remember { runCatching { playerWebView(context) }.getOrNull() }

    DisposableEffect(web) {
        if (web == null) {
            failed = NO_WEB_VIEW
            return@DisposableEffect onDispose { }
        }
        var gone = false
        val window = (context as? Activity)?.window
        val bars = window?.let { WindowCompat.getInsetsController(it, web) }
        bars?.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        bars?.hide(WindowInsetsCompat.Type.systemBars())
        web.webViewClient = PlayerClient(api.base, onLeave = { if (nav.current == route) nav.pop() }, onFail = { failed = CANT_OPEN })

        val cookies = CookieManager.getInstance()
        val cookie = sessionCookie(api.token, api.base.startsWith("https:", ignoreCase = true))
        if (cookie == null) {
            failed = CANT_OPEN
        } else {
            // One left by a run that crashed could be for another server, so that goes first.
            cookies.removeAllCookies {
                cookies.setCookie(api.base, cookie) {
                    if (!gone) web.loadUrl(api.mediaUrl(route.path))
                }
            }
        }
        web.requestFocus()

        onDispose {
            gone = true
            bars?.show(WindowInsetsCompat.Type.systemBars())
            web.stopLoading()
            web.destroy() // this closes the stream's connection too, which ends it on the server
            cookies.removeAllCookies(null)
        }
    }

    BackHandler {
        if (failed.isEmpty() && web != null && web.canGoBack()) web.goBack() else nav.pop()
    }

    Screen {
        if (failed.isNotEmpty() || web == null) {
            Column(Modifier.padding(32.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                ErrorText(failed)
                ActionButton("Back", nav::pop, requestFocus = true)
            }
        } else {
            AndroidView(factory = { web }, modifier = Modifier.fillMaxSize().imePadding())
        }
    }
}
