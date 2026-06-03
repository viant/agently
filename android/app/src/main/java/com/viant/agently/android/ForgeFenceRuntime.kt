package com.viant.agently.android

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.viant.forgeandroid.runtime.ChartAxisDef
import com.viant.forgeandroid.runtime.ChartDef
import com.viant.forgeandroid.runtime.ChartSeriesDef
import com.viant.forgeandroid.runtime.ChartValueOption
import com.viant.forgeandroid.runtime.ColumnDef
import com.viant.forgeandroid.runtime.ContainerDef
import com.viant.forgeandroid.runtime.DashboardMetricDef
import com.viant.forgeandroid.runtime.DataSourceDef
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.JsonUtil
import com.viant.forgeandroid.runtime.LinkDef
import com.viant.forgeandroid.runtime.SelectorDef
import com.viant.forgeandroid.runtime.TableDef
import com.viant.forgeandroid.runtime.ViewDef
import com.viant.forgeandroid.runtime.WindowContext
import com.viant.forgeandroid.runtime.WindowMetadata
import com.viant.forgeandroid.ui.ContainerRenderer
import com.viant.forgeandroid.ui.MarkdownRenderer
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

private const val FORGE_UI_FENCE = "forge-ui"
private const val FORGE_DATA_FENCE = "forge-data"

internal val forgeFenceJson = Json { ignoreUnknownKeys = true }

@Serializable
internal data class ForgeDataFenceBlock(
    val version: Int? = 1,
    val id: String? = null,
    val format: String? = null,
    val mode: String? = null,
    val data: JsonElement? = null
)

@Serializable
internal data class ForgeUiFencePayload(
    val version: Int? = 1,
    val title: String? = null,
    val subtitle: String? = null,
    val blocks: List<JsonObject> = emptyList()
)

internal data class MaterializedForgeDataBlock(
    val id: String,
    val rows: Any?
)

internal sealed interface TranscriptContentPart {
    data class Markdown(val text: String) : TranscriptContentPart
    data class ForgeUi(
        val payload: ForgeUiFencePayload,
        val dataStore: Map<String, MaterializedForgeDataBlock>
    ) : TranscriptContentPart
}

private data class FenceMatch(
    val rangeStart: Int,
    val rangeEndExclusive: Int,
    val kind: String,
    val body: String
)

@Composable
internal fun TranscriptMessageContent(
    markdown: String,
    forgeRuntime: ForgeRuntime,
    messageKey: String
) {
    val parts = remember(markdown) { parseTranscriptContentParts(markdown) }
    if (parts.isEmpty()) {
        MarkdownRenderer(markdown = markdown.ifBlank { "(empty response)" }, modifier = Modifier.fillMaxWidth())
        return
    }
    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        parts.forEachIndexed { index, part ->
            when (part) {
                is TranscriptContentPart.Markdown -> {
                    if (part.text.isNotBlank()) {
                        MarkdownRenderer(markdown = part.text, modifier = Modifier.fillMaxWidth())
                    }
                }

                is TranscriptContentPart.ForgeUi -> {
                    TranscriptForgeUiBlock(
                        messageKey = "$messageKey-$index",
                        payload = part.payload,
                        dataStore = part.dataStore,
                        forgeRuntime = forgeRuntime
                    )
                }
            }
        }
    }
}

@Composable
private fun TranscriptForgeUiBlock(
    messageKey: String,
    payload: ForgeUiFencePayload,
    dataStore: Map<String, MaterializedForgeDataBlock>,
    forgeRuntime: ForgeRuntime
) {
    val metadataResult = remember(payload, dataStore) {
        runCatching { buildTranscriptForgeWindowMetadata(payload, dataStore) }
    }
    val inlineMetadata = metadataResult.getOrNull()
    if (inlineMetadata == null) {
        TranscriptForgeFallback(
            title = payload.title?.takeIf { it.isNotBlank() } ?: "Forge content",
            body = metadataResult.exceptionOrNull()?.message ?: "Unable to decode forge-ui block."
        )
        return
    }

    var windowId by remember(messageKey) { mutableStateOf<String?>(null) }

    LaunchedEffect(messageKey, inlineMetadata) {
        val state = forgeRuntime.openWindowInline(
            windowKey = "transcript-forge-$messageKey",
            title = payload.title ?: "Forge content",
            metadata = inlineMetadata
        )
        hydrateTranscriptForgeDataSources(forgeRuntime, state.windowId, dataStore)
        windowId = state.windowId
    }

    val activeWindowId = windowId
    val metadataSignal = remember(activeWindowId) {
        activeWindowId?.let { forgeRuntime.metadataSignal(it) }
    }
    val resolvedMetadata by if (metadataSignal != null) {
        metadataSignal.flow.collectAsState(initial = metadataSignal.peek())
    } else {
        remember { mutableStateOf<WindowMetadata?>(null) }
    }
    val windowContext = remember(activeWindowId) {
        activeWindowId?.let { forgeRuntime.windowContext(it) }
    }

    if (resolvedMetadata == null || windowContext == null) {
        TranscriptForgeFallback(
            title = payload.title?.takeIf { it.isNotBlank() } ?: "Forge content",
            body = "Loading interactive content…"
        )
        return
    }

    Surface(
        color = Color(0xFFF8FAFD),
        shape = MaterialTheme.shapes.large,
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier.padding(vertical = 4.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            resolvedMetadata?.view?.content?.containers.orEmpty().forEach { container ->
                ContainerRenderer(forgeRuntime, windowContext, container)
            }
        }
    }
}

@Composable
private fun TranscriptForgeFallback(
    title: String,
    body: String
) {
    Surface(
        color = Color(0xFFF8FAFD),
        shape = MaterialTheme.shapes.large,
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = title,
                style = MaterialTheme.typography.titleSmall,
                fontWeight = FontWeight.SemiBold
            )
            Text(
                text = body,
                style = MaterialTheme.typography.bodySmall,
                color = Color(0xFF667085)
            )
        }
    }
}

internal fun parseTranscriptContentParts(markdown: String): List<TranscriptContentPart> {
    if (markdown.isBlank()) {
        return emptyList()
    }
    val matches = findForgeFenceMatches(markdown)
    if (matches.isEmpty()) {
        return listOf(TranscriptContentPart.Markdown(markdown))
    }
    val result = mutableListOf<TranscriptContentPart>()
    val dataBlocks = mutableListOf<ForgeDataFenceBlock>()
    var cursor = 0
    matches.forEach { match ->
        if (match.rangeStart > cursor) {
            appendMarkdownPart(result, markdown.substring(cursor, match.rangeStart))
        }
        val rawFence = markdown.substring(match.rangeStart, match.rangeEndExclusive)
        when (match.kind) {
            FORGE_DATA_FENCE -> {
                val parsed = parseForgeDataBlock(match.body)
                if (parsed?.id.isNullOrBlank()) {
                    appendMarkdownPart(result, rawFence)
                } else {
                    dataBlocks += parsed!!
                }
            }

            FORGE_UI_FENCE -> {
                val parsed = parseForgeUiPayload(match.body)
                if (parsed == null) {
                    appendMarkdownPart(result, rawFence)
                } else {
                    result += TranscriptContentPart.ForgeUi(
                        payload = parsed.copy(
                            blocks = parsed.blocks.mapIndexed { index, block ->
                                if ("id" in block) {
                                    block
                                } else {
                                    JsonObject(
                                        block.toMutableMap().apply {
                                            put("id", JsonPrimitive("block-${index + 1}"))
                                        }
                                    )
                                }
                            }
                        ),
                        dataStore = materializeForgeDataBlocks(dataBlocks)
                    )
                }
            }
        }
        cursor = match.rangeEndExclusive
    }
    if (cursor < markdown.length) {
        appendMarkdownPart(result, markdown.substring(cursor))
    }
    return result.ifEmpty { listOf(TranscriptContentPart.Markdown(markdown)) }
}

private fun parseForgeDataBlock(body: String): ForgeDataFenceBlock? {
    val jsonObject = parseJsonObject(body) ?: return null
    return ForgeDataFenceBlock(
        version = jsonInt(jsonObject["version"]) ?: 1,
        id = jsonString(jsonObject["id"]).takeIf { it.isNotBlank() },
        format = jsonString(jsonObject["format"]).takeIf { it.isNotBlank() },
        mode = jsonString(jsonObject["mode"]).takeIf { it.isNotBlank() },
        data = jsonObject["data"]
    )
}

private fun parseForgeUiPayload(body: String): ForgeUiFencePayload? {
    val jsonObject = parseJsonObject(body) ?: return null
    return ForgeUiFencePayload(
        version = jsonInt(jsonObject["version"]) ?: 1,
        title = jsonString(jsonObject["title"]).takeIf { it.isNotBlank() },
        subtitle = jsonString(jsonObject["subtitle"]).takeIf { it.isNotBlank() },
        blocks = (jsonObject["blocks"] as? JsonArray).orEmpty().mapNotNull { it as? JsonObject }
    )
}

private fun parseJsonObject(body: String): JsonObject? {
    return runCatching {
        forgeFenceJson.parseToJsonElement(body) as? JsonObject
    }.getOrNull()
}

private fun appendMarkdownPart(parts: MutableList<TranscriptContentPart>, text: String) {
    if (text.isBlank()) {
        return
    }
    val previous = parts.lastOrNull()
    if (previous is TranscriptContentPart.Markdown) {
        parts[parts.lastIndex] = previous.copy(text = previous.text + text)
    } else {
        parts += TranscriptContentPart.Markdown(text)
    }
}

private fun findForgeFenceMatches(markdown: String): List<FenceMatch> {
    val pattern = Regex("```(forge-data|forge-ui)(?:\\r?\\n|(?=[\\[{]))(.*?)```", setOf(RegexOption.IGNORE_CASE, RegexOption.DOT_MATCHES_ALL))
    return pattern.findAll(markdown).map { match ->
        FenceMatch(
            rangeStart = match.range.first,
            rangeEndExclusive = match.range.last + 1,
            kind = match.groupValues[1].trim().lowercase(),
            body = match.groupValues[2].trim()
        )
    }.toList()
}

internal fun materializeForgeDataBlocks(blocks: List<ForgeDataFenceBlock>): Map<String, MaterializedForgeDataBlock> {
    val store = linkedMapOf<String, MaterializedForgeDataBlock>()
    blocks.forEach { block ->
        val id = block.id?.trim().orEmpty()
        if (id.isBlank()) {
            return@forEach
        }
        val materialized = MaterializedForgeDataBlock(
            id = id,
            rows = materializeForgeData(block)
        )
        val mode = block.mode?.trim()?.lowercase().orEmpty().ifBlank { "replace" }
        val current = store[id]
        store[id] = when {
            current == null || mode == "replace" -> materialized
            mode == "append" -> MaterializedForgeDataBlock(id = id, rows = appendForgeRows(current.rows, materialized.rows))
            mode == "patch" -> MaterializedForgeDataBlock(id = id, rows = patchForgeRows(current.rows, materialized.rows))
            else -> materialized
        }
    }
    return store
}

private fun materializeForgeData(block: ForgeDataFenceBlock): Any? {
    val format = block.format?.trim()?.lowercase().orEmpty().ifBlank {
        if (block.data is JsonPrimitive) "csv" else "json"
    }
    return when (format) {
        "csv" -> parseCsvRows((block.data as? JsonPrimitive)?.content.orEmpty())
        else -> block.data?.let(JsonUtil::elementToAny)
    }
}

private fun appendForgeRows(current: Any?, next: Any?): Any? {
    val currentRows = current as? List<*> ?: return next
    val nextRows = next as? List<*> ?: return next
    return currentRows + nextRows
}

private fun patchForgeRows(current: Any?, next: Any?): Any? {
    val currentMap = current as? Map<*, *> ?: return next
    val nextMap = next as? Map<*, *> ?: return next
    return currentMap.toMutableMap().apply { putAll(nextMap) }
}

private fun parseCsvRows(text: String): List<Map<String, Any?>> {
    val lines = text.trim().lines().map(String::trim).filter(String::isNotBlank)
    if (lines.isEmpty()) {
        return emptyList()
    }
    val headers = splitCsvLine(lines.first())
    return lines.drop(1).map { line ->
        val cells = splitCsvLine(line)
        buildMap {
            headers.forEachIndexed { index, header ->
                put(header, autoCsvValue(cells.getOrElse(index) { "" }))
            }
        }
    }
}

private fun splitCsvLine(line: String): List<String> {
    val cells = mutableListOf<String>()
    val current = StringBuilder()
    var inQuotes = false
    var index = 0
    while (index < line.length) {
        val char = line[index]
        val next = line.getOrNull(index + 1)
        when {
            char == '"' && inQuotes && next == '"' -> {
                current.append('"')
                index += 2
            }

            char == '"' -> {
                inQuotes = !inQuotes
                index += 1
            }

            char == ',' && !inQuotes -> {
                cells += current.toString().trim()
                current.clear()
                index += 1
            }

            else -> {
                current.append(char)
                index += 1
            }
        }
    }
    cells += current.toString().trim()
    return cells
}

private fun autoCsvValue(raw: String): Any {
    val text = raw.trim()
    return when {
        text.equals("true", ignoreCase = true) -> true
        text.equals("false", ignoreCase = true) -> false
        text.toLongOrNull() != null -> text.toLong()
        text.toDoubleOrNull() != null -> text.toDouble()
        else -> text
    }
}

internal fun buildTranscriptForgeWindowMetadata(
    payload: ForgeUiFencePayload,
    dataStore: Map<String, MaterializedForgeDataBlock>
): WindowMetadata {
    val normalizedDataStore = buildNormalizedTranscriptDataStore(payload, dataStore)
    val dataSources = buildTranscriptForgeDataSources(payload, normalizedDataStore)
    val containers = payload.blocks.mapIndexedNotNull { index, block ->
        adaptTranscriptForgeBlock(block, index)
    }
    require(containers.isNotEmpty()) { "forge-ui block does not contain any renderable blocks" }
    return WindowMetadata(
        namespace = "agently.android.transcript",
        dataSources = dataSources,
        view = ViewDef(
            content = com.viant.forgeandroid.runtime.ContentDef(
                containers = listOf(
                    ContainerDef(
                        id = "forge-root",
                        title = payload.title?.takeIf { it.isNotBlank() },
                        subtitle = payload.subtitle?.takeIf { it.isNotBlank() },
                        containers = containers
                    )
                )
            )
        )
    )
}

private fun buildTranscriptForgeDataSources(
    payload: ForgeUiFencePayload,
    dataStore: Map<String, MaterializedForgeDataBlock>
): Map<String, DataSourceDef> {
    val refs = linkedSetOf<String>()
    refs += dataStore.keys
    payload.blocks.forEach { block ->
        jsonString(block["dataSourceRef"]).takeIf { it.isNotBlank() }?.let(refs::add)
        jsonString(block["dataSource"]).takeIf { it.isNotBlank() }?.let(refs::add)
        syntheticTranscriptDataSourceRef(block).takeIf { it.isNotBlank() }?.let(refs::add)
    }
    return refs.associateWith { ref ->
        val selectionMode = when {
            payload.blocks.any { block ->
                jsonString(block["kind"]) == "planner.table" &&
                    resolveTranscriptDataSourceRef(block) == ref
            } -> "single"

            else -> "none"
        }
        DataSourceDef(
            selectionMode = selectionMode,
            selectors = SelectorDef(data = "output")
        )
    }
}

private fun adaptTranscriptForgeBlock(block: JsonObject, index: Int): ContainerDef? {
    return when (jsonString(block["kind"])) {
        "planner.table" -> adaptPlannerTableBlock(block, index)
        "dashboard.table" -> adaptDashboardTableBlock(block, index)
        "dashboard.summary" -> adaptDashboardSummaryBlock(block, index)
        "dashboard.report" -> adaptDashboardReportBlock(block, index)
        "dashboard.kpiTable" -> adaptDashboardKpiTableBlock(block, index)
        "dashboard.dimensions" -> adaptDashboardDimensionsBlock(block, index)
        "dashboard.messages" -> adaptDashboardMessagesBlock(block, index)
        "dashboard.timeline" -> adaptDashboardTimelineBlock(block, index)
        else -> runCatching {
            forgeFenceJson.decodeFromString(ContainerDef.serializer(), block.toString())
        }.getOrNull()
    }
}

private fun adaptPlannerTableBlock(block: JsonObject, index: Int): ContainerDef {
    val dataSourceRef = resolveTranscriptDataSourceRef(block)
    val selectionField = jsonString((block["selection"] as? JsonObject)?.get("field"))
    val columns = mutableListOf<ColumnDef>()
    if (selectionField.isNotBlank()) {
        columns += ColumnDef(
            id = selectionField,
            name = selectionField,
            label = titleizeKey(selectionField)
        )
    }
    val declaredColumns = (block["columns"] as? JsonArray).orEmpty().mapNotNull { entry ->
        val value = entry as? JsonObject ?: return@mapNotNull null
        val key = jsonString(value["key"]).ifBlank { jsonString(value["id"]) }
        if (key.isBlank()) {
            return@mapNotNull null
        }
        ColumnDef(
            id = key,
            name = key,
            label = jsonString(value["label"]).ifBlank { titleizeKey(key) },
            format = jsonString(value["format"]).takeIf { it.isNotBlank() },
            type = jsonString(value["type"]).takeIf { it.isNotBlank() },
            link = (value["link"] as? JsonObject)?.let { linkObject ->
                LinkDef(href = jsonString(linkObject["href"]).takeIf { it.isNotBlank() })
            }
        )
    }
    declaredColumns.forEach { column ->
        if (columns.none { it.id == column.id }) {
            columns += column
        }
    }
    return ContainerDef(
        id = jsonString(block["id"]).ifBlank { "planner-table-$index" },
        title = jsonString(block["title"]).takeIf { it.isNotBlank() } ?: "Planner table",
        subtitle = jsonString(block["subtitle"]).takeIf { it.isNotBlank() },
        dataSourceRef = dataSourceRef,
        table = TableDef(columns = columns)
    )
}

private fun adaptDashboardSummaryBlock(block: JsonObject, index: Int): ContainerDef {
    val metrics = summaryMetrics(block)
    return ContainerDef(
        id = jsonString(block["id"]).ifBlank { "dashboard-summary-$index" },
        kind = "dashboard.summary",
        title = jsonString(block["title"]).takeIf { it.isNotBlank() },
        subtitle = jsonString(block["subtitle"]).takeIf { it.isNotBlank() },
        dataSourceRef = resolveTranscriptDataSourceRef(block),
        metrics = metrics
    )
}

private fun summaryMetrics(block: JsonObject): List<DashboardMetricDef> {
    val declared = (block["metrics"] as? JsonArray).orEmpty().mapNotNull { entry ->
        when (entry) {
            is JsonPrimitive -> {
                val selector = entry.content.trim()
                selector.takeIf { it.isNotBlank() }?.let {
                    DashboardMetricDef(
                        id = it,
                        label = titleizeKey(it),
                        selector = it
                    )
                }
            }

            is JsonObject -> {
                val selector = jsonString(entry["selector"]).ifBlank { jsonString(entry["key"]) }
                DashboardMetricDef(
                    id = jsonString(entry["id"]).ifBlank { selector },
                    label = jsonString(entry["label"]).ifBlank { titleizeKey(selector) },
                    selector = selector,
                    format = jsonString(entry["format"]).takeIf { it.isNotBlank() }
                )
            }

            else -> null
        }
    }
    if (declared.isNotEmpty()) {
        return declared
    }
    return (block["items"] as? JsonArray).orEmpty().mapNotNull { entry ->
        val item = entry as? JsonObject ?: return@mapNotNull null
        val rawKey = jsonString(item["key"]).ifBlank {
            jsonString(item["label"]).ifBlank { jsonString(item["id"]).ifBlank { "value" } }
        }
        val key = transcriptFieldKey(rawKey)
        DashboardMetricDef(
            id = key,
            label = jsonString(item["label"]).ifBlank { titleizeKey(key) },
            selector = key,
            format = jsonString(item["format"]).takeIf { it.isNotBlank() }
        )
    }
}

private fun adaptDashboardReportBlock(block: JsonObject, index: Int): ContainerDef {
    val sections = (block["sections"] as? JsonArray).orEmpty().mapNotNull { entry ->
        val section = entry as? JsonObject ?: return@mapNotNull null
        DashboardReportSectionDef(
            id = jsonString(section["id"]).ifBlank { null },
            title = jsonString(section["title"]).ifBlank { null },
            body = (section["body"] as? JsonArray).orEmpty()
                .mapNotNull { (it as? JsonPrimitive)?.contentOrNull?.trim()?.takeIf(String::isNotBlank) },
            tone = jsonString(section["tone"]).ifBlank { null }
        )
    }
    return ContainerDef(
        id = jsonString(block["id"]).ifBlank { "dashboard-report-$index" },
        kind = "dashboard.report",
        title = jsonString(block["title"]).takeIf { it.isNotBlank() },
        subtitle = jsonString(block["subtitle"]).takeIf { it.isNotBlank() },
        sections = sections
    )
}

private fun adaptDashboardKpiTableBlock(block: JsonObject, index: Int): ContainerDef {
    val columns = (block["columns"] as? JsonArray).orEmpty().mapNotNull { entry ->
        val value = entry as? JsonObject ?: return@mapNotNull null
        val key = jsonString(value["key"]).ifBlank { jsonString(value["id"]) }
        if (key.isBlank()) return@mapNotNull null
        ColumnDef(
            id = key,
            name = key,
            label = jsonString(value["label"]).ifBlank { titleizeKey(key) },
            format = jsonString(value["format"]).takeIf { it.isNotBlank() },
            type = jsonString(value["type"]).takeIf { it.isNotBlank() },
            link = (value["link"] as? JsonObject)?.let { linkObject ->
                LinkDef(href = jsonString(linkObject["href"]).takeIf { it.isNotBlank() })
            }
        )
    }
    return ContainerDef(
        id = jsonString(block["id"]).ifBlank { "dashboard-kpi-table-$index" },
        title = jsonString(block["title"]).takeIf { it.isNotBlank() } ?: "KPI table",
        subtitle = jsonString(block["subtitle"]).takeIf { it.isNotBlank() },
        dataSourceRef = resolveTranscriptDataSourceRef(block),
        table = TableDef(columns = columns)
    )
}

private fun adaptDashboardTableBlock(block: JsonObject, index: Int): ContainerDef {
    val columns = (block["columns"] as? JsonArray).orEmpty().mapNotNull { entry ->
        val value = entry as? JsonObject ?: return@mapNotNull null
        val key = jsonString(value["key"]).ifBlank { jsonString(value["id"]) }
        if (key.isBlank()) return@mapNotNull null
        ColumnDef(
            id = key,
            name = key,
            label = jsonString(value["label"]).ifBlank { titleizeKey(key) }
        )
    }
    return ContainerDef(
        id = jsonString(block["id"]).ifBlank { "dashboard-table-$index" },
        title = jsonString(block["title"]).takeIf { it.isNotBlank() } ?: "Table",
        subtitle = jsonString(block["subtitle"]).takeIf { it.isNotBlank() },
        dataSourceRef = resolveTranscriptDataSourceRef(block),
        table = TableDef(columns = columns)
    )
}

private fun adaptDashboardDimensionsBlock(block: JsonObject, index: Int): ContainerDef {
    val dimension = (block["dimension"] as? JsonObject)?.let { field ->
        DashboardFieldDef(
            key = jsonString(field["key"]).takeIf { it.isNotBlank() },
            label = jsonString(field["label"]).takeIf { it.isNotBlank() },
            format = jsonString(field["format"]).takeIf { it.isNotBlank() }
        )
    }
    val metric = (block["metric"] as? JsonObject)?.let { field ->
        DashboardFieldDef(
            key = jsonString(field["key"]).takeIf { it.isNotBlank() },
            label = jsonString(field["label"]).takeIf { it.isNotBlank() },
            format = jsonString(field["format"]).takeIf { it.isNotBlank() }
        )
    }
    return ContainerDef(
        id = jsonString(block["id"]).ifBlank { "dashboard-dimensions-$index" },
        kind = "dashboard.dimensions",
        title = jsonString(block["title"]).takeIf { it.isNotBlank() },
        subtitle = jsonString(block["subtitle"]).takeIf { it.isNotBlank() },
        dataSourceRef = resolveTranscriptDataSourceRef(block),
        dimension = dimension,
        metric = metric,
        viewModes = (block["viewModes"] as? JsonArray).orEmpty().mapNotNull { (it as? JsonPrimitive)?.contentOrNull?.trim()?.takeIf(String::isNotBlank) },
        limit = (block["limit"] as? JsonPrimitive)?.intOrNull,
        orderBy = jsonString(block["orderBy"]).takeIf { it.isNotBlank() }
    )
}

private fun adaptDashboardMessagesBlock(block: JsonObject, index: Int): ContainerDef {
    val items = (block["items"] as? JsonArray).orEmpty().mapNotNull { entry ->
        val item = entry as? JsonObject ?: return@mapNotNull null
        ItemDef(
            id = jsonString(item["id"]).takeIf { it.isNotBlank() },
            label = jsonString(item["label"]).takeIf { it.isNotBlank() },
            title = jsonString(item["title"]).takeIf { it.isNotBlank() },
            body = jsonString(item["body"]).takeIf { it.isNotBlank() },
            severity = jsonString(item["severity"]).takeIf { it.isNotBlank() }
        )
    }
    return ContainerDef(
        id = jsonString(block["id"]).ifBlank { "dashboard-messages-$index" },
        kind = "dashboard.messages",
        title = jsonString(block["title"]).takeIf { it.isNotBlank() },
        subtitle = jsonString(block["subtitle"]).takeIf { it.isNotBlank() },
        items = items
    )
}

private fun adaptDashboardTimelineBlock(block: JsonObject, index: Int): ContainerDef {
    val dataSourceRef = resolveTranscriptDataSourceRef(block)
    val chartType = jsonString(block["chartType"]).ifBlank { "bar" }
    val categoryKey = jsonString(block["dateField"])
        .ifBlank { jsonString(block["timeColumn"]) }
        .ifBlank { jsonString(block["groupBy"]) }
        .ifBlank { jsonString(block["seriesColumn"]) }
        .ifBlank { "label" }
    val seriesKeys = (block["series"] as? JsonArray).orEmpty()
        .mapNotNull { entry ->
            when (entry) {
                is JsonPrimitive -> entry.content.trim().takeIf(String::isNotBlank)
                is JsonObject -> jsonString(entry["key"]).ifBlank {
                    jsonString(entry["id"]).ifBlank { jsonString(entry["value"]) }
                }.takeIf { it.isNotBlank() }
                else -> null
            }
        }
    val valueKey = seriesKeys.firstOrNull()
        ?: jsonString(block["valueColumn"]).takeIf { it.isNotBlank() }
        ?: "value"

    return ContainerDef(
        id = jsonString(block["id"]).ifBlank { "dashboard-timeline-$index" },
        kind = "dashboard.timeline",
        title = jsonString(block["title"]).takeIf { it.isNotBlank() },
        subtitle = jsonString(block["subtitle"]).takeIf { it.isNotBlank() },
        dataSourceRef = dataSourceRef,
        chart = ChartDef(
            type = chartType,
            xAxis = ChartAxisDef(
                dataKey = categoryKey,
                label = titleizeKey(categoryKey)
            ),
            yAxis = ChartAxisDef(
                dataKey = valueKey,
                label = titleizeKey(valueKey)
            ),
            series = ChartSeriesDef(
                nameKey = categoryKey,
                valueKey = valueKey,
                values = listOf(
                    ChartValueOption(
                        name = titleizeKey(valueKey),
                        value = valueKey
                    )
                )
            )
        )
    )
}

private fun resolveTranscriptDataSourceRef(block: JsonObject): String {
    return jsonString(block["dataSourceRef"]).ifBlank {
        jsonString(block["dataSource"]).ifBlank { syntheticTranscriptDataSourceRef(block) }
    }
}

private fun buildNormalizedTranscriptDataStore(
    payload: ForgeUiFencePayload,
    dataStore: Map<String, MaterializedForgeDataBlock>
): Map<String, MaterializedForgeDataBlock> {
    val normalized = linkedMapOf<String, MaterializedForgeDataBlock>()
    normalized.putAll(dataStore)
    payload.blocks.forEach { block ->
        synthesizeTranscriptDataBlock(block)?.let { normalized[it.id] = it }
    }
    return normalized
}

private fun synthesizeTranscriptDataBlock(block: JsonObject): MaterializedForgeDataBlock? {
    if (jsonString(block["kind"]) != "dashboard.summary") {
        return null
    }
    val dataSourceRef = syntheticTranscriptDataSourceRef(block)
    val row = linkedMapOf<String, Any?>()
    (block["items"] as? JsonArray).orEmpty().forEach { entry ->
        val item = entry as? JsonObject ?: return@forEach
        val rawKey = jsonString(item["key"]).ifBlank {
            jsonString(item["label"]).ifBlank { jsonString(item["id"]).ifBlank { "value" } }
        }
        row[transcriptFieldKey(rawKey)] = item["value"]?.let(JsonUtil::elementToAny)
    }
    if (row.isEmpty()) {
        return null
    }
    return MaterializedForgeDataBlock(
        id = dataSourceRef,
        rows = listOf(row)
    )
}

private fun syntheticTranscriptDataSourceRef(block: JsonObject): String {
    val base = jsonString(block["id"]).ifBlank {
        jsonString(block["title"]).ifBlank { "transcript-block" }
    }
    return "inline_${transcriptFieldKey(base)}"
}

private fun transcriptFieldKey(value: String): String {
    val normalized = value.trim()
        .replace(Regex("[^A-Za-z0-9]+"), "_")
        .trim('_')
        .lowercase()
    if (normalized.isBlank()) {
        return "value"
    }
    val parts = normalized.split('_').filter { it.isNotBlank() }
    return parts.first() + parts.drop(1).joinToString("") { it.replaceFirstChar(Char::titlecase) }
}

private fun titleizeKey(value: String): String {
    return value
        .replace(Regex("([a-z0-9])([A-Z])"), "$1 $2")
        .replace('_', ' ')
        .replace('-', ' ')
        .trim()
        .split(Regex("\\s+"))
        .filter { it.isNotBlank() }
        .joinToString(" ") { token ->
            token.lowercase().replaceFirstChar { it.titlecase() }
        }
}

private fun jsonString(element: JsonElement?): String {
    return (element as? JsonPrimitive)?.content?.trim().orEmpty()
}

private fun jsonInt(element: JsonElement?): Int? {
    return (element as? JsonPrimitive)?.content?.trim()?.toIntOrNull()
}

private fun hydrateTranscriptForgeDataSources(
    forgeRuntime: ForgeRuntime,
    windowId: String,
    dataStore: Map<String, MaterializedForgeDataBlock>
) {
    val windowContext = forgeRuntime.windowContext(windowId)
    dataStore.forEach { (dataSourceRef, block) ->
        val context = windowContext.contextOrNull(dataSourceRef) ?: return@forEach
        val rows = when (val value = block.rows) {
            is List<*> -> value.filterIsInstance<Map<String, Any?>>()
            is Map<*, *> -> listOf(value.entries.associate { it.key.toString() to it.value })
            else -> emptyList()
        }
        context.collection.set(rows)
        context.control.set(context.control.peek().copy(loading = false, error = null))
        if (rows.size == 1) {
            val row = rows.first()
            context.metrics.set(row)
            if (context.peekSelection().selected == null) {
                context.setForm(row)
            }
        }
    }
}
