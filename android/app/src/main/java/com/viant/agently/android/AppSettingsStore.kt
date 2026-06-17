package com.viant.agently.android

import android.content.Context

data class AppSettings(
    val baseUrlOverride: String = "",
    val preferredAgentId: String = "",
    val hasWorkspaceEndpointSelection: Boolean = false
)

class AppSettingsStore(context: Context) {
    private val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    fun load(): AppSettings = AppSettings(
        baseUrlOverride = prefs.getString(KEY_BASE_URL_OVERRIDE, "").orEmpty(),
        preferredAgentId = prefs.getString(KEY_PREFERRED_AGENT_ID, "").orEmpty(),
        hasWorkspaceEndpointSelection = prefs.getBoolean(KEY_HAS_WORKSPACE_ENDPOINT_SELECTION, false)
    )

    fun save(settings: AppSettings) {
        prefs.edit()
            .putString(KEY_BASE_URL_OVERRIDE, settings.baseUrlOverride)
            .putString(KEY_PREFERRED_AGENT_ID, settings.preferredAgentId)
            .putBoolean(KEY_HAS_WORKSPACE_ENDPOINT_SELECTION, settings.hasWorkspaceEndpointSelection)
            .apply()
    }

    fun clear() {
        prefs.edit().clear().apply()
    }

    companion object {
        private const val PREFS_NAME = "agently.app.settings"
        private const val KEY_BASE_URL_OVERRIDE = "base_url_override"
        private const val KEY_PREFERRED_AGENT_ID = "preferred_agent_id"
        private const val KEY_HAS_WORKSPACE_ENDPOINT_SELECTION = "has_workspace_endpoint_selection"
    }
}
