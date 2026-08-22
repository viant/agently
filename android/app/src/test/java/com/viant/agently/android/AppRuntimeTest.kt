package com.viant.agently.android

import com.viant.agentlysdk.ActiveFeedState
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.AuthProvider
import com.viant.agentlysdk.AuthUser
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.EndpointConfig
import com.viant.agentlysdk.MetadataTargetContext
import com.viant.agentlysdk.PendingToolApproval
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class AppRuntimeTest {
    private val metadataTargetContext = MetadataTargetContext(
        platform = "android",
        formFactor = "phone",
        surface = "app",
        capabilities = listOf("markdown", "chart")
    )

    @Test
    fun `phone conversation policy omits model calls and payload previews`() {
        val policy = conversationLoadPolicy("phone")

        assertFalse(policy.includeModelCalls)
        assertTrue(policy.includeToolCalls)
        assertTrue(policy.includeFeeds)
        assertFalse(policy.includePayloadPreviews)
        assertEquals(8L * 1024 * 1024, policy.maxTranscriptResponseBytes)
        assertEquals(0, policy.maxPayloadPreviewCount)
        assertEquals(0, policy.maxPayloadDownloadBytes)
        assertEquals(0, policy.maxPayloadInflatedBytes)
    }

    @Test
    fun `tablet conversation policy retains diagnostics but lazily loads payload bodies`() {
        val policy = conversationLoadPolicy("tablet")

        assertTrue(policy.includeModelCalls)
        assertTrue(policy.includeToolCalls)
        assertTrue(policy.includeFeeds)
        assertFalse(policy.includePayloadPreviews)
        assertEquals(16L * 1024 * 1024, policy.maxTranscriptResponseBytes)
        assertEquals(0, policy.maxPayloadPreviewCount)
        assertEquals(0, policy.maxPayloadDownloadBytes)
        assertEquals(0, policy.maxPayloadInflatedBytes)
    }

    @Test
    fun `resolveAuthState requires sign in when oauth provider exists without user`() {
        val state = resolveAuthState(
            providers = listOf(AuthProvider(name = "oauth", type = "oauth", label = "OIDC")),
            user = null
        )

        assertEquals(AuthState.Required, state)
    }

    @Test
    fun `resolveAuthState stays ready for local-only provider`() {
        val state = resolveAuthState(
            providers = listOf(AuthProvider(name = "local", type = "local", label = "Local")),
            user = null
        )

        assertEquals(AuthState.Ready, state)
    }

    @Test
    fun `trimApprovalEdits keeps only pending approval ids`() {
        val trimmed = trimApprovalEdits(
            approvalEdits = mapOf(
                "approval-1" to mapOf("target" to JsonPrimitive("prod")),
                "approval-2" to mapOf("target" to JsonPrimitive("stage"))
            ),
            approvals = listOf(
                PendingToolApproval(id = "approval-2", toolName = "deploy", status = "pending")
            )
        )

        assertEquals(setOf("approval-2"), trimmed.keys)
    }

    @Test
    fun `android forge and metadata target contexts share the same capability set`() {
        val metadata = buildMetadataTargetContext("tablet")
        val forge = buildForgeTargetContext("tablet")

        assertEquals(buildAndroidTargetCapabilities(), metadata.capabilities)
        assertEquals(buildAndroidTargetCapabilities().toSet(), forge.capabilities)
    }

    @Test
    fun `conversation state response retains feeds for history hydration`() {
        val feed = ActiveFeedState(feedId = "plan", title = "Plan", itemCount = 3)
        val response = ConversationStateResponse(feeds = listOf(feed))

        assertEquals(listOf(feed), response.feeds)
    }

    @Test
    fun `resolveAuthCapableClient throws when no auth endpoint is reachable`() {
        val currentClient = AgentlyClient(mapOf())

        val error = kotlin.runCatching {
            kotlinx.coroutines.runBlocking {
                resolveAuthCapableClient(
                    currentBaseUrl = "http://10.0.2.2:9292",
                    candidates = listOf("http://10.0.2.2:9292", "http://127.0.0.1:9292"),
                    currentClient = currentClient,
                    buildClient = { currentClient }
                )
            }
        }.exceptionOrNull()

        assertNotNull(error)
    }

    @Test
    fun `auth session resolution reuses the provider probe response`() {
        val server = MockWebServer()
        server.enqueue(MockResponse().setBody("""[{"name":"bff","type":"bff"}]"""))
        server.start()
        try {
            val baseUrl = server.url("/").toString().trimEnd('/')
            val client = AgentlyClient(mapOf("appAPI" to EndpointConfig(baseUrl = baseUrl)))

            val resolved = runBlocking {
                resolveAuthSessionClient(
                    currentBaseUrl = baseUrl,
                    candidates = listOf(baseUrl),
                    currentClient = client,
                    buildClient = { client }
                )
            }

            assertEquals("bff", resolved.providers.single().name)
            assertEquals(1, server.requestCount)
            assertEquals("/v1/api/auth/providers", server.takeRequest().path)
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `auth refresh classifies provider connectivity failure as unavailable`() {
        val client = AgentlyClient(mapOf())

        val result = runBlocking {
            refreshAuthSession(
                currentBaseUrl = "https://steward.agently.viantinc.com",
                candidates = listOf("https://steward.agently.viantinc.com"),
                currentClient = client,
                buildClient = { client },
                loadOnSuccess = false,
                targetContext = metadataTargetContext
            )
        }

        assertEquals(AuthState.Unavailable, result.authState)
        assertNotNull(result.error)
    }

    @Test
    fun `loadWorkspaceSession returns visible error for non auth failures`() {
        val result = kotlinx.coroutines.runBlocking {
            loadWorkspaceSession({
                throw IllegalStateException("connection refused")
            }, metadataTargetContext)
        }

        assertNull(result.snapshot)
        assertNull(result.authRequiredError)
        assertNotNull(result.visibleError)
    }

    @Test
    fun `loadWorkspaceSession marks auth required for 401 failures`() {
        val result = kotlinx.coroutines.runBlocking {
            loadWorkspaceSession({
                throw IllegalStateException("401 unauthorized")
            }, metadataTargetContext)
        }

        assertNull(result.snapshot)
        assertNotNull(result.authRequiredError)
        assertNull(result.visibleError)
    }

    @Test
    fun `auth refresh preserves existing session on transient connectivity failure`() {
        assertTrue(
            shouldPreserveAuthenticatedSessionOnAuthRefreshFailure(
                previousAuthState = AuthState.Ready,
                previousUser = AuthUser(username = "test-user"),
                previousSessionId = "session-1",
                err = java.net.SocketTimeoutException("failed to connect")
            )
        )
    }

    @Test
    fun `auth refresh preserves existing session while checking stale refresh result`() {
        assertTrue(
            shouldPreserveAuthenticatedSessionOnAuthRefreshFailure(
                previousAuthState = AuthState.Checking,
                previousUser = AuthUser(username = "test-user"),
                previousSessionId = "session-1",
                err = java.net.SocketTimeoutException("failed to connect")
            )
        )
    }

    @Test
    fun `auth refresh does not preserve session on credential rejection`() {
        assertFalse(
            shouldPreserveAuthenticatedSessionOnAuthRefreshFailure(
                previousAuthState = AuthState.Ready,
                previousUser = AuthUser(username = "test-user"),
                previousSessionId = "session-1",
                err = IllegalStateException("GET /v1/api/auth/me failed: 401")
            )
        )
    }

    @Test
    fun `stale auth refresh failure is ignored after session changes`() {
        assertTrue(
            shouldIgnoreStaleAuthRefreshFailure(
                startedSessionId = null,
                currentSessionId = "session-1",
                currentUser = AuthUser(username = "test-user"),
                err = java.net.SocketTimeoutException("failed to connect")
            )
        )
    }

    @Test
    fun `stale auth refresh failure is not ignored without current session`() {
        assertFalse(
            shouldIgnoreStaleAuthRefreshFailure(
                startedSessionId = null,
                currentSessionId = null,
                currentUser = null,
                err = java.net.SocketTimeoutException("failed to connect")
            )
        )
    }

    @Test
    fun `workspace bootstrap starts only after auth is ready and metadata is missing`() {
        assertTrue(
            shouldBootstrapWorkspace(
                authState = AuthState.Ready,
                metadataLoaded = false,
                loading = false,
                workspaceBootstrapRequested = false
            )
        )
        assertFalse(
            shouldBootstrapWorkspace(
                authState = AuthState.Required,
                metadataLoaded = false,
                loading = false,
                workspaceBootstrapRequested = false
            )
        )
        assertFalse(
            shouldBootstrapWorkspace(
                authState = AuthState.Ready,
                metadataLoaded = false,
                loading = true,
                workspaceBootstrapRequested = false
            )
        )
        assertFalse(
            shouldBootstrapWorkspace(
                authState = AuthState.Ready,
                metadataLoaded = false,
                loading = false,
                workspaceBootstrapRequested = true
            )
        )
    }

    @Test
    fun `workspace bootstrap does not repeat when metadata loaded with empty recents`() {
        assertFalse(
            shouldBootstrapWorkspace(
                authState = AuthState.Ready,
                metadataLoaded = true,
                loading = false,
                workspaceBootstrapRequested = false
            )
        )
    }

    @Test
    fun `mergeConversationIntoRecentList injects missing active conversation`() {
        val existing = Conversation(
            id = "existing",
            title = "Open workspace items",
            createdAt = "2026-06-02 13:31:23.337963 -0700 PDT m=+15567.496967543",
            lastActivity = "2026-06-02 13:32:09.609359 -0700 PDT m=+15613.768971084"
        )
        val active = Conversation(
            id = "active",
            title = "Policy review request",
            createdAt = "2026-06-02 11:42:01.751982 -0700 PDT m=+9005.950440334",
            lastActivity = "2026-06-02 11:44:30.288943 -0700 PDT m=+9154.487875251"
        )

        val merged = mergeConversationIntoRecentList(listOf(existing), active)

        assertEquals(2, merged.size)
        assertTrue(merged.any { it.id == "active" })
    }

    @Test
    fun `buildConversationResetState clears active conversation state`() {
        val resetState = buildConversationResetState()

        assertNull(resetState.activeConversationId)
        assertNull(resetState.streamSnapshot)
        assertNull(resetState.streamedMarkdown)
        assertNull(resetState.result)
        assertNull(resetState.error)
        assertEquals(emptyList<PendingToolApproval>(), resetState.pendingApprovals)
        assertEquals(emptyMap<String, Map<String, JsonPrimitive>>(), resetState.approvalEdits)
        assertEquals(emptyList<com.viant.agentlysdk.GeneratedFileEntry>(), resetState.generatedFiles)
        assertNull(resetState.artifactPreview)
    }

    @Test
    fun `buildAuthRequiredSessionReset clears workspace auth and conversation state`() {
        val resetState = buildAuthRequiredSessionReset()

        assertNull(resetState.metadata)
        assertEquals(emptyList<com.viant.agentlysdk.Conversation>(), resetState.recentConversations)
        assertEquals(emptyList<AuthProvider>(), resetState.authProviders)
        assertNull(resetState.authUser)
        assertNull(resetState.authWebUrl)
        assertEquals(false, resetState.authBusy)
        assertNull(resetState.conversationReset.activeConversationId)
        assertEquals(emptyList<PendingToolApproval>(), resetState.conversationReset.pendingApprovals)
    }

    @Test
    fun `buildWorkspaceSessionReset clears workspace bootstrap auth and conversation state`() {
        val resetState = buildWorkspaceSessionReset()

        assertNull(resetState.metadata)
        assertEquals(emptyList<com.viant.agentlysdk.Conversation>(), resetState.recentConversations)
        assertEquals(emptyList<AuthProvider>(), resetState.authProviders)
        assertNull(resetState.authUser)
        assertNull(resetState.authWebUrl)
        assertEquals(false, resetState.authBusy)
        assertNull(resetState.authError)
        assertEquals(AuthState.Checking, resetState.authState)
        assertEquals(false, resetState.workspaceBootstrapRequested)
        assertNull(resetState.conversationReset.activeConversationId)
        assertEquals(emptyList<PendingToolApproval>(), resetState.conversationReset.pendingApprovals)
    }
}
