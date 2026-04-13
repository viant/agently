package com.viant.agently.android

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.ApprovalMeta
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.ResolveElicitationInput
import com.viant.agentlysdk.stream.PendingElicitation
import com.viant.forgeandroid.runtime.ContainerDef
import com.viant.forgeandroid.runtime.ContentDef
import com.viant.forgeandroid.runtime.DataSourceDef
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.ViewDef
import com.viant.forgeandroid.runtime.WindowMetadata
import com.viant.forgeandroid.ui.ContainerRenderer
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

private const val ELICITATION_FORM_DATA_SOURCE = "elicitationForm"
private val elicitationJson = Json { ignoreUnknownKeys = true }

@Composable
internal fun ElicitationOverlay(
    elicitation: PendingElicitation,
    conversationId: String,
    onResolved: () -> Unit,
    client: AgentlyClient,
    forgeRuntime: ForgeRuntime
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val requestedSchema = elicitation.requestedSchema
    val preparedSchema = remember(requestedSchema) { prepareRequestedSchema(requestedSchema) }
    val approvalMeta = remember(requestedSchema) { extractToolApprovalMeta(requestedSchema) }
    val visiblePropertyCount = remember(preparedSchema) { schemaVisiblePropertyCount(preparedSchema) }
    val isOob = remember(elicitation.mode, elicitation.url) {
        val mode = elicitation.mode?.trim()?.lowercase()
        !elicitation.url.isNullOrBlank() || mode == "oob" || mode == "webonly" || mode == "url"
    }
    var submitting by remember(elicitation.elicitationId) { mutableStateOf(false) }
    var error by remember(elicitation.elicitationId) { mutableStateOf<String?>(null) }
    var windowId by remember(elicitation.elicitationId) { mutableStateOf<String?>(null) }
    var approvalValues by remember(elicitation.elicitationId, approvalMeta) {
        mutableStateOf(buildApprovalEditorState(approvalMeta))
    }

    LaunchedEffect(elicitation.elicitationId, preparedSchema, isOob) {
        error = null
        if (isOob || approvalMeta != null || preparedSchema == null || visiblePropertyCount == 0 || windowId != null) {
            return@LaunchedEffect
        }
        val state = forgeRuntime.openWindowInline(
            windowKey = "elicitation-${elicitation.elicitationId}",
            title = "Elicitation",
            metadata = buildElicitationWindow(preparedSchema)
        )
        windowId = state.windowId
        val formContext = forgeRuntime.windowContext(state.windowId).context(ELICITATION_FORM_DATA_SOURCE)
        val seed = buildSchemaDefaultSeed(preparedSchema)
        if (seed.isNotEmpty()) {
            formContext.setForm(seed)
        }
    }

    DisposableEffect(windowId) {
        onDispose {
            windowId?.let { forgeRuntime.closeWindow(it) }
        }
    }

    val activeWindowId = windowId
    val windowContext = remember(activeWindowId) {
        activeWindowId?.let { forgeRuntime.windowContext(it) }
    }
    val metadataSignal = remember(activeWindowId) {
        activeWindowId?.let { forgeRuntime.metadataSignal(it) }
    }
    val metadata by if (metadataSignal != null) {
        metadataSignal.flow.collectAsState(initial = metadataSignal.peek())
    } else {
        remember { mutableStateOf<WindowMetadata?>(null) }
    }
    val formContext = remember(activeWindowId) {
        activeWindowId?.let { forgeRuntime.windowContext(it).context(ELICITATION_FORM_DATA_SOURCE) }
    }
    val formState by if (formContext != null) {
        formContext.form.flow.collectAsState(initial = formContext.peekForm())
    } else {
        remember { mutableStateOf(emptyMap<String, Any?>()) }
    }

    fun resolve(action: String, payload: Map<String, JsonElement> = emptyMap(), openUrl: Boolean = false) {
        if (submitting) {
            return
        }
        scope.launch {
            submitting = true
            error = null
            try {
                if (openUrl) {
                    elicitation.url?.takeIf { it.isNotBlank() }?.let { url ->
                        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
                    }
                }
                client.resolveElicitation(
                    ResolveElicitationInput(
                        conversationId = conversationId,
                        elicitationId = elicitation.elicitationId,
                        action = action,
                        payload = payload
                    )
                )
                onResolved()
            } catch (err: Throwable) {
                error = err.message ?: err.toString()
            } finally {
                submitting = false
            }
        }
    }

    val title = when {
        !approvalMeta?.title.isNullOrBlank() -> approvalMeta?.title ?: "Input Required"
        isOob -> "Action Required"
        else -> "Input Required"
    }

    AlertDialog(
        onDismissRequest = {
            if (!submitting) {
                resolve("cancel")
            }
        },
        title = { Text(title) },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                elicitation.message?.takeIf { it.isNotBlank() }?.let {
                    Text(
                        text = it,
                        style = MaterialTheme.typography.bodyMedium
                    )
                }
                approvalMeta?.toolName?.takeIf { it.isNotBlank() }?.let { toolName ->
                    Text(
                        text = "Tool: $toolName",
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                }
                if (isOob) {
                    elicitation.url?.takeIf { it.isNotBlank() }?.let { url ->
                        Text(
                            text = url,
                            style = MaterialTheme.typography.bodySmall,
                            color = Color(0xFF155EEF)
                        )
                    }
                } else if (approvalMeta != null) {
                    if (approvalMeta.editors.isNotEmpty()) {
                        ApprovalForgeEditors(
                            approvalId = elicitation.elicitationId,
                            forgeRuntime = forgeRuntime,
                            approval = PendingToolApproval(
                                id = elicitation.elicitationId,
                                toolName = approvalMeta.toolName ?: "approval",
                                title = approvalMeta.title,
                                status = "pending"
                            ),
                            meta = approvalMeta,
                            selectedFields = approvalValues,
                            onEditChange = { _, fieldName, value ->
                                approvalValues = approvalValues.toMutableMap().apply {
                                    this[fieldName] = value
                                }
                            }
                        )
                    }
                } else if (visiblePropertyCount == 0) {
                    Text(
                        text = "Respond to continue.",
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                } else if (activeWindowId == null || metadata == null || windowContext == null) {
                    CircularProgressIndicator(modifier = Modifier.width(24.dp))
                } else {
                    metadata?.view?.content?.containers?.forEach { container ->
                        ContainerRenderer(forgeRuntime, windowContext, container)
                    }
                }
                error?.takeIf { it.isNotBlank() }?.let {
                    Text(
                        text = it,
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFFB42318)
                    )
                }
            }
        },
        dismissButton = {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(
                    onClick = { resolve("decline") },
                    enabled = !submitting
                ) {
                    Text(approvalMeta?.rejectLabel ?: "Decline")
                }
                OutlinedButton(
                    onClick = { resolve("cancel") },
                    enabled = !submitting
                ) {
                    Text(approvalMeta?.cancelLabel ?: "Cancel")
                }
            }
        },
        confirmButton = {
            Button(
                onClick = {
                    if (isOob) {
                        resolve("accept", emptyMap(), openUrl = true)
                    } else if (approvalMeta != null) {
                        resolve("accept", mapOf("editedFields" to JsonObject(approvalValues)))
                    } else {
                        resolve("accept", formState.toJsonPayload())
                    }
                },
                enabled = !submitting && (isOob || approvalMeta != null || activeWindowId != null)
            ) {
                if (submitting) {
                    CircularProgressIndicator(
                        modifier = Modifier.width(16.dp),
                        strokeWidth = 2.dp
                    )
                } else {
                    Text(
                        when {
                            isOob -> "Open"
                            approvalMeta?.acceptLabel?.isNotBlank() == true -> approvalMeta.acceptLabel ?: "Submit"
                            else -> "Submit"
                        }
                    )
                }
            }
        }
    )
}

internal fun buildElicitationWindow(schema: JsonObject): WindowMetadata {
    return WindowMetadata(
        namespace = "agently.android.elicitation",
        dataSources = mapOf(
            ELICITATION_FORM_DATA_SOURCE to DataSourceDef(selectionMode = "none")
        ),
        view = ViewDef(
            content = ContentDef(
                containers = listOf(
                    ContainerDef(
                        id = "elicitationForm",
                        dataSourceRef = ELICITATION_FORM_DATA_SOURCE,
                        schemaBasedForm = com.viant.forgeandroid.runtime.SchemaBasedFormDef(
                            id = "elicitationSchemaForm",
                            dataSourceRef = ELICITATION_FORM_DATA_SOURCE,
                            schema = schema,
                            showSubmit = false
                        )
                    )
                )
            )
        )
    )
}

internal fun prepareRequestedSchema(requestedSchema: JsonObject?): JsonObject? {
    requestedSchema ?: return null
    val properties = requestedSchema["properties"] as? JsonObject ?: JsonObject(emptyMap())
    val cleanedProperties = buildMap<String, JsonElement> {
        properties.forEach { (key, value) ->
            if (!key.startsWith("_")) {
                put(key, normalizeRequestedProperty(value))
            }
        }
    }
    val result = requestedSchema.toMutableMap()
    result["properties"] = JsonObject(cleanedProperties)
    val required = (requestedSchema["required"] as? JsonArray)
        ?.filterIsInstance<JsonPrimitive>()
        ?.filterNot { it.content.startsWith("_") }
    if (required != null) {
        result["required"] = JsonArray(required)
    }
    return JsonObject(result)
}

internal fun extractToolApprovalMeta(requestedSchema: JsonObject?): ApprovalMeta? {
    requestedSchema ?: return null
    val properties = requestedSchema["properties"] as? JsonObject ?: return null
    normalizeToolApprovalMeta(readSchemaConst(properties, "_approvalMeta"))?.let {
        return it
    }
    if (readSchemaConst(properties, "_type") != "tool_approval") {
        return null
    }
    return ApprovalMeta(
        type = "tool_approval",
        title = readSchemaConst(properties, "_title").ifBlank { "Approval Required" },
        toolName = readSchemaConst(properties, "_toolName").ifBlank { null },
        acceptLabel = readSchemaConst(properties, "_acceptLabel").ifBlank { "Allow" },
        rejectLabel = readSchemaConst(properties, "_rejectLabel").ifBlank { "Decline" },
        cancelLabel = readSchemaConst(properties, "_cancelLabel").ifBlank { "Cancel" }
    )
}

private fun normalizeToolApprovalMeta(raw: String): ApprovalMeta? {
    val trimmed = raw.trim()
    if (trimmed.isBlank()) {
        return null
    }
    return runCatching {
        elicitationJson.decodeFromString<ApprovalMeta>(trimmed)
    }.getOrNull()
}

private fun readSchemaConst(properties: JsonObject, key: String): String {
    val field = properties[key] as? JsonObject ?: return ""
    (field["const"] as? JsonPrimitive)?.contentOrNull?.trim()?.takeIf { it.isNotBlank() }?.let { return it }
    (field["default"] as? JsonPrimitive)?.contentOrNull?.trim()?.takeIf { it.isNotBlank() }?.let { return it }
    ((field["enum"] as? JsonArray)?.firstOrNull() as? JsonPrimitive)?.contentOrNull?.trim()?.takeIf { it.isNotBlank() }?.let { return it }
    return ""
}

private fun buildApprovalEditorState(meta: ApprovalMeta?): Map<String, JsonElement> {
    meta ?: return emptyMap()
    val state = linkedMapOf<String, JsonElement>()
    meta.editors.forEach { editor ->
        val defaults = editor.options.filter { it.selected }.map { JsonPrimitive(it.id) }
        if (defaults.isEmpty()) {
            return@forEach
        }
        state[editor.name] = if (editor.kind.equals("radio_list", ignoreCase = true)) {
            defaults.first()
        } else {
            JsonArray(defaults)
        }
    }
    return state
}

private fun normalizeRequestedProperty(value: JsonElement): JsonElement {
    val property = value as? JsonObject ?: return value
    val type = (property["type"] as? JsonPrimitive)?.content?.lowercase()
    val normalized = property.toMutableMap()
    when (type) {
        "array" -> {
            val default = normalized["default"]
            if (default !is JsonArray) {
                normalized["default"] = JsonArray(emptyList())
            }
        }
        "object" -> {
            val default = normalized["default"]
            if (default !is JsonObject) {
                normalized["default"] = JsonObject(emptyMap())
            }
        }
    }
    return JsonObject(normalized)
}

private fun schemaVisiblePropertyCount(schema: JsonObject?): Int {
    val properties = schema?.get("properties") as? JsonObject ?: return 0
    return properties.size
}

private fun buildSchemaDefaultSeed(schema: JsonObject?): Map<String, Any?> {
    val properties = schema?.get("properties") as? JsonObject ?: return emptyMap()
    val seeded = linkedMapOf<String, Any?>()
    properties.forEach { (key, value) ->
        val property = value as? JsonObject ?: return@forEach
        if (key.startsWith("_")) {
            return@forEach
        }
        if (!property.containsKey("default")) {
            return@forEach
        }
        seeded[key] = jsonElementToValue(property["default"])
    }
    return seeded
}

private fun Map<String, Any?>.toJsonPayload(): Map<String, JsonElement> {
    return entries.associate { (key, value) ->
        key to anyToJsonElement(value)
    }
}

private fun jsonElementToValue(element: JsonElement?): Any? {
    return when (element) {
        null -> null
        is JsonObject -> element.mapValues { jsonElementToValue(it.value) }
        is JsonArray -> element.map { jsonElementToValue(it) }
        is JsonPrimitive -> {
            val content = element.content
            when {
                element.isString -> element.content
                content.equals("true", ignoreCase = true) || content.equals("false", ignoreCase = true) ->
                    content.equals("true", ignoreCase = true)
                content.toLongOrNull() != null -> content.toLong()
                content.toDoubleOrNull() != null -> content.toDouble()
                else -> content
            }
        }
        else -> element.toString()
    }
}

private fun anyToJsonElement(value: Any?): JsonElement {
    return when (value) {
        null -> JsonPrimitive(null as String?)
        is JsonElement -> value
        is String -> JsonPrimitive(value)
        is Boolean -> JsonPrimitive(value)
        is Int -> JsonPrimitive(value)
        is Long -> JsonPrimitive(value)
        is Float -> JsonPrimitive(value)
        is Double -> JsonPrimitive(value)
        is Number -> JsonPrimitive(value.toDouble())
        is Map<*, *> -> JsonObject(
            value.entries.associateNotNull { entry ->
                val key = entry.key as? String ?: return@associateNotNull null
                key to anyToJsonElement(entry.value)
            }
        )
        is Iterable<*> -> JsonArray(value.map { anyToJsonElement(it) })
        is Array<*> -> JsonArray(value.map { anyToJsonElement(it) })
        else -> JsonPrimitive(value.toString())
    }
}

private inline fun <T, R : Any> Iterable<T>.associateNotNull(transform: (T) -> Pair<String, R>?): Map<String, R> {
    val result = linkedMapOf<String, R>()
    for (item in this) {
        val pair = transform(item) ?: continue
        result[pair.first] = pair.second
    }
    return result
}
