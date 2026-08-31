package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.DownloadFileOutput
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.ContainerDef
import com.viant.forgeandroid.runtime.InlineReportRuntimeCompiler
import com.viant.forgeandroid.runtime.JsonUtil
import com.viant.forgeandroid.runtime.snapshotFeedDataSources
import com.viant.forgeandroid.ui.TranscriptCanonicalData
import com.viant.forgeandroid.ui.TranscriptCanonicalReport
import kotlinx.coroutines.delay
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonPrimitive
import java.util.Base64

private const val REPORT_EXPORT_POLL_INTERVAL_MS = 1_500L
private const val REPORT_EXPORT_POLL_ATTEMPTS = 20

internal fun registerReportRuntimeExportHandler(
    forgeRuntime: ForgeRuntime,
    client: AgentlyClient,
    conversationIdProvider: () -> String?,
    onError: (String?) -> Unit,
    openPdf: (GeneratedFileEntry, DownloadFileOutput) -> Boolean
) {
    forgeRuntime.registerHandler("reportRuntime.exportPdf") { args ->
        val exportRequest = JsonUtil.asStringMap(args.args["exportRequest"]).takeIf { it.isNotEmpty() }
        if (exportRequest == null) {
            onError("No report export request is available.")
            return@registerHandler false
        }
        val conversationId = conversationIdProvider()?.trim().orEmpty()
        val artifact = try {
            exportReportRuntimePdf(
                client = client,
                exportRequest = exportRequest,
                conversationId = conversationId
            )
        } catch (err: Throwable) {
            onError(reportRuntimeExportErrorMessage(err))
            throw err
        }
        val opened = openPdf(artifact.file, artifact.downloaded)
        if (!opened) {
            onError("PDF export completed, but no PDF viewer was available.")
            return@registerHandler false
        }
        onError(null)
        true
    }
    forgeRuntime.registerHandler("feed.print") { args ->
        val context = args.context
        if (context == null) {
            onError("No active feed is available for PDF export.")
            return@registerHandler false
        }
        val state = forgeRuntime.windowState(context.window.windowId)
        val feedId = state?.windowKey?.removePrefix("feed-")
            ?.removeSuffix("-${state.conversationId.orEmpty()}")
            ?.takeIf(String::isNotBlank)
            ?: "feed"
        val exportRequest = buildFeedReportExportRequest(
            forgeRuntime = forgeRuntime,
            windowId = context.window.windowId,
            feedId = feedId,
            title = state?.windowTitle ?: "Tool feed"
        )
        val conversationId = conversationIdProvider()?.trim().orEmpty()
        val artifact = try {
            exportReportRuntimePdf(client, exportRequest, conversationId)
        } catch (err: Throwable) {
            onError(reportRuntimeExportErrorMessage(err))
            throw err
        }
        val opened = openPdf(artifact.file, artifact.downloaded)
        if (!opened) {
            onError("PDF export completed, but no PDF viewer was available.")
            return@registerHandler false
        }
        onError(null)
        true
    }
}

internal fun buildFeedReportExportRequest(
    forgeRuntime: ForgeRuntime,
    windowId: String,
    feedId: String,
    title: String
): Map<String, Any?> {
    val metadata = forgeRuntime.metadataSignal(windowId).peek()
        ?: error("Feed PDF export requires window metadata.")
    val content = metadata.view?.content ?: error("Feed PDF export requires feed content.")
    val safeFeedId = safeFeedReportId(feedId)
    val sourceRefMap = metadata.dataSources.keys.associateWith(::safeFeedReportId)
    val blocks = content.containers.flatMap { feedPrintableBlocks(it, sourceRefMap) }
    val snapshots = snapshotFeedDataSources(
        forgeRuntime.windowContext(windowId),
        metadata.dataSources.keys
    )
    val dataSources = snapshots.mapValues { (ref, snapshot) ->
        val rows = snapshot.collection.takeIf { it.isNotEmpty() }
            ?: snapshot.form.takeIf { it.isNotEmpty() }?.let(::listOf)
            ?: emptyList()
        TranscriptCanonicalData(
            id = sourceRefMap[ref] ?: safeFeedReportId(ref),
            format = "json",
            payload = JsonArray(rows.map { JsonUtil.anyToElement(it) })
        )
    }.mapKeys { (ref, _) -> sourceRefMap[ref] ?: safeFeedReportId(ref) }
    val reportId = "feed_$safeFeedId"
    val canonicalReport = TranscriptCanonicalReport(
        scope = "feed:$safeFeedId",
        id = reportId,
        grammar = "dashboard-v1",
        status = "committed",
        source = JsonObject(
            mapOf(
                "id" to JsonPrimitive(safeFeedId),
                "title" to JsonPrimitive(title),
                "blocks" to JsonArray(blocks)
            )
        ),
        dataSources = dataSources
    )
    val compiled = InlineReportRuntimeCompiler.compile(canonicalReport)
    val reportPrint = JsonObject(
        mapOf(
            "version" to JsonPrimitive(1),
            "kind" to JsonPrimitive("reportPrint"),
            "specVersion" to JsonPrimitive(1),
            "reportId" to JsonPrimitive(reportId),
            "title" to JsonPrimitive(title)
        )
    )
    return mapOf(
        "format" to "pdf",
        "reportId" to reportId,
        "fences" to JsonArray(InlineReportRuntimeCompiler.exportFences(canonicalReport)),
        "artifactRef" to "feed://$safeFeedId",
        "title" to title,
        "reportSpec" to JsonUtil.elementToAny(compiled.reportSpec),
        "reportFill" to JsonUtil.elementToAny(compiled.reportFill),
        "reportPrint" to JsonUtil.elementToAny(reportPrint)
    )
}

private val printableFeedKinds = setOf(
    "dashboard.summary", "dashboard.kpiTable", "dashboard.compare", "dashboard.timeline",
    "dashboard.composition", "dashboard.dimensions", "dashboard.geoMap", "dashboard.status",
    "dashboard.filters", "dashboard.feed", "dashboard.table", "dashboard.report",
    "dashboard.detail", "dashboard.messages", "dashboard.badges"
)

private fun feedPrintableBlocks(
    node: ContainerDef,
    sourceRefMap: Map<String, String>
): List<JsonElement> {
    fun serialized(value: ContainerDef): JsonObject =
        Json.encodeToJsonElement(ContainerDef.serializer(), value) as JsonObject
    fun mapped(value: JsonObject): JsonObject = JsonObject(value.toMutableMap().apply {
        (this["dataSourceRef"] as? JsonPrimitive)?.contentOrNull?.let { ref ->
            sourceRefMap[ref]?.let { put("dataSourceRef", JsonPrimitive(it)) }
        }
    })
    if (node.kind in setOf("dashboard.editableTable", "dashboard.lookupChips")) {
        return listOf(mapped(serialized(node)).let { value ->
            JsonObject(value.toMutableMap().apply { put("kind", JsonPrimitive("dashboard.table")) })
        })
    }
    if (node.tabs != null && node.containers.isNotEmpty()) {
        return node.containers.mapNotNull { tab ->
            val children = tab.containers.flatMap { feedPrintableBlocks(it, sourceRefMap) }
            if (children.isEmpty()) null else JsonObject(
                mapOf(
                    "id" to JsonPrimitive(safeFeedReportId(tab.id ?: tab.title ?: "section")),
                    "kind" to JsonPrimitive("dashboard.detail"),
                    "title" to JsonPrimitive(tab.title ?: tab.id ?: "Section"),
                    "containers" to JsonArray(children)
                )
            )
        }
    }
    if (node.kind in printableFeedKinds) {
        val block = mapped(serialized(node))
        return if (node.kind == "dashboard.detail") {
            listOf(JsonObject(block.toMutableMap().apply {
                put("containers", JsonArray(node.containers.flatMap { feedPrintableBlocks(it, sourceRefMap) }))
            }))
        } else listOf(block)
    }
    val nested = node.containers.flatMap { feedPrintableBlocks(it, sourceRefMap) }
    if (nested.isNotEmpty()) {
        return listOf(JsonObject(
            mapOf(
                "id" to JsonPrimitive(safeFeedReportId(node.id ?: node.title ?: "section")),
                "kind" to JsonPrimitive("dashboard.detail"),
                "title" to JsonPrimitive(node.title.orEmpty()),
                "containers" to JsonArray(nested)
            )
        ))
    }
    val nodeDataSourceRef = node.dataSourceRef
    if (node.items.isNotEmpty() && !nodeDataSourceRef.isNullOrBlank()) {
        val rows = node.items.map { item ->
            JsonObject(
                mapOf(
                    "id" to JsonPrimitive(safeFeedReportId(item.id ?: item.field ?: item.label ?: "value")),
                    "label" to JsonPrimitive(item.label ?: item.id ?: item.field ?: "Value"),
                    "value" to JsonPrimitive(item.field ?: item.dataField ?: item.id ?: "value")
                )
            )
        }
        return listOf(JsonObject(
            mapOf(
                "id" to JsonPrimitive(safeFeedReportId(node.id ?: node.title ?: "summary")),
                "kind" to JsonPrimitive("dashboard.kpiTable"),
                "title" to JsonPrimitive(node.title.orEmpty()),
                "rows" to JsonArray(rows),
                "dataSourceRef" to JsonPrimitive(sourceRefMap[nodeDataSourceRef] ?: safeFeedReportId(nodeDataSourceRef))
            )
        ))
    }
    return emptyList()
}

private fun safeFeedReportId(value: String): String = value.trim().lowercase()
    .replace(Regex("[^a-z0-9]+"), "_")
    .trim('_')
    .ifBlank { "feed" }

internal data class ReportRuntimeExportArtifact(
    val file: GeneratedFileEntry,
    val downloaded: DownloadFileOutput
)

internal suspend fun exportReportRuntimePdf(
    client: AgentlyClient,
    exportRequest: Map<String, Any?>,
    conversationId: String = ""
): ReportRuntimeExportArtifact {
	val fences = JsonUtil.anyToElement(exportRequest["fences"]) as? JsonArray
	val completedJob = if (fences != null && fences.isNotEmpty()) {
		val result = executeReportingToolObject(
			client = client,
			toolName = "reporting:compile_and_export_fenced_report",
			args = mapOf(
				"reportId" to JsonPrimitive(stringValue(exportRequest["reportId"])),
				"format" to JsonPrimitive("pdf"),
				"fences" to fences,
				"conversationId" to JsonPrimitive(conversationId)
			),
			conversationId = conversationId
		)
		val job = normalizeReportExportJob(result["job"] as? JsonObject ?: JsonObject(emptyMap()))
		val artifactId = stringValue((result["artifact"] as? JsonObject)?.get("artifactId"))
		job.copy(artifactId = job.artifactId.ifBlank { artifactId })
	} else {
		val submitResult = executeReportingToolObject(
			client = client,
			toolName = "reporting:submit_export",
			args = normalizeReportRuntimeExportRequest(exportRequest),
			conversationId = conversationId
		)
		waitForReportExportArtifact(
			client = client,
			initialJob = normalizeReportExportJob(submitResult),
			conversationId = conversationId
		)
	}
    val artifactId = completedJob.artifactId.ifBlank {
        error("Report PDF export completed without an artifact id.")
    }
    val artifact = executeReportingToolObject(
        client = client,
        toolName = "reporting:get_artifact",
        args = mapOf(
            "artifactId" to JsonPrimitive(artifactId),
            "includeData" to JsonPrimitive(true)
        ),
        conversationId = conversationId
    )
    val bytes = decodeReportArtifactBytes(artifact)
    if (bytes.isEmpty()) {
        error("Report PDF artifact was empty.")
    }
    val title = stringValue(exportRequest["title"]).ifBlank { completedJob.artifactRef.ifBlank { artifactId } }
    val name = sanitizeDownloadFileName(
        stringValue(artifact["filename"])
            .ifBlank { stringValue(artifact["name"]) }
            .ifBlank { "$title.pdf" }
    ).ifBlank { "$artifactId.pdf" }
    val contentType = stringValue(artifact["contentType"]).ifBlank { "application/pdf" }
    return ReportRuntimeExportArtifact(
        file = GeneratedFileEntry(
            id = artifactId,
            conversationId = conversationId.ifBlank { null },
            filename = name,
            mimeType = contentType,
            sizeBytes = bytes.size
        ),
        downloaded = DownloadFileOutput(
            name = name,
            contentType = contentType,
            data = bytes
        )
    )
}

private fun normalizeReportRuntimeExportRequest(exportRequest: Map<String, Any?>): Map<String, JsonElement> {
    val artifactRef = stringValue(exportRequest["artifactRef"])
        .takeIf { it.isNotBlank() }
        ?: error("Report PDF export requires artifactRef.")
    val reportSpec = JsonUtil.anyToElement(exportRequest["reportSpec"]) as? JsonObject
        ?: error("Report PDF export requires reportSpec.")
    val reportFill = JsonUtil.anyToElement(exportRequest["reportFill"]) as? JsonObject
        ?: error("Report PDF export requires reportFill.")
    val reportPrint = JsonUtil.anyToElement(exportRequest["reportPrint"]) as? JsonObject
        ?: error("Report PDF export requires reportPrint.")
    return linkedMapOf(
        "artifactRef" to JsonPrimitive(artifactRef),
        "format" to JsonPrimitive("pdf"),
        "scope" to JsonPrimitive("draft"),
        "reportSpec" to reportSpec,
        "reportFill" to reportFill,
        "reportPrint" to reportPrint
    )
}

private suspend fun waitForReportExportArtifact(
    client: AgentlyClient,
    initialJob: ReportExportJobState,
    conversationId: String
): ReportExportJobState {
    var job = initialJob
    repeat(REPORT_EXPORT_POLL_ATTEMPTS) { attempt ->
        if (job.artifactId.isNotBlank()) {
            return job
        }
        val status = job.status.lowercase()
        if (status in setOf("failed", "canceled", "cancelled")) {
            error(job.error.ifBlank { "Report PDF export failed." })
        }
        if (job.jobId.isBlank()) {
            error("Report PDF export did not return a job id.")
        }
        if (attempt > 0) {
            delay(REPORT_EXPORT_POLL_INTERVAL_MS)
        }
        val statusResult = executeReportingToolObject(
            client = client,
            toolName = "reporting:get_export_status",
            args = mapOf("jobId" to JsonPrimitive(job.jobId)),
            conversationId = conversationId
        )
        job = normalizeReportExportJob(statusResult)
    }
    error("Report PDF export did not finish in time.")
}

private data class ReportExportJobState(
    val jobId: String,
    val artifactId: String,
    val artifactRef: String,
    val status: String,
    val error: String
)

private fun normalizeReportExportJob(value: JsonObject): ReportExportJobState {
    return ReportExportJobState(
        jobId = stringValue(value["jobId"]),
        artifactId = stringValue(value["artifactId"]),
        artifactRef = stringValue(value["artifactRef"]),
        status = stringValue(value["status"]),
        error = stringValue(value["error"])
    )
}

private suspend fun executeReportingToolObject(
    client: AgentlyClient,
    toolName: String,
    args: Map<String, JsonElement>,
    conversationId: String
): JsonObject {
    val raw = client.executeTool(
        name = toolName,
        args = args,
        conversationId = conversationId.ifBlank { null }
    )
    if (raw.isBlank()) {
        error("Reporting tool $toolName returned an empty response.")
    }
    val element = Json.parseToJsonElement(raw)
    return element as? JsonObject
        ?: error("Reporting tool $toolName returned an unexpected response.")
}

private fun decodeReportArtifactBytes(artifact: JsonObject): ByteArray {
    val data = stringValue(artifact["data"])
    if (data.isNotBlank()) {
        return Base64.getDecoder().decode(data)
    }
    val bytes = artifact["bytes"] as? JsonArray ?: return ByteArray(0)
    return bytes.mapNotNull { element ->
        element.jsonPrimitive.contentOrNull?.toIntOrNull()?.toByte()
    }.toByteArray()
}

private fun stringValue(value: Any?): String {
    return when (value) {
        null -> ""
        is String -> value.trim()
        is JsonPrimitive -> value.contentOrNull?.trim().orEmpty()
        is JsonElement -> (value as? JsonPrimitive)?.contentOrNull?.trim().orEmpty()
        else -> value.toString().trim()
    }
}

internal fun reportRuntimeExportErrorMessage(error: Throwable): String {
    val raw = error.message.orEmpty()
    val serverDetail = raw.indexOf('{')
        .takeIf { it >= 0 }
        ?.let { start ->
            runCatching {
                val response = Json.parseToJsonElement(raw.substring(start)) as? JsonObject
                (response?.get("error") as? JsonPrimitive)?.contentOrNull
            }.getOrNull()
        }
        ?.replace("\n", " ")
        ?.trim()
	val detail = serverDetail
		?.trim()
		?.takeIf { it.isNotBlank() }
    return if (detail == null) {
        "Unable to create the report PDF. Please try again."
    } else {
        "Unable to create the report PDF: $detail"
    }
}
