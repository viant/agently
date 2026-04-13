package com.viant.agently.android

internal data class SettingsApplyTransition(
    val resolvedBaseUrl: String,
    val preferredAgentId: String,
    val requiresWorkspaceReset: Boolean
)

internal fun persistAppSettings(
    store: AppSettingsStore,
    configuredBaseUrl: String,
    nextBaseUrl: String,
    nextPreferredAgentId: String
) {
    store.save(
        AppSettings(
            baseUrlOverride = nextBaseUrl.trim().takeUnless { it == configuredBaseUrl }.orEmpty(),
            preferredAgentId = nextPreferredAgentId.trim()
        )
    )
}

internal fun buildSettingsApplyTransition(
    configuredBaseUrl: String,
    currentBaseUrl: String,
    nextBaseUrl: String,
    nextPreferredAgentId: String
): SettingsApplyTransition {
    val resolvedBaseUrl = nextBaseUrl.trim().ifBlank { configuredBaseUrl }
    return SettingsApplyTransition(
        resolvedBaseUrl = resolvedBaseUrl,
        preferredAgentId = nextPreferredAgentId.trim(),
        requiresWorkspaceReset = resolvedBaseUrl != currentBaseUrl
    )
}

internal fun buildResetOverridesTransition(
    configuredBaseUrl: String,
    currentBaseUrl: String
): SettingsApplyTransition {
    return SettingsApplyTransition(
        resolvedBaseUrl = configuredBaseUrl,
        preferredAgentId = "",
        requiresWorkspaceReset = currentBaseUrl != configuredBaseUrl
    )
}
