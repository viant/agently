package com.viant.agently.android

import androidx.compose.ui.graphics.Color
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.stream.BufferedMessage
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import kotlinx.coroutines.CancellationException
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
    val latest = snapshot.bufferedMessages
        .asReversed()
        .firstOrNull { message ->
            message.role.equals("assistant", ignoreCase = true) &&
                (!message.content.isNullOrBlank() || !message.narration.isNullOrBlank())
        }
        ?: return null
    return combineAssistantMarkdown(latest)
}

private fun combineAssistantMarkdown(message: BufferedMessage): String? {
    val narration = message.narration?.trim().orEmpty()
    val content = message.content?.trim().orEmpty()
    return when {
        narration.isNotEmpty() && content.isNotEmpty() -> "$narration\n\n$content"
        content.isNotEmpty() -> content
        narration.isNotEmpty() -> narration
        else -> null
    }
}

internal fun syncAssistantTranscript(
    transcript: MutableList<ChatEntry>,
    snapshot: ConversationStreamSnapshot
) {
    val assistantMessages = snapshot.bufferedMessages
        .filter { it.role.equals("assistant", ignoreCase = true) }
        .sortedBy { it.createdAt ?: it.id }

    assistantMessages.forEach { message ->
        val markdown = combineAssistantMarkdown(message) ?: return@forEach
        val existingIndex = transcript.indexOfFirst { it.id == message.id }
        val syntheticIndex = transcript.indexOfFirst { entry ->
            entry.id.startsWith("assistant-final-") &&
                entry.role.equals("assistant", ignoreCase = true) &&
                entry.markdown == markdown
        }
        val updated = ChatEntry(
            id = message.id,
            role = "assistant",
            markdown = markdown,
            streaming = snapshot.activeTurnId != null && message.turnId == snapshot.activeTurnId,
            timestampLabel = formatTimestampLabel(message.createdAt)
        )
        if (existingIndex >= 0) {
            transcript[existingIndex] = updated
        } else if (syntheticIndex >= 0) {
            transcript[syntheticIndex] = updated
        } else {
            transcript.add(updated)
        }
    }
}

internal fun syncAssistantResult(
    transcript: MutableList<ChatEntry>,
    messageId: String?,
    markdown: String
) {
    if (markdown.isBlank()) return
    val id = messageId?.takeIf { it.isNotBlank() } ?: "assistant-final-${System.currentTimeMillis()}"
    val existingIndex = transcript.indexOfFirst { it.id == id }
    val updated = ChatEntry(
        id = id,
        role = "assistant",
        markdown = markdown,
        streaming = false,
        timestampLabel = formatTimestampLabel(System.currentTimeMillis())
    )
    if (existingIndex >= 0) {
        transcript[existingIndex] = updated
    } else {
        transcript.add(updated)
    }
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
        val assistantId = turn.assistant?.final?.messageId ?: turn.assistant?.narration?.messageId
        val assistantContent = buildString {
            val narration = turn.assistant?.narration?.content?.trim().orEmpty()
            val final = turn.assistant?.final?.content?.trim().orEmpty()
            if (narration.isNotEmpty()) {
                append(narration)
            }
            if (final.isNotEmpty()) {
                if (isNotEmpty()) append("\n\n")
                append(final)
            }
        }.trim()
        if (!assistantId.isNullOrBlank() && assistantContent.isNotBlank()) {
            entries.add(
                ChatEntry(
                    id = assistantId,
                    role = "assistant",
                    markdown = assistantContent,
                    streaming = false,
                    timestampLabel = formatTimestampLabel(turn.createdAt)
                )
            )
        }
    }
    return entries
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
    return err?.message ?: err?.toString()
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
