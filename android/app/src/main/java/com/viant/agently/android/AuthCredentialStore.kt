package com.viant.agently.android

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

data class SavedLoginConfig(
    val username: String = "",
    val password: String = ""
) {
    val hasStoredIdpCredential: Boolean
        get() = username.isNotBlank() && password.isNotBlank()
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

    override fun load(): SavedLoginConfig = SavedLoginConfig(
        username = prefs.getString(KEY_USERNAME, "").orEmpty(),
        password = prefs.getString(KEY_PASSWORD, "").orEmpty()
    )

    override fun save(config: SavedLoginConfig) {
        prefs.edit()
            .putString(KEY_USERNAME, config.username)
            .putString(KEY_PASSWORD, config.password)
            .apply()
    }

    override fun clear() {
        prefs.edit().clear().apply()
    }

    companion object {
        private const val PREFS_NAME = "agently.auth.credentials"
        private const val KEY_USERNAME = "username"
        private const val KEY_PASSWORD = "password"
    }
}
