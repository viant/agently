package com.viant.agently.android

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
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
    val turnId: String? = null,
    val renderedParts: List<RenderedContentPart>? = null,
    val renderedReports: List<TranscriptCanonicalReport>? = null,
    val diagnosticMessages: List<String> = emptyList(),
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

internal fun streamSnapshotHasAcceptedActivity(snapshot: ConversationStreamSnapshot): Boolean =
    !snapshot.activeTurnId.isNullOrBlank() ||
        snapshot.bufferedMessages.isNotEmpty() ||
        snapshot.liveExecutionGroupsById.isNotEmpty() ||
        snapshot.pendingElicitation != null

internal data class TurnProgressPresentation(
    val title: String,
    val detail: String,
    val activity: String,
    val toolProgress: String?,
    val toolDetails: List<TurnProgressToolDetail> = emptyList(),
    val tokenUsage: String?,
    val tokenDetails: TurnProgressTokenDetails? = null,
    val isWaitingForUser: Boolean = false,
    val canStop: Boolean
)

internal data class TurnProgressToolDetail(val id: String, val name: String, val status: String)
internal data class TurnProgressTokenModel(
    val id: String,
    val label: String,
    val total: Int,
    val input: Int? = null,
    val output: Int? = null,
    val cachedInput: Int? = null,
    val reasoning: Int? = null,
    val embedding: Int? = null
)
internal data class TurnProgressTokenDetails(
    val scope: String,
    val total: Int,
    val input: Int,
    val output: Int,
    val cachedInput: Int,
    val reasoning: Int,
    val embedding: Int,
    val models: List<TurnProgressTokenModel>
)

internal fun progressStatusAnnotatedText(markdown: String): AnnotatedString = buildAnnotatedString {
    val lines = markdown.lineSequence()
        .map(String::trim)
        .filter(String::isNotBlank)
        .toList()
    lines.forEachIndexed { index, rawLine ->
        if (index > 0) append('\n')
        val heading = Regex("^#{1,6}\\s+").find(rawLine)
        val withoutHeading = heading?.let { rawLine.removeRange(it.range) } ?: rawLine
        val normalized = when {
            withoutHeading.startsWith("- ") || withoutHeading.startsWith("* ") ->
                "• ${withoutHeading.drop(2).trimStart()}"
            else -> withoutHeading
        }
        val lineStart = length
        appendProgressInlineMarkdown(normalized)
        if (heading != null && length > lineStart) {
            addStyle(SpanStyle(fontWeight = FontWeight.Bold), lineStart, length)
        }
    }
}

private fun AnnotatedString.Builder.appendProgressInlineMarkdown(value: String) {
    var cursor = 0
    while (cursor < value.length) {
        val boldStart = value.indexOf("**", cursor)
        if (boldStart < 0) {
            append(value.substring(cursor))
            return
        }
        append(value.substring(cursor, boldStart))
        val boldEnd = value.indexOf("**", boldStart + 2)
        if (boldEnd < 0) {
            append(value.substring(boldStart))
            return
        }
        val start = length
        append(value.substring(boldStart + 2, boldEnd))
        addStyle(SpanStyle(fontWeight = FontWeight.Bold), start, length)
        cursor = boldEnd + 2
    }
}

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
        val tokenDetails = progressTokenDetails(emptyList(), snapshot, conversationState)
        return TurnProgressPresentation(
            title = "Sending request",
            detail = "Connecting to the workspace.",
            activity = "Connecting",
            toolProgress = null,
            tokenUsage = formattedTokenUsage(tokenDetails),
            tokenDetails = tokenDetails,
            canStop = false
        )
    }
    val groups = snapshot?.liveExecutionGroupsById.orEmpty().values
        .filter { it.turnId?.trim() == activeTurnId }
        .sortedWith(compareBy<LiveExecutionGroup> { it.sequence ?: Int.MIN_VALUE }.thenBy { it.iteration ?: Int.MIN_VALUE })
    val toolDetails = progressToolDetails(groups)
    val activeTurnStatus = conversationState?.conversation?.turns.orEmpty()
        .lastOrNull { it.turnId.trim() == activeTurnId }
        ?.status
    val waitingForUser = snapshot?.pendingElicitation != null || isWaitingForUserStatus(activeTurnStatus)
    if (waitingForUser) {
        val tokenDetails = progressTokenDetails(groups, snapshot, conversationState)
        return TurnProgressPresentation(
            title = "Needs your input",
            detail = "Review the requested action to continue.",
            activity = "Needs your input",
            toolProgress = explicitToolProgress(groups),
            toolDetails = toolDetails,
            tokenUsage = formattedTokenUsage(tokenDetails),
            tokenDetails = tokenDetails,
            isWaitingForUser = true,
            canStop = false
        )
    }
    val activeTools = groups.flatMap { it.toolSteps }.filter { isActiveExecutionStatus(it.status) }
    val activeModel = groups.asReversed().asSequence()
        .flatMap { it.modelSteps.asReversed().asSequence() }
        .firstOrNull { isActiveExecutionStatus(it.status) }
    val plannerStatus = snapshot?.plannerByTurnId?.get(activeTurnId)?.status?.trim().orEmpty()
    val activity = when {
        activeTools.size == 1 -> userFacingToolActivity(activeTools.single().toolName) ?: "Using tool"
        activeTools.size > 1 -> "Calling tools"
        else -> when {
            activeAssistantHasVisibleOutput(snapshot) -> "Writing response"
            activeModel != null -> "Thinking"
            plannerStatus.isNotBlank() -> "Planning"
            else -> "Thinking"
        }
    }
    val fallback = when (activity) {
        "Writing response" -> "Preparing the response for display."
        "Planning" -> "Preparing the execution plan."
        "Thinking" -> if (activeModel != null) "The model is analyzing the request." else "Planning the next step."
        else -> "Using a workspace tool."
    }
    val toolProgress = explicitToolProgress(groups)
    val tokenDetails = progressTokenDetails(groups, snapshot, conversationState)
    return TurnProgressPresentation(
        title = "Working on your request",
        detail = fallback,
        activity = activity,
        toolProgress = toolProgress,
        toolDetails = toolDetails,
        tokenUsage = formattedTokenUsage(tokenDetails),
        tokenDetails = tokenDetails,
        canStop = true
    )
}

private fun progressTokenDetails(
    groups: List<LiveExecutionGroup>,
    snapshot: ConversationStreamSnapshot?,
    conversationState: ConversationStateResponse?
): TurnProgressTokenDetails? {
    val modelRows = groups.flatMap { it.modelSteps }.mapNotNull { step ->
        step.usage?.takeIf { (it.totalTokens ?: 0) > 0 }?.let { step to it }
    }
    if (modelRows.isNotEmpty()) {
        return TurnProgressTokenDetails(
            scope = "turn",
            total = modelRows.sumOf { it.second.totalTokens ?: 0 },
            input = modelRows.sumOf { it.second.inputTokens ?: 0 },
            output = modelRows.sumOf { it.second.outputTokens ?: 0 },
            cachedInput = modelRows.sumOf { it.second.cachedInputTokens ?: 0 },
            reasoning = modelRows.sumOf { it.second.reasoningTokens ?: 0 },
            embedding = modelRows.sumOf { it.second.embeddingTokens ?: 0 },
            models = modelRows.map { (step, usage) ->
                TurnProgressTokenModel(
                    id = step.modelCallId,
                    label = listOfNotNull(step.provider, step.model).joinToString("/"),
                    total = usage.totalTokens ?: 0,
                    input = usage.inputTokens,
                    output = usage.outputTokens,
                    cachedInput = usage.cachedInputTokens,
                    reasoning = usage.reasoningTokens,
                    embedding = usage.embeddingTokens
                )
            }.sortedByDescending { it.total }
        )
    }
    val conversationUsage = conversationState?.usage
    val input = snapshot?.usage?.inputTokens ?: conversationUsage?.totalInputTokens ?: 0
    val output = snapshot?.usage?.outputTokens ?: conversationUsage?.totalOutputTokens ?: 0
    val embedding = snapshot?.usage?.embeddingTokens ?: conversationUsage?.totalEmbeddingTokens ?: 0
    val total = snapshot?.usage?.totalTokens ?: conversationUsage?.totalTokens ?: (input + output + embedding)
    if (total <= 0) return null
    val models = conversationUsage?.models.orEmpty().mapIndexed { index, model ->
        val role = model.executionRole?.trim().orEmpty()
        TurnProgressTokenModel(
            id = "${model.provider.orEmpty()}:${model.model}:$role:$index",
            label = buildString {
                append(listOfNotNull(model.provider, model.model).filter(String::isNotBlank).joinToString("/"))
                if (role.isNotEmpty()) append(" · $role")
            },
            total = model.totalTokens ?: ((model.inputTokens ?: 0) + (model.outputTokens ?: 0)),
            input = model.inputTokens,
            output = model.outputTokens,
            cachedInput = model.cachedInputTokens,
            reasoning = model.reasoningTokens,
            embedding = null
        )
    }.sortedByDescending { it.total }
    return TurnProgressTokenDetails(
        "conversation",
        total,
        input,
        output,
        conversationUsage?.totalCachedInputTokens ?: 0,
        conversationUsage?.totalReasoningTokens ?: 0,
        embedding,
        models
    )
}

private fun formattedTokenUsage(details: TurnProgressTokenDetails?): String? {
    details ?: return null
    return "${"%,d".format(details.total)} ${if (details.scope == "turn") "turn tokens" else "total tokens"}"
}

private fun explicitToolProgress(groups: List<LiveExecutionGroup>): String? {
    val statuses = linkedMapOf<String, String>()
    var identityComplete = true
    groups.forEach { group ->
        group.toolCallsPlanned.forEach { planned ->
            val id = planned.toolCallId?.trim().orEmpty()
            if (id.isEmpty()) identityComplete = false else statuses.putIfAbsent(id, "queued")
        }
        group.toolSteps.forEach { step ->
            val id = step.toolCallId?.trim().orEmpty()
            if (id.isEmpty()) identityComplete = false else statuses[id] = step.status?.trim()?.lowercase(Locale.US).orEmpty()
        }
    }
    if (!identityComplete || statuses.isEmpty()) return null
    val done = statuses.values.count { it in setOf("completed", "done", "success", "succeeded") }
    val failed = statuses.values.count { it in setOf("canceled", "cancelled", "declined", "error", "failed", "terminated", "timed_out", "timeout") }
    val queued = statuses.values.count { it in setOf("open", "pending", "planned", "queued", "waiting") }
    val active = statuses.size - done - failed - queued
    return buildList {
        add("$done/${statuses.size} done")
        if (active > 0) add("$active active")
        if (queued > 0) add("$queued queued")
        if (failed > 0) add("$failed failed")
    }.joinToString(" · ")
}

private fun progressToolDetails(groups: List<LiveExecutionGroup>): List<TurnProgressToolDetail> {
    val rows = linkedMapOf<String, TurnProgressToolDetail>()
    groups.forEach { group ->
        group.toolCallsPlanned.forEach { planned ->
            val id = planned.toolCallId?.trim().orEmpty()
            if (id.isNotEmpty()) {
                rows[id] = TurnProgressToolDetail(
                    id = id,
                    name = planned.toolName?.trim().takeUnless { it.isNullOrEmpty() } ?: "Tool",
                    status = rows[id]?.status ?: "queued"
                )
            }
        }
        group.toolSteps.forEach { step ->
            val id = step.toolCallId?.trim().orEmpty()
            if (id.isNotEmpty()) {
                rows[id] = TurnProgressToolDetail(
                    id = id,
                    name = step.toolName?.trim().takeUnless { it.isNullOrEmpty() } ?: rows[id]?.name ?: "Tool",
                    status = step.status?.trim()?.lowercase(Locale.US).takeUnless { it.isNullOrEmpty() } ?: rows[id]?.status ?: "running"
                )
            }
        }
    }
    return rows.values.sortedWith(compareBy<TurnProgressToolDetail> { it.status }.thenBy { it.name.lowercase(Locale.US) })
}

private fun isActiveExecutionStatus(status: String?): Boolean {
    val value = status?.trim()?.lowercase(Locale.US).orEmpty()
    return value.isEmpty() || value in setOf("active", "executing", "in_progress", "processing", "starting", "started", "running", "streaming")
}

private fun isWaitingForUserStatus(status: String?): Boolean =
    status?.trim()?.lowercase(Locale.US) in setOf("blocked", "eliciting", "waiting_for_user")

private fun isTerminalExecutionStatus(status: String?): Boolean =
    status?.trim()?.lowercase(Locale.US) in setOf(
        "completed", "succeeded", "success", "failed", "canceled", "cancelled"
    )

private fun isPendingTurnStatus(status: String?): Boolean =
    status?.trim()?.lowercase(Locale.US) in setOf(
        "queued", "pending", "starting", "running", "streaming", "processing",
        "waiting", "waiting_for_model", "waiting_for_tool", "waiting_for_user", "blocked", "eliciting"
    )

internal fun userFacingToolActivity(rawName: String?): String? {
    val raw = rawName?.trim().orEmpty()
    if (raw.isEmpty()) return null
    val encoded = raw.any { it == '/' || it == ':' || it == '_' || it == '-' } ||
        Regex("[a-z0-9][A-Z]").containsMatchIn(raw)
    if (!encoded) return raw
    return raw.replace('/', ' ').replace(':', ' ').replace('_', ' ').replace('-', ' ')
        .replace(Regex("([a-z0-9])([A-Z])"), "$1 $2")
        .trim()
        .split(Regex("\\s+"))
        .filter { it.isNotBlank() }
        .joinToString(" ") { token -> token.replaceFirstChar { it.uppercase(Locale.US) } }
        .takeIf { it.isNotBlank() }
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
    val activeTurnId = snapshot?.activeTurnId?.trim().orEmpty()
    if (snapshot == null || activeTurnId.isBlank()) return transcript
    val active = assistantEntryForTurn(snapshot, activeTurnId, streaming = true) ?: return transcript
    return transcript.filterNot { entry ->
        entry.id == active.id ||
            (entry.isTransientAssistantEntry() &&
                (entry.turnId.isNullOrBlank() || entry.turnId == activeTurnId))
    } + active
}

internal fun commitAssistantTurnFromSnapshot(
    transcript: MutableList<ChatEntry>,
    snapshot: ConversationStreamSnapshot,
    turnId: String
): Boolean {
    val completed = assistantEntryForTurn(snapshot, turnId.trim(), streaming = false) ?: return false
    val replaceIndexes = transcript.indices.filter { index ->
        val entry = transcript[index]
        entry.role.equals("assistant", ignoreCase = true) &&
            (entry.id == completed.id || entry.turnId == turnId ||
                (entry.turnId.isNullOrBlank() && entry.isTransientAssistantEntry()))
    }
    val insertionIndex = replaceIndexes.firstOrNull() ?: transcript.size
    replaceIndexes.asReversed().forEach(transcript::removeAt)
    transcript.add(insertionIndex.coerceAtMost(transcript.size), completed)
    return true
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
    return assistantEntryForTurn(snapshot, activeTurnId, streaming = true)
}

private data class AssistantTurnFragment(
    val message: BufferedMessage,
    val markdown: String,
    val rendered: RenderedContent?
)

private fun assistantEntryForTurn(
    snapshot: ConversationStreamSnapshot,
    turnId: String,
    streaming: Boolean
): ChatEntry? {
    if (turnId.isBlank()) {
        return null
    }
    val fragments = snapshot.bufferedMessages.mapNotNull { message ->
        if (!message.role.equals("assistant", ignoreCase = true) || message.turnId?.trim() != turnId) {
            return@mapNotNull null
        }
        val candidateMarkdown = visibleAssistantContent(message).orEmpty()
        val candidateRendered = snapshot.liveExecutionGroupsById[message.id]?.renderedContent
        if (candidateMarkdown.isNotBlank() || candidateRendered?.reports?.isNotEmpty() == true || candidateRendered?.diagnostics?.isNotEmpty() == true) {
            AssistantTurnFragment(message, candidateMarkdown, candidateRendered)
        } else {
            null
        }
    }
    if (fragments.isEmpty()) return null

    val hasCanonicalContent = fragments.any { fragment ->
        fragment.rendered?.parts?.isNotEmpty() == true || fragment.rendered?.reports?.isNotEmpty() == true
    }
    val renderedParts = if (hasCanonicalContent) {
        buildList<RenderedContentPart> {
            fragments.forEach { fragment ->
                val fragmentParts = fragment.rendered?.parts.orEmpty().ifEmpty {
                    fragment.markdown.takeIf { it.isNotBlank() }
                        ?.let { listOf(RenderedContentPart(kind = "markdown", text = it)) }
                        .orEmpty()
                }
                if (isNotEmpty() && fragmentParts.isNotEmpty()) {
                    val first = fragmentParts.first()
                    if (first.kind.equals("markdown", ignoreCase = true)) {
                        add(first.copy(text = "\n\n${first.text.orEmpty().trimStart()}"))
                        addAll(fragmentParts.drop(1))
                        return@forEach
                    }
                    add(RenderedContentPart(kind = "markdown", text = "\n\n"))
                }
                addAll(fragmentParts)
            }
        }
    } else {
        null
    }
    val reportsByIdentity = linkedMapOf<String, TranscriptCanonicalReport>()
    fragments.forEach { fragment ->
        fragment.rendered?.let(::canonicalReports).orEmpty().forEach { report ->
            reportsByIdentity["${report.scope}\u0000${report.id}"] = report
        }
    }
    val diagnosticMessages = fragments.flatMap { fragment ->
        fragment.rendered?.diagnostics.orEmpty().mapNotNull { it.message.trim().takeIf(String::isNotEmpty) }
    }
    val visibleLatest = fragments.last().message
    return ChatEntry(
        id = visibleLatest.id,
        role = "assistant",
        markdown = fragments.mapNotNull { it.markdown.takeIf(String::isNotBlank) }.joinToString("\n\n"),
        turnId = turnId,
        renderedParts = renderedParts,
        renderedReports = reportsByIdentity.values.toList().takeIf { it.isNotEmpty() },
        diagnosticMessages = diagnosticMessages,
        streaming = streaming,
        timestampLabel = formatTimestampLabel(visibleLatest.createdAt)
    )
}

private fun visibleAssistantContent(message: BufferedMessage): String? {
    val content = sanitizeAssistantTranscriptText(message.content).orEmpty()
    if (content.isNotEmpty()) return content
    return sanitizeAssistantTranscriptText(message.narration)?.takeIf { it.isNotEmpty() }
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
                    turnId = turn.turnId,
                    timestampLabel = formatTimestampLabel(turn.createdAt)
                )
            )
        }
        val assistantMessages = listOfNotNull(turn.assistant?.narration, turn.assistant?.final)
        val assistantId = turn.assistant?.final?.messageId ?: turn.assistant?.narration?.messageId
        // Narration is the active assistant bubble. The global turn-status card
        // carries compact metadata only; final content replaces narration.
        val finalContent = sanitizeAssistantTranscriptText(turn.assistant?.final?.content).orEmpty()
        val narrationFallback = sanitizeAssistantTranscriptText(turn.assistant?.narration?.content).orEmpty()
        val assistantContent = (finalContent.ifBlank { narrationFallback }).trim()
        val renderedReports = canonicalAssistantReports(assistantMessages)
        val diagnosticMessages = assistantMessages.flatMap { message ->
            message.renderedContent?.diagnostics.orEmpty().mapNotNull { it.message.trim().takeIf(String::isNotEmpty) }
        }
        if (!assistantId.isNullOrBlank() && (assistantContent.isNotBlank() || !renderedReports.isNullOrEmpty() || diagnosticMessages.isNotEmpty())) {
            entries.add(
                ChatEntry(
                    id = assistantId,
                    role = "assistant",
                    markdown = assistantContent,
                    turnId = turn.turnId,
                    renderedParts = canonicalAssistantParts(assistantMessages),
                    renderedReports = renderedReports,
                    diagnosticMessages = diagnosticMessages,
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
    return buildList {
        messages.forEach { message ->
            val messageParts = message.renderedContent?.parts.orEmpty().ifEmpty {
            sanitizeAssistantTranscriptText(message.content)
                ?.let { listOf(RenderedContentPart(kind = "markdown", text = it)) }
                .orEmpty()
            }
            if (isNotEmpty() && messageParts.isNotEmpty()) {
                val first = messageParts.first()
                if (first.kind.equals("markdown", ignoreCase = true)) {
                    add(first.copy(text = "\n\n${first.text.orEmpty().trimStart()}"))
                    addAll(messageParts.drop(1))
                    return@forEach
                }
                add(RenderedContentPart(kind = "markdown", text = "\n\n"))
            }
            addAll(messageParts)
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
