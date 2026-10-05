package com.viant.agently.android

import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Test

class NativeReportAuthoringSummaryTest {
    @Test fun actualStewardCatalogMatchesWebSnapshotOracleWithoutChangingRawMetadata() {
        val fixture=Json.parseToJsonElement(javaClass.getResource("/report-builder-authoring-summary.json")!!.readText()).jsonObject
        val metadata=fixture.getValue("metadata").jsonObject;val form=fixture.getValue("form").jsonObject
        val summary=nativeReportAuthoringSummary(metadata,form)!!
        assertEquals(fixture.getValue("expected"),summary)
        assertEquals(17,summary.getValue("dataSources").jsonArray.size)
        val augmented=nativeMetadataWithReportAuthoringSummary(metadata,form)
        assertEquals(metadata,JsonObject(augmented.filterKeys { it!="reportBuilder" }))
        assertEquals(summary,augmented["reportBuilder"])
        assertNull(summary["rows"]);assertNull(summary["ready"]);assertNull(summary["canRun"])
    }
    @Test fun missingSelectedBuilderNeverInventsAuthoringCatalog() {
        val fixture=Json.parseToJsonElement(javaClass.getResource("/report-builder-authoring-summary.json")!!.readText()).jsonObject
        assertNull(nativeReportAuthoringSummary(fixture.getValue("metadata").jsonObject,buildJsonObject { put("reportBuilderRef","missing") }))
    }
}
