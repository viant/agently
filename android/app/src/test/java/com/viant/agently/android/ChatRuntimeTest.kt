package com.viant.agently.android

import com.viant.agentlysdk.stream.BufferedMessage
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
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
    fun visibleAppError_hidesLifecycleCancellationNoise() {
        assertNull(visibleAppError(IllegalStateException("left the composition")))
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
    fun parseConversationActivityInstantMillis_handlesGoMonotonicSuffix() {
        val parsed = parseConversationActivityInstantMillis(
            "2026-06-02 11:44:30.288943 -0700 PDT m=+9154.487875251"
        )

        assertNotNull(parsed)
    }
}
