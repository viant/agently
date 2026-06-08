package com.viant.agently.android

import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.HostedWorkspaceRestoreState
import com.viant.agentlysdk.WorkspaceWindowSnapshot
import com.viant.agentlysdk.deriveHostedWorkspaceRestoreState
import com.viant.agentlysdk.stream.ConversationStreamSnapshot

internal fun deriveAgentlyHostedWorkspaceRestoreState(
    state: ConversationStateResponse?,
    streamSnapshot: ConversationStreamSnapshot? = null
): HostedWorkspaceRestoreState? {
    val liveRestore = if (streamSnapshot?.activeTurnId?.trim().isNullOrEmpty()) {
        null
    } else {
        streamSnapshot?.let(::deriveHostedWorkspaceRestoreState)
    }
    if (liveRestore != null) {
        return filterAgentlyHostedWorkspaceRestoreState(liveRestore)
    }
    return filterAgentlyHostedWorkspaceRestoreState(
        state?.let(::deriveHostedWorkspaceRestoreState)
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
    val windows = restoreState.windows.filter(::isAgentlyHostedWorkspaceWindow)
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
