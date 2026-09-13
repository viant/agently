package com.viant.agently.android

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import androidx.core.view.WindowCompat
import androidx.compose.ui.platform.LocalView
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.viant.forgeandroid.ui.ForgeThemePreview
import com.viant.forgeandroid.ui.ForgeThemeAppearance
import com.viant.forgeandroid.ui.LocalForgeThemeAppearance
import kotlinx.serialization.json.*

internal val LocalWorkspaceThemeRuntime = staticCompositionLocalOf<WorkspaceThemeRuntime?> { null }
internal fun forgeThemeAppearance(tokens: JsonObject?): ForgeThemeAppearance? {
    if (tokens == null) return null
    fun color(key: String): Color {
        val hex = tokens.getValue(key).jsonPrimitive.content.drop(1)
        val rgba = hex.toLong(16)
        val argb = if (hex.length == 8) ((rgba and 255) shl 24) or (rgba shr 8) else 0xff000000L or rgba
        return Color(argb.toInt())
    }
    fun dimension(key: String) = tokens.getValue(key).jsonPrimitive.double.toFloat()
    return ForgeThemeAppearance(color("surface"), color("text"), color("control.background"), color("control.foreground"),
        color("control.border"), color("focus.color"), color("button.background"), color("button.foreground"),
        color("disabled.background"), color("disabled.foreground"), color("validation.border"),
        dimension("typography.size"), dimension("control.minHeight"), dimension("control.radius"), dimension("control.paddingInline"))
}
@Composable
internal fun WorkspaceThemeHost(runtime: WorkspaceThemeRuntime, content: @Composable () -> Unit) {
    val state by runtime.state.collectAsState()
    val systemMode = if (isSystemInDarkTheme()) "dark" else "light"
    val view = LocalView.current
    val dark = state.effectiveMode(systemMode) == "dark"
    SideEffect {
        view.context.themeActivity()?.window?.let { window ->
            WindowCompat.getInsetsController(window, view).apply {
                isAppearanceLightStatusBars = !dark
                isAppearanceLightNavigationBars = !dark
            }
        }
    }
    val appearance = remember(state, systemMode) { forgeThemeAppearance(state.tokens(systemMode)) }
    AgentlyTheme(darkTheme = dark, appearance = appearance) {
        CompositionLocalProvider(LocalWorkspaceThemeRuntime provides runtime, LocalForgeThemeAppearance provides appearance, content = content)
    }
}
@Composable
internal fun WorkspaceThemeSettings(onRefresh: () -> Unit) {
    val runtime = LocalWorkspaceThemeRuntime.current ?: return
    val state by runtime.state.collectAsState()
    val systemMode = if (isSystemInDarkTheme()) "dark" else "light"
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Workspace appearance", style = MaterialTheme.typography.titleMedium)
            if (state.catalog != null) {
                var expanded by remember { mutableStateOf(false) }
                Box {
                    OutlinedButton(onClick = { expanded = true }) { Text("Theme: ${state.selectedTheme?.label ?: "Default"}") }
                    DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                        DropdownMenuItem(text = { Text("Default") }, onClick = { runtime.select("", "system"); expanded = false })
                        state.catalog!!.themes.forEach { theme -> DropdownMenuItem(text = { Text(theme.label) }, onClick = { runtime.select(theme.id, state.modePreference); expanded = false }) }
                    }
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    listOf("system", "light", "dark").forEach { mode ->
                        FilterChip(selected = state.modePreference == mode, onClick = { runtime.select(state.themeId, mode) },
                            enabled = state.selectedTheme != null && (mode == "system" || state.selectedTheme!!.modes.containsKey(mode)),
                            label = { Text(mode.replaceFirstChar { it.uppercase() }) })
                    }
                }
                Text("Currently ${state.effectiveMode(systemMode)}", style = MaterialTheme.typography.bodySmall)
            } else Text("No workspace theme is loaded.")
            if (state.catalog != null) ForgeThemePreview()
            state.diagnostic?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
            TextButton(onClick = onRefresh) { Text("Reload appearance") }
        }
    }
}

private tailrec fun Context.themeActivity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.themeActivity()
    else -> null
}
