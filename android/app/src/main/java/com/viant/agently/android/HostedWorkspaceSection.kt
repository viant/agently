package com.viant.agently.android
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.ui.Alignment
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.HostedWorkspaceRestoreState
import com.viant.agentlysdk.WorkspaceWindowSnapshot
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.WindowContext
import com.viant.forgeandroid.runtime.WindowMetadata
import com.viant.forgeandroid.ui.ContainerRenderer
import com.viant.forgeandroid.ui.WindowContentView
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.longOrNull

internal data class HostedWorkspaceWindowUiState(
    val metadata: WindowMetadata?,
    val windowContext: WindowContext?,
    val selectedWindowId: String,
    val windows: List<WorkspaceWindowSnapshot>,
    val error: String? = null
)

@Composable
internal fun HostedWorkspaceSection(
    restoreState: HostedWorkspaceRestoreState?,
    forgeRuntime: ForgeRuntime,
    modifier: Modifier = Modifier,
    maxBodyHeight: androidx.compose.ui.unit.Dp = 420.dp,
    showTitle: Boolean = true,
    headerActions: (@Composable () -> Unit)? = null
) {
    val resolvedRestoreState = restoreState ?: return
    var selectedWindowId by remember(resolvedRestoreState.selectionKey()) {
        mutableStateOf(defaultHostedWorkspaceWindowId(resolvedRestoreState))
    }
    val windowState = rememberHostedWorkspaceWindowUiState(
        resolvedRestoreState,
        forgeRuntime,
        selectedWindowId
    )
    val minBodyHeight = minOf(
        (
            windowState.windows.firstOrNull { it.windowId == windowState.selectedWindowId }?.workspaceMinHeight
                ?: 320
        ).coerceAtLeast(240).dp,
        maxBodyHeight
    )

    Card(
        modifier = modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = Color(0xFFF6F8FD))
    ) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            if (showTitle) {
                Box(
                    modifier = Modifier.fillMaxWidth(),
                    contentAlignment = Alignment.Center
                ) {
                    Box(modifier = Modifier.align(Alignment.CenterStart)) {
                        headerActions?.invoke()
                    }
                    Text("Workspace", style = MaterialTheme.typography.titleMedium)
                }
            }
            if (windowState.windows.size > 1) {
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    windowState.windows.forEach { snapshot ->
                        FilterChip(
                            selected = snapshot.windowId == windowState.selectedWindowId,
                            onClick = { selectedWindowId = snapshot.windowId },
                            label = {
                                Text(
                                    snapshot.windowTitle?.takeIf { it.isNotBlank() }
                                        ?: snapshot.windowKey
                                )
                            }
                        )
                    }
                }
            }
            when {
                windowState.error != null -> Text(
                    windowState.error,
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFFB42318)
                )
                windowState.windowContext == null -> Text(
                    "Loading workspace view…",
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFF667085)
                )
                else -> WindowContentView(
                    runtime = forgeRuntime,
                    windowId = windowState.windowContext.windowId,
                    windowKey = windowState.windows.firstOrNull { it.windowId == windowState.selectedWindowId }?.windowKey
                        ?: windowState.selectedWindowId,
                    showWindowHeader = false,
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(min = minBodyHeight, max = maxBodyHeight)
                )
            }
        }
    }
}

@Composable
internal fun rememberHostedWorkspaceWindowUiState(
    restoreState: HostedWorkspaceRestoreState,
    forgeRuntime: ForgeRuntime,
    selectedWindowId: String = defaultHostedWorkspaceWindowId(restoreState)
): HostedWorkspaceWindowUiState {
    val selected = remember(restoreState.windows, selectedWindowId) {
        restoreState.windows.firstOrNull { it.windowId == selectedWindowId }
            ?: restoreState.windows.lastOrNull()
    }
    if (selected == null) {
        return HostedWorkspaceWindowUiState(
            metadata = null,
            windowContext = null,
            selectedWindowId = "",
            windows = emptyList(),
            error = "No hosted workspace window is available."
        )
    }

    var runtimeWindowId by remember(selected.windowId) { mutableStateOf<String?>(null) }
    var loadError by remember(selected.windowId) { mutableStateOf<String?>(null) }

    LaunchedEffect(selected.windowId) {
        try {
            val state = openHostedWorkspaceWindow(forgeRuntime, selected)
            selected.windowForm?.let(::jsonObjectToParameterMap)?.takeIf { it.isNotEmpty() }?.let { windowForm ->
                forgeRuntime.setWindowFormValues(
                    windowId = state.windowId,
                    values = windowForm,
                    replace = true,
                    bumpPrefillRevision = false
                )
            }
            runtimeWindowId = state.windowId
            loadError = null
        } catch (err: Throwable) {
            runtimeWindowId = null
            loadError = err.message ?: "Unable to open hosted workspace."
        }
    }

    val metadataSignal = remember(runtimeWindowId) {
        runtimeWindowId?.let { forgeRuntime.metadataSignal(it) }
    }
    val resolvedMetadata by if (metadataSignal != null) {
        metadataSignal.flow.collectAsState(initial = metadataSignal.peek())
    } else {
        remember { mutableStateOf<WindowMetadata?>(null) }
    }
    val windowContext = remember(runtimeWindowId) {
        runtimeWindowId?.let { forgeRuntime.windowContext(it) }
    }

    return HostedWorkspaceWindowUiState(
        metadata = resolvedMetadata,
        windowContext = windowContext,
        selectedWindowId = selected.windowId,
        windows = restoreState.windows,
        error = loadError
    )
}

internal fun defaultHostedWorkspaceWindowId(
    restoreState: HostedWorkspaceRestoreState
): String {
    return restoreState.selectedWindowId
        ?: restoreState.windows.lastOrNull()?.windowId
        ?: ""
}

private fun HostedWorkspaceRestoreState.selectionKey(): String {
    val selected = selectedWindowId.orEmpty()
    val windowsKey = windows.joinToString("|") { it.windowId }
    return "$selected#$windowsKey"
}

internal fun openHostedWorkspaceWindow(
    forgeRuntime: ForgeRuntime,
    snapshot: WorkspaceWindowSnapshot
) = forgeRuntime.openWindow(
    windowKey = snapshot.windowKey,
    title = snapshot.windowTitle?.takeIf { it.isNotBlank() } ?: snapshot.windowKey,
    inTab = snapshot.inTab != false,
    parameters = snapshot.parameters?.let(::jsonObjectToParameterMap).orEmpty(),
    windowIdOverride = snapshot.windowId,
    conversationId = snapshot.conversationId,
    presentation = snapshot.presentation,
    region = snapshot.region,
    workspaceSharePct = snapshot.workspaceSharePct,
    workspaceMinHeight = snapshot.workspaceMinHeight,
    parentKey = snapshot.parentKey
)

private fun jsonObjectToParameterMap(value: JsonObject): Map<String, Any?> {
    return value.mapValues { (_, entry) -> jsonElementToAny(entry) }
}

private fun jsonElementToAny(value: JsonElement): Any? {
    return when (value) {
        is JsonObject -> value.mapValues { (_, entry) -> jsonElementToAny(entry) }
        is JsonArray -> value.map { jsonElementToAny(it) }
        is JsonPrimitive -> {
            if (value.isString) {
                value.content
            } else {
                value.booleanOrNull ?: value.longOrNull ?: value.doubleOrNull ?: value.contentOrNull
            }
        }
        else -> null
    }
}

private fun jsonString(value: JsonElement?): String {
    return (value as? JsonPrimitive)?.contentOrNull?.trim().orEmpty()
}

private fun jsonBoolean(value: JsonElement): Boolean? {
    return (value as? JsonPrimitive)?.booleanOrNull
}

private fun jsonInt(value: JsonElement): Int? {
    val primitive = value as? JsonPrimitive ?: return null
    return primitive.contentOrNull?.trim()?.toIntOrNull()
}
