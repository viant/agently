package com.viant.agently.android

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

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
    val onAuthWebUrlChange: (String?) -> Unit
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
        try {
            authBindings.onAuthWebUrlChange(requestAuthWebUrl())
        } catch (err: Throwable) {
            authBindings.onAuthErrorChange(normalizeAuthThrowable(err, normalizeAuthError))
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
        try {
            runOperation()
        } catch (err: Throwable) {
            authBindings.onAuthErrorChange(normalizeAuthThrowable(err, normalizeAuthError))
        } finally {
            authBindings.onAuthBusyChange(false)
        }
    }
}
