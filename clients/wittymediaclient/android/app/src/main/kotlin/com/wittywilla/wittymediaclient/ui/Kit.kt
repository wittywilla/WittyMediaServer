package com.wittywilla.wittymediaclient.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil3.compose.AsyncImage
import coil3.request.ImageRequest

private val Corner = RoundedCornerShape(8.dp)

// The one focusable surface in the app. Focus shows as a border, with no
// animation, so it stays smooth on a slow TV stick.
@Composable
fun FocusBox(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    label: String? = null,
    requestFocus: Boolean = false,
    enabled: Boolean = true,
    filled: Color = Color.Transparent,
    content: @Composable () -> Unit,
) {
    val interaction = remember { MutableInteractionSource() }
    val focused by interaction.collectIsFocusedAsState()
    val requester = remember { FocusRequester() }
    if (requestFocus) LaunchedEffect(Unit) { runCatching { requester.requestFocus() } }
    val ring = if (focused) MaterialTheme.colorScheme.onBackground else Color.Transparent
    Box(
        modifier
            .focusRequester(requester)
            .then(if (label != null) Modifier.semantics { contentDescription = label } else Modifier)
            .border(BorderStroke(3.dp, ring), Corner)
            .padding(3.dp)
            .clip(Corner)
            .background(filled)
            .clickable(interactionSource = interaction, indication = null, enabled = enabled, role = Role.Button, onClick = onClick),
    ) { content() }
}

@Composable
fun ActionButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, primary: Boolean = false, requestFocus: Boolean = false, enabled: Boolean = true) {
    val scheme = MaterialTheme.colorScheme
    FocusBox(onClick, modifier, requestFocus = requestFocus, enabled = enabled, filled = if (primary) scheme.primary else scheme.surfaceVariant) {
        Text(
            text,
            color = if (primary) scheme.onPrimary else scheme.onSurface,
            modifier = Modifier.padding(horizontal = 18.dp, vertical = 11.dp),
        )
    }
}

@Composable
fun Heading(text: String, modifier: Modifier = Modifier) {
    Text(text, color = MaterialTheme.colorScheme.onBackground, fontSize = 24.sp, fontWeight = FontWeight.SemiBold, modifier = modifier)
}

@Composable
fun Secondary(text: String, modifier: Modifier = Modifier, maxLines: Int = Int.MAX_VALUE) {
    Text(text, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = modifier, maxLines = maxLines, overflow = TextOverflow.Ellipsis)
}

@Composable
fun ErrorText(text: String, modifier: Modifier = Modifier) {
    if (text.isNotEmpty()) Text(text, color = MaterialTheme.colorScheme.error, modifier = modifier)
}

@Composable
fun Loading() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
}

@Composable
fun Screen(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    Box(modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) { content() }
}

// A poster tile. The picture is asked for at the size shown.
@Composable
fun PosterCard(
    title: String,
    subtitle: String,
    imageUrl: String,
    onClick: () -> Unit,
    width: Int,
    modifier: Modifier = Modifier,
    requestFocus: Boolean = false,
) {
    val context = LocalContext.current
    val density = context.resources.displayMetrics.density
    val pixelsWide = (width * density).toInt()
    val request = remember(imageUrl, pixelsWide) {
        ImageRequest.Builder(context).data(imageUrl).size(pixelsWide, pixelsWide * 3 / 2).build()
    }
    val label = if (subtitle.isEmpty()) title else "$title, $subtitle"
    FocusBox(onClick, modifier.width(width.dp), label = label, requestFocus = requestFocus) {
        Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            AsyncImage(
                model = request,
                contentDescription = null,
                contentScale = ContentScale.Crop,
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(Corner)
                    .background(MaterialTheme.colorScheme.surfaceVariant)
                    .aspectRatio(2f / 3f),
            )
            Text(title, color = MaterialTheme.colorScheme.onBackground, maxLines = 1, overflow = TextOverflow.Ellipsis, fontSize = 14.sp)
            if (subtitle.isNotEmpty()) Secondary(subtitle, maxLines = 1)
        }
    }
}
