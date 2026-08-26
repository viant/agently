package com.viant.agently.android

import android.util.Log
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
import androidx.compose.material.icons.outlined.PictureAsPdf
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.CircularProgressIndicator
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
import kotlinx.coroutines.TimeoutCancellationException
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonPrimitive
import java.net.SocketTimeoutException

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
    conversationId: String? = null,
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
                conversationId = conversationId,
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
    conversationId: String?,
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
    val reportPending = isPendingInlineReport(report)
    val navigation = (report.source as? JsonObject)?.get("navigation") as? JsonObject
    val destinationTitle = (navigation?.get("label") as? JsonPrimitive)?.contentOrNull
        ?.trim()?.takeIf(String::isNotEmpty) ?: previewTitle
    val destinationDetail = (navigation?.get("supportingText") as? JsonPrimitive)?.contentOrNull
        ?.trim()?.takeIf(String::isNotEmpty) ?: "Open $destinationTitle."
    val destinationIcon = assistantDestinationIcon(
        (navigation?.get("icon") as? JsonPrimitive)?.contentOrNull
    )

    Surface(
        color = Color(0xFFF8FAFD),
        shape = MaterialTheme.shapes.large,
        modifier = Modifier
            .fillMaxWidth()
            .clickable(enabled = !reportPending) { reportOpen = true }
    ) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Text(previewTitle, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
            previewSubtitle?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = Color(0xFF667085))
            }
            if (reportPending) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
                    Text(
                        inlineReportBuildStatus(report),
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                }
            } else {
                AssistantDestinationLink(
                    title = destinationTitle,
                    supportingText = destinationDetail,
                    icon = destinationIcon,
                    contentDescription = "Open $destinationTitle",
                    onOpen = { reportOpen = true },
                )
            }
        }
    }
    if (reportOpen && !reportPending) {
        TranscriptInlineReportDialog(
            report = report,
            conversationId = conversationId,
            client = client,
            forgeRuntime = forgeRuntime,
            onOpenInlineReportPdf = onOpenInlineReportPdf,
            onDismiss = { reportOpen = false }
        )
    }
}

internal fun isPendingInlineReport(report: ForgeTranscriptCanonicalReport): Boolean =
    report.status.trim().lowercase() in setOf("rendering", "pending", "incomplete", "building")

internal fun inlineReportBuildStatus(report: ForgeTranscriptCanonicalReport): String {
    val blockCount = (report.source as? JsonObject)
        ?.get("blocks")
        ?.let { it as? JsonArray }
        ?.size
        ?: 0
    val dataSourceCount = report.dataSources.size
    return buildString {
        append("Building report")
        if (dataSourceCount > 0) {
            append(" · ")
            append(dataSourceCount)
            append(if (dataSourceCount == 1) " data source" else " data sources")
        }
        if (blockCount > 0) {
            append(" · ")
            append(blockCount)
            append(if (blockCount == 1) " block" else " blocks")
        }
    }
}

@Composable
private fun TranscriptInlineReportDialog(
    report: ForgeTranscriptCanonicalReport,
    conversationId: String?,
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
        val result = runCatching {
            val hydratedReport = hydrateInlineReport(report, client, conversationId)
            val artifact = InlineReportRuntimeCompiler.compile(hydratedReport)
            InlineReportPresentation(
                report = hydratedReport,
                artifact = artifact
            )
        }
        result.exceptionOrNull()?.let { error ->
            runCatching { Log.e("InlineReport", "open failed", error) }
        }
        presentationResult = result
    }
    val resolvedPresentation = presentationResult
    val presentation = resolvedPresentation?.getOrNull()
    val artifact = presentation?.artifact
    val title = artifact?.reportSpec?.get("title")?.jsonPrimitive?.contentOrNull
        ?: (report.source as? JsonObject)?.get("title")?.jsonPrimitive?.contentOrNull
        ?: report.id
    var windowId by remember(stableIdentity) { mutableStateOf<String?>(null) }

    LaunchedEffect(stableIdentity, artifact?.metadata) {
        val reportArtifact = artifact ?: return@LaunchedEffect
        val state = forgeRuntime.openWindowInline(
            windowKey = "inline-report-${report.scope}-${report.id}",
            title = title,
            metadata = reportArtifact.metadata
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
                    if (presentation != null && report.status.trim().lowercase().let { it.isEmpty() || it == "committed" || it == "ready" }) {
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
                when {
                    resolvedPresentation == null -> TranscriptForgeFallback(
                        title = title,
                        body = "Loading report…"
                    )
                    presentation == null -> TranscriptForgeFallback(
                        title = title,
                        body = inlineReportPresentationErrorMessage(
                            resolvedPresentation.exceptionOrNull()
                        )
                    )
                    resolvedMetadata == null || windowContext == null -> TranscriptForgeFallback(
                        title = title,
                        body = "Loading report…"
                    )
                    else -> WindowContentView(
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
}

internal fun inlineReportPresentationErrorMessage(error: Throwable?): String {
    val timedOut = generateSequence(error) { it.cause }
        .any { it is SocketTimeoutException || it is TimeoutCancellationException }
    return when {
        timedOut ->
            "The report took too long to open. Try again."
        else -> "Unable to open this report. Try again."
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
    val reportStart = JsonObject(source.toMutableMap().apply {
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
            "id" to JsonPrimitive(key.ifBlank { dataSource.id }),
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

private fun exportFence(kind: String, index: Int, payload: JsonObject): Map<String, Any?> =
    mapOf("kind" to kind, "index" to index, "payload" to payload)

private suspend fun hydrateInlineReport(
    report: ForgeTranscriptCanonicalReport,
    client: AgentlyClient,
    conversationId: String?
): ForgeTranscriptCanonicalReport {
    val dataSources = report.dataSources.toMutableMap()
    InlineReportRuntimeCompiler.workspaceDatasetRequests(report).forEach { request ->
        val response = client.fetchDatasource(
            FetchDatasourceInput(
                id = request.dataSourceRef,
                inputs = request.inputs.toMap(),
                conversationId = conversationId?.trim()?.takeIf { it.isNotEmpty() }
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
