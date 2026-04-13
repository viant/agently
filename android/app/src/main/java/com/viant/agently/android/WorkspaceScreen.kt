package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.FilterChip
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.forgeandroid.runtime.ForgeRuntime
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement

@Composable
internal fun TabletWorkspacePane(
    metadata: WorkspaceMetadata?,
    effectiveAgentId: String?,
    loading: Boolean,
    activeConversation: Conversation?,
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
    onEditChange: (String, String, JsonElement) -> Unit,
    onDecision: (PendingToolApproval, String) -> Unit,
    onOpenFile: (GeneratedFileEntry) -> Unit,
    onClosePreview: () -> Unit,
    query: String,
    onQueryChange: (String) -> Unit,
    composerAttachments: List<ComposerAttachmentDraft>,
    canCapturePhoto: Boolean,
    canUseVoiceInput: Boolean,
    onAddPhoto: () -> Unit,
    onTakePhoto: () -> Unit,
    onVoiceInput: () -> Unit,
    onRemoveAttachment: (String) -> Unit,
    onRunQuery: () -> Unit
) {
    val hasMainContent = transcript.isNotEmpty() || pendingApprovals.isNotEmpty() || generatedFiles.isNotEmpty() || !activeConversationId.isNullOrBlank()

    Surface(
        color = Color(0xFFFBFCFE),
        modifier = Modifier.fillMaxSize()
    ) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 14.dp, vertical = 12.dp)
        ) {
            Column(
                modifier = Modifier
                    .align(Alignment.TopCenter)
                    .fillMaxWidth()
                    .widthIn(max = 1120.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Surface(
                    color = Color(0xFFF8FAFD),
                    border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
                    shape = MaterialTheme.shapes.large,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 18.dp, vertical = 12.dp),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Text("Chat Workspace", style = MaterialTheme.typography.titleLarge, color = Color(0xFF101828))
                            Text(
                                activeConversation?.title?.takeIf { it.isNotBlank() }
                                    ?: if (!activeConversationId.isNullOrBlank()) "Focused on the active conversation." else "Ready for a new conversation.",
                                style = MaterialTheme.typography.bodySmall,
                                color = Color(0xFF667085),
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                        metadata?.let { meta ->
                            Row(
                                modifier = Modifier.horizontalScroll(rememberScrollState()),
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                FilterChip(
                                    selected = true,
                                    onClick = {},
                                    label = { Text("Agent ${effectiveAgentId ?: meta.defaultAgent ?: meta.defaults?.agent ?: "n/a"}") }
                                )
                                FilterChip(
                                    selected = true,
                                    onClick = {},
                                    label = { Text("Model ${meta.defaultModel ?: meta.defaults?.model ?: "n/a"}") }
                                )
                            }
                        }
                    }
                }
                Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
                    Column(
                        modifier = Modifier
                            .fillMaxSize()
                            .verticalScroll(rememberScrollState())
                            .padding(bottom = 188.dp),
                        verticalArrangement = Arrangement.spacedBy(14.dp)
                    ) {
                        error?.let {
                            Card(modifier = Modifier.fillMaxWidth()) {
                                Text(
                                    "Error: $it",
                                    color = Color(0xFFB42318),
                                    style = MaterialTheme.typography.bodySmall,
                                    modifier = Modifier.padding(14.dp)
                                )
                            }
                        }
                        if (!activeConversationId.isNullOrBlank()) {
                            Text(
                                "Conversation: $activeConversationId",
                                style = MaterialTheme.typography.bodySmall,
                                color = Color(0xFF667085)
                            )
                        }
                        streamSnapshot?.activeTurnId?.let { turnId ->
                            Text(
                                "Active turn: $turnId",
                                style = MaterialTheme.typography.bodySmall,
                                color = Color(0xFF667085)
                            )
                        }
                        if (loading || streamSnapshot?.activeTurnId != null) {
                            LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
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
                        if (!hasMainContent) {
                            Surface(
                                color = Color(0xFFF8FAFD),
                                border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
                                shape = MaterialTheme.shapes.large,
                                modifier = Modifier.fillMaxWidth()
                            ) {
                                Column(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .padding(horizontal = 28.dp, vertical = 36.dp),
                                    verticalArrangement = Arrangement.spacedBy(10.dp)
                                ) {
                                    Text("Start a conversation", style = MaterialTheme.typography.headlineSmall)
                                    Text(
                                        "This tablet layout mirrors the web app: choose a conversation from the left rail or start a new task from the composer below.",
                                        style = MaterialTheme.typography.bodyMedium,
                                        color = Color(0xFF667085)
                                    )
                                }
                            }
                        } else {
                            PendingApprovalsSection(
                                approvals = pendingApprovals,
                                forgeRuntime = forgeRuntime,
                                approvalJson = approvalJson,
                                approvalEdits = approvalEdits,
                                onEditChange = onEditChange,
                                onDecision = onDecision
                            )
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
                            Spacer(modifier = Modifier.padding(bottom = 24.dp))
                        }
                    }

                    Surface(
                        modifier = Modifier
                            .align(Alignment.BottomCenter)
                            .fillMaxWidth(0.74f)
                            .widthIn(max = 900.dp)
                            .navigationBarsPadding(),
                        color = Color(0xFFFDFDFE),
                        border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
                        shape = MaterialTheme.shapes.large
                    ) {
                        Column(
                            modifier = Modifier.padding(horizontal = 18.dp, vertical = 14.dp),
                            verticalArrangement = Arrangement.spacedBy(12.dp)
                        ) {
                            ComposerHeader(
                                title = "Composer",
                                attachments = composerAttachments,
                                canCapturePhoto = canCapturePhoto,
                                canUseVoiceInput = canUseVoiceInput,
                                agentLabel = effectiveAgentId?.takeIf { it.isNotBlank() },
                                subtitle = if (!activeConversationId.isNullOrBlank()) {
                                    "Continuing current conversation"
                                } else {
                                    "A new conversation will be created"
                                },
                                onAddPhoto = onAddPhoto,
                                onTakePhoto = onTakePhoto,
                                onVoiceInput = onVoiceInput,
                                onRemoveAttachment = onRemoveAttachment
                            )
                            OutlinedTextField(
                                value = query,
                                onValueChange = onQueryChange,
                                label = { Text("Message") },
                                placeholder = { Text("Ask a follow-up or start a new task") },
                                modifier = Modifier.fillMaxWidth(),
                                minLines = 2,
                                maxLines = 5
                            )
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.End,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Button(
                                    onClick = onRunQuery,
                                    enabled = !loading && (query.isNotBlank() || composerAttachments.isNotEmpty())
                                ) {
                                    Text("Send")
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
