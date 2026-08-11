package com.viant.agently.android

import com.viant.agentlysdk.LookupRegistryEntry
import com.viant.agentlysdk.LookupTokenFormat
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Test

class ComposerScreenTest {
    @Test
    fun `compact composer expands for long selected prompts`() {
        assertEquals(2, composerInputMaxLines(compactConversationDock = true, query = ""))
        assertEquals(2, composerInputMaxLines(compactConversationDock = true, query = "short reply"))
        assertEquals(
            4,
            composerInputMaxLines(
                compactConversationDock = true,
                query = """
                    line one
                    line two
                    line three
                    line four
                """.trimIndent()
            )
        )
        assertEquals(
            6,
            composerInputMaxLines(
                compactConversationDock = true,
                query = "Troubleshoot ad order delivery issues and identify the primary causal blocker family " +
                    "such as setup, supply, bid competitiveness, or change pressure."
            )
        )
    }

    @Test
    fun `new conversation composer keeps full prompt room`() {
        assertEquals(6, composerInputMaxLines(compactConversationDock = false, query = "short reply"))
    }

    @Test
    fun `lookup controls make unresolved required selection explicit`() {
        val occurrence = orderLookupOccurrence()

        assertSame(
            occurrence,
            firstUnresolvedRequiredComposerLookup(listOf(occurrence), emptyMap())
        )
        assertEquals("Send", composerSendButtonLabel(occurrence))
        assertEquals("Select Order", composerLookupControlLabel(occurrence.title, null))
    }

    @Test
    fun `lookup controls use selected label after selection`() {
        val occurrence = orderLookupOccurrence()
        val selection = ComposerLookupSelection(
            token = """@{order:fixture-order-1 "Fixture Order"}""",
            label = "Fixture Order"
        )

        assertNull(
            firstUnresolvedRequiredComposerLookup(
                listOf(occurrence),
                mapOf(occurrence.key to selection)
            )
        )
        assertEquals("Send", composerSendButtonLabel(null))
        assertEquals("Fixture Order", composerLookupControlLabel(occurrence.title, selection))
    }

    private fun orderLookupOccurrence(): ComposerLookupOccurrence {
        val registry = listOf(
            LookupRegistryEntry(
                name = "order",
                title = "Order",
                dataSource = "order_lookup",
                required = true,
                token = LookupTokenFormat(
                    store = "\${id}",
                    display = "\${name}",
                    modelForm = "order \${id}",
                    queryInput = "q"
                )
            )
        )
        return parseComposerLookupOccurrences("Troubleshoot /order", registry).single()
    }
}
