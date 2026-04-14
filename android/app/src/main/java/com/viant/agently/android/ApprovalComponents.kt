package com.viant.agently.android

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.DisposableEffect
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.PendingToolApproval
import com.viant.forgeandroid.runtime.ContentDef
import com.viant.forgeandroid.runtime.ContainerDef
import com.viant.forgeandroid.runtime.DataSourceDef
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.ForgeTargetContext
import com.viant.forgeandroid.runtime.ItemDef
import com.viant.forgeandroid.runtime.OptionDef
import com.viant.forgeandroid.runtime.SchemaBasedFormDef
import com.viant.forgeandroid.runtime.ViewDef
import com.viant.forgeandroid.runtime.WindowMetadata
import com.viant.forgeandroid.ui.ContainerRenderer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.encodeToJsonElement

private const val APPROVAL_FORM_DATA_SOURCE = "approvalForm"
private val approvalStateJson = Json { ignoreUnknownKeys = true }
private val approvalReservedFormKeys = setOf(
    "approvalSchemaJSON",
    "approval",
    "originalArgs",
    "editedFields"
)

@Composable
internal fun PendingApprovalsSection(
    approvals: List<PendingToolApproval>,
    forgeRuntime: ForgeRuntime,
    approvalJson: Json,
    approvalEdits: Map<String, Map<String, JsonElement>>,
    onEditChange: (String, String, JsonElement) -> Unit,
    onDecision: (PendingToolApproval, String) -> Unit
) {
    if (approvals.isEmpty()) {
        return
    }
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Text("Approvals", style = MaterialTheme.typography.titleMedium)
            approvals.forEach { approval ->
                ApprovalCardContent(
                    approval = approval,
                    forgeRuntime = forgeRuntime,
                    meta = decodeApprovalMeta(approval.metadata, approvalJson),
                    selectedFields = approvalEdits[approval.id].orEmpty(),
                    onEditChange = onEditChange,
                    onDecision = onDecision,
                    containerColor = MaterialTheme.colorScheme.surfaceVariant
                )
            }
        }
    }
}

@Composable
internal fun InlineApprovalCard(
    approval: PendingToolApproval,
    forgeRuntime: ForgeRuntime,
    approvalJson: Json,
    selectedFields: Map<String, JsonElement>,
    onEditChange: (String, String, JsonElement) -> Unit,
    onDecision: (PendingToolApproval, String) -> Unit
) {
    ApprovalCardContent(
        approval = approval,
        forgeRuntime = forgeRuntime,
        meta = decodeApprovalMeta(approval.metadata, approvalJson),
        selectedFields = selectedFields,
        onEditChange = onEditChange,
        onDecision = onDecision,
        containerColor = Color(0xFFFFF7E8)
    )
}

internal fun decodeApprovalMeta(
    metadata: JsonElement?,
    approvalJson: Json
): com.viant.agentlysdk.ApprovalMeta? {
    metadata ?: return null
    val candidates = buildList {
        add(metadata)
        val metadataObject = metadata as? JsonObject
        metadataObject?.get("approval")?.let { add(it) }
    }
    candidates.forEach { candidate ->
        val direct = decodeApprovalMetaCandidate(candidate, approvalJson)
        if (direct != null) {
            return direct
        }
        if (candidate is JsonObject && looksLikeApprovalMeta(candidate)) {
            return normalizeApprovalMeta(candidate)
        }
    }
    return null
}

private fun decodeApprovalMetaCandidate(
    candidate: JsonElement,
    approvalJson: Json
): com.viant.agentlysdk.ApprovalMeta? {
    return try {
        approvalJson.decodeFromJsonElement<com.viant.agentlysdk.ApprovalMeta>(candidate)
            .takeIf(::hasMeaningfulApprovalMeta)
    } catch (_: Exception) {
        null
    }
}

private fun hasMeaningfulApprovalMeta(meta: com.viant.agentlysdk.ApprovalMeta): Boolean {
    return !meta.toolName.isNullOrBlank() ||
        !meta.title.isNullOrBlank() ||
        !meta.message.isNullOrBlank() ||
        meta.editors.isNotEmpty() ||
        meta.forge != null
}

private fun looksLikeApprovalMeta(candidate: JsonObject): Boolean {
    val value = (candidate["approval"] as? JsonObject) ?: candidate
    return "toolName" in value ||
        "title" in value ||
        "message" in value ||
        "editors" in value ||
        "forge" in value
}

private fun normalizeApprovalMeta(candidate: JsonObject): com.viant.agentlysdk.ApprovalMeta? {
    val value = (candidate["approval"] as? JsonObject) ?: candidate
    val type = jsonPrimitiveString(value["type"])
    if (type.isNotEmpty() && type != "tool_approval") {
        return null
    }
    return com.viant.agentlysdk.ApprovalMeta(
        type = "tool_approval",
        toolName = jsonPrimitiveString(value["toolName"]).ifBlank { null },
        title = jsonPrimitiveString(value["title"]).ifBlank { null },
        message = jsonPrimitiveString(value["message"]).ifBlank { null },
        acceptLabel = jsonPrimitiveString(value["acceptLabel"]).ifBlank { null },
        rejectLabel = jsonPrimitiveString(value["rejectLabel"]).ifBlank { null },
        cancelLabel = jsonPrimitiveString(value["cancelLabel"]).ifBlank { null },
        forge = normalizeApprovalForgeView(value["forge"] as? JsonObject),
        editors = decodeApprovalEditors(value["editors"])
    )
}

private fun normalizeApprovalForgeView(candidate: JsonObject?): com.viant.agentlysdk.ApprovalForgeView? {
    candidate ?: return null
    return com.viant.agentlysdk.ApprovalForgeView(
        windowRef = jsonPrimitiveString(candidate["windowRef"]).ifBlank { null },
        containerRef = jsonPrimitiveString(candidate["containerRef"]).ifBlank { null },
        dataSource = jsonPrimitiveString(candidate["dataSource"]).ifBlank { null },
        callbacks = decodeApprovalCallbacks(candidate["callbacks"])
    )
}

private fun normalizeApprovalEditor(candidate: JsonObject?): com.viant.agentlysdk.ApprovalEditor? {
    candidate ?: return null
    val name = jsonPrimitiveString(candidate["name"])
    if (name.isBlank()) {
        return null
    }
    return com.viant.agentlysdk.ApprovalEditor(
        name = name,
        kind = jsonPrimitiveString(candidate["kind"]).ifBlank { "checkbox_list" },
        path = jsonPrimitiveString(candidate["path"]).ifBlank { null },
        label = jsonPrimitiveString(candidate["label"]).ifBlank { null },
        description = jsonPrimitiveString(candidate["description"]).ifBlank { null },
        options = decodeApprovalOptions(candidate["options"])
    )
}

private fun normalizeApprovalOption(candidate: JsonObject?): com.viant.agentlysdk.ApprovalOption? {
    candidate ?: return null
    val id = jsonPrimitiveString(candidate["id"])
    val label = jsonPrimitiveString(candidate["label"])
    if (id.isBlank() || label.isBlank()) {
        return null
    }
    return com.viant.agentlysdk.ApprovalOption(
        id = id,
        label = label,
        description = jsonPrimitiveString(candidate["description"]).ifBlank { null },
        item = candidate["item"],
        selected = jsonPrimitiveBoolean(candidate["selected"]) ?: true
    )
}

private fun decodeApprovalEditors(element: JsonElement?): List<com.viant.agentlysdk.ApprovalEditor> {
    element ?: return emptyList()
    val array = element as? JsonArray ?: error("Approval editors must be a JSON array")
    return array.mapIndexed { index, entry ->
        normalizeApprovalEditor(entry as? JsonObject)
            ?: error("Approval editor at index $index is malformed")
    }
}

private fun decodeApprovalCallbacks(element: JsonElement?): List<com.viant.agentlysdk.ApprovalCallback> {
    element ?: return emptyList()
    val array = element as? JsonArray ?: error("Approval callbacks must be a JSON array")
    return array.mapIndexed { index, entry ->
        val callback = entry as? JsonObject ?: error("Approval callback at index $index must be a JSON object")
        val handler = jsonPrimitiveString(callback["handler"])
        if (handler.isBlank()) {
            error("Approval callback at index $index is missing a handler")
        }
        com.viant.agentlysdk.ApprovalCallback(
            elementId = jsonPrimitiveString(callback["elementId"]).ifBlank { null },
            event = jsonPrimitiveString(callback["event"]).ifBlank { null },
            handler = handler
        )
    }
}

private fun decodeApprovalOptions(element: JsonElement?): List<com.viant.agentlysdk.ApprovalOption> {
    element ?: return emptyList()
    val array = element as? JsonArray ?: error("Approval options must be a JSON array")
    return array.mapIndexed { index, entry ->
        normalizeApprovalOption(entry as? JsonObject)
            ?: error("Approval option at index $index is malformed")
    }
}

private fun jsonPrimitiveString(element: JsonElement?): String {
    return (element as? JsonPrimitive)?.content?.trim().orEmpty()
}

private fun jsonPrimitiveBoolean(element: JsonElement?): Boolean? {
    return (element as? JsonPrimitive)?.content?.trim()?.lowercase()?.let { value ->
        when (value) {
            "true" -> true
            "false" -> false
            else -> null
        }
    }
}

@Composable
internal fun ApprovalCardContent(
    approval: PendingToolApproval,
    forgeRuntime: ForgeRuntime,
    meta: com.viant.agentlysdk.ApprovalMeta?,
    selectedFields: Map<String, JsonElement>,
    onEditChange: (String, String, JsonElement) -> Unit,
    onDecision: (PendingToolApproval, String) -> Unit,
    containerColor: Color
) {
    var approvalForgeError by remember(approval.id) { mutableStateOf<String?>(null) }
    Surface(
        color = containerColor,
        shape = MaterialTheme.shapes.medium,
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier.padding(start = 12.dp, top = 12.dp, end = 12.dp, bottom = 20.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            if (meta != null) {
                ApprovalForgeEditors(
                    approvalId = approval.id,
                    forgeRuntime = forgeRuntime,
                    approval = approval,
                    meta = meta,
                    originalArgs = approval.arguments,
                    selectedFields = selectedFields,
                    onEditChange = onEditChange,
                    onAvailabilityChange = { approvalForgeError = it }
                )
            } else {
                Text(
                    approval.title ?: approval.toolName,
                    style = MaterialTheme.typography.titleSmall
                )
                Text(
                    "Tool ${approval.toolName} is waiting for approval.",
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFF667085)
                )
            }
            approvalForgeError?.takeIf { it.isNotBlank() }?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFFB42318)
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(
                    onClick = { onDecision(approval, "approve") },
                    enabled = approvalForgeError.isNullOrBlank()
                ) {
                    Text(meta?.acceptLabel ?: "Approve")
                }
                OutlinedButton(onClick = { onDecision(approval, "reject") }) {
                    Text(meta?.rejectLabel ?: "Reject")
                }
            }
        }
    }
}

@Composable
internal fun ApprovalForgeEditors(
    approvalId: String,
    forgeRuntime: ForgeRuntime? = null,
    approval: PendingToolApproval,
    meta: com.viant.agentlysdk.ApprovalMeta,
    originalArgs: JsonElement? = null,
    selectedFields: Map<String, JsonElement>,
    onEditChange: (String, String, JsonElement) -> Unit,
    onAvailabilityChange: (String?) -> Unit = {}
) {
    val scope = rememberCoroutineScope()
    val configuration = LocalConfiguration.current
    val formFactor = if (configuration.smallestScreenWidthDp >= 600) "tablet" else "phone"
    val localRuntime = remember(approvalId, formFactor) {
        ForgeRuntime(
            endpoints = emptyMap(),
            scope = scope,
            targetContext = buildForgeTargetContext(formFactor)
        )
    }
    val runtime = forgeRuntime ?: localRuntime
    var windowId by remember(approvalId) { mutableStateOf<String?>(null) }
    val dataSourceRef = remember(meta) { approvalDataSourceRef(meta) }
    val requiresSharedRuntime = remember(meta) { approvalRequiresSharedRuntime(meta) }
    val usesRemoteWindow = remember(meta, forgeRuntime) { approvalUsesRemoteWindow(meta, forgeRuntime) }
    val missingRuntimeError = remember(requiresSharedRuntime, forgeRuntime) {
        if (requiresSharedRuntime && forgeRuntime == null) {
            "Forge approval view unavailable: window \"${meta.forge?.windowRef.orEmpty()}\" requires a shared Forge runtime."
        } else {
            null
        }
    }

    DisposableEffect(runtime, windowId) {
        onDispose {
            windowId?.let(runtime::closeWindow)
        }
    }

    LaunchedEffect(missingRuntimeError) {
        onAvailabilityChange(missingRuntimeError)
    }

    LaunchedEffect(approvalId, usesRemoteWindow, runtime, missingRuntimeError) {
        if (missingRuntimeError != null) {
            return@LaunchedEffect
        }
        if (windowId != null) {
            return@LaunchedEffect
        }
        val state = if (usesRemoteWindow) {
            runtime.openWindow(
                windowKey = meta.forge?.windowRef.orEmpty(),
                title = meta.title ?: "Approval",
                inTab = false
            )
        } else {
            runtime.openWindowInline(
                windowKey = "approval-$approvalId",
                title = meta.title ?: "Approval",
                inTab = false,
                metadata = buildApprovalSurfaceWindow(approval, meta)
            )
        }
        windowId = state.windowId
        val formContext = runtime.windowContext(state.windowId).context(dataSourceRef)
        val seed = buildApprovalEditorSeed(meta, selectedFields, originalArgs)
        if (seed.isNotEmpty()) {
            formContext.setForm(seed)
        }
    }

    val activeWindowId = windowId
    if (activeWindowId == null) {
        if (missingRuntimeError == null) {
            CircularProgressIndicator(modifier = Modifier.width(24.dp))
        }
        return
    }

    val metadataSignal = runtime.metadataSignal(activeWindowId)
    val metadata by metadataSignal.flow.collectAsState(initial = null)
    val windowContext = remember(activeWindowId) { runtime.windowContext(activeWindowId) }
    val formContext = remember(activeWindowId, dataSourceRef) { windowContext.context(dataSourceRef) }
    val formState by formContext.form.flow.collectAsState(initial = formContext.peekForm())
    val approvalRenderState = remember(metadata, meta, usesRemoteWindow) {
        resolveApprovalRenderState(metadata, meta, usesRemoteWindow)
    }

    LaunchedEffect(formState) {
        extractApprovalEditedFields(formState).forEach { (key, value) ->
            onEditChange(approvalId, key, approvalValueToJson(value))
        }
    }

    LaunchedEffect(approvalRenderState.error) {
        onAvailabilityChange(approvalRenderState.error)
    }

    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        approvalRenderState.containers.forEach { container ->
            ContainerRenderer(runtime, windowContext, container)
        }
    }
}

internal fun approvalUsesRemoteWindow(
    meta: com.viant.agentlysdk.ApprovalMeta,
    forgeRuntime: ForgeRuntime?
): Boolean {
    return forgeRuntime != null && !meta.forge?.windowRef.isNullOrBlank()
}

internal fun approvalRequiresSharedRuntime(
    meta: com.viant.agentlysdk.ApprovalMeta
): Boolean {
    return !meta.forge?.windowRef.isNullOrBlank()
}

internal data class ApprovalRenderState(
    val containers: List<ContainerDef> = emptyList(),
    val error: String? = null
)

internal fun resolveApprovalRenderState(
    metadata: WindowMetadata?,
    meta: com.viant.agentlysdk.ApprovalMeta,
    usesRemoteWindow: Boolean
): ApprovalRenderState {
    metadata ?: return ApprovalRenderState(
        error = if (usesRemoteWindow) "Forge approval view unavailable: window metadata not loaded." else null
    )
    val targetContainerRef = meta.forge?.containerRef?.trim().orEmpty()
    if (targetContainerRef.isEmpty()) {
        return ApprovalRenderState(containers = metadata.view?.content?.containers.orEmpty())
    }
    val container = resolveApprovalContainer(metadata, targetContainerRef)
    if (container == null) {
        return ApprovalRenderState(
            error = "Forge approval view unavailable: container \"$targetContainerRef\" not found."
        )
    }
    return ApprovalRenderState(containers = listOf(container))
}

private fun resolveApprovalContainer(
    metadata: WindowMetadata,
    containerRef: String
): ContainerDef? {
    metadata.view?.content?.containers?.forEach { container ->
        findApprovalContainer(container, containerRef)?.let { return it }
    }
    metadata.dialogs.forEach { dialog ->
        findApprovalContainer(dialog.content, containerRef)?.let { return it }
    }
    return null
}

private fun findApprovalContainer(
    container: ContainerDef?,
    targetId: String
): ContainerDef? {
    container ?: return null
    if (container.id?.trim() == targetId) {
        return container
    }
    container.containers.forEach { child ->
        findApprovalContainer(child, targetId)?.let { return it }
    }
    return null
}

internal fun buildApprovalEditorWindow(meta: com.viant.agentlysdk.ApprovalMeta): WindowMetadata {
    val dataSourceRef = approvalDataSourceRef(meta)
    return WindowMetadata(
        namespace = "agently.android.approval",
        dataSources = mapOf(
            dataSourceRef to DataSourceDef(selectionMode = "none")
        ),
        view = ViewDef(
            content = ContentDef(
                containers = listOf(
                    buildApprovalEditorContainer(meta)
                )
            )
        )
    )
}

internal fun buildApprovalSurfaceWindow(
    approval: PendingToolApproval,
    meta: com.viant.agentlysdk.ApprovalMeta
): WindowMetadata {
    val dataSourceRef = approvalDataSourceRef(meta)
    return WindowMetadata(
        namespace = "agently.android.approval",
        dataSources = mapOf(
            dataSourceRef to DataSourceDef(selectionMode = "none")
        ),
        view = ViewDef(
            content = ContentDef(
                containers = listOf(
                    ContainerDef(
                        id = "approvalSurface",
                        title = meta.title ?: approval.title ?: approval.toolName,
                        subtitle = meta.message?.takeIf { it.isNotBlank() }
                            ?: "Tool ${approval.toolName} is waiting for approval.",
                        containers = listOf(buildApprovalEditorContainer(meta))
                    )
                )
            )
        )
    )
}

private fun buildApprovalEditorContainer(meta: com.viant.agentlysdk.ApprovalMeta): ContainerDef {
    val useForgeContainer = !meta.forge?.containerRef.isNullOrBlank()
    val containerId = meta.forge?.containerRef?.takeIf { it.isNotBlank() } ?: "approvalEditors"
    val dataSourceRef = approvalDataSourceRef(meta)
    return ContainerDef(
        id = containerId,
        dataSourceRef = dataSourceRef,
        schemaBasedForm = if (useForgeContainer) {
            SchemaBasedFormDef(
                id = "approvalForgeForm",
                dataSourceRef = dataSourceRef,
                schema = buildApprovalForgeSchema(meta),
                showSubmit = false
            )
        } else {
            null
        },
        items = if (useForgeContainer) {
            emptyList()
        } else {
            meta.editors.map { editor ->
                val type = when (editor.kind.lowercase()) {
                    "radio_list" -> "radio"
                    else -> "multiSelect"
                }
                ItemDef(
                    id = editor.name,
                    dataField = editor.name,
                    label = editor.label ?: editor.name,
                    type = type,
                    options = editor.options.map { option ->
                        OptionDef(
                            value = option.id,
                            label = option.label,
                            default = option.selected
                        )
                    }
                )
            }
        }
    )
}

internal fun buildApprovalEditorSeed(
    meta: com.viant.agentlysdk.ApprovalMeta,
    selectedFields: Map<String, JsonElement>,
    originalArgs: JsonElement? = null
): Map<String, Any?> {
    val seeded = linkedMapOf<String, Any?>()
    seeded["approvalSchemaJSON"] = buildApprovalForgeSchema(meta).toString()
    seeded["approval"] = jsonElementToApprovalValue(approvalStateJson.encodeToJsonElement(meta))
    if (originalArgs != null && originalArgs !is JsonNull) {
        seeded["originalArgs"] = jsonElementToApprovalValue(originalArgs)
    }
    val editedFields = linkedMapOf<String, Any?>()
    meta.editors.forEach { editor ->
        val existing = selectedFields[editor.name]
        if (existing != null && existing !is JsonNull) {
            val value = jsonElementToApprovalValue(existing)
            seeded[editor.name] = value
            editedFields[editor.name] = value
            return@forEach
        }
        val defaults = editor.options.filter { it.selected }.map { it.id }
        if (defaults.isEmpty()) {
            return@forEach
        }
        val value: Any = if (editor.kind.equals("radio_list", ignoreCase = true)) {
            defaults.first()
        } else {
            defaults
        }
        seeded[editor.name] = value
        editedFields[editor.name] = value
    }
    if (editedFields.isNotEmpty()) {
        seeded["editedFields"] = editedFields
    }
    return seeded
}

internal fun extractApprovalEditedFields(formState: Map<String, Any?>): Map<String, Any?> {
    val explicitEditedFields = (formState["editedFields"] as? Map<*, *>)
        ?.entries
        ?.mapNotNull { (key, value) -> (key as? String)?.let { it to value } }
        ?.toMap()
        .orEmpty()
    val liveFields = formState.filterKeys { key -> key !in approvalReservedFormKeys }
    return when {
        explicitEditedFields.isEmpty() -> liveFields
        liveFields.isEmpty() -> explicitEditedFields
        else -> explicitEditedFields + liveFields
    }
}

internal fun approvalDataSourceRef(meta: com.viant.agentlysdk.ApprovalMeta): String {
    return meta.forge?.dataSource?.trim().takeUnless { it.isNullOrBlank() } ?: APPROVAL_FORM_DATA_SOURCE
}

private fun buildApprovalForgeSchema(meta: com.viant.agentlysdk.ApprovalMeta): JsonObject {
    val properties = linkedMapOf<String, JsonElement>()
    meta.editors.forEach { editor ->
        val values = editor.options.map { JsonPrimitive(it.id) }
        val defaults = editor.options.filter { it.selected }.map { JsonPrimitive(it.id) }
        val widget = if (editor.kind.equals("radio_list", ignoreCase = true)) "radio" else "multiSelect"
        val property = linkedMapOf<String, JsonElement>(
            "type" to JsonPrimitive(if (widget == "radio") "string" else "array"),
            "title" to JsonPrimitive(editor.label ?: editor.name),
            "description" to JsonPrimitive(editor.description ?: ""),
            "enum" to JsonArray(values),
            "x-ui-widget" to JsonPrimitive(widget)
        )
        if (widget == "radio") {
            property["default"] = defaults.firstOrNull() ?: JsonPrimitive("")
        } else {
            property["default"] = JsonArray(defaults)
        }
        properties[editor.name] = JsonObject(property)
    }
    return JsonObject(
        mapOf(
            "type" to JsonPrimitive("object"),
            "properties" to JsonObject(properties),
            "required" to JsonArray(emptyList())
        )
    )
}

private fun jsonElementToApprovalValue(value: JsonElement): Any? {
    return when (value) {
        is JsonPrimitive -> value.content
        is JsonArray -> value.mapNotNull { (it as? JsonPrimitive)?.content }
        is JsonObject -> value.mapValues { (_, nested) ->
            when (nested) {
                is JsonPrimitive -> nested.content
                is JsonArray -> nested.map { entry ->
                    (entry as? JsonPrimitive)?.content ?: entry.toString()
                }
                is JsonObject -> jsonElementToApprovalValue(nested)
                else -> nested.toString()
            }
        }
        else -> value.toString()
    }
}

private fun approvalValueToJson(value: Any?): JsonElement {
    return when (value) {
        null -> JsonNull
        is JsonElement -> value
        is Collection<*> -> buildJsonArray {
            value.forEach { entry -> add(JsonPrimitive(entry?.toString() ?: "")) }
        }
        else -> JsonPrimitive(value.toString())
    }
}
