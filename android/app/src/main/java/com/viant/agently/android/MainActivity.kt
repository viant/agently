package com.viant.agently.android

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.TextButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.toMutableStateList
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.Conversation
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.CreateConversationInput
import com.viant.agentlysdk.DecideToolApprovalInput
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.ListConversationsInput
import com.viant.agentlysdk.ListPendingToolApprovalsInput
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.QueryInput
import com.viant.agentlysdk.QueryOutput
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.stream.BufferedMessage
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.forgeandroid.runtime.EndpointConfig
import com.viant.forgeandroid.runtime.sessionHttpClient
import com.viant.forgeandroid.ui.MarkdownRenderer
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonPrimitive
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.time.OffsetDateTime

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    AgentlyApp()
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AgentlyApp() {
    val scope = rememberCoroutineScope()
    val client = remember {
        AgentlyClient(
            endpoints = mapOf(
                "appAPI" to EndpointConfig(
                    baseUrl = "http://10.0.2.2:9393",
                    httpClient = sessionHttpClient()
                )
            )
        )
    }
    var loading by remember { mutableStateOf(false) }
    var metadata by remember { mutableStateOf<WorkspaceMetadata?>(null) }
    var query by remember { mutableStateOf("Give me a short Android-ready markdown response.") }
    var result by remember { mutableStateOf<QueryOutput?>(null) }
    var streamSnapshot by remember { mutableStateOf<ConversationStreamSnapshot?>(null) }
    var streamedMarkdown by remember { mutableStateOf<String?>(null) }
    var activeConversationId by remember { mutableStateOf<String?>(null) }
    var recentConversations by remember { mutableStateOf<List<Conversation>>(emptyList()) }
    var currentScreen by remember { mutableStateOf(AppScreen.Chat) }
    var pendingApprovals by remember { mutableStateOf<List<PendingToolApproval>>(emptyList()) }
    var generatedFiles by remember { mutableStateOf<List<GeneratedFileEntry>>(emptyList()) }
    var artifactPreview by remember { mutableStateOf<ArtifactPreview?>(null) }
    var streamJob by remember { mutableStateOf<Job?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    var approvalEdits by remember { mutableStateOf<Map<String, Map<String, JsonElement>>>(emptyMap()) }
    val transcript = remember { mutableListOf<ChatEntry>().toMutableStateList() }
    val approvalJson = remember { Json { ignoreUnknownKeys = true } }

    fun resetConversation() {
        streamJob?.cancel()
        streamJob = null
        activeConversationId = null
        streamSnapshot = null
        streamedMarkdown = null
        result = null
        error = null
        transcript.clear()
        pendingApprovals = emptyList()
        approvalEdits = emptyMap()
        generatedFiles = emptyList()
        artifactPreview = null
    }

    suspend fun refreshRecentConversations() {
        recentConversations = client.listConversations(
            ListConversationsInput(
                page = com.viant.agentlysdk.PageInput(limit = 10)
            )
        ).rows
    }

    suspend fun bindConversation(conversationId: String, replaceTranscript: Boolean) {
        streamJob?.cancelAndJoin()
        val state = client.getLiveState(conversationId)
        activeConversationId = conversationId
        pendingApprovals = runCatching {
            client.listPendingToolApprovals(
                ListPendingToolApprovalsInput(
                    conversationId = conversationId,
                    status = "pending",
                    limit = 20
                )
            )
        }.getOrDefault(emptyList())
        approvalEdits = approvalEdits.filterKeys { approvalId ->
            pendingApprovals.any { it.id == approvalId }
        }
        generatedFiles = runCatching { client.listGeneratedFiles(conversationId) }.getOrDefault(emptyList())
        streamSnapshot = null
        streamedMarkdown = null
        if (replaceTranscript) {
            transcript.clear()
            transcript.addAll(transcriptFromState(state))
        }
        streamJob = scope.launch {
            try {
                client.trackConversation(conversationId).collect { snapshot ->
                    streamSnapshot = snapshot
                    if (snapshot.conversationId.isNotBlank()) {
                        activeConversationId = snapshot.conversationId
                    }
                    streamedMarkdown = latestAssistantMarkdown(snapshot) ?: streamedMarkdown
                    syncAssistantTranscript(transcript, snapshot)
                }
            } catch (err: Throwable) {
                error = err.message ?: err.toString()
            }
        }
    }

    fun loadWorkspace() {
        scope.launch {
            loading = true
            error = null
            try {
                metadata = client.getWorkspaceMetadata()
                refreshRecentConversations()
            } catch (err: Throwable) {
                error = err.message ?: err.toString()
            } finally {
                loading = false
            }
        }
    }

    fun runQuery() {
        scope.launch {
            loading = true
            error = null
            try {
                val meta = metadata ?: client.getWorkspaceMetadata().also { metadata = it }
                streamJob?.cancelAndJoin()
                result = null
                streamSnapshot = null
                streamedMarkdown = null
                val userEntryId = "user-${System.currentTimeMillis()}"
                transcript.add(
                    ChatEntry(
                        id = userEntryId,
                        role = "user",
                        markdown = query.trim(),
                        timestampLabel = formatTimestampLabel(System.currentTimeMillis())
                    )
                )

                val conversationId = activeConversationId ?: client.createConversation(
                    CreateConversationInput(
                        agentId = meta.defaultAgent ?: meta.defaults?.agent,
                        title = query.take(80)
                    )
                ).id
                activeConversationId = conversationId

                bindConversation(conversationId, replaceTranscript = false)

                result = client.query(
                    QueryInput(
                        conversationId = conversationId,
                        agentId = meta.defaultAgent ?: meta.defaults?.agent,
                        model = meta.defaultModel ?: meta.defaults?.model,
                        query = query
                    )
                )
                activeConversationId = result?.conversationId ?: conversationId
                if (!result?.content.isNullOrBlank()) {
                    streamedMarkdown = result?.content
                    syncAssistantResult(transcript, result?.messageId, result?.content.orEmpty())
                }
                generatedFiles = runCatching { client.listGeneratedFiles(conversationId) }.getOrDefault(generatedFiles)
                pendingApprovals = runCatching {
                    client.listPendingToolApprovals(
                        ListPendingToolApprovalsInput(
                            conversationId = conversationId,
                            status = "pending",
                            limit = 20
                        )
                    )
                }.getOrDefault(pendingApprovals)
                approvalEdits = approvalEdits.filterKeys { approvalId ->
                    pendingApprovals.any { it.id == approvalId }
                }
                refreshRecentConversations()
            } catch (err: Throwable) {
                error = err.message ?: err.toString()
            } finally {
                loading = false
            }
        }
    }

    fun openGeneratedFile(file: GeneratedFileEntry) {
        scope.launch {
            error = null
            try {
                val downloaded = client.downloadGeneratedFile(file.id)
                val previewText = if (isPreviewableText(downloaded.contentType, downloaded.name)) {
                    downloaded.data.toString(Charsets.UTF_8)
                } else {
                    null
                }
                artifactPreview = ArtifactPreview(
                    artifactId = file.id,
                    name = downloaded.name ?: file.filename ?: file.id.take(12),
                    contentType = downloaded.contentType ?: file.mimeType,
                    text = previewText,
                    sizeBytes = downloaded.data.size
                )
            } catch (err: Throwable) {
                error = err.message ?: err.toString()
            }
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            streamJob?.cancel()
        }
    }

    LaunchedEffect(Unit) {
        loadWorkspace()
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .padding(16.dp)
    ) {
        when (currentScreen) {
            AppScreen.Chat -> {
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
                            Text("Agently Android", style = MaterialTheme.typography.headlineSmall)
                            Text(
                                "Mobile chat shell backed by agently-core streaming and Forge rendering.",
                                style = MaterialTheme.typography.bodyMedium,
                                color = Color(0xFF667085)
                            )
                            Row(
                                modifier = Modifier.horizontalScroll(rememberScrollState()),
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                AssistChip(
                                    onClick = {},
                                    enabled = false,
                                    label = { Text("Backend 10.0.2.2:9393") }
                                )
                                metadata?.defaultAgent?.takeIf { it.isNotBlank() }?.let {
                                    AssistChip(onClick = {}, enabled = false, label = { Text("Agent $it") })
                                }
                                (metadata?.defaultModel ?: metadata?.defaults?.model)?.takeIf { it.isNotBlank() }?.let {
                                    AssistChip(onClick = {}, enabled = false, label = { Text("Model $it") })
                                }
                            }
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                Button(onClick = { loadWorkspace() }, enabled = !loading) {
                                    Text("Refresh")
                                }
                                OutlinedButton(onClick = { resetConversation() }, enabled = !loading) {
                                    Text("New Conversation")
                                }
                                OutlinedButton(
                                    onClick = { currentScreen = AppScreen.History },
                                    enabled = recentConversations.isNotEmpty()
                                ) {
                                    Text("History")
                                }
                                if (loading) {
                                    CircularProgressIndicator(modifier = Modifier.width(24.dp))
                                }
                            }
                        }
                    }
                    metadata?.let { meta ->
                        Card(modifier = Modifier.fillMaxWidth()) {
                            Column(
                                modifier = Modifier.padding(14.dp),
                                verticalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Text("Workspace", style = MaterialTheme.typography.titleMedium)
                                Text(meta.workspaceRoot ?: "unknown", style = MaterialTheme.typography.bodySmall)
                                Row(
                                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    FilterChip(
                                        selected = true,
                                        onClick = {},
                                        label = { Text("Agent ${meta.defaultAgent ?: meta.defaults?.agent ?: "n/a"}") }
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
                    RecentConversationsSection(
                        conversations = recentConversations,
                        activeConversationId = activeConversationId,
                        onSelectConversation = { conversationId ->
                            scope.launch {
                                loading = true
                                error = null
                                try {
                                    bindConversation(conversationId, replaceTranscript = true)
                                } catch (err: Throwable) {
                                    error = err.message ?: err.toString()
                                } finally {
                                    loading = false
                                }
                            }
                        }
                    )
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
                        Text("Conversation: ${activeConversationId ?: "n/a"}", style = MaterialTheme.typography.bodySmall)
                    }
                    streamSnapshot?.activeTurnId?.let { turnId ->
                        Text("Active turn: $turnId", style = MaterialTheme.typography.bodySmall, color = Color(0xFF667085))
                    }
                    if (loading || streamSnapshot?.activeTurnId != null) {
                        LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
                    }
                    PendingApprovalsSection(
                        approvals = pendingApprovals,
                        approvalJson = approvalJson,
                        approvalEdits = approvalEdits,
                        onEditChange = { approvalId, fieldName, value ->
                            approvalEdits = approvalEdits.toMutableMap().apply {
                                val nextFields = (this[approvalId] ?: emptyMap()).toMutableMap()
                                nextFields[fieldName] = value
                                this[approvalId] = nextFields
                            }
                        },
                        onDecision = { approval, action ->
                            scope.launch {
                                loading = true
                                error = null
                                try {
                                    client.decideToolApproval(
                                        DecideToolApprovalInput(
                                            id = approval.id,
                                            action = action,
                                            editedFields = approvalEdits[approval.id] ?: emptyMap()
                                        )
                                    )
                                    val conversationId = activeConversationId
                                    pendingApprovals = if (!conversationId.isNullOrBlank()) {
                                        client.listPendingToolApprovals(
                                            ListPendingToolApprovalsInput(
                                                conversationId = conversationId,
                                                status = "pending",
                                                limit = 20
                                            )
                                        )
                                    } else {
                                        emptyList()
                                    }
                                    approvalEdits = approvalEdits - approval.id
                                } catch (err: Throwable) {
                                    error = err.message ?: err.toString()
                                } finally {
                                    loading = false
                                }
                            }
                        }
                    )
                    ConversationArtifactsSection(
                        files = generatedFiles,
                        onOpenFile = { openGeneratedFile(it) }
                    )
                    artifactPreview?.let { preview ->
                        if (generatedFiles.none { it.id == preview.artifactId }) {
                            ArtifactPreviewSection(
                                preview = preview,
                                onClose = { artifactPreview = null }
                            )
                        }
                    }
                    RenderTranscript(
                        items = transcript,
                        pendingApprovals = pendingApprovals,
                        generatedFiles = generatedFiles,
                        approvalJson = approvalJson,
                        approvalEdits = approvalEdits,
                        onEditChange = { approvalId, fieldName, value ->
                            approvalEdits = approvalEdits.toMutableMap().apply {
                                val nextFields = (this[approvalId] ?: emptyMap()).toMutableMap()
                                nextFields[fieldName] = value
                                this[approvalId] = nextFields
                            }
                        },
                        onDecision = { approval, action ->
                            scope.launch {
                                loading = true
                                error = null
                                try {
                                    client.decideToolApproval(
                                        DecideToolApprovalInput(
                                            id = approval.id,
                                            action = action,
                                            editedFields = approvalEdits[approval.id] ?: emptyMap()
                                        )
                                    )
                                    val conversationId = activeConversationId
                                    pendingApprovals = if (!conversationId.isNullOrBlank()) {
                                        client.listPendingToolApprovals(
                                            ListPendingToolApprovalsInput(
                                                conversationId = conversationId,
                                                status = "pending",
                                                limit = 20
                                            )
                                        )
                                    } else {
                                        emptyList()
                                    }
                                    approvalEdits = approvalEdits - approval.id
                                } catch (err: Throwable) {
                                    error = err.message ?: err.toString()
                                } finally {
                                    loading = false
                                }
                            }
                        },
                        artifactPreview = artifactPreview,
                        onClosePreview = { artifactPreview = null },
                        onOpenFile = { openGeneratedFile(it) }
                    )
                    Spacer(modifier = Modifier.height(200.dp))
                }
            }
            AppScreen.History -> {
                ConversationHistoryScreen(
                    conversations = recentConversations,
                    activeConversationId = activeConversationId,
                    loading = loading,
                    onBack = { currentScreen = AppScreen.Chat },
                    onRefresh = { loadWorkspace() },
                    onSelectConversation = { conversationId ->
                        scope.launch {
                            loading = true
                            error = null
                            try {
                                bindConversation(conversationId, replaceTranscript = true)
                                currentScreen = AppScreen.Chat
                            } catch (err: Throwable) {
                                error = err.message ?: err.toString()
                            } finally {
                                loading = false
                            }
                        }
                    }
                )
            }
        }
        if (currentScreen == AppScreen.Chat) {
            Card(
                modifier = Modifier
                    .fillMaxWidth()
                    .align(Alignment.BottomCenter)
                    .navigationBarsPadding()
            ) {
                Column(
                    modifier = Modifier.padding(14.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    Text("Composer", style = MaterialTheme.typography.titleMedium)
                    OutlinedTextField(
                        value = query,
                        onValueChange = { query = it },
                        label = { Text("Message") },
                        placeholder = { Text("Ask a follow-up or start a new task") },
                        modifier = Modifier.fillMaxWidth(),
                        minLines = 3,
                        maxLines = 6
                    )
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            if (!activeConversationId.isNullOrBlank()) "Continuing current conversation" else "A new conversation will be created",
                            style = MaterialTheme.typography.bodySmall,
                            color = Color(0xFF667085)
                        )
                        Button(onClick = { runQuery() }, enabled = !loading && query.isNotBlank()) {
                            Text("Send")
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ConversationHistoryScreen(
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
private fun PendingApprovalsSection(
    approvals: List<PendingToolApproval>,
    approvalJson: Json,
    approvalEdits: Map<String, Map<String, JsonElement>>,
    onEditChange: (String, String, JsonElement) -> Unit,
    onDecision: (PendingToolApproval, String) -> Unit
) {
    if (approvals.isEmpty()) {
        return
    }
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Text("Approvals", style = MaterialTheme.typography.titleMedium)
            approvals.forEach { approval ->
                ApprovalCardContent(
                    approval = approval,
                    meta = approval.metadata?.let {
                        runCatching { approvalJson.decodeFromJsonElement<com.viant.agentlysdk.ApprovalMeta>(it) }.getOrNull()
                    },
                    selectedFields = approvalEdits[approval.id].orEmpty(),
                    onEditChange = onEditChange,
                    onDecision = onDecision,
                    containerColor = MaterialTheme.colorScheme.surfaceVariant
                )
            }
        }
    }
}

@Composable
private fun ConversationArtifactsSection(
    files: List<GeneratedFileEntry>,
    onOpenFile: (GeneratedFileEntry) -> Unit
) {
    if (files.isEmpty()) {
        return
    }
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Text("Artifacts", style = MaterialTheme.typography.titleMedium)
            Row(
                modifier = Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                files.forEach { file ->
                    Surface(
                        color = MaterialTheme.colorScheme.surfaceVariant,
                        shape = MaterialTheme.shapes.medium,
                        modifier = Modifier.width(220.dp)
                    ) {
                        Column(
                            modifier = Modifier.padding(12.dp),
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Text(
                                file.filename ?: file.id.take(12),
                                style = MaterialTheme.typography.titleSmall
                            )
                            Text(
                                file.status ?: "unknown",
                                style = MaterialTheme.typography.labelMedium,
                                color = Color(0xFF667085)
                            )
                            file.mimeType?.let {
                                Text(it, style = MaterialTheme.typography.bodySmall)
                            }
                            file.sizeBytes?.let {
                                Text("$it bytes", style = MaterialTheme.typography.bodySmall)
                            }
                            TextButton(
                                onClick = { onOpenFile(file) },
                                modifier = Modifier.fillMaxWidth()
                            ) {
                                Text("Open")
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ArtifactPreviewSection(
    preview: ArtifactPreview,
    onClose: () -> Unit
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text("Artifact Preview", style = MaterialTheme.typography.titleMedium)
                    Text(preview.name, style = MaterialTheme.typography.bodySmall)
                    Text(
                        "${preview.contentType ?: "unknown"} · ${preview.sizeBytes} bytes",
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                }
                TextButton(onClick = onClose) {
                    Text("Close")
                }
            }
            if (preview.text != null) {
                Surface(
                    color = MaterialTheme.colorScheme.surfaceVariant,
                    shape = MaterialTheme.shapes.medium,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text(
                        preview.text,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.padding(12.dp)
                    )
                }
            } else {
                Text(
                    "Binary artifact downloaded. Inline preview is available for text-based outputs only.",
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFF667085)
                )
            }
        }
    }
}

@Composable
private fun RecentConversationsSection(
    conversations: List<Conversation>,
    activeConversationId: String?,
    onSelectConversation: (String) -> Unit
) {
    if (conversations.isEmpty()) {
        return
    }
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Text("Recent Conversations", style = MaterialTheme.typography.titleMedium)
            Row(
                modifier = Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                conversations.forEach { conversation ->
                    val isActive = conversation.id == activeConversationId
                    val summary = conversation.summary
                    Surface(
                        color = if (isActive) Color(0xFFEFF8FF) else MaterialTheme.colorScheme.surfaceVariant,
                        shape = MaterialTheme.shapes.medium,
                        modifier = Modifier.width(250.dp)
                    ) {
                        Column(
                            modifier = Modifier.padding(12.dp),
                            verticalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Text(
                                conversation.title?.takeIf { it.isNotBlank() } ?: conversation.id.take(12),
                                style = MaterialTheme.typography.titleSmall
                            )
                            Text(
                                summary?.takeIf { it.isNotBlank() } ?: "Conversation ${conversation.id.take(12)}",
                                style = MaterialTheme.typography.bodySmall,
                                color = Color(0xFF667085)
                            )
                            OutlinedButton(
                                onClick = { onSelectConversation(conversation.id) },
                                enabled = !isActive,
                                modifier = Modifier.fillMaxWidth()
                            ) {
                                Text(if (isActive) "Active" else "Open")
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun RenderTranscript(
    items: List<ChatEntry>,
    pendingApprovals: List<PendingToolApproval>,
    generatedFiles: List<GeneratedFileEntry>,
    approvalJson: Json,
    approvalEdits: Map<String, Map<String, JsonElement>>,
    onEditChange: (String, String, JsonElement) -> Unit,
    onDecision: (PendingToolApproval, String) -> Unit,
    artifactPreview: ArtifactPreview?,
    onClosePreview: () -> Unit,
    onOpenFile: (GeneratedFileEntry) -> Unit
) {
    if (items.isEmpty()) {
        return
    }
    Text("Transcript", style = MaterialTheme.typography.titleMedium)
    items.forEachIndexed { index, item ->
        val previous = items.getOrNull(index - 1)
        val startsGroup = previous?.role != item.role
        val messageApprovals = if (item.role == "assistant") {
            pendingApprovals.filter { it.messageId == item.id }
        } else {
            emptyList()
        }
        val messageArtifacts = if (item.role == "assistant") {
            generatedFiles.filter { it.messageId == item.id }
        } else {
            emptyList()
        }
        val inlinePreview = artifactPreview?.takeIf { preview ->
            messageArtifacts.any { it.id == preview.artifactId }
        }
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = if (item.role == "user") Arrangement.End else Arrangement.Start
        ) {
            Surface(
                color = if (item.role == "user") Color(0xFFF5F8FF) else MaterialTheme.colorScheme.surfaceVariant,
                shape = MaterialTheme.shapes.large,
                modifier = Modifier.fillMaxWidth(0.92f)
            ) {
                Column(
                    modifier = Modifier.padding(14.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    if (startsGroup || item.streaming) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text(
                                if (item.role == "user") "You" else if (item.streaming) "Assistant is responding..." else "Assistant",
                                style = MaterialTheme.typography.labelLarge,
                                color = if (item.role == "user") Color(0xFF1849A9) else Color(0xFF344054)
                            )
                            item.timestampLabel?.let {
                                Text(
                                    it,
                                    style = MaterialTheme.typography.labelSmall,
                                    color = Color(0xFF667085)
                                )
                            }
                        }
                    } else {
                        item.timestampLabel?.let {
                            Text(
                                it,
                                style = MaterialTheme.typography.labelSmall,
                                color = Color(0xFF667085)
                            )
                        }
                    }
                    MarkdownRenderer(
                        markdown = item.markdown.ifBlank { "(empty response)" },
                        modifier = Modifier.fillMaxWidth()
                    )
                    if (messageApprovals.isNotEmpty()) {
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(
                                "Approval required",
                                style = MaterialTheme.typography.labelMedium,
                                color = Color(0xFF667085)
                            )
                            messageApprovals.forEach { approval ->
                                InlineApprovalCard(
                                    approval = approval,
                                    approvalJson = approvalJson,
                                    selectedFields = approvalEdits[approval.id].orEmpty(),
                                    onEditChange = onEditChange,
                                    onDecision = onDecision
                                )
                            }
                        }
                    }
                    if (messageArtifacts.isNotEmpty()) {
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(
                                "Artifacts from this response",
                                style = MaterialTheme.typography.labelMedium,
                                color = Color(0xFF667085)
                            )
                            Row(
                                modifier = Modifier.horizontalScroll(rememberScrollState()),
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                messageArtifacts.forEach { file ->
                                    OutlinedButton(onClick = { onOpenFile(file) }) {
                                        Text(file.filename ?: file.id.take(12))
                                    }
                                }
                            }
                            inlinePreview?.let { preview ->
                                InlineArtifactPreviewSection(
                                    preview = preview,
                                    onClose = onClosePreview
                                )
                            }
                        }
                    }
                }
            }
        }
        Spacer(modifier = Modifier.height(if (startsGroup) 10.dp else 4.dp))
    }
}

@Composable
private fun InlineArtifactPreviewSection(
    preview: ArtifactPreview,
    onClose: () -> Unit
) {
    Surface(
        color = MaterialTheme.colorScheme.background,
        shape = MaterialTheme.shapes.medium,
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text("Inline Preview", style = MaterialTheme.typography.labelLarge)
                    Text(
                        preview.name,
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                }
                TextButton(onClick = onClose) {
                    Text("Close")
                }
            }
            if (preview.text != null) {
                Surface(
                    color = MaterialTheme.colorScheme.surfaceVariant,
                    shape = MaterialTheme.shapes.medium,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text(
                        preview.text,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.padding(12.dp)
                    )
                }
            } else {
                Text(
                    "Binary artifact downloaded. Inline preview is available for text-based outputs only.",
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFF667085)
                )
            }
        }
    }
}

@Composable
private fun InlineApprovalCard(
    approval: PendingToolApproval,
    approvalJson: Json,
    selectedFields: Map<String, JsonElement>,
    onEditChange: (String, String, JsonElement) -> Unit,
    onDecision: (PendingToolApproval, String) -> Unit
) {
    ApprovalCardContent(
        approval = approval,
        meta = approval.metadata?.let {
            runCatching { approvalJson.decodeFromJsonElement<com.viant.agentlysdk.ApprovalMeta>(it) }.getOrNull()
        },
        selectedFields = selectedFields,
        onEditChange = onEditChange,
        onDecision = onDecision,
        containerColor = Color(0xFFFFF7E8)
    )
}

@Composable
private fun ApprovalCardContent(
    approval: PendingToolApproval,
    meta: com.viant.agentlysdk.ApprovalMeta?,
    selectedFields: Map<String, JsonElement>,
    onEditChange: (String, String, JsonElement) -> Unit,
    onDecision: (PendingToolApproval, String) -> Unit,
    containerColor: Color
) {
    Surface(
        color = containerColor,
        shape = MaterialTheme.shapes.medium,
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Text(
                meta?.title ?: approval.title ?: approval.toolName,
                style = MaterialTheme.typography.titleSmall
            )
            Text(
                meta?.message ?: "Tool ${approval.toolName} is waiting for approval.",
                style = MaterialTheme.typography.bodySmall,
                color = Color(0xFF667085)
            )
            if (!meta?.editors.isNullOrEmpty()) {
                meta?.editors?.forEach { editor ->
                    if (editor.options.isNotEmpty()) {
                        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                            Text(
                                editor.label ?: editor.name,
                                style = MaterialTheme.typography.labelMedium
                            )
                            Row(
                                modifier = Modifier.horizontalScroll(rememberScrollState()),
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                editor.options.forEach { option ->
                                    val optionValue = option.item ?: JsonPrimitive(option.id)
                                    val selected = selectedFields[editor.name] == optionValue ||
                                        (selectedFields[editor.name] == null && option.selected)
                                    FilterChip(
                                        selected = selected,
                                        onClick = { onEditChange(approval.id, editor.name, optionValue) },
                                        label = { Text(option.label) }
                                    )
                                }
                            }
                            editor.description?.takeIf { it.isNotBlank() }?.let {
                                Text(
                                    it,
                                    style = MaterialTheme.typography.bodySmall,
                                    color = Color(0xFF667085)
                                )
                            }
                        }
                    }
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = { onDecision(approval, "approve") }) {
                    Text(meta?.acceptLabel ?: "Approve")
                }
                OutlinedButton(onClick = { onDecision(approval, "reject") }) {
                    Text(meta?.rejectLabel ?: "Reject")
                }
            }
        }
    }
}

private fun latestAssistantMarkdown(snapshot: ConversationStreamSnapshot): String? {
    val latest = snapshot.bufferedMessages
        .asReversed()
        .firstOrNull { message ->
            message.role.equals("assistant", ignoreCase = true) &&
                (!message.content.isNullOrBlank() || !message.preamble.isNullOrBlank())
        }
        ?: return null
    return combineAssistantMarkdown(latest)
}

private fun combineAssistantMarkdown(message: BufferedMessage): String? {
    val preamble = message.preamble?.trim().orEmpty()
    val content = message.content?.trim().orEmpty()
    return when {
        preamble.isNotEmpty() && content.isNotEmpty() -> "$preamble\n\n$content"
        content.isNotEmpty() -> content
        preamble.isNotEmpty() -> preamble
        else -> null
    }
}

private fun syncAssistantTranscript(
    transcript: MutableList<ChatEntry>,
    snapshot: ConversationStreamSnapshot
) {
    val assistantMessages = snapshot.bufferedMessages
        .filter { it.role.equals("assistant", ignoreCase = true) }
        .sortedBy { it.createdAt ?: it.id }

    assistantMessages.forEach { message ->
        val markdown = combineAssistantMarkdown(message) ?: return@forEach
        val existingIndex = transcript.indexOfFirst { it.id == message.id }
        val updated = ChatEntry(
            id = message.id,
            role = "assistant",
            markdown = markdown,
            streaming = snapshot.activeTurnId != null && message.turnId == snapshot.activeTurnId,
            timestampLabel = formatTimestampLabel(message.createdAt)
        )
        if (existingIndex >= 0) {
            transcript[existingIndex] = updated
        } else {
            transcript.add(updated)
        }
    }
}

private fun syncAssistantResult(
    transcript: MutableList<ChatEntry>,
    messageId: String?,
    markdown: String
) {
    if (markdown.isBlank()) return
    val id = messageId?.takeIf { it.isNotBlank() } ?: "assistant-final-${System.currentTimeMillis()}"
    val existingIndex = transcript.indexOfFirst { it.id == id }
    val updated = ChatEntry(
        id = id,
        role = "assistant",
        markdown = markdown,
        streaming = false,
        timestampLabel = formatTimestampLabel(System.currentTimeMillis())
    )
    if (existingIndex >= 0) {
        transcript[existingIndex] = updated
    } else {
        transcript.add(updated)
    }
}

private fun transcriptFromState(state: ConversationStateResponse): List<ChatEntry> {
    val entries = mutableListOf<ChatEntry>()
    state.conversation?.turns?.forEach { turn ->
        val user = turn.user
        user?.content?.takeIf { it.isNotBlank() }?.let { content ->
            entries.add(
                ChatEntry(
                    id = user.messageId,
                    role = "user",
                    markdown = content,
                    timestampLabel = formatTimestampLabel(turn.createdAt)
                )
            )
        }
        val assistantId = turn.assistant?.final?.messageId ?: turn.assistant?.preamble?.messageId
        val assistantContent = buildString {
            val preamble = turn.assistant?.preamble?.content?.trim().orEmpty()
            val final = turn.assistant?.final?.content?.trim().orEmpty()
            if (preamble.isNotEmpty()) {
                append(preamble)
            }
            if (final.isNotEmpty()) {
                if (isNotEmpty()) append("\n\n")
                append(final)
            }
        }.trim()
        if (!assistantId.isNullOrBlank() && assistantContent.isNotBlank()) {
            entries.add(
                ChatEntry(
                    id = assistantId,
                    role = "assistant",
                    markdown = assistantContent,
                    streaming = false,
                    timestampLabel = formatTimestampLabel(turn.createdAt)
                )
            )
        }
    }
    return entries
}

private data class ChatEntry(
    val id: String,
    val role: String,
    val markdown: String,
    val streaming: Boolean = false,
    val timestampLabel: String? = null
)

private data class ArtifactPreview(
    val artifactId: String,
    val name: String,
    val contentType: String?,
    val text: String?,
    val sizeBytes: Int
)

private enum class AppScreen {
    Chat,
    History
}

private fun isPreviewableText(contentType: String?, name: String?): Boolean {
    val normalizedType = contentType?.lowercase().orEmpty()
    val normalizedName = name?.lowercase().orEmpty()
    return normalizedType.startsWith("text/") ||
        normalizedType.contains("json") ||
        normalizedType.contains("xml") ||
        normalizedType.contains("javascript") ||
        normalizedName.endsWith(".md") ||
        normalizedName.endsWith(".txt") ||
        normalizedName.endsWith(".json") ||
        normalizedName.endsWith(".yaml") ||
        normalizedName.endsWith(".yml") ||
        normalizedName.endsWith(".xml") ||
        normalizedName.endsWith(".csv")
}

private fun formatTimestampLabel(value: Long?): String? {
    if (value == null || value <= 0) return null
    return SimpleDateFormat("h:mm a", Locale.US).format(Date(value))
}

private fun formatTimestampLabel(value: String?): String? {
    val raw = value?.trim().orEmpty()
    if (raw.isBlank()) return null
    raw.toLongOrNull()?.let { return formatTimestampLabel(it) }
    return runCatching {
        val instant = OffsetDateTime.parse(raw).toInstant()
        formatTimestampLabel(instant.toEpochMilli())
    }.getOrNull()
}
