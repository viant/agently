package com.viant.agently.android

import com.viant.agentlysdk.stream.BufferedMessage
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertFalse
import org.junit.Test

class ChatRuntimeTest {

    @Test
    fun latestAssistantMarkdown_prefersNewestAssistantMessage() {
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-2",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = listOf(
                BufferedMessage(id = "m1", turnId = "turn-1", role = "assistant", content = "Earlier"),
                BufferedMessage(id = "m2", turnId = "turn-2", role = "assistant", narration = "Heads up", content = "Latest")
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
    fun syncAssistantTranscript_replacesSyntheticFinalEntryWithRealMessage() {
        val transcript = mutableListOf(
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
            activeTurnId = null,
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

        syncAssistantTranscript(transcript, snapshot)

        assertEquals(1, transcript.size)
        assertEquals("assistant-real-1", transcript.single().id)
        assertEquals("maple", transcript.single().markdown)
        assertFalse(transcript.single().streaming)
    }
}
