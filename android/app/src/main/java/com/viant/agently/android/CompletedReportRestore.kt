package com.viant.agently.android

import com.viant.agentlysdk.*
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.JsonUtil
import kotlinx.coroutines.CancellationException
import kotlinx.serialization.json.*

internal fun restoredCompletedReportForm(form: JsonObject?, run: ReportRun, conversationId: String, metadata: JsonElement): JsonObject? {
    form ?: return null
    if (run.status != "completed" || run.conversationId != conversationId) return null
    val builder = (form["reportBuilderRef"] as? JsonPrimitive)?.content ?: return null
    if (builder != run.builderRef) return null
    if ((run.requestedParams as? JsonObject)?.containsKey(com.viant.forgeandroid.runtime.NATIVE_REPORT_ADMISSION_KEY) == true) {
        return restoreNativeReportContext(form,run,conversationId,metadata)?.form
    }
    val definition = form["reportDefinition"] as? JsonObject ?: return null
    val document = (definition["documentPatch"] ?: definition["reportDocument"]) as? JsonObject ?: return null
    val spec = run.reportSpec as? JsonObject ?: return null
    val fill = run.reportFill as? JsonObject ?: return null
    if (spec["source"] == null || spec["source"] != fill["source"]) return null
    val filters = (run.effectiveParams as? JsonObject)?.get("filters") as? JsonObject ?: return null
    val scope = linkedMapOf<String, JsonElement>()
    (form["prefill"] as? JsonObject)?.forEach { (key, value) -> scope[when(key) { "from" -> "From"; "to" -> "To"; else -> key }] = value }
    val state = form["reportBuilder:$builder"] as? JsonObject
    (state?.get("scopeParams") as? JsonObject)?.forEach { (key, value) ->
        if (key == "dateRange" && value is JsonObject) { value["start"]?.let { scope["From"] = it }; value["end"]?.let { scope["To"] = it } }
        else scope[key] = value
    }
    (state?.get("dynamicGroups") as? JsonObject)?.values?.forEach { values ->
        (values as? JsonArray)?.forEach { raw ->
            val group = raw as? JsonObject ?: return@forEach
            if (group["enabled"] == JsonPrimitive(false)) return@forEach
            val key = (group["filterId"] as? JsonPrimitive)?.content ?: return@forEach
            scope[key] = JsonArray((group["selections"] as? JsonArray).orEmpty().mapNotNull { (it as? JsonObject)?.get("value") })
        }
    }
    fun empty(value: JsonElement) = value == JsonNull || value == JsonPrimitive("") || value == JsonArray(emptyList())
    if (filters.any { !empty(it.value) && scope[it.key] != it.value } || scope.any { !empty(it.value) && filters[it.key] != it.value }) return null
    val declarations = linkedMapOf<String, JsonObject>()
    fun collect(value: JsonElement) {
        when (value) {
            is JsonObject -> {
                val variants = value["reportBuilders"] as? JsonObject
                val selected = (variants?.get(builder) as? JsonObject)?.get("reportBuilder")
                if (selected != null) { collect(selected); return }
                (value["dataSources"] as? JsonArray)?.forEach { raw ->
                    val source = raw as? JsonObject ?: return@forEach
                    (source["id"] as? JsonPrimitive)?.content?.let { declarations[it] = source }
                }
                value.filterKeys { it != "reportBuilders" }.values.forEach(::collect)
            }
            is JsonArray -> value.forEach(::collect)
            else -> Unit
        }
    }
    collect(metadata)
    val refs = linkedSetOf<String>()
    fun referenced(value: JsonElement) { when(value) {
        is JsonObject -> { (value["datasetRef"] as? JsonPrimitive)?.content?.let(refs::add); value.values.forEach(::referenced) }
        is JsonArray -> value.forEach(::referenced)
        else -> Unit
    } }
    referenced(document)
    if (refs.isEmpty() || "primary" in refs) return null
    val nativeDatasets = (spec["datasets"] as? JsonArray).orEmpty().filterIsInstance<JsonObject>()
    val filledDatasets = (fill["datasets"] as? JsonArray).orEmpty().filterIsInstance<JsonObject>()
    val restored = refs.sorted().map { id ->
        val declared = declarations[id] ?: return null
        val request = declared["request"] as? JsonObject ?: return null
        val native = nativeDatasets.firstOrNull { it["id"] == JsonPrimitive(id) } ?: return null
        val filled = filledDatasets.firstOrNull { it["id"] == JsonPrimitive(id) } ?: return null
        if (native["dataSourceRef"] != declared["dataSourceRef"] || filled["dataSourceRef"] != native["dataSourceRef"] || native["request"] != filled["request"] || filled["rows"] !is JsonArray) return null
        val effective = request.toMutableMap()
        if ((declared["scope"] as? JsonObject)?.get("mode") == JsonPrimitive("inherit")) {
            effective["filters"] = JsonObject(filters + (request["filters"] as? JsonObject).orEmpty())
        }
        if (JsonObject(effective) != native["request"]) return null
        com.viant.forgeandroid.runtime.nativeReportImmutableJson(filled)
    }
    return JsonObject(form.toMutableMap().apply {
        remove("reportRunRequest"); put("executeOnOpen", JsonPrimitive(false)); put("reportStaticDatasets", JsonArray(restored))
        put("reportMaterialization", buildJsonObject { put("id", run.reportRunId); put("requestId", run.reportRunId);put("reportRunId",run.reportRunId); put("status", "completed"); put("materialized", true) })
    })
}

internal suspend fun hydrateCompletedReportWindow(client: AgentlyClient?, runtime: ForgeRuntime, window: WorkspaceWindowSnapshot): WorkspaceWindowSnapshot {
    val form = window.windowForm ?: return window
    if (client == null || form["reportBuilderRef"] == null) return window
    val conversationId = window.conversationId?.takeIf(String::isNotBlank) ?: return window
    val account = runtime.frozenNativeReports.accountBinding()
    return try {
        val context = client.getReportContext(conversationId)
        if (context.conversationId != conversationId || context.activeReportRunId.isBlank()) return window
        val run = client.getReportRun(context.activeReportRunId, conversationId)
        if (run.reportRunId != context.activeReportRunId || run.ownerId != context.ownerId) return window
        val metadata = makeForgeAgentlyWindowMetadataLoader(client, runtime.targetContext)(ForgeRuntime.WindowMetadataRequest(window.windowId, window.windowKey,
            window.parameters?.mapValues { JsonUtil.elementToAny(it.value) }.orEmpty(), conversationId)) ?: return window
        val raw = JsonUtil.json.encodeToJsonElement(com.viant.forgeandroid.runtime.WindowMetadata.serializer(), metadata)
        val hasAdmission = (run.requestedParams as? JsonObject)?.containsKey(com.viant.forgeandroid.runtime.NATIVE_REPORT_ADMISSION_KEY) == true
        val restoration = if (hasAdmission) restoreNativeReportContext(form,run,conversationId,raw,window.windowId) { reason ->
            if(BuildConfig.DEBUG) {
                val ns=(run.requestedParams as? JsonObject)?.get(com.viant.forgeandroid.runtime.NATIVE_REPORT_ADMISSION_KEY) as? JsonObject
                val key=(ns?.get("stateKey") as? JsonPrimitive)?.content
                val builder=(form["reportBuilderRef"] as? JsonPrimitive)?.content.orEmpty()
                fun digest(value:JsonElement?)=value?.let { runCatching { com.viant.forgeandroid.runtime.nativeReportAdmissionDigest(it) }.getOrNull() }
                val hashes=mapOf("authorState" to digest(key?.let(form::get)),"configuration" to digest(selectedNativeReportConfiguration(raw,builder)?.first),"document" to digest(com.viant.forgeandroid.runtime.nativeReportSelectedDocument(form,key)),"prefill" to digest(com.viant.forgeandroid.runtime.nativeReportPrefillIdentity(form)))
                android.util.Log.d("CompletedReportRestore","Rejected ${run.reportRunId} reason=$reason expected=${ns?.get("digests")} actual=$hashes")
            }
        } else null
        val hydrated = if (hasAdmission) restoration?.form ?: return window else restoredCompletedReportForm(form, run, conversationId, raw) ?: return window
        if (restoration != null && (account == null || !runtime.frozenNativeReports.install(window.windowId,conversationId,hydrated,restoration.admission,run.reportRunId,run.ownerId,account))) return window
        if(!hasAdmission) {
            if(account==null || !runtime.completedDatasetProofs.install(window.windowId,conversationId,run.reportRunId,run.ownerId,hydrated.getValue("reportStaticDatasets").jsonArray,account) { current,currentMetadata ->
                currentMetadata!=null && restoredCompletedReportForm(current,run,conversationId,JsonUtil.json.encodeToJsonElement(com.viant.forgeandroid.runtime.WindowMetadata.serializer(),currentMetadata))!=null
            }) return window
        }
        if (BuildConfig.DEBUG) android.util.Log.d("CompletedReportRestore", "Restored ${run.reportRunId} for $conversationId")
        window.copy(windowForm = hydrated)
    } catch (cancelled: CancellationException) { throw cancelled }
    catch (_: Exception) { window }
}
