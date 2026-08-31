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
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.FilterChip
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.platform.LocalContext
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.Goal
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
@OptIn(ExperimentalComposeUiApi::class)
internal fun TabletWorkspacePane(
    loading: Boolean,
    activeConversationId: String?,
    metadata: WorkspaceMetadata?,
    preferredAgentId: String,
    conversationState: ConversationStateResponse?,
    activeGoal: Goal?,
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
    onOpenInlineReportPdf: (Map<String, Any?>, () -> Unit) -> Unit,
    onClosePreview: () -> Unit,
    onSelectAgent: (String?) -> Unit,
    query: String,
    onQueryChange: (String) -> Unit,
    composerAttachments: List<ComposerAttachmentDraft>,
    lookupOccurrences: List<ComposerLookupOccurrence> = emptyList(),
    lookupSelections: Map<String, ComposerLookupSelection> = emptyMap(),
    onLookupClick: (ComposerLookupOccurrence) -> Unit = {},
    canCapturePhoto: Boolean,
    canUseVoiceInput: Boolean,
    onAddPhoto: () -> Unit,
    onTakePhoto: () -> Unit,
    onVoiceInput: () -> Unit,
    onRemoveAttachment: (String) -> Unit,
    onRunQuery: () -> Unit,
    onCancelTurn: () -> Unit = {}
) {
    val context = LocalContext.current
    val runtimeWindows by forgeRuntime.windows.collectAsState(initial = emptyList())
    val localWorkspaceSnapshot = remember(activeConversationId, runtimeWindows) {
        buildAndroidUIBridgeSnapshot(activeConversationId, forgeRuntime)
    }
    val hostedWorkspaceState = deriveAgentlyHostedWorkspaceRestoreState(
        conversationState,
        streamSnapshot,
        localWorkspaceSnapshot
    )
    val hostedWorkspacePresentation = remember(hostedWorkspaceState?.selectedWindowId, hostedWorkspaceState?.windows) {
        resolveHostedWorkspacePresentation(hostedWorkspaceState)
    }
    val displayTranscript = transcriptWithActiveAssistant(transcript, streamSnapshot)
    val hasMainContent = displayTranscript.isNotEmpty() || pendingApprovals.isNotEmpty() || generatedFiles.isNotEmpty() || !activeConversationId.isNullOrBlank()
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
                        activeGoal?.let { GoalSummaryCard(goal = it) }
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
                                    showTitle = true,
                                    headerActions = {
                                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                            WorkspacePanelActionChip(
                                                label = "Hide",
                                                selected = workspacePanelMode == WorkspacePanelMode.Hidden,
                                                onClick = { workspacePanelMode = WorkspacePanelMode.Hidden }
                                            )
                                            WorkspacePanelActionChip(
                                                label = "Split",
                                                selected = workspacePanelMode == WorkspacePanelMode.Split,
                                                onClick = { workspacePanelMode = WorkspacePanelMode.Split }
                                            )
                                            WorkspacePanelActionChip(
                                                label = "Focus",
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
                                            Text(
                                                hostedWorkspacePresentation?.title ?: "Workspace closed",
                                                style = MaterialTheme.typography.titleSmall
                                            )
                                            Text(
                                                "Reopen the ${hostedWorkspacePresentation?.badgeLabel?.lowercase() ?: "hosted"} workspace when you want the full data-driven view back.",
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
                                ActiveFeedsSection(
                                    feeds = mergedVisibleFeeds(streamSnapshot, conversationState, activeConversationId),
                                    conversationId = activeConversationId,
                                    client = client,
                                    forgeRuntime = forgeRuntime
                                )
                                ActiveFeedsSection(
                                    feeds = mergedVisibleFeeds(streamSnapshot, conversationState, activeConversationId),
                                    conversationId = activeConversationId,
                                    client = client,
                                    forgeRuntime = forgeRuntime,
                                    placement = AndroidFeedPlacement.Detached,
                                    sectionTitle = "Feed apps"
                                )
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
                                if (showExecutionDetails) {
                                    ExecutionInspectorSection(
                                        state = conversationState,
                                        client = client
                                    )
                                } else {
                                    RenderTranscript(
                                        items = displayTranscript,
                                        conversationId = activeConversationId,
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
                                        onOpenInlineReportPdf = onOpenInlineReportPdf,
                                        activeFeeds = mergedVisibleFeeds(streamSnapshot, conversationState, activeConversationId)
                                    )
                                }
                                Spacer(modifier = Modifier.padding(bottom = 24.dp))
                            }
                        }
                    }

                    if (workspacePanelMode != WorkspacePanelMode.Expanded) {
                        val compactComposer = !activeConversationId.isNullOrBlank()
                        val composerWidthFraction = if (compactComposer) 0.82f else 0.9f
                        val inputContentDescription = "Message"
                        val sendContentDescription = "Send"
                        val inputTestTag = if (activeConversationId.isNullOrBlank()) {
                            "new_conversation_composer_input"
                        } else {
                            "reply_composer_input"
                        }
                        val sendTestTag = if (activeConversationId.isNullOrBlank()) {
                            "send_new_conversation"
                        } else {
                            "send_reply"
                        }
                        Surface(
                            modifier = Modifier
                                .fillMaxWidth(composerWidthFraction)
                                .widthIn(max = if (compactComposer) 1040.dp else 1200.dp)
                                .align(Alignment.CenterHorizontally)
                                .imePadding()
                                .navigationBarsPadding()
                                .semantics { testTagsAsResourceId = true },
                            color = Color(0xFFFDFDFE),
                            border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
                            shape = MaterialTheme.shapes.large
                        ) {
                            Column(
                                modifier = Modifier.padding(
                                    horizontal = if (compactComposer) 16.dp else 20.dp,
                                    vertical = if (compactComposer) 10.dp else 12.dp
                                ),
                                verticalArrangement = Arrangement.spacedBy(if (compactComposer) 8.dp else 10.dp)
                            ) {
                                val unresolvedRequiredLookup = firstUnresolvedRequiredComposerLookup(
                                    lookupOccurrences,
                                    lookupSelections
                                )
                                val sendButtonLabel = composerSendButtonLabel(unresolvedRequiredLookup)
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
                                            modifier = Modifier
                                                .weight(1f)
                                                .testTag(inputTestTag)
                                                .semantics { contentDescription = inputContentDescription },
                                            minLines = 1,
                                            maxLines = composerInputMaxLines(compactComposer, query),
                                            keyboardOptions = KeyboardOptions(
                                                capitalization = KeyboardCapitalization.None
                                            ),
                                            visualTransformation = composerLookupVisualTransformation(lookupOccurrences),
                                            colors = OutlinedTextFieldDefaults.colors(
                                                focusedContainerColor = ComposerInputFill,
                                                unfocusedContainerColor = ComposerInputFill,
                                                disabledContainerColor = ComposerInputFill,
                                                focusedBorderColor = ComposerInputBorder,
                                                unfocusedBorderColor = ComposerInputBorder,
                                                disabledBorderColor = ComposerInputBorder.copy(alpha = 0.6f)
                                            )
                                        )
                                        Button(
                                            onClick = onRunQuery,
                                            enabled = !loading && (query.isNotBlank() || composerAttachments.isNotEmpty()),
                                            modifier = Modifier
                                                .testTag(sendTestTag)
                                                .semantics { contentDescription = sendContentDescription }
                                        ) {
                                            Text(sendButtonLabel)
                                        }
                                    }
                                    if (lookupOccurrences.isNotEmpty()) {
                                        ComposerLookupChipsRow(
                                            occurrences = lookupOccurrences,
                                            selections = lookupSelections,
                                            onLookupClick = onLookupClick
                                        )
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
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .testTag(inputTestTag)
                                            .semantics { contentDescription = inputContentDescription },
                                        minLines = 1,
                                        maxLines = composerInputMaxLines(compactComposer, query),
                                        keyboardOptions = KeyboardOptions(
                                            capitalization = KeyboardCapitalization.None
                                        ),
                                        visualTransformation = composerLookupVisualTransformation(lookupOccurrences),
                                        colors = OutlinedTextFieldDefaults.colors(
                                            focusedContainerColor = ComposerInputFill,
                                            unfocusedContainerColor = ComposerInputFill,
                                            disabledContainerColor = ComposerInputFill,
                                            focusedBorderColor = ComposerInputBorder,
                                            unfocusedBorderColor = ComposerInputBorder,
                                            disabledBorderColor = ComposerInputBorder.copy(alpha = 0.6f)
                                        )
                                    )
                                    if (lookupOccurrences.isNotEmpty()) {
                                        ComposerLookupChipsRow(
                                            occurrences = lookupOccurrences,
                                            selections = lookupSelections,
                                            onLookupClick = onLookupClick
                                        )
                                    }
                                    Row(
                                        modifier = Modifier.fillMaxWidth(),
                                        horizontalArrangement = Arrangement.End,
                                        verticalAlignment = Alignment.CenterVertically
                                    ) {
                                        Button(
                                            onClick = onRunQuery,
                                            enabled = !loading && (query.isNotBlank() || composerAttachments.isNotEmpty()),
                                            modifier = Modifier
                                                .testTag(sendTestTag)
                                                .semantics { contentDescription = sendContentDescription }
                                        ) {
                                            Text(sendButtonLabel)
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
private fun WorkspacePanelActionChip(
    label: String,
    selected: Boolean,
    onClick: () -> Unit
) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        modifier = Modifier.semantics { contentDescription = label },
        label = { Text(label) }
    )
}
