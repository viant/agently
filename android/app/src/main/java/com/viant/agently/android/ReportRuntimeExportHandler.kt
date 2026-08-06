package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.DownloadFileOutput
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.JsonUtil
import kotlinx.coroutines.delay
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
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
            onError(err.message ?: "Unable to export report PDF.")
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

internal data class ReportRuntimeExportArtifact(
    val file: GeneratedFileEntry,
    val downloaded: DownloadFileOutput
)

internal suspend fun exportReportRuntimePdf(
    client: AgentlyClient,
    exportRequest: Map<String, Any?>,
    conversationId: String = ""
): ReportRuntimeExportArtifact {
    val request = normalizeReportRuntimeExportRequest(exportRequest)
    val submitResult = executeReportingToolObject(
        client = client,
        toolName = "reporting:submit_export",
        args = request,
        conversationId = conversationId
    )
    val initialJob = normalizeReportExportJob(submitResult)
    val completedJob = waitForReportExportArtifact(
        client = client,
        initialJob = initialJob,
        conversationId = conversationId
    )
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
    val artifactRef = stringValue(exportRequest["artifactRef"]).ifBlank {
        "report://runtime/${stringValue(exportRequest["title"]).ifBlank { "report" }}"
    }
    return linkedMapOf(
        "artifactRef" to JsonPrimitive(artifactRef),
        "format" to JsonPrimitive("pdf"),
        "scope" to JsonPrimitive("draft"),
        "reportSpec" to JsonUtil.anyToElement(exportRequest["reportSpec"]),
        "reportFill" to JsonUtil.anyToElement(exportRequest["reportFill"]),
        "reportPrint" to JsonUtil.anyToElement(exportRequest["reportPrint"])
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
