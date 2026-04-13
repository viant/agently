package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CameraAlt
import androidx.compose.material.icons.outlined.Image
import androidx.compose.material.icons.outlined.Mic
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.unit.dp

@Composable
internal fun PhoneComposerDock(
    loading: Boolean,
    activeConversationId: String?,
    query: String,
    onQueryChange: (String) -> Unit,
    composerAttachments: List<ComposerAttachmentDraft>,
    canCapturePhoto: Boolean,
    canUseVoiceInput: Boolean,
    onAddPhoto: () -> Unit,
    onTakePhoto: () -> Unit,
    onVoiceInput: () -> Unit,
    onRemoveAttachment: (String) -> Unit,
    onOpenSettings: () -> Unit,
    onRunQuery: () -> Unit
) {
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .navigationBarsPadding(),
        color = Color(0xFFFDFDFE),
        border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
        shape = RoundedCornerShape(topStart = 28.dp, topEnd = 28.dp),
        tonalElevation = 2.dp,
        shadowElevation = 10.dp
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 14.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text(
                        if (!activeConversationId.isNullOrBlank()) "Message" else "New message",
                        style = MaterialTheme.typography.titleMedium
                    )
                    Text(
                        if (!activeConversationId.isNullOrBlank()) "Reply in the current chat"
                        else "Start a fresh conversation",
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
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                ComposerActionButton(
                    label = "Photo",
                    icon = { Icon(Icons.Outlined.Image, contentDescription = "Add photo") },
                    onClick = onAddPhoto
                )
                if (canCapturePhoto) {
                    ComposerActionButton(
                        label = "Camera",
                        icon = { Icon(Icons.Outlined.CameraAlt, contentDescription = "Take photo") },
                        onClick = onTakePhoto
                    )
                }
                if (canUseVoiceInput) {
                    ComposerActionButton(
                        label = "Voice",
                        icon = { Icon(Icons.Outlined.Mic, contentDescription = "Voice input") },
                        onClick = onVoiceInput
                    )
                }
            }
            if (composerAttachments.isNotEmpty()) {
                AttachmentChipsRow(
                    attachments = composerAttachments,
                    onRemoveAttachment = onRemoveAttachment
                )
            }
            OutlinedTextField(
                value = query,
                onValueChange = onQueryChange,
                placeholder = { Text("Ask anything") },
                modifier = Modifier.fillMaxWidth(),
                minLines = 3,
                maxLines = 6,
                shape = RoundedCornerShape(22.dp)
            )
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    if (!activeConversationId.isNullOrBlank()) "Continuing current conversation" else "A new conversation will be created",
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFF667085),
                    modifier = Modifier.weight(1f)
                )
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

@Composable
private fun ComposerActionButton(
    label: String,
    icon: @Composable () -> Unit,
    onClick: () -> Unit
) {
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        FilledTonalIconButton(onClick = onClick) {
            icon()
        }
        Text(
            text = label,
            style = MaterialTheme.typography.labelSmall,
            color = Color(0xFF667085)
        )
    }
}

@Composable
private fun AttachmentChipsRow(
    attachments: List<ComposerAttachmentDraft>,
    onRemoveAttachment: (String) -> Unit
) {
    Row(
        modifier = Modifier.horizontalScroll(rememberScrollState()),
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        attachments.forEach { attachment ->
            Surface(
                color = MaterialTheme.colorScheme.surfaceVariant,
                shape = MaterialTheme.shapes.large
            ) {
                Row(
                    modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Text(
                            attachment.name,
                            style = MaterialTheme.typography.labelLarge,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                        Text(
                            "${attachment.source} · ${formatSizeLabel(attachment.bytes.size.toLong())}",
                            style = MaterialTheme.typography.labelSmall,
                            color = Color(0xFF667085)
                        )
                    }
                    TextButton(onClick = { onRemoveAttachment(attachment.id) }) {
                        Text("Remove")
                    }
                }
            }
        }
    }
}

@Composable
internal fun ComposerHeader(
    title: String,
    attachments: List<ComposerAttachmentDraft>,
    canCapturePhoto: Boolean,
    canUseVoiceInput: Boolean,
    agentLabel: String? = null,
    subtitle: String? = null,
    onAddPhoto: () -> Unit,
    onTakePhoto: () -> Unit,
    onVoiceInput: () -> Unit,
    onRemoveAttachment: (String) -> Unit
) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(title, style = MaterialTheme.typography.titleMedium)
                subtitle?.takeIf { it.isNotBlank() }?.let {
                    Text(
                        it,
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                }
            }
            Row(
                modifier = Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                agentLabel?.takeIf { it.isNotBlank() }?.let {
                    AssistChip(onClick = {}, enabled = false, label = { Text("Agent $it") })
                }
                AssistChip(onClick = onAddPhoto, label = { Text("Photo") })
                if (canCapturePhoto) {
                    AssistChip(onClick = onTakePhoto, label = { Text("Camera") })
                }
                if (canUseVoiceInput) {
                    AssistChip(onClick = onVoiceInput, label = { Text("Voice") })
                }
            }
        }
        if (attachments.isNotEmpty()) {
            Row(
                modifier = Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                attachments.forEach { attachment ->
                    Surface(
                        color = MaterialTheme.colorScheme.surfaceVariant,
                        shape = MaterialTheme.shapes.large
                    ) {
                        Row(
                            modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                                Text(
                                    attachment.name,
                                    style = MaterialTheme.typography.labelLarge,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                                Text(
                                    "${attachment.source} · ${formatSizeLabel(attachment.bytes.size.toLong())}",
                                    style = MaterialTheme.typography.labelSmall,
                                    color = Color(0xFF667085)
                                )
                            }
                            TextButton(onClick = { onRemoveAttachment(attachment.id) }) {
                                Text("Remove")
                            }
                        }
                    }
                }
            }
        }
    }
}
