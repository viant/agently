package com.viant.agently.android

import com.viant.agentlysdk.Conversation
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class ConversationHistoryRuntimeTest {
    private val conversations = listOf(
        Conversation(
            id = "private-id-1",
            title = "Order%20Performance%20Report",
            summary = "Daily spend and pacing",
            agentId = "steward"
        ),
        Conversation(
            id = "private-id-2",
            title = "Inventory review",
            summary = "Supply forecast",
            agentId = "planner"
        )
    )

    @Test
    fun `phone history search matches decoded title summary and agent`() {
        assertEquals(listOf("private-id-1"), filterPhoneConversationHistory(conversations, "order pacing").map { it.id })
        assertEquals(listOf("private-id-2"), filterPhoneConversationHistory(conversations, "planner").map { it.id })
    }

    @Test
    fun `phone history labels never fall back to conversation uid`() {
        val untitled = Conversation(id = "private-id-only")

        assertEquals("Untitled conversation", conversationHistoryTitle(untitled))
        assertEquals("", conversationHistorySummary(untitled))
        assertFalse(conversationHistoryTitle(untitled).contains(untitled.id))
        assertFalse(conversationHistorySummary(untitled).contains(untitled.id))
    }

    @Test
    fun `history search accepts a conversation url without displaying its uid`() {
        assertEquals(
            "2e1c19a7-1f2b-42f3-aea9-c1bd1cdb3240",
            conversationIdFromHistorySearch(
                "https://steward.agently.viantinc.com/v1/conversation/2e1c19a7-1f2b-42f3-aea9-c1bd1cdb3240"
            )
        )
    }
}
