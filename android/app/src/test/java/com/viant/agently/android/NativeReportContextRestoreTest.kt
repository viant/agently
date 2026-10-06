package com.viant.agently.android

import com.viant.agentlysdk.ReportRun
import com.viant.forgeandroid.runtime.*
import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Test
import java.time.Instant

class NativeReportContextRestoreTest {
    @Test fun actualIOSCompletedF5RecordRestoresAllHashesRowsAndParameters() {
        val fixture=JsonUtil.json.parseToJsonElement(javaClass.getResource("/actual-ios-f5-native-report.json")!!.readText()).jsonObject
        val run=JsonUtil.json.decodeFromJsonElement<ReportRun>(fixture.getValue("run"))
        val restored=restoreNativeReportContext(fixture.getValue("form").jsonObject,run,run.conversationId!!,fixture.getValue("metadata"))!!
        assertEquals(25,run.reportSpec!!.jsonObject.getValue("blocks").jsonArray.size)
        assertEquals(run.reportFill!!.jsonObject.getValue("datasets"),restored.form["reportStaticDatasets"])
        assertEquals(run.reportSpec!!.jsonObject.getValue("parameters"),nativeReportBuilderParameters(restored.admission.preparation.state,restored.admission.authoredConfiguration))
        assertEquals(JsonPrimitive(""),run.reportSpec!!.jsonObject.getValue("parameters").jsonObject["groupBy"])
    }
    private data class Fixture(val form:JsonObject,val run:ReportRun,val metadata:JsonObject,val admission:NativeReportAdmission)
    private fun fixture():Fixture {
        val primary=buildJsonObject { put("filters",buildJsonObject { put("From","2026-09-27");put("To","2026-10-04");put("orderIds",JsonArray(listOf(JsonPrimitive(2659534)))) });put("limit",50);put("offset",0) }
        val catalog=buildJsonObject { put("filters",JsonObject(emptyMap()));put("limit",1);put("offset",0) }
        val sources=listOf(
            ReportBuilderPublishedDataSourceDef("inherit","cube",catalog),
            ReportBuilderPublishedDataSourceDef("append","cube",catalog,buildJsonObject { put("mode","append");put("local",buildJsonObject { put("filters",buildJsonObject { put("channelIds",JsonArray(listOf(JsonPrimitive(7)))) }) }) }),
            ReportBuilderPublishedDataSourceDef("relative","cube",catalog,buildJsonObject { put("mode","override");put("relativeDateRange",buildJsonObject { put("preset","last7days");put("startParamPath","filters.From");put("endParamPath","filters.To") }) }),
            ReportBuilderPublishedDataSourceDef("exclude","cube",catalog,buildJsonObject { put("mode","exclude");put("exclude",JsonArray(listOf(JsonPrimitive("orderIds")))) },listOf(buildJsonObject { put("id","orderIds");put("paramPath","filters.orderIds") })))
        val policyState=buildJsonObject { put("viewMode","table");put("pageSize",50);put("orderDir","desc");put("scopeParams",JsonObject(emptyMap()));put("opaque",buildJsonObject { put("unknown",JsonArray(listOf(JsonPrimitive("😀")))) }) }
        val packet=PreparedReportRequest(ReportPreparationIdentity("live","builder","form","state"),"ready","cube",primary,policyState,publishedSources=sources,
            capturedTimeMillis=Instant.parse("2026-10-01T01:02:03.456Z").toEpochMilli(),capturedZoneId="America/Los_Angeles")
        val ids=listOf("primary")+sources.map { it.id }
        val blocks=JsonArray(ids.map { buildJsonObject { put("id",it);put("kind","tableBlock");put("datasetRef",it) } })
        val document=buildJsonObject { put("title","Original report");put("opaque",true);put("blocks",blocks) }
        val rawState=JsonObject(policyState+mapOf("reportDocumentBlocks" to blocks,"dynamicFilterDrafts" to buildJsonObject { put("draft","preserved") }))
        val config=buildJsonObject { put("opaque",buildJsonObject { put("authorOnly",true) });put("dataSources",JsonArray(sources.map { JsonUtil.json.encodeToJsonElement(ReportBuilderPublishedDataSourceDef.serializer(),it) })) }
        val datasets=listOf(NativeReportDatasetAdmission("primary","cube",primary))+sources.map { NativeReportDatasetAdmission(it.id,it.dataSourceRef,preparePublishedReportRequest(packet.identity,packet,it).request!!) }
        val admission=NativeReportAdmission(packet,"conversation","builder-state",document,datasets,config,rawState)
        val source=buildJsonObject { put("kind","dashboard.reportBuilder");put("containerId","builder");put("stateKey","builder-state");put("dataSourceRef","cube") }
        val parameters=nativeReportBuilderParameters(policyState,config)
        val specDatasets=JsonArray(datasets.map { buildJsonObject { put("id",it.id);put("dataSourceRef",it.dataSourceRef);put("request",it.request) } })
        val spec=buildJsonObject { put("source",source);put("parameters",parameters);put("datasets",specDatasets) }
        val fill=buildJsonObject { put("source",source);put("parameters",parameters);put("datasets",JsonArray(specDatasets.map { JsonObject(it.jsonObject+mapOf("rows" to JsonArray(emptyList()))) })) }
        val run=JsonUtil.json.decodeFromJsonElement<ReportRun>(buildJsonObject { put("reportRunId","saved-run");put("revision",2);put("ownerId","owner");put("conversationId","conversation");put("builderRef","builder");put("status","completed");put("requestedParams",nativeReportRequestedParams(admission));put("effectiveParams",primary);put("reportSpec",spec);put("reportFill",fill) })
        val form=buildJsonObject { put("reportBuilderRef","builder");put("builder-state",rawState);put("other-state",buildJsonObject { put("reportDocumentBlocks",JsonArray(listOf(buildJsonObject { put("datasetRef","wrong") }))) });put("reportDefinition",buildJsonObject { put("documentPatch",document) });put("reportRunRequest",buildJsonObject { put("id","old-ui-id") }) }
        val metadata=buildJsonObject { put("reportBuilders",buildJsonObject { put("builder",buildJsonObject { put("dataSourceRef","cube");put("reportBuilder",config) }) }) }
        return Fixture(form,run,metadata,admission)
    }
    @Test fun genericRestorationSupportsPrimaryAllPoliciesAndFrozenEmptyDatasets() {
        val f=fixture();val restored=restoreNativeReportContext(f.form,f.run,"conversation",f.metadata)!!
        assertEquals(f.admission.datasets,restored.admission.datasets)
        assertEquals(f.admission.authorState,restored.admission.authorState)
        assertEquals(f.admission.preparation.state,restored.admission.preparation.state)
        assertNotEquals(restored.admission.authorState,restored.admission.preparation.state)
        assertEquals(f.admission.preparation.capturedTimeMillis,restored.admission.preparation.capturedTimeMillis)
        assertEquals(JsonPrimitive(false),restored.form["executeOnOpen"]);assertNull(restored.form["reportRunRequest"])
        assertEquals(5,restored.form.getValue("reportStaticDatasets").jsonArray.size)
        assertEquals(f.form["reportDefinition"],restored.form["reportDefinition"])
        val relative=restored.admission.datasets.single { it.id=="relative" }.request.getValue("filters").jsonObject
        assertEquals(JsonPrimitive("2026-09-30"),relative["To"])
    }
    @Test fun namespaceNeverFallsBackWhenRawIntentConfigurationOrPersistedRequestsChange() {
        val f=fixture()
        val changedState=JsonObject(f.admission.authorState+mapOf("dynamicFilterDrafts" to JsonObject(emptyMap())))
        assertNull(restoredCompletedReportForm(JsonObject(f.form+mapOf("builder-state" to changedState)),f.run,"conversation",f.metadata))
        val requested=f.run.requestedParams!!.jsonObject;val ns=requested.getValue(NATIVE_REPORT_ADMISSION_KEY).jsonObject
        val tampered=JsonObject(requested+mapOf(NATIVE_REPORT_ADMISSION_KEY to JsonObject(ns+mapOf("state" to JsonObject(emptyMap())))))
        assertNull(restoredCompletedReportForm(f.form,f.run.copy(requestedParams=tampered),"conversation",f.metadata))
        val filled=f.run.reportFill!!.jsonObject;val rows=filled.getValue("datasets").jsonArray.toMutableList()
        rows[0]=JsonObject(rows[0].jsonObject+mapOf("request" to JsonObject(emptyMap())))
        assertNull(restoredCompletedReportForm(f.form,f.run.copy(reportFill=JsonObject(filled+mapOf("datasets" to JsonArray(rows)))),"conversation",f.metadata))
        assertNull(restoreNativeReportContext(f.form,f.run,"different",f.metadata))
        assertNull(restoreNativeReportContext(JsonObject(f.form+mapOf("prefill" to buildJsonObject { put("orderIds",JsonArray(listOf(JsonPrimitive(2703801)))) })),f.run,"conversation",f.metadata))
    }
}
