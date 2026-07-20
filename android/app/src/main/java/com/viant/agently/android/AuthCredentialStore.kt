package com.viant.agently.android

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

data class SavedLoginConfig(
    val oobSecretRef: String = ""
) {
    val hasStoredOobSecretRef: Boolean
        get() = oobSecretRef.isNotBlank()
}

internal interface SavedLoginStore {
    fun load(): SavedLoginConfig
    fun save(config: SavedLoginConfig)
    fun clear()
}

class SavedLoginStoreImpl(context: Context) : SavedLoginStore {
    private val prefs = EncryptedSharedPreferences.create(
        context,
        PREFS_NAME,
        MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
    )

    override fun load(): SavedLoginConfig {
        // One-time cleanup for the retired OAuth credential helper.
        prefs.edit()
            .remove(KEY_USERNAME)
            .remove(KEY_PASSWORD)
            .apply()
        return SavedLoginConfig(
            oobSecretRef = prefs.getString(KEY_OOB_SECRET_REF, "").orEmpty()
        )
    }

    override fun save(config: SavedLoginConfig) {
        prefs.edit()
            .putString(KEY_OOB_SECRET_REF, config.oobSecretRef)
            .remove(KEY_USERNAME)
            .remove(KEY_PASSWORD)
            .apply()
    }

    override fun clear() {
        prefs.edit().clear().apply()
    }

    companion object {
        private const val PREFS_NAME = "agently.auth.credentials"
        private const val KEY_USERNAME = "username"
        private const val KEY_PASSWORD = "password"
        private const val KEY_OOB_SECRET_REF = "oob_secret_ref"
    }
}
