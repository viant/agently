package com.viant.agently.android

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import java.net.URI

internal data class WorkspaceEndpointOption(
    val title: String,
    val subtitle: String,
    val value: String
)

private val buildWorkspaceEndpointOptions = listOfNotNull(
    workspaceEndpointOption(BuildConfig.APP_API_BASE_URL)
)

internal val workspaceEndpointOptions = mergeWorkspaceEndpointOptions(
    parseWorkspaceEndpointOptions(BuildConfig.WORKSPACE_ENDPOINTS_JSON),
    buildWorkspaceEndpointOptions
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
    buildOptions: List<WorkspaceEndpointOption> = emptyList()
): List<WorkspaceEndpointOption> {
    return (configured + buildOptions)
        .filter { it.value.isNotBlank() }
        .distinctBy { normalizeApiBaseUrl(it.value) }
}

internal fun workspaceEndpointOption(value: String): WorkspaceEndpointOption? {
    val normalized = normalizeApiBaseUrl(value)
    if (!isValidWorkspaceBaseUrl(normalized)) return null
    val host = URI(normalized).host?.trim().orEmpty()
    return WorkspaceEndpointOption(
        title = host,
        subtitle = "Configured workspace",
        value = normalized
    )
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

internal fun isValidWorkspaceBaseUrl(value: String): Boolean {
    val normalized = normalizeApiBaseUrl(value)
    val uri = runCatching { URI(normalized) }.getOrNull() ?: return false
    return (uri.scheme.equals("http", ignoreCase = true) ||
        uri.scheme.equals("https", ignoreCase = true)) &&
        !uri.host.isNullOrBlank() &&
        uri.userInfo.isNullOrBlank()
}

internal fun resolveInitialApiBaseUrl(
    configuredBaseUrl: String,
    storedSettings: AppSettings,
    preferExplicitBuildEndpoint: Boolean,
    availableOptions: List<WorkspaceEndpointOption> = workspaceEndpointOptions
): String {
    val configured = normalizeApiBaseUrl(configuredBaseUrl)
    if (preferExplicitBuildEndpoint && isValidWorkspaceBaseUrl(configured)) return configured

    val candidate = normalizeApiBaseUrl(
        storedSettings.baseUrlOverride.trim().ifBlank {
            if (storedSettings.hasWorkspaceEndpointSelection) configured
            else availableOptions.firstOrNull()?.value.orEmpty()
        }
    )
    return when {
        isValidWorkspaceBaseUrl(candidate) -> candidate
        isValidWorkspaceBaseUrl(configured) -> configured
        else -> availableOptions.firstOrNull()?.value.orEmpty()
    }
}

internal fun hasInitialWorkspaceEndpointSelection(
    configuredBaseUrl: String,
    storedSettings: AppSettings,
    preferExplicitBuildEndpoint: Boolean
): Boolean {
    val configured = normalizeApiBaseUrl(configuredBaseUrl)
    val storedOverride = normalizeApiBaseUrl(storedSettings.baseUrlOverride)
    return (preferExplicitBuildEndpoint && isValidWorkspaceBaseUrl(configured)) ||
        isValidWorkspaceBaseUrl(storedOverride) ||
        (storedSettings.hasWorkspaceEndpointSelection && isValidWorkspaceBaseUrl(configured))
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
    require(isValidWorkspaceBaseUrl(normalizedBaseUrl)) {
        "Workspace endpoint must be an http or https URL with a valid host."
    }
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
    require(isValidWorkspaceBaseUrl(resolvedBaseUrl)) {
        "Workspace endpoint must be an http or https URL with a valid host."
    }
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
