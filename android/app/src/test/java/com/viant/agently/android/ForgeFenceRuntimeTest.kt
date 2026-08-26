package com.viant.agently.android

import com.viant.forgeandroid.ui.TranscriptCanonicalData
import com.viant.forgeandroid.ui.TranscriptCanonicalReport
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.net.SocketTimeoutException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ForgeFenceRuntimeTest {

    @Test
    fun `pending inline report is inactive and exposes build counts`() {
        val report = TranscriptCanonicalReport(
            scope = "message",
            id = "delivery",
            grammar = "report-document-v1",
            status = "rendering",
            source = JsonObject(mapOf(
                "title" to JsonPrimitive("Delivery"),
                "blocks" to JsonArray(listOf(
                    JsonObject(mapOf("id" to JsonPrimitive("one"))),
                    JsonObject(mapOf("id" to JsonPrimitive("two")))
                ))
            )),
            dataSources = mapOf(
                "summary" to TranscriptCanonicalData(id = "summary"),
                "trend" to TranscriptCanonicalData(id = "trend")
            )
        )

        assertTrue(isPendingInlineReport(report))
        assertEquals("Building report · 2 data sources · 2 blocks", inlineReportBuildStatus(report))
        assertEquals(false, isPendingInlineReport(report.copy(status = "committed")))
    }

    @Test
    fun `inline report presentation errors never expose implementation class names`() {
        val missingCompiler = NoClassDefFoundError(
            "com.viant.forgeandroid.runtime.InlineReportRuntimeCompiler"
        )

        assertEquals(
            "Unable to open this report. Try again.",
            inlineReportPresentationErrorMessage(missingCompiler)
        )
        assertEquals(
            "The report took too long to open. Try again.",
            inlineReportPresentationErrorMessage(SocketTimeoutException("request timed out"))
        )
    }

    @Test
    fun `inline report export fences preserve report and hydrated data without ids leaking into labels`() {
        val report = TranscriptCanonicalReport(
            scope = "message",
            id = "performance",
            grammar = "report-document-v1",
            status = "committed",
            source = JsonObject(
                mapOf(
                    "title" to JsonPrimitive("Performance"),
                    "scope" to JsonPrimitive("authored-scope"),
                    "blocks" to JsonArray(emptyList())
                )
            ),
            dataSources = mapOf(
                "rows" to TranscriptCanonicalData(
                    version = 2,
                    id = "transport_rows",
                    format = "json",
                    payload = JsonArray(listOf(JsonObject(mapOf("spend" to JsonPrimitive(12)))))
                )
            )
        )

        val fences = inlineReportExportFences(report)

        assertEquals(listOf("forge-report", "forge-data", "forge-report"), fences.map { it["kind"] })
        assertEquals(JsonPrimitive("start"), (fences[0]["payload"] as JsonObject)["mode"])
        assertEquals(JsonPrimitive("replace"), (fences[1]["payload"] as JsonObject)["mode"])
        assertEquals(JsonPrimitive("commit"), (fences[2]["payload"] as JsonObject)["mode"])
        assertEquals(JsonPrimitive("authored-scope"), (fences[0]["payload"] as JsonObject)["scope"])
        assertEquals(JsonPrimitive("authored-scope"), (fences[1]["payload"] as JsonObject)["scope"])
        assertEquals(JsonPrimitive("authored-scope"), (fences[2]["payload"] as JsonObject)["scope"])
        assertEquals(JsonPrimitive("rows"), (fences[1]["payload"] as JsonObject)["id"])
    }

    @Test
    fun `inline report export preserves authored canonical source without host rewrites`() {
        val source = JsonObject(mapOf(
            "blocks" to JsonArray(listOf(
                JsonObject(mapOf(
                    "id" to JsonPrimitive("spend"),
                    "kind" to JsonPrimitive("kpiBlock"),
                    "subtitle" to JsonPrimitive("Last seven days"),
                    "suffix" to JsonPrimitive("x"),
                    "tone" to JsonPrimitive("warning")
                )),
                JsonObject(mapOf(
                    "id" to JsonPrimitive("changes"),
                    "kind" to JsonPrimitive("timelineBlock"),
                    "datasetRef" to JsonPrimitive("change_rows"),
                    "timeField" to JsonPrimitive("timestamp"),
                    "titleField" to JsonPrimitive("event"),
                    "descriptionField" to JsonPrimitive("detail")
                )),
                JsonObject(mapOf(
                    "id" to JsonPrimitive("posture"),
                    "kind" to JsonPrimitive("badgesBlock"),
                    "items" to JsonArray(listOf(JsonObject(mapOf("label" to JsonPrimitive("Setup")))))
                )),
                JsonObject(mapOf(
                    "id" to JsonPrimitive("capacity"),
                    "kind" to JsonPrimitive("infoPanelBlock"),
                    "title" to JsonPrimitive("Capacity"),
                    "body" to JsonPrimitive("Capacity detail")
                )),
                JsonObject(mapOf(
                    "id" to JsonPrimitive("delivery"),
                    "kind" to JsonPrimitive("tableBlock"),
                    "title" to JsonPrimitive("Daily delivery"),
                    "description" to JsonPrimitive("Delivery by date"),
                    "link" to JsonObject(mapOf("href" to JsonPrimitive("detailUrl"))),
                    "datasetRef" to JsonPrimitive("daily_rows"),
                    "columns" to JsonArray(listOf(JsonObject(mapOf(
                        "key" to JsonPrimitive("spend"),
                        "label" to JsonPrimitive("Spend"),
                        "type" to JsonPrimitive("link"),
                        "link" to JsonObject(mapOf("href" to JsonPrimitive("detailUrl"))),
                        "cellVisual" to JsonObject(mapOf("kind" to JsonPrimitive("dataBar")))
                    ))))
                )),
                JsonObject(mapOf(
                    "id" to JsonPrimitive("trend"),
                    "kind" to JsonPrimitive("chartBlock"),
                    "title" to JsonPrimitive("Delivery trend"),
                    "description" to JsonPrimitive("Interactive chart copy"),
                    "datasetRef" to JsonPrimitive("daily_rows"),
                    "chartSpec" to JsonObject(mapOf("type" to JsonPrimitive("line"))),
                    "chartModel" to JsonObject(mapOf("type" to JsonPrimitive("line")))
                )),
                JsonObject(mapOf(
                    "id" to JsonPrimitive("states"),
                    "kind" to JsonPrimitive("geoMapBlock"),
                    "title" to JsonPrimitive("State reach"),
                    "description" to JsonPrimitive("Interactive map copy"),
                    "datasetRef" to JsonPrimitive("state_rows"),
                    "geo" to JsonObject(mapOf("key" to JsonPrimitive("stateCode")))
                )),
                JsonObject(mapOf(
                    "id" to JsonPrimitive("findings"),
                    "kind" to JsonPrimitive("collectionBlock"),
                    "title" to JsonPrimitive("Findings"),
                    "datasetRef" to JsonPrimitive("finding_rows"),
                    "itemTitleField" to JsonPrimitive("finding"),
                    "toneField" to JsonPrimitive("importance"),
                    "bodyTemplate" to JsonPrimitive("**Feasibility:** ${'$'}{row.feasibility}\n**Driver:** ${'$'}{row.driver}\n**Next check:** ${'$'}{row.next_check}")
                ))
            ))
        ))

        val report = TranscriptCanonicalReport(
            scope = "message",
            id = "parity",
            grammar = "report-document-v1",
            status = "committed",
            source = source
        )
        val start = inlineReportExportFences(report).first()["payload"] as JsonObject
        val exportedBlocks = start["blocks"] as JsonArray

        assertEquals(source["blocks"], exportedBlocks)
        assertEquals(JsonPrimitive("x"), (exportedBlocks[0] as JsonObject)["suffix"])
        assertEquals(JsonPrimitive("warning"), (exportedBlocks[0] as JsonObject)["tone"])
        assertEquals(JsonPrimitive("timelineBlock"), (exportedBlocks[1] as JsonObject)["kind"])
        assertEquals(JsonPrimitive("infoPanelBlock"), (exportedBlocks[3] as JsonObject)["kind"])
        assertEquals(JsonPrimitive("geoMapBlock"), (exportedBlocks[6] as JsonObject)["kind"])
        assertEquals(JsonPrimitive("collectionBlock"), (exportedBlocks[7] as JsonObject)["kind"])
    }

    @Test
    fun `parseTranscriptContentParts extracts forge ui blocks with preceding forge data`() {
        val content = listOf(
            "Intro text",
            "```forge-data",
            """{"version":1,"id":"review_items","format":"json","mode":"replace","data":[{"item_id":101,"item_name":"Item Alpha","reason":"Strong overlap","selected":true}]}""",
            "```",
            "```forge-ui",
            """{"version":1,"title":"Review items","blocks":[{"id":"item-review","kind":"planner.table","title":"Item review","dataSourceRef":"review_items","selection":{"mode":"checkbox","field":"selected"},"columns":[{"key":"item_id","label":"Item ID"},{"key":"item_name","label":"Item name"},{"key":"reason","label":"Why selected"}]}]}""",
            "```",
            "Closing text"
        ).joinToString("\n")

        val parts = parseTranscriptContentParts(content)

        assertEquals(3, parts.size)
        assertTrue(parts[0] is TranscriptContentPart.Markdown)
        assertTrue(parts[1] is TranscriptContentPart.ForgeUi)
        assertTrue(parts[2] is TranscriptContentPart.Markdown)
        val forge = parts[1] as TranscriptContentPart.ForgeUi
        assertEquals("Review items", forge.payload.title)
        assertEquals(listOf("review_items"), forge.dataStore.keys.toList())
    }

    @Test
    fun `parseTranscriptContentParts leaves malformed legacy marker plus json fence as markdown`() {
        val content = listOf(
            "forge-data",
            "```json",
            """{"version":1,"id":"summary_metrics","format":"json","mode":"replace","data":[{"primary_value":42}]}""",
            "```",
            "forge-ui",
            "```json",
            """{"version":1,"title":"Legacy dashboard","blocks":[{"id":"summary","kind":"dashboard.summary","dataSourceRef":"summary_metrics","metrics":["primary_value"]}]}""",
            "```"
        ).joinToString("\n")

        val parts = parseTranscriptContentParts(content)

        assertEquals(1, parts.size)
        val markdown = parts.single() as TranscriptContentPart.Markdown
        assertTrue(markdown.text.contains("forge-data"))
        assertTrue(markdown.text.contains("```json"))
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts planner table blocks to forge table containers`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-data",
                """{"version":1,"id":"review_items","format":"json","mode":"replace","data":[{"item_id":101,"item_name":"Item Alpha","reason":"Strong overlap","selected":true}]}""",
                "```",
                "```forge-ui",
                """{"version":1,"title":"Review items","blocks":[{"id":"item-review","kind":"planner.table","title":"Item review","dataSourceRef":"review_items","selection":{"mode":"checkbox","field":"selected"},"columns":[{"key":"item_id","label":"Item ID"},{"key":"item_name","label":"Item name"},{"key":"reason","label":"Why selected"}]}]}""",
                "```"
            ).joinToString("\n")
        )
        val forge = parts.filterIsInstance<TranscriptContentPart.ForgeUi>().first()

        val metadata = buildTranscriptForgeWindowMetadata(forge.payload, forge.dataStore)
        val root = metadata.view?.content?.containers?.single()
        val table = root?.containers?.single()

        assertEquals("Review items", root?.title)
        assertEquals("item-review", table?.id)
        assertEquals("review_items", table?.dataSourceRef)
        assertEquals(listOf("selected", "item_id", "item_name", "reason"), table?.table?.columns?.map { it.id })
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts dashboard summary metric strings`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-data",
                """{"version":1,"id":"summary_metrics","format":"json","mode":"replace","data":[{"primary_value":1316.86,"secondary_ratio":0.17,"success_rate":4.02}]}""",
                "```",
                "```forge-ui",
                """{"version":1,"title":"Record","subtitle":"Group","blocks":[{"id":"summary","kind":"dashboard.summary","dataSourceRef":"summary_metrics","metrics":["primary_value","secondary_ratio","success_rate"]}]}""",
                "```"
            ).joinToString("\n")
        )
        val forge = parts.filterIsInstance<TranscriptContentPart.ForgeUi>().first()

        val metadata = buildTranscriptForgeWindowMetadata(forge.payload, forge.dataStore)
        val summary = metadata.view?.content?.containers?.single()?.containers?.single()

        assertEquals("dashboard.summary", summary?.kind)
        assertEquals("summary_metrics", summary?.dataSourceRef)
        assertEquals(listOf("primary_value", "secondary_ratio", "success_rate"), summary?.metrics?.map { it.id })
        assertEquals(listOf("Primary Value", "Secondary Ratio", "Success Rate"), summary?.metrics?.map { it.label })
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts dashboard timeline into chart config`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-data",
                """{"version":1,"id":"pie_data","format":"json","mode":"replace","data":[{"channel":"Alpha","activity":1316.86},{"channel":"Beta","activity":842.10},{"channel":"Gamma","activity":402.40}]}""",
                "```",
                "```forge-ui",
                """{"version":1,"title":"Android chart verification","subtitle":"Group 4257","blocks":[{"id":"share","kind":"dashboard.timeline","title":"Activity share","dataSourceRef":"pie_data","chartType":"pie","dateField":"channel","series":[{"key":"activity","label":"Activity"}]}]}""",
                "```"
            ).joinToString("\n")
        )
        val forge = parts.filterIsInstance<TranscriptContentPart.ForgeUi>().first()

        val metadata = buildTranscriptForgeWindowMetadata(forge.payload, forge.dataStore)
        val timeline = metadata.view?.content?.containers?.single()?.containers?.single()

        assertEquals("dashboard.timeline", timeline?.kind)
        assertEquals("pie_data", timeline?.dataSourceRef)
        assertEquals("pie", timeline?.chart?.type)
        assertEquals("channel", timeline?.chart?.xAxis?.dataKey)
        assertEquals("activity", timeline?.chart?.series?.valueKey)
        assertEquals(listOf("activity"), timeline?.chart?.series?.values?.map { it.value })
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts dashboard table blocks`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-data",
                """{"version":1,"id":"summary_metrics","format":"json","mode":"replace","data":[{"record_name":"Record Alpha","primary_status":"Input mismatch"}]}""",
                "```",
                "```forge-ui",
                """{"version":1,"title":"Android dashboard verification","subtitle":"Group 4257","blocks":[{"id":"primary-evidence","kind":"dashboard.table","title":"Primary evidence","dataSourceRef":"summary_metrics","columns":[{"key":"record_name","label":"Record","format":"text","type":"link","link":{"href":"record_detail_url"}},{"key":"primary_status","label":"Primary status","format":"text"}]}]}""",
                "```"
            ).joinToString("\n")
        )
        val forge = parts.filterIsInstance<TranscriptContentPart.ForgeUi>().first()

        val metadata = buildTranscriptForgeWindowMetadata(forge.payload, forge.dataStore)
        val table = metadata.view?.content?.containers?.single()?.containers?.single()

        assertEquals("summary_metrics", table?.dataSourceRef)
        assertEquals(listOf("record_name", "primary_status"), table?.table?.columns?.map { it.id })
        assertEquals(listOf("Record", "Primary status"), table?.table?.columns?.map { it.label })
        assertEquals(listOf("text", "text"), table?.table?.columns?.map { it.format })
        assertEquals("link", table?.table?.columns?.firstOrNull()?.type)
        assertEquals("record_detail_url", table?.table?.columns?.firstOrNull()?.link?.href)
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts summary items report and kpi table blocks`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-data",
                """{"version":1,"id":"recent_activity","format":"json","mode":"replace","data":[{"total_value":6061.727,"review_status":"behind"}]}""",
                "```",
                "```forge-ui",
                """{"version":1,"title":"Policy review","subtitle":"Blocked before execution","blocks":[{"kind":"dashboard.summary","title":"Review summary","items":[{"label":"Record","value":"Record Alpha (2657754)"},{"label":"Submission status","value":"Blocked"}]},{"kind":"dashboard.report","title":"Why this report item is not yet safe","sections":[{"id":"interpretation","title":"Interpretation","body":["Wait until the current threshold is confirmed."]}]},{"kind":"dashboard.kpiTable","title":"Recent activity posture","dataSourceRef":"recent_activity","columns":[{"key":"total_value","label":"Total value"},{"key":"review_status","label":"Review status"}]}]}""",
                "```"
            ).joinToString("\n")
        )
        val forge = parts.filterIsInstance<TranscriptContentPart.ForgeUi>().first()

        val metadata = buildTranscriptForgeWindowMetadata(forge.payload, forge.dataStore)
        val containers = metadata.view?.content?.containers?.single()?.containers.orEmpty()

        assertEquals("dashboard.summary", containers.getOrNull(0)?.kind)
        assertEquals(listOf("Record", "Submission status"), containers.getOrNull(0)?.metrics?.map { it.label })
        assertEquals("dashboard.report", containers.getOrNull(1)?.kind)
        assertEquals("Interpretation", containers.getOrNull(1)?.sections?.firstOrNull()?.title)
        assertEquals(listOf("total_value", "review_status"), containers.getOrNull(2)?.table?.columns?.map { it.id })
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts dimensions and messages blocks`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-ui",
                """{"version":1,"title":"Android dashboard verification","subtitle":"Group 4257","blocks":[{"kind":"dashboard.dimensions","title":"Region concentration","dataSourceRef":"region_breakdown","dimension":{"key":"region_id","label":"Region"},"metric":{"key":"value_share","label":"Value share","format":"percentFraction"},"viewModes":["chart","table"],"limit":10},{"kind":"dashboard.messages","title":"Next action","items":[{"title":"Primary next step","body":"Validate threshold next.","severity":"warning"}]}]}""",
                "```"
            ).joinToString("\n")
        )
        val forge = parts.filterIsInstance<TranscriptContentPart.ForgeUi>().first()

        val metadata = buildTranscriptForgeWindowMetadata(forge.payload, forge.dataStore)
        val containers = metadata.view?.content?.containers?.single()?.containers.orEmpty()

        assertEquals("dashboard.dimensions", containers.getOrNull(0)?.kind)
        assertEquals("region_id", containers.getOrNull(0)?.dimension?.key)
        assertEquals("value_share", containers.getOrNull(0)?.metric?.key)
        assertEquals(listOf("chart", "table"), containers.getOrNull(0)?.viewModes)
        assertEquals(10, containers.getOrNull(0)?.limit)

        assertEquals("dashboard.messages", containers.getOrNull(1)?.kind)
        assertEquals("Primary next step", containers.getOrNull(1)?.items?.firstOrNull()?.title)
        assertEquals("warning", containers.getOrNull(1)?.items?.firstOrNull()?.severity)
    }
}
