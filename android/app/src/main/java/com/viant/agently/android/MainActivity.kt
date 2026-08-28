package com.viant.agently.android

import android.content.Intent
import android.graphics.Bitmap
import android.os.Bundle
import android.net.Uri
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.core.content.FileProvider
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
import com.viant.agentlysdk.AttachSessionInput
import com.viant.agentlysdk.AuthProvider
import com.viant.agentlysdk.AuthUser
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.CreateConversationInput
import com.viant.agentlysdk.CreateSessionInput
import com.viant.agentlysdk.DecideToolApprovalInput
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.DownloadFileOutput
import com.viant.agentlysdk.EndpointConfig
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.Goal
import com.viant.agentlysdk.ListPendingToolApprovalsInput
import com.viant.agentlysdk.ListLookupRegistryInput
import com.viant.agentlysdk.LookupRegistryEntry
import com.viant.agentlysdk.MetadataTargetContext
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.QueryAttachment
import com.viant.agentlysdk.QueryInput
import com.viant.agentlysdk.QueryOutput
import com.viant.agentlysdk.OAuthCallbackInput
import com.viant.agentlysdk.OAuthInitiateInput
import com.viant.agentlysdk.UploadFileInput
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.listLookupRegistry
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.forgeandroid.runtime.ContentDef
import com.viant.forgeandroid.runtime.ContainerDef
import com.viant.forgeandroid.runtime.DataSourceDef
import com.viant.forgeandroid.runtime.ActionHookRuntime
import com.viant.forgeandroid.runtime.ForgeTargetContext
import com.viant.forgeandroid.runtime.SchemaBasedFormDef
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.ItemDef
import com.viant.forgeandroid.runtime.OptionDef
import com.viant.forgeandroid.runtime.ViewDef
import com.viant.forgeandroid.runtime.WindowMetadata
import com.viant.forgeandroid.ui.ContainerRenderer
import com.viant.forgeandroid.ui.MarkdownRenderer
import kotlinx.coroutines.Job
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import java.text.SimpleDateFormat
import java.io.File
import java.util.Date
import java.util.Locale
import java.time.OffsetDateTime

class MainActivity : ComponentActivity() {
    private val oauthCallbackUri = MutableStateFlow<Uri?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        handleIncomingOAuthCallback(intent)
        setContent {
            AgentlyTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    AgentlyApp(oauthCallbackUri)
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handleIncomingOAuthCallback(intent)
    }

    private fun handleIncomingOAuthCallback(intent: Intent?) {
        val uri = intent?.data ?: return
        if (uri.scheme == "agently-android" && uri.host == "oauth" && uri.path == "/callback") {
            oauthCallbackUri.value = uri
        }
    }
}

private const val AUTH_LOG_TAG = "AgentlyAuth"

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AgentlyApp(oauthCallbackUriFlow: MutableStateFlow<Uri?>) {
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val configuration = LocalConfiguration.current
    val formFactor = if (configuration.smallestScreenWidthDp >= 600) "tablet" else "phone"
    val isTablet = formFactor == "tablet"
    val conversationPolicy = remember(formFactor) { conversationLoadPolicy(formFactor) }
    val configuredAppApiBaseUrl = BuildConfig.APP_API_BASE_URL
    val appSettingsStore = remember(context) { AppSettingsStore(context.applicationContext) }
    val storedAppSettings = remember(appSettingsStore) { appSettingsStore.load() }
    val preferExplicitBuildEndpoint = BuildConfig.DEBUG && BuildConfig.APP_API_BASE_URL_EXPLICIT
    val initialHasWorkspaceEndpointSelection = remember(storedAppSettings, preferExplicitBuildEndpoint) {
        hasInitialWorkspaceEndpointSelection(
            configuredBaseUrl = configuredAppApiBaseUrl,
            storedSettings = storedAppSettings,
            preferExplicitBuildEndpoint = preferExplicitBuildEndpoint
        )
    }
    var hasWorkspaceEndpointSelection by remember {
        mutableStateOf(initialHasWorkspaceEndpointSelection)
    }
    var appApiBaseUrl by remember {
        mutableStateOf(
            resolveInitialApiBaseUrl(
                configuredBaseUrl = configuredAppApiBaseUrl,
                storedSettings = storedAppSettings,
                preferExplicitBuildEndpoint = preferExplicitBuildEndpoint
            )
        )
    }
    var preferredAgentId by remember { mutableStateOf(storedAppSettings.preferredAgentId) }
    val savedLoginStore = remember(context) { SavedLoginStoreImpl(context.applicationContext) }
    val storedSavedLoginConfig = remember(savedLoginStore) {
        val stored = savedLoginStore.load()
        if (BuildConfig.DEBUG) {
            stored.withBootstrapDefaults(
                oobSecretRef = BuildConfig.BOOTSTRAP_OOB_SECRET_REF
            )
        } else {
            stored
        }
    }
    var savedLoginConfig by remember { mutableStateOf(storedSavedLoginConfig) }
    var authSessionId by remember { mutableStateOf<String?>(null) }
    val sessionCookieJar = remember(context) { AppSessionCookieJar(context.applicationContext) }
    val appHttpClient = remember(sessionCookieJar) { appSessionHttpClient(sessionCookieJar) }
    val appLongRunningHttpClient = remember(sessionCookieJar) { appLongRunningHttpClient(sessionCookieJar) }
    val appStreamHttpClient = remember(sessionCookieJar) { appStreamHttpClient(sessionCookieJar) }
    val forgeTargetContext = remember(formFactor) { buildForgeTargetContext(formFactor) }
    fun buildClient(baseUrl: String): AgentlyClient = AgentlyClient(
        endpoints = mapOf(
            "appAPI" to EndpointConfig(
                baseUrl = baseUrl,
                httpClient = appHttpClient,
                longRunningHttpClient = appLongRunningHttpClient,
                streamHttpClient = appStreamHttpClient
            )
        )
    )
    val client = remember(appApiBaseUrl) { buildClient(appApiBaseUrl) }
    var loading by remember { mutableStateOf(false) }
    var metadata by remember { mutableStateOf<WorkspaceMetadata?>(null) }
    var query by remember { mutableStateOf("") }
    var composerAttachments by remember { mutableStateOf<List<ComposerAttachmentDraft>>(emptyList()) }
    var composerLookupRegistry by remember { mutableStateOf<List<LookupRegistryEntry>>(emptyList()) }
    var composerLookupSelections by remember { mutableStateOf<Map<String, ComposerLookupSelection>>(emptyMap()) }
    var activeComposerLookupOccurrence by remember { mutableStateOf<ComposerLookupOccurrence?>(null) }
    var result by remember { mutableStateOf<QueryOutput?>(null) }
    var streamSnapshot by remember { mutableStateOf<ConversationStreamSnapshot?>(null) }
    var streamedMarkdown by remember { mutableStateOf<String?>(null) }
    var activeConversationId by remember { mutableStateOf<String?>(null) }
    var conversationState by remember { mutableStateOf<ConversationStateResponse?>(null) }
    var activeGoal by remember { mutableStateOf<Goal?>(null) }
    var recentConversations by remember { mutableStateOf<List<Conversation>>(emptyList()) }
    var openingConversationId by remember { mutableStateOf<String?>(null) }
    var currentScreen by remember { mutableStateOf(AppScreen.Chat) }
    var scheduleHistoryFilter by remember { mutableStateOf<ScheduleHistoryFilter?>(null) }
    var pendingApprovals by remember { mutableStateOf<List<PendingToolApproval>>(emptyList()) }
    var generatedFiles by remember { mutableStateOf<List<GeneratedFileEntry>>(emptyList()) }
    var payloadPreviews by remember { mutableStateOf<Map<String, ArtifactPreview>>(emptyMap()) }
    var artifactPreview by remember { mutableStateOf<ArtifactPreview?>(null) }
    var streamJob by remember { mutableStateOf<Job?>(null) }
    var postTurnRefreshJob by remember { mutableStateOf<Job?>(null) }
    var stoppingTurnId by remember { mutableStateOf<String?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    var approvalEdits by remember { mutableStateOf<Map<String, Map<String, JsonElement>>>(emptyMap()) }
    val transcript = remember { mutableListOf<ChatEntry>().toMutableStateList() }
    val approvalJson = remember { Json { ignoreUnknownKeys = true } }
    LaunchedEffect(context) {
        ActionHookRuntime.initialize(context.applicationContext)
    }
    val forgeRuntime = remember(scope, forgeTargetContext, appApiBaseUrl) {
        ForgeRuntime(
            endpoints = emptyMap(),
            scope = scope,
            targetContext = forgeTargetContext
        ).also { runtime ->
            runtime.registerWindowMetadataLoader(
                makeForgeAgentlyWindowMetadataLoader(client, forgeTargetContext)
            )
            runtime.registerDataSourceLoader(makeForgeAgentlyDataSourceLoader(client))
            registerScheduleHandlers(runtime, client) { filter ->
                scheduleHistoryFilter = filter
                currentScreen = AppScreen.History
            }
        }
    }
    val uiBridge = remember(appApiBaseUrl, forgeRuntime) {
        AndroidUIBridgeClient(
            context = context.applicationContext,
            client = client,
            scope = scope,
            snapshotProvider = {
                buildAndroidUIBridgeSnapshot(
                    activeConversationId = activeConversationId,
                    forgeRuntime = forgeRuntime
                )
            },
            commandHandler = { method, params ->
                val commandResult = handleAndroidUIBridgeCommand(
                    method = method,
                    params = params,
                    forgeRuntime = forgeRuntime
                )
                if (
                    method == "ui.window.open" &&
                    (commandResult["ok"] as? JsonPrimitive)?.booleanOrNull == true
                ) {
                    loading = false
                    markLatestSubmittedUserEntryDelivered(transcript)
                    error = null
                }
                commandResult
            }
        )
    }
    var authState by remember { mutableStateOf(AuthState.Checking) }
    var authProviders by remember { mutableStateOf<List<AuthProvider>>(emptyList()) }
    var authUser by remember { mutableStateOf<AuthUser?>(null) }
    var authWebUrl by remember { mutableStateOf<String?>(null) }
    var authBusy by remember { mutableStateOf(false) }
    var authError by remember { mutableStateOf<String?>(null) }
    var authInteractiveFailure by remember { mutableStateOf(false) }
    val pendingOAuthCallback by oauthCallbackUriFlow.collectAsState()
    var workspaceBootstrapRequested by remember { mutableStateOf(false) }
    var bootstrapOobSignInAttempted by remember { mutableStateOf(false) }
    val effectiveAgentId = resolvePreferredAgentId(preferredAgentId, metadata)
    val composerLookupOccurrences = remember(query, composerLookupRegistry) {
        parseComposerLookupOccurrences(query, composerLookupRegistry)
    }

    DisposableEffect(uiBridge) {
        onDispose {
            uiBridge.stop()
        }
    }

    LaunchedEffect(authState, uiBridge) {
        if (authState == AuthState.Ready) {
            uiBridge.start()
        } else {
            uiBridge.stop()
        }
    }

    fun updateComposerAttachments(attachments: List<ComposerAttachmentDraft>) {
        composerAttachments = attachments
    }

    fun setQueryText(value: String) {
        query = value
        composerLookupSelections = pruneComposerLookupSelections(
            query = value,
            registry = composerLookupRegistry,
            selections = composerLookupSelections
        )
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
            candidates = listOf(appApiBaseUrl),
            currentClient = client,
            buildClient = ::buildClient,
            onResolvedBaseUrl = ::setAppApiBaseUrl,
            targetContext = buildMetadataTargetContext(formFactor)
        )
    }

    suspend fun resolveAuthClient(): AgentlyClient {
        return resolveAuthClientWithFallback(
            currentBaseUrl = appApiBaseUrl,
            candidates = listOf(appApiBaseUrl),
            currentClient = client,
            buildClient = ::buildClient,
            onResolvedBaseUrl = ::setAppApiBaseUrl
        )
    }

    LaunchedEffect(authState, effectiveAgentId, client) {
        val agentId = effectiveAgentId?.trim().orEmpty()
        if (authState != AuthState.Ready || agentId.isBlank()) {
            composerLookupRegistry = emptyList()
            composerLookupSelections = emptyMap()
            activeComposerLookupOccurrence = null
            return@LaunchedEffect
        }
        try {
            composerLookupRegistry = resolveClient()
                .listLookupRegistry(ListLookupRegistryInput(context = "chat-composer:$agentId"))
                .entries
            composerLookupSelections = pruneComposerLookupSelections(
                query = query,
                registry = composerLookupRegistry,
                selections = composerLookupSelections
            )
        } catch (_: Throwable) {
            composerLookupRegistry = emptyList()
            composerLookupSelections = emptyMap()
            activeComposerLookupOccurrence = null
        }
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
            webUrl = resetState.authWebUrl,
            busy = resetState.authBusy,
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
        postTurnRefreshJob?.cancel()
        postTurnRefreshJob = null
    }

    fun applyAuthRequiredErrorState(err: Throwable?) {
        if (BuildConfig.DEBUG) {
            Log.w(
                AUTH_LOG_TAG,
                "Auth state demoted to Required: ${err?.javaClass?.simpleName ?: "unknown"}: ${err?.message.orEmpty()}"
            )
        }
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

    fun setWorkspaceUnavailable(err: Throwable) {
        clearActiveStreamJob()
        val resetState = buildAuthRequiredSessionReset()
        applyAuthRequiredSessionReset(resetState)
        setAuthState(AuthState.Unavailable)
        setVisibleError(null)
        authError = normalizeAuthThrowable(err, ::normalizeAuthError)
        if (BuildConfig.DEBUG) {
            Log.w(
                AUTH_LOG_TAG,
                "Workspace unavailable: ${err.javaClass.simpleName}: ${err.message.orEmpty()}"
            )
        }
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
        if (BuildConfig.DEBUG) {
            Log.d(
                AUTH_LOG_TAG,
                "Auth refresh result state=${authRefreshResult.authState} userPresent=${authRefreshResult.user != null} " +
                    "providers=${authRefreshResult.providers.joinToString(",") { it.type }} " +
                    "hasWorkspace=${authRefreshResult.workspaceSnapshot != null} " +
                    "authError=${authRefreshResult.error?.message.orEmpty()}"
            )
        }
        if (authRefreshResult.error != null) {
            if (authRefreshResult.authState == AuthState.Required) {
                setAuthRequired(authRefreshResult.error)
            } else {
                setWorkspaceUnavailable(authRefreshResult.error)
            }
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
        val previousAuthState = authState
        val previousAuthUser = authUser
        val previousSessionId = authSessionId
        enterAuthCheckingState()
        val authRefreshResult = refreshAuthSession(
            currentBaseUrl = appApiBaseUrl,
            candidates = listOf(appApiBaseUrl),
            currentClient = client,
            buildClient = ::buildClient,
            loadOnSuccess = loadOnSuccess,
            targetContext = buildMetadataTargetContext(formFactor)
        )
        if (shouldIgnoreStaleAuthRefreshFailure(
                startedSessionId = previousSessionId,
                currentSessionId = authSessionId,
                currentUser = authUser,
                err = authRefreshResult.error
            )
        ) {
            if (BuildConfig.DEBUG) {
                Log.w(
                    AUTH_LOG_TAG,
                    "Ignoring stale auth refresh failure after session changed: " +
                        authRefreshResult.error?.message.orEmpty()
                )
            }
            setAuthState(AuthState.Ready)
            return
        }
        if (shouldPreserveAuthenticatedSessionOnAuthRefreshFailure(
                previousAuthState = previousAuthState,
                previousUser = previousAuthUser,
                previousSessionId = previousSessionId,
                err = authRefreshResult.error
            )
        ) {
            if (BuildConfig.DEBUG) {
                Log.w(
                    AUTH_LOG_TAG,
                    "Preserving authenticated session after transient auth refresh failure: " +
                        authRefreshResult.error?.message.orEmpty()
                )
            }
            setAuthState(previousAuthState)
            return
        }
        applyAuthRefreshResult(authRefreshResult)
    }

    fun refreshAuthAfterSuccessfulLogin() {
        scope.launch {
            refreshAuthState(loadOnSuccess = true)
        }
    }

    fun resetWorkspaceForBaseUrl(baseUrl: String) {
        val resetState = buildWorkspaceSessionReset()
        authSessionId = null
        sessionCookieJar.clear()
        bootstrapOobSignInAttempted = false
        setAppApiBaseUrl(baseUrl)
        applyWorkspaceSessionReset(resetState)
        refreshAuthAfterSuccessfulLogin()
    }

    suspend fun completeOAuthLogin(code: String, state: String) {
        val authClient = resolveAuthClient()
        val output = authClient.oauthMobileCallback(OAuthCallbackInput(code = code, state = state))
        authSessionId = output.sessionId?.trim()?.takeIf { it.isNotBlank() }
        authWebUrl = null
        refreshAuthAfterSuccessfulLogin()
    }

    suspend fun requestOAuthSignInUrl(): String {
        val authUrl = resolveOAuthInitiateUrl(
            resolveAuthClient().oauthMobileInitiate(
                OAuthInitiateInput(redirectURI = AndroidOAuthRedirectURI)
            )
        )
        require(authUrlUsesRedirect(authUrl, AndroidOAuthRedirectURI)) {
            "This workspace returned a web OAuth callback. Mobile sign-in needs $AndroidOAuthRedirectURI."
        }
        return authUrl
    }

    suspend fun runOobSignIn() {
        val secretRef = savedLoginConfig.oobSecretRef.trim()
        require(secretRef.isNotBlank()) { "Add an OOB secret reference in Settings before starting OOB sign-in." }
        if (BuildConfig.DEBUG) {
            Log.d(AUTH_LOG_TAG, "Starting OOB sign-in against ${appApiBaseUrl.trim()} with configured secret ref")
        }
        val output = resolveAuthClient().oobLogin(
            com.viant.agentlysdk.OOBLoginInput(secretsURL = secretRef)
        )
        authSessionId = output.sessionId?.trim()?.takeIf { it.isNotBlank() }
        if (BuildConfig.DEBUG) {
            Log.d(AUTH_LOG_TAG, "OOB sign-in completed sessionPresent=${authSessionId != null}")
        }
        refreshAuthAfterSuccessfulLogin()
    }

    suspend fun runDeveloperSessionSignIn(rawCredential: String) {
        val credential = normalizeDeveloperSessionCredential(rawCredential)
        require(credential.isNotBlank()) { "Paste a session ID, cookie, or token." }
        val authClient = resolveAuthClient()

        val attachResult = runCatching {
            authClient.attachAuthSession(AttachSessionInput(sessionId = credential))
        }
        if (attachResult.isSuccess) {
            authSessionId = attachResult.getOrThrow().sessionId?.trim()?.takeIf { it.isNotBlank() }
                ?: credential
            refreshAuthAfterSuccessfulLogin()
            return
        }

        // Local OOB tooling returns the session cookie value itself. Some hosted
        // deployments intentionally do not expose the optional session/attach
        // endpoint, so honor the cookie formats accepted by the developer UI by
        // installing the value in the app's persistent cookie jar and verifying it.
        val cookieResult = runCatching {
            val installed = sessionCookieJar.installSession(appApiBaseUrl, credential)
            require(installed) {
                "Invalid workspace endpoint or session cookie"
            }
            if (BuildConfig.DEBUG) {
                Log.d(
                    AUTH_LOG_TAG,
                    "Installed local OOB session cookie for $appApiBaseUrl"
                )
            }
            authClient.authMe()
        }
        if (cookieResult.isSuccess) {
            authSessionId = credential
            refreshAuthAfterSuccessfulLogin()
            return
        }
        if (BuildConfig.DEBUG) {
            Log.w(
                AUTH_LOG_TAG,
                "Local OOB session cookie verification failed: " +
                    cookieResult.exceptionOrNull()?.message.orEmpty()
            )
        }
        sessionCookieJar.clear()

        val accessTokenResult = runCatching {
            authClient.createAuthSession(CreateSessionInput(accessToken = credential))
        }
        if (accessTokenResult.isSuccess) {
            authSessionId = accessTokenResult.getOrThrow().sessionId.trim().takeIf { it.isNotBlank() }
            refreshAuthAfterSuccessfulLogin()
            return
        }

        val idTokenResult = runCatching {
            authClient.createAuthSession(CreateSessionInput(idToken = credential))
        }
        if (idTokenResult.isSuccess) {
            authSessionId = idTokenResult.getOrThrow().sessionId.trim().takeIf { it.isNotBlank() }
            refreshAuthAfterSuccessfulLogin()
            return
        }

        throw IllegalStateException("Could not use that session ID or token.")
    }

    fun setSavedLoginConfig(next: SavedLoginConfig) {
        savedLoginConfig = next
    }

    fun setAuthBusy(busy: Boolean) {
        authBusy = busy
    }

    fun setAuthError(message: String?) {
        authError = message
    }

    fun setInteractiveAuthFailure(failed: Boolean) {
        authInteractiveFailure = failed
    }

    fun setAuthWebUrl(url: String?) {
        authWebUrl = url
    }

    fun authUiBindings(): AuthUiBindings {
        return AuthUiBindings(
            onAuthBusyChange = ::setAuthBusy,
            onAuthErrorChange = ::setAuthError,
            onAuthWebUrlChange = ::setAuthWebUrl,
            onInteractiveAuthFailureChange = ::setInteractiveAuthFailure
        )
    }

    fun dismissAuthWeb() {
        setAuthWebUrl(null)
    }

    fun closeArtifactPreview() {
        artifactPreview = null
    }

    fun savedLoginBindings(): SavedLoginBindings {
        return SavedLoginBindings(
            onSavedLoginConfigChange = ::setSavedLoginConfig
        )
    }

    fun clearAuthSecrets() {
        authSessionId = null
        sessionCookieJar.clear()
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

    fun startOobSignIn() {
        launchAuthOperation(
            scope = scope,
            authBindings = authUiBindings(),
            runOperation = ::runOobSignIn,
            normalizeAuthError = ::normalizeAuthError
        )
    }

    fun startDeveloperSessionSignIn(rawCredential: String) {
        launchAuthOperation(
            scope = scope,
            authBindings = authUiBindings(),
            runOperation = { runDeveloperSessionSignIn(rawCredential) },
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

    LaunchedEffect(pendingOAuthCallback) {
        val uri = pendingOAuthCallback ?: return@LaunchedEffect
        val code = uri.getQueryParameter("code").orEmpty()
        val state = uri.getQueryParameter("state").orEmpty()
        if (code.isNotBlank() && state.isNotBlank()) {
            handleOAuthCallback(code, state)
        }
        oauthCallbackUriFlow.value = null
    }

    LaunchedEffect(authState, authBusy, savedLoginConfig) {
        if (BuildConfig.DEBUG && BuildConfig.BOOTSTRAP_AUTO_OOB_SIGN_IN) {
            Log.d(
                AUTH_LOG_TAG,
                "Bootstrap OOB check state=$authState busy=$authBusy attempted=$bootstrapOobSignInAttempted hasSecret=${savedLoginConfig.hasStoredOobSecretRef}"
            )
        }
        if (shouldAttemptBootstrapOobSignIn(
                debugBuild = BuildConfig.DEBUG,
                bootstrapAutoOobSignIn = BuildConfig.BOOTSTRAP_AUTO_OOB_SIGN_IN,
                authState = authState,
                authBusy = authBusy,
                alreadyAttempted = bootstrapOobSignInAttempted,
                savedLoginConfig = savedLoginConfig
            )
        ) {
            bootstrapOobSignInAttempted = true
            if (BuildConfig.DEBUG) {
                Log.d(AUTH_LOG_TAG, "Bootstrap OOB sign-in triggered")
            }
            startOobSignIn()
        }
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
        activeGoal = null
    }

    suspend fun refreshRecentConversations() {
        val resolvedClient = resolveClient()
        applyRecentConversations(loadRecentConversations(resolvedClient))
    }

    fun applyPreparedConversationBinding(preparedBinding: PreparedConversationBinding) {
        val preserveStreamSnapshot = shouldPreserveConversationStreamSnapshot(
            targetConversationId = preparedBinding.conversationId,
            replaceTranscript = preparedBinding.replaceTranscript,
            streamSnapshot = streamSnapshot
        )
        activeConversationId = preparedBinding.conversationId
        conversationState = preparedBinding.state
        activeGoal = preparedBinding.goal
        pendingApprovals = preparedBinding.pendingApprovals
        approvalEdits = preparedBinding.approvalEdits
        generatedFiles = preparedBinding.generatedFiles
        payloadPreviews = preparedBinding.payloadPreviews
        if (!preserveStreamSnapshot) {
            streamSnapshot = null
        }
        streamedMarkdown = null
        if (preparedBinding.replaceTranscript) {
            transcript.clear()
            transcript.addAll(preparedBinding.transcriptEntries)
        }
    }

    fun applyConversationSnapshot(snapshot: ConversationStreamSnapshot) {
        val previousSnapshot = streamSnapshot
        val completedTurnId = previousSnapshot?.activeTurnId?.trim().orEmpty()
            .takeIf { it.isNotBlank() && snapshot.activeTurnId.isNullOrBlank() }
        if (completedTurnId != null) {
            if (!commitAssistantTurnFromSnapshot(transcript, snapshot, completedTurnId)) {
                previousSnapshot?.let {
                    commitAssistantTurnFromSnapshot(transcript, it, completedTurnId)
                }
            }
        }
        streamSnapshot = snapshot
        if (snapshot.conversationId.isNotBlank()) {
            activeConversationId = snapshot.conversationId
        }
        streamedMarkdown = latestAssistantMarkdown(snapshot) ?: streamedMarkdown
        if (streamSnapshotHasAcceptedActivity(snapshot)) {
            loading = false
            markLatestSubmittedUserEntryDelivered(transcript)
            setVisibleError(null)
        }
    }

    fun schedulePostTurnRefresh(conversationId: String) {
        postTurnRefreshJob?.cancel()
        postTurnRefreshJob = scope.launch {
            delay(350)
            if (activeConversationId != conversationId) {
                return@launch
            }
            val resolvedClient = resolveClient()
            val preparedBinding = prepareConversationBinding(
                client = resolvedClient,
                conversationId = conversationId,
                policy = conversationPolicy,
                replaceTranscript = false,
                approvalEdits = approvalEdits,
                transcriptBuilder = ::transcriptFromState
            )
            applyPreparedConversationBinding(preparedBinding)
            refreshRecentConversations()
        }
    }

    fun handleConversationStreamError(err: Throwable) {
        applyVisibleAppError(err)
    }

    fun startConversationStream(client: AgentlyClient, conversationId: String) {
        streamJob = scope.launch {
            var sawActiveTurn = false
            try {
                client.trackConversation(
                    conversationId,
                    maxResponseBytes = conversationPolicy.maxTranscriptResponseBytes
                ).collect { snapshot ->
                    val previousActiveTurnId = streamSnapshot?.activeTurnId
                    applyConversationSnapshot(snapshot)
                    if (!snapshot.activeTurnId.isNullOrBlank()) {
                        sawActiveTurn = true
                    } else if (!previousActiveTurnId.isNullOrBlank() || sawActiveTurn) {
                        sawActiveTurn = false
                        schedulePostTurnRefresh(snapshot.conversationId.ifBlank { conversationId })
                    }
                }
            } catch (err: Throwable) {
                handleConversationStreamError(err)
            }
        }
    }

    suspend fun bindConversation(conversationId: String, replaceTranscript: Boolean) {
        val resolvedClient = resolveClient()
        val keepCurrentStream = shouldKeepConversationStream(
            activeConversationId = activeConversationId,
            targetConversationId = conversationId,
            replaceTranscript = replaceTranscript,
            hasStreamJob = streamJob != null
        )
        if (!keepCurrentStream) {
            streamJob?.cancelAndJoin()
        }
        val preparedBinding = prepareConversationBinding(
            client = resolvedClient,
            conversationId = conversationId,
            policy = conversationPolicy,
            replaceTranscript = replaceTranscript,
            approvalEdits = approvalEdits,
            transcriptBuilder = ::transcriptFromState
        )
        recentConversations = ensureConversationPresentInRecentList(
            client = resolvedClient,
            conversations = recentConversations,
            conversationId = conversationId
        )
        applyPreparedConversationBinding(preparedBinding)
        if (!keepCurrentStream) {
            startConversationStream(resolvedClient, conversationId)
        }
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

    suspend fun handleGoalCommand(command: GoalCommandAction) {
        val conversationId = activeConversationId?.trim().orEmpty()
        if (conversationId.isBlank()) {
            setVisibleError("Open an existing conversation before using /goal.")
            return
        }
        val goalClient = resolveClient()
        when (command) {
            GoalCommandAction.Show -> {
                activeGoal = goalClient.getGoal(conversationId)
            }
            is GoalCommandAction.Set -> {
                val objective = command.objective.trim()
                if (objective.isBlank()) {
                    setVisibleError("Provide a goal objective after /goal set.")
                    return
                }
                try {
                    goalClient.createGoal(conversationId, com.viant.agentlysdk.CreateGoalInput(objective = objective))
                } catch (err: Throwable) {
                    val normalizedMessage = (err.message ?: "").lowercase()
                    val shouldUpdateExistingGoal = normalizedMessage.contains("goal already exists") ||
                        normalizedMessage.contains("failed: 409") ||
                        normalizedMessage.contains("status 409")
                    if (shouldUpdateExistingGoal) {
                        goalClient.updateGoal(conversationId, com.viant.agentlysdk.UpdateGoalInput(objective = objective))
                    } else {
                        throw err
                    }
                }
            }
            GoalCommandAction.Pause -> {
                goalClient.updateGoal(conversationId, com.viant.agentlysdk.UpdateGoalInput(status = "paused"))
            }
            GoalCommandAction.Resume -> {
                goalClient.updateGoal(conversationId, com.viant.agentlysdk.UpdateGoalInput(status = "active"))
            }
            GoalCommandAction.Clear -> {
                goalClient.clearGoal(conversationId)
            }
            GoalCommandAction.Help -> {
                setVisibleError("Goal commands: /goal show, /goal set <objective>, /goal pause, /goal resume, /goal clear")
                return
            }
        }
        bindConversation(conversationId, replaceTranscript = true)
        refreshRecentConversations()
        query = ""
        composerAttachments = emptyList()
        setVisibleError(null)
    }

    fun runQuery() {
        launchAppOperation(showLoading = true) {
            var userEntryId: String? = null
            var submittedPrompt: String? = null
            var submittedConversationId: String? = null
            var submittedClient: AgentlyClient? = null
            val currentDraft = currentComposerDraft()
            try {
                val rawPrompt = currentDraft.prompt.trim()
                val goalCommand = parseGoalCommand(rawPrompt)
                if (goalCommand != null) {
                    handleGoalCommand(goalCommand)
                    return@launchAppOperation
                }
                val lookupResolution = resolveComposerLookupSubmission(
                    query = rawPrompt,
                    registry = composerLookupRegistry,
                    selections = composerLookupSelections
                )
                lookupResolution.unresolvedRequiredLookup?.let { occurrence ->
                    activeComposerLookupOccurrence = occurrence
                    return@launchAppOperation
                }
                val resolvedDraft = currentDraft.copy(
                    prompt = lookupResolution.resolvedQuery ?: rawPrompt
                )
                val resolvedClient = resolveClient()
                submittedClient = resolvedClient
                streamJob?.cancelAndJoin()
                resetQueryResponseState()
                val preparedQuerySubmission = prepareQuerySubmission(
                    draft = resolvedDraft,
                    timestampMs = System.currentTimeMillis()
                )
                userEntryId = preparedQuerySubmission.entryId
                submittedPrompt = preparedQuerySubmission.effectivePrompt
                transcript.add(preparedQuerySubmission.pendingEntry)
                clearComposerInputs()

                val queryExecution = executeQueryTurn(
                    client = resolvedClient,
                    metadata = metadata,
                    activeConversationId = activeConversationId,
                    effectiveAgentId = effectiveAgentId,
                    prompt = preparedQuerySubmission.effectivePrompt,
                    attachments = resolvedDraft.attachments,
                    queryContext = buildClientQueryContext(
                        formFactor = formFactor,
                        uiClientId = uiBridge.ensureConnected()
                    ),
                    targetContext = buildMetadataTargetContext(formFactor),
                    onConversationReady = { conversationId ->
                        submittedConversationId = conversationId
                        activeConversationId = conversationId
                        uiBridge.publishSnapshotNow()
                        startConversationStream(resolvedClient, conversationId)
                    }
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
                val recoveredState = submittedClient?.let { recoveryClient ->
                    val conversationId = submittedConversationId ?: activeConversationId
                    val prompt = submittedPrompt
                    if (conversationId.isNullOrBlank() || prompt.isNullOrBlank()) {
                        null
                    } else {
                        var recovered: ConversationStateResponse? = null
                        for (attempt in 0 until 3) {
                            recovered = runCatching {
                                recoveryClient.getLiveState(
                                    conversationId = conversationId,
                                    includeFeeds = true,
                                    maxResponseBytes = conversationPolicy.maxTranscriptResponseBytes
                                )
                            }.getOrNull()?.takeIf { submittedTurnWasAccepted(it, prompt) }
                            if (recovered != null) break
                            if (attempt < 2) delay(350)
                        }
                        recovered
                    }
                }
                if (recoveredState != null) {
                    val conversationId = submittedConversationId ?: activeConversationId.orEmpty()
                    conversationState = recoveredState
                    updateChatEntryDeliveryState(transcript, userEntryId, null)
                    clearComposerInputs()
                    setVisibleError(null)
                    if (streamJob?.isActive != true && conversationId.isNotBlank()) {
                        submittedClient?.let { startConversationStream(it, conversationId) }
                    }
                    if (!hasPendingConversationTurn(recoveredState) && conversationId.isNotBlank()) {
                        bindConversation(conversationId, replaceTranscript = true)
                        refreshRecentConversations()
                    }
                } else {
                    handleQueryFailure(userEntryId, currentDraft, err)
                }
            }
        }
    }

    fun openDownloadedArtifactExternally(
        file: GeneratedFileEntry,
        downloaded: DownloadFileOutput
    ): Boolean {
        val name = downloaded.name ?: file.filename ?: "${file.id.take(12)}.pdf"
        val contentType = downloaded.contentType ?: file.mimeType
        val isPdf = contentType.equals("application/pdf", ignoreCase = true) ||
            name.endsWith(".pdf", ignoreCase = true)
        if (!isPdf) {
            return false
        }
        val downloadsDir = File(context.cacheDir, "downloads").apply { mkdirs() }
        val sanitizedName = sanitizeDownloadFileName(name).ifBlank { file.id.take(12) }
        val safeName = if (sanitizedName.endsWith(".pdf", ignoreCase = true)) {
            sanitizedName
        } else {
            "$sanitizedName.pdf"
        }
        val target = File(downloadsDir, safeName)
        target.writeBytes(downloaded.data)
        val uri = FileProvider.getUriForFile(context, "${context.packageName}.files", target)
        val intent = Intent(Intent.ACTION_VIEW)
            .setDataAndType(uri, "application/pdf")
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        return runCatching {
            context.startActivity(Intent.createChooser(intent, "Open PDF"))
            true
        }.getOrElse {
            false
        }
    }

    fun openGeneratedFile(file: GeneratedFileEntry) {
        launchVisibleErrorOperation {
            val downloaded = resolveClient().downloadGeneratedFile(file.id)
            if (openDownloadedArtifactExternally(file, downloaded)) {
                return@launchVisibleErrorOperation
            }
            artifactPreview = buildArtifactPreview(file, downloaded)
        }
    }

    fun openInlineReportPdf(exportRequest: Map<String, Any?>, onComplete: () -> Unit) {
        launchVisibleErrorOperation {
            try {
                val artifact = try {
                    exportReportRuntimePdf(
                        client = resolveClient(),
                        exportRequest = exportRequest,
                        conversationId = activeConversationId.orEmpty()
                    )
                } catch (err: Throwable) {
                    error(reportRuntimeExportErrorMessage(err))
                }
                if (!openDownloadedArtifactExternally(artifact.file, artifact.downloaded)) {
                    error("PDF export completed, but no PDF viewer was available.")
                }
            } finally {
                onComplete()
            }
        }
    }

    LaunchedEffect(forgeRuntime, client, activeConversationId) {
        registerReportRuntimeExportHandler(
            forgeRuntime = forgeRuntime,
            client = client,
            conversationIdProvider = { activeConversationId },
            onError = { error = it },
            openPdf = ::openDownloadedArtifactExternally
        )
    }

    fun setCurrentScreen(screen: AppScreen) {
        currentScreen = screen
    }

    fun showChatScreen() {
        setCurrentScreen(AppScreen.Chat)
    }

    fun applySettingsTransition(transition: SettingsApplyTransition) {
        preferredAgentId = transition.preferredAgentId
        hasWorkspaceEndpointSelection = true
        persistAppSettings(
            store = appSettingsStore,
            configuredBaseUrl = configuredAppApiBaseUrl,
            nextBaseUrl = transition.resolvedBaseUrl,
            nextPreferredAgentId = transition.preferredAgentId,
            hasWorkspaceEndpointSelection = true
        )
        if (transition.requiresWorkspaceReset) {
            resetWorkspaceForBaseUrl(transition.resolvedBaseUrl)
        }
        showChatScreen()
    }

    fun selectPreferredAgent(agentId: String?) {
        preferredAgentId = agentId?.trim().orEmpty()
        persistAppSettings(
            store = appSettingsStore,
            configuredBaseUrl = configuredAppApiBaseUrl,
            nextBaseUrl = appApiBaseUrl,
            nextPreferredAgentId = preferredAgentId,
            hasWorkspaceEndpointSelection = hasWorkspaceEndpointSelection
        )
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
        setQueryText(value)
    }

    fun cancelCurrentTurn() {
        val turnId = streamSnapshot?.activeTurnId?.trim().orEmpty()
        if (turnId.isEmpty() || stoppingTurnId != null) return
        stoppingTurnId = turnId
        launchVisibleErrorOperation {
            try {
                resolveClient().cancelTurn(turnId)
                clearActiveStreamJob()
                activeConversationId?.takeIf { it.isNotBlank() }?.let { conversationId ->
                    bindConversation(conversationId, replaceTranscript = true)
                }
            } finally {
                stoppingTurnId = null
            }
        }
    }

    fun clearComposerDraft() {
        setQueryText("")
        composerAttachments = emptyList()
        composerLookupSelections = emptyMap()
        activeComposerLookupOccurrence = null
    }

    fun selectStarterTask(prompt: String) {
        setQueryText(prompt)
        showChatScreen()
    }

    fun selectComposerLookup(occurrence: ComposerLookupOccurrence) {
        activeComposerLookupOccurrence = occurrence
    }

    fun applySavedLoginSettings(next: SavedLoginConfig) {
        persistSavedLoginConfig(
            store = savedLoginStore,
            next = next,
            bindings = savedLoginBindings()
        )
    }

    fun selectWorkspaceEndpoint(option: WorkspaceEndpointOption) {
        hasWorkspaceEndpointSelection = true
        val resolvedBaseUrl = normalizeApiBaseUrl(option.value)
        preferredAgentId = ""
        persistAppSettings(
            store = appSettingsStore,
            configuredBaseUrl = configuredAppApiBaseUrl,
            nextBaseUrl = resolvedBaseUrl,
            nextPreferredAgentId = "",
            hasWorkspaceEndpointSelection = true
        )
        resetWorkspaceForBaseUrl(resolvedBaseUrl)
    }

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

    if (!hasWorkspaceEndpointSelection) {
        WorkspaceSelectionScreen(onContinue = ::selectWorkspaceEndpoint)
        return
    }

    AppEffects(
        forgeRuntime = forgeRuntime,
        isTablet = isTablet,
        authState = authState,
        metadataLoaded = metadata != null,
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

    fun selectConversation(conversationId: String, navigateToChat: Boolean = false) {
        openingConversationId = conversationId
        launchVisibleErrorOperation(showLoading = true) {
            try {
                bindConversation(conversationId, replaceTranscript = true)
                if (navigateToChat) {
                    showChatScreen()
                }
            } finally {
                openingConversationId = null
            }
        }
    }

    fun deleteConversation(conversationId: String) {
        val normalized = conversationId.trim()
        if (normalized.isEmpty()) return
        launchVisibleErrorOperation(showLoading = true) {
            resolveClient().deleteConversation(normalized)
            if (activeConversationId == normalized) {
                resetConversation()
            }
            refreshRecentConversations()
        }
    }

    val callbacks = buildAppUiCallbacks(
        currentScreenProvider = { currentScreen },
        setCurrentScreen = ::setCurrentScreen,
        onRefreshWorkspace = ::loadWorkspace,
        onNewConversation = ::resetConversation,
        onSelectAgent = ::selectPreferredAgent,
        onOpenHistory = {
            scheduleHistoryFilter = null
            currentScreen = AppScreen.History
        },
        onBackFromHistory = {
            val returnToAutomation = scheduleHistoryFilter != null
            scheduleHistoryFilter = null
            if (returnToAutomation) {
                currentScreen = AppScreen.Automation
            } else {
                resetConversation()
                currentScreen = AppScreen.Chat
            }
        },
        onSelectConversation = { conversationId, navigateToChat ->
            if (navigateToChat) scheduleHistoryFilter = null
            selectConversation(conversationId, navigateToChat)
        },
        onDeleteConversation = ::deleteConversation,
        onApprovalEditChange = ::handleApprovalEditChange,
        onApprovalDecision = ::handleApprovalDecision,
        onOpenFile = ::openGeneratedFile,
        onOpenInlineReportPdf = ::openInlineReportPdf,
        onClosePreview = ::closeArtifactPreview,
        onQueryChange = ::updateQuery,
        onClearComposer = ::clearComposerDraft,
        onComposerLookupSelected = ::selectComposerLookup,
        onStarterTaskSelected = ::selectStarterTask,
        onRunQuery = ::runQuery,
        onCancelTurn = ::cancelCurrentTurn,
        onRefreshAuth = ::refreshAuthFromUi,
        onSaveSettings = ::applySettings,
        onResetAppOverrides = ::resetAppOverrides,
        onClearAuthSecrets = ::clearAuthSecrets,
        onAuthSignIn = ::startOAuthSignIn,
        onAuthOobSignIn = ::startOobSignIn,
        onDeveloperSessionSignIn = ::startDeveloperSessionSignIn,
        onAuthRetry = ::retryAuthConnection,
        onDismissAuthWeb = ::dismissAuthWeb,
        onOAuthCallback = ::handleOAuthCallback
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
        authInteractiveFailure = authInteractiveFailure,
        error = error,
        authSessionId = authSessionId,
        authWebUrl = authWebUrl,
        recentConversations = recentConversations,
        scheduleHistoryFilter = scheduleHistoryFilter,
        activeConversationId = activeConversationId,
        openingConversationId = openingConversationId,
        conversationState = conversationState,
        activeGoal = activeGoal,
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
        query = query,
        composerAttachments = composerAttachments,
        lookupOccurrences = composerLookupOccurrences,
        lookupSelections = composerLookupSelections,
        mediaController = mediaController,
        callbacks = callbacks
    )
    activeComposerLookupOccurrence?.let { occurrence ->
        ComposerLookupDialog(
            client = client,
            occurrence = occurrence,
            currentSelection = composerLookupSelections[occurrence.key],
            activeConversationId = activeConversationId,
            onDismiss = { activeComposerLookupOccurrence = null },
            onSelect = { row ->
                val selection = composerLookupSelection(occurrence, row)
                composerLookupSelections = composerLookupSelections + (occurrence.key to selection)
                activeComposerLookupOccurrence = null
            },
            onClear = if (composerLookupSelections.containsKey(occurrence.key)) {
                {
                    composerLookupSelections = composerLookupSelections - occurrence.key
                    activeComposerLookupOccurrence = null
                }
            } else {
                null
            }
        )
    }
}

internal fun sanitizeDownloadFileName(value: String): String {
    val name = value.substringAfterLast('/').substringAfterLast('\\').trim()
    return name.replace(Regex("[^A-Za-z0-9._ -]"), "_")
        .trim()
        .take(96)
}

private fun SavedLoginConfig.withBootstrapDefaults(
    oobSecretRef: String
): SavedLoginConfig {
    return copy(
        oobSecretRef = this.oobSecretRef.ifBlank { oobSecretRef.trim() }
    )
}

internal fun shouldAttemptBootstrapOobSignIn(
    debugBuild: Boolean,
    bootstrapAutoOobSignIn: Boolean,
    authState: AuthState,
    authBusy: Boolean,
    alreadyAttempted: Boolean,
    savedLoginConfig: SavedLoginConfig
): Boolean =
    debugBuild &&
        bootstrapAutoOobSignIn &&
        authState == AuthState.Required &&
        !authBusy &&
        !alreadyAttempted &&
        savedLoginConfig.hasStoredOobSecretRef

internal fun shouldShowDeveloperSessionEntry(
    debugBuild: Boolean,
    interactiveAuthFailure: Boolean
): Boolean =
    debugBuild && interactiveAuthFailure

internal enum class AppScreen {
    Chat,
    History,
    Automation,
    Settings
}

internal fun buildClientQueryContext(
    formFactor: String,
    uiClientId: String? = null
): Map<String, JsonElement> {
    val clientKind = when (formFactor) {
        "tablet" -> "tablet"
        else -> "mobile"
    }
    val context = linkedMapOf<String, JsonElement>(
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
    uiClientId?.trim()?.takeIf { it.isNotBlank() }?.let {
        context["uiClientId"] = JsonPrimitive(it)
    }
    return context
}

internal fun buildAndroidTargetCapabilities(): List<String> {
    return listOf("markdown", "chart", "attachments", "camera", "voice")
}

internal fun buildForgeTargetContext(formFactor: String): ForgeTargetContext {
    return ForgeTargetContext(
        platform = "android",
        formFactor = formFactor,
        surface = "app",
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
