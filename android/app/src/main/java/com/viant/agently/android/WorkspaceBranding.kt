package com.viant.agently.android

import com.viant.agentlysdk.WorkspaceMetadata
import java.io.File
import java.util.Locale

internal fun resolveWorkspaceBrandLabel(
    metadata: WorkspaceMetadata?,
    fallbackLabel: String = "Agently"
): String {
    val explicit = metadata?.appName?.trim()
        ?.takeIf { it.isNotEmpty() }
        ?: metadata?.defaults?.appName?.trim()
            ?.takeIf { it.isNotEmpty() }
        ?: return fallbackLabel
    return explicit
}

internal fun resolveWorkspaceBrandTitle(
    workspaceRoot: String?,
    defaultAgent: String?,
    fallbackTitle: String = "Agently"
): String {
    val preferred = workspaceRoot.workspaceDisplayTitle()
        ?: defaultAgent?.trim()?.takeIf { it.isNotEmpty() }
        ?: return fallbackTitle
    val normalized = preferred
        .replace("_", " ")
        .replace("-", " ")
        .trim()
        .split(Regex("\\s+"))
        .filter { it.isNotBlank() }
        .joinToString(" ") { token ->
            token.lowercase(Locale.US).replaceFirstChar { ch ->
                if (ch.isLowerCase()) ch.titlecase(Locale.US) else ch.toString()
            }
        }
        .trim()
    if (normalized.isEmpty()) {
        return fallbackTitle
    }
    return normalized
        .replace(Regex("^viant\\s+", RegexOption.IGNORE_CASE), "")
        .trim()
        .ifBlank { fallbackTitle }
}

internal fun String?.workspaceDisplayTitle(): String? {
    val trimmed = this?.trim().orEmpty()
    if (trimmed.isEmpty()) return null
    val normalized = if (trimmed.endsWith("/")) trimmed.dropLast(1) else trimmed
    val file = File(normalized)
    val candidate = file.name
    if (candidate.isNotBlank() && !candidate.startsWith(".")) {
        return candidate
    }
    val parent = file.parentFile?.name
    return parent?.takeIf { it.isNotBlank() } ?: normalized
}
