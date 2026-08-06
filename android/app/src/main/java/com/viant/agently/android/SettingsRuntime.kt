package com.viant.agently.android

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject

internal data class WorkspaceEndpointOption(
    val title: String,
    val subtitle: String,
    val value: String
)

private val defaultWorkspaceEndpointOptions = listOf(
    WorkspaceEndpointOption(
        title = "Steward",
        subtitle = "Viant Steward workspace",
        value = "https://steward.agently.viantinc.com"
    )
)

internal val workspaceEndpointOptions = mergeWorkspaceEndpointOptions(
    parseWorkspaceEndpointOptions(BuildConfig.WORKSPACE_ENDPOINTS_JSON),
    defaultWorkspaceEndpointOptions
)

internal fun parseWorkspaceEndpointOptions(raw: String): List<WorkspaceEndpointOption> {
    val trimmed = raw.trim()
    if (trimmed.isEmpty()) return emptyList()
    val root = runCatching { Json.parseToJsonElement(trimmed) }.getOrNull() ?: return emptyList()
    val entries = when (root) {
        is JsonObject -> runCatching { root["endpoints"]?.jsonArray }.getOrNull() ?: return emptyList()
        else -> runCatching { root.jsonArray }.getOrNull() ?: return emptyList()
    }
    return entries.mapNotNull { entry ->
        val item = runCatching { entry.jsonObject }.getOrNull() ?: return@mapNotNull null
        val value = item.stringValue("value") ?: item.stringValue("url") ?: return@mapNotNull null
        val normalizedValue = normalizeApiBaseUrl(value)
        if (normalizedValue.isBlank()) return@mapNotNull null
        WorkspaceEndpointOption(
            title = item.stringValue("title") ?: item.stringValue("name") ?: normalizedValue,
            subtitle = item.stringValue("subtitle") ?: item.stringValue("description") ?: "",
            value = normalizedValue
        )
    }
}

internal fun mergeWorkspaceEndpointOptions(
    configured: List<WorkspaceEndpointOption>,
    defaults: List<WorkspaceEndpointOption> = defaultWorkspaceEndpointOptions
): List<WorkspaceEndpointOption> {
    return (configured + defaults)
        .filter { it.value.isNotBlank() }
        .distinctBy { normalizeApiBaseUrl(it.value) }
}

private fun JsonObject.stringValue(key: String): String? {
    return (this[key] as? JsonPrimitive)
        ?.content
        ?.trim()
        ?.takeIf { it.isNotEmpty() }
}

internal data class SettingsApplyTransition(
    val resolvedBaseUrl: String,
    val preferredAgentId: String,
    val requiresWorkspaceReset: Boolean
)

internal fun normalizeApiBaseUrl(value: String): String {
    var normalized = value.trim().trimEnd('/')
    if (normalized.endsWith("/v1/api", ignoreCase = true)) {
        normalized = normalized.dropLast("/v1/api".length)
    } else if (normalized.endsWith("/v1", ignoreCase = true)) {
        normalized = normalized.dropLast("/v1".length)
    }
    return normalized.trimEnd('/')
}

internal fun selectedWorkspaceEndpointOption(baseUrl: String): WorkspaceEndpointOption? {
    val normalized = normalizeApiBaseUrl(baseUrl)
    return workspaceEndpointOptions.firstOrNull { option -> option.value == normalized }
}

internal fun persistAppSettings(
    store: AppSettingsStore,
    configuredBaseUrl: String,
    nextBaseUrl: String,
    nextPreferredAgentId: String,
    hasWorkspaceEndpointSelection: Boolean = true
) {
    val normalizedBaseUrl = normalizeApiBaseUrl(nextBaseUrl)
    val normalizedConfiguredBaseUrl = normalizeApiBaseUrl(configuredBaseUrl)
    store.save(
        AppSettings(
            baseUrlOverride = normalizedBaseUrl.takeUnless { it == normalizedConfiguredBaseUrl }.orEmpty(),
            preferredAgentId = nextPreferredAgentId.trim(),
            hasWorkspaceEndpointSelection = hasWorkspaceEndpointSelection
        )
    )
}

internal fun buildSettingsApplyTransition(
    configuredBaseUrl: String,
    currentBaseUrl: String,
    nextBaseUrl: String,
    nextPreferredAgentId: String
): SettingsApplyTransition {
    val resolvedBaseUrl = normalizeApiBaseUrl(nextBaseUrl).ifBlank { normalizeApiBaseUrl(configuredBaseUrl) }
    return SettingsApplyTransition(
        resolvedBaseUrl = resolvedBaseUrl,
        preferredAgentId = nextPreferredAgentId.trim(),
        requiresWorkspaceReset = resolvedBaseUrl != normalizeApiBaseUrl(currentBaseUrl)
    )
}

internal fun buildResetOverridesTransition(
    configuredBaseUrl: String,
    currentBaseUrl: String
): SettingsApplyTransition {
    val resolvedBaseUrl = normalizeApiBaseUrl(configuredBaseUrl)
    return SettingsApplyTransition(
        resolvedBaseUrl = resolvedBaseUrl,
        preferredAgentId = "",
        requiresWorkspaceReset = normalizeApiBaseUrl(currentBaseUrl) != resolvedBaseUrl
    )
}
