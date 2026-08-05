package com.viant.agently.android

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import com.viant.forgeandroid.runtime.ForgeRuntime

@Composable
internal fun AppEffects(
    forgeRuntime: ForgeRuntime,
    isTablet: Boolean,
    authState: AuthState,
    metadataLoaded: Boolean,
    loading: Boolean,
    workspaceBootstrapRequested: Boolean,
    onWorkspaceBootstrapRequestedChange: (Boolean) -> Unit,
    onWorkspaceBootstrap: suspend () -> Unit,
    onSetCurrentScreen: (AppScreen) -> Unit,
    onLoadWorkspace: () -> Unit,
    onResetConversation: () -> Unit,
    onDisposeStreamJob: () -> Unit,
    onInitialAuthRefresh: suspend () -> Unit,
    onSetAuthRequired: (Throwable?) -> Unit
) {
    LaunchedEffect(authState, metadataLoaded, loading, workspaceBootstrapRequested) {
        if (!shouldBootstrapWorkspace(
                authState = authState,
                metadataLoaded = metadataLoaded,
                loading = loading,
                workspaceBootstrapRequested = workspaceBootstrapRequested
            )
        ) {
            return@LaunchedEffect
        }
        onWorkspaceBootstrapRequestedChange(true)
        try {
            onWorkspaceBootstrap()
        } finally {
            onWorkspaceBootstrapRequestedChange(false)
        }
    }

    LaunchedEffect(forgeRuntime, isTablet) {
        forgeRuntime.registerHandler("app.navigate") { args ->
            when (args.execution.args.firstOrNull()?.trim()?.lowercase()) {
                "history" -> onSetCurrentScreen(if (isTablet) AppScreen.Chat else AppScreen.History)
                else -> onSetCurrentScreen(AppScreen.Chat)
            }
            null
        }
        forgeRuntime.registerHandler("app.refreshWorkspace") {
            onLoadWorkspace()
            null
        }
        forgeRuntime.registerHandler("app.newConversation") {
            onResetConversation()
            onSetCurrentScreen(AppScreen.Chat)
            null
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            onDisposeStreamJob()
        }
    }

    LaunchedEffect(Unit) {
        runCatching {
            onInitialAuthRefresh()
        }.onFailure { err ->
            onSetAuthRequired(err)
        }
    }
}

internal fun shouldBootstrapWorkspace(
    authState: AuthState,
    metadataLoaded: Boolean,
    loading: Boolean,
    workspaceBootstrapRequested: Boolean
): Boolean {
    return authState == AuthState.Ready &&
        !metadataLoaded &&
        !loading &&
        !workspaceBootstrapRequested
}
