package com.wittywilla.wittymediaclient.ui

import android.os.Build
import android.view.KeyEvent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import com.wittywilla.wittymediaclient.data.SavedServer
import com.wittywilla.wittymediaclient.net.Address
import com.wittywilla.wittymediaclient.net.ApiError
import com.wittywilla.wittymediaclient.net.Probe
import com.wittywilla.wittymediaclient.net.Profile
import com.wittywilla.wittymediaclient.net.Profiles
import com.wittywilla.wittymediaclient.net.probeServer
import kotlinx.coroutines.launch

private fun reasonText(s: Map<String, String>, reason: String) = when (reason) {
    "empty" -> s["errEmpty"]
    "scheme" -> s["errScheme"]
    "credentials" -> s["errCredentials"]
    "public-http" -> s["errPublicHttp"]
    "unreachable" -> s["errUnreachable"]
    "not-server" -> s["errNotServer"]
    else -> s["errInvalid"]
}.orEmpty()

// A text box a remote can leave: down moves on instead of being swallowed.
@Composable
private fun Field(
    value: String,
    onChange: (String) -> Unit,
    label: String,
    modifier: Modifier = Modifier,
    password: Boolean = false,
    keyboard: KeyboardType = KeyboardType.Text,
    onDone: () -> Unit,
) {
    val focus = LocalFocusManager.current
    OutlinedTextField(
        value = value,
        onValueChange = onChange,
        label = { Text(label) },
        singleLine = true,
        visualTransformation = if (password) PasswordVisualTransformation() else androidx.compose.ui.text.input.VisualTransformation.None,
        keyboardOptions = KeyboardOptions(keyboardType = if (password) KeyboardType.Password else keyboard, imeAction = ImeAction.Go, autoCorrectEnabled = false),
        keyboardActions = KeyboardActions(onGo = { onDone() }),
        modifier = modifier.fillMaxWidth().onPreviewKeyEvent {
            if (it.type == KeyEventType.KeyDown && it.key.nativeKeyCode == KeyEvent.KEYCODE_DPAD_DOWN) {
                focus.moveFocus(FocusDirection.Down)
            } else if (it.type == KeyEventType.KeyDown && it.key.nativeKeyCode == KeyEvent.KEYCODE_DPAD_UP) {
                focus.moveFocus(FocusDirection.Up)
            } else {
                false
            }
        },
    )
}

@Composable
private fun Centred(content: @Composable () -> Unit) {
    Screen {
        Column(
            Modifier.fillMaxWidth().imePadding().verticalScroll(rememberScrollState()).padding(32.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Column(Modifier.widthIn(max = 560.dp).fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(12.dp)) { content() }
        }
    }
}

@Composable
fun ConnectScreen(nav: Nav, s: Map<String, String>, message: String) {
    val scope = rememberCoroutineScope()
    val recent = remember { nav.app.store.recent }
    var address by rememberSaveable { mutableStateOf("") }
    var error by rememberSaveable { mutableStateOf(message) }
    var busy by remember { mutableStateOf(false) }
    var showForm by rememberSaveable { mutableStateOf(recent.isEmpty()) }
    val fieldFocus = remember { FocusRequester() }

    fun connect(url: String) {
        if (busy) return
        busy = true
        scope.launch {
            when (val r = probeServer(url)) {
                is Probe.Found -> {
                    nav.openServer(SavedServer(url, r.status.brandName.ifEmpty { r.status.name }))
                    nav.app.api.resolveApiPath()
                    nav.themeId = r.status.theme
                    nav.reset(Route.SignIn)
                }
                is Probe.Failed -> {
                    error = reasonText(s, r.reason)
                    address = url
                    showForm = true
                }
            }
            busy = false
        }
    }

    fun submit() {
        when (val r = Address.normalize(address)) {
            is Address.Result.Ok -> connect(r.url)
            is Address.Result.Bad -> error = reasonText(s, r.reason)
        }
    }

    Centred {
        Heading(if (showForm) s["addressTitle"].orEmpty() else s["findTitle"].orEmpty())
        if (showForm) {
            LaunchedEffect(Unit) { runCatching { fieldFocus.requestFocus() } }
            Secondary(s["addressHint"].orEmpty())
            Field(address, { address = it; error = "" }, s["addressLabel"].orEmpty(), Modifier.focusRequester(fieldFocus), keyboard = KeyboardType.Uri, onDone = ::submit)
            ErrorText(error)
            ActionButton(if (busy) s["connecting"].orEmpty() else s["connect"].orEmpty(), ::submit, Modifier.fillMaxWidth(), primary = true, enabled = !busy)
            if (recent.isNotEmpty()) ActionButton(s["back"].orEmpty(), { showForm = false; error = "" }, Modifier.fillMaxWidth())
        } else {
            ErrorText(error)
            Secondary(s["recentServers"].orEmpty())
            recent.forEachIndexed { i, server ->
                FocusBox({ connect(server.url) }, Modifier.fillMaxWidth(), label = "${server.name}, ${server.url}", requestFocus = i == 0, filled = MaterialTheme.colorScheme.surfaceVariant) {
                    Column(Modifier.padding(horizontal = 18.dp, vertical = 11.dp)) {
                        Text(server.name, color = MaterialTheme.colorScheme.onSurface)
                        Secondary(server.url)
                    }
                }
            }
            ActionButton(s["enterAddress"].orEmpty(), { showForm = true }, Modifier.fillMaxWidth())
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun SignInScreen(nav: Nav, s: Map<String, String>) {
    val scope = rememberCoroutineScope()
    var profiles by remember { mutableStateOf<Profiles?>(null) }
    var chosen by remember { mutableStateOf<Profile?>(null) }
    var useForm by rememberSaveable { mutableStateOf(false) }
    var name by rememberSaveable { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var error by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    val passwordFocus = remember { FocusRequester() }

    LaunchedEffect(Unit) {
        profiles = runCatching { nav.app.api.profiles() }.getOrDefault(Profiles())
    }

    fun login(who: String, secret: String) {
        if (busy) return
        busy = true
        error = ""
        scope.launch {
            try {
                nav.signedIn(nav.app.api.login(who, secret, "${Build.MANUFACTURER} ${Build.MODEL}".take(80)).token)
            } catch (e: ApiError) {
                error = e.message.orEmpty()
            } catch (_: Exception) {
                error = s["errLost"].orEmpty()
            }
            busy = false
        }
    }

    val list = profiles
    Centred {
        when {
            list == null -> Loading()
            chosen != null -> {
                val who = chosen!!
                LaunchedEffect(who.id) { runCatching { passwordFocus.requestFocus() } }
                Heading(who.name)
                Field(password, { password = it }, "Password", Modifier.focusRequester(passwordFocus), password = true, onDone = { login(who.name, password) })
                ErrorText(error)
                ActionButton("Sign in", { login(who.name, password) }, Modifier.fillMaxWidth(), primary = true, enabled = !busy)
                ActionButton(s["back"].orEmpty(), { chosen = null; password = ""; error = "" }, Modifier.fillMaxWidth())
            }
            list.style == "profiles" && !useForm && list.profiles.isNotEmpty() -> {
                Heading("Who's watching?")
                ErrorText(error)
                FlowRow(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    list.profiles.forEachIndexed { i, p ->
                        ActionButton(p.name, { if (p.hasPassword) chosen = p else login(p.name, "") }, requestFocus = i == 0)
                    }
                }
                ActionButton("Sign in with another account", { useForm = true }, Modifier.fillMaxWidth())
                ActionButton(s["changeServer"].orEmpty(), nav::changeServer, Modifier.fillMaxWidth())
            }
            else -> {
                Heading("Sign in")
                Field(name, { name = it }, "Name", onDone = { runCatching { passwordFocus.requestFocus() } })
                Field(password, { password = it }, "Password", Modifier.focusRequester(passwordFocus), password = true, onDone = { login(name, password) })
                ErrorText(error)
                ActionButton("Sign in", { login(name, password) }, Modifier.fillMaxWidth(), primary = true, enabled = !busy)
                if (list.profiles.isNotEmpty()) ActionButton(s["back"].orEmpty(), { useForm = false; error = "" }, Modifier.fillMaxWidth())
                ActionButton(s["changeServer"].orEmpty(), nav::changeServer, Modifier.fillMaxWidth())
            }
        }
    }
}
