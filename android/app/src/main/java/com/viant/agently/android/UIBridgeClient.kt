package com.viant.agently.android

import android.content.Context
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.UIBridgeRpcClient
import com.viant.forgeandroid.runtime.JsonUtil
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.FeedPatchOperation
import com.viant.forgeandroid.runtime.WindowMetadata
import com.viant.forgeandroid.runtime.WindowState
import com.viant.forgeandroid.runtime.applyFeedPatchOperations
import com.viant.forgeandroid.runtime.snapshotFeedDataSources
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.put
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.longOrNull
import java.util.UUID

private const val UI_BRIDGE_PREFS = "agently.ui_bridge"
private const val UI_BRIDGE_CLIENT_ID_KEY = "client_id"
private const val UI_BRIDGE_SESSION_HEADER = "Mcp-Session-Id"

@Serializable
internal data class NativeUIBridgeWindow(
    val windowId: String,
    val windowKey: String,
    val windowTitle: String,
    val conversationId: String? = null,
    val presentation: String? = null,
    val region: String? = null,
    val parentKey: String? = null,
    val workspaceSharePct: Int? = null,
    val workspaceMinHeight: Int? = null,
    val parameters: JsonObject = JsonObject(emptyMap()),
    val windowForm: JsonObject = JsonObject(emptyMap()),
    val metadata: JsonObject = JsonObject(emptyMap()),
    val dataSources: JsonObject = JsonObject(emptyMap()),
    val inTab: Boolean = true,
    val isModal: Boolean = false
)

@Serializable
internal data class NativeUIBridgeSnapshot(
    val conversationId: String? = null,
    val windows: List<NativeUIBridgeWindow> = emptyList()
)

internal class AndroidUIBridgeClient(
    context: Context,
    private val client: AgentlyClient,
    private val scope: CoroutineScope,
    private val snapshotProvider: suspend () -> NativeUIBridgeSnapshot,
    private val commandHandler: suspend (String, JsonObject) -> JsonObject
) {
    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = false }
    private val clientId = loadOrCreateUIBridgeClientId(context)
    private val rpcClient = UIBridgeRpcClient(client)

    @Volatile
    private var selectedWindowId: String? = null

    @Volatile
    private var started = false

    private var pollJob: Job? = null
    private var snapshotJob: Job? = null
    private var lastSnapshotFingerprint: String = ""

    fun clientId(): String = clientId

    suspend fun ensureConnected(): String {
        if (!started) {
            start()
        }
        helloIfNeeded()
        return clientId
    }

    suspend fun publishSnapshotNow() {
        try {
            helloIfNeeded()
            publishSnapshot(force = true)
        } catch (_: Throwable) {
            rpcClient.resetSession()
        }
    }

    fun start() {
        started = true
        if (pollJob?.isActive != true) {
            pollJob?.cancel()
            // Keep the long-poll and command acknowledgement off the Compose main
            // thread. A large report form mutation can schedule an expensive
            // recomposition; if the acknowledgement shares that thread, the MCP
            // command can time out even though the mutation was accepted.
            pollJob = scope.launch(Dispatchers.IO) {
                while (started) {
                    try {
                        helloIfNeeded()
                        pollOnce()
                    } catch (_: Throwable) {
                        rpcClient.resetSession()
                        delay(1_000)
                    }
                }
            }
        }
        if (snapshotJob?.isActive != true) {
            snapshotJob?.cancel()
            snapshotJob = scope.launch {
                while (started) {
                    try {
                        helloIfNeeded()
                        publishSnapshot(force = false)
                    } catch (_: Throwable) {
                        rpcClient.resetSession()
                    }
                    delay(1_000)
                }
            }
        }
    }

    fun stop() {
        started = false
        pollJob?.cancel()
        snapshotJob?.cancel()
        pollJob = null
        snapshotJob = null
        lastSnapshotFingerprint = ""
    }

    private suspend fun helloIfNeeded() {
        rpcClient.hello(clientId)
    }

    private suspend fun pollOnce() {
        val result = rpcClient.poll(clientId, timeoutMs = 20_000) ?: return
        val params = result["params"] as? JsonObject ?: return
        val commandId = jsonString(params["id"]).ifBlank { return }
        val method = jsonString(params["method"]).ifBlank { return }
        val commandParams = params["params"] as? JsonObject ?: JsonObject(emptyMap())
        try {
            val commandResult = withContext(Dispatchers.Main.immediate) {
                commandHandler(method, commandParams)
            }
            updateSelectedWindow(method, commandParams, commandResult)
            rpcClient.respond(commandId = commandId, ok = true, result = commandResult)
            try {
                publishSnapshot(force = true)
            } catch (_: Throwable) {
                rpcClient.resetSession()
            }
        } catch (err: Throwable) {
            rpcClient.respond(commandId = commandId, ok = false, error = err.message ?: err.toString())
        }
    }

    private fun updateSelectedWindow(method: String, params: JsonObject, result: JsonObject) {
        when (method) {
            "ui.window.open" -> {
                selectedWindowId = jsonString(result["windowId"]).ifBlank { selectedWindowId }
            }
            "ui.window.activate", "ui.window.selectTab" -> {
                selectedWindowId = jsonString(params["windowId"]).ifBlank { selectedWindowId }
            }
            "ui.window.close" -> {
                val closing = jsonString(params["windowId"]).ifBlank { return }
                if (selectedWindowId == closing) {
                    selectedWindowId = null
                }
            }
        }
    }

    private suspend fun publishSnapshot(force: Boolean, requireAck: Boolean = false) {
        val snapshot = snapshotProvider()
        val snapshotJson = buildJsonObject {
            snapshot.conversationId?.takeIf { it.isNotBlank() }?.let {
                put("conversationId", JsonPrimitive(it))
            }
            put("clientId", JsonPrimitive(clientId))
            put(
                "selected",
                buildJsonObject {
                    put("windowId", JsonPrimitive(selectedWindowId ?: "chat/new"))
                    put("tabId", JsonPrimitive(selectedWindowId ?: "chat/new"))
                }
            )
            put(
                "windows",
                buildJsonArray {
                    snapshot.windows.forEach { window ->
                        add(window.toJsonObject())
                    }
                }
            )
        }
        val fingerprint = json.encodeToString(snapshotJson)
        if (!force && fingerprint == lastSnapshotFingerprint) {
            return
        }
        val result = rpcClient.snapshot(clientId = clientId, data = snapshotJson)
        if (result == null) {
            if (requireAck) {
                throw IllegalStateException("UI bridge snapshot was not accepted")
            }
            return
        }
        lastSnapshotFingerprint = fingerprint
    }
}

private fun jsonString(value: JsonElement?): String {
    return (value as? JsonPrimitive)?.content?.trim().orEmpty()
}

internal fun loadOrCreateUIBridgeClientId(context: Context): String {
    val prefs = context.getSharedPreferences(UI_BRIDGE_PREFS, Context.MODE_PRIVATE)
    val existing = prefs.getString(UI_BRIDGE_CLIENT_ID_KEY, "")?.trim().orEmpty()
    if (existing.isNotEmpty()) {
        return existing
    }
    val generated = "android-ui-${UUID.randomUUID()}"
    prefs.edit().putString(UI_BRIDGE_CLIENT_ID_KEY, generated).apply()
    return generated
}

internal fun buildAndroidUIBridgeSnapshot(
    activeConversationId: String?,
    forgeRuntime: ForgeRuntime
): NativeUIBridgeSnapshot {
    val conversationId = activeConversationId?.trim().orEmpty()
    val windows = mutableListOf<NativeUIBridgeWindow>()
    if (conversationId.isNotEmpty()) {
        windows += NativeUIBridgeWindow(
            windowId = "chat/new",
            windowKey = "chat/new",
            windowTitle = "Chat",
            conversationId = conversationId,
            inTab = true
        )
    }
    forgeRuntime.windows.value
        .asSequence()
        .filter { conversationId.isNotEmpty() }
        .filter { window ->
            val windowConversationId = window.conversationId?.trim().orEmpty()
            windowConversationId.isEmpty() || windowConversationId == conversationId
        }
        .forEach { window ->
            val windowForm = runCatching {
                forgeRuntime.windowContext(window.windowId).peekWindowForm()
            }.getOrDefault(emptyMap())
            val metadata = forgeRuntime.metadataSignal(window.windowId).peek()?.let { value ->
                runCatching {
                    Json.encodeToJsonElement(WindowMetadata.serializer(), value) as? JsonObject
                }.getOrNull()
            } ?: JsonObject(emptyMap())
            windows += window.toUIBridgeWindow(
                conversationId = conversationId.ifEmpty { window.conversationId?.trim() },
                windowForm = windowForm,
                metadata = nativeMetadataWithReportAuthoringSummary(metadata,windowForm.toJsonObject()),
                dataSources = nativeWindowDataSourceSnapshot(forgeRuntime, window.windowId)
            )
        }
    return NativeUIBridgeSnapshot(
        conversationId = conversationId.ifEmpty { null },
        windows = windows
    )
}

internal fun nativeWindowDataSourceSnapshot(runtime: ForgeRuntime, windowId: String): JsonObject = buildJsonObject {
    runtime.registeredWindowDataSources(windowId).forEach { (instance, context) ->
        val alias = instance.removePrefix("reportDocument:")
        val input = context.input.peek()
        val control = context.control.peek()
        val selection = context.selection.peek()
        put(alias, buildJsonObject {
            put("dataSourceRef", context.dataSourceRef)
            put("input", input.parameters.toJsonObject())
            put("filter", (input.parameters["filters"] as? Map<String, Any?> ?: input.filter).toJsonObject())
            put("control", mapOf("loading" to control.loading, "resolved" to control.resolved, "inactive" to control.inactive, "error" to control.error, "warnings" to control.warnings).toJsonObject())
            put("form", context.form.peek().toJsonObject())
            put("selection", mapOf("selected" to selection.selected, "selection" to selection.selection, "rowIndex" to selection.rowIndex).toJsonObject())
            put("collection", context.collection.peek().toJsonElement())
            put("metrics", context.metrics.peek().toJsonObject())
        })
    }
}

private fun WindowState.toUIBridgeWindow(
    conversationId: String?,
    windowForm: Map<String, Any?> = emptyMap(),
    metadata: JsonObject = JsonObject(emptyMap()),
    dataSources: JsonObject = JsonObject(emptyMap())
): NativeUIBridgeWindow {
    return NativeUIBridgeWindow(
        windowId = windowId,
        windowKey = windowKey,
        windowTitle = windowTitle,
        conversationId = conversationId,
        presentation = presentation,
        region = region,
        parentKey = parentKey,
        workspaceSharePct = workspaceSharePct,
        workspaceMinHeight = workspaceMinHeight,
        parameters = parameters.toJsonObject(),
        windowForm = windowForm.toJsonObject(),
        metadata = metadata,
        dataSources = dataSources,
        inTab = inTab,
        isModal = isModal
    )
}

private fun NativeUIBridgeWindow.toJsonObject(): JsonObject {
    return buildJsonObject {
        put("windowId", JsonPrimitive(windowId))
        put("windowKey", JsonPrimitive(windowKey))
        put("windowTitle", JsonPrimitive(windowTitle))
        conversationId?.takeIf { it.isNotBlank() }?.let { put("conversationId", JsonPrimitive(it)) }
        presentation?.takeIf { it.isNotBlank() }?.let { put("presentation", JsonPrimitive(it)) }
        region?.takeIf { it.isNotBlank() }?.let { put("region", JsonPrimitive(it)) }
        parentKey?.takeIf { it.isNotBlank() }?.let { put("parentKey", JsonPrimitive(it)) }
        workspaceSharePct?.let { put("workspaceSharePct", JsonPrimitive(it)) }
        workspaceMinHeight?.let { put("workspaceMinHeight", JsonPrimitive(it)) }
        put("parameters", parameters)
        put("windowForm", windowForm)
        if (metadata.isNotEmpty()) put("metadata", metadata)
        put("dataSources", dataSources)
        put("inTab", JsonPrimitive(inTab))
        put("isModal", JsonPrimitive(isModal))
    }
}

private fun WindowState.toUIBridgeOpenResult(windowForm: Map<String, Any?> = emptyMap()): JsonObject {
    val window = toUIBridgeWindow(conversationId = conversationId, windowForm = windowForm)
    return buildJsonObject {
        put("ok", JsonPrimitive(true))
        put("selectedWindowId", JsonPrimitive(window.windowId))
        window.toJsonObject().forEach { (key, value) ->
            put(key, value)
        }
    }
}

internal suspend fun handleAndroidUIBridgeCommand(
    method: String,
    params: JsonObject,
    forgeRuntime: ForgeRuntime
): JsonObject {
    return when (method) {
        "ui.window.open" -> {
            val windowKey = jsonString(params["windowKey"]).ifBlank {
                throw IllegalArgumentException("windowKey is required")
            }
            val windowTitle = jsonString(params["windowTitle"]).ifBlank { windowKey }
            val windowId = jsonString(params["windowId"]).ifBlank { null }
            val parameterMap = (params["parameters"] as? JsonObject).toMapValue()
            val options = params["options"] as? JsonObject ?: JsonObject(emptyMap())
            val replaceHostedRegion = (options["replaceHostedRegion"] as? JsonPrimitive)?.booleanOrNull == true
            val presentation = jsonString(options["presentation"]).ifBlank { null }
            val region = jsonString(options["region"]).ifBlank { null }
            val parentKey = jsonString(options["parentKey"]).ifBlank { null }
            val conversationId = jsonString(options["conversationId"]).ifBlank { null }

            if (replaceHostedRegion && presentation?.equals("hosted", ignoreCase = true) == true && !region.isNullOrBlank()) {
                val staleWindowIds = forgeRuntime.windows.value
                    .filter { existing ->
                        existing.windowId != windowId &&
                            existing.presentation.equals("hosted", ignoreCase = true) &&
                            existing.region.equals(region, ignoreCase = true) &&
                            existing.parentKey == parentKey &&
                            existing.conversationId == conversationId
                    }
                    .map { it.windowId }
                staleWindowIds.forEach(forgeRuntime::closeWindow)
            }

            val state = forgeRuntime.openWindow(
                windowKey = windowKey,
                title = windowTitle,
                inTab = true,
                parameters = parameterMap,
                windowIdOverride = windowId,
                conversationId = conversationId,
                presentation = presentation,
                region = region,
                workspaceSharePct = (options["workspaceSharePct"] as? JsonPrimitive)?.intOrNull,
                workspaceMinHeight = (options["workspaceMinHeight"] as? JsonPrimitive)?.intOrNull,
                parentKey = parentKey,
                isModal = false
            )
            state.toUIBridgeOpenResult(
                windowForm = forgeRuntime.windowContext(state.windowId).peekWindowForm()
            )
        }

        "ui.window.close" -> {
            val windowId = jsonString(params["windowId"]).ifBlank {
                throw IllegalArgumentException("windowId is required")
            }
            forgeRuntime.closeWindow(windowId)
            AndroidFeedCanonicalRegistry.clear(forgeRuntime, windowId)
            buildJsonObject { put("ok", JsonPrimitive(true)) }
        }

        "ui.window.setFormData" -> {
            val windowId = jsonString(params["windowId"]).ifBlank {
                throw IllegalArgumentException("windowId is required")
            }
            val existingWindow = forgeRuntime.windows.value.firstOrNull { it.windowId == windowId }
            if (existingWindow == null) {
                throw IllegalArgumentException("window not found: $windowId")
            }
            val values = ((params["values"] as? JsonObject) ?: (params["parameters"] as? JsonObject))
                ?: throw IllegalArgumentException("values must be an object")
            val valuesMap = mergeBridgeMaps(
                base = existingWindow.parameters,
                overlay = values.toMapValue()
            )
            if (valuesMap.isEmpty()) {
                throw IllegalArgumentException("values are required")
            }
            val replace = (params["replace"] as? JsonPrimitive)?.booleanOrNull == true
            forgeRuntime.setWindowFormValues(
                windowId = windowId,
                values = valuesMap,
                replace = replace
            )
            buildJsonObject {
                put("ok", JsonPrimitive(true))
                put("windowId", JsonPrimitive(windowId))
                put("windowForm", forgeRuntime.windowContext(windowId).peekWindowForm().toJsonObject())
            }
        }

        "ui.window.activate" -> buildJsonObject {
            put("ok", JsonPrimitive(true))
        }

        "ui.report.getCurrent" -> {
            val windowId = jsonString(params["windowId"]).ifBlank {
                throw IllegalArgumentException("windowId is required")
            }
            androidReportCurrentResult(
                windowId,
                forgeRuntime.windowContext(windowId).peekWindowForm(),
                forgeRuntime.preparedReportRequest(windowId),
                forgeRuntime.reportRequestInspectionOnly,
                forgeRuntime.nativeReportLifecycle.admission(windowId)?.let { forgeRuntime.reportPreparationIsCurrent(it.preparation) } == true,
                forgeRuntime.nativeReportLifecycle.status(forgeRuntime.preparedReportRequest(windowId)),
                forgeRuntime.verifiedCompletedReportDatasets(windowId)
            )
        }

        "ui.report.run" -> {
            check(!forgeRuntime.reportRequestInspectionOnly) { "Debug report request inspection is active; report execution is disabled." }
            val windowId = jsonString(params["windowId"]).ifBlank {
                throw IllegalArgumentException("windowId is required")
            }
            if (forgeRuntime.windows.value.none { it.windowId == windowId }) {
                throw IllegalArgumentException("report window not found: $windowId")
            }
            val prepared = forgeRuntime.awaitReadyReportPreparation(windowId)
            check(prepared != null && prepared.status == "ready" && forgeRuntime.reportPreparationIsCurrent(prepared)) {
                "The selected report request is not ready: ${prepared?.reason ?: "pending"}"
            }
            val gate = com.viant.forgeandroid.runtime.preparedReportPrimaryGate(prepared.identity, prepared)
            check(gate.status == "ready") { "The selected report request cannot run: ${gate.reason ?: gate.status}" }
            val commandIdentity = nativeReportCommandIdentity(params)
            val requestId = commandIdentity.requestId
            repeat(100) {
                if (forgeRuntime.nativeReportLifecycle.admission(windowId)?.let { forgeRuntime.reportPreparationIsCurrent(it.preparation) } == true) return@repeat
                delay(50)
            }
            val admitted = forgeRuntime.nativeReportLifecycle.begin(windowId, requestId, "prompt", prepared, commandIdentity.reportAdmissionRef) { forgeRuntime.reportPreparationIsCurrent(it) }
            check(forgeRuntime.reportPreparationIsCurrent(admitted.admission.preparation)) { "The report preparation changed during admission." }
            forgeRuntime.setWindowFormValues(
                windowId = windowId,
                values = mapOf(
                    "reportRunRequest" to mapOf(
                        "id" to requestId,
                        "origin" to "ui.report.run"
                    )
                ),
                replace = false,
                bumpPrefillRevision = false
            )
            buildJsonObject {
                put("ok", JsonPrimitive(true))
                put("windowId", JsonPrimitive(windowId))
                put("accepted", JsonPrimitive(true))
                put("materialized", JsonPrimitive(false))
                put("materializationId", JsonPrimitive(requestId))
                put("reportRunId", JsonPrimitive(admitted.reportRunId))
                put("status", JsonPrimitive("running"))
            }
        }

        "ui.data.fetch" -> {
            val windowId = jsonString(params["windowId"]).ifBlank {
                throw IllegalArgumentException("windowId is required")
            }
            val requestedRef = jsonString(params["dataSourceRef"]).ifBlank { null }
            val plan = forgeRuntime.awaitScopedWindowFetchPlan(windowId, requestedRef)
            if (plan.status == "ready") forgeRuntime.executeScopedWindowFetchPlan(windowId, plan)
            buildJsonObject {
                put("ok", plan.status == "ready"); put("status", plan.status)
                plan.reason?.let { put("reason", it) }
                put("dataSourceRefs", JsonArray(plan.targets.map(::JsonPrimitive)))
            }
        }

        "ui.feed.get" -> {
            val feedId = jsonString(params["feedId"]).ifBlank {
                throw IllegalArgumentException("feedId is required")
            }
            val conversationId = jsonString(params["conversationId"]).ifBlank {
                throw IllegalArgumentException("conversationId is required")
            }
            val refs = (params["dataSourceRefs"] as? JsonArray)
                .orEmpty()
                .mapNotNull { (it as? JsonPrimitive)?.content?.trim()?.takeIf(String::isNotEmpty) }
            require(refs.isNotEmpty()) { "dataSourceRefs are required" }
            val window = findAndroidFeedWindow(forgeRuntime, feedId, conversationId)
            val snapshots = snapshotFeedDataSources(forgeRuntime.windowContext(window.windowId), refs)
            buildJsonObject {
                put("conversationId", JsonPrimitive(conversationId))
                put("feedId", JsonPrimitive(feedId))
                put("dataSources", snapshots.mapValues { (_, snapshot) ->
                    mapOf(
                        "form" to snapshot.form,
                        "collection" to snapshot.collection,
                        "selection" to snapshot.selection
                    )
                }.toJsonObject())
            }
        }

        "ui.feed.update" -> {
            val feedId = jsonString(params["feedId"]).ifBlank {
                throw IllegalArgumentException("feedId is required")
            }
            val conversationId = jsonString(params["conversationId"]).ifBlank {
                throw IllegalArgumentException("conversationId is required")
            }
            val rawOperations = (params["operations"] as? JsonArray).orEmpty()
            require(rawOperations.isNotEmpty()) { "operations are required" }
            val operations = rawOperations.mapIndexed { index, element ->
                val value = element as? JsonObject
                    ?: throw IllegalArgumentException("operations[$index] must be an object")
                FeedPatchOperation(
                    dataSourceRef = jsonString(value["dataSourceRef"]).ifBlank {
                        throw IllegalArgumentException("operations[$index].dataSourceRef is required")
                    },
                    op = jsonString(value["op"]).lowercase(),
                    path = jsonString(value["path"]),
                    value = value["value"]?.toKotlinValue()
                )
            }
            val window = findAndroidFeedWindow(forgeRuntime, feedId, conversationId)
            val changedRefs = if (AndroidFeedCanonicalRegistry.has(forgeRuntime, window.windowId)) {
                AndroidFeedCanonicalRegistry.apply(
                    forgeRuntime,
                    window.windowId,
                    operations,
                    turnId = jsonString(params["turnId"])
                )
            } else {
                applyFeedPatchOperations(forgeRuntime.windowContext(window.windowId), operations)
            }
            buildJsonObject {
                put("ok", JsonPrimitive(true))
                put("feedId", JsonPrimitive(feedId))
                put("changedDataSourceRefs", changedRefs.toList().toJsonElement())
            }
        }

        else -> throw IllegalArgumentException("unsupported UI bridge command: $method")
    }
}

private fun findAndroidFeedWindow(
    forgeRuntime: ForgeRuntime,
    feedId: String,
    conversationId: String
): WindowState {
    val expectedKey = "feed-$feedId-$conversationId"
    return forgeRuntime.windows.value.firstOrNull { window ->
        window.windowKey == expectedKey && window.conversationId == conversationId
    } ?: throw IllegalArgumentException("feed is not rendered for this conversation: $feedId")
}

private fun androidReportCurrentResult(
    windowId: String,
    form: Map<String, Any?>,
    preparation: com.viant.forgeandroid.runtime.PreparedReportRequest? = null,
    inspectionOnly: Boolean = false,
    admissionReady: Boolean = false,
    admissionStatus: com.viant.forgeandroid.runtime.NativeReportAdmissionStatus? = null,
    verifiedDatasets:JsonArray?=null
): JsonObject {
    val definition = stringKeyMap(form["reportDefinition"])
    val document = stringKeyMap(definition["documentPatch"])
        .ifEmpty { stringKeyMap(definition["reportDocument"]) }
        .ifEmpty { stringKeyMap(form["documentPatch"]) }
        .ifEmpty { stringKeyMap(form["reportDocument"]) }
    val blocks = document["blocks"] as? List<*>
    val materialization = stringKeyMap(form["reportMaterialization"])
    val status = materialization["status"]?.toString()?.lowercase().orEmpty()
    return buildJsonObject {
        put("ok", JsonPrimitive(true))
        put("windowId", JsonPrimitive(windowId))
        definition["id"]?.let { put("reportId", it.toJsonElement()) }
        document["title"]?.let { put("reportName", it.toJsonElement()) }
        put("canRun", JsonPrimitive(!inspectionOnly && admissionReady && admissionStatus?.status != "error" && !blocks.isNullOrEmpty() && preparation?.let { com.viant.forgeandroid.runtime.preparedReportPrimaryGate(it.identity, it).status == "ready" } == true))
        materialization["reportRunId"]?.let { put("reportRunId", it.toJsonElement()) }
        materialization["contextStatus"]?.let { put("contextStatus", it.toJsonElement()); put("active", materialization["active"].toJsonElement()) }
        materialization["activationError"]?.let { put("activationError", it.toJsonElement()) }
        val preparationStatus = if (preparation?.status != "ready") preparation?.status ?: "pending" else admissionStatus?.status ?: if (admissionReady) "ready" else "pending"
        put("preparationStatus", preparationStatus)
        (preparation?.reason ?: admissionStatus?.reason ?: if (!admissionReady) "Waiting for a prepared authored report and all declared dataset requests." else null)?.let { put("preparationReason", it) }
        put("canSave", JsonPrimitive(false))
        put("hasCompletedRun", JsonPrimitive(status == "completed"))
        put("materialized",JsonPrimitive(status=="completed" && verifiedDatasets!=null))
        if (materialization.isNotEmpty()) {
            val safe=materialization.toMutableMap()
            if(status=="completed" && verifiedDatasets==null) {
                safe["status"]="failed";safe["materialized"]=false;safe["errors"]=listOf("The saved report data does not match its verified datasets.")
            } else if(status=="completed") {
                safe["datasetRefs"]=verifiedDatasets!!.map { it.jsonObject.getValue("id").jsonPrimitive.content }
                safe["rowCounts"]=verifiedDatasets.associate { it.jsonObject.getValue("id").jsonPrimitive.content to it.jsonObject.getValue("rows").jsonArray.size }
            }
            put("materialization", safe.toJsonObject())
        }
    }
}

private suspend fun waitForAndroidReportMaterialization(
    windowId: String,
    requestId: String,
    forgeRuntime: ForgeRuntime
): JsonObject {
    repeat(3_000) {
        val form = forgeRuntime.windowContext(windowId).peekWindowForm()
        val materialization = stringKeyMap(form["reportMaterialization"])
        if (materialization["requestId"]?.toString() == requestId) {
            when (materialization["status"]?.toString()?.lowercase()) {
                "completed" -> {
                    val verified=forgeRuntime.verifiedCompletedReportDatasets(windowId) ?: error("The saved report data does not match its verified datasets.")
                    val referenced = androidReportReferencedDatasetRefs(form)
                    val materialized = verified.map { it.jsonObject.getValue("id").jsonPrimitive.content }.toSet()
                    val missing = referenced.filterNot(materialized::contains)
                    if (missing.isNotEmpty()) {
                        throw IllegalStateException(
                            "Report run did not materialize referenced datasets: ${missing.joinToString(", ")}"
                        )
                    }
                    return buildJsonObject {
                        put("ok", JsonPrimitive(true))
                        put("windowId", JsonPrimitive(windowId))
                        put("materialized", JsonPrimitive(true))
                        put("materializationId", JsonPrimitive(requestId))
                        put("status", JsonPrimitive("completed"))
                        materialization["reportRunId"]?.let { put("reportRunId", it.toJsonElement()) }
                        materialization["contextStatus"]?.let { put("contextStatus", it.toJsonElement()); put("active", materialization["active"].toJsonElement()) }
                        materialization["activationError"]?.let { put("activationError", it.toJsonElement()) }
                        put("datasetRefs", materialized.toList().toJsonElement())
                        put("rowCounts", verified.associate { it.jsonObject.getValue("id").jsonPrimitive.content to it.jsonObject.getValue("rows").jsonArray.size }.toJsonElement())
                    }
                }
                "failed" -> {
                    val errors = (materialization["errors"] as? List<*>)
                        .orEmpty().joinToString("; ") { it.toString() }
                    throw IllegalStateException(errors.ifBlank { "The native report run failed." })
                }
            }
        }
        delay(100)
    }
    throw IllegalStateException("The native report run did not finish in time.")
}

private fun androidReportReferencedDatasetRefs(form: Map<String, Any?>): Set<String> {
    val definition = stringKeyMap(form["reportDefinition"])
    val document = stringKeyMap(definition["documentPatch"])
        .ifEmpty { stringKeyMap(definition["reportDocument"]) }
    return (document["blocks"] as? List<*>)
        .orEmpty()
        .mapNotNull { block -> stringKeyMap(block)["datasetRef"]?.toString()?.trim()?.takeIf(String::isNotEmpty) }
        .toSet()
}

private fun stringKeyMap(value: Any?): Map<String, Any?> =
    (value as? Map<*, *>)?.entries?.associate { it.key.toString() to it.value }.orEmpty()

private suspend fun ForgeRuntime.awaitReadyReportPreparation(windowId: String): com.viant.forgeandroid.runtime.PreparedReportRequest? {
    repeat(100) {
        val prepared = preparedReportRequest(windowId)
        if (prepared != null && prepared.status != "pending") return prepared
        delay(100)
    }
    return preparedReportRequest(windowId)
}

private suspend fun ForgeRuntime.awaitScopedWindowFetchPlan(windowId: String, requestedRef: String?): com.viant.forgeandroid.runtime.PreparedFetchPlan {
    repeat(100) {
        val metadata = metadataSignal(windowId).peek()
        if (metadata != null) {
            val prepared = preparedReportRequest(windowId)
            val active = visibleWindowDataSourceRefs(windowId)
            if (active == null && requestedRef == null) { delay(100); return@repeat }
            val owned = com.viant.forgeandroid.runtime.reportOwnedDataSourceRefs(metadata)
            val ready = prepared?.takeIf { com.viant.forgeandroid.runtime.preparedReportPrimaryGate(it.identity, it).status == "ready" }
            val readyRefs = ready?.let { setOf(it.dataSourceRef) + it.publishedSources.filter { it.request != null }.map { it.dataSourceRef } }.orEmpty()
            val plan = com.viant.forgeandroid.runtime.preparedReportFetchPlan(requestedRef, active.orEmpty(), metadata.dataSources.keys, owned, readyRefs)
            if (plan.status != "pending") return plan
            if (prepared?.status == "error") return plan.copy(status = "error", reason = prepared.reason)
        }
        delay(100)
    }
    return com.viant.forgeandroid.runtime.PreparedFetchPlan("pending", reason = "Report request preparation is pending")
}

private fun ForgeRuntime.executeScopedWindowFetchPlan(windowId: String, plan: com.viant.forgeandroid.runtime.PreparedFetchPlan) {
    val metadata = metadataSignal(windowId).peek() ?: error("Window metadata is unavailable")
    val owned = com.viant.forgeandroid.runtime.reportOwnedDataSourceRefs(metadata)
    val prepared = preparedReportRequest(windowId)
    // Resolve and validate EVERY target before allowing the first effect.
    val requests = plan.targets.map { ref ->
        if (ref !in owned) ref to null else {
            check(prepared != null && reportPreparationIsCurrent(prepared)) { "The report preparation is stale" }
            if (ref == prepared.dataSourceRef) {
                val gate = com.viant.forgeandroid.runtime.preparedReportPrimaryGate(prepared.identity, prepared)
                check(gate.status == "ready") { "The report request is not ready" }
                ref to gate.request
            } else {
                val source = prepared.publishedSources.filter { it.dataSourceRef == ref }.singleOrNull() ?: error("The report datasource has ambiguous dataset requests")
                val result = com.viant.forgeandroid.runtime.preparePublishedReportRequest(prepared.identity, prepared, source)
                check(result.status == "ready") { result.reason ?: "The report dataset request is not ready" }
                ref to result.request
            }
        }
    }
    if (requests.any { it.first in owned }) {
        check(prepared != null && reportPreparationIsCurrent(prepared)) { "The report preparation changed" }
        check(nativeReportLifecycle.canPreview(windowContext(windowId))) { "This report is admitted or completed. Use an explicit report run to refresh its data." }
    }
    requests.forEach { (ref, request) ->
        if (request == null) refreshDataSourceCollection(windowId, ref)
        else checkNotNull(windowContext(windowId).contextOrNull(ref)?.setPreparedInputParameters(JsonUtil.asStringMap(JsonUtil.elementToAny(request))) { prepared != null && reportPreparationIsCurrent(prepared) }) { "The report preview query is not authorized." }
    }
}

private fun Map<String, Any?>.toJsonObject(): JsonObject {
    return JsonObject(entries.associate { (key, value) -> key to value.toJsonElement() })
}

private fun JsonObject?.toMapValue(): Map<String, Any?> {
    return this?.entries?.associate { (key, value) -> key to value.toKotlinValue() } ?: emptyMap()
}

private fun mergeBridgeMaps(
    base: Map<String, Any?>,
    overlay: Map<String, Any?>
): Map<String, Any?> {
    if (base.isEmpty()) {
        return overlay
    }
    if (overlay.isEmpty()) {
        return base
    }
    val merged = LinkedHashMap<String, Any?>()
    merged.putAll(base)
    overlay.forEach { (key, value) ->
        val baseMap = merged[key] as? Map<*, *>
        val overlayMap = value as? Map<*, *>
        merged[key] = if (baseMap != null && overlayMap != null) {
            mergeBridgeMaps(
                base = baseMap.entries.associate { it.key.toString() to it.value },
                overlay = overlayMap.entries.associate { it.key.toString() to it.value }
            )
        } else {
            value
        }
    }
    return merged
}

private fun JsonElement?.toKotlinValue(): Any? {
    return when (this) {
        null, JsonNull -> null
        is JsonObject -> entries.associate { (key, value) -> key to value.toKotlinValue() }
        is kotlinx.serialization.json.JsonArray -> map { it.toKotlinValue() }
        is JsonPrimitive -> when {
            isString -> content
            booleanOrNull != null -> booleanOrNull
            longOrNull != null -> longOrNull
            doubleOrNull != null -> doubleOrNull
            else -> content
        }
        else -> null
    }
}

private fun Any?.toJsonElement(): JsonElement {
    return when (this) {
        null -> JsonNull
        is JsonElement -> this
        is String -> JsonPrimitive(this)
        is Boolean -> JsonPrimitive(this)
        is Int -> JsonPrimitive(this)
        is Long -> JsonPrimitive(this)
        is Float -> JsonPrimitive(this)
        is Double -> JsonPrimitive(this)
        is Number -> JsonPrimitive(this.toDouble())
        is Map<*, *> -> JsonObject(entries.associate { (key, value) -> key.toString() to value.toJsonElement() })
        is Iterable<*> -> buildJsonArray { this@toJsonElement.forEach { add(it.toJsonElement()) } }
        else -> JsonPrimitive(toString())
    }
}
