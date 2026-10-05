package com.viant.agently.android

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.viant.agentlysdk.AgentlyClient
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import com.viant.agentlysdk.FeedDataResponse
import com.viant.agentlysdk.stream.ActiveFeed
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
    forgeRuntime: ForgeRuntime,
    activeFeed: ActiveFeed? = null,
    client: AgentlyClient? = null
): FeedWindowUiState {
    val requiredDialogs = remember(payload.ui) { referencedFeedLookupDialogs(payload.ui) }
    var sharedMetadata by remember(conversationId, client, forgeRuntime) { mutableStateOf<WindowMetadata?>(null) }
    LaunchedEffect(conversationId, client, forgeRuntime, requiredDialogs) {
        if (client != null && requiredDialogs.isNotEmpty()) {
            try {
                val metadata = makeForgeAgentlyWindowMetadataLoader(client, forgeRuntime.targetContext)(
                    ForgeRuntime.WindowMetadataRequest("feed-shared-$conversationId", "chat/new", conversationId = conversationId)
                )
                currentCoroutineContext().ensureActive()
                android.util.Log.d("FeedLookupMetadata", "Required=$requiredDialogs loaded=${metadata?.dialogs?.map { it.id }}")
                sharedMetadata = metadata
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (error: Exception) { android.util.Log.w("FeedLookupMetadata", "Unable to hydrate referenced lookup dialogs", error); sharedMetadata = null }
        }
    }
    val metadataResult = remember(payload, sharedMetadata) { kotlin.runCatching { buildFeedWindowMetadata(payload, sharedMetadata) } }
    val inlineMetadata = metadataResult.getOrNull()
    if (inlineMetadata == null) {
        return FeedWindowUiState(
            metadata = null,
            windowContext = null,
            error = metadataResult.exceptionOrNull()?.message ?: "Unable to decode feed window metadata."
        )
    }

    var windowId by remember(payload.feedId, conversationId) { mutableStateOf<String?>(null) }
    var windowRevision by remember(payload.feedId, conversationId) { mutableStateOf(0) }

    LaunchedEffect(payload, conversationId, inlineMetadata, activeFeed?.turnId) {
        val state = forgeRuntime.openWindowInline(
            windowKey = "feed-${payload.feedId ?: "unknown"}-$conversationId",
            title = payload.title ?: payload.feedId ?: "Feed",
            metadata = inlineMetadata,
            conversationId = conversationId,
            presentation = payload.presentation?.target ?: "auto"
        )
        windowId = state.windowId
        wireFeedWindow(forgeRuntime, state.windowId, payload, activeFeed?.turnId)
        // Metadata replacement resets Forge signals even when the window ID stays the same.
        windowRevision++
    }

    val activeWindowId = windowId
    val metadataSignal = remember(activeWindowId, windowRevision) {
        activeWindowId?.let { forgeRuntime.metadataSignal(it) }
    }
    val resolvedMetadata by if (metadataSignal != null) {
        metadataSignal.flow.collectAsState(initial = metadataSignal.peek())
    } else {
        remember { mutableStateOf<WindowMetadata?>(null) }
    }
    val windowContext = remember(activeWindowId, windowRevision) {
        activeWindowId?.let { forgeRuntime.windowContext(it) }
    }

    return FeedWindowUiState(
        metadata = resolvedMetadata,
        windowContext = windowContext
    )
}
