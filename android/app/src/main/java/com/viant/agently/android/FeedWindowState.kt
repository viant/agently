package com.viant.agently.android

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.viant.agentlysdk.FeedDataResponse
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.WindowContext
import com.viant.forgeandroid.runtime.WindowMetadata

internal data class FeedWindowUiState(
    val metadata: WindowMetadata?,
    val windowContext: WindowContext?,
    val error: String? = null
)

@Composable
internal fun rememberFeedWindowUiState(
    payload: FeedDataResponse,
    conversationId: String,
    forgeRuntime: ForgeRuntime
): FeedWindowUiState {
    val metadataResult = remember(payload) { kotlin.runCatching { buildFeedWindowMetadata(payload) } }
    val inlineMetadata = metadataResult.getOrNull()
    if (inlineMetadata == null) {
        return FeedWindowUiState(
            metadata = null,
            windowContext = null,
            error = metadataResult.exceptionOrNull()?.message ?: "Unable to decode feed window metadata."
        )
    }

    var windowId by remember(payload.feedId, conversationId) { mutableStateOf<String?>(null) }

    LaunchedEffect(payload, conversationId, inlineMetadata) {
        val state = forgeRuntime.openWindowInline(
            windowKey = "feed-${payload.feedId ?: "unknown"}-$conversationId",
            title = payload.title ?: payload.feedId ?: "Feed",
            metadata = inlineMetadata
        )
        windowId = state.windowId
        wireFeedWindow(forgeRuntime, state.windowId, payload)
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

    return FeedWindowUiState(
        metadata = resolvedMetadata,
        windowContext = windowContext
    )
}
