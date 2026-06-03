package com.viant.agently.android

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ForgeFenceRuntimeTest {

    @Test
    fun `parseTranscriptContentParts extracts forge ui blocks with preceding forge data`() {
        val content = listOf(
            "Intro text",
            "```forge-data",
            """{"version":1,"id":"recommended_sites","format":"json","mode":"replace","data":[{"site_id":101,"site_name":"example.com","reason":"Strong overlap","selected":true}]}""",
            "```",
            "```forge-ui",
            """{"version":1,"title":"Recommended sites","blocks":[{"id":"site-review","kind":"planner.table","title":"Site review","dataSourceRef":"recommended_sites","selection":{"mode":"checkbox","field":"selected"},"columns":[{"key":"site_id","label":"Site ID"},{"key":"site_name","label":"Site name"},{"key":"reason","label":"Why recommended"}]}]}""",
            "```",
            "Closing text"
        ).joinToString("\n")

        val parts = parseTranscriptContentParts(content)

        assertEquals(3, parts.size)
        assertTrue(parts[0] is TranscriptContentPart.Markdown)
        assertTrue(parts[1] is TranscriptContentPart.ForgeUi)
        assertTrue(parts[2] is TranscriptContentPart.Markdown)
        val forge = parts[1] as TranscriptContentPart.ForgeUi
        assertEquals("Recommended sites", forge.payload.title)
        assertEquals(listOf("recommended_sites"), forge.dataStore.keys.toList())
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts planner table blocks to forge table containers`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-data",
                """{"version":1,"id":"recommended_sites","format":"json","mode":"replace","data":[{"site_id":101,"site_name":"example.com","reason":"Strong overlap","selected":true}]}""",
                "```",
                "```forge-ui",
                """{"version":1,"title":"Recommended sites","blocks":[{"id":"site-review","kind":"planner.table","title":"Site review","dataSourceRef":"recommended_sites","selection":{"mode":"checkbox","field":"selected"},"columns":[{"key":"site_id","label":"Site ID"},{"key":"site_name","label":"Site name"},{"key":"reason","label":"Why recommended"}]}]}""",
                "```"
            ).joinToString("\n")
        )
        val forge = parts.filterIsInstance<TranscriptContentPart.ForgeUi>().first()

        val metadata = buildTranscriptForgeWindowMetadata(forge.payload, forge.dataStore)
        val root = metadata.view?.content?.containers?.single()
        val table = root?.containers?.single()

        assertEquals("Recommended sites", root?.title)
        assertEquals("site-review", table?.id)
        assertEquals("recommended_sites", table?.dataSourceRef)
        assertEquals(listOf("selected", "site_id", "site_name", "reason"), table?.table?.columns?.map { it.id })
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts dashboard summary metric strings`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-data",
                """{"version":1,"id":"summary_metrics","format":"json","mode":"replace","data":[{"spend":1316.86,"pacing_ratio":0.17,"win_rate":4.02}]}""",
                "```",
                "```forge-ui",
                """{"version":1,"title":"Ad order","subtitle":"Agency","blocks":[{"id":"summary","kind":"dashboard.summary","dataSourceRef":"summary_metrics","metrics":["spend","pacing_ratio","win_rate"]}]}""",
                "```"
            ).joinToString("\n")
        )
        val forge = parts.filterIsInstance<TranscriptContentPart.ForgeUi>().first()

        val metadata = buildTranscriptForgeWindowMetadata(forge.payload, forge.dataStore)
        val summary = metadata.view?.content?.containers?.single()?.containers?.single()

        assertEquals("dashboard.summary", summary?.kind)
        assertEquals("summary_metrics", summary?.dataSourceRef)
        assertEquals(listOf("spend", "pacing_ratio", "win_rate"), summary?.metrics?.map { it.id })
        assertEquals(listOf("Spend", "Pacing Ratio", "Win Rate"), summary?.metrics?.map { it.label })
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts dashboard timeline into chart config`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-data",
                """{"version":1,"id":"pie_data","format":"json","mode":"replace","data":[{"channel":"Alpha","spend":1316.86},{"channel":"Beta","spend":842.10},{"channel":"Gamma","spend":402.40}]}""",
                "```",
                "```forge-ui",
                """{"version":1,"title":"Android chart verification","subtitle":"Agency 4257","blocks":[{"id":"share","kind":"dashboard.timeline","title":"Spend share","dataSourceRef":"pie_data","chartType":"pie","dateField":"channel","series":[{"key":"spend","label":"Spend"}]}]}""",
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
        assertEquals("spend", timeline?.chart?.series?.valueKey)
        assertEquals(listOf("spend"), timeline?.chart?.series?.values?.map { it.value })
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts dashboard table blocks`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-data",
                """{"version":1,"id":"summary_metrics","format":"json","mode":"replace","data":[{"ad_order_name":"CID-30432_DH_Retargeting","primary_blocker_family":"Supply restriction"}]}""",
                "```",
                "```forge-ui",
                """{"version":1,"title":"Android dashboard verification","subtitle":"Agency 4257","blocks":[{"id":"primary-evidence","kind":"dashboard.table","title":"Primary evidence","dataSourceRef":"summary_metrics","columns":[{"key":"ad_order_name","label":"Ad order","format":"currency","type":"link","link":{"href":"ad_order_url"}},{"key":"primary_blocker_family","label":"Primary blocker","format":"percentFraction"}]}]}""",
                "```"
            ).joinToString("\n")
        )
        val forge = parts.filterIsInstance<TranscriptContentPart.ForgeUi>().first()

        val metadata = buildTranscriptForgeWindowMetadata(forge.payload, forge.dataStore)
        val table = metadata.view?.content?.containers?.single()?.containers?.single()

        assertEquals("summary_metrics", table?.dataSourceRef)
        assertEquals(listOf("ad_order_name", "primary_blocker_family"), table?.table?.columns?.map { it.id })
        assertEquals(listOf("Ad order", "Primary blocker"), table?.table?.columns?.map { it.label })
        assertEquals(listOf("currency", "percentFraction"), table?.table?.columns?.map { it.format })
        assertEquals("link", table?.table?.columns?.firstOrNull()?.type)
        assertEquals("ad_order_url", table?.table?.columns?.firstOrNull()?.link?.href)
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts summary items report and kpi table blocks`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-data",
                """{"version":1,"id":"recent_delivery","format":"json","mode":"replace","data":[{"total_spend":6061.727,"flight_pacing_status":"behind"}]}""",
                "```",
                "```forge-ui",
                """{"version":1,"title":"Frequency cap recommendation review","subtitle":"Blocked before execution","blocks":[{"kind":"dashboard.summary","title":"Review summary","items":[{"label":"Ad order","value":"Houston (Galleria) - Display (2657754)"},{"label":"Submission status","value":"Blocked"}]},{"kind":"dashboard.report","title":"Why this recommendation is not yet safe","sections":[{"id":"interpretation","title":"Interpretation","body":["Block submission until current cap truth is confirmed."]}]},{"kind":"dashboard.kpiTable","title":"Recent delivery posture","dataSourceRef":"recent_delivery","columns":[{"key":"total_spend","label":"Spend"},{"key":"flight_pacing_status","label":"Flight pacing"}]}]}""",
                "```"
            ).joinToString("\n")
        )
        val forge = parts.filterIsInstance<TranscriptContentPart.ForgeUi>().first()

        val metadata = buildTranscriptForgeWindowMetadata(forge.payload, forge.dataStore)
        val containers = metadata.view?.content?.containers?.single()?.containers.orEmpty()

        assertEquals("dashboard.summary", containers.getOrNull(0)?.kind)
        assertEquals(listOf("Ad order", "Submission status"), containers.getOrNull(0)?.metrics?.map { it.label })
        assertEquals("dashboard.report", containers.getOrNull(1)?.kind)
        assertEquals("Interpretation", containers.getOrNull(1)?.sections?.firstOrNull()?.title)
        assertEquals(listOf("total_spend", "flight_pacing_status"), containers.getOrNull(2)?.table?.columns?.map { it.id })
    }

    @Test
    fun `buildTranscriptForgeWindowMetadata adapts dimensions and messages blocks`() {
        val parts = parseTranscriptContentParts(
            listOf(
                "```forge-ui",
                """{"version":1,"title":"Android dashboard verification","subtitle":"Agency 4257","blocks":[{"kind":"dashboard.dimensions","title":"Publisher concentration","dataSourceRef":"publisher_breakdown","dimension":{"key":"publisher_id","label":"Publisher"},"metric":{"key":"spend_share","label":"Spend share","format":"percentFraction"},"viewModes":["chart","table"],"limit":10},{"kind":"dashboard.messages","title":"Next action","items":[{"title":"Primary next step","body":"Validate supply restriction next.","severity":"warning"}]}]}""",
                "```"
            ).joinToString("\n")
        )
        val forge = parts.filterIsInstance<TranscriptContentPart.ForgeUi>().first()

        val metadata = buildTranscriptForgeWindowMetadata(forge.payload, forge.dataStore)
        val containers = metadata.view?.content?.containers?.single()?.containers.orEmpty()

        assertEquals("dashboard.dimensions", containers.getOrNull(0)?.kind)
        assertEquals("publisher_id", containers.getOrNull(0)?.dimension?.key)
        assertEquals("spend_share", containers.getOrNull(0)?.metric?.key)
        assertEquals(listOf("chart", "table"), containers.getOrNull(0)?.viewModes)
        assertEquals(10, containers.getOrNull(0)?.limit)

        assertEquals("dashboard.messages", containers.getOrNull(1)?.kind)
        assertEquals("Primary next step", containers.getOrNull(1)?.items?.firstOrNull()?.title)
        assertEquals("warning", containers.getOrNull(1)?.items?.firstOrNull()?.severity)
    }
}
