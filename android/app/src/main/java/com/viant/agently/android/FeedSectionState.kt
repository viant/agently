package com.viant.agently.android

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.FeedDataResponse
import com.viant.agentlysdk.stream.ActiveFeed
import com.viant.forgeandroid.runtime.ForgeRuntime

internal data class FeedSectionUiState(
    val visibleFeeds: List<ActiveFeed>,
    val selectedFeedId: String,
    val payload: FeedDataResponse?,
    val loading: Boolean,
    val error: String?,
    val preview: FeedTextPreview?,
    val onSelectFeed: (String) -> Unit,
    val onClosePreview: () -> Unit
)

internal data class FeedPayloadLoadState(
    val payload: FeedDataResponse?,
    val error: String?
)

@Composable
internal fun rememberFeedSectionUiState(
    feeds: List<ActiveFeed>,
    conversationId: String,
    client: AgentlyClient,
    forgeRuntime: ForgeRuntime
): FeedSectionUiState {
    val visibleFeeds = remember(feeds) { visibleFeeds(feeds) }
    var selectedFeedId by remember(conversationId, visibleFeeds.map { it.feedId }) {
        mutableStateOf(visibleFeeds.first().feedId)
    }
    var payload by remember(conversationId, selectedFeedId) { mutableStateOf<FeedDataResponse?>(null) }
    var loading by remember(conversationId, selectedFeedId) { mutableStateOf(false) }
    var error by remember(conversationId, selectedFeedId) { mutableStateOf<String?>(null) }
    var preview by remember(conversationId) { mutableStateOf<FeedTextPreview?>(null) }
    var refreshTick by remember(conversationId, selectedFeedId) { mutableIntStateOf(0) }

    LaunchedEffect(forgeRuntime, client, conversationId) {
        registerFeedInteractionHandlers(
            forgeRuntime = forgeRuntime,
            client = client,
            onError = { error = it },
            onPreview = { preview = it },
            onRefreshRequested = { refreshTick += 1 }
        )
    }

    LaunchedEffect(visibleFeeds) {
        if (visibleFeeds.none { it.feedId == selectedFeedId }) {
            selectedFeedId = visibleFeeds.first().feedId
        }
    }

    LaunchedEffect(conversationId, selectedFeedId, refreshTick, visibleFeeds.firstOrNull { it.feedId == selectedFeedId }?.updatedAt) {
        loading = true
        error = null
        val nextState = loadFeedPayloadState(client, selectedFeedId, conversationId)
        payload = nextState.payload
        error = nextState.error
        loading = false
    }

    return FeedSectionUiState(
        visibleFeeds = visibleFeeds,
        selectedFeedId = selectedFeedId,
        payload = payload,
        loading = loading,
        error = error,
        preview = preview,
        onSelectFeed = { selectedFeedId = it },
        onClosePreview = { preview = null }
    )
}

internal suspend fun loadFeedPayloadState(
    client: AgentlyClient,
    feedId: String,
    conversationId: String
): FeedPayloadLoadState {
    return try {
        FeedPayloadLoadState(
            payload = loadFeedPayload(client, feedId, conversationId),
            error = null
        )
    } catch (error: Throwable) {
        FeedPayloadLoadState(
            payload = null,
            error = error.message ?: "Unable to load feed."
        )
    }
}
