package com.viant.agently.android

import androidx.compose.ui.graphics.Color
import com.viant.agentlysdk.AssistantMessageState
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.RenderedContent
import com.viant.agentlysdk.RenderedContentPart
import com.viant.agentlysdk.stream.BufferedMessage
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.agentlysdk.stream.LiveExecutionGroup
import com.viant.forgeandroid.ui.TranscriptCanonicalData
import com.viant.forgeandroid.ui.TranscriptCanonicalReport
import com.viant.forgeandroid.ui.TranscriptEnvelope
import kotlinx.coroutines.CancellationException
import java.io.EOFException
import java.text.SimpleDateFormat
import java.time.Instant
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeParseException
import java.util.Date
import java.util.Locale

private val conversationActivityFormatters: List<DateTimeFormatter> = listOf(
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss.SSSSSS Z z", Locale.US),
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss.SSSSSS Z", Locale.US),
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss.SSS Z z", Locale.US),
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss.SSS Z", Locale.US),
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss Z z", Locale.US),
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss Z", Locale.US)
)

internal data class ChatEntry(
    val id: String,
    val role: String,
    val markdown: String,
    val renderedParts: List<RenderedContentPart>? = null,
    val renderedReports: List<TranscriptCanonicalReport>? = null,
    val streaming: Boolean = false,
    val deliveryState: String? = null,
    val timestampLabel: String? = null
)

internal data class ArtifactPreview(
    val artifactId: String,
    val name: String,
    val contentType: String?,
    val text: String?,
    val sizeBytes: Int
)

internal fun latestAssistantMarkdown(snapshot: ConversationStreamSnapshot): String? {
    return activeAssistantEntry(snapshot)?.markdown
}

internal data class TurnProgressPresentation(
    val title: String,
    val detail: String,
    val activity: String,
    val toolProgress: String?,
    val tokenUsage: String?,
    val canStop: Boolean
)

internal fun turnProgressPresentation(
    loading: Boolean,
    conversationState: ConversationStateResponse?,
    snapshot: ConversationStreamSnapshot?
): TurnProgressPresentation? {
    val activeTurnId = snapshot?.activeTurnId?.trim().orEmpty().ifBlank {
        conversationState?.conversation?.turns.orEmpty()
            .lastOrNull { isPendingTurnStatus(it.status) }
            ?.turnId.orEmpty()
    }
    if (!loading && activeTurnId.isBlank()) return null
    if (activeTurnId.isBlank()) {
        return TurnProgressPresentation(
            title = "Sending request",
            detail = "Connecting to the workspace.",
            activity = "Connecting",
            toolProgress = null,
            tokenUsage = formattedConversationTokenUsage(snapshot, conversationState),
            canStop = false
        )
    }
    val groups = snapshot?.liveExecutionGroupsById.orEmpty().values
        .filter { it.turnId?.trim() == activeTurnId }
        .sortedWith(compareBy<LiveExecutionGroup> { it.sequence ?: Int.MIN_VALUE }.thenBy { it.iteration ?: Int.MIN_VALUE })
    val latestGroup = groups.lastOrNull()
    val narration = sequenceOf(
        latestGroup?.narration,
        snapshot?.bufferedMessages.orEmpty().asReversed().firstOrNull { message ->
            message.role.equals("assistant", ignoreCase = true) &&
                message.turnId?.trim() == activeTurnId &&
                !message.narration.isNullOrBlank()
        }?.narration,
        latestPendingNarration(conversationState)
    ).mapNotNull(::sanitizeAssistantTranscriptText).firstOrNull { it.isNotBlank() }
    val activeToolName = groups.asReversed().asSequence()
        .flatMap { it.toolSteps.asReversed().asSequence() }
        .firstOrNull { isActiveExecutionStatus(it.status) }
        ?.toolName
    val plannedToolName = latestGroup?.toolCallsPlanned?.asReversed()?.firstNotNullOfOrNull { it.toolName }
    val bufferedToolName = snapshot?.bufferedMessages.orEmpty().asReversed().firstOrNull { message ->
        message.turnId?.trim() == activeTurnId && !message.toolName.isNullOrBlank()
    }?.toolName
    val activeModel = groups.asReversed().asSequence()
        .flatMap { it.modelSteps.asReversed().asSequence() }
        .firstOrNull { isActiveExecutionStatus(it.status) }
    val plannerStatus = snapshot?.plannerByTurnId?.get(activeTurnId)?.status?.trim().orEmpty()
    val activity = userFacingToolActivity(activeToolName ?: plannedToolName ?: bufferedToolName)
        ?: when {
            activeAssistantHasVisibleOutput(snapshot) -> "Writing response"
            activeModel != null -> "Thinking"
            plannerStatus.isNotBlank() -> "Planning"
            else -> "Thinking"
        }
    val fallback = when (activity) {
        "Writing response" -> "Preparing the response for display."
        "Planning" -> "Preparing the execution plan."
        "Thinking" -> if (activeModel != null) "The model is analyzing the request." else "Planning the next step."
        else -> "Using a workspace tool."
    }
    val toolSteps = groups.flatMap { it.toolSteps }
    val plannedToolCount = groups.sumOf { it.toolCallsPlanned.size }
    val toolTotal = maxOf(toolSteps.size, plannedToolCount)
    val toolCompleted = toolSteps.count { isTerminalExecutionStatus(it.status) }.coerceAtMost(toolTotal)
    return TurnProgressPresentation(
        title = "Working on your request",
        detail = narration ?: fallback,
        activity = activity,
        toolProgress = toolTotal.takeIf { it > 0 }?.let { "Tools $toolCompleted/$it" },
        tokenUsage = formattedConversationTokenUsage(snapshot, conversationState),
        canStop = true
    )
}

private fun formattedConversationTokenUsage(
    snapshot: ConversationStreamSnapshot?,
    conversationState: ConversationStateResponse?
): String? {
    val streamedTotal = snapshot?.usage?.totalTokens ?: 0
    val persistedTotal = (conversationState?.usage?.totalInputTokens ?: 0) +
        (conversationState?.usage?.totalOutputTokens ?: 0)
    val total = if (streamedTotal > 0) streamedTotal else persistedTotal
    return total.takeIf { it > 0 }?.let {
        "${java.text.NumberFormat.getIntegerInstance(Locale.US).format(it)} tokens"
    }
}

private fun isActiveExecutionStatus(status: String?): Boolean {
    val value = status?.trim()?.lowercase(Locale.US).orEmpty()
    return value.isEmpty() || value in setOf("queued", "pending", "starting", "started", "running", "streaming", "processing")
}

private fun isTerminalExecutionStatus(status: String?): Boolean =
    status?.trim()?.lowercase(Locale.US) in setOf(
        "completed", "succeeded", "success", "failed", "canceled", "cancelled"
    )

private fun isPendingTurnStatus(status: String?): Boolean =
    status?.trim()?.lowercase(Locale.US) in setOf(
        "queued", "pending", "starting", "running", "streaming", "processing",
        "waiting", "waiting_for_model", "waiting_for_tool"
    )

internal fun userFacingToolActivity(rawName: String?): String? {
    val raw = rawName?.trim().orEmpty()
    if (raw.isEmpty()) return null
    val lower = raw.lowercase(Locale.US)
    if ("reporting" in lower && "export" in lower) return "Preparing report export"
    if ("reporting" in lower) return "Reporting"
    if ("diagnostic" in lower) return "Delivery diagnostics"
    if ("forecast" in lower) return "Forecasting"
    if ("hierarchy" in lower) return "Loading hierarchy"
    if ("resource" in lower) return "Reading workspace"
    val leaf = raw.substringAfterLast('/').substringAfterLast(':')
    return leaf.replace('_', ' ').replace('-', ' ')
        .replace(Regex("([a-z0-9])([A-Z])"), "$1 $2")
        .trim()
        .split(Regex("\\s+"))
        .filter { it.isNotBlank() }
        .joinToString(" ") { token -> token.replaceFirstChar { it.uppercase(Locale.US) } }
        .ifBlank { "Workspace tool" }
}

internal fun latestActiveNarration(snapshot: ConversationStreamSnapshot?): String? {
    val activeTurnId = snapshot?.activeTurnId?.trim().orEmpty()
    if (activeTurnId.isBlank()) return null
    return snapshot?.bufferedMessages
        ?.asReversed()
        ?.firstOrNull { message ->
            message.role.equals("assistant", ignoreCase = true) &&
                message.turnId?.trim() == activeTurnId &&
                !message.narration.isNullOrBlank()
        }
        ?.narration
        ?.let(::sanitizeVisibleAssistantText)
}

internal fun activeAssistantHasVisibleOutput(snapshot: ConversationStreamSnapshot?): Boolean {
    val activeTurnId = snapshot?.activeTurnId?.trim().orEmpty()
    if (activeTurnId.isEmpty()) return false
    return snapshot?.bufferedMessages.orEmpty().any { message ->
        message.role.equals("assistant", ignoreCase = true) &&
            message.turnId?.trim() == activeTurnId &&
            (visibleAssistantContent(message).orEmpty().isNotBlank() ||
                snapshot?.liveExecutionGroupsById?.get(message.id)?.renderedContent?.reports?.isNotEmpty() == true)
    }
}

internal fun transcriptWithActiveAssistant(
    transcript: List<ChatEntry>,
    snapshot: ConversationStreamSnapshot?
): List<ChatEntry> {
    val active = snapshot?.let(::activeAssistantEntry) ?: return transcript
    return transcript.filterNot { entry ->
        entry.id == active.id || entry.isTransientAssistantEntry()
    } + active
}

private fun ChatEntry.isTransientAssistantEntry(): Boolean {
    if (!role.equals("assistant", ignoreCase = true)) {
        return false
    }
    if (streaming) {
        return true
    }
    return when (deliveryState?.trim()?.lowercase(Locale.US)) {
        "waiting", "streaming", "sending" -> true
        else -> false
    }
}

private fun activeAssistantEntry(snapshot: ConversationStreamSnapshot): ChatEntry? {
    val activeTurnId = snapshot.activeTurnId?.trim().orEmpty()
    if (activeTurnId.isBlank()) {
        return null
    }
    var latest: BufferedMessage? = null
    var markdown = ""
    var rendered: RenderedContent? = null
    snapshot.bufferedMessages.asReversed().forEach { message ->
        if (latest != null ||
            !message.role.equals("assistant", ignoreCase = true) ||
            message.turnId?.trim() != activeTurnId
        ) {
            return@forEach
        }
        val candidateMarkdown = visibleAssistantContent(message).orEmpty()
        val candidateRendered = snapshot.liveExecutionGroupsById[message.id]?.renderedContent
        if (candidateMarkdown.isNotBlank() || candidateRendered?.reports?.isNotEmpty() == true) {
            latest = message
            markdown = candidateMarkdown
            rendered = candidateRendered
        }
    }
    val visibleLatest = latest ?: return null
    return ChatEntry(
        id = visibleLatest.id,
        role = "assistant",
        markdown = markdown,
        renderedParts = rendered?.parts,
        renderedReports = rendered?.let(::canonicalReports)?.takeIf { it.isNotEmpty() },
        streaming = true,
        timestampLabel = formatTimestampLabel(visibleLatest.createdAt)
    )
}

private fun visibleAssistantContent(message: BufferedMessage): String? {
    val content = sanitizeAssistantTranscriptText(message.content).orEmpty()
    return content.takeIf { it.isNotEmpty() }
}

internal fun updateChatEntryDeliveryState(
    transcript: MutableList<ChatEntry>,
    entryId: String?,
    deliveryState: String?
) {
    if (entryId.isNullOrBlank()) {
        return
    }
    val existingIndex = transcript.indexOfFirst { it.id == entryId }
    if (existingIndex < 0) {
        return
    }
    transcript[existingIndex] = transcript[existingIndex].copy(deliveryState = deliveryState)
}

internal fun markLatestSubmittedUserEntryDelivered(transcript: MutableList<ChatEntry>) {
    val index = transcript.indexOfLast { entry ->
        entry.role.equals("user", ignoreCase = true) &&
            entry.deliveryState?.lowercase(Locale.US) in setOf("sending", "waiting", "pending")
    }
    if (index >= 0) {
        transcript[index] = transcript[index].copy(deliveryState = null)
    }
}

internal fun submittedTurnWasAccepted(state: ConversationStateResponse, prompt: String): Boolean {
    val expected = prompt.trim()
    if (expected.isEmpty()) return false
    return state.conversation?.turns.orEmpty().any { turn ->
        turn.user?.content?.trim() == expected || turn.users.any { it.content?.trim() == expected }
    }
}

internal fun hasPendingConversationTurn(state: ConversationStateResponse?): Boolean =
    state?.conversation?.turns.orEmpty().any { turn ->
        turn.status?.trim()?.lowercase(Locale.US) in setOf(
            "queued", "pending", "starting", "running", "streaming", "processing",
            "waiting", "waiting_for_model", "waiting_for_tool", "waiting_for_user"
        )
    }

internal fun latestPendingNarration(state: ConversationStateResponse?): String? {
    val turn = state?.conversation?.turns.orEmpty().lastOrNull { candidate ->
        candidate.status?.trim()?.lowercase(Locale.US) in setOf(
            "queued", "pending", "starting", "running", "streaming", "processing",
            "waiting", "waiting_for_model", "waiting_for_tool", "waiting_for_user"
        )
    } ?: return null
    return sanitizeAssistantTranscriptText(turn.assistant?.narration?.content)
        ?.takeIf { it.isNotBlank() }
}

internal fun transcriptFromState(state: ConversationStateResponse): List<ChatEntry> {
    val entries = mutableListOf<ChatEntry>()
    state.conversation?.turns?.forEach { turn ->
        val user = turn.user
        user?.content?.takeIf { it.isNotBlank() }?.let { content ->
            entries.add(
                ChatEntry(
                    id = user.messageId,
                    role = "user",
                    markdown = content,
                    timestampLabel = formatTimestampLabel(turn.createdAt)
                )
            )
        }
        val assistantMessages = listOfNotNull(turn.assistant?.narration, turn.assistant?.final)
        val assistantId = turn.assistant?.final?.messageId ?: turn.assistant?.narration?.messageId
        // Narration is live progress and is already rendered by the single
        // global turn-status card. Do not duplicate it in the transcript. For
        // old terminal turns without a final message, preserve it as fallback.
        val finalContent = sanitizeAssistantTranscriptText(turn.assistant?.final?.content).orEmpty()
        val narrationFallback = if (isPendingTurnStatus(turn.status)) {
            ""
        } else {
            sanitizeAssistantTranscriptText(turn.assistant?.narration?.content).orEmpty()
        }
        val assistantContent = (finalContent.ifBlank { narrationFallback }).trim()
        val renderedReports = canonicalAssistantReports(assistantMessages)
        if (!assistantId.isNullOrBlank() && (assistantContent.isNotBlank() || !renderedReports.isNullOrEmpty())) {
            entries.add(
                ChatEntry(
                    id = assistantId,
                    role = "assistant",
                    markdown = assistantContent,
                    renderedParts = canonicalAssistantParts(assistantMessages),
                    renderedReports = renderedReports,
                    streaming = false,
                    timestampLabel = formatTimestampLabel(turn.createdAt)
                )
            )
        }
    }
    return entries
}

private fun canonicalAssistantParts(messages: List<AssistantMessageState>): List<RenderedContentPart>? {
    if (messages.none { it.renderedContent != null }) return null
    return messages.flatMap { message ->
        message.renderedContent?.parts.orEmpty().ifEmpty {
            sanitizeAssistantTranscriptText(message.content)
                ?.let { listOf(RenderedContentPart(kind = "markdown", text = it)) }
                .orEmpty()
        }
    }
}

internal fun sanitizeAssistantTranscriptText(value: String?): String? {
    val text = value ?: return null
    return sanitizeVisibleAssistantText(TranscriptEnvelope.suppressProgressiveTransport(text))
}

private fun canonicalAssistantReports(messages: List<AssistantMessageState>): List<TranscriptCanonicalReport>? {
    return messages.flatMap { message ->
        message.renderedContent?.let(::canonicalReports).orEmpty()
    }.takeIf { it.isNotEmpty() }
}

private fun canonicalReports(rendered: RenderedContent): List<TranscriptCanonicalReport> =
    rendered.reports.mapNotNull { report ->
        val source = report.source ?: return@mapNotNull null
        TranscriptCanonicalReport(
            scope = report.scope,
            id = report.id,
            grammar = report.grammar ?: "dashboard-v1",
            status = report.status,
            sequence = report.sequence,
            resetVersion = report.resetVersion,
            source = source,
            dataSources = report.dataSources.mapValues { (_, data) ->
                TranscriptCanonicalData(
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

private fun isBenignLifecycleCancellation(err: Throwable?): Boolean {
    if (err == null) {
        return false
    }
    if (err is CancellationException) {
        return true
    }
    val message = err.message?.trim().orEmpty()
    return message.contains("left the composition", ignoreCase = true) ||
        message.contains("job was cancelled", ignoreCase = true) ||
        message.contains("job was canceled", ignoreCase = true)
}

internal fun visibleAppError(err: Throwable?): String? {
    if (isBenignLifecycleCancellation(err)) {
        return null
    }
    if (generateSequence(err) { it.cause }.any { it is EOFException }) {
        return "The connection ended before the report finished loading. Refresh to try again."
    }
    val detail = err?.message?.trim().orEmpty()
    if (detail.contains("api key is required", ignoreCase = true)) {
        return "The workspace model is not configured. Ask an administrator to add the model API key, then try again."
    }
    if (detail.contains("/v1/agent/query", ignoreCase = true) ||
        detail.contains("failed to stream", ignoreCase = true)
    ) {
        return "The assistant could not start this request. Try again, or contact the workspace administrator if it continues."
    }
    return detail.ifBlank { err?.toString().orEmpty() }
}

internal fun isPreviewableText(contentType: String?, name: String?): Boolean {
    val normalizedType = contentType?.lowercase().orEmpty()
    val normalizedName = name?.lowercase().orEmpty()
    return normalizedType.startsWith("text/") ||
        normalizedType.contains("json") ||
        normalizedType.contains("xml") ||
        normalizedType.contains("javascript") ||
        normalizedName.endsWith(".md") ||
        normalizedName.endsWith(".txt") ||
        normalizedName.endsWith(".json") ||
        normalizedName.endsWith(".yaml") ||
        normalizedName.endsWith(".yml") ||
        normalizedName.endsWith(".xml") ||
        normalizedName.endsWith(".csv")
}

internal fun formatTimestampLabel(value: Long?): String? {
    if (value == null || value <= 0) return null
    return SimpleDateFormat("h:mm a", Locale.US).format(Date(value))
}

internal fun formatTimestampLabel(value: String?): String? {
    return formatTimestampLabel(parseConversationActivityInstantMillis(value))
}

internal fun formatConversationRecency(value: String?): String? {
    val instant = parseConversationActivityInstantMillis(value) ?: return formatTimestampLabel(value)
    val diffMinutes = ((System.currentTimeMillis() - instant) / 60_000L).coerceAtLeast(0L)
    return when {
        diffMinutes < 1 -> "Now"
        diffMinutes < 60 -> "${diffMinutes}m"
        diffMinutes < 24 * 60 -> "${diffMinutes / 60}h"
        diffMinutes < 7 * 24 * 60 -> "${diffMinutes / (24 * 60)}d"
        else -> formatTimestampLabel(instant)
    }
}

internal fun parseConversationActivityInstantMillis(value: String?): Long? {
    val raw = value?.trim().orEmpty()
    if (raw.isBlank()) return null
    raw.toLongOrNull()?.let { return it }
    val sanitized = raw.replace(Regex("\\s+m=\\+.*$"), "")
    runCatching { OffsetDateTime.parse(sanitized).toInstant().toEpochMilli() }.getOrNull()?.let { return it }
    runCatching { Instant.parse(sanitized).toEpochMilli() }.getOrNull()?.let { return it }
    for (formatter in conversationActivityFormatters) {
        try {
            return OffsetDateTime.parse(sanitized, formatter).toInstant().toEpochMilli()
        } catch (_: DateTimeParseException) {
        }
    }
    return null
}

internal fun conversationToneColor(status: String?): Color {
    return when (status?.trim()?.lowercase()) {
        "failed", "error", "rejected" -> Color(0xFFB42318)
        "running", "pending", "queued" -> Color(0xFFB54708)
        "done", "completed", "approved" -> Color(0xFF067647)
        else -> Color(0xFF98A2B3)
    }
}
