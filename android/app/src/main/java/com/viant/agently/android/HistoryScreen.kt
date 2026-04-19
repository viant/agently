package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.WorkspaceMetadata
import java.net.URLDecoder
import java.nio.charset.StandardCharsets

@Composable
internal fun TabletConversationSidebar(
    appApiBaseUrl: String,
    loading: Boolean,
    recentConversations: List<Conversation>,
    activeConversationId: String?,
    onNewConversation: () -> Unit,
    onRefresh: () -> Unit,
    onSelectConversation: (String) -> Unit
) {
    var sidebarQuery by remember { mutableStateOf("") }
    val filteredConversations = remember(recentConversations, sidebarQuery) {
        val filterText = sidebarQuery.trim().lowercase()
        if (filterText.isBlank()) {
            recentConversations
        } else {
            recentConversations.filter { conversation ->
                listOf(
                    conversation.title,
                    conversation.summary,
                    conversation.agentId,
                    conversation.id
                ).any { value ->
                    value?.lowercase()?.contains(filterText) == true
                }
            }
        }
    }

    Surface(
        color = Color(0xFFF5F7FB),
        border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
        modifier = Modifier.width(332.dp).fillMaxHeight()
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 12.dp, vertical = 14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.Start,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    "VIANT.",
                    style = MaterialTheme.typography.labelLarge,
                    color = Color(0xFFDB1F2F)
                )
                Spacer(modifier = Modifier.width(8.dp))
                Text(
                    "Agently",
                    style = MaterialTheme.typography.titleMedium,
                    color = Color(0xFF101828)
                )
            }
            Button(
                onClick = onNewConversation,
                enabled = !loading,
                modifier = Modifier.fillMaxWidth()
            ) {
                Text("New Conversation")
            }
            OutlinedTextField(
                value = sidebarQuery,
                onValueChange = { sidebarQuery = it },
                label = { Text("Filter conversations") },
                placeholder = { Text("Search title, agent, or summary") },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true
            )
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    "Conversations",
                    style = MaterialTheme.typography.labelMedium,
                    color = Color(0xFF667085)
                )
                TextButton(onClick = onRefresh, enabled = !loading) {
                    Text("Refresh")
                }
            }
            Column(
                modifier = Modifier
                    .weight(1f)
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                if (filteredConversations.isEmpty()) {
                    Surface(
                        color = Color(0xFFFFFFFF),
                        border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
                        shape = MaterialTheme.shapes.medium,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Column(
                            modifier = Modifier.padding(14.dp),
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Text(
                                "No conversations yet",
                                style = MaterialTheme.typography.titleSmall
                            )
                            Text(
                                if (sidebarQuery.isBlank()) {
                                    "Start a new prompt from the composer and your conversation list will appear here."
                                } else {
                                    "No conversation matches the current filter."
                                },
                                style = MaterialTheme.typography.bodySmall,
                                color = Color(0xFF667085)
                            )
                        }
                    }
                } else {
                    filteredConversations.forEach { conversation ->
                        val isActive = conversation.id == activeConversationId
                        val toneColor = conversationToneColor(conversation.stage)
                        val subtitle = conversation.agentId?.takeIf { it.isNotBlank() } ?: "Conversation"
                        Surface(
                            color = if (isActive) Color(0xFFFCFDFE) else Color(0xAAFFFFFF),
                            border = BorderStroke(
                                1.dp,
                                if (isActive) Color(0xFFCFD7E7) else Color.Transparent
                            ),
                            shape = MaterialTheme.shapes.large,
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable { onSelectConversation(conversation.id) }
                        ) {
                            Row(
                                modifier = Modifier.padding(horizontal = 11.dp, vertical = 8.dp),
                                horizontalArrangement = Arrangement.spacedBy(8.dp),
                                verticalAlignment = Alignment.Top
                            ) {
                                Box(
                                    modifier = Modifier
                                        .padding(top = 5.dp)
                                        .width(9.dp)
                                        .height(9.dp)
                                ) {
                                    Surface(
                                        color = toneColor,
                                        shape = MaterialTheme.shapes.small,
                                        modifier = Modifier.fillMaxSize()
                                    ) {}
                                }
                                Column(
                                    modifier = Modifier.weight(1f),
                                    verticalArrangement = Arrangement.spacedBy(3.dp)
                                ) {
                                    Row(
                                        modifier = Modifier.fillMaxWidth(),
                                        horizontalArrangement = Arrangement.SpaceBetween,
                                        verticalAlignment = Alignment.CenterVertically
                                    ) {
                                        Text(
                                            conversation.title?.takeIf { it.isNotBlank() } ?: conversation.id.take(12),
                                            style = MaterialTheme.typography.titleSmall,
                                            modifier = Modifier.weight(1f),
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis,
                                            color = Color(0xFF101828)
                                        )
                                        Text(
                                            formatConversationRecency(conversation.lastActivity ?: conversation.createdAt) ?: "Recent",
                                            style = MaterialTheme.typography.labelSmall,
                                            color = Color(0xFF667085)
                                        )
                                    }
                                    Text(
                                        subtitle,
                                        style = MaterialTheme.typography.labelMedium,
                                        color = Color(0xFF155EEF),
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    Text(
                                        conversation.summary?.takeIf { it.isNotBlank() } ?: "Conversation ${conversation.id.take(12)}",
                                        style = MaterialTheme.typography.bodySmall,
                                        color = Color(0xFF667085),
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    if (isActive) {
                                        Text(
                                            "Open in workspace",
                                            style = MaterialTheme.typography.labelSmall,
                                            color = Color(0xFF3538CD),
                                            maxLines = 1
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
            }
            Text(
                "Backend $appApiBaseUrl",
                style = MaterialTheme.typography.labelSmall,
                color = Color(0xFF98A2B3),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }
    }
}

@Composable
internal fun ConversationHistoryScreen(
    conversations: List<Conversation>,
    activeConversationId: String?,
    loading: Boolean,
    onBack: () -> Unit,
    onRefresh: () -> Unit,
    onSelectConversation: (String) -> Unit
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        ElevatedCard(modifier = Modifier.fillMaxWidth()) {
            Column(
                modifier = Modifier.padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Text("Conversation History", style = MaterialTheme.typography.headlineSmall)
                Text(
                    "Browse recent threads and jump back into an earlier conversation.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = Color(0xFF667085)
                )
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(onClick = onBack) {
                        Text("Back")
                    }
                    Button(onClick = onRefresh, enabled = !loading) {
                        Text("Refresh")
                    }
                    if (loading) {
                        CircularProgressIndicator(modifier = Modifier.width(24.dp))
                    }
                }
            }
        }
        if (conversations.isEmpty()) {
            Card(modifier = Modifier.fillMaxWidth()) {
                Text(
                    "No conversations yet.",
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.padding(16.dp)
                )
            }
            return
        }
        conversations.forEach { conversation ->
            val isActive = conversation.id == activeConversationId
            Card(modifier = Modifier.fillMaxWidth()) {
                Column(
                    modifier = Modifier.padding(14.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text(
                        conversation.title?.takeIf { it.isNotBlank() } ?: conversation.id.take(12),
                        style = MaterialTheme.typography.titleMedium
                    )
                    Text(
                        conversation.summary?.takeIf { it.isNotBlank() } ?: "Conversation ${conversation.id.take(12)}",
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                    Text(
                        formatTimestampLabel(conversation.lastActivity ?: conversation.createdAt) ?: "Recent conversation",
                        style = MaterialTheme.typography.labelSmall,
                        color = Color(0xFF667085)
                    )
                    OutlinedButton(
                        onClick = { onSelectConversation(conversation.id) },
                        enabled = !isActive,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text(if (isActive) "Active" else "Open conversation")
                    }
                }
            }
        }
        Spacer(modifier = Modifier.height(24.dp))
    }
}

@Composable
internal fun RecentConversationsSection(
    conversations: List<Conversation>,
    activeConversationId: String?,
    onSelectConversation: (String) -> Unit
) {
    if (conversations.isEmpty()) {
        return
    }
    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Text("Recent", style = MaterialTheme.typography.titleMedium, color = Color(0xFF101828))
            Text(
                "Jump back into the threads you touched most recently.",
                style = MaterialTheme.typography.bodySmall,
                color = Color(0xFF667085)
            )
        }
        Column(
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            conversations.forEach { conversation ->
                val isActive = conversation.id == activeConversationId
                val summary = conversation.summary?.takeIf { it.isNotBlank() } ?: "Conversation ${conversation.id.take(12)}"
                Surface(
                    color = if (isActive) Color(0xFFF2F6FF) else Color(0xFFFFFFFF),
                    border = BorderStroke(
                        1.dp,
                        if (isActive) Color(0xFFB2CCFF) else Color(0xFFE4E7EC)
                    ),
                    shape = MaterialTheme.shapes.large,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Row(
                        modifier = Modifier.padding(horizontal = 14.dp, vertical = 12.dp),
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Column(
                            modifier = Modifier.weight(1f),
                            verticalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            Text(
                                conversation.title?.takeIf { it.isNotBlank() }?.let(::decodeConversationLabel) ?: conversation.id.take(12),
                                style = MaterialTheme.typography.titleSmall,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                            Text(
                                decodeConversationLabel(summary),
                                style = MaterialTheme.typography.bodySmall,
                                color = Color(0xFF667085),
                                maxLines = 2,
                                overflow = TextOverflow.Ellipsis
                            )
                            Text(
                                formatConversationRecency(conversation.lastActivity ?: conversation.createdAt) ?: "Recent",
                                style = MaterialTheme.typography.labelSmall,
                                color = Color(0xFF98A2B3)
                            )
                        }
                        OutlinedButton(
                            onClick = { onSelectConversation(conversation.id) },
                            enabled = !isActive
                        ) {
                            Text(if (isActive) "Open" else "View")
                        }
                    }
                }
            }
        }
    }
}

private fun decodeConversationLabel(value: String): String {
    return runCatching {
        URLDecoder.decode(value, StandardCharsets.UTF_8)
    }.getOrDefault(value)
}
