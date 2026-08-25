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
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Assessment
import androidx.compose.material.icons.outlined.Build
import androidx.compose.material.icons.outlined.Checklist
import androidx.compose.material.icons.outlined.Description
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.Terminal
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.FeedDataResponse
import com.viant.agentlysdk.FeedPresentation
import com.viant.agentlysdk.stream.ActiveFeed
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.ui.ContainerRenderer

internal fun toolFeedIcon(presentation: FeedPresentation?): ImageVector = when (presentation?.icon?.trim()?.lowercase()) {
    "list", "checklist" -> Icons.Outlined.Checklist
    "terminal", "console" -> Icons.Outlined.Terminal
    "changes", "refresh" -> Icons.Outlined.Refresh
    "chart", "report" -> Icons.Outlined.Assessment
    "database", "data" -> Icons.Outlined.Dns
    "document", "file" -> Icons.Outlined.Description
    "folder", "explorer" -> Icons.Outlined.Folder
    else -> Icons.Outlined.Build
}

internal fun toolFeedAccent(presentation: FeedPresentation?): Color {
    val raw = presentation?.accent?.trim().orEmpty()
    val named = mapOf(
        "blue" to Color(0xFF1A73F0), "orange" to Color(0xFFE08A1E),
        "purple" to Color(0xFF7D52D9), "teal" to Color(0xFF0A9B98), "pink" to Color(0xFFDF5B78)
    )
    named[raw.lowercase()]?.let { return it }
    return runCatching { Color(android.graphics.Color.parseColor(raw)) }.getOrDefault(Color(0xFF5965D8))
}

@Composable
@OptIn(ExperimentalMaterial3Api::class)
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
    var sheetOpen by remember(scopedConversationId) { mutableStateOf(false) }
    val state = rememberFeedSectionUiState(
        feeds = feeds,
        conversationId = scopedConversationId,
        client = client,
        forgeRuntime = forgeRuntime,
        loadEnabled = sheetOpen
    )
    if (state.visibleFeeds.isEmpty()) return

    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                Text("Tool feeds", style = MaterialTheme.typography.titleMedium)
                TextButton(onClick = { sheetOpen = true }) { Text("Open") }
            }
            Row(
                modifier = Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                state.visibleFeeds.forEach { feed ->
                    val accent = toolFeedAccent(feed.presentation)
                    FilterChip(
                        selected = feed.feedId == state.selectedFeedId,
                        onClick = {
                            state.onSelectFeed(feed.feedId)
                            sheetOpen = true
                        },
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
                        },
                        leadingIcon = { Icon(toolFeedIcon(feed.presentation), contentDescription = null) },
                        colors = FilterChipDefaults.filterChipColors(
                            selectedContainerColor = accent.copy(alpha = 0.12f),
                            selectedLabelColor = accent,
                            selectedLeadingIconColor = accent
                        )
                    )
                }
            }
        }
    }
    if (sheetOpen) {
        ModalBottomSheet(onDismissRequest = { sheetOpen = false }) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 18.dp, vertical = 10.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Text("Tool feeds", style = MaterialTheme.typography.titleLarge)
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    state.visibleFeeds.forEach { feed ->
                        val accent = toolFeedAccent(feed.presentation)
                        FilterChip(
                            selected = feed.feedId == state.selectedFeedId,
                            onClick = { state.onSelectFeed(feed.feedId) },
                            label = { Text(feed.title.ifBlank { feed.feedId }) },
                            leadingIcon = { Icon(toolFeedIcon(feed.presentation), contentDescription = null) },
                            colors = FilterChipDefaults.filterChipColors(
                                selectedContainerColor = accent.copy(alpha = 0.12f),
                                selectedLabelColor = accent,
                                selectedLeadingIconColor = accent
                            )
                        )
                    }
                }
                when {
                    state.loading -> CircularProgressIndicator()
                    state.error != null -> Text(state.error, style = MaterialTheme.typography.bodySmall, color = Color(0xFFB42318))
                    state.payload != null -> FeedPanel(
                        payload = state.payload,
                        conversationId = scopedConversationId,
                        forgeRuntime = forgeRuntime
                    )
                }
                state.preview?.let { activePreview ->
                    FeedTextPreviewSection(preview = activePreview, onClose = state.onClosePreview)
                }
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
