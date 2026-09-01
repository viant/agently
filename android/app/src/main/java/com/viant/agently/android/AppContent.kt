package com.viant.agently.android

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.Goal
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
    authInteractiveFailure: Boolean,
    error: String?,
    authSessionId: String?,
    authWebUrl: String?,
    mcpAuthServer: String?,
    mcpAuthWebUrl: String?,
    mcpAuthCookies: List<String>,
    recentConversations: List<Conversation>,
    scheduleHistoryFilter: ScheduleHistoryFilter?,
    activeConversationId: String?,
    openingConversationId: String?,
    conversationState: ConversationStateResponse?,
    activeGoal: Goal?,
    streamSnapshot: ConversationStreamSnapshot?,
    transcript: List<ChatEntry>,
    pendingApprovals: List<PendingToolApproval>,
    generatedFiles: List<GeneratedFileEntry>,
    payloadPreviews: Map<String, ArtifactPreview>,
    artifactPreview: ArtifactPreview?,
    client: AgentlyClient,
    forgeRuntime: ForgeRuntime,
    approvalJson: Json,
    approvalEdits: Map<String, Map<String, JsonElement>>,
    query: String,
    composerAttachments: List<ComposerAttachmentDraft>,
    lookupOccurrences: List<ComposerLookupOccurrence> = emptyList(),
    lookupSelections: Map<String, ComposerLookupSelection> = emptyMap(),
    mediaController: ComposerMediaController,
    callbacks: AppUiCallbacks
) {
    var phoneComposerInset by remember { mutableStateOf(232.dp) }
    // Every conversation must open with a usable composer. The toolbar can still
    // hide it explicitly, but changing conversations must not silently inherit a
    // hidden dock (or hide it merely because the conversation has an id).
    var phoneComposerVisible by remember { mutableStateOf(true) }
    LaunchedEffect(activeConversationId) {
        phoneComposerVisible = true
    }
    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(MaterialTheme.colorScheme.background)
            .statusBarsPadding()
            .navigationBarsPadding()
            .padding(16.dp)
    ) {
        if (authState == AuthState.Checking) {
            Column(
                modifier = Modifier.fillMaxSize(),
                verticalArrangement = Arrangement.spacedBy(14.dp)
            ) {
                Text(
                    text = metadata?.appName?.takeIf { it.isNotBlank() } ?: "Agently",
                    style = MaterialTheme.typography.headlineSmall
                )
                TurnProgressStatus(
                    TurnProgressPresentation(
                        title = "Connecting to workspace",
                        detail = "Checking your saved sign-in and loading workspace details.",
                        activity = "Connecting",
                        toolProgress = null,
                        tokenUsage = null,
                        canStop = false
                    )
                )
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
                    authSessionId = authSessionId,
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
                developerSessionEnabled = shouldShowDeveloperSessionEntry(
                    debugBuild = BuildConfig.DEBUG,
                    interactiveAuthFailure = authInteractiveFailure
                ),
                onSignIn = callbacks.onAuthSignIn,
                onOpenSettings = callbacks.onOpenSettings,
                onDeveloperSessionSignIn = callbacks.onDeveloperSessionSignIn
            )
            authWebUrl?.let { authUrl ->
                OAuthWebDialog(
                    authUrl = authUrl,
                    callbackPrefix = AndroidOAuthRedirectURI,
                    onDismiss = callbacks.onDismissAuthWeb,
                    onCallback = callbacks.onOAuthCallback
                )
            }
            return
        }
        if (authState == AuthState.Unavailable) {
            if (currentScreen == AppScreen.Settings) {
                SettingsScreen(
                    configuredAppApiBaseUrl = configuredAppApiBaseUrl,
                    currentAppApiBaseUrl = appApiBaseUrl,
                    metadata = metadata,
                    currentPreferredAgentId = preferredAgentId,
                    savedLoginConfig = savedLoginConfig,
                    authSessionId = authSessionId,
                    loading = loading,
                    error = authError ?: error,
                    onBack = callbacks.onBackFromSettings,
                    onRefreshWorkspace = callbacks.onRefreshAuth,
                    onSave = callbacks.onSaveSettings,
                    onResetAppOverrides = callbacks.onResetAppOverrides,
                    onClearAuthSecrets = callbacks.onClearAuthSecrets
                )
            } else {
                WorkspaceUnavailableScreen(
                    error = authError,
                    onRetry = callbacks.onAuthRetry,
                    onOpenSettings = callbacks.onOpenSettings
                )
            }
            return
        }
        when (currentScreen) {
            AppScreen.Chat -> {
                val workspaceTitle = resolveWorkspaceBrandTitle(
                    workspaceRoot = metadata?.workspaceRoot,
                    defaultAgent = metadata?.defaultAgent
                )
                if (isTablet) {
                    TabletChatScreen(
                        workspaceTitle = workspaceTitle,
                        appApiBaseUrl = appApiBaseUrl,
                        metadata = metadata,
                        preferredAgentId = preferredAgentId,
                        loading = loading,
                        recentConversations = recentConversations,
                        activeConversationId = activeConversationId,
                        conversationState = conversationState,
                        activeGoal = activeGoal,
                        error = error,
                        streamSnapshot = streamSnapshot,
                        transcript = transcript,
                        pendingApprovals = pendingApprovals,
                        generatedFiles = generatedFiles,
                        payloadPreviews = payloadPreviews,
                        artifactPreview = artifactPreview,
                        client = client,
                        forgeRuntime = forgeRuntime,
                        approvalJson = approvalJson,
                        approvalEdits = approvalEdits,
                        onRefresh = callbacks.onRefreshWorkspace,
                        onNewConversation = callbacks.onNewConversation,
                        onSelectAgent = callbacks.onSelectAgent,
                        onSelectConversation = callbacks.onSelectConversation,
                        onEditChange = callbacks.onApprovalEditChange,
                        onDecision = callbacks.onApprovalDecision,
                        onOpenFile = callbacks.onOpenFile,
                        onOpenInlineReportPdf = callbacks.onOpenInlineReportPdf,
                        onClosePreview = callbacks.onClosePreview,
                        query = query,
                        onQueryChange = callbacks.onQueryChange,
                        composerAttachments = composerAttachments,
                        lookupOccurrences = lookupOccurrences,
                        lookupSelections = lookupSelections,
                        onLookupClick = callbacks.onComposerLookupSelected,
                        canCapturePhoto = mediaController.canCapturePhoto,
                        canUseVoiceInput = mediaController.canUseVoiceInput,
                        onAddPhoto = mediaController.launchPhotoPicker,
                        onTakePhoto = mediaController.launchCameraCapture,
                        onVoiceInput = mediaController.launchVoiceInput,
                        onRemoveAttachment = mediaController.removeAttachment,
                        onRunQuery = callbacks.onRunQuery,
                        onCancelTurn = callbacks.onCancelTurn
                    )
                } else {
                    PhoneChatScreen(
                        workspaceTitle = workspaceTitle,
                        metadata = metadata,
                        preferredAgentId = preferredAgentId,
                        loading = loading,
                        recentConversations = recentConversations,
                        activeConversationId = activeConversationId,
                        openingConversationId = openingConversationId,
                        conversationState = conversationState,
                        activeGoal = activeGoal,
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
                        onSelectAgent = callbacks.onSelectAgent,
                        onOpenHistory = callbacks.onOpenHistory,
                        onOpenAutomation = callbacks.onOpenAutomation,
                        onOpenSettings = callbacks.onOpenSettings,
                        onSelectConversation = callbacks.onSelectConversation,
                        onEditChange = callbacks.onApprovalEditChange,
                        onDecision = callbacks.onApprovalDecision,
                        onOpenFile = callbacks.onOpenFile,
                        onOpenInlineReportPdf = callbacks.onOpenInlineReportPdf,
                        onClosePreview = callbacks.onClosePreview,
                        onStarterTaskSelected = callbacks.onStarterTaskSelected,
                        onCancelTurn = callbacks.onCancelTurn,
                        bottomComposerInset = if (phoneComposerVisible || activeConversationId.isNullOrBlank()) {
                            phoneComposerInset
                        } else {
                            0.dp
                        },
                        composerVisible = phoneComposerVisible || activeConversationId.isNullOrBlank(),
                        onToggleComposer = { phoneComposerVisible = !phoneComposerVisible }
                    )
                }
            }
            AppScreen.History -> {
                ConversationHistoryScreen(
                    workspaceTitle = resolveWorkspaceBrandTitle(
                        workspaceRoot = metadata?.workspaceRoot,
                        defaultAgent = metadata?.defaultAgent
                    ),
                    client = client,
                    conversations = recentConversations,
                    scheduleFilter = scheduleHistoryFilter,
                    activeConversationId = activeConversationId,
                    openingConversationId = openingConversationId,
                    loading = loading,
                    onBack = callbacks.onBackFromHistory,
                    onRefresh = callbacks.onRefreshWorkspace,
                    onSelectConversation = callbacks.onSelectConversation,
                    onDeleteConversation = callbacks.onDeleteConversation
                )
            }
            AppScreen.Automation -> {
                AutomationScreen(
                    forgeRuntime = forgeRuntime,
                    onBack = callbacks.onBackFromAutomation
                )
            }
            AppScreen.Settings -> {
                SettingsScreen(
                    configuredAppApiBaseUrl = configuredAppApiBaseUrl,
                    currentAppApiBaseUrl = appApiBaseUrl,
                    metadata = metadata,
                    currentPreferredAgentId = preferredAgentId,
                    savedLoginConfig = savedLoginConfig,
                    authSessionId = authSessionId,
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
        if (currentScreen == AppScreen.Chat && !isTablet &&
            (phoneComposerVisible || activeConversationId.isNullOrBlank())
        ) {
            Box(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    // MainActivity uses adjustNothing, making Compose the single
                    // owner of IME avoidance. Move the complete measured dock—
                    // input plus action row—to the keyboard boundary as one unit.
                    .fillMaxSize()
                    .imePadding()
                    // AppContent applies 16dp padding to the workspace. The dock
                    // should meet the keyboard edge rather than inherit that gap.
                    .offset(y = 16.dp),
                contentAlignment = Alignment.BottomCenter
            ) {
                PhoneComposerDock(
                    loading = loading,
                    activeConversationId = activeConversationId,
                    agentLabel = resolveSelectedAgentLabel(preferredAgentId, metadata)
                        ?.takeIf { showWorkspaceAgentSelection(metadata) },
                    query = query,
                    onQueryChange = callbacks.onQueryChange,
                    onClearComposer = callbacks.onClearComposer,
                    composerAttachments = composerAttachments,
                    lookupOccurrences = lookupOccurrences,
                    lookupSelections = lookupSelections,
                    onLookupClick = callbacks.onComposerLookupSelected,
                    canCapturePhoto = mediaController.canCapturePhoto,
                    canUseVoiceInput = mediaController.canUseVoiceInput,
                    voiceInputState = mediaController.voiceInputState,
                    composerCursorPosition = mediaController.composerCursorPosition,
                    onComposerCursorPositionChange = mediaController.updateComposerCursorPosition,
                    onAddPhoto = mediaController.launchPhotoPicker,
                    onTakePhoto = mediaController.launchCameraCapture,
                    onVoiceInput = mediaController.launchVoiceInput,
                    onRemoveAttachment = mediaController.removeAttachment,
                    onOpenSettings = callbacks.onOpenSettings,
                    onRunQuery = callbacks.onRunQuery,
                    onMeasuredHeight = { phoneComposerInset = it }
                )
            }
        }
        if (mcpAuthWebUrl != null) {
            HostedMCPAuthWebDialog(
                authUrl = mcpAuthWebUrl,
                appBaseUrl = appApiBaseUrl,
                cookies = mcpAuthCookies,
                onDismiss = callbacks.onMCPAuthDismiss,
                onReturn = callbacks.onMCPAuthReturned
            )
        } else if (mcpAuthServer != null) {
            AlertDialog(
                onDismissRequest = callbacks.onMCPAuthDismiss,
                title = { Text("Connect $mcpAuthServer") },
                text = { Text("This request needs an authorized provider connection. Sign in, then retry the request.") },
                confirmButton = {
                    TextButton(onClick = callbacks.onMCPAuthConnect) { Text("Connect") }
                },
                dismissButton = {
                    TextButton(onClick = callbacks.onMCPAuthDismiss) { Text("Not now") }
                }
            )
        }
    }
}
