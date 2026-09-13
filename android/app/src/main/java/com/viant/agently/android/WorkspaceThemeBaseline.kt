package com.viant.agently.android

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.serialization.json.*

/** Baseline adoption surface; default app appearance remains unchanged. */
@Composable
internal fun WorkspaceThemeBaseline(theme: WorkspaceTheme, preference: String, systemMode: String) {
    val mode = theme.effectiveMode(preference, systemMode)
    val tokens = theme.modes.getValue(mode)
    fun dimension(key: String) = tokens.getValue(key).jsonPrimitive.double.toFloat()
    fun color(key: String): Color {
        val hex = tokens.getValue(key).jsonPrimitive.content.drop(1)
        val value = hex.toLong(16)
        val argb = if (hex.length == 8) ((value and 255) shl 24) or (value shr 8) else 0xff000000L or value
        return Color(argb.toInt())
    }
    var text by remember { mutableStateOf("") }
    val shape = RoundedCornerShape(dimension("control.radius").dp)
    val base = if (mode == "dark") darkColorScheme() else lightColorScheme()
    MaterialTheme(colorScheme = base.copy(
        surface = color("surface"), onSurface = color("text"),
        primary = color("button.background"), onPrimary = color("button.foreground"),
    )) {
        Column(Modifier.background(color("surface")).padding(16.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Text(theme.label, color = color("text"), fontSize = dimension("typography.size").sp)
            OutlinedTextField(
                value = text, onValueChange = { text = it }, label = { Text("Customer name") },
                modifier = Modifier.heightIn(min = maxOf(48f, dimension("control.minHeight")).dp),
                shape = shape,
                textStyle = LocalTextStyle.current.copy(fontSize = dimension("typography.size").sp),
                colors = OutlinedTextFieldDefaults.colors(
                    focusedTextColor = color("control.foreground"), unfocusedTextColor = color("control.foreground"),
                    focusedContainerColor = color("control.background"), unfocusedContainerColor = color("control.background"),
                    focusedBorderColor = color("focus.color"), unfocusedBorderColor = color("control.border"),
                ),
            )
            Button(onClick = { text = "" }, shape = shape,
                modifier = Modifier.heightIn(min = maxOf(48f, dimension("control.minHeight")).dp),
                contentPadding = PaddingValues(horizontal = dimension("control.paddingInline").dp, vertical = 8.dp),
            ) { Text("Clear", fontSize = dimension("typography.size").sp) }
        }
    }
}
