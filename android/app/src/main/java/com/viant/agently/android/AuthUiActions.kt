package com.viant.agently.android

import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

private const val AUTH_UI_LOG_TAG = "AgentlyAuth"

internal fun normalizeAuthThrowable(
    err: Throwable?,
    normalizeAuthError: (String?) -> String?
): String? {
    return normalizeAuthError(err?.message ?: err?.toString())
}

internal data class SavedLoginBindings(
    val onSavedLoginConfigChange: (SavedLoginConfig) -> Unit
)

internal data class AuthUiBindings(
    val onAuthBusyChange: (Boolean) -> Unit,
    val onAuthErrorChange: (String?) -> Unit,
    val onAuthWebUrlChange: (String?) -> Unit,
    val onInteractiveAuthFailureChange: (Boolean) -> Unit = {}
)

internal fun persistSavedLoginConfig(
    store: SavedLoginStore,
    next: SavedLoginConfig,
    bindings: SavedLoginBindings
) {
    store.save(next)
    bindings.onSavedLoginConfigChange(next)
}

internal fun clearSavedLoginConfig(
    store: SavedLoginStore,
    bindings: SavedLoginBindings
) {
    store.clear()
    bindings.onSavedLoginConfigChange(SavedLoginConfig())
}

internal fun clearSavedAuthSecrets(
    store: SavedLoginStore,
    bindings: SavedLoginBindings
) {
    clearSavedLoginConfig(
        store = store,
        bindings = bindings
    )
}

internal fun launchAuthRefresh(
    scope: CoroutineScope,
    loadOnSuccess: Boolean,
    refreshAuthState: suspend (Boolean) -> Unit
) {
    scope.launch {
        refreshAuthState(loadOnSuccess)
    }
}

internal fun launchAuthSignIn(
    scope: CoroutineScope,
    authBindings: AuthUiBindings,
    requestAuthWebUrl: suspend () -> String,
    normalizeAuthError: (String?) -> String?
) {
    scope.launch {
        authBindings.onAuthBusyChange(true)
        authBindings.onAuthErrorChange(null)
        authBindings.onInteractiveAuthFailureChange(false)
        try {
            authBindings.onAuthWebUrlChange(requestAuthWebUrl())
        } catch (err: Throwable) {
            Log.w(AUTH_UI_LOG_TAG, "Auth sign-in failed: ${err.javaClass.simpleName}: ${err.message}")
            val normalized = normalizeAuthThrowable(err, normalizeAuthError)
            authBindings.onAuthErrorChange(normalized)
            authBindings.onInteractiveAuthFailureChange(normalized != null)
        } finally {
            authBindings.onAuthBusyChange(false)
        }
    }
}

internal fun launchAuthOperation(
    scope: CoroutineScope,
    authBindings: AuthUiBindings,
    runOperation: suspend () -> Unit,
    normalizeAuthError: (String?) -> String?
) {
    scope.launch {
        authBindings.onAuthBusyChange(true)
        authBindings.onAuthErrorChange(null)
        authBindings.onInteractiveAuthFailureChange(false)
        try {
            runOperation()
            authBindings.onInteractiveAuthFailureChange(false)
        } catch (err: Throwable) {
            Log.w(AUTH_UI_LOG_TAG, "Auth operation failed: ${err.javaClass.simpleName}: ${err.message}")
            val normalized = normalizeAuthThrowable(err, normalizeAuthError)
            authBindings.onAuthErrorChange(normalized)
            authBindings.onInteractiveAuthFailureChange(normalized != null)
        } finally {
            authBindings.onAuthBusyChange(false)
        }
    }
}
