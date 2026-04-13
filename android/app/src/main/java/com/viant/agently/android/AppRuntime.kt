package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.AuthProvider
import com.viant.agentlysdk.AuthUser
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.ListConversationsInput
import com.viant.agentlysdk.ListPendingToolApprovalsInput
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.WorkspaceMetadata
import kotlinx.serialization.json.JsonElement

internal data class ResolvedClient(
    val baseUrl: String,
    val client: AgentlyClient
)

internal data class WorkspaceSnapshot(
    val metadata: com.viant.agentlysdk.WorkspaceMetadata,
    val conversations: List<Conversation>
)

internal data class ConversationBindingData(
    val state: ConversationStateResponse,
    val approvals: List<PendingToolApproval>,
    val generatedFiles: List<GeneratedFileEntry>
)

internal data class WorkspaceAgentChoice(
    val id: String,
    val label: String
)

internal data class AuthRefreshResult(
    val resolvedBaseUrl: String,
    val authState: AuthState,
    val providers: List<AuthProvider>,
    val user: AuthUser?,
    val workspaceSnapshot: WorkspaceSnapshot? = null,
    val authRequiredError: Throwable? = null
)

internal data class WorkspaceLoadResult(
    val snapshot: WorkspaceSnapshot? = null,
    val authRequiredError: Throwable? = null,
    val visibleError: String? = null
)

internal data class PreparedConversationBinding(
    val conversationId: String,
    val pendingApprovals: List<PendingToolApproval>,
    val approvalEdits: Map<String, Map<String, JsonElement>>,
    val generatedFiles: List<GeneratedFileEntry>,
    val transcriptEntries: List<ChatEntry>,
    val replaceTranscript: Boolean
)

internal data class ConversationResetState(
    val activeConversationId: String? = null,
    val streamSnapshot: com.viant.agentlysdk.stream.ConversationStreamSnapshot? = null,
    val streamedMarkdown: String? = null,
    val result: com.viant.agentlysdk.QueryOutput? = null,
    val error: String? = null,
    val pendingApprovals: List<PendingToolApproval> = emptyList(),
    val approvalEdits: Map<String, Map<String, JsonElement>> = emptyMap(),
    val generatedFiles: List<GeneratedFileEntry> = emptyList(),
    val artifactPreview: ArtifactPreview? = null
)

internal data class AuthRequiredSessionReset(
    val metadata: WorkspaceMetadata? = null,
    val recentConversations: List<Conversation> = emptyList(),
    val authProviders: List<AuthProvider> = emptyList(),
    val authUser: AuthUser? = null,
    val authWebUrl: String? = null,
    val authBusy: Boolean = false,
    val conversationReset: ConversationResetState = buildConversationResetState()
)

internal data class WorkspaceSessionReset(
    val metadata: WorkspaceMetadata? = null,
    val recentConversations: List<Conversation> = emptyList(),
    val authProviders: List<AuthProvider> = emptyList(),
    val authUser: AuthUser? = null,
    val authError: String? = null,
    val authState: AuthState = AuthState.Checking,
    val workspaceBootstrapRequested: Boolean = false,
    val conversationReset: ConversationResetState = buildConversationResetState()
)

internal suspend fun resolveReachableClient(
    currentBaseUrl: String,
    candidates: List<String>,
    currentClient: AgentlyClient,
    buildClient: (String) -> AgentlyClient,
    probe: suspend (AgentlyClient) -> Unit
): ResolvedClient {
    var lastError: Throwable? = null
    for (candidate in candidates.distinct()) {
        val probeClient = if (candidate == currentBaseUrl) currentClient else buildClient(candidate)
        try {
            probe(probeClient)
            return ResolvedClient(baseUrl = candidate, client = probeClient)
        } catch (err: Throwable) {
            lastError = err
        }
    }
    throw lastError ?: IllegalStateException("Unable to reach app API")
}

internal suspend fun resolveWorkspaceClient(
    currentBaseUrl: String,
    candidates: List<String>,
    currentClient: AgentlyClient,
    buildClient: (String) -> AgentlyClient,
    onResolvedBaseUrl: (String) -> Unit
): AgentlyClient {
    val resolved = resolveReachableClient(
        currentBaseUrl = currentBaseUrl,
        candidates = candidates,
        currentClient = currentClient,
        buildClient = buildClient
    ) { probeClient ->
        probeClient.getWorkspaceMetadata()
    }
    if (resolved.baseUrl != currentBaseUrl) {
        onResolvedBaseUrl(resolved.baseUrl)
    }
    return resolved.client
}

internal suspend fun resolveAuthCapableClient(
    currentBaseUrl: String,
    candidates: List<String>,
    currentClient: AgentlyClient,
    buildClient: (String) -> AgentlyClient
): ResolvedClient {
    var lastError: Throwable? = null
    for (candidate in candidates.distinct()) {
        val probeClient = if (candidate == currentBaseUrl) currentClient else buildClient(candidate)
        try {
            probeClient.authProviders()
            return ResolvedClient(baseUrl = candidate, client = probeClient)
        } catch (err: Throwable) {
            lastError = err
        }
    }
    throw lastError ?: IllegalStateException("No auth-capable Agently endpoint is reachable")
}

internal suspend fun resolveAuthClientWithFallback(
    currentBaseUrl: String,
    candidates: List<String>,
    currentClient: AgentlyClient,
    buildClient: (String) -> AgentlyClient,
    onResolvedBaseUrl: (String) -> Unit
): AgentlyClient {
    val resolved = resolveAuthCapableClient(
        currentBaseUrl = currentBaseUrl,
        candidates = candidates,
        currentClient = currentClient,
        buildClient = buildClient
    )
    if (resolved.baseUrl != currentBaseUrl) {
        onResolvedBaseUrl(resolved.baseUrl)
    }
    return resolved.client
}

internal suspend fun loadWorkspaceSnapshot(
    client: AgentlyClient
): WorkspaceSnapshot {
    val metadata = client.getWorkspaceMetadata()
    val conversations = loadRecentConversations(client)
    return WorkspaceSnapshot(metadata = metadata, conversations = conversations)
}

internal suspend fun loadRecentConversations(
    client: AgentlyClient
): List<Conversation> {
    val conversations = client.listConversations(
        ListConversationsInput(
            page = com.viant.agentlysdk.PageInput(limit = 10)
        )
    ).rows
    return conversations
}

internal suspend fun loadConversationBindingData(
    client: AgentlyClient,
    conversationId: String
): ConversationBindingData {
    val state = client.getLiveState(conversationId, includeFeeds = true)
    val approvals = client.listPendingToolApprovals(
        ListPendingToolApprovalsInput(
            conversationId = conversationId,
            status = "pending",
            limit = 20
        )
    )
    val generatedFiles = client.listGeneratedFiles(conversationId)
    return ConversationBindingData(
        state = state,
        approvals = approvals,
        generatedFiles = generatedFiles
    )
}

internal suspend fun prepareConversationBinding(
    client: AgentlyClient,
    conversationId: String,
    replaceTranscript: Boolean,
    approvalEdits: Map<String, Map<String, JsonElement>>,
    transcriptBuilder: (ConversationStateResponse) -> List<ChatEntry>
): PreparedConversationBinding {
    val binding = loadConversationBindingData(client, conversationId)
    return PreparedConversationBinding(
        conversationId = conversationId,
        pendingApprovals = binding.approvals,
        approvalEdits = trimApprovalEdits(approvalEdits, binding.approvals),
        generatedFiles = binding.generatedFiles,
        transcriptEntries = if (replaceTranscript) transcriptBuilder(binding.state) else emptyList(),
        replaceTranscript = replaceTranscript
    )
}

internal fun trimApprovalEdits(
    approvalEdits: Map<String, Map<String, JsonElement>>,
    approvals: List<PendingToolApproval>
): Map<String, Map<String, JsonElement>> {
    return approvalEdits.filterKeys { approvalId ->
        approvals.any { it.id == approvalId }
    }
}

internal fun resolvePreferredAgentId(
    preferredAgentId: String?,
    metadata: WorkspaceMetadata?
): String? {
    val trimmed = preferredAgentId?.trim().orEmpty()
    if (trimmed.isNotBlank()) {
        return trimmed
    }
    return metadata?.defaultAgent?.takeIf { it.isNotBlank() }
        ?: metadata?.defaults?.agent?.takeIf { it.isNotBlank() }
}

internal fun workspaceAgentChoices(metadata: WorkspaceMetadata?): List<WorkspaceAgentChoice> {
    if (metadata == null) return emptyList()
    val infoChoices = metadata.agentInfos.mapNotNull { info ->
        val id = info.id?.trim().orEmpty()
        if (id.isBlank()) {
            null
        } else {
            WorkspaceAgentChoice(
                id = id,
                label = info.name?.takeIf { it.isNotBlank() } ?: id
            )
        }
    }
    val fallbackChoices = metadata.agents
        .map { it.trim() }
        .filter { it.isNotBlank() }
        .map { WorkspaceAgentChoice(id = it, label = it) }
    return (infoChoices + fallbackChoices).distinctBy { it.id }
}

internal fun resolveAuthState(
    providers: List<AuthProvider>,
    user: AuthUser?
): AuthState {
    val hasOAuth = providers.any { provider ->
        val type = provider.type.trim().lowercase()
        type == "oauth" || type == "bff" || type == "oidc" || type == "jwt"
    }
    val localOnly = providers.isNotEmpty() && providers.all { it.type.trim().lowercase() == "local" }
    return when {
        user != null -> AuthState.Ready
        providers.isEmpty() -> AuthState.Ready
        localOnly -> AuthState.Ready
        hasOAuth -> AuthState.Required
        else -> AuthState.Ready
    }
}

internal suspend fun refreshAuthSession(
    currentBaseUrl: String,
    candidates: List<String>,
    currentClient: AgentlyClient,
    buildClient: (String) -> AgentlyClient,
    loadOnSuccess: Boolean
): AuthRefreshResult {
    val authResolved = try {
        resolveAuthCapableClient(
            currentBaseUrl = currentBaseUrl,
            candidates = candidates,
            currentClient = currentClient,
            buildClient = buildClient
        )
    } catch (err: Throwable) {
        return AuthRefreshResult(
            resolvedBaseUrl = currentBaseUrl,
            authState = AuthState.Required,
            providers = emptyList(),
            user = null,
            authRequiredError = err
        )
    }
    val authClient = authResolved.client
    val resolvedBaseUrl = authResolved.baseUrl

    val providers = try {
        authClient.authProviders()
    } catch (err: Throwable) {
        return AuthRefreshResult(
            resolvedBaseUrl = resolvedBaseUrl,
            authState = AuthState.Required,
            providers = emptyList(),
            user = null,
            authRequiredError = err
        )
    }

    val user = try {
        authClient.authMe()
    } catch (err: Throwable) {
        return AuthRefreshResult(
            resolvedBaseUrl = resolvedBaseUrl,
            authState = AuthState.Required,
            providers = providers,
            user = null,
            authRequiredError = err
        )
    }
    val nextAuthState = resolveAuthState(providers, user)
    val workspaceSnapshot = if (nextAuthState == AuthState.Ready && loadOnSuccess) {
        val workspaceClient = resolveWorkspaceClient(
            currentBaseUrl = resolvedBaseUrl,
            candidates = candidates,
            currentClient = if (resolvedBaseUrl == currentBaseUrl) currentClient else buildClient(resolvedBaseUrl),
            buildClient = buildClient,
            onResolvedBaseUrl = {}
        )
        loadWorkspaceSnapshot(workspaceClient)
    } else {
        null
    }

    return AuthRefreshResult(
        resolvedBaseUrl = resolvedBaseUrl,
        authState = nextAuthState,
        providers = providers,
        user = user,
        workspaceSnapshot = workspaceSnapshot
    )
}

internal suspend fun loadWorkspaceSession(
    resolveClient: suspend () -> AgentlyClient
): WorkspaceLoadResult {
    return try {
        val snapshot = loadWorkspaceSnapshot(resolveClient())
        WorkspaceLoadResult(snapshot = snapshot)
    } catch (err: Throwable) {
        val message = err.message ?: err.toString()
        if (message.contains("401") || message.contains("403")) {
            WorkspaceLoadResult(authRequiredError = err)
        } else {
            WorkspaceLoadResult(visibleError = visibleAppError(err))
        }
    }
}

internal fun buildConversationResetState(): ConversationResetState {
    return ConversationResetState()
}

internal fun buildAuthRequiredSessionReset(): AuthRequiredSessionReset {
    return AuthRequiredSessionReset()
}

internal fun buildWorkspaceSessionReset(): WorkspaceSessionReset {
    return WorkspaceSessionReset()
}
