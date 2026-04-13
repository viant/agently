package com.viant.agently.android

import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.PendingToolApproval
import kotlinx.serialization.json.JsonElement

internal data class AppUiCallbacks(
    val onRefreshWorkspace: () -> Unit,
    val onNewConversation: () -> Unit,
    val onOpenHistory: () -> Unit,
    val onOpenSettings: () -> Unit,
    val onSelectConversation: (String) -> Unit,
    val onApprovalEditChange: (String, String, JsonElement) -> Unit,
    val onApprovalDecision: (PendingToolApproval, String) -> Unit,
    val onOpenFile: (GeneratedFileEntry) -> Unit,
    val onClosePreview: () -> Unit,
    val onQueryChange: (String) -> Unit,
    val onRunQuery: () -> Unit,
    val onBackFromHistory: () -> Unit,
    val onBackFromSettings: () -> Unit,
    val onRefreshAuth: () -> Unit,
    val onSaveSettings: (String, String, SavedLoginConfig) -> Unit,
    val onResetAppOverrides: () -> Unit,
    val onClearAuthSecrets: () -> Unit,
    val onAuthSignIn: () -> Unit,
    val onManageSavedLogin: () -> Unit,
    val onAuthRetry: () -> Unit,
    val onDismissAuthWeb: () -> Unit,
    val onOAuthCallback: (String, String) -> Unit,
    val onDismissSavedLoginSettings: () -> Unit,
    val onSaveSavedLoginSettings: (SavedLoginConfig) -> Unit,
    val onClearSavedLoginSettings: () -> Unit
)

internal fun buildAppUiCallbacks(
    currentScreenProvider: () -> AppScreen,
    setCurrentScreen: (AppScreen) -> Unit,
    onRefreshWorkspace: () -> Unit,
    onNewConversation: () -> Unit,
    onSelectConversation: (String, Boolean) -> Unit,
    onApprovalEditChange: (String, String, JsonElement) -> Unit,
    onApprovalDecision: (PendingToolApproval, String) -> Unit,
    onOpenFile: (GeneratedFileEntry) -> Unit,
    onClosePreview: () -> Unit,
    onQueryChange: (String) -> Unit,
    onRunQuery: () -> Unit,
    onRefreshAuth: () -> Unit,
    onSaveSettings: (String, String, SavedLoginConfig) -> Unit,
    onResetAppOverrides: () -> Unit,
    onClearAuthSecrets: () -> Unit,
    onAuthSignIn: () -> Unit,
    onManageSavedLogin: () -> Unit,
    onAuthRetry: () -> Unit,
    onDismissAuthWeb: () -> Unit,
    onOAuthCallback: (String, String) -> Unit,
    onDismissSavedLoginSettings: () -> Unit,
    onSaveSavedLoginSettings: (SavedLoginConfig) -> Unit,
    onClearSavedLoginSettings: () -> Unit
): AppUiCallbacks = AppUiCallbacks(
    onRefreshWorkspace = onRefreshWorkspace,
    onNewConversation = onNewConversation,
    onOpenHistory = { setCurrentScreen(AppScreen.History) },
    onOpenSettings = { setCurrentScreen(AppScreen.Settings) },
    onSelectConversation = {
        onSelectConversation(it, currentScreenProvider() == AppScreen.History)
    },
    onApprovalEditChange = onApprovalEditChange,
    onApprovalDecision = onApprovalDecision,
    onOpenFile = onOpenFile,
    onClosePreview = onClosePreview,
    onQueryChange = onQueryChange,
    onRunQuery = onRunQuery,
    onBackFromHistory = { setCurrentScreen(AppScreen.Chat) },
    onBackFromSettings = { setCurrentScreen(AppScreen.Chat) },
    onRefreshAuth = onRefreshAuth,
    onSaveSettings = onSaveSettings,
    onResetAppOverrides = onResetAppOverrides,
    onClearAuthSecrets = onClearAuthSecrets,
    onAuthSignIn = onAuthSignIn,
    onManageSavedLogin = onManageSavedLogin,
    onAuthRetry = onAuthRetry,
    onDismissAuthWeb = onDismissAuthWeb,
    onOAuthCallback = onOAuthCallback,
    onDismissSavedLoginSettings = onDismissSavedLoginSettings,
    onSaveSavedLoginSettings = onSaveSavedLoginSettings,
    onClearSavedLoginSettings = onClearSavedLoginSettings
)
