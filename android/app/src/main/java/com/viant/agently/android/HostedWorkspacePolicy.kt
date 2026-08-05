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
    streamSnapshot: ConversationStreamSnapshot? = null
): HostedWorkspaceRestoreState? {
    return filterAgentlyHostedWorkspaceRestoreState(
        deriveHostedWorkspaceRestoreState(state, streamSnapshot)
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
