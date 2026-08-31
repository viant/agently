package com.viant.agently.android

import androidx.compose.ui.text.AnnotatedString
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
    fun `lookup authoring directive is hidden from the editable prompt`() {
        val prompt = "Troubleshoot /order order for delivery issues."
        val start = prompt.indexOf("/order")
        val transformed = ComposerLookupVisualTransformation(
            listOf(start until start + "/order".length)
        ).filter(AnnotatedString(prompt))

        assertEquals("Troubleshoot order for delivery issues.", transformed.text.text)
        assertEquals(
            transformed.text.length,
            transformed.offsetMapping.originalToTransformed(prompt.length)
        )
        assertEquals(
            prompt.length,
            transformed.offsetMapping.transformedToOriginal(transformed.text.length)
        )
    }

    @Test
    fun `lookup directive before punctuation does not leave a stray space`() {
        val prompt = "Recommend SPO for audience /line ."
        val start = prompt.indexOf("/line")
        val transformed = ComposerLookupVisualTransformation(
            listOf(start until start + "/line".length)
        ).filter(AnnotatedString(prompt))

        assertEquals("Recommend SPO for audience.", transformed.text.text)
    }

    @Test
    fun `phone inline lookup replaces duplicated following entity noun`() {
        val prompt = "Troubleshoot /order order for delivery issues."

        assertEquals(
            "Troubleshoot Order for delivery issues.",
            composerInlineLookupDisplayText(
                source = prompt,
                occurrences = listOf(orderLookupOccurrence(prompt))
            )
        )
    }

    @Test
    fun `phone inline lookup preserves entity noun before lookup token`() {
        val prompt = "show me line /line"
        val registry = listOf(
            LookupRegistryEntry(
                name = "line",
                title = "Line",
                dataSource = "line_lookup",
                required = true,
                token = LookupTokenFormat(
                    store = "\${id}",
                    display = "\${name}",
                    modelForm = "line \${id}",
                    queryInput = "q"
                )
            )
        )

        assertEquals(
            "show me line Line",
            composerInlineLookupDisplayText(
                source = prompt,
                occurrences = parseComposerLookupOccurrences(prompt, registry)
            )
        )
    }

    @Test
    fun `phone inline lookup keeps selected order label compact`() {
        val prompt = "Troubleshoot /order order for delivery issues and identify the blocker."
        val occurrence = orderLookupOccurrence(prompt)
        val selection = ComposerLookupSelection(
            token = """@{order:2691875 "2691875 · Pillar 1 - Brand | BHE - 2026-27"}""",
            label = "2691875 · Pillar 1 - Brand | BHE - 2026-27"
        )

        assertEquals("2691875", composerInlineLookupLabel(occurrence, selection))
        assertEquals(
            "Troubleshoot 2691875 for delivery issues and identify the blocker.",
            composerInlineLookupDisplayText(
                source = prompt,
                occurrences = listOf(occurrence),
                selections = mapOf(occurrence.key to selection)
            )
        )
    }

    @Test
    fun `lookup controls make unresolved required selection explicit`() {
        val occurrence = orderLookupOccurrence()

        assertSame(
            occurrence,
            firstUnresolvedRequiredComposerLookup(listOf(occurrence), emptyMap())
        )
        assertEquals("Send", composerSendButtonLabel(occurrence))
        assertEquals(false, composerSendEnabled(false, true, occurrence))
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
        assertEquals(true, composerSendEnabled(false, true, null))
        assertEquals("Fixture Order", composerLookupControlLabel(occurrence.title, selection))
    }

    private fun orderLookupOccurrence(prompt: String = "Troubleshoot /order"): ComposerLookupOccurrence {
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
        return parseComposerLookupOccurrences(prompt, registry).single()
    }
}
