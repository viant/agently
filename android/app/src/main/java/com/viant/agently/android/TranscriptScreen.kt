package com.viant.agently.android

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.PendingToolApproval
import com.viant.forgeandroid.runtime.ForgeRuntime
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement

@Composable
internal fun RenderTranscript(
    items: List<ChatEntry>,
    pendingApprovals: List<PendingToolApproval>,
    generatedFiles: List<GeneratedFileEntry>,
    forgeRuntime: ForgeRuntime,
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
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                                item.deliveryState?.let { state ->
                                    Text(
                                        when (state) {
                                            "sending" -> "Sending..."
                                            "failed" -> "Send failed"
                                            else -> state
                                        },
                                        style = MaterialTheme.typography.labelSmall,
                                        color = if (state == "failed") Color(0xFFB42318) else Color(0xFF667085)
                                    )
                                }
                                item.timestampLabel?.let {
                                    Text(
                                        it,
                                        style = MaterialTheme.typography.labelSmall,
                                        color = Color(0xFF667085)
                                    )
                                }
                            }
                        }
                    } else {
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                            item.deliveryState?.let { state ->
                                Text(
                                    when (state) {
                                        "sending" -> "Sending..."
                                        "failed" -> "Send failed"
                                        else -> state
                                    },
                                    style = MaterialTheme.typography.labelSmall,
                                    color = if (state == "failed") Color(0xFFB42318) else Color(0xFF667085)
                                )
                            }
                            item.timestampLabel?.let {
                                Text(
                                    it,
                                    style = MaterialTheme.typography.labelSmall,
                                    color = Color(0xFF667085)
                                )
                            }
                        }
                    }
                    TranscriptMessageContent(
                        markdown = item.markdown.ifBlank { "(empty response)" },
                        forgeRuntime = forgeRuntime,
                        messageKey = item.id
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
                                    forgeRuntime = forgeRuntime,
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
