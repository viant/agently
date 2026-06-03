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
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.HostedWorkspaceRestoreState
import com.viant.agentlysdk.WorkspaceWindowSnapshot
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.WindowContext
import com.viant.forgeandroid.runtime.WindowMetadata
import com.viant.forgeandroid.ui.ContainerRenderer
import com.viant.forgeandroid.ui.WindowContentView
import kotlinx.serialization.json.Json
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
    conversationState: ConversationStateResponse?,
    forgeRuntime: ForgeRuntime,
    modifier: Modifier = Modifier,
    maxBodyHeight: androidx.compose.ui.unit.Dp = 420.dp,
    showTitle: Boolean = true,
    headerActions: (@Composable () -> Unit)? = null
) {
    val restoreState = remember(conversationState) {
        conversationState?.let(::deriveHostedWorkspaceRestoreState)
    } ?: return
    val windowState = rememberHostedWorkspaceWindowUiState(restoreState, forgeRuntime)

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
                            onClick = { },
                            enabled = false,
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
                windowState.metadata == null || windowState.windowContext == null -> Text(
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
                        .heightIn(max = maxBodyHeight)
                )
            }
        }
    }
}

@Composable
internal fun rememberHostedWorkspaceWindowUiState(
    restoreState: HostedWorkspaceRestoreState,
    forgeRuntime: ForgeRuntime
): HostedWorkspaceWindowUiState {
    var selectedWindowId by remember(restoreState.selectedWindowId, restoreState.windows) {
        mutableStateOf(
            restoreState.selectedWindowId
                ?: restoreState.windows.lastOrNull()?.windowId
                ?: ""
        )
    }
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
                forgeRuntime.setWindowFormValues(state.windowId, windowForm, replace = true)
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
    parentKey = snapshot.parentKey
)

internal fun deriveHostedWorkspaceRestoreState(
    state: ConversationStateResponse
): HostedWorkspaceRestoreState? {
    val turns = state.conversation?.turns.orEmpty()
    val lastTurn = turns.lastOrNull() ?: return null
    val toolSteps = lastTurn.execution?.pages.orEmpty()
        .flatMap { it.toolSteps }
        .filter { it.status?.trim()?.lowercase() == "completed" }
    for (step in toolSteps.asReversed()) {
        when (normalizeToolName(step.toolName)) {
            "ui/window/list" -> {
                val windows = hostedWorkspaceWindowsFromListPayload(
                    firstParsedPayload(step.responsePayload, step.content)
                )
                if (windows.isNotEmpty()) {
                    return HostedWorkspaceRestoreState(
                        windows = windows,
                        selectedWindowId = selectedWindowIdFromToolSteps(toolSteps, windows)
                            .ifBlank { null }
                    )
                }
            }
            "ui/view/open" -> {
                val windows = hostedWorkspaceWindowsFromViewOpenStep(step)
                if (windows.isNotEmpty()) {
                    val response = firstParsedPayload(step.responsePayload, step.content) as? JsonObject
                    val selectedWindowId = jsonString(response?.get("selectedWindowId"))
                        .ifBlank { windows.lastOrNull()?.windowId.orEmpty() }
                    return HostedWorkspaceRestoreState(
                        windows = windows,
                        selectedWindowId = selectedWindowId.ifBlank { null }
                    )
                }
            }
        }
    }
    return null
}

private fun normalizeToolName(raw: String?): String {
    return raw.orEmpty().trim().lowercase().replace(":", "/")
}

private fun firstParsedPayload(rawPayload: JsonElement?, rawText: String?): JsonElement? {
    val candidates = listOfNotNull(rawPayload, rawText?.takeIf { it.isNotBlank() }?.let(::JsonPrimitive))
    candidates.forEach { candidate ->
        val parsed = parsePayload(candidate) ?: return@forEach
        if (!isPayloadEnvelope(parsed)) {
            return parsed
        }
    }
    return null
}

private fun parsePayload(raw: JsonElement): JsonElement? {
    return when (raw) {
        is JsonPrimitive -> {
            val text = raw.contentOrNull?.trim().orEmpty()
            if (text.isBlank()) {
                null
            } else {
                runCatching { Json.parseToJsonElement(text) }.getOrNull()
            }
        }
        is JsonObject -> {
            val inlineBody = raw["inlineBody"] ?: raw["InlineBody"]
            if (inlineBody is JsonPrimitive) {
                val text = inlineBody.contentOrNull?.trim().orEmpty()
                if (text.isNotBlank()) {
                    return runCatching { Json.parseToJsonElement(text) }.getOrNull() ?: raw
                }
            }
            raw
        }
        else -> raw
    }
}

private fun isPayloadEnvelope(value: JsonElement): Boolean {
    val obj = value as? JsonObject ?: return false
    val hasInlineBody = jsonString(obj["inlineBody"]).isNotBlank() || jsonString(obj["InlineBody"]).isNotBlank()
    val hasCompression = jsonString(obj["compression"]).isNotBlank() || jsonString(obj["Compression"]).isNotBlank()
    val hasDirectWorkspaceShape = obj["items"] != null || obj["windowId"] != null || obj["focusedWindowId"] != null
    return (hasInlineBody || hasCompression) && !hasDirectWorkspaceShape
}

private fun hostedWorkspaceWindowsFromListPayload(raw: JsonElement?): List<WorkspaceWindowSnapshot> {
    val payload = raw as? JsonObject ?: return emptyList()
    val items = payload["items"] as? JsonArray ?: return emptyList()
    return items.mapNotNull { normalizeHostedWorkspaceWindow(it as? JsonObject) }
}

private fun hostedWorkspaceWindowsFromViewOpenStep(
    step: com.viant.agentlysdk.ToolStepState
): List<WorkspaceWindowSnapshot> {
    val response = firstParsedPayload(step.responsePayload, step.content) as? JsonObject ?: return emptyList()
    val request = firstParsedPayload(step.requestPayload, null) as? JsonObject
    val items = response["items"] as? JsonArray
    if (items != null && items.isNotEmpty()) {
        return items.mapNotNull { normalizeHostedWorkspaceWindow(it as? JsonObject) }
    }
    val normalized = normalizeHostedWorkspaceWindow(
        JsonObject(
            buildMap {
                put("windowId", JsonPrimitive(jsonString(response["windowId"])))
                put("conversationId", JsonPrimitive(jsonString(response["conversationId"])))
                put("windowKey", JsonPrimitive(jsonString(response["windowKey"]).ifBlank { jsonString(request?.get("id")) }))
                put("windowTitle", JsonPrimitive(jsonString(response["windowTitle"])))
                put("presentation", JsonPrimitive(jsonString(response["presentation"])))
                put("region", JsonPrimitive(jsonString(response["region"])))
                put("parentKey", JsonPrimitive(jsonString(response["parentKey"])))
                request?.get("parameters")?.let { put("parameters", it) }
                response["windowForm"]?.let { put("windowForm", it) }
            }
        )
    )
    return listOfNotNull(normalized)
}

private fun normalizeHostedWorkspaceWindow(raw: JsonObject?): WorkspaceWindowSnapshot? {
    raw ?: return null
    val presentation = jsonString(raw["presentation"]).lowercase()
    val region = jsonString(raw["region"]).lowercase()
    val parentKey = jsonString(raw["parentKey"])
    val windowId = jsonString(raw["windowId"])
    val windowKey = jsonString(raw["windowKey"])
    if (windowId.isBlank() || windowKey.isBlank()) return null
    if (presentation != "hosted") return null
    if (region != "chat.top") return null
    if (parentKey != "chat/new") return null
    return WorkspaceWindowSnapshot(
        windowId = windowId,
        conversationId = jsonString(raw["conversationId"]).ifBlank { null },
        windowKey = windowKey,
        windowTitle = jsonString(raw["windowTitle"]).ifBlank { windowKey },
        presentation = jsonString(raw["presentation"]).ifBlank { null },
        region = jsonString(raw["region"]).ifBlank { null },
        parentKey = parentKey,
        inTab = raw["inTab"]?.let(::jsonBoolean) ?: true,
        parameters = raw["parameters"] as? JsonObject,
        windowForm = raw["windowForm"] as? JsonObject
    )
}

private fun selectedWindowIdFromToolSteps(
    toolSteps: List<com.viant.agentlysdk.ToolStepState>,
    windows: List<WorkspaceWindowSnapshot>
): String {
    val windowIds = windows.map { it.windowId }.toSet()
    toolSteps.asReversed().forEach { step ->
        when (normalizeToolName(step.toolName)) {
            "ui/window/show" -> {
                val request = firstParsedPayload(step.requestPayload, null) as? JsonObject
                val windowId = jsonString(request?.get("windowId"))
                if (windowId in windowIds) return windowId
            }
            "ui/window/list" -> {
                val response = firstParsedPayload(step.responsePayload, step.content) as? JsonObject
                val focused = jsonString(response?.get("focusedWindowId"))
                if (focused in windowIds) return focused
            }
        }
    }
    return ""
}

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
