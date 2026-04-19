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
                """{"version":1,"title":"Android chart verification","subtitle":"Agency 4257","blocks":[{"id":"share","kind":"dashboard.timeline","title":"Spend share","dataSourceRef":"pie_data","chartType":"pie","dateField":"channel","series":["spend"]}]}""",
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
}
