package com.viant.agently.android

import com.viant.agentlysdk.HostedWorkspaceRestoreState
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.WorkspaceWindowSnapshot

internal data class HostedWorkspacePresentation(
    val badgeLabel: String,
    val title: String,
    val subtitle: String? = null,
    val supportingText: String = "",
    val navigationIcon: String? = null,
)

internal data class HostedWorkspaceAttachment(
    val turnId: String,
    val presentation: HostedWorkspacePresentation,
)

internal fun isHostedWorkspaceOpeningToolName(value: String): Boolean {
    val normalized = value.trim().lowercase().replace(':', '/')
    return normalized in setOf("ui/view/open", "ui/window/open", "ui/window/show")
}

internal fun hostedWorkspaceAttachment(
    conversationState: ConversationStateResponse?,
    restoreState: HostedWorkspaceRestoreState?,
): HostedWorkspaceAttachment? {
    val presentation = resolveHostedWorkspacePresentation(restoreState) ?: return null
    val turn = conversationState?.conversation?.turns?.asReversed()?.firstOrNull { current ->
        current.execution?.pages.orEmpty().any { page ->
            page.toolSteps.any { step ->
                val status = step.status?.trim()?.lowercase().orEmpty()
                (status.isEmpty() || status in setOf("completed", "succeeded", "success", "done")) &&
                    isHostedWorkspaceOpeningToolName(step.toolName)
            }
        }
    } ?: return null
    return turn.turnId.trim().takeIf(String::isNotEmpty)?.let { HostedWorkspaceAttachment(it, presentation) }
}

internal fun resolveHostedWorkspacePresentation(
    restoreState: HostedWorkspaceRestoreState?
): HostedWorkspacePresentation? {
    restoreState ?: return null
    val window = restoreState.windows.firstOrNull { it.windowId == restoreState.selectedWindowId }
        ?: restoreState.windows.lastOrNull()
    return resolveHostedWorkspacePresentation(window)
}

internal fun resolveHostedWorkspacePresentation(
    window: WorkspaceWindowSnapshot?
): HostedWorkspacePresentation? {
    window ?: return null
    val badgeLabel = normalizeHostedWorkspaceText(window.navigation?.label)
        ?: humanizeHostedWorkspaceKey(window.windowKey)
        ?: "Workspace"
    val normalizedTitle = normalizeHostedWorkspaceText(window.windowTitle)
    val title = when {
        normalizedTitle != null && !normalizedTitle.equals(window.windowKey, ignoreCase = true) -> normalizedTitle
        else -> badgeLabel
    }
    return HostedWorkspacePresentation(
        badgeLabel = badgeLabel,
        title = title,
        subtitle = null,
        supportingText = "Open the ${badgeLabel.lowercase()} workspace.",
        navigationIcon = window.navigation?.icon,
    )
}

private fun humanizeHostedWorkspaceKey(windowKey: String): String? {
    val normalized = windowKey
        .replace("/", " ")
        .replace("_", " ")
        .replace(Regex("([a-z0-9])([A-Z])"), "$1 $2")
        .replace(Regex("([A-Z]+)([A-Z][a-z])"), "$1 $2")
        .trim()
    if (normalized.isEmpty()) {
        return null
    }
    return normalized
        .split(Regex("\\s+"))
        .filter { it.isNotBlank() }
        .joinToString(" ") { token -> humanizeHostedWorkspaceToken(token) }
}

private fun humanizeHostedWorkspaceToken(token: String): String {
    val trimmed = token.trim()
    if (trimmed.isEmpty()) {
        return trimmed
    }
    if (trimmed.all { !it.isLetter() || it.isUpperCase() }) {
        return trimmed
    }
    return trimmed.lowercase().replaceFirstChar { ch ->
        if (ch.isLowerCase()) ch.titlecase() else ch.toString()
    }
}

private fun normalizeHostedWorkspaceText(value: String?): String? {
    val trimmed = value?.trim().orEmpty()
    return trimmed.ifEmpty { null }
}
