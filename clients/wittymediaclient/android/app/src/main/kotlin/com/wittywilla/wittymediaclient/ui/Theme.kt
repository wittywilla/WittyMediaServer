package com.wittywilla.wittymediaclient.ui

import android.content.Context
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.core.graphics.toColorInt
import com.wittywilla.wittymediaclient.net.json
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive

// Themes and wording are shared data: see shared/data/, bundled as assets.

@Serializable
data class ThemeColours(
    val background: String,
    val panel: String,
    val card: String,
    val border: String,
    val text: String,
    val textSecondary: String,
    val accent: String,
    val onAccent: String,
    val danger: String,
)

@Serializable
data class ThemeData(val id: String, val scheme: String, val colours: ThemeColours)

@Serializable
data class ThemeFile(val defaultTheme: String, val themes: List<ThemeData>)

object Assets {
    private fun read(context: Context, name: String) = context.assets.open(name).bufferedReader().use { it.readText() }

    fun themes(context: Context): ThemeFile = json.decodeFromString(read(context, "themes.json"))

    fun strings(context: Context): Map<String, String> =
        json.decodeFromString<JsonObject>(read(context, "strings.en.json")).mapValues { it.value.jsonPrimitive.content }
}

private fun c(hex: String) = Color(hex.toColorInt())

// A theme uploaded to the server has no entry here, and gets the default look.
fun pickTheme(file: ThemeFile, id: String): ThemeData =
    file.themes.firstOrNull { it.id == id } ?: file.themes.first { it.id == file.defaultTheme }

@Composable
fun WittyTheme(theme: ThemeData, content: @Composable () -> Unit) {
    val t = theme.colours
    val scheme = if (theme.scheme == "light") {
        lightColorScheme(
            primary = c(t.accent), onPrimary = c(t.onAccent), background = c(t.background), onBackground = c(t.text),
            surface = c(t.panel), onSurface = c(t.text), surfaceVariant = c(t.card), onSurfaceVariant = c(t.textSecondary),
            outline = c(t.border), error = c(t.danger),
        )
    } else {
        darkColorScheme(
            primary = c(t.accent), onPrimary = c(t.onAccent), background = c(t.background), onBackground = c(t.text),
            surface = c(t.panel), onSurface = c(t.text), surfaceVariant = c(t.card), onSurfaceVariant = c(t.textSecondary),
            outline = c(t.border), error = c(t.danger),
        )
    }
    MaterialTheme(colorScheme = scheme, content = content)
}
