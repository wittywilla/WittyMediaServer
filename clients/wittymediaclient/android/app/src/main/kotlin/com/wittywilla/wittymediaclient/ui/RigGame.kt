package com.wittywilla.wittymediaclient.ui

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.wittywilla.wittymediaclient.net.ApiError
import com.wittywilla.wittymediaclient.net.Item
import com.wittywilla.wittymediaclient.net.MoonlightStart
import com.wittywilla.wittymediaclient.net.RigChoice
import com.wittywilla.wittymediaclient.net.RigInstall
import com.wittywilla.wittymediaclient.net.RigPlay
import com.wittywilla.wittymediaclient.net.RigPlayRequest
import com.wittywilla.wittymediaclient.net.isWebLink
import com.wittywilla.wittymediaclient.net.moonlightStoreLinks
import com.wittywilla.wittymediaclient.net.rigChoice
import com.wittywilla.wittymediaclient.net.rigOsName
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

// Its own task, or coming back to this app from the launcher would close Moonlight
// along with everything above this app's only screen.
private fun openMoonlight(context: Context, start: MoonlightStart): Boolean = try {
    val intent = Intent().setClassName(start.packageName, start.activity).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    start.extras.forEach { (name, value) -> intent.putExtra(name, value) }
    context.startActivity(intent)
    true
} catch (_: ActivityNotFoundException) {
    false
} catch (_: SecurityException) {
    false // a version from before it let others start games
}

private fun openLink(context: Context, url: String): Boolean = try {
    context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    true
} catch (_: ActivityNotFoundException) {
    false
}

private fun missingMoonlight(install: RigInstall?, storeOpened: Boolean): String {
    val name = install?.name?.ifEmpty { null } ?: "Moonlight"
    return if (storeOpened) "Moonlight isn't on this device, or is too old. Get $name from the store, then press Play again."
    else "Moonlight isn't on this device, or is too old, and the store couldn't be opened. Search this device's store for $name, then press Play again."
}

// A game on someone's own PC. Play asks the server what this device can do, and hands over
// to Moonlight where that's possible. Nothing here is ever disabled, because a disabled
// button loses the focus and leaves a remote with nowhere to go.
@Composable
fun RigPlayPanel(nav: Nav, item: Item) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val app = nav.app
    val isTv = app.isTv || app.isFireTv
    var play by remember(item.id) { mutableStateOf<RigPlay?>(null) }
    var said by remember(item.id) { mutableStateOf("") }
    var error by remember(item.id) { mutableStateOf("") }
    var busy by remember(item.id) { mutableStateOf(false) }
    var missing by remember(item.id) { mutableStateOf(false) }
    var showSetup by remember(item.id) { mutableStateOf(false) }

    fun getMoonlight(install: RigInstall?) {
        val opened = moonlightStoreLinks(app.isFireTv, install).any { openLink(context, it) }
        missing = true
        said = missingMoonlight(install, opened)
    }

    fun work(job: suspend () -> Unit) {
        if (busy) return
        busy = true
        error = ""
        said = ""
        missing = false
        scope.launch {
            try {
                job()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (!nav.handleExpired(e)) error = (e as? ApiError)?.message ?: "The server couldn't be reached. Check it's on, then try again."
            } finally {
                busy = false
            }
        }
    }

    fun ask() = work {
        val answer = app.api.rigPlay(item.id, RigPlayRequest("android", rigOsName(app.isTv, app.isFireTv)))
        play = answer
        val choice = if (answer.ready) rigChoice(answer, isTv) else null
        if (choice is RigChoice.Open && !openMoonlight(context, choice.start)) getMoonlight(answer.install)
    }

    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        ActionButton(if (busy) "One moment" else "Play", { ask() }, primary = true, requestFocus = true)
        ErrorText(error)
        if (said.isNotEmpty()) Secondary(said)
        val p = play
        if (p != null) {
            if (p.message.isNotEmpty()) Secondary(p.message)
            if (!p.ready) {
                if (p.rig.canWake) {
                    ActionButton("Wake ${p.rig.name}", { work { said = app.api.rigWake(p.rig.id).note.ifEmpty { "A wake-up signal was sent." } } })
                }
            } else {
                when (val choice = rigChoice(p, isTv)) {
                    is RigChoice.Open -> {
                        if (choice.note.isNotEmpty()) Secondary(choice.note)
                        if (missing) ActionButton("Get Moonlight", { getMoonlight(p.install) })
                    }
                    is RigChoice.Browser -> {
                        ActionButton("Open in your browser", {
                            if (isWebLink(choice.url) && !openLink(context, choice.url)) said = "This device has no browser to open it in."
                        })
                        if (choice.note.isNotEmpty()) Secondary(choice.note)
                    }
                    is RigChoice.Steps -> choice.steps.forEach { Secondary(it) }
                    RigChoice.None -> if (p.message.isEmpty()) Secondary("Nothing on this device can reach this PC from here.")
                }
                if (missing || rigChoice(p, isTv) is RigChoice.Steps) p.install?.let { install ->
                    val license = if (install.license.isEmpty()) "" else " (${install.license})"
                    Secondary("${install.name} is a separate, free app$license. ${install.note}".trim())
                }
                if (p.setup.isNotEmpty()) {
                    ActionButton(if (showSetup) "Hide the setup steps" else "How to set up Moonlight", { showSetup = !showSetup })
                    if (showSetup) p.setup.forEach { Secondary(it) }
                }
            }
        }
    }
}
