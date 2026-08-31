package com.viant.agently.android

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import com.viant.forgeandroid.runtime.ExecutionDef
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.ui.WindowContentView
import com.viant.forgeandroid.ui.ForgePresentationDensity
import com.viant.forgeandroid.ui.LocalForgePresentationDensity
import kotlinx.serialization.json.JsonPrimitive

@Composable
internal fun AutomationScreen(
    forgeRuntime: ForgeRuntime,
    onBack: () -> Unit
) {
    val windows by forgeRuntime.windows.collectAsState(initial = emptyList())
    val automationRoot = windows.lastOrNull { it.windowKey == AUTOMATION_WINDOW_KEY }
    val selectedWindow = windows.lastOrNull { it.windowKey in AUTOMATION_WINDOW_KEYS } ?: automationRoot
    val windowForm = automationRoot?.let { state -> forgeRuntime.windowContext(state.windowId).windowFormSignal() }
    val automationView by if (windowForm != null) {
        windowForm.flow.collectAsState(initial = windowForm.peek())
    } else {
        androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(emptyMap()) }
    }

    LaunchedEffect(automationRoot?.windowId) {
        if (automationRoot == null) {
            forgeRuntime.openWindow(
                windowKey = AUTOMATION_WINDOW_KEY,
                title = "Automation",
                inTab = true
            )
        }
    }

    if (selectedWindow != null) {
        CompositionLocalProvider(LocalForgePresentationDensity provides ForgePresentationDensity.Compact) {
            WindowContentView(
                runtime = forgeRuntime,
                windowId = selectedWindow.windowId,
                windowKey = selectedWindow.windowKey,
                modifier = Modifier.fillMaxSize(),
                showWindowHeader = true,
                canGoBack = true,
                onBack = {
                    if (selectedWindow.windowKey == AUTOMATION_HISTORY_WINDOW_KEY) {
                        forgeRuntime.closeWindow(selectedWindow.windowId)
                    } else if (automationView["automationView"]?.toString() == "editor") {
                        val context = forgeRuntime.windowContext(selectedWindow.windowId).contextOrNull("schedules")
                        forgeRuntime.execute(
                            ExecutionDef(
                                handler = "schedule.backToList",
                                state = mapOf("automationView" to JsonPrimitive("list"))
                            ),
                            context
                        )
                    } else {
                        forgeRuntime.closeWindow(selectedWindow.windowId)
                        onBack()
                    }
                }
            )
        }
    }
}

private const val AUTOMATION_WINDOW_KEY = "schedule"
private const val AUTOMATION_HISTORY_WINDOW_KEY = "schedule/history"
private val AUTOMATION_WINDOW_KEYS = setOf(AUTOMATION_WINDOW_KEY, AUTOMATION_HISTORY_WINDOW_KEY)
