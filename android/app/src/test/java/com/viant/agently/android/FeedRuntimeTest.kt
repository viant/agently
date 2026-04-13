package com.viant.agently.android

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class FeedRuntimeTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun `computeFeedCollections resolves root and child data sources from feed payload`() {
        val dataSources = json.parseToJsonElement(
            """
            {
              "snapshot": { "source": "output" },
              "changes": {
                "dataSourceRef": "snapshot",
                "selectors": { "data": "changes" }
              }
            }
            """.trimIndent()
        ) as JsonObject
        val feedData = json.parseToJsonElement(
            """
            {
              "output": {
                "changes": [
                  { "path": "foo.go", "action": "modify" },
                  { "path": "bar.go", "action": "add" }
                ]
              }
            }
            """.trimIndent()
        )

        val collections = computeFeedCollections(dataSources, feedData)

        assertEquals("snapshot", collections.rootDataSource)
        assertEquals(
            listOf(
                mapOf(
                    "changes" to listOf(
                        mapOf("path" to "foo.go", "action" to "modify"),
                        mapOf("path" to "bar.go", "action" to "add")
                    )
                )
            ),
            collections.collections["snapshot"]
        )
        assertEquals(
            listOf(
                mapOf("path" to "foo.go", "action" to "modify"),
                mapOf("path" to "bar.go", "action" to "add")
            ),
            collections.collections["changes"]
        )
    }

    @Test
    fun `buildFeedWindowMetadata decodes feed ui and drops local paging`() {
        val payload = com.viant.agentlysdk.FeedDataResponse(
            feedId = "explorer",
            title = "Explorer",
            dataSources = json.parseToJsonElement(
                """
                {
                  "results": {
                    "source": "output.files",
                    "paging": { "enabled": true, "size": 3 }
                  }
                }
                """.trimIndent()
            ) as JsonObject,
            ui = json.parseToJsonElement(
                """
                {
                  "title": "Explorer",
                  "containers": [
                    {
                      "id": "results",
                      "title": "Results"
                    }
                  ]
                }
                """.trimIndent()
            ) as JsonObject
        )

        val metadata = buildFeedWindowMetadata(payload)

        assertNotNull(metadata)
        assertEquals("results", metadata.view?.content?.containers?.lastOrNull()?.id)
        assertNull(metadata.dataSources?.get("results")?.paging)
    }

    @Test
    fun `buildFeedWindowMetadata decodes content-shaped ui payloads`() {
        val payload = com.viant.agentlysdk.FeedDataResponse(
            feedId = "plan",
            title = "Plan",
            ui = json.parseToJsonElement(
                """
                {
                  "title": "Plan",
                  "containers": [
                    { "id": "header", "items": [{ "id": "explanation", "type": "label" }] },
                    { "id": "planTable", "type": "table" }
                  ]
                }
                """.trimIndent()
            ) as JsonObject
        )

        val metadata = buildFeedWindowMetadata(payload)

        assertNotNull(metadata)
        assertEquals(listOf("header", "planTable"), metadata.view?.content?.containers?.map { it.id })
    }

    @Test
    fun `buildFeedWindowMetadata wraps single container ui payloads`() {
        val payload = com.viant.agentlysdk.FeedDataResponse(
            feedId = "explorer",
            title = "Explorer",
            ui = json.parseToJsonElement(
                """
                {
                  "id": "results",
                  "title": "Results",
                  "type": "table"
                }
                """.trimIndent()
            ) as JsonObject
        )

        val metadata = buildFeedWindowMetadata(payload)

        assertNotNull(metadata)
        assertEquals(listOf("results"), metadata.view?.content?.containers?.map { it.id })
    }

    @Test
    fun `normalizeFeedDataSources adds missing parent placeholders`() {
        val dataSources = json.parseToJsonElement(
            """
            {
              "changes": {
                "dataSourceRef": "snapshot",
                "selectors": { "data": "changes" }
              }
            }
            """.trimIndent()
        ) as JsonObject

        val normalized = normalizeFeedDataSources(dataSources)

        assertEquals(setOf("changes", "snapshot"), normalized.keys)
        assertEquals(emptySet<String>(), normalized.getValue("snapshot").keys)
    }

    @Test
    fun `resolveRootFeedDataSource prefers explicit output or input source`() {
        val dataSources = json.parseToJsonElement(
            """
            {
              "child": { "dataSourceRef": "snapshot" },
              "snapshot": { "source": "output" },
              "other": { "source": "details" }
            }
            """.trimIndent()
        ) as JsonObject

        val normalized = normalizeFeedDataSources(dataSources)

        assertEquals("snapshot", resolveRootFeedDataSource(normalized))
    }

    @Test
    fun `resolveRootFeedDataSource falls back to first top level data source`() {
        val dataSources = json.parseToJsonElement(
            """
            {
              "details": { "source": "details" },
              "child": { "dataSourceRef": "details" }
            }
            """.trimIndent()
        ) as JsonObject

        val normalized = normalizeFeedDataSources(dataSources)

        assertEquals("details", resolveRootFeedDataSource(normalized))
    }

    @Test
    fun `selectPath supports bracket notation and implicit output prefix stripping`() {
        val root = mapOf(
            "items" to listOf(
                mapOf("name" to "first"),
                mapOf("name" to "second")
            )
        )

        assertEquals("second", selectPath("output.items[1].name", root))
        assertEquals("second", selectPath(".items[1].name", root))
    }

    @Test
    fun `selectPath parser ignores empty selector segments and bracket spacing`() {
        val root = mapOf(
            "items" to listOf(
                mapOf("name" to "first"),
                mapOf("name" to "second")
            )
        )

        assertEquals("second", selectPath("..items[ 1 ].name", root))
    }

    @Test
    fun `selectPath returns root for direct output selector when channel missing`() {
        val root = mapOf("value" to "plain")

        assertEquals(root, selectPath("output", root))
        assertEquals(root, selectPath("input", root))
    }

    @Test
    fun `computeFeedCollections unwraps single parent rows for child selectors`() {
        val dataSources = json.parseToJsonElement(
            """
            {
              "snapshot": { "source": "output" },
              "items": {
                "dataSourceRef": "snapshot",
                "selectors": { "data": "items" }
              }
            }
            """.trimIndent()
        ) as JsonObject
        val feedData = json.parseToJsonElement(
            """
            {
              "output": {
                "items": [
                  { "label": "one" },
                  { "label": "two" }
                ]
              }
            }
            """.trimIndent()
        )

        val collections = computeFeedCollections(dataSources, feedData)

        assertEquals(
            listOf(
                mapOf("label" to "one"),
                mapOf("label" to "two")
            ),
            collections.collections["items"]
        )
    }

    @Test
    fun `buildFeedWindowMetadata throws when ui cannot decode`() {
        val payload = com.viant.agentlysdk.FeedDataResponse(
            feedId = "broken",
            title = "Broken",
            ui = json.parseToJsonElement(
                """
                {
                  "title": "Broken",
                  "containers": "not-an-array"
                }
                """.trimIndent()
            ) as JsonObject
        )

        val error = kotlin.runCatching {
            buildFeedWindowMetadata(payload)
        }.exceptionOrNull()

        assertNotNull(error)
    }

    @Test
    fun `buildFeedWindowMetadata throws when datasource is not an object`() {
        val payload = com.viant.agentlysdk.FeedDataResponse(
            feedId = "broken-ds",
            title = "Broken",
            dataSources = json.parseToJsonElement(
                """
                {
                  "results": "not-an-object"
                }
                """.trimIndent()
            ) as JsonObject,
            ui = json.parseToJsonElement(
                """
                {
                  "id": "results",
                  "type": "table"
                }
                """.trimIndent()
            ) as JsonObject
        )

        val error = kotlin.runCatching {
            buildFeedWindowMetadata(payload)
        }.exceptionOrNull()

        assertNotNull(error)
    }
}
