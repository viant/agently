package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.FeedDataResponse
import com.viant.agentlysdk.stream.ActiveFeed
import com.viant.forgeandroid.runtime.ForgeRuntime
import kotlinx.serialization.json.JsonElement

internal data class FeedTextPreview(
    val title: String,
    val subtitle: String,
    val content: String
)

internal fun visibleFeeds(feeds: List<ActiveFeed>): List<ActiveFeed> {
    return feeds.distinctBy { it.feedId }.sortedBy { it.title.lowercase() }
}

internal suspend fun loadFeedPayload(
    client: AgentlyClient,
    feedId: String,
    conversationId: String
): FeedDataResponse {
    return client.getFeedData(feedId, conversationId)
}

internal fun registerFeedInteractionHandlers(
    forgeRuntime: ForgeRuntime,
    client: AgentlyClient,
    onError: (String?) -> Unit,
    onPreview: (FeedTextPreview?) -> Unit,
    onRefreshRequested: () -> Unit
) {
    registerWorkspacePreviewHandler(
        forgeRuntime = forgeRuntime,
        name = "chat.explorerOpen",
        emptySelectionMessage = "Select a file to preview.",
        failureMessage = "Unable to open file.",
        onError = onError,
        onPreview = onPreview
    ) { row ->
        val uri = rowLocation(row) ?: return@registerWorkspacePreviewHandler null
        FeedTextPreview(
            title = uri.substringAfterLast('/').ifBlank { uri },
            subtitle = uri,
            content = client.downloadWorkspaceFile(uri)
        )
    }

    registerWorkspacePreviewHandler(
        forgeRuntime = forgeRuntime,
        name = "chat.onChangedFileSelect",
        emptySelectionMessage = "Select a changed file to preview.",
        failureMessage = "Unable to preview changed file.",
        onError = onError,
        onPreview = onPreview
    ) { row ->
        val currentUri = rowLocation(row) ?: return@registerWorkspacePreviewHandler null
        val previousUri = rowValue(row, "origUri", "origUrl")
        buildChangedFilePreview(client, currentUri, previousUri)
    }

    registerToolRefreshHandler(
        forgeRuntime = forgeRuntime,
        name = "chat.runPatchCommit",
        tool = "system_patch-commit",
        failureMessage = "Patch commit failed.",
        client = client,
        onError = onError,
        onRefreshRequested = onRefreshRequested
    )

    registerToolRefreshHandler(
        forgeRuntime = forgeRuntime,
        name = "chat.runPatchRollback",
        tool = "system_patch-rollback",
        failureMessage = "Patch rollback failed.",
        client = client,
        onError = onError,
        onRefreshRequested = onRefreshRequested
    )
}

private fun registerWorkspacePreviewHandler(
    forgeRuntime: ForgeRuntime,
    name: String,
    emptySelectionMessage: String,
    failureMessage: String,
    onError: (String?) -> Unit,
    onPreview: (FeedTextPreview?) -> Unit,
    buildPreview: suspend (Map<String, Any?>) -> FeedTextPreview?
) {
    forgeRuntime.registerHandler(name) { args ->
        val row = feedRowFromArgs(args.args)
        if (row.isEmpty()) {
            onError(emptySelectionMessage)
            return@registerHandler false
        }

        val preview = try {
            buildPreview(row)
        } catch (err: Throwable) {
            onError(err.message ?: failureMessage)
            throw err
        }

        if (preview == null) {
            onError(emptySelectionMessage)
            return@registerHandler false
        }

        onPreview(preview)
        onError(null)
        true
    }
}

private fun registerToolRefreshHandler(
    forgeRuntime: ForgeRuntime,
    name: String,
    tool: String,
    failureMessage: String,
    client: AgentlyClient,
    onError: (String?) -> Unit,
    onRefreshRequested: () -> Unit
) {
    forgeRuntime.registerHandler(name) {
        try {
            client.executeTool(tool, emptyMap<String, JsonElement>())
            onError(null)
            onRefreshRequested()
        } catch (err: Throwable) {
            onError(err.message ?: failureMessage)
            throw err
        }
        true
    }
}

private suspend fun buildChangedFilePreview(
    client: AgentlyClient,
    currentUri: String,
    previousUri: String?
): FeedTextPreview {
    val current = client.downloadWorkspaceFile(currentUri)
    val previous = previousUri?.takeIf { it.isNotBlank() }?.let { client.downloadWorkspaceFile(it) }.orEmpty()
    return FeedTextPreview(
        title = currentUri.substringAfterLast('/').ifBlank { currentUri },
        subtitle = if (previousUri.isNullOrBlank()) currentUri else "$currentUri vs $previousUri",
        content = buildString {
            append("Current: ")
            append(currentUri)
            append("\n\n")
            append(current)
            if (previous.isNotBlank()) {
                append("\n\n--- Previous: ")
                append(previousUri.orEmpty())
                append(" ---\n\n")
                append(previous)
            }
        }
    )
}

private fun feedRowFromArgs(args: Map<String, Any?>): Map<String, Any?> {
    val direct = args["row"] as? Map<*, *> ?: return emptyMap()
    return direct.entries.associate { it.key.toString() to it.value }
}

private fun rowLocation(row: Map<String, Any?>): String? {
    return rowValue(row, "uri", "URI", "url", "path", "Path")
}

private fun rowValue(row: Map<String, Any?>, vararg keys: String): String? {
    return keys.asSequence()
        .mapNotNull { key -> row[key]?.toString()?.trim() }
        .firstOrNull { it.isNotBlank() }
}
