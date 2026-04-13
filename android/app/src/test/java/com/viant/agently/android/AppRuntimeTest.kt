package com.viant.agently.android

import com.viant.agentlysdk.ActiveFeedState
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.AuthProvider
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.PendingToolApproval
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class AppRuntimeTest {

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
                    currentBaseUrl = "http://10.0.2.2:9393",
                    candidates = listOf("http://10.0.2.2:9393", "http://127.0.0.1:9393"),
                    currentClient = currentClient,
                    buildClient = { currentClient }
                )
            }
        }.exceptionOrNull()

        assertNotNull(error)
    }

    @Test
    fun `loadWorkspaceSession returns visible error for non auth failures`() {
        val result = kotlinx.coroutines.runBlocking {
            loadWorkspaceSession {
                throw IllegalStateException("connection refused")
            }
        }

        assertNull(result.snapshot)
        assertNull(result.authRequiredError)
        assertNotNull(result.visibleError)
    }

    @Test
    fun `loadWorkspaceSession marks auth required for 401 failures`() {
        val result = kotlinx.coroutines.runBlocking {
            loadWorkspaceSession {
                throw IllegalStateException("401 unauthorized")
            }
        }

        assertNull(result.snapshot)
        assertNotNull(result.authRequiredError)
        assertNull(result.visibleError)
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
        assertNull(resetState.authError)
        assertEquals(AuthState.Checking, resetState.authState)
        assertEquals(false, resetState.workspaceBootstrapRequested)
        assertNull(resetState.conversationReset.activeConversationId)
        assertEquals(emptyList<PendingToolApproval>(), resetState.conversationReset.pendingApprovals)
    }
}
