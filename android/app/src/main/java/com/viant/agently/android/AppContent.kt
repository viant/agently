package com.viant.agently.android

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.AuthProvider
import com.viant.agentlysdk.AuthUser
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.forgeandroid.runtime.ForgeRuntime
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement

@Composable
internal fun AppBody(
    authState: AuthState,
    currentScreen: AppScreen,
    isTablet: Boolean,
    loading: Boolean,
    configuredAppApiBaseUrl: String,
    appApiBaseUrl: String,
    metadata: WorkspaceMetadata?,
    preferredAgentId: String,
    savedLoginConfig: SavedLoginConfig,
    authBusy: Boolean,
    authError: String?,
    error: String?,
    authProviders: List<AuthProvider>,
    authUser: AuthUser?,
    authWebUrl: String?,
    showSavedLoginSettings: Boolean,
    effectiveAgentId: String?,
    recentConversations: List<Conversation>,
    activeConversationId: String?,
    streamSnapshot: ConversationStreamSnapshot?,
    transcript: List<ChatEntry>,
    pendingApprovals: List<PendingToolApproval>,
    generatedFiles: List<GeneratedFileEntry>,
    artifactPreview: ArtifactPreview?,
    client: AgentlyClient,
    forgeRuntime: ForgeRuntime,
    approvalJson: Json,
    approvalEdits: Map<String, Map<String, JsonElement>>,
    query: String,
    composerAttachments: List<ComposerAttachmentDraft>,
    mediaController: ComposerMediaController,
    callbacks: AppUiCallbacks
) {
    Box(
        modifier = Modifier
            .fillMaxSize()
            .padding(16.dp)
    ) {
        if (authState == AuthState.Checking) {
            Box(
                modifier = Modifier.fillMaxSize(),
                contentAlignment = Alignment.Center
            ) {
                CircularProgressIndicator()
            }
            return
        }
        if (authState == AuthState.Required) {
            if (currentScreen == AppScreen.Settings) {
                SettingsScreen(
                    configuredAppApiBaseUrl = configuredAppApiBaseUrl,
                    currentAppApiBaseUrl = appApiBaseUrl,
                    metadata = metadata,
                    currentPreferredAgentId = preferredAgentId,
                    savedLoginConfig = savedLoginConfig,
                    loading = loading,
                    error = authError ?: error,
                    onBack = callbacks.onBackFromSettings,
                    onRefreshWorkspace = callbacks.onRefreshAuth,
                    onSave = callbacks.onSaveSettings,
                    onResetAppOverrides = callbacks.onResetAppOverrides,
                    onClearAuthSecrets = callbacks.onClearAuthSecrets
                )
                return
            }
            AuthRequiredScreen(
                busy = authBusy,
                error = authError,
                providers = authProviders,
                user = authUser,
                savedLoginConfig = savedLoginConfig,
                onSignIn = callbacks.onAuthSignIn,
                onManageSavedLogin = callbacks.onManageSavedLogin,
                onOpenSettings = callbacks.onOpenSettings,
                onRetry = callbacks.onAuthRetry
            )
            authWebUrl?.let { authUrl ->
                OAuthWebDialog(
                    authUrl = authUrl,
                    callbackPrefix = "/v1/api/auth/oauth/callback",
                    savedLoginConfig = savedLoginConfig,
                    onDismiss = callbacks.onDismissAuthWeb,
                    onCallback = callbacks.onOAuthCallback
                )
            }
            if (showSavedLoginSettings) {
                SavedLoginConfigDialog(
                    initial = savedLoginConfig,
                    onDismiss = callbacks.onDismissSavedLoginSettings,
                    onSave = callbacks.onSaveSavedLoginSettings,
                    onClear = callbacks.onClearSavedLoginSettings
                )
            }
            return
        }
        when (currentScreen) {
            AppScreen.Chat -> {
                if (isTablet) {
                    TabletChatScreen(
                        appApiBaseUrl = appApiBaseUrl,
                        metadata = metadata,
                        effectiveAgentId = effectiveAgentId,
                        loading = loading,
                        recentConversations = recentConversations,
                        activeConversationId = activeConversationId,
                        error = error,
                        streamSnapshot = streamSnapshot,
                        transcript = transcript,
                        pendingApprovals = pendingApprovals,
                        generatedFiles = generatedFiles,
                        artifactPreview = artifactPreview,
                        client = client,
                        forgeRuntime = forgeRuntime,
                        approvalJson = approvalJson,
                        approvalEdits = approvalEdits,
                        onRefresh = callbacks.onRefreshWorkspace,
                        onNewConversation = callbacks.onNewConversation,
                        onSelectConversation = callbacks.onSelectConversation,
                        onEditChange = callbacks.onApprovalEditChange,
                        onDecision = callbacks.onApprovalDecision,
                        onOpenFile = callbacks.onOpenFile,
                        onClosePreview = callbacks.onClosePreview,
                        query = query,
                        onQueryChange = callbacks.onQueryChange,
                        composerAttachments = composerAttachments,
                        canCapturePhoto = mediaController.canCapturePhoto,
                        canUseVoiceInput = mediaController.canUseVoiceInput,
                        onAddPhoto = mediaController.launchPhotoPicker,
                        onTakePhoto = mediaController.launchCameraCapture,
                        onVoiceInput = mediaController.launchVoiceInput,
                        onRemoveAttachment = mediaController.removeAttachment,
                        onRunQuery = callbacks.onRunQuery
                    )
                } else {
                    PhoneChatScreen(
                        appApiBaseUrl = appApiBaseUrl,
                        metadata = metadata,
                        effectiveAgentId = effectiveAgentId,
                        loading = loading,
                        recentConversations = recentConversations,
                        activeConversationId = activeConversationId,
                        error = error,
                        streamSnapshot = streamSnapshot,
                        transcript = transcript,
                        pendingApprovals = pendingApprovals,
                        generatedFiles = generatedFiles,
                        artifactPreview = artifactPreview,
                        client = client,
                        forgeRuntime = forgeRuntime,
                        approvalJson = approvalJson,
                        approvalEdits = approvalEdits,
                        onRefresh = callbacks.onRefreshWorkspace,
                        onNewConversation = callbacks.onNewConversation,
                        onOpenHistory = callbacks.onOpenHistory,
                        onOpenSettings = callbacks.onOpenSettings,
                        onSelectConversation = callbacks.onSelectConversation,
                        onEditChange = callbacks.onApprovalEditChange,
                        onDecision = callbacks.onApprovalDecision,
                        onOpenFile = callbacks.onOpenFile,
                        onClosePreview = callbacks.onClosePreview
                    )
                }
            }
            AppScreen.History -> {
                ConversationHistoryScreen(
                    conversations = recentConversations,
                    activeConversationId = activeConversationId,
                    loading = loading,
                    onBack = callbacks.onBackFromHistory,
                    onRefresh = callbacks.onRefreshWorkspace,
                    onSelectConversation = callbacks.onSelectConversation
                )
            }
            AppScreen.Settings -> {
                SettingsScreen(
                    configuredAppApiBaseUrl = configuredAppApiBaseUrl,
                    currentAppApiBaseUrl = appApiBaseUrl,
                    metadata = metadata,
                    currentPreferredAgentId = preferredAgentId,
                    savedLoginConfig = savedLoginConfig,
                    loading = loading,
                    error = error,
                    onBack = callbacks.onBackFromSettings,
                    onRefreshWorkspace = callbacks.onRefreshWorkspace,
                    onSave = callbacks.onSaveSettings,
                    onResetAppOverrides = callbacks.onResetAppOverrides,
                    onClearAuthSecrets = callbacks.onClearAuthSecrets
                )
            }
        }
        if (currentScreen == AppScreen.Chat && !isTablet) {
            Box(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .fillMaxSize(),
                contentAlignment = Alignment.BottomCenter
            ) {
                PhoneComposerDock(
                    loading = loading,
                    activeConversationId = activeConversationId,
                    query = query,
                    onQueryChange = callbacks.onQueryChange,
                    composerAttachments = composerAttachments,
                    canCapturePhoto = mediaController.canCapturePhoto,
                    canUseVoiceInput = mediaController.canUseVoiceInput,
                    onAddPhoto = mediaController.launchPhotoPicker,
                    onTakePhoto = mediaController.launchCameraCapture,
                    onVoiceInput = mediaController.launchVoiceInput,
                    onRemoveAttachment = mediaController.removeAttachment,
                    onOpenSettings = callbacks.onOpenSettings,
                    onRunQuery = callbacks.onRunQuery
                )
            }
        }
    }
}
