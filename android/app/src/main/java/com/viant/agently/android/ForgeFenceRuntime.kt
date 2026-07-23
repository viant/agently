package com.viant.agently.android

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
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
import kotlinx.serialization.json.JsonObject
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
    messageKey: String
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
                forgeRuntime = forgeRuntime
            )
        }
    }
}

@Composable
private fun TranscriptInlineReportBlock(
    report: ForgeTranscriptCanonicalReport,
    client: AgentlyClient,
    forgeRuntime: ForgeRuntime
) {
    val stableIdentity = "${report.scope}:${report.id}:${report.resetVersion}"
    var artifactResult by remember(stableIdentity, report) {
        mutableStateOf<Result<com.viant.forgeandroid.runtime.InlineReportRuntimeArtifact>?>(null)
    }
    LaunchedEffect(stableIdentity, report, client) {
        artifactResult = runCatching {
            InlineReportRuntimeCompiler.compile(hydrateInlineReport(report, client))
        }
    }
    val resolvedArtifact = artifactResult
    if (resolvedArtifact == null) {
        TranscriptForgeFallback(title = report.id, body = "Loading report…")
        return
    }
    val artifact = resolvedArtifact.getOrNull()
    if (artifact == null) {
        TranscriptForgeFallback(
            title = report.id,
            body = resolvedArtifact.exceptionOrNull()?.message ?: "Unable to compile inline report."
        )
        return
    }
    val inlinePresentation = rememberTranscriptInlinePresentation(artifact.metadata)
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

    Surface(
        color = Color(0xFFF8FAFD),
        shape = MaterialTheme.shapes.large,
        modifier = Modifier.fillMaxWidth()
    ) {
        WindowContentView(
            runtime = forgeRuntime,
            windowId = windowContext.windowId,
            windowKey = title,
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(max = inlinePresentation.maximumHeight),
            scrollEnabled = true,
            showWindowHeader = false
        )
    }
}

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
