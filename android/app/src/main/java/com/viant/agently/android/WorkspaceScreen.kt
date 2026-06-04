package com.viant.agently.android

import android.content.Context
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.gestures.detectVerticalDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.platform.LocalContext
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.forgeandroid.runtime.ForgeRuntime
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement

private enum class WorkspacePanelMode {
    Split,
    Expanded,
    Hidden
}

@Composable
internal fun TabletWorkspacePane(
    loading: Boolean,
    activeConversationId: String?,
    metadata: WorkspaceMetadata?,
    preferredAgentId: String,
    conversationState: ConversationStateResponse?,
    error: String?,
    streamSnapshot: ConversationStreamSnapshot?,
    transcript: List<ChatEntry>,
    pendingApprovals: List<PendingToolApproval>,
    generatedFiles: List<GeneratedFileEntry>,
    payloadPreviews: Map<String, ArtifactPreview>,
    artifactPreview: ArtifactPreview?,
    client: AgentlyClient,
    forgeRuntime: ForgeRuntime,
    approvalJson: Json,
    approvalEdits: Map<String, Map<String, JsonElement>>,
    onEditChange: (String, String, JsonElement) -> Unit,
    onDecision: (PendingToolApproval, String) -> Unit,
    onOpenFile: (GeneratedFileEntry) -> Unit,
    onClosePreview: () -> Unit,
    onSelectAgent: (String?) -> Unit,
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
    val context = LocalContext.current
    val hostedWorkspaceState = conversationState?.let(::deriveHostedWorkspaceRestoreState)
    val hasMainContent = transcript.isNotEmpty() || pendingApprovals.isNotEmpty() || generatedFiles.isNotEmpty() || !activeConversationId.isNullOrBlank()
    val hasHostedWorkspace = hostedWorkspaceState != null
    val hostedWorkspaceMinHeight = remember(hostedWorkspaceState) {
        hostedWorkspaceState?.windows
            ?.firstOrNull { it.windowId == hostedWorkspaceState.selectedWindowId }
            ?.workspaceMinHeight
            ?: hostedWorkspaceState?.windows?.lastOrNull()?.workspaceMinHeight
            ?: 420
    }.coerceIn(320, 1200)
    val prefs = remember(context) {
        context.applicationContext.getSharedPreferences("agently.workspace.pane", Context.MODE_PRIVATE)
    }
    var workspaceBodyHeight by remember {
        mutableStateOf(
            prefs.getFloat("hosted_workspace_height_dp", 420f)
                .coerceAtLeast(hostedWorkspaceMinHeight.toFloat())
                .coerceIn(hostedWorkspaceMinHeight.toFloat(), 1200f)
        )
    }
    var workspacePanelMode by remember(activeConversationId, hostedWorkspaceState?.selectedWindowId, hasHostedWorkspace) {
        mutableStateOf(
            if (hasHostedWorkspace) WorkspacePanelMode.Expanded
            else WorkspacePanelMode.Hidden
        )
    }
    var showExecutionDetails by remember(activeConversationId) { mutableStateOf(false) }
    val contentScrollState = rememberScrollState()
    val hasExecutionDetails = conversationState?.conversation?.turns
        ?.lastOrNull { it.execution?.pages?.isNotEmpty() == true } != null

    LaunchedEffect(activeConversationId) {
        contentScrollState.scrollTo(0)
    }
    LaunchedEffect(hostedWorkspaceMinHeight) {
        if (workspaceBodyHeight < hostedWorkspaceMinHeight.toFloat()) {
            workspaceBodyHeight = hostedWorkspaceMinHeight.toFloat()
        }
    }

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
                    .widthIn(max = if (workspacePanelMode == WorkspacePanelMode.Expanded) 1600.dp else 1120.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Column(
                    modifier = Modifier
                        .weight(1f)
                        .fillMaxWidth(),
                    verticalArrangement = Arrangement.spacedBy(12.dp)
                ) {
                    Column(
                        modifier = Modifier
                            .weight(1f)
                            .fillMaxWidth()
                            .verticalScroll(contentScrollState),
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
                            WorkspaceTaskStartSection(
                                metadata = metadata,
                                preferredAgentId = preferredAgentId,
                                onSelectAgent = onSelectAgent,
                                onSelectStarterTask = onQueryChange
                            )
                        } else {
                            if (hasHostedWorkspace && workspacePanelMode != WorkspacePanelMode.Hidden) {
                                HostedWorkspaceSection(
                                    restoreState = hostedWorkspaceState,
                                    forgeRuntime = forgeRuntime,
                                    maxBodyHeight = if (workspacePanelMode == WorkspacePanelMode.Expanded) 1100.dp else workspaceBodyHeight.dp,
                                    showTitle = false,
                                    headerActions = {
                                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                            WorkspaceHeaderDot(
                                                color = Color(0xFFFF5F57),
                                                description = "Close workspace",
                                                selected = workspacePanelMode == WorkspacePanelMode.Hidden,
                                                onClick = { workspacePanelMode = WorkspacePanelMode.Hidden }
                                            )
                                            WorkspaceHeaderDot(
                                                color = Color(0xFFFEBB2E),
                                                description = "Minimize workspace",
                                                selected = workspacePanelMode == WorkspacePanelMode.Split,
                                                onClick = { workspacePanelMode = WorkspacePanelMode.Split }
                                            )
                                            WorkspaceHeaderDot(
                                                color = Color(0xFF28C840),
                                                description = "Expand workspace",
                                                selected = workspacePanelMode == WorkspacePanelMode.Expanded,
                                                onClick = { workspacePanelMode = WorkspacePanelMode.Expanded }
                                            )
                                        }
                                    }
                                )
                                if (workspacePanelMode != WorkspacePanelMode.Expanded) {
                                    Box(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .padding(top = 4.dp, bottom = 10.dp),
                                        contentAlignment = Alignment.Center
                                    ) {
                                        Box(
                                            modifier = Modifier
                                                .widthIn(min = 96.dp)
                                                .padding(horizontal = 12.dp)
                                                .pointerInput(Unit) {
                                                    detectVerticalDragGestures { _, dragAmount ->
                                                        workspaceBodyHeight = (workspaceBodyHeight + dragAmount)
                                                            .coerceIn(hostedWorkspaceMinHeight.toFloat(), 1200f)
                                                        prefs.edit()
                                                            .putFloat("hosted_workspace_height_dp", workspaceBodyHeight)
                                                            .apply()
                                                    }
                                                }
                                        ) {
                                            Surface(
                                                color = Color(0xFFD0D5DD),
                                                shape = MaterialTheme.shapes.large,
                                                modifier = Modifier
                                                    .fillMaxWidth()
                                                    .padding(horizontal = 16.dp)
                                            ) {
                                                Spacer(modifier = Modifier.padding(vertical = 3.dp))
                                            }
                                        }
                                    }
                                }
                            } else if (workspacePanelMode == WorkspacePanelMode.Hidden && hasHostedWorkspace) {
                                Surface(
                                    color = Color(0xFFF8FAFD),
                                    border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
                                    shape = MaterialTheme.shapes.large,
                                    modifier = Modifier.fillMaxWidth()
                                ) {
                                    Row(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .padding(horizontal = 18.dp, vertical = 14.dp),
                                        horizontalArrangement = Arrangement.SpaceBetween,
                                        verticalAlignment = Alignment.CenterVertically
                                    ) {
                                        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                                            Text("Workspace closed", style = MaterialTheme.typography.titleSmall)
                                            Text(
                                                "Reopen the hosted workspace when you want the full data-driven view back.",
                                                style = MaterialTheme.typography.bodySmall,
                                                color = Color(0xFF667085)
                                            )
                                        }
                                        Button(onClick = { workspacePanelMode = WorkspacePanelMode.Split }) {
                                            Text("Reopen")
                                        }
                                    }
                                }
                            }
                            if (workspacePanelMode != WorkspacePanelMode.Expanded) {
                                if (hasExecutionDetails) {
                                    Row(
                                        modifier = Modifier.fillMaxWidth(),
                                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                                    ) {
                                        androidx.compose.material3.FilterChip(
                                            selected = !showExecutionDetails,
                                            onClick = { showExecutionDetails = false },
                                            label = { Text("Transcript") }
                                        )
                                        androidx.compose.material3.FilterChip(
                                            selected = showExecutionDetails,
                                            onClick = { showExecutionDetails = true },
                                            label = { Text("Execution details") }
                                        )
                                    }
                                }
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
                                if (showExecutionDetails) {
                                    ExecutionInspectorSection(
                                        state = conversationState,
                                        payloadPreviews = payloadPreviews
                                    )
                                }
                                Spacer(modifier = Modifier.padding(bottom = 24.dp))
                            }
                        }
                    }

                    if (workspacePanelMode != WorkspacePanelMode.Expanded) {
                        Surface(
                            modifier = Modifier
                                .fillMaxWidth(0.74f)
                                .widthIn(max = 900.dp)
                                .align(Alignment.CenterHorizontally)
                                .navigationBarsPadding(),
                            color = Color(0xFFFDFDFE),
                            border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
                            shape = MaterialTheme.shapes.large
                        ) {
                            val compactComposer = !activeConversationId.isNullOrBlank()
                            Column(
                                modifier = Modifier.padding(
                                    horizontal = if (compactComposer) 16.dp else 18.dp,
                                    vertical = if (compactComposer) 10.dp else 14.dp
                                ),
                                verticalArrangement = Arrangement.spacedBy(if (compactComposer) 8.dp else 12.dp)
                            ) {
                                if (compactComposer) {
                                    Row(
                                        modifier = Modifier.fillMaxWidth(),
                                        horizontalArrangement = Arrangement.spacedBy(10.dp),
                                        verticalAlignment = Alignment.CenterVertically
                                    ) {
                                        OutlinedTextField(
                                            value = query,
                                            onValueChange = onQueryChange,
                                            label = { Text("Message") },
                                            placeholder = { Text("Follow up") },
                                            modifier = Modifier.weight(1f),
                                            minLines = 1,
                                            maxLines = 2
                                        )
                                        Button(
                                            onClick = onRunQuery,
                                            enabled = !loading && (query.isNotBlank() || composerAttachments.isNotEmpty())
                                        ) {
                                            Text("Send")
                                        }
                                    }
                                } else {
                                    ComposerHeader(
                                        title = null,
                                        attachments = composerAttachments,
                                        canCapturePhoto = canCapturePhoto,
                                        canUseVoiceInput = canUseVoiceInput,
                                        agentLabel = resolveSelectedAgentLabel(preferredAgentId, metadata)
                                            ?.takeIf { showWorkspaceAgentSelection(metadata) },
                                        subtitle = if (!activeConversationId.isNullOrBlank()) {
                                            if (showWorkspaceAgentSelection(metadata)) {
                                                "Replying as ${resolveSelectedAgentLabel(preferredAgentId, metadata) ?: "the selected agent"}"
                                            } else {
                                                "Continue the conversation"
                                            }
                                        } else {
                                            if (showWorkspaceAgentSelection(metadata)) {
                                                "Start a task with ${resolveSelectedAgentLabel(preferredAgentId, metadata) ?: "the selected agent"}"
                                            } else {
                                                "Start a new task"
                                            }
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
    }
}

@Composable
private fun WorkspaceHeaderDot(
    color: Color,
    description: String,
    selected: Boolean,
    onClick: () -> Unit
) {
    Surface(
        onClick = onClick,
        shape = CircleShape,
        color = color.copy(alpha = if (selected) 1f else 0.9f),
        border = if (selected) BorderStroke(2.dp, Color.White.copy(alpha = 0.9f)) else null,
        modifier = Modifier
            .semantics { contentDescription = description }
            .size(14.dp)
    ) {
        Spacer(modifier = Modifier.fillMaxSize())
    }
}
