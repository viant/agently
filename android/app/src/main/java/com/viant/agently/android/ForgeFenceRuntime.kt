package com.viant.agently.android

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBarsPadding
import androidx.compose.foundation.clickable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.OpenInFull
import androidx.compose.material.icons.outlined.PictureAsPdf
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
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
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.InlineReportRuntimeCompiler
import com.viant.forgeandroid.runtime.WindowContext
import com.viant.forgeandroid.runtime.WindowMetadata
import com.viant.forgeandroid.ui.ContainerRenderer
import com.viant.forgeandroid.ui.MarkdownRenderer
import com.viant.forgeandroid.ui.TranscriptEnvelope as ForgeTranscriptEnvelope
import com.viant.forgeandroid.ui.TranscriptEnvelopePart as ForgeTranscriptEnvelopePart
import com.viant.forgeandroid.ui.TranscriptCanonicalData as ForgeTranscriptCanonicalData
import com.viant.forgeandroid.ui.TranscriptCanonicalPart as ForgeTranscriptCanonicalPart
import com.viant.forgeandroid.ui.TranscriptCanonicalReport as ForgeTranscriptCanonicalReport
import com.viant.forgeandroid.ui.TranscriptForgeDataBlock as ForgeTranscriptDataBlock
import com.viant.forgeandroid.ui.TranscriptForgeDataStore as ForgeTranscriptDataStore
import com.viant.forgeandroid.ui.TranscriptForgeUiPayload as ForgeTranscriptUiPayload
import com.viant.forgeandroid.ui.WindowContentView
import com.viant.forgeandroid.ui.buildTranscriptWindowMetadata as forgeBuildTranscriptWindowMetadata
import com.viant.forgeandroid.ui.buildTranscriptWindowPresentation as forgeBuildTranscriptWindowPresentation
import com.viant.forgeandroid.ui.rememberTranscriptInlinePresentation
import com.viant.agentlysdk.RenderedContentPart
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.FetchDatasourceInput
import com.viant.agentlysdk.fetchDatasource
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonPrimitive

internal typealias ForgeDataFenceBlock = ForgeTranscriptDataBlock
internal typealias ForgeUiFencePayload = ForgeTranscriptUiPayload
internal typealias MaterializedForgeDataBlock = ForgeTranscriptDataStore

internal sealed interface TranscriptContentPart {
    data class Markdown(val text: String) : TranscriptContentPart
    data class ForgeUi(
        val payload: ForgeUiFencePayload,
        val dataStore: Map<String, MaterializedForgeDataBlock>
    ) : TranscriptContentPart
}

@Composable
internal fun TranscriptMessageContent(
    markdown: String,
    renderedParts: List<RenderedContentPart>? = null,
    renderedReports: List<ForgeTranscriptCanonicalReport>? = null,
    client: AgentlyClient,
    forgeRuntime: ForgeRuntime,
    messageKey: String,
    onOpenInlineReportPdf: (Map<String, Any?>, () -> Unit) -> Unit = { _, completed -> completed() }
) {
    val parts = remember(markdown, renderedParts, renderedReports) {
        val sourceParts = renderedParts?.let(::canonicalTranscriptContentParts)
            ?.let(ForgeTranscriptEnvelope::fromCanonical)
            ?.let(::displayTranscriptContentParts)
            ?: parseTranscriptContentParts(markdown)
        if (renderedReports.isNullOrEmpty()) {
            sourceParts
        } else {
            sourceParts.map { part ->
                when (part) {
                    is TranscriptContentPart.Markdown -> TranscriptContentPart.Markdown(
                        ForgeTranscriptEnvelope.suppressProgressiveTransport(part.text)
                    )
                    is TranscriptContentPart.ForgeUi -> part
                }
            }
        }
    }
    if (parts.isEmpty() && renderedReports.isNullOrEmpty()) {
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
        renderedReports.orEmpty().forEach { report ->
            TranscriptInlineReportBlock(
                report = report,
                client = client,
                forgeRuntime = forgeRuntime,
                onOpenInlineReportPdf = onOpenInlineReportPdf
            )
        }
    }
}

@Composable
private fun TranscriptInlineReportBlock(
    report: ForgeTranscriptCanonicalReport,
    client: AgentlyClient,
    forgeRuntime: ForgeRuntime,
    onOpenInlineReportPdf: (Map<String, Any?>, () -> Unit) -> Unit
) {
    val previewTitle = (report.source as? JsonObject)
        ?.get("title")?.jsonPrimitive?.contentOrNull
        ?.takeIf { it.isNotBlank() }
        ?: report.id
    val previewSubtitle = (report.source as? JsonObject)
        ?.get("subtitle")?.jsonPrimitive?.contentOrNull
        ?.takeIf { it.isNotBlank() }
    var reportOpen by remember(report.scope, report.id, report.resetVersion) { mutableStateOf(false) }
    var pdfExporting by remember(report.scope, report.id, report.resetVersion) { mutableStateOf(false) }

    Surface(
        color = Color(0xFFF8FAFD),
        shape = MaterialTheme.shapes.large,
        modifier = Modifier
            .fillMaxWidth()
            .clickable { reportOpen = true }
    ) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Text(previewTitle, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
            previewSubtitle?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = Color(0xFF667085))
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = { reportOpen = true }) {
                    Icon(Icons.Outlined.OpenInFull, contentDescription = null)
                    Text("Open report", modifier = Modifier.padding(start = 8.dp))
                }
                PhoneToolbarAction(
                    icon = Icons.Outlined.PictureAsPdf,
                    contentDescription = "Open PDF",
                    accent = Color(0xFFD34B5F),
                    enabled = !pdfExporting,
                    loading = pdfExporting,
                    onClick = {
                        pdfExporting = true
                        onOpenInlineReportPdf(
                            mapOf(
                                "title" to previewTitle,
                                "artifactRef" to "report://inline/${report.scope}/${report.id}",
                                "reportId" to report.id,
                                "fences" to inlineReportExportFences(report)
                            ),
                            { pdfExporting = false }
                        )
                    }
                )
            }
        }
    }
    if (reportOpen) {
        TranscriptInlineReportDialog(
            report = report,
            client = client,
            forgeRuntime = forgeRuntime,
            onOpenInlineReportPdf = onOpenInlineReportPdf,
            onDismiss = { reportOpen = false }
        )
    }
}

@Composable
private fun TranscriptInlineReportDialog(
    report: ForgeTranscriptCanonicalReport,
    client: AgentlyClient,
    forgeRuntime: ForgeRuntime,
    onOpenInlineReportPdf: (Map<String, Any?>, () -> Unit) -> Unit,
    onDismiss: () -> Unit
) {
    val stableIdentity = "${report.scope}:${report.id}:${report.resetVersion}"
    var presentationResult by remember(stableIdentity, report) {
        mutableStateOf<Result<InlineReportPresentation>?>(null)
    }
    var pdfExporting by remember(stableIdentity) { mutableStateOf(false) }
    LaunchedEffect(stableIdentity, report, client) {
        presentationResult = runCatching {
            val hydratedReport = hydrateInlineReport(report, client)
            InlineReportPresentation(
                report = hydratedReport,
                artifact = InlineReportRuntimeCompiler.compile(hydratedReport)
            )
        }
    }
    val resolvedPresentation = presentationResult
    if (resolvedPresentation == null) {
        TranscriptForgeFallback(title = report.id, body = "Loading report…")
        return
    }
    val presentation = resolvedPresentation.getOrNull()
    if (presentation == null) {
        TranscriptForgeFallback(
            title = report.id,
            body = resolvedPresentation.exceptionOrNull()?.message ?: "Unable to compile inline report."
        )
        return
    }
    val artifact = presentation.artifact
    val title = artifact.reportSpec["title"]?.jsonPrimitive?.contentOrNull ?: report.id
    var windowId by remember(stableIdentity) { mutableStateOf<String?>(null) }

    LaunchedEffect(stableIdentity, artifact.metadata) {
        val state = forgeRuntime.openWindowInline(
            windowKey = "inline-report-${report.scope}-${report.id}",
            title = title,
            metadata = artifact.metadata
        )
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
        TranscriptForgeFallback(title = title, body = "Loading report…")
        return
    }

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(
            usePlatformDefaultWidth = false,
            decorFitsSystemWindows = false
        )
    ) {
        Surface(color = Color.White, modifier = Modifier.fillMaxSize()) {
            Column(modifier = Modifier.fillMaxSize().systemBarsPadding()) {
                Row(
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 4.dp, vertical = 4.dp),
                    verticalAlignment = androidx.compose.ui.Alignment.CenterVertically
                ) {
                    PhoneToolbarAction(
                        icon = Icons.AutoMirrored.Outlined.ArrowBack,
                        contentDescription = "Back to conversation",
                        onClick = onDismiss,
                        accent = Color(0xFF5965D8)
                    )
                    Text(
                        "Report",
                        modifier = Modifier.weight(1f),
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.SemiBold
                    )
                    if (report.status.trim().lowercase().let { it.isEmpty() || it == "committed" || it == "ready" }) {
                        PhoneToolbarAction(
                            icon = Icons.Outlined.PictureAsPdf,
                            contentDescription = "Open PDF",
                            accent = Color(0xFFD34B5F),
                            enabled = !pdfExporting,
                            loading = pdfExporting,
                            onClick = {
                                pdfExporting = true
                                onOpenInlineReportPdf(
                                    mapOf(
                                        "title" to title,
                                        "artifactRef" to "report://inline/${report.scope}/${report.id}",
                                        "reportId" to report.id,
                                        "fences" to inlineReportExportFences(presentation.report)
                                    ),
                                    { pdfExporting = false }
                                )
                            }
                        )
                    }
                }
                HorizontalDivider()
                WindowContentView(
                    runtime = forgeRuntime,
                    windowId = windowContext.windowId,
                    windowKey = title,
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                        .padding(horizontal = 12.dp),
                    scrollEnabled = true,
                    showWindowHeader = false
                )
            }
        }
    }
}

private data class InlineReportPresentation(
    val report: ForgeTranscriptCanonicalReport,
    val artifact: com.viant.forgeandroid.runtime.InlineReportRuntimeArtifact
)

internal fun inlineReportExportFences(
    report: ForgeTranscriptCanonicalReport
): List<Map<String, Any?>> {
    val source = report.source as? JsonObject
        ?: error("Inline report source must be a JSON object.")
    val exportScope = (source["scope"] as? JsonPrimitive)?.content
        ?.trim()
        ?.takeIf { it.isNotEmpty() }
        ?: report.scope
    var sequence = 1
    val fences = mutableListOf<Map<String, Any?>>()
    val reportStart = JsonObject(inlineReportPdfSource(source).toMutableMap().apply {
        put("version", JsonPrimitive(1))
        put("scope", JsonPrimitive(exportScope))
        put("id", JsonPrimitive(report.id))
        put("sequence", JsonPrimitive(sequence++))
        put("mode", JsonPrimitive("start"))
        put("grammar", JsonPrimitive(report.grammar))
    })
    fences += exportFence("forge-report", fences.size, reportStart)
    report.dataSources.toSortedMap().forEach { (key, dataSource) ->
        val payload = dataSource.payload ?: return@forEach
        val dataFence = linkedMapOf<String, JsonElement>(
            "version" to JsonPrimitive(dataSource.version ?: 2),
            "scope" to JsonPrimitive(exportScope),
            "reportRef" to JsonPrimitive(dataSource.reportRef ?: report.id),
            "sequence" to JsonPrimitive(sequence++),
            "id" to JsonPrimitive(dataSource.id.ifBlank { key }),
            "format" to JsonPrimitive(dataSource.format?.ifBlank { "json" } ?: "json"),
            "mode" to JsonPrimitive("replace"),
            "data" to payload
        )
        fences += exportFence("forge-data", fences.size, JsonObject(dataFence))
    }
    val commit = JsonObject(
        mapOf(
            "version" to JsonPrimitive(1),
            "scope" to JsonPrimitive(exportScope),
            "id" to JsonPrimitive(report.id),
            "sequence" to JsonPrimitive(sequence),
            "mode" to JsonPrimitive("commit")
        )
    )
    fences += exportFence("forge-report", fences.size, commit)
    return fences
}

internal fun inlineReportPdfSource(source: JsonObject): JsonObject {
    val blocks = source["blocks"] as? JsonArray ?: return source
    val normalizedBlocks = blocks.map { element ->
        val block = element as? JsonObject ?: return@map element
        val values = block.toMutableMap()
        when ((values["kind"] as? JsonPrimitive)?.content) {
            "tableBlock" -> {
                // Interactive report cards may carry explanatory copy here, but the
                // Go reporting schema keeps table descriptions outside the block.
                values.remove("description")
                // Link/drill metadata is interactive-only. The Go PDF schema is
                // deliberately strict and renders the table snapshot without it.
                values.remove("link")
                values["columns"] = normalizeInlineReportPdfColumns(values["columns"])
            }
            "kpiBlock" -> {
                if (values["description"] == null) {
                    values["subtitle"]?.let { values["description"] = it }
                }
                values.remove("subtitle")
                values.remove("suffix")
                values.remove("tone")
            }
            "chartBlock" -> {
                // Descriptive/interaction fields are rendered by the native UI,
                // but the Go chart export contract accepts only the chart model.
                val allowed = setOf("id", "kind", "title", "datasetRef", "chartSpec", "chartModel", "runtime")
                values.keys.toList().filterNot(allowed::contains).forEach(values::remove)
            }
            "geoMapBlock" -> {
                // The deployed Go fenced compiler validates geo fills but older
                // versions do not materialize them. Preserve the evidence as a
                // printable table until that backend is upgraded.
                val geo = values.remove("geo") as? JsonObject
                val key = (geo?.get("key") as? JsonPrimitive)?.content.orEmpty()
                val labelKey = (geo?.get("labelKey") as? JsonPrimitive)?.content.orEmpty()
                val metric = geo?.get("metric") as? JsonObject
                val metricKey = (metric?.get("key") as? JsonPrimitive)?.content.orEmpty()
                val metricLabel = (metric?.get("label") as? JsonPrimitive)?.content.orEmpty()
                val metricFormat = metric?.get("format")
                val columns = mutableListOf<JsonElement>()
                fun addColumn(field: String, label: String, format: JsonElement? = null) {
                    if (field.isBlank() || columns.any { (it as? JsonObject)?.get("key") == JsonPrimitive(field) }) return
                    columns += JsonObject(buildMap {
                        put("key", JsonPrimitive(field))
                        put("label", JsonPrimitive(label.ifBlank { field.replaceFirstChar(Char::uppercase) }))
                        format?.let { put("format", it) }
                    })
                }
                addColumn(key, "Region")
                addColumn(labelKey, "Region name")
                addColumn(metricKey, metricLabel, metricFormat)
                values["kind"] = JsonPrimitive("tableBlock")
                values["columns"] = JsonArray(columns)
                values.remove("description")
            }
            "collectionBlock" -> {
                // Older Go fenced compilers validate collections but do not
                // materialize collection fill content. Export their authored
                // fields as a full-width evidence table instead.
                val fields = linkedSetOf<String>()
                (values["itemTitleField"] as? JsonPrimitive)?.content?.takeIf(String::isNotBlank)?.let(fields::add)
                (values["toneField"] as? JsonPrimitive)?.content?.takeIf(String::isNotBlank)?.let(fields::add)
                (values["valueField"] as? JsonPrimitive)?.content?.takeIf(String::isNotBlank)?.let(fields::add)
                (values["secondaryField"] as? JsonPrimitive)?.content?.takeIf(String::isNotBlank)?.let(fields::add)
                (values["bodyTemplate"] as? JsonPrimitive)?.content
                    ?.let(::inlineReportTemplateFields)
                    ?.let(fields::addAll)
                values["kind"] = JsonPrimitive("tableBlock")
                values["columns"] = JsonArray(fields.map { field ->
                    JsonObject(mapOf(
                        "key" to JsonPrimitive(field),
                        "label" to JsonPrimitive(field.replace('_', ' ').replaceFirstChar(Char::uppercase))
                    ))
                })
            }
            "timelineBlock" -> {
                val timeField = (values.remove("timeField") as? JsonPrimitive)?.content.orEmpty()
                val titleField = (values.remove("titleField") as? JsonPrimitive)?.content.orEmpty()
                val descriptionField = (values.remove("descriptionField") as? JsonPrimitive)?.content.orEmpty()
                val columns = mutableListOf<JsonElement>()
                fun addColumn(key: String, label: String) {
                    if (key.isNotBlank()) {
                        columns += JsonObject(mapOf("key" to JsonPrimitive(key), "label" to JsonPrimitive(label)))
                    }
                }
                addColumn(timeField, "Time")
                addColumn(titleField, "Event")
                addColumn(descriptionField, "Detail")
                (values["columns"] as? JsonArray)?.let(columns::addAll)
                values["kind"] = JsonPrimitive("tableBlock")
                values["columns"] = JsonArray(columns.distinctBy { column ->
                    ((column as? JsonObject)?.get("key") as? JsonPrimitive)?.content.orEmpty()
                })
            }
            "badgesBlock" -> {
                val items = values["items"] as? JsonArray
                if (items != null) {
                    values["items"] = JsonArray(items.mapIndexed { index, item ->
                        val badge = item as? JsonObject ?: return@mapIndexed item
                        if (badge["id"] != null) return@mapIndexed badge
                        JsonObject(badge.toMutableMap().apply {
                            put("id", JsonPrimitive("badge_${index + 1}"))
                        })
                    })
                }
            }
            "infoPanelBlock", "calloutBlock" -> {
                val markdown = values["body"] ?: values["description"] ?: JsonPrimitive("")
                val id = values["id"]
                val title = values["title"]
                values.clear()
                id?.let { values["id"] = it }
                values["kind"] = JsonPrimitive("markdownBlock")
                title?.let { values["title"] = it }
                values["markdown"] = markdown
            }
        }
        JsonObject(retainInlineReportPdfBlockFields(values))
    }
    return JsonObject(source.toMutableMap().apply { put("blocks", JsonArray(normalizedBlocks)) })
}

private fun inlineReportTemplateFields(template: String): List<String> {
    val prefix = "${'$'}{row."
    val result = mutableListOf<String>()
    var cursor = 0
    while (cursor < template.length) {
        val start = template.indexOf(prefix, cursor)
        if (start < 0) break
        val end = template.indexOf('}', start + prefix.length)
        if (end < 0) break
        template.substring(start + prefix.length, end)
            .trim().takeIf { it.isNotEmpty() }?.let(result::add)
        cursor = end + 1
    }
    return result.distinct()
}

private fun retainInlineReportPdfBlockFields(values: MutableMap<String, JsonElement>): Map<String, JsonElement> {
    val common = setOf("id", "kind", "runtime")
    val allowed = when ((values["kind"] as? JsonPrimitive)?.content) {
        "tableBlock" -> common + setOf("title", "datasetRef", "columns")
        "chartBlock" -> common + setOf("title", "datasetRef", "chartSpec", "chartModel")
        "kpiBlock" -> common + setOf(
            "title", "datasetRef", "valueField", "valueLabel", "valueFormat",
            "secondaryField", "secondaryLabel", "secondaryFormat", "secondaryDisplayKey",
            "secondaryDisplayValueMap", "description", "emptyLabel", "rowSelector",
            "presentationMode", "bodyFormat", "bodyTemplate"
        )
        "badgesBlock" -> common + setOf("title", "datasetRef", "items")
        "collectionBlock" -> common + setOf(
            "title", "description", "datasetRef", "itemTitleField", "itemTitleLabel",
            "toneField", "toneRules", "valueField", "valueLabel", "valueFormat",
            "secondaryField", "secondaryLabel", "secondaryFormat", "layout", "columns",
            "rowLimit", "bodyFormat", "bodyTemplate", "emptyLabel"
        )
        "sectionBlock" -> common + setOf("title", "subtitle", "description", "navigationLabel")
        "compositeBlock" -> common + setOf("title", "description", "childBlockIds")
        "tabGroupBlock" -> common + setOf("title", "sectionIds", "defaultSectionId")
        "stepperBlock" -> common + setOf("title", "description", "steps")
        "infoPanelBlock" -> common + setOf("title", "eyebrow", "description", "tone", "bodyFormat", "body")
        "calloutBlock" -> common + setOf("title", "icon", "description", "tone", "badges", "bodyFormat", "body")
        "kanbanBlock" -> common + setOf("title", "description", "columns")
        "timelineBlock" -> common + setOf("title", "description", "events")
        "filterBarBlock" -> common + setOf(
            "title", "datasetRef", "paramIds", "mode", "placement", "groupOrder",
            "visibleGroups", "collapsedGroups"
        )
        "refinementBarBlock" -> common + setOf("title", "actionKinds", "emptyLabel")
        "markdownBlock" -> common + setOf("title", "markdown")
        "geoMapBlock" -> common + setOf("title", "datasetRef", "geo")
        else -> values.keys
    }
    return values.filterKeys(allowed::contains)
}

private fun normalizeInlineReportPdfColumns(value: JsonElement?): JsonArray {
    val allowed = setOf(
        "key", "sourceKey", "displayKey", "label", "kind", "format", "align",
        "cellVisual", "runtimeFilterable"
    )
    return JsonArray((value as? JsonArray).orEmpty().map { element ->
        val column = element as? JsonObject ?: return@map element
        JsonObject(buildMap {
            column.forEach { (key, item) ->
                when {
                    key in allowed -> put(key, item)
                    key == "type" && column["kind"] == null -> put("kind", item)
                }
            }
        })
    })
}

private fun exportFence(kind: String, index: Int, payload: JsonObject): Map<String, Any?> =
    mapOf("kind" to kind, "index" to index, "payload" to payload)

private suspend fun hydrateInlineReport(
    report: ForgeTranscriptCanonicalReport,
    client: AgentlyClient
): ForgeTranscriptCanonicalReport {
    val dataSources = report.dataSources.toMutableMap()
    InlineReportRuntimeCompiler.workspaceDatasetRequests(report).forEach { request ->
        val response = client.fetchDatasource(
            FetchDatasourceInput(
                id = request.dataSourceRef,
                inputs = request.inputs.toMap()
            )
        )
        dataSources[request.id] = ForgeTranscriptCanonicalData(
            version = 2,
            scope = report.scope,
            reportRef = report.id,
            id = request.id,
            format = "json",
            mode = "replace",
            payload = JsonArray(response.rows.map(::JsonObject))
        )
    }
    return report.copy(dataSources = dataSources)
}
@Composable
private fun TranscriptForgeUiBlock(
    messageKey: String,
    payload: ForgeUiFencePayload,
    dataStore: Map<String, MaterializedForgeDataBlock>,
    forgeRuntime: ForgeRuntime
) {
    val presentationResult = remember(payload, dataStore) {
        runCatching { forgeBuildTranscriptWindowPresentation(payload, dataStore) }
    }
    val presentation = presentationResult.getOrNull()
    if (presentation == null) {
        TranscriptForgeFallback(
            title = payload.title?.takeIf { it.isNotBlank() } ?: "Forge content",
            body = presentationResult.exceptionOrNull()?.message ?: "Unable to decode forge-ui block."
        )
        return
    }
    val inlineMetadata = presentation.metadata
    val inlinePresentation = rememberTranscriptInlinePresentation(inlineMetadata)

    var windowId by remember(messageKey) { mutableStateOf<String?>(null) }

    LaunchedEffect(messageKey, inlineMetadata, presentation.dataStore) {
        val state = forgeRuntime.openWindowInline(
            windowKey = "transcript-forge-$messageKey",
            title = payload.title ?: "Forge content",
            metadata = inlineMetadata
        )
        hydrateTranscriptForgeDataSources(forgeRuntime, state.windowId, presentation.dataStore)
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
        WindowContentView(
            runtime = forgeRuntime,
            windowId = windowContext.windowId,
            windowKey = payload.title ?: "Forge content",
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(max = inlinePresentation.maximumHeight),
            scrollEnabled = true,
            showWindowHeader = false
        )
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

internal fun parseTranscriptContentParts(markdown: String): List<TranscriptContentPart> =
    displayTranscriptContentParts(ForgeTranscriptEnvelope.parse(markdown))

private fun displayTranscriptContentParts(parts: List<ForgeTranscriptEnvelopePart>): List<TranscriptContentPart> =
    parts.map { part ->
        when (part) {
            is ForgeTranscriptEnvelopePart.Markdown -> TranscriptContentPart.Markdown(part.text)
            is ForgeTranscriptEnvelopePart.ForgeUi -> TranscriptContentPart.ForgeUi(part.payload, part.dataStore)
        }
    }

private fun canonicalTranscriptContentParts(parts: List<RenderedContentPart>): List<ForgeTranscriptCanonicalPart> =
    parts.map { part ->
        ForgeTranscriptCanonicalPart(
            kind = part.kind,
            text = part.text,
            source = part.source,
            payload = part.payload,
            data = part.data?.let { data ->
                ForgeTranscriptCanonicalData(
                    version = data.version,
                    scope = data.scope,
                    reportRef = data.reportRef,
                    sequence = data.sequence,
                    id = data.id,
                    format = data.format,
                    mode = data.mode,
                    payload = data.payload
                )
            }
        )
    }

internal fun buildTranscriptForgeWindowMetadata(
    payload: ForgeUiFencePayload,
    dataStore: Map<String, MaterializedForgeDataBlock>
): WindowMetadata = forgeBuildTranscriptWindowMetadata(payload, dataStore)

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
        if (rows.isEmpty()) {
            context.metrics.set(emptyMap())
            context.resetSelection()
            context.setForm(emptyMap())
        } else if (rows.size == 1) {
            val row = rows.first()
            context.metrics.set(row)
            if (context.peekSelection().selected == null) {
                context.setForm(row)
            }
        }
    }
}
