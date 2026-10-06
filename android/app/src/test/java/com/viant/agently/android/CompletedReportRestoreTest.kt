package com.viant.agently.android

import com.viant.agentlysdk.ReportRun
import kotlinx.serialization.json.*
import org.junit.Test
import org.junit.Assert.*

class CompletedReportRestoreTest {
    private val json = Json { ignoreUnknownKeys = true }
    private val fixture = json.parseToJsonElement(javaClass.getResource("/completed-report-restore.json")!!.readText()).jsonObject
    private val run = json.decodeFromJsonElement<ReportRun>(fixture.getValue("run"))
    private val form = fixture.getValue("form").jsonObject
    @Test fun restoresOnlyMatchingCompletedDatasetsAndKeepsAuthoredLayout() {
        val result = restoredCompletedReportForm(form, run, run.conversationId!!, fixture.getValue("metadata"))!!
        assertEquals(form["reportDefinition"], result["reportDefinition"])
        assertEquals(4, result["reportStaticDatasets"]!!.jsonArray.size)
        assertEquals(JsonPrimitive(false), result["executeOnOpen"])
        assertNull(result["reportRunRequest"])
    }
    @Test fun rejectsWrongConversationAndChangedScope() {
        assertNull(restoredCompletedReportForm(form, run, "different", fixture.getValue("metadata")))
        val changed = JsonObject(form.toMutableMap().apply { put("prefill", buildJsonObject { put("from", "2026-09-27"); put("to", "2026-10-04"); put("orderIds", JsonArray(listOf(JsonPrimitive(1)))) }); remove("reportBuilder:metricsCubeBuilder") })
        assertNull(restoredCompletedReportForm(changed, run, run.conversationId!!, fixture.getValue("metadata")))
    }
    @Test fun rejectsDatasetRequestMismatch() {
        val fill = run.reportFill!!.jsonObject
        val datasets = fill["datasets"]!!.jsonArray.toMutableList()
        datasets[0] = JsonObject(datasets[0].jsonObject.toMutableMap().apply { put("request", buildJsonObject { put("limit", 99) }) })
        val wrong = run.copy(reportFill = JsonObject(fill.toMutableMap().apply { put("datasets", JsonArray(datasets)) }))
        assertNull(restoredCompletedReportForm(form, wrong, run.conversationId!!, fixture.getValue("metadata")))
    }
}
