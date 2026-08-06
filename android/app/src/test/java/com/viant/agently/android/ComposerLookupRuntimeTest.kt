package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.EndpointConfig
import com.viant.agentlysdk.LookupRegistryEntry
import com.viant.agentlysdk.LookupTokenFormat
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class ComposerLookupRuntimeTest {

    @Test
    fun `parses registered slash lookup occurrences only`() {
        val registry = listOf(orderLookupEntry())

        val occurrences = parseComposerLookupOccurrences(
            query = "Troubleshoot /order and ignore /unknown",
            registry = registry
        )

        assertEquals(1, occurrences.size)
        assertEquals("order#0", occurrences[0].key)
        assertEquals("Order", occurrences[0].title)
        assertEquals(true, occurrences[0].required)
        assertEquals("/order", "Troubleshoot /order and ignore /unknown".substring(occurrences[0].displayRange))
    }

    @Test
    fun `required lookup must be selected before resolving prompt`() {
        val registry = listOf(orderLookupEntry())

        assertThrows(ComposerLookupUnresolvedRequiredException::class.java) {
            resolveComposerQuery(
                query = "Troubleshoot /order",
                registry = registry,
                selections = emptyMap()
            )
        }
    }

    @Test
    fun `selected lookup resolves to model prompt through stored token`() {
        val registry = listOf(orderLookupEntry())
        val occurrence = parseComposerLookupOccurrences("Troubleshoot /order", registry).single()
        val selection = composerLookupSelection(
            occurrence = occurrence,
            row = mapOf(
                "id" to JsonPrimitive("fixture-order-1"),
                "name" to JsonPrimitive("Fixture Order")
            )
        )

        val resolved = resolveComposerQuery(
            query = "Troubleshoot /order",
            registry = registry,
            selections = mapOf(occurrence.key to selection)
        )

        assertEquals("Troubleshoot order fixture-order-1", resolved)
        assertEquals("Fixture Order", selection.label)
    }

    @Test
    fun `submission resolution returns unresolved required lookup before sending`() {
        val registry = listOf(orderLookupEntry())

        val resolution = resolveComposerLookupSubmission(
            query = "Troubleshoot /order",
            registry = registry,
            selections = emptyMap()
        )

        assertEquals(null, resolution.resolvedQuery)
        assertEquals("Order", resolution.unresolvedRequiredLookup?.title)
    }

    @Test
    fun `submission resolution returns flattened prompt after lookup selection`() {
        val registry = listOf(orderLookupEntry())
        val occurrence = parseComposerLookupOccurrences("Troubleshoot /order", registry).single()
        val selection = composerLookupSelection(
            occurrence = occurrence,
            row = mapOf(
                "id" to JsonPrimitive("fixture-order-1"),
                "name" to JsonPrimitive("Fixture Order")
            )
        )

        val resolution = resolveComposerLookupSubmission(
            query = "Troubleshoot /order",
            registry = registry,
            selections = mapOf(occurrence.key to selection)
        )

        assertEquals("Troubleshoot order fixture-order-1", resolution.resolvedQuery)
        assertEquals(null, resolution.unresolvedRequiredLookup)
    }

    @Test
    fun `lookup selections are pruned when prompt no longer contains occurrence`() {
        val registry = listOf(orderLookupEntry())
        val occurrence = parseComposerLookupOccurrences("Troubleshoot /order", registry).single()
        val selection = ComposerLookupSelection(token = """@{order:fixture-order-1 "Fixture Order"}""", label = "Fixture Order")

        val pruned = pruneComposerLookupSelections(
            query = "Troubleshoot delivery",
            registry = registry,
            selections = mapOf(occurrence.key to selection)
        )

        assertEquals(emptyMap<String, ComposerLookupSelection>(), pruned)
    }

    @Test
    fun `lookup row datasource includes active conversation id when present`() = runBlocking {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(
                MockResponse()
                    .setHeader("Content-Type", "application/json")
                    .setBody("""{"rows":[{"id":"fixture-order-1","name":"Fixture Order"}]}""")
            )
            val client = AgentlyClient(
                endpoints = mapOf(
                    "appAPI" to EndpointConfig(baseUrl = server.url("/").toString().trimEnd('/'))
                )
            )

            val rows = loadComposerLookupRows(
                client = client,
                entry = orderLookupEntry(),
                searchText = "fixture",
                activeConversationId = " conv-1 "
            )

            val recorded = server.takeRequest()
            assertEquals("/v1/api/datasources/order_lookup/fetch", recorded.path)
            val body = Json.parseToJsonElement(recorded.body.readUtf8()).jsonObject
            assertEquals("conv-1", body["conversationId"]?.jsonPrimitive?.content)
            assertEquals("fixture", body["inputs"]?.jsonObject?.get("q")?.jsonPrimitive?.content)
            assertEquals("fixture-order-1", rows.first()["id"]?.jsonPrimitive?.content)
        } finally {
            server.shutdown()
        }
    }

    private fun orderLookupEntry(): LookupRegistryEntry {
        return LookupRegistryEntry(
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
    }
}
