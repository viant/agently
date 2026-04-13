package com.viant.agently.android

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.FeedDataResponse
import com.viant.agentlysdk.stream.ActiveFeed
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.ui.ContainerRenderer

@Composable
internal fun ActiveFeedsSection(
    feeds: List<ActiveFeed>,
    conversationId: String?,
    client: AgentlyClient,
    forgeRuntime: ForgeRuntime
) {
    val scopedConversationId = conversationId?.takeIf { it.isNotBlank() }
        ?: feeds.firstOrNull()?.conversationId?.takeIf { it.isNotBlank() }
        ?: return
    if (feeds.isEmpty()) {
        return
    }
    val state = rememberFeedSectionUiState(
        feeds = feeds,
        conversationId = scopedConversationId,
        client = client,
        forgeRuntime = forgeRuntime
    )
    if (state.visibleFeeds.isEmpty()) return

    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Text("Tool feeds", style = MaterialTheme.typography.titleMedium)
            Row(
                modifier = Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                state.visibleFeeds.forEach { feed ->
                    FilterChip(
                        selected = feed.feedId == state.selectedFeedId,
                        onClick = { state.onSelectFeed(feed.feedId) },
                        label = {
                            Text(
                                buildString {
                                    append(feed.title.ifBlank { feed.feedId })
                                    if (feed.itemCount > 0) {
                                        append(" · ")
                                        append(feed.itemCount)
                                    }
                                }
                            )
                        }
                    )
                }
            }
            when {
                state.loading -> CircularProgressIndicator()
                state.error != null -> Text(
                    state.error,
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFFB42318)
                )
                state.payload != null -> FeedPanel(
                    payload = state.payload,
                    conversationId = scopedConversationId,
                    forgeRuntime = forgeRuntime
                )
            }
            state.preview?.let { activePreview ->
                FeedTextPreviewSection(
                    preview = activePreview,
                    onClose = state.onClosePreview
                )
            }
        }
    }
}

@Composable
private fun FeedPanel(
    payload: FeedDataResponse,
    conversationId: String,
    forgeRuntime: ForgeRuntime
) {
    val windowState = rememberFeedWindowUiState(
        payload = payload,
        conversationId = conversationId,
        forgeRuntime = forgeRuntime
    )
    if (windowState.metadata == null || windowState.windowContext == null) {
        Text(
            text = windowState.error ?: "Unable to render feed.",
            style = MaterialTheme.typography.bodyMedium
        )
        return
    }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .heightIn(max = 340.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        windowState.metadata.view?.content?.containers?.forEach { container ->
            ContainerRenderer(forgeRuntime, windowState.windowContext, container)
        }
    }
}

@Composable
private fun FeedTextPreviewSection(
    preview: FeedTextPreview,
    onClose: () -> Unit
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(preview.title, style = MaterialTheme.typography.titleSmall)
                    Text(
                        preview.subtitle,
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                }
                TextButton(onClick = onClose) {
                    Text("Close")
                }
            }
            Surface(
                color = MaterialTheme.colorScheme.surfaceVariant,
                shape = MaterialTheme.shapes.medium,
                modifier = Modifier.fillMaxWidth()
            ) {
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(max = 260.dp)
                        .horizontalScroll(rememberScrollState())
                        .padding(12.dp)
                ) {
                    Text(
                        text = preview.content,
                        style = MaterialTheme.typography.bodySmall
                    )
                }
            }
        }
    }
}
