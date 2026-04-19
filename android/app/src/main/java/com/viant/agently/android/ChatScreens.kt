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
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.FilterChip
import androidx.compose.material3.LinearProgressIndicator
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
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.ui.MarkdownRenderer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement

@Composable
internal fun PhoneChatScreen(
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
    PhoneWorkspacePane(
        loading = loading,
        recentConversations = recentConversations,
        activeConversationId = activeConversationId,
        error = error,
        streamSnapshot = streamSnapshot,
        transcript = transcript,
        pendingApprovals = pendingApprovals,
        generatedFiles = generatedFiles,
        artifactPreview = artifactPreview,
        client = client,
        forgeRuntime = forgeRuntime,
        approvalJson = approvalJson,
        approvalEdits = approvalEdits,
        onRefresh = onRefresh,
        onNewConversation = onNewConversation,
        onOpenHistory = onOpenHistory,
        onOpenSettings = onOpenSettings,
        onSelectConversation = onSelectConversation,
        onEditChange = onEditChange,
        onDecision = onDecision,
        onOpenFile = onOpenFile,
        onClosePreview = onClosePreview
    )
}


@Composable
internal fun TabletChatScreen(
    appApiBaseUrl: String,
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
    onSelectConversation: (String) -> Unit,
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
    Row(
        modifier = Modifier.fillMaxSize(),
        horizontalArrangement = Arrangement.spacedBy(0.dp)
    ) {
        TabletConversationSidebar(
            appApiBaseUrl = appApiBaseUrl,
            loading = loading,
            recentConversations = recentConversations,
            activeConversationId = activeConversationId,
            onNewConversation = onNewConversation,
            onRefresh = onRefresh,
            onSelectConversation = onSelectConversation
        )

        Surface(
            color = Color(0xFFDDE4F1),
            modifier = Modifier
                .width(1.dp)
                .fillMaxHeight()
        ) {}

        Box(
            modifier = Modifier
                .weight(1f)
                .fillMaxHeight()
        ) {
            TabletWorkspacePane(
                loading = loading,
                activeConversationId = activeConversationId,
                error = error,
                streamSnapshot = streamSnapshot,
                transcript = transcript,
                pendingApprovals = pendingApprovals,
                generatedFiles = generatedFiles,
                artifactPreview = artifactPreview,
                client = client,
                forgeRuntime = forgeRuntime,
                approvalJson = approvalJson,
                approvalEdits = approvalEdits,
                onEditChange = onEditChange,
                onDecision = onDecision,
                onOpenFile = onOpenFile,
                onClosePreview = onClosePreview,
                query = query,
                onQueryChange = onQueryChange,
                composerAttachments = composerAttachments,
                canCapturePhoto = canCapturePhoto,
                canUseVoiceInput = canUseVoiceInput,
                onAddPhoto = onAddPhoto,
                onTakePhoto = onTakePhoto,
                onVoiceInput = onVoiceInput,
                onRemoveAttachment = onRemoveAttachment,
                onRunQuery = onRunQuery
            )
        }
    }
}
