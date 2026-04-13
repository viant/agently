package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.forgeandroid.runtime.ForgeRuntime
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement

@Composable
internal fun PhoneWorkspacePane(
    appApiBaseUrl: String,
    metadata: com.viant.agentlysdk.WorkspaceMetadata?,
    effectiveAgentId: String?,
    loading: Boolean,
    recentConversations: List<Conversation>,
    activeConversationId: String?,
    error: String?,
    streamSnapshot: ConversationStreamSnapshot?,
    transcript: List<ChatEntry>,
    pendingApprovals: List<PendingToolApproval>,
    generatedFiles: List<GeneratedFileEntry>,
    artifactPreview: ArtifactPreview?,
    client: AgentlyClient,
    forgeRuntime: ForgeRuntime,
    approvalJson: Json,
    approvalEdits: Map<String, Map<String, JsonElement>>,
    onRefresh: () -> Unit,
    onNewConversation: () -> Unit,
    onOpenHistory: () -> Unit,
    onOpenSettings: () -> Unit,
    onSelectConversation: (String) -> Unit,
    onEditChange: (String, String, JsonElement) -> Unit,
    onDecision: (PendingToolApproval, String) -> Unit,
    onOpenFile: (GeneratedFileEntry) -> Unit,
    onClosePreview: () -> Unit
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        Column(
            modifier = Modifier.fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Column(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(3.dp)
                ) {
                    Text(
                        metadata?.workspaceRoot?.takeIf { it.isNotBlank() }?.let(::compactWorkspaceLabel)
                            ?: "Workspace loading",
                        style = MaterialTheme.typography.titleSmall,
                        color = Color(0xFF182230),
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis
                    )
                    Text(
                        if (!activeConversationId.isNullOrBlank()) "Continuing your latest chat"
                        else "Ready for a new conversation",
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                }
                TextButton(onClick = onOpenSettings) {
                    Text("Settings")
                }
            }
            Row(
                modifier = Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                TextButton(onClick = onNewConversation, enabled = !loading) {
                    Text("New chat")
                }
                TextButton(
                    onClick = onOpenHistory,
                    enabled = recentConversations.isNotEmpty()
                ) {
                    Text("History")
                }
                TextButton(onClick = onRefresh, enabled = !loading) {
                    Text("Refresh")
                }
                if (loading) {
                    CircularProgressIndicator(modifier = Modifier.height(24.dp))
                }
            }
            val statusLine = buildString {
                effectiveAgentId?.takeIf { it.isNotBlank() }?.let {
                    append("Agent ")
                    append(it.replaceFirstChar { ch -> ch.titlecase() })
                }
                (metadata?.defaultModel ?: metadata?.defaults?.model)?.takeIf { it.isNotBlank() }?.let { model ->
                    if (isNotEmpty()) append(" · ")
                    append(model)
                }
                if (streamSnapshot?.activeTurnId != null) {
                    if (isNotEmpty()) append(" · ")
                    append("Live turn")
                } else {
                    if (isNotEmpty()) append(" · ")
                    append(compactBackendLabel(appApiBaseUrl))
                }
            }
            Text(
                statusLine,
                style = MaterialTheme.typography.labelSmall,
                color = Color(0xFF98A2B3),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            if (loading || streamSnapshot?.activeTurnId != null) {
                LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
            }
        }
        if (pendingApprovals.isNotEmpty()) {
            PendingApprovalsSection(
                approvals = pendingApprovals,
                forgeRuntime = forgeRuntime,
                approvalJson = approvalJson,
                approvalEdits = approvalEdits,
                onEditChange = onEditChange,
                onDecision = onDecision
            )
        }
        RecentConversationsSection(
            conversations = recentConversations,
            activeConversationId = activeConversationId,
            onSelectConversation = onSelectConversation
        )
        error?.let {
            Surface(
                color = Color(0xFFFFF1F0),
                border = BorderStroke(1.dp, Color(0xFFF4C7C3)),
                shape = MaterialTheme.shapes.large,
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(
                    modifier = Modifier.padding(14.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    Text("Something needs attention", style = MaterialTheme.typography.titleSmall, color = Color(0xFF912018))
                    Text(
                        it,
                        color = Color(0xFFB42318),
                        style = MaterialTheme.typography.bodySmall
                    )
                }
            }
        }
        if (!activeConversationId.isNullOrBlank() || streamSnapshot?.activeTurnId != null) {
            Surface(
                color = Color(0xFFFFFFFF),
                border = BorderStroke(1.dp, Color(0xFFE2E8F3)),
                shape = MaterialTheme.shapes.large,
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(
                    modifier = Modifier.padding(14.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    activeConversationId?.let {
                        Text(
                            "Conversation ${it.take(12)}",
                            style = MaterialTheme.typography.labelLarge,
                            color = Color(0xFF344054)
                        )
                    }
                    streamSnapshot?.activeTurnId?.let { turnId ->
                        Text(
                            "Streaming turn ${turnId.take(12)}",
                            style = MaterialTheme.typography.bodySmall,
                            color = Color(0xFF667085)
                        )
                    }
                }
            }
        }
        streamSnapshot?.pendingElicitation?.let { elicitation ->
            elicitation.conversationId.takeIf { it.isNotBlank() }?.let { conversationId ->
                ElicitationOverlay(
                    elicitation = elicitation,
                    conversationId = conversationId,
                    onResolved = {},
                    client = client,
                    forgeRuntime = forgeRuntime
                )
            }
        }
        if (streamSnapshot?.feeds?.isNotEmpty() == true) {
            ActiveFeedsSection(
                feeds = streamSnapshot.feeds,
                conversationId = activeConversationId,
                client = client,
                forgeRuntime = forgeRuntime
            )
        }
        ConversationArtifactsSection(
            files = generatedFiles,
            onOpenFile = onOpenFile
        )
        artifactPreview?.let { preview ->
            if (generatedFiles.none { it.id == preview.artifactId }) {
                ArtifactPreviewSection(
                    preview = preview,
                    onClose = onClosePreview
                )
            }
        }
        RenderTranscript(
            items = transcript,
            pendingApprovals = pendingApprovals,
            generatedFiles = generatedFiles,
            forgeRuntime = forgeRuntime,
            approvalJson = approvalJson,
            approvalEdits = approvalEdits,
            onEditChange = onEditChange,
            onDecision = onDecision,
            artifactPreview = artifactPreview,
            onClosePreview = onClosePreview,
            onOpenFile = onOpenFile
        )
        Spacer(modifier = Modifier.height(320.dp))
    }
}

private fun compactWorkspaceLabel(workspaceRoot: String): String {
    val parts = workspaceRoot.split('/').filter { it.isNotBlank() }
    if (parts.size <= 4) {
        return workspaceRoot
    }
    return ".../${parts.takeLast(4).joinToString("/")}"
}

private fun compactBackendLabel(baseUrl: String): String = baseUrl.removePrefix("http://").removePrefix("https://")
