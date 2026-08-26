package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
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
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.ChatBubbleOutline
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.KeyboardHide
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material.icons.outlined.Stop
import androidx.compose.material3.FilterChip
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.draw.shadow
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
import kotlinx.coroutines.delay

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
    onCancelTurn: (() -> Unit)? = null,
    bottomComposerInset: androidx.compose.ui.unit.Dp = 232.dp,
    composerVisible: Boolean = true,
    onToggleComposer: () -> Unit = {}
) {
    val headerTitle = resolveWorkspaceHeaderTitle(metadata, workspaceTitle)
    val runtimeWindows by forgeRuntime.windows.collectAsState(initial = emptyList())
    val localWorkspaceSnapshot = remember(activeConversationId, runtimeWindows) {
        buildAndroidUIBridgeSnapshot(activeConversationId, forgeRuntime)
    }
    val hostedWorkspaceState = deriveAgentlyHostedWorkspaceRestoreState(
        conversationState,
        streamSnapshot,
        localWorkspaceSnapshot
    )
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
            .then(
                if (workspaceFocused) {
                    Modifier.padding(bottom = bottomComposerInset)
                } else {
                    Modifier.verticalScroll(rememberScrollState())
                }
            ),
        verticalArrangement = Arrangement.spacedBy(if (workspaceFocused) 10.dp else 16.dp)
    ) {
        Column(
            modifier = Modifier.fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(if (workspaceFocused) 2.dp else 6.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(3.dp)
                ) {
                    Text(
                        headerTitle,
                        style = MaterialTheme.typography.titleSmall,
                        color = Color(0xFF182230),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                    if (!workspaceFocused) {
                        Text(
                            if (!activeConversationId.isNullOrBlank()) "Continuing your latest chat"
                            else "Ready for a new conversation",
                            style = MaterialTheme.typography.bodySmall,
                            color = Color(0xFF667085),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }
                Surface(
                    color = Color(0xFFF4F7FB),
                    shape = MaterialTheme.shapes.extraLarge,
                    border = BorderStroke(1.dp, Color(0xFFE4EAF2))
                ) {
                    Row(
                        modifier = Modifier.padding(4.dp),
                        horizontalArrangement = Arrangement.spacedBy(4.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                    if (activeConversationId.isNullOrBlank()) {
                        PhoneToolbarAction(
                            icon = Icons.Outlined.Edit,
                            contentDescription = "New chat",
                            onClick = onNewConversation,
                            accent = Color(0xFF1A73F0),
                            enabled = !loading
                        )
                        PhoneToolbarAction(
                            icon = Icons.Outlined.History,
                            contentDescription = "Conversation history",
                            onClick = onOpenHistory,
                            accent = Color(0xFFE08A1E),
                            enabled = recentConversations.isNotEmpty()
                        )
                    } else {
                        PhoneToolbarAction(
                            icon = Icons.Outlined.Home,
                            contentDescription = "Home",
                            onClick = onNewConversation,
                            accent = Color(0xFF1A73F0),
                            enabled = !loading
                        )
                        PhoneToolbarAction(
                            icon = Icons.Outlined.History,
                            contentDescription = "Conversation history",
                            onClick = onOpenHistory,
                            accent = Color(0xFFE08A1E),
                            enabled = recentConversations.isNotEmpty()
                        )
                    }
                    if (!activeConversationId.isNullOrBlank()) {
                        PhoneToolbarAction(
                            icon = if (composerVisible) Icons.Outlined.KeyboardHide else Icons.Outlined.ChatBubbleOutline,
                            contentDescription = if (composerVisible) "Hide chat composer" else "Show chat composer",
                            onClick = onToggleComposer,
                            accent = Color(0xFFDF5B78),
                            selected = composerVisible
                        )
                    }
                    PhoneToolbarAction(
                        icon = Icons.Outlined.Settings,
                        contentDescription = "Settings",
                        onClick = onOpenSettings,
                        accent = Color(0xFF7D52D9)
                    )
                    }
                }
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
                onSelectStarterTask = onStarterTaskSelected,
                starterTaskLayout = StarterTaskLayout.VerticalList,
            )
        }
        turnProgressPresentation(loading, conversationState, streamSnapshot)?.let { progress ->
            TurnProgressStatus(progress, onCancelTurn)
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
                    modifier = Modifier.weight(1f),
                    maxBodyHeight = hostedWorkspaceMinHeight.dp,
                    fillAvailableHeight = true,
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
                ActiveFeedsSection(
                    feeds = mergedVisibleFeeds(streamSnapshot, conversationState, activeConversationId),
                    conversationId = activeConversationId,
                    client = client,
                    forgeRuntime = forgeRuntime
                )
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
                    showStreamingStatusInHeader = false,
                    onClosePreview = onClosePreview,
                    onOpenFile = onOpenFile,
                    onOpenInlineReportPdf = onOpenInlineReportPdf,
                    workspaceAttachment = hostedWorkspaceAttachment(conversationState, hostedWorkspaceState),
                    onOpenWorkspace = { selectedMode = PhoneWorkspaceContentMode.Workspace },
                )
            }
        }
        if (!workspaceFocused) {
            Spacer(modifier = Modifier.height(bottomComposerInset))
        }
    }
}

@Composable
internal fun TurnProgressStatus(
    presentation: TurnProgressPresentation,
    onStop: (() -> Unit)? = null
) {
    var tokenDetailsOpen by remember { mutableStateOf(false) }
    var toolDetailsOpen by remember { mutableStateOf(false) }
    var elapsedSeconds by remember(
        presentation.title,
        presentation.detail,
        presentation.activity
    ) { mutableIntStateOf(0) }
    LaunchedEffect(presentation.title, presentation.detail, presentation.activity) {
        while (true) {
            delay(1_000)
            elapsedSeconds += 1
        }
    }
    val activityLabel = if (elapsedSeconds >= 5) {
        "${presentation.activity} · ${formatProgressElapsed(elapsedSeconds)}"
    } else {
        presentation.activity
    }
    Surface(
        color = Color(0xFFEFF7FF),
        border = BorderStroke(1.dp, Color(0xFFBFDBFE)),
        shape = MaterialTheme.shapes.large,
        modifier = Modifier.fillMaxWidth()
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            if (presentation.isWaitingForUser) {
                Text("!", color = Color(0xFFC26A00), fontWeight = androidx.compose.ui.text.font.FontWeight.Bold)
            } else {
                CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
            }
            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(3.dp)
            ) {
                Text(presentation.title, style = MaterialTheme.typography.labelLarge, color = Color(0xFF172B4D))
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    StatusChip(activityLabel, Color(0xFF1A73F0))
                    presentation.toolProgress?.let { StatusChip(it, Color(0xFF7D52D9)) { toolDetailsOpen = true } }
                    presentation.tokenUsage?.let { StatusChip(it, Color(0xFF667085)) { tokenDetailsOpen = true } }
                }
            }
            if (presentation.canStop && onStop != null) {
                IconButton(onClick = onStop, modifier = Modifier.size(32.dp)) {
                    Icon(Icons.Outlined.Stop, contentDescription = "Stop current request", tint = Color(0xFFD34B5F))
                }
            }
        }
    }
    if (toolDetailsOpen) {
        AlertDialog(
            onDismissRequest = { toolDetailsOpen = false },
            confirmButton = { TextButton(onClick = { toolDetailsOpen = false }) { Text("Done") } },
            title = { Text("Tool progress") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    if (presentation.toolDetails.isEmpty()) {
                        Text("Tool identities are not available yet.")
                    } else {
                        presentation.toolDetails.forEach { tool ->
                            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                                Text(userFacingToolActivity(tool.name) ?: tool.name)
                                Text(tool.status.replace('_', ' ').replaceFirstChar { it.uppercase() })
                            }
                        }
                    }
                }
            }
        )
    }
    if (tokenDetailsOpen && presentation.tokenDetails != null) {
        val details = presentation.tokenDetails
        AlertDialog(
            onDismissRequest = { tokenDetailsOpen = false },
            confirmButton = { TextButton(onClick = { tokenDetailsOpen = false }) { Text("Done") } },
            title = { Text(if (details.scope == "turn") "This turn" else "Conversation total") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    TokenDetailRow("Total", details.total)
                    TokenDetailRow("Input", details.input)
                    TokenDetailRow("Output", details.output)
                    if (details.cachedInput > 0) TokenDetailRow("Cached input", details.cachedInput)
                    if (details.reasoning > 0) TokenDetailRow("Reasoning", details.reasoning)
                    if (details.embedding > 0) TokenDetailRow("Embedding", details.embedding)
                    details.models.forEach { model ->
                        Text(model.label.ifBlank { "Model" }, fontWeight = androidx.compose.ui.text.font.FontWeight.SemiBold)
                        TokenDetailRow("Total", model.total)
                        TokenDetailRow("Input", model.input)
                        TokenDetailRow("Output", model.output)
                        TokenDetailRow("Cached input", model.cachedInput)
                        TokenDetailRow("Reasoning", model.reasoning)
                        TokenDetailRow("Embedding", model.embedding)
                    }
                }
            }
        )
    }
}

private fun formatProgressElapsed(seconds: Int): String = when {
    seconds < 60 -> "${seconds}s"
    else -> "${seconds / 60}m ${seconds % 60}s"
}

@Composable
private fun StatusChip(label: String, color: Color, onClick: (() -> Unit)? = null) {
    Text(
        label,
        style = MaterialTheme.typography.labelSmall,
        color = color,
        modifier = Modifier
            .background(color.copy(alpha = 0.09f), CircleShape)
            .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
            .padding(horizontal = 7.dp, vertical = 2.dp)
    )
}

@Composable
private fun TokenDetailRow(label: String, value: Int) {
    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        Text(label)
        Text("%,d".format(value), fontWeight = androidx.compose.ui.text.font.FontWeight.SemiBold)
    }
}

@Composable
private fun TokenDetailRow(label: String, value: Int?) {
    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        Text(label)
        Text(
            value?.let { "%,d".format(it) } ?: "Not reported",
            fontWeight = androidx.compose.ui.text.font.FontWeight.SemiBold,
            color = if (value == null) Color(0xFF667085) else Color.Unspecified
        )
    }
}

@Composable
internal fun PhoneToolbarAction(
    icon: ImageVector,
    contentDescription: String,
    onClick: () -> Unit,
    accent: Color,
    enabled: Boolean = true,
    selected: Boolean = false,
    loading: Boolean = false
) {
    val effectiveAccent = if (enabled || loading) accent else Color(0xFF98A2B3)
    Box(
        modifier = Modifier
            .size(44.dp)
            .shadow(
                elevation = if (enabled) 5.dp else 1.dp,
                shape = CircleShape,
                ambientColor = effectiveAccent.copy(alpha = 0.22f),
                spotColor = effectiveAccent.copy(alpha = 0.30f)
            )
            .background(
                brush = Brush.linearGradient(
                    colors = if (selected) {
                        listOf(effectiveAccent.copy(alpha = 0.28f), effectiveAccent.copy(alpha = 0.13f))
                    } else {
                        listOf(Color.White, effectiveAccent.copy(alpha = 0.10f))
                    }
                ),
                shape = CircleShape
            )
            .border(
                width = 1.dp,
                color = effectiveAccent.copy(alpha = if (selected) 0.48f else 0.24f),
                shape = CircleShape
            ),
        contentAlignment = Alignment.Center
    ) {
        IconButton(
            onClick = onClick,
            enabled = enabled,
            modifier = Modifier.size(44.dp)
        ) {
            if (loading) {
                CircularProgressIndicator(
                    modifier = Modifier.size(21.dp),
                    color = effectiveAccent,
                    strokeWidth = 2.dp
                )
            } else {
                Icon(
                    imageVector = icon,
                    contentDescription = contentDescription,
                    tint = effectiveAccent,
                    modifier = Modifier.size(22.dp)
                )
            }
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
