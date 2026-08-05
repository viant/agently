package com.viant.agently.android

import com.viant.agentlysdk.HostedWorkspaceRestoreState
import com.viant.agentlysdk.WorkspaceWindowSnapshot

internal data class HostedWorkspacePresentation(
    val badgeLabel: String,
    val title: String,
    val subtitle: String? = null,
    val supportingText: String = "",
)

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
    val badgeLabel = humanizeHostedWorkspaceKey(window.windowKey) ?: "Workspace"
    val normalizedTitle = normalizeHostedWorkspaceText(window.windowTitle)
    val title = when {
        normalizedTitle != null && !normalizedTitle.equals(window.windowKey, ignoreCase = true) -> normalizedTitle
        else -> badgeLabel
    }
    return HostedWorkspacePresentation(
        badgeLabel = badgeLabel,
        title = title,
        subtitle = null,
        supportingText = "",
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
