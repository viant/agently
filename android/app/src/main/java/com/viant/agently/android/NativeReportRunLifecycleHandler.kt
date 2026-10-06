package com.viant.agently.android

import com.viant.agentlysdk.*
import com.viant.forgeandroid.runtime.*
import kotlinx.serialization.json.*

internal fun makeNativeReportRunLifecycleHandler(client: AgentlyClient, observe: ((String, String, String?) -> Unit)? = null): NativeReportLifecycleHandler = object : NativeReportLifecycleHandler {
    private val completedRuns = java.util.concurrent.ConcurrentHashMap.newKeySet<String>()
    override suspend fun begin(admission: NativeReportAdmission, uiRunRequestId: String, origin: String): NativeReportRunHandle = begin(admission, uiRunRequestId, origin, null)
    override suspend fun begin(admission: NativeReportAdmission, uiRunRequestId: String, origin: String, reportAdmissionRef: String?): NativeReportRunHandle {
        check(admission.conversationId.isNotBlank()) { "The report has no authorized conversation binding." }
        val prepared = admission.preparation
        val requestedParams = nativeReportRequestedParams(admission)
        check(!requestedParams.containsKey("_agentlyForecastCommand")) { "The report command namespace is server-owned." }
        val durableOrigin = when (origin) { "prompt", "ui.report.run" -> "prompt"; "manual" -> "manual"; else -> error("Unsupported native report origin.") }
        val result = client.beginReportRun(BeginReportRunInput(
            uiRunRequestId = uiRunRequestId, conversationId = admission.conversationId, origin = durableOrigin,
            builderRef = prepared.identity.builderRef, sourceKind = "dashboard.reportBuilder", sourceId = prepared.identity.builderRef,
            presetId = (prepared.state["selectedReportPresetId"] as? JsonPrimitive)?.content,
            requestedParams = requestedParams, effectiveParams = prepared.primaryRequest, reportAdmissionRef = reportAdmissionRef))
        try {
            check(result.run.status == "running" && result.run.conversationId == admission.conversationId && result.run.builderRef == prepared.identity.builderRef && result.run.effectiveParams == prepared.primaryRequest && nativeReportRequestedParamsMatch(result.run.requestedParams, requestedParams, uiRunRequestId, reportAdmissionRef)) {
                "The report service did not preserve the admitted report identity."
            }
            result.context?.let { previous -> check(previous.ownerId == result.run.ownerId && previous.conversationId == admission.conversationId) { "The previous report context belongs to another owner or conversation." } }
        } catch (error: Exception) {
            if(result.run.status=="running" && result.run.conversationId==admission.conversationId) runCatching { client.failReportRun(result.run.reportRunId, FailReportRunInput(result.run.revision, admission.conversationId, "invalid_admission", error.message.orEmpty())) }
            throw error
        }
        observe?.invoke("begin", result.run.reportRunId, null)
        return NativeReportRunHandle(result.run.reportRunId, result.run.revision, uiRunRequestId, admission, result.context?.revision ?: 0, result.run.ownerId, reportAdmissionRef)
    }

    override suspend fun complete(handle: NativeReportRunHandle, rows: JsonObject, current: () -> Boolean): NativeReportCompletedRun {
        check(current()) { "The report admission changed before compilation." }
        val invocation = nativeReportInvocation(handle.admission)
        observe?.invoke("compile", handle.reportRunId, null)
        val compilerArgs = linkedMapOf<String, JsonElement>(
            "reportId" to JsonPrimitive(handle.uiRunRequestId),
            "fences" to nativeReportFences(handle.uiRunRequestId, handle.admission.document, rows),
            "invocation" to invocation)
        handle.reportAdmissionRef?.let { compilerArgs["reportAdmissionRef"] = JsonPrimitive(it) }
        val raw = client.executeTool("reporting:compile_fenced_report", compilerArgs, handle.admission.conversationId)
        val compiled = Json.parseToJsonElement(raw) as? JsonObject ?: error("The report compiler returned an invalid result.")
        val spec = compiled["reportSpec"] as? JsonObject ?: error("The report compiler omitted reportSpec.")
        val fill = compiled["reportFill"] as? JsonObject ?: error("The report compiler omitted reportFill.")
        val print = compiled["reportPrint"] as? JsonObject ?: error("The report compiler omitted reportPrint.")
        validateNativeReportCompilerIdentity(spec, invocation)
        validateNativeReportFilledDatasets(fill,invocation)
        check(current()) { "The report admission changed during compilation." }
        val run = try {
            client.completeReportRun(handle.reportRunId, CompleteReportRunInput(handle.revision, spec, fill, print, handle.admission.conversationId))
        } catch (error: Exception) {
            // A lost response may follow a successful write. Read the exact run; never repeat the write.
            val saved = runCatching { client.getReportRun(handle.reportRunId, handle.admission.conversationId) }.getOrNull()
            if (saved?.status == "completed" && saved.reportRunId == handle.reportRunId && saved.conversationId == handle.admission.conversationId &&
                (handle.ownerId == null || saved.ownerId == handle.ownerId) && saved.reportSpec == spec && saved.reportFill == fill && saved.reportPrint == print) saved else throw error
        }
        check(run.status == "completed" && nativeReportRequestedParamsMatch(run.requestedParams, nativeReportRequestedParams(handle.admission), handle.uiRunRequestId, handle.reportAdmissionRef) && run.reportRunId == handle.reportRunId && run.conversationId == handle.admission.conversationId && (handle.ownerId == null || run.ownerId == handle.ownerId) && run.reportSpec == spec && run.reportFill == fill && run.reportPrint == print) { "The report service did not preserve the completed report artifact." }
        completedRuns.add(run.reportRunId)
        val verifiedDatasets=(run.reportFill as JsonObject).getValue("datasets").jsonArray
        fun completedOutcome(status:String="active",active:Boolean?=true,error:String?=null)=NativeReportCompletedRun(run.reportRunId,run.revision,status,active,error,verifiedDatasets)
        observe?.invoke("complete", run.reportRunId, null)
        if (!current()) return completedOutcome("unconfirmed",null,"The report was saved but its preparation changed before activation.")
        return try {
            val context = client.activateReportRun(run.reportRunId, ActivateReportRunInput(handle.admission.conversationId, run.revision, handle.contextRevision, "prompt"))
            check(context.ownerId == run.ownerId && context.conversationId == handle.admission.conversationId && context.activeReportRunId == run.reportRunId) { "The report activation returned a different owner, conversation, or run." }
            observe?.invoke("activate", run.reportRunId, "active")
            completedOutcome()
        } catch (error: Exception) {
            val context = runCatching { client.getReportContext(handle.admission.conversationId) }.getOrNull()
            val outcome = when {
                context != null && context.ownerId == run.ownerId && context.conversationId == handle.admission.conversationId && context.activeReportRunId == run.reportRunId -> completedOutcome()
                context != null && context.ownerId == run.ownerId && context.conversationId == handle.admission.conversationId && context.revision > handle.contextRevision -> completedOutcome("superseded",false,"The report was saved; a newer conversation report context remains active.")
                else -> completedOutcome("unconfirmed",null,error.message?.takeIf(String::isNotBlank) ?: "The saved report activation could not be confirmed.")
            }
            observe?.invoke("activation-reconciled", run.reportRunId, outcome.contextStatus)
            outcome
        }
    }

    override suspend fun fail(handle: NativeReportRunHandle, code: String, text: String) {
        if (completedRuns.contains(handle.reportRunId)) return
        val current = client.getReportRun(handle.reportRunId, handle.admission.conversationId)
        check(current.conversationId == handle.admission.conversationId && (handle.ownerId == null || current.ownerId == handle.ownerId)) { "The report failure target belongs to a different owner or conversation." }
        if (current.status == "completed") { completedRuns.add(current.reportRunId); return }
        client.failReportRun(handle.reportRunId, FailReportRunInput(handle.revision, handle.admission.conversationId, code, text))
    }
}

internal fun nativeReportInvocation(admission: NativeReportAdmission): JsonObject = buildJsonObject {
    put("source", buildJsonObject {
        put("kind", "dashboard.reportBuilder"); put("containerId", admission.preparation.identity.builderRef)
        put("stateKey", admission.stateKey); put("dataSourceRef", admission.preparation.dataSourceRef)
    })
    put("parameters", nativeReportBuilderParameters(admission.preparation.state, admission.authoredConfiguration))
    put("datasets", JsonArray(admission.datasets.map { dataset -> buildJsonObject {
        put("id", dataset.id); put("dataSourceRef", dataset.dataSourceRef); put("request", dataset.request)
    } }))
}

internal fun nativeReportFences(reportId: String, document: JsonObject, rows: JsonObject): JsonArray {
    val fences = mutableListOf<JsonElement>()
    fences += buildJsonObject { put("kind", "forge-report"); put("payload", JsonObject(document.toMutableMap().apply {
        put("version", JsonPrimitive(1)); put("scope", JsonPrimitive("message")); put("id", JsonPrimitive(reportId))
        put("sequence", JsonPrimitive(1)); put("mode", JsonPrimitive("start")); put("grammar", JsonPrimitive("report-document-v1"))
    })) }
    rows.entries.forEachIndexed { index, (id, data) -> fences += buildJsonObject {
        put("kind", "forge-data"); put("payload", buildJsonObject {
            put("version", 2); put("scope", "message"); put("id", id); put("reportRef", reportId)
            put("sequence", index + 2); put("format", "json"); put("mode", "replace"); put("data", data)
        })
    } }
    fences += buildJsonObject { put("kind", "forge-report"); put("payload", buildJsonObject {
        put("version", 1); put("scope", "message"); put("id", reportId); put("sequence", fences.size + 1); put("mode", "commit")
    }) }
    return JsonArray(fences)
}

internal fun validateNativeReportCompilerIdentity(spec: JsonObject, invocation: JsonObject) {
    check(spec["parameters"] == invocation["parameters"]) { "The report compiler does not preserve the admitted builder parameters." }
    check(spec["source"] == invocation["source"]) { "The report compiler does not preserve the admitted report identity." }
    val expected = (invocation["datasets"] as? JsonArray).orEmpty().associate { item -> val value = item.jsonObject; value.getValue("id").jsonPrimitive.content to value }
    val actualList = (spec["datasets"] as? JsonArray).orEmpty().map { it.jsonObject }
    val actual = actualList.associateBy { it.getValue("id").jsonPrimitive.content }
    check(actual.size == actualList.size && actual.keys == expected.keys && actual.all { (id, value) ->
        value["dataSourceRef"] == expected.getValue(id)["dataSourceRef"] && value["request"] == expected.getValue(id)["request"]
    }) { "The report compiler does not preserve the admitted dataset requests." }
}

internal fun validateNativeReportFilledDatasets(fill:JsonObject,invocation:JsonObject):JsonArray {
    check(fill["source"]==invocation["source"] && fill["parameters"]==invocation["parameters"]) { "The report compiler fill does not preserve the admitted identity and parameters." }
    val expected=invocation.getValue("datasets").jsonArray.associate { it.jsonObject.getValue("id").jsonPrimitive.content to it.jsonObject }
    val datasets=fill["datasets"] as? JsonArray ?: error("The report compiler fill omitted datasets.")
    val actual=datasets.map { it as? JsonObject ?: error("The report compiler fill returned an invalid dataset.") }
    check(actual.size==expected.size && actual.map { it["id"] }.toSet().size==actual.size && actual.all { dataset ->
        val source=expected[(dataset["id"] as? JsonPrimitive)?.content]
        source!=null && dataset["dataSourceRef"]==source["dataSourceRef"] && dataset["request"]==source["request"] && dataset["rows"] is JsonArray
    }) { "The report compiler fill does not preserve the admitted datasets and rows." }
    return datasets
}
