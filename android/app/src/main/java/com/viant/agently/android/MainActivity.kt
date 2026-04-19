package com.viant.agently.android

import android.graphics.Bitmap
import android.os.Bundle
import android.net.Uri
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.TextButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.toMutableStateList
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.AuthProvider
import com.viant.agentlysdk.AuthUser
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.CreateConversationInput
import com.viant.agentlysdk.DecideToolApprovalInput
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.ListPendingToolApprovalsInput
import com.viant.agentlysdk.MetadataTargetContext
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.QueryAttachment
import com.viant.agentlysdk.QueryInput
import com.viant.agentlysdk.QueryOutput
import com.viant.agentlysdk.OAuthCallbackInput
import com.viant.agentlysdk.UploadFileInput
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.forgeandroid.runtime.EndpointConfig
import com.viant.forgeandroid.runtime.ContentDef
import com.viant.forgeandroid.runtime.ContainerDef
import com.viant.forgeandroid.runtime.DataSourceDef
import com.viant.forgeandroid.runtime.ForgeTargetContext
import com.viant.forgeandroid.runtime.MemoryCookieJar
import com.viant.forgeandroid.runtime.SchemaBasedFormDef
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.ItemDef
import com.viant.forgeandroid.runtime.OptionDef
import com.viant.forgeandroid.runtime.ViewDef
import com.viant.forgeandroid.runtime.WindowMetadata
import com.viant.forgeandroid.runtime.sessionHttpClient
import com.viant.forgeandroid.ui.ContainerRenderer
import com.viant.forgeandroid.ui.MarkdownRenderer
import kotlinx.coroutines.Job
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.first
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import java.text.SimpleDateFormat
import java.net.URI
import java.util.Date
import java.util.Locale
import java.time.OffsetDateTime

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            AgentlyTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    AgentlyApp()
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AgentlyApp() {
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val configuration = LocalConfiguration.current
    val formFactor = if (configuration.smallestScreenWidthDp >= 600) "tablet" else "phone"
    val isTablet = formFactor == "tablet"
    val configuredAppApiBaseUrl = BuildConfig.APP_API_BASE_URL
    val appSettingsStore = remember(context) { AppSettingsStore(context.applicationContext) }
    val storedAppSettings = remember(appSettingsStore) { appSettingsStore.load() }
    var appApiBaseUrl by remember {
        mutableStateOf(storedAppSettings.baseUrlOverride.trim().ifBlank { configuredAppApiBaseUrl })
    }
    var preferredAgentId by remember { mutableStateOf(storedAppSettings.preferredAgentId) }
    val savedLoginStore = remember(context) { SavedLoginStoreImpl(context.applicationContext) }
    val storedSavedLoginConfig = remember(savedLoginStore) { savedLoginStore.load() }
    var savedLoginConfig by remember { mutableStateOf(storedSavedLoginConfig) }
    var showSavedLoginSettings by remember { mutableStateOf(false) }
    val sessionCookieJar = remember { MemoryCookieJar() }
    val forgeTargetContext = remember(formFactor) { buildForgeTargetContext(formFactor) }
    fun buildClient(baseUrl: String): AgentlyClient = AgentlyClient(
        endpoints = mapOf(
            "appAPI" to EndpointConfig(
                baseUrl = baseUrl,
                httpClient = sessionHttpClient(sessionCookieJar)
            )
        )
    )
    val appApiCandidates = remember(configuredAppApiBaseUrl) {
        buildApiCandidates(configuredAppApiBaseUrl)
    }
    val client = remember(appApiBaseUrl) { buildClient(appApiBaseUrl) }
    var loading by remember { mutableStateOf(false) }
    var metadata by remember { mutableStateOf<WorkspaceMetadata?>(null) }
    var query by remember { mutableStateOf("") }
    var composerAttachments by remember { mutableStateOf<List<ComposerAttachmentDraft>>(emptyList()) }
    var result by remember { mutableStateOf<QueryOutput?>(null) }
    var streamSnapshot by remember { mutableStateOf<ConversationStreamSnapshot?>(null) }
    var streamedMarkdown by remember { mutableStateOf<String?>(null) }
    var activeConversationId by remember { mutableStateOf<String?>(null) }
    var conversationState by remember { mutableStateOf<ConversationStateResponse?>(null) }
    var recentConversations by remember { mutableStateOf<List<Conversation>>(emptyList()) }
    var currentScreen by remember { mutableStateOf(AppScreen.Chat) }
    var pendingApprovals by remember { mutableStateOf<List<PendingToolApproval>>(emptyList()) }
    var generatedFiles by remember { mutableStateOf<List<GeneratedFileEntry>>(emptyList()) }
    var payloadPreviews by remember { mutableStateOf<Map<String, ArtifactPreview>>(emptyMap()) }
    var artifactPreview by remember { mutableStateOf<ArtifactPreview?>(null) }
    var streamJob by remember { mutableStateOf<Job?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    var approvalEdits by remember { mutableStateOf<Map<String, Map<String, JsonElement>>>(emptyMap()) }
    val transcript = remember { mutableListOf<ChatEntry>().toMutableStateList() }
    val approvalJson = remember { Json { ignoreUnknownKeys = true } }
    val forgeRuntime = remember(scope, forgeTargetContext, appApiBaseUrl) {
        ForgeRuntime(
            endpoints = mapOf(
                "appAPI" to EndpointConfig(
                    baseUrl = appApiBaseUrl,
                    httpClient = sessionHttpClient(sessionCookieJar)
                )
            ),
            scope = scope,
            targetContext = forgeTargetContext,
            windowMetadataBaseUri = "v1/api/agently/forge/window"
        )
    }
    var authState by remember { mutableStateOf(AuthState.Checking) }
    var authProviders by remember { mutableStateOf<List<AuthProvider>>(emptyList()) }
    var authUser by remember { mutableStateOf<AuthUser?>(null) }
    var authWebUrl by remember { mutableStateOf<String?>(null) }
    var authBusy by remember { mutableStateOf(false) }
    var authError by remember { mutableStateOf<String?>(null) }
    var workspaceBootstrapRequested by remember { mutableStateOf(false) }
    val effectiveAgentId = resolvePreferredAgentId(preferredAgentId, metadata)

    fun updateComposerAttachments(attachments: List<ComposerAttachmentDraft>) {
        composerAttachments = attachments
    }

    fun setQueryText(value: String) {
        query = value
    }

    fun setVisibleError(message: String?) {
        error = message
    }

    val mediaController = rememberComposerMediaController(
        attachments = composerAttachments,
        onAttachmentsChange = ::updateComposerAttachments,
        query = query,
        onQueryChange = ::setQueryText,
        onError = ::setVisibleError
    )

    fun setAppApiBaseUrl(baseUrl: String) {
        appApiBaseUrl = baseUrl
    }

    suspend fun resolveClient(): AgentlyClient {
        return resolveWorkspaceClient(
            currentBaseUrl = appApiBaseUrl,
            candidates = mergeApiCandidates(appApiBaseUrl, appApiCandidates),
            currentClient = client,
            buildClient = ::buildClient,
            onResolvedBaseUrl = ::setAppApiBaseUrl,
            targetContext = buildMetadataTargetContext(formFactor)
        )
    }

    suspend fun resolveAuthClient(): AgentlyClient {
        return resolveAuthClientWithFallback(
            currentBaseUrl = appApiBaseUrl,
            candidates = mergeApiCandidates(appApiBaseUrl, appApiCandidates),
            currentClient = client,
            buildClient = ::buildClient,
            onResolvedBaseUrl = ::setAppApiBaseUrl
        )
    }

    fun applyConversationResetState(resetState: ConversationResetState) {
        activeConversationId = resetState.activeConversationId
        conversationState = null
        streamSnapshot = resetState.streamSnapshot
        streamedMarkdown = resetState.streamedMarkdown
        result = resetState.result
        setVisibleError(resetState.error)
        transcript.clear()
        pendingApprovals = resetState.pendingApprovals
        approvalEdits = resetState.approvalEdits
        generatedFiles = resetState.generatedFiles
        payloadPreviews = emptyMap()
        artifactPreview = resetState.artifactPreview
    }

    fun applyRecentConversations(conversations: List<Conversation>) {
        recentConversations = conversations
    }

    fun applyWorkspaceSnapshot(snapshot: WorkspaceSnapshot) {
        metadata = snapshot.metadata
        applyRecentConversations(snapshot.conversations)
    }

    fun applyAuthSessionState(
        providers: List<AuthProvider>,
        user: AuthUser?,
        state: AuthState
    ) {
        authProviders = providers
        authUser = user
        authState = state
    }

    fun setAuthState(state: AuthState) {
        authState = state
    }

    fun applyAuthUiState(
        webUrl: String?,
        busy: Boolean,
        error: String?
    ) {
        authWebUrl = webUrl
        authBusy = busy
        authError = error
    }

    fun applyWorkspaceSessionReset(resetState: WorkspaceSessionReset) {
        metadata = resetState.metadata
        applyRecentConversations(resetState.recentConversations)
        applyAuthSessionState(
            providers = resetState.authProviders,
            user = resetState.authUser,
            state = resetState.authState
        )
        applyAuthUiState(
            webUrl = authWebUrl,
            busy = authBusy,
            error = resetState.authError
        )
        workspaceBootstrapRequested = resetState.workspaceBootstrapRequested
        applyConversationResetState(resetState.conversationReset)
    }

    fun applyAuthRequiredSessionReset(resetState: AuthRequiredSessionReset) {
        metadata = resetState.metadata
        applyRecentConversations(resetState.recentConversations)
        applyAuthSessionState(
            providers = resetState.authProviders,
            user = resetState.authUser,
            state = authState
        )
        applyAuthUiState(
            webUrl = resetState.authWebUrl,
            busy = resetState.authBusy,
            error = authError
        )
        applyConversationResetState(resetState.conversationReset)
    }

    fun clearActiveStreamJob() {
        streamJob?.cancel()
        streamJob = null
    }

    fun applyAuthRequiredErrorState(err: Throwable?) {
        setAuthState(AuthState.Required)
        setVisibleError(null)
        authError = normalizeAuthThrowable(err, ::normalizeAuthError)
    }

    fun setAuthRequired(err: Throwable? = null) {
        clearActiveStreamJob()
        val resetState = buildAuthRequiredSessionReset()
        applyAuthRequiredSessionReset(resetState)
        applyAuthRequiredErrorState(err)
    }

    fun applyWorkspaceSnapshotIfPresent(snapshot: WorkspaceSnapshot?) {
        snapshot?.let(::applyWorkspaceSnapshot)
    }

    fun applyAuthRequiredErrorIfPresent(err: Throwable?) {
        err?.let(::setAuthRequired)
    }

    fun applyVisibleErrorIfPresent(message: String?) {
        message?.let(::setVisibleError)
    }

    fun applyAuthRefreshResult(authRefreshResult: AuthRefreshResult) {
        if (authRefreshResult.resolvedBaseUrl != appApiBaseUrl) {
            setAppApiBaseUrl(authRefreshResult.resolvedBaseUrl)
        }
        if (authRefreshResult.authRequiredError != null) {
            setAuthRequired(authRefreshResult.authRequiredError)
            return
        }
        applyAuthSessionState(
            providers = authRefreshResult.providers,
            user = authRefreshResult.user,
            state = authRefreshResult.authState
        )
        applyWorkspaceSnapshotIfPresent(authRefreshResult.workspaceSnapshot)
    }

    fun applyWorkspaceLoadResult(workspaceResult: WorkspaceLoadResult) {
        applyWorkspaceSnapshotIfPresent(workspaceResult.snapshot)
        applyAuthRequiredErrorIfPresent(workspaceResult.authRequiredError)
        applyVisibleErrorIfPresent(workspaceResult.visibleError)
    }

    fun enterAuthCheckingState() {
        setAuthState(AuthState.Checking)
        authError = null
    }

    fun applyVisibleAppError(err: Throwable) {
        visibleAppError(err)?.let(::setVisibleError)
    }

    fun launchAppOperation(
        showLoading: Boolean = false,
        block: suspend () -> Unit
    ) {
        scope.launch {
            if (showLoading) {
                loading = true
            }
            setVisibleError(null)
            try {
                block()
            } finally {
                if (showLoading) {
                    loading = false
                }
            }
        }
    }

    fun launchVisibleErrorOperation(
        showLoading: Boolean = false,
        block: suspend () -> Unit
    ) {
        launchAppOperation(showLoading = showLoading) {
            try {
                block()
            } catch (err: Throwable) {
                applyVisibleAppError(err)
            }
        }
    }

    suspend fun refreshAuthState(loadOnSuccess: Boolean = false) {
        enterAuthCheckingState()
        val authRefreshResult = refreshAuthSession(
            currentBaseUrl = appApiBaseUrl,
            candidates = mergeApiCandidates(appApiBaseUrl, appApiCandidates),
            currentClient = client,
            buildClient = ::buildClient,
            loadOnSuccess = loadOnSuccess,
            targetContext = buildMetadataTargetContext(formFactor)
        )
        applyAuthRefreshResult(authRefreshResult)
    }

    fun refreshAuthAfterSuccessfulLogin() {
        scope.launch {
            refreshAuthState(loadOnSuccess = true)
        }
    }

    fun resetWorkspaceForBaseUrl(baseUrl: String) {
        val resetState = buildWorkspaceSessionReset()
        setAppApiBaseUrl(baseUrl)
        applyWorkspaceSessionReset(resetState)
        refreshAuthAfterSuccessfulLogin()
    }

    suspend fun completeOAuthLogin(code: String, state: String) {
        val authClient = resolveAuthClient()
        authClient.oauthCallback(OAuthCallbackInput(code = code, state = state))
        authWebUrl = null
        refreshAuthAfterSuccessfulLogin()
    }

    suspend fun requestOAuthSignInUrl(): String {
        return resolveOAuthInitiateUrl(resolveAuthClient().oauthInitiate())
    }

    fun setSavedLoginConfig(next: SavedLoginConfig) {
        savedLoginConfig = next
    }

    fun setShowSavedLoginSettings(show: Boolean) {
        showSavedLoginSettings = show
    }

    fun setAuthBusy(busy: Boolean) {
        authBusy = busy
    }

    fun setAuthError(message: String?) {
        authError = message
    }

    fun setAuthWebUrl(url: String?) {
        authWebUrl = url
    }

    fun authUiBindings(): AuthUiBindings {
        return AuthUiBindings(
            onAuthBusyChange = ::setAuthBusy,
            onAuthErrorChange = ::setAuthError,
            onAuthWebUrlChange = ::setAuthWebUrl
        )
    }

    fun openSavedLoginSettings() {
        setShowSavedLoginSettings(true)
    }

    fun dismissSavedLoginSettings() {
        setShowSavedLoginSettings(false)
    }

    fun dismissAuthWeb() {
        setAuthWebUrl(null)
    }

    fun closeArtifactPreview() {
        artifactPreview = null
    }

    fun savedLoginBindings(): SavedLoginBindings {
        return SavedLoginBindings(
            onSavedLoginConfigChange = ::setSavedLoginConfig,
            onShowSavedLoginSettingsChange = ::setShowSavedLoginSettings
        )
    }

    fun clearAuthSecrets() {
        clearSavedAuthSecrets(
            store = savedLoginStore,
            bindings = savedLoginBindings()
        )
    }

    fun refreshAuthFromUi() {
        launchAuthRefresh(
            scope = scope,
            loadOnSuccess = false,
            refreshAuthState = ::refreshAuthState
        )
    }

    fun startOAuthSignIn() {
        launchAuthSignIn(
            scope = scope,
            authBindings = authUiBindings(),
            requestAuthWebUrl = ::requestOAuthSignInUrl,
            normalizeAuthError = ::normalizeAuthError
        )
    }

    fun handleOAuthCallback(code: String, state: String) {
        launchAuthOperation(
            scope = scope,
            authBindings = authUiBindings(),
            runOperation = { completeOAuthLogin(code, state) },
            normalizeAuthError = ::normalizeAuthError
        )
    }

    fun retryAuthConnection() {
        launchAuthRefresh(
            scope = scope,
            loadOnSuccess = true,
            refreshAuthState = ::refreshAuthState
        )
    }

    fun resetConversation() {
        clearActiveStreamJob()
        val resetState = buildConversationResetState()
        applyConversationResetState(resetState)
    }

    suspend fun refreshRecentConversations() {
        val resolvedClient = resolveClient()
        applyRecentConversations(loadRecentConversations(resolvedClient))
    }

    fun applyPreparedConversationBinding(preparedBinding: PreparedConversationBinding) {
        activeConversationId = preparedBinding.conversationId
        conversationState = preparedBinding.state
        pendingApprovals = preparedBinding.pendingApprovals
        approvalEdits = preparedBinding.approvalEdits
        generatedFiles = preparedBinding.generatedFiles
        payloadPreviews = preparedBinding.payloadPreviews
        streamSnapshot = null
        streamedMarkdown = null
        if (preparedBinding.replaceTranscript) {
            transcript.clear()
            transcript.addAll(preparedBinding.transcriptEntries)
        }
    }

    fun applyConversationSnapshot(snapshot: ConversationStreamSnapshot) {
        streamSnapshot = snapshot
        if (snapshot.conversationId.isNotBlank()) {
            activeConversationId = snapshot.conversationId
        }
        streamedMarkdown = latestAssistantMarkdown(snapshot) ?: streamedMarkdown
        syncAssistantTranscript(transcript, snapshot)
    }

    fun handleConversationStreamError(err: Throwable) {
        applyVisibleAppError(err)
    }

    fun startConversationStream(client: AgentlyClient, conversationId: String) {
        streamJob = scope.launch {
            try {
                client.trackConversation(conversationId).collect(::applyConversationSnapshot)
            } catch (err: Throwable) {
                handleConversationStreamError(err)
            }
        }
    }

    suspend fun bindConversation(conversationId: String, replaceTranscript: Boolean) {
        val resolvedClient = resolveClient()
        streamJob?.cancelAndJoin()
        val preparedBinding = prepareConversationBinding(
            client = resolvedClient,
            conversationId = conversationId,
            replaceTranscript = replaceTranscript,
            approvalEdits = approvalEdits,
            transcriptBuilder = ::transcriptFromState
        )
        applyPreparedConversationBinding(preparedBinding)
        startConversationStream(resolvedClient, conversationId)
    }

    fun applyApprovalRefreshState(approvalState: ApprovalRefreshState) {
        pendingApprovals = approvalState.pendingApprovals
        approvalEdits = approvalState.approvalEdits
    }

    suspend fun refreshPendingApprovalsForActiveConversation() {
        val approvalState = refreshApprovalState(
            client = resolveClient(),
            conversationId = activeConversationId,
            approvalEdits = approvalEdits
        )
        applyApprovalRefreshState(approvalState)
    }

    fun handleApprovalEditChange(approvalId: String, fieldName: String, value: JsonElement) {
        approvalEdits = updateApprovalEdit(
            approvalEdits = approvalEdits,
            approvalId = approvalId,
            fieldName = fieldName,
            value = value
        )
    }

    fun handleApprovalDecision(approval: PendingToolApproval, action: String) {
        launchVisibleErrorOperation(showLoading = true) {
            val decision = buildApprovalDecisionRequest(
                approval = approval,
                action = action,
                approvalJson = approvalJson,
                approvalEdits = approvalEdits
            )
            submitApprovalDecision(
                client = resolveClient(),
                decision = decision
            )
            val approvalState = refreshApprovalState(
                client = resolveClient(),
                conversationId = activeConversationId,
                approvalEdits = approvalEdits,
                clearedApprovalId = approval.id
            )
            applyApprovalRefreshState(approvalState)
        }
    }

    fun loadWorkspace() {
        launchAppOperation(showLoading = true) {
            val workspaceResult = loadWorkspaceSession(::resolveClient, buildMetadataTargetContext(formFactor))
            applyWorkspaceLoadResult(workspaceResult)
        }
    }

    fun applyComposerDraft(draft: ComposerDraftState) {
        setQueryText(draft.prompt)
        updateComposerAttachments(draft.attachments)
    }

    fun clearComposerInputs() {
        applyComposerDraft(clearComposerDraft())
    }

    fun restoreComposerDraftIfNeeded(draft: ComposerDraftState) {
        if (shouldRestoreComposerDraft(query, composerAttachments)) {
            applyComposerDraft(draft)
        }
    }

    fun applyQuerySuccessState(
        queryExecution: QueryExecutionResult,
        querySuccessState: QuerySuccessState,
        userEntryId: String?
    ) {
        metadata = querySuccessState.metadata
        activeConversationId = queryExecution.conversationId
        updateChatEntryDeliveryState(transcript, userEntryId, null)
        clearComposerInputs()
        activeConversationId = querySuccessState.activeConversationId
        result = querySuccessState.result
        querySuccessState.streamedMarkdown?.let { markdown ->
            streamedMarkdown = markdown
            syncAssistantResult(transcript, querySuccessState.result?.messageId, markdown)
        }
        generatedFiles = querySuccessState.generatedFiles
        pendingApprovals = querySuccessState.pendingApprovals
        approvalEdits = querySuccessState.approvalEdits
    }

    fun resetQueryResponseState() {
        result = null
        streamSnapshot = null
        streamedMarkdown = null
    }

    fun currentComposerDraft(): ComposerDraftState {
        return ComposerDraftState(
            prompt = query,
            attachments = composerAttachments
        )
    }

    fun handleQueryFailure(
        userEntryId: String?,
        draftToRestore: ComposerDraftState,
        err: Throwable
    ) {
        updateChatEntryDeliveryState(
            transcript,
            userEntryId,
            "failed"
        )
        restoreComposerDraftIfNeeded(draftToRestore)
        applyVisibleAppError(err)
    }

    fun runQuery() {
        launchAppOperation(showLoading = true) {
            var userEntryId: String? = null
            val currentDraft = currentComposerDraft()
            try {
                val resolvedClient = resolveClient()
                streamJob?.cancelAndJoin()
                resetQueryResponseState()
                val preparedQuerySubmission = prepareQuerySubmission(
                    draft = currentDraft,
                    timestampMs = System.currentTimeMillis()
                )
                userEntryId = preparedQuerySubmission.entryId
                transcript.add(preparedQuerySubmission.pendingEntry)
                clearComposerInputs()

                val queryExecution = executeQueryTurn(
                    client = resolvedClient,
                    metadata = metadata,
                    activeConversationId = activeConversationId,
                    effectiveAgentId = effectiveAgentId,
                    prompt = preparedQuerySubmission.effectivePrompt,
                    attachments = currentDraft.attachments,
                    queryContext = buildClientQueryContext(formFactor),
                    targetContext = buildMetadataTargetContext(formFactor)
                )
                val querySuccessState = buildQuerySuccessState(
                    execution = queryExecution,
                    approvalEdits = approvalEdits
                )
                bindConversation(queryExecution.conversationId, replaceTranscript = false)
                applyQuerySuccessState(
                    queryExecution = queryExecution,
                    querySuccessState = querySuccessState,
                    userEntryId = userEntryId
                )
                refreshRecentConversations()
            } catch (err: Throwable) {
                handleQueryFailure(userEntryId, currentDraft, err)
            }
        }
    }

    fun openGeneratedFile(file: GeneratedFileEntry) {
        launchVisibleErrorOperation {
            val downloaded = resolveClient().downloadGeneratedFile(file.id)
            artifactPreview = buildArtifactPreview(file, downloaded)
        }
    }

    fun setCurrentScreen(screen: AppScreen) {
        currentScreen = screen
    }

    fun showChatScreen() {
        setCurrentScreen(AppScreen.Chat)
    }

    fun applySettingsTransition(transition: SettingsApplyTransition) {
        preferredAgentId = transition.preferredAgentId
        persistAppSettings(
            store = appSettingsStore,
            configuredBaseUrl = configuredAppApiBaseUrl,
            nextBaseUrl = transition.resolvedBaseUrl,
            nextPreferredAgentId = transition.preferredAgentId
        )
        if (transition.requiresWorkspaceReset) {
            resetWorkspaceForBaseUrl(transition.resolvedBaseUrl)
        }
        showChatScreen()
    }

    suspend fun bootstrapWorkspaceSession() {
        val workspaceResult = loadWorkspaceSession(::resolveClient, buildMetadataTargetContext(formFactor))
        applyWorkspaceLoadResult(workspaceResult)
    }

    fun runInitialAuthRefresh() {
        refreshAuthAfterSuccessfulLogin()
    }

    fun setWorkspaceBootstrapRequested(requested: Boolean) {
        workspaceBootstrapRequested = requested
    }

    fun disposeStreamJob() {
        clearActiveStreamJob()
    }

    fun updateQuery(value: String) {
        query = value
    }

    fun applySavedLoginSettings(next: SavedLoginConfig) {
        persistSavedLoginConfig(
            store = savedLoginStore,
            next = next,
            bindings = savedLoginBindings()
        )
    }

    AppEffects(
        forgeRuntime = forgeRuntime,
        isTablet = isTablet,
        authState = authState,
        metadataLoaded = metadata != null,
        recentConversationCount = recentConversations.size,
        loading = loading,
        workspaceBootstrapRequested = workspaceBootstrapRequested,
        onWorkspaceBootstrapRequestedChange = ::setWorkspaceBootstrapRequested,
        onWorkspaceBootstrap = ::bootstrapWorkspaceSession,
        onSetCurrentScreen = ::setCurrentScreen,
        onLoadWorkspace = ::loadWorkspace,
        onResetConversation = ::resetConversation,
        onDisposeStreamJob = ::disposeStreamJob,
        onInitialAuthRefresh = ::runInitialAuthRefresh,
        onSetAuthRequired = ::setAuthRequired
    )

    fun applySettings(nextBaseUrl: String, nextPreferredAgentId: String, nextSavedLoginConfig: SavedLoginConfig) {
        val transition = buildSettingsApplyTransition(
            configuredBaseUrl = configuredAppApiBaseUrl,
            currentBaseUrl = appApiBaseUrl,
            nextBaseUrl = nextBaseUrl,
            nextPreferredAgentId = nextPreferredAgentId
        )
        applySavedLoginSettings(nextSavedLoginConfig)
        applySettingsTransition(transition)
    }

    fun resetAppOverrides() {
        val transition = buildResetOverridesTransition(
            configuredBaseUrl = configuredAppApiBaseUrl,
            currentBaseUrl = appApiBaseUrl
        )
        applySettingsTransition(transition)
    }

    fun saveSavedLoginSettings(next: SavedLoginConfig) {
        persistSavedLoginConfig(
            store = savedLoginStore,
            next = next,
            bindings = savedLoginBindings(),
            dismissSettings = true
        )
    }

    fun clearSavedLoginSettings() {
        clearSavedLoginConfig(
            store = savedLoginStore,
            bindings = savedLoginBindings(),
            dismissSettings = true
        )
    }

    fun selectConversation(conversationId: String, navigateToChat: Boolean = false) {
        launchVisibleErrorOperation(showLoading = true) {
            bindConversation(conversationId, replaceTranscript = true)
            if (navigateToChat) {
                showChatScreen()
            }
        }
    }

    val callbacks = buildAppUiCallbacks(
        currentScreenProvider = { currentScreen },
        setCurrentScreen = ::setCurrentScreen,
        onRefreshWorkspace = ::loadWorkspace,
        onNewConversation = ::resetConversation,
        onSelectConversation = ::selectConversation,
        onApprovalEditChange = ::handleApprovalEditChange,
        onApprovalDecision = ::handleApprovalDecision,
        onOpenFile = ::openGeneratedFile,
        onClosePreview = ::closeArtifactPreview,
        onQueryChange = ::updateQuery,
        onRunQuery = ::runQuery,
        onRefreshAuth = ::refreshAuthFromUi,
        onSaveSettings = ::applySettings,
        onResetAppOverrides = ::resetAppOverrides,
        onClearAuthSecrets = ::clearAuthSecrets,
        onAuthSignIn = ::startOAuthSignIn,
        onManageSavedLogin = ::openSavedLoginSettings,
        onAuthRetry = ::retryAuthConnection,
        onDismissAuthWeb = ::dismissAuthWeb,
        onOAuthCallback = ::handleOAuthCallback,
        onDismissSavedLoginSettings = ::dismissSavedLoginSettings,
        onSaveSavedLoginSettings = ::saveSavedLoginSettings,
        onClearSavedLoginSettings = ::clearSavedLoginSettings
    )

    AppBody(
        authState = authState,
        currentScreen = currentScreen,
        isTablet = isTablet,
        loading = loading,
        configuredAppApiBaseUrl = configuredAppApiBaseUrl,
        appApiBaseUrl = appApiBaseUrl,
        metadata = metadata,
        preferredAgentId = preferredAgentId,
        savedLoginConfig = savedLoginConfig,
        authBusy = authBusy,
        authError = authError,
        error = error,
        authProviders = authProviders,
        authUser = authUser,
        authWebUrl = authWebUrl,
        showSavedLoginSettings = showSavedLoginSettings,
        recentConversations = recentConversations,
        activeConversationId = activeConversationId,
        streamSnapshot = streamSnapshot,
        transcript = transcript,
        pendingApprovals = pendingApprovals,
        generatedFiles = generatedFiles,
        artifactPreview = artifactPreview,
        client = client,
        forgeRuntime = forgeRuntime,
        approvalJson = approvalJson,
        approvalEdits = approvalEdits,
        query = query,
        composerAttachments = composerAttachments,
        mediaController = mediaController,
        callbacks = callbacks
    )
}

internal fun buildApiCandidates(configuredBaseUrl: String): List<String> {
    val trimmed = configuredBaseUrl.trim().ifBlank { "http://10.0.2.2:9393" }
    val parsed = runCatching { URI(trimmed) }.getOrNull()
    val scheme = parsed?.scheme?.takeIf { it.isNotBlank() } ?: "http"
    val host = parsed?.host?.trim().orEmpty()
    val port = when {
        parsed?.port != null && parsed.port > 0 -> parsed.port
        scheme.equals("https", ignoreCase = true) -> 443
        else -> 80
    }
    val path = parsed?.rawPath?.takeIf { it.isNotBlank() && it != "/" }.orEmpty()
    val candidates = mutableListOf(
        trimmed,
        "$scheme://10.0.2.2:$port$path",
        "$scheme://10.0.3.2:$port$path",
        "$scheme://localhost:$port$path",
        "$scheme://127.0.0.1:$port$path"
    )
    if (
        host.isNotBlank() &&
        !host.equals("localhost", ignoreCase = true) &&
        host != "127.0.0.1" &&
        host != "10.0.2.2" &&
        host != "10.0.3.2"
    ) {
        candidates += "$scheme://$host:$port$path"
    }
    return candidates.distinct()
}

internal fun mergeApiCandidates(
    currentBaseUrl: String,
    configuredCandidates: List<String>
): List<String> {
    return buildList {
        add(currentBaseUrl)
        addAll(configuredCandidates)
        addAll(buildApiCandidates(currentBaseUrl))
    }.distinct()
}

internal enum class AppScreen {
    Chat,
    History,
    Settings
}

private fun buildClientQueryContext(formFactor: String): Map<String, JsonElement> {
    val clientKind = when (formFactor) {
        "tablet" -> "tablet"
        else -> "mobile"
    }
    return mapOf(
        "client" to buildJsonObject {
            put("kind", JsonPrimitive(clientKind))
            put("platform", JsonPrimitive("android"))
            put("formFactor", JsonPrimitive(formFactor))
            put("surface", JsonPrimitive("app"))
            put(
                "capabilities",
                buildJsonArray {
                    add(JsonPrimitive("markdown"))
                    add(JsonPrimitive("chart"))
                    add(JsonPrimitive("attachments"))
                    add(JsonPrimitive("camera"))
                    add(JsonPrimitive("voice"))
                }
            )
        }
    )
}

internal fun buildAndroidTargetCapabilities(): List<String> {
    return listOf("markdown", "chart", "attachments", "camera", "voice")
}

internal fun buildForgeTargetContext(formFactor: String): ForgeTargetContext {
    return ForgeTargetContext(
        platform = "android",
        formFactor = formFactor,
        capabilities = buildAndroidTargetCapabilities().toSet()
    )
}

internal fun buildMetadataTargetContext(formFactor: String): MetadataTargetContext {
    return MetadataTargetContext(
        platform = "android",
        formFactor = formFactor,
        surface = "app",
        capabilities = buildAndroidTargetCapabilities()
    )
}
