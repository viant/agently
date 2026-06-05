package com.viant.agently.android

import org.junit.Assert.assertEquals
import org.junit.Test

class AuthUiActionsTest {

    @Test
    fun clearPersistedAuthSettings_resetsStoredConfigAndDismissesSettings() {
        val store = RecordingSavedLoginStore()
        var appliedConfig = SavedLoginConfig(
            username = "user",
            password = "pass"
        )
        var showSavedLoginSettings = true

        clearSavedLoginConfig(
            store = store,
            bindings = SavedLoginBindings(
                onSavedLoginConfigChange = { appliedConfig = it },
                onShowSavedLoginSettingsChange = { showSavedLoginSettings = it }
            ),
            dismissSettings = true
        )

        assertEquals(1, store.clearCount)
        assertEquals(SavedLoginConfig(), appliedConfig)
        assertEquals(false, showSavedLoginSettings)
    }

    @Test
    fun clearSavedAuthSecrets_resetsStoredConfig() {
        val store = RecordingSavedLoginStore()
        var appliedConfig = SavedLoginConfig(
            username = "user",
            password = "pass",
            oobSecretRef = "~/.secret/app_oob.enc|blowfish://default"
        )

        clearSavedAuthSecrets(
            store = store,
            bindings = SavedLoginBindings(
                onSavedLoginConfigChange = { appliedConfig = it },
                onShowSavedLoginSettingsChange = {}
            )
        )

        assertEquals(1, store.clearCount)
        assertEquals(SavedLoginConfig(), appliedConfig)
    }

    @Test
    fun savedLoginConfig_reportsStoredOobSecretReference() {
        val config = SavedLoginConfig(oobSecretRef = "~/.secret/app_oob.enc|blowfish://default")

        assertEquals(true, config.hasStoredOobSecretRef)
    }
}

private class RecordingSavedLoginStore : SavedLoginStore {
    var clearCount: Int = 0
        private set

    override fun save(config: SavedLoginConfig) = Unit

    override fun load(): SavedLoginConfig = SavedLoginConfig()

    override fun clear() {
        clearCount += 1
    }
}
