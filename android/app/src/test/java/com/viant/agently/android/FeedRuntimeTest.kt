package com.viant.agently.android

import com.viant.agentlysdk.stream.ActiveFeed
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonElement
import com.viant.agentlysdk.ActiveFeedState
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.FeedPresentation
import androidx.compose.ui.graphics.Color
import com.viant.forgeandroid.runtime.deduplicateFileBrowserRows
import com.viant.forgeandroid.runtime.previousTextFromUnifiedDiff
import com.viant.forgeandroid.runtime.DataSourceDef
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.WindowMetadata
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class FeedRuntimeTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun `visible feeds hide only explicitly developer-only metadata`() {
        val feeds = listOf(
            ActiveFeed(feedId = "plan", title = "Plan", itemCount = 1),
            ActiveFeed(feedId = "terminal", title = "Terminal", itemCount = 1, developerOnly = true)
        )

        assertEquals(listOf("plan"), visibleFeeds(feeds).map { it.feedId })
    }

    @Test
    fun `feed presentation target selects native placement and inline owner`() {
        val feeds = listOf(
            ActiveFeed("auto", "Auto", 1),
            ActiveFeed("inline", "Inline", 1, turnId = "turn-1", presentation = FeedPresentation(target = "inline")),
            ActiveFeed("workspace", "Workspace", 1, presentation = FeedPresentation(target = "workspace")),
            ActiveFeed("detached", "Detached", 1, presentation = FeedPresentation(target = "detached")),
            ActiveFeed("future", "Future", 1, presentation = FeedPresentation(target = "future"))
        )

        assertEquals(listOf("auto", "future", "workspace"), feedsForPlacement(feeds, AndroidFeedPlacement.Workspace).map { it.feedId })
        assertEquals(listOf("detached"), feedsForPlacement(feeds, AndroidFeedPlacement.Detached).map { it.feedId })
        assertEquals(listOf("inline"), inlineFeedsForTurn(feeds, "turn-1").map { it.feedId })
        assertEquals(emptyList<String>(), inlineFeedsForTurn(feeds, "turn-2").map { it.feedId })
        assertEquals(
            setOf("legacy-plan"),
            suppressedFeedReportIds(
                listOf(ActiveFeed("plan", "Plan", 1, presentation = FeedPresentation(suppressReportIds = listOf(" legacy-plan "))))
            )
        )
    }

    @Test
    fun `inline feed attaches only to final assistant row for owning turn`() {
        val items = listOf(
            ChatEntry("assistant-early", "assistant", "Working", turnId = "turn-feed"),
            ChatEntry("assistant-final", "assistant", "Done", turnId = "turn-feed"),
            ChatEntry("user-later", "user", "Unrelated", turnId = "turn-later")
        )
        val feeds = listOf(
            ActiveFeed(
                "plan",
                "Plan",
                1,
                turnId = "turn-feed",
                presentation = FeedPresentation(target = "inline")
            )
        )

        assertEquals(mapOf("plan" to "assistant-final"), inlineFeedAttachmentItemIds(items, feeds))
    }

    @Test
    fun `android UI bridge exposes and patches a rendered feed draft`() = runBlocking {
        val runtime = ForgeRuntime(emptyMap(), CoroutineScope(Dispatchers.Unconfined))
        val window = runtime.openWindowInline(
            windowKey = "feed-plan-conv-1",
            metadata = WindowMetadata(dataSources = mapOf("items" to DataSourceDef())),
            conversationId = "conv-1",
            presentation = "inline"
        )
        runtime.windowContext(window.windowId).contextOrNull("items")!!.collection.set(
            listOf(mapOf("value" to 1), mapOf("value" to 2))
        )

        val getResult = handleAndroidUIBridgeCommand(
            "ui.feed.get",
            buildJsonObject {
                put("conversationId", JsonPrimitive("conv-1"))
                put("feedId", JsonPrimitive("plan"))
                put("dataSourceRefs", buildJsonArray { add(JsonPrimitive("items")) })
            },
            runtime
        )
        assertNotNull(getResult["dataSources"])

        val updateResult = handleAndroidUIBridgeCommand(
            "ui.feed.update",
            buildJsonObject {
                put("conversationId", JsonPrimitive("conv-1"))
                put("feedId", JsonPrimitive("plan"))
                put("operations", buildJsonArray {
                    add(buildJsonObject {
                        put("dataSourceRef", JsonPrimitive("items"))
                        put("op", JsonPrimitive("replace"))
                        put("path", JsonPrimitive("/collection/1/value"))
                        put("value", JsonPrimitive(20))
                    })
                })
            },
            runtime
        )

        assertEquals("true", updateResult["ok"].toString())
        assertEquals(20L, runtime.windowContext(window.windowId).contextOrNull("items")!!.collection.peek()[1]["value"])
    }

    @Test
    fun `canonical feed patches recompute parents children and selection identity`() = runBlocking {
        val runtime = ForgeRuntime(emptyMap(), CoroutineScope(Dispatchers.Unconfined))
        val payload = com.viant.agentlysdk.FeedDataResponse(
            feedId = "plan",
            title = "Plan",
            data = json.parseToJsonElement(
                """
                {"output":{"record":{"title":"Original","dates":{"start":{"year":2026,"month":8,"day":1},"end":{"year":2026,"month":8,"day":31}},"codes":[],"channels":[{"name":"CTV","publishers":[{"name":"One","cost":1}]}],"items":[
                  {"id":"a","value":1},
                  {"id":"b","value":2},
                  {"id":"c","value":3}
                ]}}}
                """.trimIndent()
            ),
            dataSources = json.parseToJsonElement(
                """
                {
                  "root":{"source":"output"},
                  "record":{"dataSourceRef":"root","selectors":{"data":"record"}},
                  "editDraft":{
                    "dataSourceRef":"record",
                    "fields":{
                      "title":"title",
                      "window":{"transform":"dateRange","startPath":"dates.start","endPath":"dates.end"}
                    }
                  },
                  "items":{
                    "dataSourceRef":"record",
                    "selectors":{"data":"items"},
                    "selectionMode":"multi",
                    "uniqueKey":[{"field":"id"}]
                  },
                  "codes":{"dataSourceRef":"record","selectors":{"data":"codes"},"flatten":{"sources":[{"path":"$","fields":{"code":"$"}}]},"uniqueKey":[{"field":"code"}]},
                  "publishers":{"dataSourceRef":"record","selectors":{"data":"channels"},"flatten":{"sources":[{
                    "path":"publishers","parentFields":{"Channel":"name"},"values":{"InventoryType":"Publisher"},"fields":{"Name":"name","Cost":"cost"}
                  }]},"uniqueKey":[{"field":"Channel"},{"field":"InventoryType"},{"field":"Name"}]
                  }
                }
                """.trimIndent()
            ) as JsonObject,
            ui = json.parseToJsonElement(
                """{"containers":[{"id":"items","dataSourceRef":"items"}]}"""
            ) as JsonObject
        )
        val metadata = buildFeedWindowMetadata(payload)
        val window = runtime.openWindowInline(
            windowKey = "feed-plan-conv-1",
            metadata = metadata,
            conversationId = "conv-1",
            presentation = "inline"
        )
        wireFeedWindow(runtime, window.windowId, payload, "turn-1")
        val items = runtime.windowContext(window.windowId).contextOrNull("items")!!
        items.setSelection(com.viant.forgeandroid.runtime.SelectionState(selection = listOf(items.collection.peek()[1])))

        suspend fun update(vararg operations: JsonObject) {
            handleAndroidUIBridgeCommand(
                "ui.feed.update",
                buildJsonObject {
                    put("conversationId", JsonPrimitive("conv-1"))
                    put("turnId", JsonPrimitive("turn-1"))
                    put("feedId", JsonPrimitive("plan"))
                    put("operations", buildJsonArray { operations.forEach(::add) })
                },
                runtime
            )
        }
        fun operation(ref: String, op: String, path: String, value: JsonElement? = null) = buildJsonObject {
            put("dataSourceRef", JsonPrimitive(ref))
            put("op", JsonPrimitive(op))
            put("path", JsonPrimitive(path))
            value?.let { put("value", it) }
        }

        update(operation("items", "replace", "/collection/1/value", JsonPrimitive(20)))
        assertEquals(listOf(1L, 20L, 3L), items.collection.peek().map { it["value"] })
        val record = runtime.windowContext(window.windowId).contextOrNull("record")!!
        assertEquals(20L, ((record.peekForm()["items"] as List<*>)[1] as Map<*, *>)["value"])

        update(operation("record", "replace", "/form/items/0/value", JsonPrimitive(10)))
        assertEquals(listOf(10L, 20L, 3L), items.collection.peek().map { it["value"] })

        update(
            operation("items", "remove", "/collection/2"),
            operation("items", "add", "/collection/-", json.parseToJsonElement("""{"id":"d","value":4}"""))
        )
        assertEquals(listOf("a", "b", "d"), items.collection.peek().map { it["id"] })

        update(operation("editDraft", "replace", "/form/window/start", JsonPrimitive("2026-08-05")))
        assertEquals("Original", record.peekForm()["title"])
        assertEquals("2026-08-05", ((record.peekForm()["dates"] as Map<*, *>)["start"]))

        update(operation(
            "publishers",
            "add",
            "/collection/-",
            json.parseToJsonElement("""{"Channel":"CTV","InventoryType":"Publisher","Name":"Two","Cost":2}""")
        ))
        val channels = record.peekForm()["channels"] as List<*>
        val rawPublishers = (channels[0] as Map<*, *>)["publishers"] as List<*>
        assertEquals(listOf("One", "Two"), rawPublishers.map { (it as Map<*, *>)["name"] })

        update(operation("codes", "add", "/collection/-", json.parseToJsonElement("""{"code":"AK"}""")))
        assertEquals(listOf("AK"), runtime.windowContext(window.windowId).contextOrNull("codes")!!.collection.peek().map { it["code"] })
        assertEquals(listOf("AK"), record.peekForm()["codes"])

        update(operation("items", "replace", "/selection/selection/0/value", JsonPrimitive(25)))
        assertEquals(25L, items.collection.peek()[1]["value"])
        assertEquals(25L, items.peekSelection().selection.single()["value"])

        wireFeedWindow(runtime, window.windowId, payload, "turn-1")
        assertEquals(25L, items.collection.peek()[1]["value"])
        wireFeedWindow(runtime, window.windowId, payload, "turn-2")
        assertEquals(listOf(1L, 2L, 3L), items.collection.peek().map { it["value"] })
    }

    @Test
    fun `native feed projection covers fields flatten exclude aggregate derive and numeric selectors`() {
        val definitions = json.parseToJsonElement(
            """
            {
              "root":{"source":"output"},
              "plan":{"dataSourceRef":"root","selectors":{"data":"plan"}},
              "overview":{"dataSourceRef":"plan","fields":{
                "name":"name",
                "active":{"path":"active_flag","transform":"boolean"},
                "flight":{"transform":"dateRangeLabel","startPath":"dates.start","endPath":"dates.end"}
              }},
              "publishers":{"dataSourceRef":"plan","selectors":{"data":"channels"},"flatten":{"sources":[
                {"path":"publishers","exclude":{"field":"name","equals":"TOTAL"},"parentFields":{"channel":"name"},"values":{"kind":"Publisher"},"fields":{"publisher":"name","cost":"cost"}}
              ]},"uniqueKey":[{"field":"channel"},{"field":"publisher"}]},
              "coverage":{"dataSourceRef":"publishers","aggregate":{"countAs":"count"}},
              "segments":{"dataSourceRef":"plan","selectors":{"data":"segments"},"exclude":{"field":"name","equalsIgnoreCase":"TOTAL"},"derive":{"label":"${'$'}{id}:${'$'}{name}"}},
              "secondCode":{"dataSourceRef":"plan","selectors":{"data":"codes[1]"},"flatten":{"sources":[{"path":"$","fields":{"code":"$"}}]}}
            }
            """.trimIndent()
        ) as JsonObject
        val data = json.parseToJsonElement(
            """
            {"output":{"plan":{
              "name":"Plan A","active_flag":"true",
              "dates":{"start":{"year":2026,"month":8,"day":1},"end":{"year":2026,"month":8,"day":31}},
              "channels":[{"name":"CTV","publishers":[{"name":"One","cost":3},{"name":"TOTAL","cost":3}]}],
              "segments":[{"id":"s1","name":"Sports"},{"id":"total","name":"total"}],
              "codes":["CA","NY"]
            }}}
            """.trimIndent()
        )

        val computed = computeFeedCollections(definitions, data).collections

        assertEquals("Plan A", computed.getValue("overview").single()["name"])
        assertEquals(true, computed.getValue("overview").single()["active"])
        assertEquals("2026-08-01 – 2026-08-31", computed.getValue("overview").single()["flight"])
        assertEquals(listOf(mapOf("publisher" to "One", "cost" to 3L, "channel" to "CTV", "kind" to "Publisher")), computed["publishers"])
        assertEquals(1, computed.getValue("coverage").single()["count"])
        assertEquals("s1:Sports", computed.getValue("segments").single()["label"])
        assertEquals("NY", computed.getValue("secondCode").single()["code"])
    }

    @Test
    fun `feed accent comes from generic presentation metadata`() {
        assertEquals(Color(0xFF0A9B98), toolFeedAccent(FeedPresentation(icon = "changes", accent = "teal")))
        assertEquals(Color(0xFF5965D8), toolFeedAccent(null))
    }

    @Test
    fun `file preview primitives dedupe and reconstruct previous content`() {
        val rows = listOf(
            mapOf<String, Any?>("url" to "/tmp/a.txt", "diff" to "first"),
            mapOf<String, Any?>("url" to "/tmp/a.txt", "diff" to "latest")
        )
        assertEquals("latest", deduplicateFileBrowserRows(rows, "url").single()["diff"])
        assertEquals(
            "old\nkeep\n",
            previousTextFromUnifiedDiff("new\nkeep\nadded\n", "@@ -1,2 +1,3 @@\n-old\n+new\n keep\n+added")
        )
    }

    @Test
    fun `merged visible feeds preserve persisted feeds and let live metadata win`() {
        val state = ConversationStateResponse(
            feeds = listOf(ActiveFeedState("plan", "Persisted Plan", 1, turnId = "turn-persisted"))
        )
        assertEquals("Persisted Plan", mergedVisibleFeeds(null, state, "conv-1").first().title)
        assertEquals("turn-persisted", mergedVisibleFeeds(null, state, "conv-1").first().turnId)

        val snapshot = com.viant.agentlysdk.stream.ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = null,
            feeds = listOf(ActiveFeed("plan", "Live Plan", 2, conversationId = "conv-1")),
            pendingElicitation = null,
            bufferedMessages = emptyList(),
            liveExecutionGroupsById = emptyMap()
        )
        val merged = mergedVisibleFeeds(snapshot, state, "conv-1")
        assertEquals(1, merged.size)
        assertEquals("Live Plan", merged.first().title)
    }

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
    fun `buildFeedWindowMetadata adds remote contexts for governed lookup dependencies`() {
        val payload = com.viant.agentlysdk.FeedDataResponse(
            feedId = "plan",
            dataSources = json.parseToJsonElement("""{"selected":{"source":"output.rows"}}""") as JsonObject,
            ui = json.parseToJsonElement(
                """{"containers":[{"id":"deals","kind":"dashboard.lookupChips","dataSourceRef":"selected","lookup":{"dataSourceRef":"deal_catalog","drill":{"dataSourceRef":"deal_children"}}}]}"""
            ) as JsonObject
        )

        val metadata = buildFeedWindowMetadata(payload)

        assertEquals("/v1/api/datasources/deal_catalog/fetch", metadata.dataSources?.get("deal_catalog")?.service?.uri)
        assertEquals("/v1/api/datasources/deal_children/fetch", metadata.dataSources?.get("deal_children")?.service?.uri)
        assertEquals(false, metadata.dataSources?.get("deal_catalog")?.autoFetch)
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
