package com.viant.agently.android

import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.HostedWorkspaceRestoreState
import com.viant.agentlysdk.WorkspaceWindowSnapshot
import com.viant.agentlysdk.deriveHostedWorkspaceRestoreState
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject

internal fun deriveAgentlyHostedWorkspaceRestoreState(
    state: ConversationStateResponse?,
    streamSnapshot: ConversationStreamSnapshot? = null,
    localSnapshot: NativeUIBridgeSnapshot? = null
): HostedWorkspaceRestoreState? {
    val durable = filterAgentlyHostedWorkspaceRestoreState(
        state?.let(::deriveHostedWorkspaceRestoreState)
    )
    val live = filterAgentlyHostedWorkspaceRestoreState(
        streamSnapshot
            ?.takeIf { !it.activeTurnId.isNullOrBlank() }
            ?.let(::deriveHostedWorkspaceRestoreState)
    )
    val transcript = mergeHostedWorkspaceRestoreStates(durable, live)
    val local = deriveAgentlyHostedWorkspaceRestoreState(localSnapshot)
    return mergeHostedWorkspaceRestoreStates(transcript, local)
}

private fun mergeHostedWorkspaceRestoreStates(
    transcript: HostedWorkspaceRestoreState?,
    local: HostedWorkspaceRestoreState?
): HostedWorkspaceRestoreState? {
    if (transcript == null) return local
    if (local == null) return transcript
    val localById = local.windows.associateBy { it.windowId }
    val merged = transcript.windows.map { durable ->
        val live = localById[durable.windowId] ?: return@map durable
        durable.copy(
            conversationId = durable.conversationId ?: live.conversationId,
            windowTitle = durable.windowTitle ?: live.windowTitle,
            presentation = durable.presentation ?: live.presentation,
            region = durable.region ?: live.region,
            parentKey = durable.parentKey ?: live.parentKey,
            workspaceSharePct = durable.workspaceSharePct ?: live.workspaceSharePct,
            workspaceMinHeight = durable.workspaceMinHeight ?: live.workspaceMinHeight,
            inTab = durable.inTab ?: live.inTab,
            parameters = mergeHostedWorkspaceJson(live.parameters, durable.parameters),
            windowForm = mergeHostedWorkspaceWindowForm(durable.windowForm, live.windowForm)
        )
    }
    val selected = local.selectedWindowId
        ?.takeIf { candidate -> merged.any { it.windowId == candidate } }
        ?: transcript.selectedWindowId
        ?: merged.lastOrNull()?.windowId
    return HostedWorkspaceRestoreState(merged, selected)
}

private fun mergeHostedWorkspaceWindowForm(
    durable: JsonObject?,
    live: JsonObject?
): JsonObject? {
    val durableForm = sanitizeOrphanedReportMaterialization(durable)
    val liveForm = sanitizeOrphanedReportMaterialization(live)
    if (durableForm == null || durableForm.isEmpty()) return liveForm
    if (liveForm == null || liveForm.isEmpty()) return durableForm
    // reportBuilder:* is renderer-derived state. When a durable authored
    // reportDefinition exists, a just-opened local window may already contain
    // the default builder state; retaining it prevents the authored definition
    // from being projected. The conversation transcript is authoritative.
    val sanitizedLive = if (durableForm["reportDefinition"] is JsonObject) {
        JsonObject(liveForm.filterKeys { !it.startsWith("reportBuilder:") })
    } else {
        liveForm
    }
    return mergeHostedWorkspaceJson(sanitizedLive, durableForm)
}

private fun sanitizeOrphanedReportMaterialization(form: JsonObject?): JsonObject? {
    form ?: return null
    val sanitized = form.toMutableMap().apply { remove("reportRunRequest") }
    val materialization = sanitized["reportMaterialization"] as? JsonObject ?: return JsonObject(sanitized)
    val status = (materialization["status"] as? kotlinx.serialization.json.JsonPrimitive)
        ?.content?.trim()?.lowercase().orEmpty()
    if (status == "running") sanitized.remove("reportMaterialization")
    return JsonObject(sanitized)
}

private fun mergeHostedWorkspaceJson(base: JsonObject?, overlay: JsonObject?): JsonObject? {
    if (overlay == null || overlay.isEmpty()) return base
    if (base == null || base.isEmpty()) return overlay
    val merged = base.toMutableMap()
    overlay.forEach { (key, value) ->
        val baseObject = merged[key] as? JsonObject
        val overlayObject = value as? JsonObject
        merged[key] = when {
            value is kotlinx.serialization.json.JsonPrimitive && value.isString && value.content == "[MaxDepth]" -> merged[key] ?: value
            baseObject != null && overlayObject != null -> mergeHostedWorkspaceJson(baseObject, overlayObject) ?: overlayObject
            else -> value
        }
    }
    return JsonObject(merged)
}

internal fun deriveAgentlyHostedWorkspaceRestoreState(
    localSnapshot: NativeUIBridgeSnapshot?
): HostedWorkspaceRestoreState? {
    localSnapshot ?: return null
    val conversationId = localSnapshot.conversationId?.trim().orEmpty()
    if (conversationId.isEmpty()) {
        return null
    }
    val windows = localSnapshot.windows
        .asSequence()
        .filterNot { it.isModal || it.windowKey == "chat/new" }
        .filter { window ->
            val windowConversationId = window.conversationId?.trim().orEmpty()
            conversationId.isEmpty() || windowConversationId.isEmpty() || windowConversationId == conversationId
        }
        .map { window ->
            WorkspaceWindowSnapshot(
                windowId = window.windowId,
                conversationId = window.conversationId,
                windowKey = window.windowKey,
                windowTitle = window.windowTitle,
                presentation = window.presentation,
                region = window.region,
                parentKey = window.parentKey,
                workspaceSharePct = window.workspaceSharePct,
                workspaceMinHeight = window.workspaceMinHeight,
                inTab = window.inTab,
                parameters = window.parameters,
                windowForm = window.windowForm
            )
        }
        .toList()
    return filterAgentlyHostedWorkspaceRestoreState(
        HostedWorkspaceRestoreState(
            windows = windows,
            selectedWindowId = windows.lastOrNull()?.windowId
        )
    )
}

internal fun deriveAgentlyHostedWorkspaceRestoreState(
    state: ConversationStateResponse
): HostedWorkspaceRestoreState? {
    return filterAgentlyHostedWorkspaceRestoreState(
        deriveHostedWorkspaceRestoreState(state)
    )
}

internal fun filterAgentlyHostedWorkspaceRestoreState(
    restoreState: HostedWorkspaceRestoreState?
): HostedWorkspaceRestoreState? {
    restoreState ?: return null
    val windows = restoreState.windows
        .filter(::isAgentlyHostedWorkspaceWindow)
        .map(::seedHostedWorkspaceWindowForm)
    if (windows.isEmpty()) {
        return null
    }
    val selectedWindowId = restoreState.selectedWindowId
        ?.takeIf { selected -> windows.any { it.windowId == selected } }
        ?: windows.last().windowId
    return HostedWorkspaceRestoreState(
        windows = windows,
        selectedWindowId = selectedWindowId
    )
}

private fun isAgentlyHostedWorkspaceWindow(window: WorkspaceWindowSnapshot): Boolean {
    return window.presentation?.trim()?.lowercase() == "hosted" &&
        window.region?.trim()?.lowercase() == "chat.top" &&
        window.parentKey?.trim() == "chat/new"
}

private fun seedHostedWorkspaceWindowForm(window: WorkspaceWindowSnapshot): WorkspaceWindowSnapshot {
    val parameters = window.parameters ?: return window
    if (parameters.isEmpty()) {
        return window
    }
    val seeded = mergeHostedWorkspaceJsonObjects(
        base = parameters,
        overlay = window.windowForm ?: JsonObject(emptyMap())
    )
    return window.copy(windowForm = seeded)
}

private fun mergeHostedWorkspaceJsonObjects(
    base: JsonObject,
    overlay: JsonObject
): JsonObject {
    val merged = LinkedHashMap<String, JsonElement>()
    merged.putAll(base)
    overlay.forEach { (key, value) ->
        val baseObject = merged[key] as? JsonObject
        val overlayObject = value as? JsonObject
        merged[key] = if (baseObject != null && overlayObject != null) {
            mergeHostedWorkspaceJsonObjects(baseObject, overlayObject)
        } else {
            value
        }
    }
    return JsonObject(merged)
}
