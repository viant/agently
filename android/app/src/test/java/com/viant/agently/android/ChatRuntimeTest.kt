package com.viant.agently.android

import com.viant.agentlysdk.RenderedContent
import com.viant.agentlysdk.RenderedReportAssembly
import com.viant.agentlysdk.AssistantMessageState
import com.viant.agentlysdk.AssistantState
import com.viant.agentlysdk.ConversationState
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.TurnState
import com.viant.agentlysdk.UserMessageState
import com.viant.agentlysdk.stream.BufferedMessage
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.agentlysdk.stream.LiveExecutionGroup
import kotlinx.serialization.json.Json
import java.io.EOFException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class ChatRuntimeTest {

    @Test
    fun latestAssistantMarkdown_prefersNewestActiveAssistantMessage() {
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-2",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(id = "m1", turnId = "turn-1", role = "assistant", content = "Earlier"),
                BufferedMessage(id = "m2", turnId = "turn-2", role = "assistant", narration = "Heads up", content = "Latest"),
                BufferedMessage(id = "m3", turnId = "turn-1", role = "assistant", content = "Historical but newer")
            ),
            liveExecutionGroupsById = emptyMap()
        )

        assertEquals("Heads up\n\nLatest", latestAssistantMarkdown(snapshot))
    }

    @Test
    fun latestActiveNarration_usesNewestNarrationFromActiveTurn() {
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-2",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(id = "m1", turnId = "turn-2", role = "assistant", narration = "Checking order details"),
                BufferedMessage(id = "m2", turnId = "turn-1", role = "assistant", narration = "Old status"),
                BufferedMessage(id = "m3", turnId = "turn-2", role = "assistant", narration = "Comparing delivery signals")
            ),
            liveExecutionGroupsById = emptyMap()
        )

        assertEquals("Comparing delivery signals", latestActiveNarration(snapshot))
    }

    @Test
    fun activeAssistant_hidesProgressiveReportTransportAndKeepsNarration() {
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-1",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(
                    id = "m1",
                    turnId = "turn-1",
                    role = "assistant",
                    narration = "Comparing forecast evidence",
                    content = """```forge-data
                        |{"version":2,"scope":"forecast","id":"rows","format":"json","data":[{"avails":317132}]}
                        |```""".trimMargin()
                )
            ),
            liveExecutionGroupsById = emptyMap()
        )

        assertEquals("Comparing forecast evidence", latestAssistantMarkdown(snapshot))
        assertEquals(true, activeAssistantHasVisibleOutput(snapshot))
    }

    @Test
    fun acceptedRunningTurn_isRecoveredAsPending() {
        val state = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(
                    TurnState(
                        turnId = "turn-1",
                        status = "running",
                        user = UserMessageState("user-1", "Forecast line 7354223"),
                        assistant = AssistantState(
                            narration = AssistantMessageState("assistant-1", "Checking reachable supply")
                        )
                    )
                )
            )
        )

        assertEquals(true, submittedTurnWasAccepted(state, "Forecast line 7354223"))
        assertEquals(true, hasPendingConversationTurn(state))
        assertEquals("Checking reachable supply", latestPendingNarration(state))
    }

    @Test
    fun visibleAppError_hidesLifecycleCancellationNoise() {
        assertNull(visibleAppError(IllegalStateException("left the composition")))
    }

    @Test
    fun visibleAppError_replacesRawEofExceptionWithRecoveryGuidance() {
        assertEquals(
            "The connection ended before the report finished loading. Refresh to try again.",
            visibleAppError(EOFException())
        )
    }

    @Test
    fun visibleAppError_hidesQueryEndpointAndMissingKeyResponse() {
        assertEquals(
            "The workspace model is not configured. Ask an administrator to add the model API key, then try again.",
            visibleAppError(
                IllegalStateException(
                    "POST http://127.0.0.1:8080/v1/agent/query failed: 500: {\"error\":\"failed to stream: API key is required\"}"
                )
            )
        )
    }

    @Test
    fun visibleAppError_hidesGenericQueryTransportDetails() {
        assertEquals(
            "The assistant could not start this request. Try again, or contact the workspace administrator if it continues.",
            visibleAppError(IllegalStateException("POST /v1/agent/query failed: 500: internal failure"))
        )
    }

    @Test
    fun transcriptWithActiveAssistant_appendsActiveEntryWithoutMutatingHistory() {
        val transcript = listOf(
            ChatEntry(
                id = "assistant-final-123",
                role = "assistant",
                markdown = "maple",
                streaming = false,
                timestampLabel = "8:20 PM"
            )
        )
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-1",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(
                    id = "assistant-real-1",
                    turnId = "turn-1",
                    role = "assistant",
                    content = "maple",
                    createdAt = "2026-04-10T20:21:00Z"
                )
            ),
            liveExecutionGroupsById = emptyMap()
        )

        val displayTranscript = transcriptWithActiveAssistant(transcript, snapshot)

        assertEquals(1, transcript.size)
        assertEquals("assistant-final-123", transcript.single().id)
        assertEquals(2, displayTranscript.size)
        assertEquals("assistant-real-1", displayTranscript.last().id)
        assertEquals("maple", displayTranscript.last().markdown)
        assertEquals(true, displayTranscript.last().streaming)
    }

    @Test
    fun transcriptWithActiveAssistant_replacesOptimisticStreamingAssistantForDisplay() {
        val transcript = listOf(
            ChatEntry(
                id = "user-1",
                role = "user",
                markdown = "open forecast builder",
                streaming = false,
                deliveryState = "sending"
            ),
            ChatEntry(
                id = "assistant-pending-1",
                role = "assistant",
                markdown = "Working...",
                streaming = false,
                deliveryState = "streaming"
            ),
            ChatEntry(
                id = "assistant-history-1",
                role = "assistant",
                markdown = "previous completed answer",
                streaming = false
            )
        )
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-1",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(
                    id = "assistant-real-1",
                    turnId = "turn-1",
                    role = "assistant",
                    content = "Opening the forecast builder.",
                    createdAt = "2026-06-11T20:21:00Z"
                )
            ),
            liveExecutionGroupsById = emptyMap()
        )

        val displayTranscript = transcriptWithActiveAssistant(transcript, snapshot)

        assertEquals(listOf("user-1", "assistant-history-1", "assistant-real-1"), displayTranscript.map { it.id })
        assertEquals("assistant-pending-1", transcript[1].id)
        assertEquals("Opening the forecast builder.", displayTranscript.last().markdown)
        assertEquals(true, displayTranscript.last().streaming)
    }

    @Test
    fun transcriptWithActiveAssistant_ignoresHydratedHistoryWhenThereIsNoActiveTurn() {
        val transcript = listOf(
            ChatEntry(
                id = "history-1",
                role = "assistant",
                markdown = "existing history",
                streaming = false
            )
        )
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = null,
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(
                    id = "assistant-hydrated-1",
                    turnId = "turn-1",
                    role = "assistant",
                    content = "hydrated history"
                )
            ),
            liveExecutionGroupsById = emptyMap()
        )

        val displayTranscript = transcriptWithActiveAssistant(transcript, snapshot)

        assertEquals(1, transcript.size)
        assertEquals("history-1", transcript.single().id)
        assertEquals("existing history", transcript.single().markdown)
        assertEquals(transcript, displayTranscript)
    }

    @Test
    fun transcriptWithActiveAssistant_preservesReportOnlyStreamingResponse() {
        val rendered = RenderedContent(
            schemaVersion = "1",
            reports = listOf(
                RenderedReportAssembly(
                    scope = "order-1",
                    id = "delivery",
                    grammar = "report-document-v1",
                    status = "committed",
                    source = Json.parseToJsonElement(
                        """{"title":"Delivery","blocks":[{"id":"note","kind":"markdownBlock","markdown":"Ready"}]}"""
                    )
                )
            )
        )
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-1",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(id = "assistant-1", turnId = "turn-1", role = "assistant")
            ),
            liveExecutionGroupsById = mapOf(
                "assistant-1" to LiveExecutionGroup(
                    pageId = "page-1",
                    assistantMessageId = "assistant-1",
                    turnId = "turn-1",
                    renderedContent = rendered
                )
            )
        )

        val displayTranscript = transcriptWithActiveAssistant(emptyList(), snapshot)

        assertEquals(1, displayTranscript.size)
        assertEquals("", displayTranscript.single().markdown)
        assertEquals("delivery", displayTranscript.single().renderedReports?.single()?.id)
    }

    @Test
    fun sanitizeVisibleAssistantText_stripsPureRouterPayload() {
        val raw = """{"classification":{"title":"Show entity 7391245","intent":"entity_lookup","confidence":0.72},"directAction":null,"prompting":{"appendToolBundles":["orchestrator"]}}"""

        assertNull(sanitizeVisibleAssistantText(raw))
    }

    @Test
    fun sanitizeVisibleAssistantText_preservesHumanPrefix() {
        val raw = """
            I need the entity type before opening 7391245. {
              "classification": {"intent":"entity_lookup"},
              "prompting": {"appendToolBundles":["orchestrator"]}
            }
        """.trimIndent()

        assertEquals("I need the entity type before opening 7391245.", sanitizeVisibleAssistantText(raw))
    }

    @Test
    fun transcriptWithActiveAssistant_skipsNewestRouterPayload() {
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-1",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(
                    id = "human",
                    turnId = "turn-1",
                    role = "assistant",
                    content = "I need the entity type before opening 7391245."
                ),
                BufferedMessage(
                    id = "router",
                    turnId = "turn-1",
                    role = "assistant",
                    content = """{"classification":{"intent":"entity_lookup"},"prompting":{"appendToolBundles":["orchestrator"]}}"""
                )
            ),
            liveExecutionGroupsById = emptyMap()
        )

        val display = transcriptWithActiveAssistant(emptyList(), snapshot)

        assertEquals(1, display.size)
        assertEquals("human", display.single().id)
        assertEquals("I need the entity type before opening 7391245.", display.single().markdown)
    }

    @Test
    fun parseConversationActivityInstantMillis_handlesGoMonotonicSuffix() {
        val parsed = parseConversationActivityInstantMillis(
            "2026-06-02 11:44:30.288943 -0700 PDT m=+9154.487875251"
        )

        assertNotNull(parsed)
    }
}
