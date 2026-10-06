package com.viant.agently.android

import com.viant.agentlysdk.ReportRun
import com.viant.forgeandroid.runtime.*
import kotlinx.serialization.json.*

internal data class NativeReportContextRestoration(val form:JsonObject, val admission:NativeReportAdmission)

internal fun selectedNativeReportConfiguration(metadata:JsonElement,builderRef:String):Pair<JsonObject,String>? = nativeReportMetadataConfiguration(metadata,builderRef)

/** Namespace presence is authoritative: an invalid V1 must never fall back to legacy scope guessing. */
internal fun restoreNativeReportContext(form:JsonObject,run:ReportRun,conversationId:String,metadata:JsonElement,windowId:String="restored",rejected:((String)->Unit)?=null):NativeReportContextRestoration? = runCatching {
    require(run.status=="completed" && run.conversationId==conversationId) { "run-identity-mismatch" }
    val requested=run.requestedParams as? JsonObject ?: error("missing-requested-params")
    val namespace=requested[NATIVE_REPORT_ADMISSION_KEY] as? JsonObject ?: error("missing-admission-context")
    val effective=run.effectiveParams as? JsonObject ?: error("missing-effective-params")
    require(JsonObject(requested.filterKeys { it!=NATIVE_REPORT_ADMISSION_KEY })==effective) { "primary-envelope-mismatch" }
    val builder=(form["reportBuilderRef"] as? JsonPrimitive)?.content ?: error("missing-selected-builder")
    require(builder==run.builderRef) { "builder-mismatch" }
    val stateKey=namespace.getValue("stateKey").jsonPrimitive.content
    val rawState=form[stateKey] as? JsonObject ?: error("missing-selected-author-state")
    val document=nativeReportSelectedDocument(form,stateKey) ?: error("missing-selected-document")
    val (configuration,primaryRef)=selectedNativeReportConfiguration(metadata,builder) ?: error("missing-selected-configuration")
    val sources=(configuration["dataSources"] as? JsonArray).orEmpty().map { JsonUtil.json.decodeFromJsonElement<ReportBuilderPublishedDataSourceDef>(it) }
    val admission=restoredNativeReportAdmission(namespace,effective,rawState,document,configuration,conversationId,windowId,builder,stateKey,primaryRef,sources,nativeReportPrefillIdentity(form))
    val spec=run.reportSpec as? JsonObject ?: error("missing-spec")
    val fill=run.reportFill as? JsonObject ?: error("missing-fill")
    val expectedSource=buildJsonObject { put("kind","dashboard.reportBuilder");put("containerId",builder);put("stateKey",stateKey);put("dataSourceRef",primaryRef) }
    require(spec["source"]==expectedSource && fill["source"]==expectedSource) { "stored-source-mismatch" }
    val parameters=nativeReportBuilderParameters(admission.preparation.state,configuration)
    require(spec["parameters"]==parameters && fill["parameters"]==parameters) { "stored-parameters-mismatch" }
    fun datasetMap(value:JsonElement?):Map<String,JsonObject> {
        val list=value as? JsonArray ?: error("missing-datasets")
        val objects=list.map { it as? JsonObject ?: error("invalid-dataset") }
        val map=objects.associateBy { it.getValue("id").jsonPrimitive.content }
        require(map.size==objects.size && map.keys==admission.datasets.map { it.id }.toSet()) { "stored-dataset-identity-mismatch" }
        return map
    }
    val specs=datasetMap(spec["datasets"]);val filled=datasetMap(fill["datasets"])
    val restored=admission.datasets.map { dataset ->
        val declared=specs.getValue(dataset.id);val actual=filled.getValue(dataset.id)
        require(declared["dataSourceRef"]==JsonPrimitive(dataset.dataSourceRef) && actual["dataSourceRef"]==JsonPrimitive(dataset.dataSourceRef)) { "stored-dataset-ref-mismatch:${dataset.id}" }
        require(declared["request"]==dataset.request && actual["request"]==dataset.request && actual["rows"] is JsonArray) { "stored-dataset-request-mismatch:${dataset.id}" }
        nativeReportImmutableJson(actual)
    }
    val hydrated=JsonObject(form.toMutableMap().apply {
        remove("reportRunRequest");put("executeOnOpen",JsonPrimitive(false));put("reportStaticDatasets",JsonArray(restored))
        put("reportMaterialization",buildJsonObject { put("id",run.reportRunId);put("requestId",run.reportRunId);put("reportRunId",run.reportRunId);put("status","completed");put("materialized",true);put("contextStatus","active");put("active",true) })
    })
    NativeReportContextRestoration(hydrated,admission)
}.onFailure { rejected?.invoke(it.message.orEmpty().take(160)) }.getOrNull()
