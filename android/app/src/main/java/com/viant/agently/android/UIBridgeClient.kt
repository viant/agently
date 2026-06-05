package com.viant.agently.android

import android.content.Context
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.UIBridgeRpcClient
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.WindowMetadata
import com.viant.forgeandroid.runtime.WindowState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.booleanOrNull
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
            pollJob = scope.launch {
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
            val commandResult = commandHandler(method, commandParams)
            updateSelectedWindow(method, commandParams, commandResult)
            rpcClient.respond(commandId = commandId, ok = true, result = commandResult)
            publishSnapshot(force = true)
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

    private suspend fun publishSnapshot(force: Boolean) {
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
        .filter { window ->
            val windowConversationId = window.conversationId?.trim().orEmpty()
            conversationId.isEmpty() || windowConversationId.isEmpty() || windowConversationId == conversationId
        }
        .forEach { window ->
            val windowForm = runCatching {
                forgeRuntime.windowContext(window.windowId).peekWindowForm()
            }.getOrDefault(emptyMap())
            windows += window.toUIBridgeWindow(
                conversationId = conversationId.ifEmpty { window.conversationId?.trim() },
                windowForm = windowForm
            )
        }
    return NativeUIBridgeSnapshot(
        conversationId = conversationId.ifEmpty { null },
        windows = windows
    )
}

private fun WindowState.toUIBridgeWindow(
    conversationId: String?,
    windowForm: Map<String, Any?> = emptyMap()
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
            buildJsonObject { put("ok", JsonPrimitive(true)) }
        }

        "ui.window.setFormData" -> {
            val windowId = jsonString(params["windowId"]).ifBlank {
                throw IllegalArgumentException("windowId is required")
            }
            if (forgeRuntime.windows.value.none { it.windowId == windowId }) {
                throw IllegalArgumentException("window not found: $windowId")
            }
            val values = ((params["values"] as? JsonObject) ?: (params["parameters"] as? JsonObject))
                ?: throw IllegalArgumentException("values must be an object")
            val valuesMap = values.toMapValue()
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

        "ui.data.fetch" -> {
            val windowId = jsonString(params["windowId"]).ifBlank {
                throw IllegalArgumentException("windowId is required")
            }
            val requestedRef = jsonString(params["dataSourceRef"]).ifBlank { null }
            val metadataSignal = forgeRuntime.metadataSignal(windowId)
            val metadata = metadataSignal.peek()
            if (metadata != null || requestedRef != null) {
                forgeRuntime.refreshWindowDataSources(windowId, requestedRef, metadata)
            } else {
                forgeRuntime.scope.launch {
                    val loaded = metadataSignal.flow.first { it != null }
                    forgeRuntime.refreshWindowDataSources(windowId, requestedRef = null, metadata = loaded)
                }
            }
            buildJsonObject { put("ok", JsonPrimitive(true)) }
        }

        else -> throw IllegalArgumentException("unsupported UI bridge command: $method")
    }
}

private fun ForgeRuntime.refreshWindowDataSources(
    windowId: String,
    requestedRef: String?,
    metadata: WindowMetadata?
) {
    val dataSourceRefs = requestedRef?.let(::listOf)
        ?: metadata.defaultDataSourceRefs()
    dataSourceRefs.forEach { ref ->
        refreshDataSourceCollection(
            windowID = windowId,
            dataSourceRef = ref
        )
    }
}

private fun WindowMetadata?.defaultDataSourceRefs(): List<String> {
    if (this == null) {
        return emptyList()
    }
    val refs = mutableListOf<String>()
    view?.content?.containers.orEmpty()
        .mapNotNullTo(refs) { it.dataSourceRef?.takeIf(String::isNotBlank) }
    dataSources.keys.forEach(refs::add)
    return refs.distinct()
}

private fun Map<String, Any?>.toJsonObject(): JsonObject {
    return JsonObject(entries.associate { (key, value) -> key to value.toJsonElement() })
}

private fun JsonObject?.toMapValue(): Map<String, Any?> {
    return this?.entries?.associate { (key, value) -> key to value.toKotlinValue() } ?: emptyMap()
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
