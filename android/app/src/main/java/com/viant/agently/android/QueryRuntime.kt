package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.CreateConversationInput
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.DownloadFileOutput
import com.viant.agentlysdk.ExecutionPageState
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.ModelStepState
import com.viant.agentlysdk.ListPendingToolApprovalsInput
import com.viant.agentlysdk.MetadataTargetContext
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.QueryAttachment
import com.viant.agentlysdk.QueryInput
import com.viant.agentlysdk.QueryOutput
import com.viant.agentlysdk.ToolStepState
import com.viant.agentlysdk.UploadFileInput
import com.viant.agentlysdk.WorkspaceMetadata
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.io.ByteArrayInputStream
import java.util.zip.GZIPInputStream

internal data class QueryExecutionResult(
    val metadata: WorkspaceMetadata,
    val conversationId: String,
    val queryOutput: QueryOutput?,
    val generatedFiles: List<GeneratedFileEntry>,
    val pendingApprovals: List<PendingToolApproval>
)

internal data class QuerySuccessState(
    val metadata: WorkspaceMetadata,
    val activeConversationId: String,
    val result: QueryOutput?,
    val streamedMarkdown: String?,
    val generatedFiles: List<GeneratedFileEntry>,
    val pendingApprovals: List<PendingToolApproval>,
    val approvalEdits: Map<String, Map<String, JsonElement>>
)

internal data class ComposerDraftState(
    val prompt: String,
    val attachments: List<ComposerAttachmentDraft>
)

internal data class PreparedQuerySubmission(
    val effectivePrompt: String,
    val entryId: String,
    val pendingEntry: ChatEntry
)

internal fun resolveEffectivePrompt(
    prompt: String,
    attachments: List<ComposerAttachmentDraft>
): String {
    return prompt.trim().ifBlank {
        if (attachments.isNotEmpty()) {
            "Please analyze the attached file(s)."
        } else {
            ""
        }
    }
}

internal fun prepareQuerySubmission(
    draft: ComposerDraftState,
    timestampMs: Long
): PreparedQuerySubmission {
    val effectivePrompt = resolveEffectivePrompt(draft.prompt, draft.attachments)
    require(effectivePrompt.isNotBlank()) { "Enter a message or attach a file before sending." }
    val entryId = "user-$timestampMs"
    return PreparedQuerySubmission(
        effectivePrompt = effectivePrompt,
        entryId = entryId,
        pendingEntry = buildPendingUserChatEntry(
            entryId = entryId,
            prompt = effectivePrompt,
            attachments = draft.attachments,
            timestampMs = timestampMs
        )
    )
}

internal fun buildPendingUserChatEntry(
    entryId: String,
    prompt: String,
    attachments: List<ComposerAttachmentDraft>,
    timestampMs: Long
): ChatEntry {
    return ChatEntry(
        id = entryId,
        role = "user",
        markdown = buildUserComposerMarkdown(
            prompt = prompt,
            attachments = attachments
        ),
        deliveryState = "sending",
        timestampLabel = formatTimestampLabel(timestampMs)
    )
}

internal fun clearComposerDraft(): ComposerDraftState {
    return ComposerDraftState(
        prompt = "",
        attachments = emptyList()
    )
}

internal fun shouldRestoreComposerDraft(
    currentPrompt: String,
    currentAttachments: List<ComposerAttachmentDraft>
): Boolean {
    return currentPrompt.isBlank() && currentAttachments.isEmpty()
}

internal suspend fun uploadComposerAttachments(
    client: AgentlyClient,
    conversationId: String,
    attachments: List<ComposerAttachmentDraft>
): List<QueryAttachment> {
    return attachments.map { attachment ->
        val uploaded = client.uploadFile(
            UploadFileInput(
                conversationId = conversationId,
                name = attachment.name,
                contentType = attachment.mimeType,
                data = attachment.bytes
            )
        )
        QueryAttachment(
            name = attachment.name,
            uri = uploaded.uri,
            size = attachment.bytes.size.toLong(),
            mime = attachment.mimeType
        )
    }
}

internal fun buildArtifactPreview(
    file: GeneratedFileEntry,
    downloaded: DownloadFileOutput
): ArtifactPreview {
    val previewText = if (isPreviewableText(downloaded.contentType, downloaded.name)) {
        downloaded.data.toString(Charsets.UTF_8)
    } else {
        null
    }
    return ArtifactPreview(
        artifactId = file.id,
        name = downloaded.name ?: file.filename ?: file.id.take(12),
        contentType = downloaded.contentType ?: file.mimeType,
        text = previewText,
        sizeBytes = downloaded.data.size
    )
}

internal fun buildPayloadPreview(
    payloadId: String,
    title: String,
    downloaded: DownloadFileOutput
): ArtifactPreview {
    val body = decodePreviewBytes(downloaded.data)
    val previewText = if (isPreviewableText(downloaded.contentType, downloaded.name) || body.isLikelyText()) {
        body.toString(Charsets.UTF_8)
    } else {
        null
    }
    return ArtifactPreview(
        artifactId = payloadId,
        name = title,
        contentType = downloaded.contentType,
        text = previewText,
        sizeBytes = downloaded.data.size
    )
}

internal fun executionPayloadTitles(state: ConversationStateResponse): Map<String, String> {
    val result = linkedMapOf<String, String>()
    state.conversation?.turns.orEmpty().forEach { turn ->
        turn.execution?.pages.orEmpty().forEach { page ->
            collectPayloadTitles(result, page)
        }
    }
    return result
}

private fun collectPayloadTitles(
    result: LinkedHashMap<String, String>,
    page: ExecutionPageState
) {
    page.modelSteps.forEach { step ->
        addPayloadTitle(result, step.requestPayloadId, "llm.request.request")
        addPayloadTitle(result, step.providerRequestPayloadId, "llm.request.providerRequest")
        addPayloadTitle(result, step.responsePayloadId, "llm.request.response")
        addPayloadTitle(result, step.providerResponsePayloadId, "llm.request.providerResponse")
    }
    page.toolSteps.forEach { step ->
        addPayloadTitle(result, step.requestPayloadId, "tool.request")
        addPayloadTitle(result, step.responsePayloadId, "tool.response")
    }
}

private fun addPayloadTitle(
    result: LinkedHashMap<String, String>,
    payloadId: String?,
    title: String
) {
    val id = payloadId?.trim().orEmpty()
    if (id.isNotBlank() && !result.containsKey(id)) {
        result[id] = title
    }
}

internal fun decodePreviewBytes(data: ByteArray): ByteArray {
    if (data.size < 2 || data[0] != 0x1f.toByte() || data[1] != 0x8b.toByte()) {
        return data
    }
    return runCatching {
        GZIPInputStream(ByteArrayInputStream(data)).use { it.readBytes() }
    }.getOrDefault(data)
}

private fun ByteArray.isLikelyText(): Boolean {
    if (isEmpty()) {
        return true
    }
    return none { byte ->
        val value = byte.toInt() and 0xff
        value == 0
    }
}

internal fun buildQuerySuccessState(
    execution: QueryExecutionResult,
    approvalEdits: Map<String, Map<String, JsonElement>>
): QuerySuccessState {
    val queryOutput = execution.queryOutput
    return QuerySuccessState(
        metadata = execution.metadata,
        activeConversationId = queryOutput?.conversationId ?: execution.conversationId,
        result = queryOutput,
        streamedMarkdown = queryOutput?.content?.takeIf { it.isNotBlank() },
        generatedFiles = execution.generatedFiles,
        pendingApprovals = execution.pendingApprovals,
        approvalEdits = trimApprovalEdits(approvalEdits, execution.pendingApprovals)
    )
}

internal suspend fun executeQueryTurn(
    client: AgentlyClient,
    metadata: WorkspaceMetadata?,
    activeConversationId: String?,
    effectiveAgentId: String?,
    prompt: String,
    attachments: List<ComposerAttachmentDraft>,
    queryContext: Map<String, JsonElement>,
    targetContext: MetadataTargetContext
): QueryExecutionResult {
    val workspaceMetadata = metadata ?: client.getWorkspaceMetadata(targetContext)
    val conversationId = activeConversationId ?: client.createConversation(
        CreateConversationInput(
            agentId = effectiveAgentId,
            title = buildConversationTitle(prompt, attachments)
        )
    ).id
    val uploadedAttachments = uploadComposerAttachments(
        client = client,
        conversationId = conversationId,
        attachments = attachments
    )
    val queryOutput = client.query(
        QueryInput(
            conversationId = conversationId,
            agentId = effectiveAgentId,
            model = workspaceMetadata.defaultModel ?: workspaceMetadata.defaults?.model,
            query = prompt,
            attachments = uploadedAttachments,
            context = queryContext
        )
    )
    val generatedFiles = client.listGeneratedFiles(conversationId)
    val pendingApprovals = client.listPendingToolApprovals(
        ListPendingToolApprovalsInput(
            conversationId = conversationId,
            status = "pending",
            limit = 20
        )
    )
    return QueryExecutionResult(
        metadata = workspaceMetadata,
        conversationId = conversationId,
        queryOutput = queryOutput,
        generatedFiles = generatedFiles,
        pendingApprovals = pendingApprovals
    )
}
