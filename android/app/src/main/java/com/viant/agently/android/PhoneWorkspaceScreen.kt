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
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.AddComment
import androidx.compose.material.icons.outlined.ChatBubbleOutline
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.KeyboardHide
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.Goal
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.forgeandroid.runtime.ForgeRuntime
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement

private enum class PhoneWorkspaceContentMode {
    Workspace,
    Conversation
}

@Composable
internal fun PhoneWorkspacePane(
    workspaceTitle: String,
    metadata: WorkspaceMetadata?,
    preferredAgentId: String,
    loading: Boolean,
    recentConversations: List<Conversation>,
    activeConversationId: String?,
    openingConversationId: String?,
    conversationState: ConversationStateResponse?,
    activeGoal: Goal?,
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
    onSelectAgent: (String?) -> Unit,
    onOpenHistory: () -> Unit,
    onOpenSettings: () -> Unit,
    onSelectConversation: (String) -> Unit,
    onEditChange: (String, String, JsonElement) -> Unit,
    onDecision: (PendingToolApproval, String) -> Unit,
    onOpenFile: (GeneratedFileEntry) -> Unit,
    onOpenInlineReportPdf: (Map<String, Any?>, () -> Unit) -> Unit,
    onClosePreview: () -> Unit,
    onStarterTaskSelected: (String) -> Unit,
    bottomComposerInset: androidx.compose.ui.unit.Dp = 232.dp,
    composerVisible: Boolean = true,
    onToggleComposer: () -> Unit = {}
) {
    val headerTitle = resolveWorkspaceHeaderTitle(metadata, workspaceTitle)
    val hostedWorkspaceState = deriveAgentlyHostedWorkspaceRestoreState(conversationState, streamSnapshot)
    val displayTranscript = transcriptWithActiveAssistant(transcript, streamSnapshot)
    val hostedWorkspaceMinHeight = remember(hostedWorkspaceState) {
        hostedWorkspaceState?.windows
            ?.firstOrNull { it.windowId == hostedWorkspaceState.selectedWindowId }
            ?.workspaceMinHeight
            ?: hostedWorkspaceState?.windows?.lastOrNull()?.workspaceMinHeight
            ?: 420
    }.coerceIn(260, 900)
    val hasWorkspaceSurface = hostedWorkspaceState != null ||
        pendingApprovals.isNotEmpty() ||
        generatedFiles.isNotEmpty() ||
        artifactPreview != null
    var selectedMode by remember(activeConversationId, hostedWorkspaceState?.selectedWindowId, hasWorkspaceSurface) {
        mutableStateOf(
            if (hasWorkspaceSurface) PhoneWorkspaceContentMode.Workspace
            else PhoneWorkspaceContentMode.Conversation
        )
    }
    val workspaceFocused = hostedWorkspaceState != null && selectedMode == PhoneWorkspaceContentMode.Workspace

    LaunchedEffect(activeConversationId, hostedWorkspaceState?.selectedWindowId) {
        if (hostedWorkspaceState != null) {
            selectedMode = PhoneWorkspaceContentMode.Workspace
        }
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(if (workspaceFocused) 10.dp else 16.dp)
    ) {
        Column(
            modifier = Modifier.fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(if (workspaceFocused) 2.dp else 6.dp)
        ) {
            if (workspaceFocused) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Column(
                        modifier = Modifier.weight(1f),
                        verticalArrangement = Arrangement.spacedBy(3.dp)
                    ) {
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(
                                headerTitle,
                                style = MaterialTheme.typography.titleSmall,
                                color = Color(0xFF182230),
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                    }
                }
            } else {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Column(
                        modifier = Modifier.weight(1f),
                        verticalArrangement = Arrangement.spacedBy(3.dp)
                    ) {
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(
                                headerTitle,
                                style = MaterialTheme.typography.titleSmall,
                                color = Color(0xFF182230),
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                        Text(
                            if (!activeConversationId.isNullOrBlank()) "Continuing your latest chat"
                            else "Ready for a new conversation",
                            style = MaterialTheme.typography.bodySmall,
                            color = Color(0xFF667085)
                        )
                    }
                }
            }
            Surface(
                color = Color(0xFFF4F7FB),
                shape = MaterialTheme.shapes.extraLarge,
                border = BorderStroke(1.dp, Color(0xFFE4EAF2)),
                modifier = Modifier.fillMaxWidth()
            ) {
                Row(
                    modifier = Modifier.padding(horizontal = 7.dp, vertical = 6.dp),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    PhoneToolbarAction(
                        icon = Icons.Outlined.AddComment,
                        contentDescription = "New chat",
                        onClick = onNewConversation,
                        enabled = !loading
                    )
                    PhoneToolbarAction(
                        icon = Icons.Outlined.History,
                        contentDescription = "Conversation history",
                        onClick = onOpenHistory,
                        enabled = recentConversations.isNotEmpty()
                    )
                    if (!activeConversationId.isNullOrBlank()) {
                        PhoneToolbarAction(
                            icon = Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = "Back to conversation list",
                            onClick = onOpenHistory,
                            enabled = recentConversations.isNotEmpty()
                        )
                    }
                    PhoneToolbarAction(
                        icon = Icons.Outlined.Refresh,
                        contentDescription = "Refresh workspace",
                        onClick = onRefresh,
                        enabled = !loading
                    )
                    if (!activeConversationId.isNullOrBlank()) {
                        PhoneToolbarAction(
                            icon = if (composerVisible) Icons.Outlined.KeyboardHide else Icons.Outlined.ChatBubbleOutline,
                            contentDescription = if (composerVisible) "Hide chat composer" else "Show chat composer",
                            onClick = onToggleComposer,
                            selected = composerVisible
                        )
                    }
                    PhoneToolbarAction(
                        icon = Icons.Outlined.Settings,
                        contentDescription = "Settings",
                        onClick = onOpenSettings
                    )
                }
            }
            if (loading && streamSnapshot?.activeTurnId == null) {
                LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
            }
        }
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
        activeGoal?.let { GoalSummaryCard(goal = it) }
        if (hasWorkspaceSurface && displayTranscript.isNotEmpty()) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                FilterChip(
                    selected = selectedMode == PhoneWorkspaceContentMode.Workspace,
                    onClick = { selectedMode = PhoneWorkspaceContentMode.Workspace },
                    label = { Text("Workspace") }
                )
                FilterChip(
                    selected = selectedMode == PhoneWorkspaceContentMode.Conversation,
                    onClick = { selectedMode = PhoneWorkspaceContentMode.Conversation },
                    label = { Text("Conversation") }
                )
            }
        }
        if (activeConversationId.isNullOrBlank()) {
            WorkspaceTaskStartSection(
                metadata = metadata,
                preferredAgentId = preferredAgentId,
                onSelectAgent = onSelectAgent,
                onSelectStarterTask = onStarterTaskSelected
            )
        }
        if (streamSnapshot?.activeTurnId != null && hostedWorkspaceState == null) {
            val liveNarration = latestActiveNarration(streamSnapshot)
            Surface(
                color = Color(0xFFFFFFFF),
                border = BorderStroke(1.dp, Color(0xFFE2E8F3)),
                shape = MaterialTheme.shapes.large,
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(
                    modifier = Modifier.padding(14.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text(
                        liveNarration ?: "Preparing response…",
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF475467),
                        maxLines = 3,
                        overflow = TextOverflow.Ellipsis
                    )
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
        when {
            hasWorkspaceSurface && selectedMode == PhoneWorkspaceContentMode.Workspace -> {
                HostedWorkspaceSection(
                    restoreState = hostedWorkspaceState,
                    forgeRuntime = forgeRuntime,
                    maxBodyHeight = hostedWorkspaceMinHeight.dp,
                    // The report/dashboard owns its title and section navigation.
                    // A second hosted-workspace card wastes scarce phone width and
                    // makes a selected report tab look like a nested preview.
                    showTitle = false,
                    flatPresentation = true
                )
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
                artifactPreview?.let { preview ->
                    if (generatedFiles.none { it.id == preview.artifactId }) {
                        ArtifactPreviewSection(
                            preview = preview,
                            onClose = onClosePreview
                        )
                    }
                }
                ConversationArtifactsSection(
                    files = generatedFiles,
                    onOpenFile = onOpenFile
                )
                if (hostedWorkspaceState == null &&
                    pendingApprovals.isEmpty() &&
                    generatedFiles.isEmpty() &&
                    artifactPreview == null
                ) {
                    WorkspaceModePlaceholder()
                }
            }

            else -> {
                if (displayTranscript.isEmpty() &&
                    (!activeConversationId.isNullOrBlank() || recentConversations.isNotEmpty())
                ) {
                    RecentConversationsSection(
                        conversations = recentConversations,
                        activeConversationId = activeConversationId,
                        openingConversationId = openingConversationId,
                        onSelectConversation = onSelectConversation
                    )
                }
                RenderTranscript(
                    items = displayTranscript,
                    pendingApprovals = pendingApprovals,
                    generatedFiles = generatedFiles,
                    client = client,
                    forgeRuntime = forgeRuntime,
                    approvalJson = approvalJson,
                    approvalEdits = approvalEdits,
                    onEditChange = onEditChange,
                    onDecision = onDecision,
                    artifactPreview = artifactPreview,
                    onClosePreview = onClosePreview,
                    onOpenFile = onOpenFile,
                    onOpenInlineReportPdf = onOpenInlineReportPdf
                )
            }
        }
        Spacer(modifier = Modifier.height(bottomComposerInset))
    }
}

@Composable
private fun PhoneToolbarAction(
    icon: ImageVector,
    contentDescription: String,
    onClick: () -> Unit,
    enabled: Boolean = true,
    selected: Boolean = false
) {
    Surface(
        color = if (selected) Color(0xFFE5EEFF) else Color.White,
        shape = MaterialTheme.shapes.large,
        border = BorderStroke(
            1.dp,
            if (selected) Color(0xFF9FC0FF) else Color(0xFFDCE3EC)
        ),
        modifier = Modifier.size(42.dp)
    ) {
        IconButton(
            onClick = onClick,
            enabled = enabled,
            modifier = Modifier.size(42.dp)
        ) {
            Icon(
                imageVector = icon,
                contentDescription = contentDescription,
                tint = when {
                    !enabled -> Color(0xFF98A2B3)
                    selected -> Color(0xFF2457C5)
                    else -> Color(0xFF344054)
                },
                modifier = Modifier.size(21.dp)
            )
        }
    }
}

@Composable
private fun WorkspaceModePlaceholder() {
    Surface(
        color = Color(0xFFF8FAFD),
        border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
        shape = MaterialTheme.shapes.large,
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 18.dp, vertical = 22.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Text(
                "Workspace ready",
                style = MaterialTheme.typography.titleSmall,
                color = Color(0xFF182230)
            )
            Text(
                "Hosted workspace views, approvals, and generated outputs appear here when the conversation opens them.",
                style = MaterialTheme.typography.bodySmall,
                color = Color(0xFF667085)
            )
        }
    }
}
