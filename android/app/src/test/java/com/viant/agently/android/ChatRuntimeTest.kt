package com.viant.agently.android

import com.viant.agentlysdk.RenderedContent
import com.viant.agentlysdk.RenderedContentPart
import com.viant.agentlysdk.RenderedReportAssembly
import com.viant.agentlysdk.AssistantMessageState
import com.viant.agentlysdk.AssistantState
import com.viant.agentlysdk.ConversationState
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.ModelUsageState
import com.viant.agentlysdk.TurnState
import com.viant.agentlysdk.UserMessageState
import com.viant.agentlysdk.stream.BufferedMessage
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.agentlysdk.stream.LiveExecutionGroup
import com.viant.agentlysdk.stream.LiveModelStepState
import com.viant.agentlysdk.stream.LiveToolStepState
import com.viant.agentlysdk.stream.StreamUsageState
import kotlinx.serialization.json.Json
import java.io.EOFException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class ChatRuntimeTest {

    @Test
    fun streamAcceptance_doesNotRequireTransientActiveTurn() {
        val workspaceAccepted = ConversationStreamSnapshot(
            conversationId = "conv-workspace",
            activeTurnId = null,
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(id = "workspace-accepted", role = "assistant", content = "")
            ),
            liveExecutionGroupsById = emptyMap()
        )
        val empty = ConversationStreamSnapshot(
            conversationId = "conv-empty",
            activeTurnId = null,
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = emptyList(),
            liveExecutionGroupsById = emptyMap()
        )

        assertEquals(true, streamSnapshotHasAcceptedActivity(workspaceAccepted))
        assertEquals(false, streamSnapshotHasAcceptedActivity(empty))
    }

    @Test
    fun progressStatusAnnotatedText_rendersHeadingAndBoldWithoutMarkdownMarkers() {
        val rendered = progressStatusAnnotatedText(
            """### Key findings

                - **Primary blocker:** bid competitiveness
            """.trimIndent()
        )

        assertEquals("Key findings\n• Primary blocker: bid competitiveness", rendered.text)
        val boldText = rendered.spanStyles
            .filter { it.item.fontWeight == androidx.compose.ui.text.font.FontWeight.Bold }
            .map { rendered.text.substring(it.start, it.end) }
        assertEquals(listOf("Key findings", "Primary blocker:"), boldText)
    }

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

        assertEquals("Latest", latestAssistantMarkdown(snapshot))
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
    fun activeAssistant_hidesProgressiveReportTransportAndUsesNarrationBubble() {
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
    fun turnProgress_usesNarrationReportingToolAndNonIntrusiveTokenUsage() {
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-1",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(
                    id = "assistant-1",
                    turnId = "turn-1",
                    role = "assistant",
                    narration = "Checking delivery evidence."
                )
            ),
            liveExecutionGroupsById = mapOf(
                "assistant-1" to LiveExecutionGroup(
                    pageId = "page-1",
                    assistantMessageId = "assistant-1",
                    turnId = "turn-1",
                    sequence = 1,
                    narration = "Validating performance metrics.",
                    status = "running",
                    toolSteps = listOf(
                        LiveToolStepState(
                            toolCallId = "tool-1",
                            toolName = "reporting:run_report",
                            status = "running"
                        )
                    )
                )
            ),
            usage = StreamUsageState(inputTokens = 120, outputTokens = 30, totalTokens = 150)
        )

        val presentation = turnProgressPresentation(true, null, snapshot)

        assertEquals("Working on your request", presentation?.title)
        assertEquals("Using a workspace tool.", presentation?.detail)
        assertEquals("Reporting", presentation?.activity)
        assertEquals("0/1 done · 1 active", presentation?.toolProgress)
        assertEquals(listOf("reporting:run_report"), presentation?.toolDetails?.map { it.name })
        assertEquals(listOf("running"), presentation?.toolDetails?.map { it.status })
        assertEquals("150 total tokens", presentation?.tokenUsage)
    }

    @Test
    fun turnProgress_distinguishesQueuedToolsAndWaitingForInput() {
        val state = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(TurnState(turnId = "turn-1", status = "waiting_for_user"))
            )
        )
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-1",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = emptyList(),
            liveExecutionGroupsById = mapOf(
                "assistant-1" to LiveExecutionGroup(
                    pageId = "page-1",
                    assistantMessageId = "assistant-1",
                    turnId = "turn-1",
                    status = "running",
                    toolSteps = listOf(LiveToolStepState(toolCallId = "queued-1", toolName = "search_orders", status = "queued"))
                )
            )
        )

        val presentation = turnProgressPresentation(true, state, snapshot)

        assertEquals("Needs your input", presentation?.title)
        assertEquals("Needs your input", presentation?.activity)
        assertEquals("0/1 done · 1 queued", presentation?.toolProgress)
        assertEquals("queued", presentation?.toolDetails?.firstOrNull()?.status)
        assertEquals(true, presentation?.isWaitingForUser)
        assertEquals(false, presentation?.canStop)
    }

    @Test
    fun turnProgress_prefersPerModelTurnTokenDetails() {
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-1",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = emptyList(),
            liveExecutionGroupsById = mapOf(
                "assistant-1" to LiveExecutionGroup(
                    pageId = "page-1",
                    assistantMessageId = "assistant-1",
                    turnId = "turn-1",
                    status = "running",
                    modelSteps = listOf(
                        LiveModelStepState(
                            modelCallId = "model-1",
                            provider = "openai",
                            model = "gpt-5",
                            usage = ModelUsageState(inputTokens = 100, outputTokens = 30, cachedInputTokens = 20, reasoningTokens = 5, totalTokens = 130)
                        ),
                        LiveModelStepState(
                            modelCallId = "model-2",
                            provider = "openai",
                            model = "gpt-5-mini",
                            usage = ModelUsageState(inputTokens = 20, outputTokens = 10, totalTokens = 30)
                        )
                    )
                )
            ),
            usage = StreamUsageState(inputTokens = 900, outputTokens = 100, totalTokens = 1_000)
        )

        val presentation = turnProgressPresentation(true, null, snapshot)

        assertEquals("160 turn tokens", presentation?.tokenUsage)
        assertEquals("turn", presentation?.tokenDetails?.scope)
        assertEquals(20, presentation?.tokenDetails?.cachedInput)
        assertEquals(5, presentation?.tokenDetails?.reasoning)
        assertEquals(listOf("openai/gpt-5", "openai/gpt-5-mini"), presentation?.tokenDetails?.models?.map { it.label })
        assertEquals(100, presentation?.tokenDetails?.models?.firstOrNull()?.input)
        assertEquals(30, presentation?.tokenDetails?.models?.firstOrNull()?.output)
        assertEquals(20, presentation?.tokenDetails?.models?.firstOrNull()?.cachedInput)
        assertEquals(5, presentation?.tokenDetails?.models?.firstOrNull()?.reasoning)
        assertNull(presentation?.tokenDetails?.models?.firstOrNull()?.embedding)
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
    fun transcriptFromState_doesNotDuplicateNarrationBesideFinalAnswer() {
        val state = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(
                    TurnState(
                        turnId = "turn-1",
                        status = "completed",
                        assistant = AssistantState(
                            narration = AssistantMessageState("n1", "Waiting for response"),
                            final = AssistantMessageState("a1", "The report is ready.")
                        )
                    )
                )
            )
        )

        assertEquals("The report is ready.", transcriptFromState(state).single().markdown)
    }

    @Test
    fun transcriptFromState_separatesNarrationFromFinalMarkdownHeading() {
        val state = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(
                    TurnState(
                        turnId = "turn-1",
                        status = "completed",
                        assistant = AssistantState(
                            narration = AssistantMessageState(
                                messageId = "n1",
                                content = "I’ll check delivery evidence.",
                                renderedContent = RenderedContent(
                                    schemaVersion = "1",
                                    parts = listOf(
                                        RenderedContentPart(kind = "markdown", text = "I’ll check delivery evidence.")
                                    )
                                )
                            ),
                            final = AssistantMessageState(
                                messageId = "a1",
                                content = "### Key findings\n- **Primary blocker:** bid competitiveness.",
                                renderedContent = RenderedContent(
                                    schemaVersion = "1",
                                    parts = listOf(
                                        RenderedContentPart(
                                            kind = "markdown",
                                            text = "### Key findings\n- **Primary blocker:** bid competitiveness."
                                        )
                                    )
                                )
                            )
                        )
                    )
                )
            )
        )

        val parts = transcriptFromState(state).single().renderedParts.orEmpty()

        assertEquals("I’ll check delivery evidence.", parts[0].text)
        assertEquals("\n\n### Key findings\n- **Primary blocker:** bid competitiveness.", parts[1].text)
    }

    @Test
    fun transcriptFromState_usesPendingNarrationAsAssistantBubble() {
        val state = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(
                    TurnState(
                        turnId = "turn-1",
                        status = "running",
                        assistant = AssistantState(
                            narration = AssistantMessageState("n1", "Waiting for response")
                        )
                    )
                )
            )
        )

        assertEquals("Waiting for response", transcriptFromState(state).single().markdown)
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
    fun transcriptWithActiveAssistant_composesProseReportAndLaterProseWithoutDroppingContent() {
        val report = RenderedContent(
            schemaVersion = "1",
            reports = listOf(
                RenderedReportAssembly(
                    scope = "order-1",
                    id = "delivery",
                    grammar = "report-document-v1",
                    status = "building",
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
                BufferedMessage(
                    id = "assistant-prose-1",
                    turnId = "turn-1",
                    role = "assistant",
                    content = "### Key findings\n\nPrimary evidence."
                ),
                BufferedMessage(id = "assistant-report", turnId = "turn-1", role = "assistant"),
                BufferedMessage(
                    id = "assistant-prose-2",
                    turnId = "turn-1",
                    role = "assistant",
                    content = "Follow-up interpretation."
                )
            ),
            liveExecutionGroupsById = mapOf(
                "assistant-report" to LiveExecutionGroup(
                    pageId = "page-1",
                    assistantMessageId = "assistant-report",
                    turnId = "turn-1",
                    renderedContent = report
                )
            )
        )

        val entry = transcriptWithActiveAssistant(emptyList(), snapshot).single()

        assertEquals("assistant-prose-2", entry.id)
        assertEquals("turn-1", entry.turnId)
        assertEquals(true, entry.streaming)
        assertEquals(
            listOf("### Key findings\n\nPrimary evidence.", "\n\nFollow-up interpretation."),
            entry.renderedParts.orEmpty().mapNotNull { it.text }
        )
        assertEquals("delivery", entry.renderedReports?.single()?.id)
    }

    @Test
    fun commitAssistantTurnFromSnapshot_keepsSseContentWhenTurnCompletes() {
        val transcript = mutableListOf(
            ChatEntry(id = "user-1", role = "user", markdown = "diagnose order", turnId = "turn-1"),
            ChatEntry(
                id = "assistant-pending",
                role = "assistant",
                markdown = "Working…",
                turnId = "turn-1",
                streaming = true
            )
        )
        val terminalSnapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = null,
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(
                    id = "assistant-final",
                    turnId = "turn-1",
                    role = "assistant",
                    content = "### Key findings\n\nThe report is ready.",
                    status = "completed",
                    interim = 0
                )
            ),
            liveExecutionGroupsById = emptyMap()
        )

        assertEquals(true, commitAssistantTurnFromSnapshot(transcript, terminalSnapshot, "turn-1"))
        assertEquals(listOf("user-1", "assistant-final"), transcript.map { it.id })
        assertEquals("### Key findings\n\nThe report is ready.", transcript.last().markdown)
        assertEquals(false, transcript.last().streaming)
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
