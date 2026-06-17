package com.viant.agently.android

internal data class WorkspaceEndpointOption(
    val title: String,
    val subtitle: String,
    val value: String
)

internal val workspaceEndpointOptions = listOf(
    WorkspaceEndpointOption(
        title = "Steward",
        subtitle = "Viant Steward workspace",
        value = "https://steward.agently.viantinc.com"
    ),
    WorkspaceEndpointOption(
        title = "Localhost 9191",
        subtitle = "Local Agently server on this device",
        value = "http://localhost:9191"
    ),
    WorkspaceEndpointOption(
        title = "Android Host 9191",
        subtitle = "Local Agently server on the emulator host",
        value = "http://10.0.2.2:9191"
    )
)

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
