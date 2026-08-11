package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.InputChip
import androidx.compose.material3.InputChipDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CameraAlt
import androidx.compose.material.icons.outlined.ChatBubbleOutline
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.Image
import androidx.compose.material.icons.outlined.KeyboardHide
import androidx.compose.material.icons.outlined.Mic
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.unit.dp

internal val ComposerInputFill = Color(0xFFF0FAF1)
internal val ComposerInputBorder = Color(0xFFB9DBBD)

@Composable
internal fun PhoneComposerDock(
    loading: Boolean,
    activeConversationId: String?,
    agentLabel: String?,
    query: String,
    onQueryChange: (String) -> Unit,
    composerAttachments: List<ComposerAttachmentDraft>,
    canCapturePhoto: Boolean,
    canUseVoiceInput: Boolean,
    onAddPhoto: () -> Unit,
    onTakePhoto: () -> Unit,
    onVoiceInput: () -> Unit,
    onRemoveAttachment: (String) -> Unit,
    lookupOccurrences: List<ComposerLookupOccurrence> = emptyList(),
    lookupSelections: Map<String, ComposerLookupSelection> = emptyMap(),
    onLookupClick: (ComposerLookupOccurrence) -> Unit = {},
    onOpenSettings: () -> Unit,
    onRunQuery: () -> Unit,
    onMeasuredHeight: (androidx.compose.ui.unit.Dp) -> Unit = {}
) {
    val hasActiveConversation = !activeConversationId.isNullOrBlank()
    val compactConversationDock = true
    var composerExpanded by remember(activeConversationId) {
        mutableStateOf(query.isNotBlank() || composerAttachments.isNotEmpty())
    }
    LaunchedEffect(query, composerAttachments.size) {
        if (query.isNotBlank() || composerAttachments.isNotEmpty()) {
            composerExpanded = true
        }
    }
    val density = LocalDensity.current
    val focusManager = LocalFocusManager.current
    val keyboardController = LocalSoftwareKeyboardController.current
    fun hideKeyboard() {
        focusManager.clearFocus(force = true)
        keyboardController?.hide()
    }
    val unresolvedRequiredLookup = firstUnresolvedRequiredComposerLookup(
        lookupOccurrences,
        lookupSelections
    )
    val sendLabel = composerSendButtonLabel(unresolvedRequiredLookup)
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .imePadding()
            .navigationBarsPadding()
            .onGloballyPositioned { coordinates ->
                onMeasuredHeight(with(density) { coordinates.size.height.toDp() })
            },
        color = Color(0xFFFDFDFE),
        border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
        shape = RoundedCornerShape(topStart = if (compactConversationDock) 24.dp else 28.dp, topEnd = if (compactConversationDock) 24.dp else 28.dp),
        tonalElevation = 2.dp,
        shadowElevation = if (compactConversationDock) 6.dp else 10.dp
    ) {
        Column(
            modifier = Modifier
                .padding(
                    horizontal = if (compactConversationDock) 14.dp else 16.dp,
                    vertical = if (compactConversationDock) 10.dp else 14.dp
                ),
            verticalArrangement = Arrangement.spacedBy(if (compactConversationDock) 8.dp else 12.dp)
        ) {
            if (!compactConversationDock) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Text(
                            "New message",
                            style = MaterialTheme.typography.titleMedium
                        )
                        Text(
                            "Start a fresh conversation",
                            style = MaterialTheme.typography.bodySmall,
                            color = Color(0xFF667085)
                        )
                    }
                    agentLabel?.takeIf { it.isNotBlank() }?.let {
                        AssistChip(onClick = {}, enabled = false, label = { Text(it) })
                    }
                    IconButton(onClick = onOpenSettings) {
                        Icon(Icons.Outlined.Settings, contentDescription = "Settings")
                    }
                }
            }
            if (compactConversationDock && !composerExpanded) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    FilledTonalIconButton(onClick = { composerExpanded = true }) {
                        Icon(Icons.Outlined.ChatBubbleOutline, contentDescription = "Write a reply")
                    }
                    Surface(
                        onClick = { composerExpanded = true },
                        color = ComposerInputFill,
                        border = BorderStroke(1.dp, ComposerInputBorder),
                        shape = RoundedCornerShape(20.dp),
                        modifier = Modifier.weight(1f)
                    ) {
                        Text(
                            if (hasActiveConversation) "Reply in the workspace" else "Start a conversation",
                            modifier = Modifier.padding(horizontal = 14.dp, vertical = 12.dp),
                            style = MaterialTheme.typography.bodyMedium,
                            color = Color(0xFF667085)
                        )
                    }
                    if (canUseVoiceInput) {
                        CompactComposerIconButton(
                            contentDescription = "Voice input",
                            icon = { Icon(Icons.Outlined.Mic, contentDescription = null) },
                            onClick = {
                                composerExpanded = true
                                onVoiceInput()
                            }
                        )
                    }
                }
            } else if (compactConversationDock) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    OutlinedTextField(
                        value = query,
                        onValueChange = onQueryChange,
                        placeholder = {
                            Text(if (hasActiveConversation) "Reply in the workspace" else "Ask anything")
                        },
                        modifier = Modifier.weight(1f),
                        minLines = 1,
                        // Starter prompts are often intentionally verbose. Keep the
                        // lookup action and media controls reachable above the IME;
                        // the text field remains scrollable for editing the full prompt.
                        maxLines = if (lookupOccurrences.isNotEmpty()) {
                            3
                        } else {
                            composerInputMaxLines(compactConversationDock, query)
                        },
                        keyboardOptions = KeyboardOptions(
                            capitalization = KeyboardCapitalization.None,
                            imeAction = ImeAction.Done
                        ),
                        keyboardActions = KeyboardActions(onDone = { hideKeyboard() }),
                        shape = RoundedCornerShape(20.dp),
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
                        enabled = !loading && (query.isNotBlank() || composerAttachments.isNotEmpty())
                    ) {
                        Text(sendLabel)
                    }
                }
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(12.dp)
                ) {
                    CompactComposerIconButton(
                        contentDescription = "Collapse composer",
                        icon = { Icon(Icons.Outlined.ExpandMore, contentDescription = null) },
                        onClick = {
                            hideKeyboard()
                            composerExpanded = false
                        }
                    )
                    CompactComposerIconButton(
                        contentDescription = "Add photo",
                        icon = { Icon(Icons.Outlined.Image, contentDescription = "Add photo") },
                        onClick = onAddPhoto
                    )
                    if (canCapturePhoto) {
                        CompactComposerIconButton(
                            contentDescription = "Take photo",
                            icon = { Icon(Icons.Outlined.CameraAlt, contentDescription = "Take photo") },
                            onClick = onTakePhoto
                        )
                    }
                    if (canUseVoiceInput) {
                        CompactComposerIconButton(
                            contentDescription = "Voice input",
                            icon = { Icon(Icons.Outlined.Mic, contentDescription = "Voice input") },
                            onClick = onVoiceInput
                        )
                    }
                }
            } else {
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
                    ComposerActionButton(
                        label = "Hide",
                        icon = { Icon(Icons.Outlined.KeyboardHide, contentDescription = "Hide keyboard") },
                        onClick = ::hideKeyboard
                    )
                }
            }
            if (composerAttachments.isNotEmpty()) {
                AttachmentChipsRow(
                    attachments = composerAttachments,
                    onRemoveAttachment = onRemoveAttachment
                )
            }
            if (lookupOccurrences.isNotEmpty()) {
                ComposerLookupChipsRow(
                    occurrences = lookupOccurrences,
                    selections = lookupSelections,
                    onLookupClick = onLookupClick
                )
            }
            if (!compactConversationDock) {
                OutlinedTextField(
                    value = query,
                    onValueChange = onQueryChange,
                    placeholder = { Text("Ask anything") },
                    modifier = Modifier.fillMaxWidth(),
                    minLines = 3,
                    maxLines = composerInputMaxLines(compactConversationDock, query),
                    keyboardOptions = KeyboardOptions(
                        capitalization = KeyboardCapitalization.None,
                        imeAction = ImeAction.Done
                    ),
                    keyboardActions = KeyboardActions(onDone = { hideKeyboard() }),
                    shape = RoundedCornerShape(22.dp),
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedContainerColor = ComposerInputFill,
                        unfocusedContainerColor = ComposerInputFill,
                        disabledContainerColor = ComposerInputFill,
                        focusedBorderColor = ComposerInputBorder,
                        unfocusedBorderColor = ComposerInputBorder,
                        disabledBorderColor = ComposerInputBorder.copy(alpha = 0.6f)
                    )
                )
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text(
                        "A new conversation will be created",
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085),
                        modifier = Modifier.weight(1f)
                    )
                    Button(
                        onClick = onRunQuery,
                        enabled = !loading && (query.isNotBlank() || composerAttachments.isNotEmpty())
                    ) {
                        Text(sendLabel)
                    }
                }
            }
        }
    }
}

internal fun composerInputMaxLines(compactConversationDock: Boolean, query: String): Int {
    if (!compactConversationDock) {
        return 6
    }
    val explicitLines = query.lineSequence().count().coerceAtLeast(1)
    val wrappedLines = ((query.trim().length + 23) / 24).coerceAtLeast(1)
    return maxOf(explicitLines, wrappedLines).coerceIn(2, 6)
}

internal fun firstUnresolvedRequiredComposerLookup(
    occurrences: List<ComposerLookupOccurrence>,
    selections: Map<String, ComposerLookupSelection>
): ComposerLookupOccurrence? =
    occurrences.firstOrNull { occurrence ->
        occurrence.required && selections[occurrence.key] == null
    }

internal fun composerSendButtonLabel(
    @Suppress("UNUSED_PARAMETER") unresolvedRequiredLookup: ComposerLookupOccurrence?
): String =
    "Send"

internal fun composerLookupControlLabel(title: String, selection: ComposerLookupSelection?): String =
    selection?.let(::composerLookupSelectionLabel) ?: "Select $title"

internal fun composerLookupSelectionLabel(selection: ComposerLookupSelection): String {
    val label = selection.label.trim()
    val detail = selection.detail?.trim().orEmpty()
    return when {
        detail.isBlank() -> label
        label.isBlank() -> detail
        label.contains(detail, ignoreCase = true) -> label
        else -> "$detail · $label"
    }
}

@Composable
internal fun ComposerLookupChipsRow(
    occurrences: List<ComposerLookupOccurrence>,
    selections: Map<String, ComposerLookupSelection>,
    onLookupClick: (ComposerLookupOccurrence) -> Unit
) {
    Row(
        modifier = Modifier.horizontalScroll(rememberScrollState()),
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        occurrences.forEach { occurrence ->
            val selection = selections[occurrence.key]
            InputChip(
                selected = selection != null,
                onClick = { onLookupClick(occurrence) },
                modifier = Modifier.widthIn(max = 320.dp),
                colors = InputChipDefaults.inputChipColors(
                    selectedContainerColor = Color(0xFFEAF2FF),
                    selectedLabelColor = Color(0xFF1849A9)
                ),
                label = {
                    Text(
                        composerLookupControlLabel(occurrence.title, selection),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            )
        }
    }
}

@Composable
private fun CompactComposerIconButton(
    contentDescription: String,
    icon: @Composable () -> Unit,
    onClick: () -> Unit
) {
    FilledTonalIconButton(
        onClick = onClick,
        modifier = Modifier.semantics { this.contentDescription = contentDescription }
    ) {
        icon()
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
    title: String? = null,
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
                title?.takeIf { it.isNotBlank() }?.let {
                    Text(it, style = MaterialTheme.typography.titleMedium)
                }
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
                ComposerHeaderActionIconButton(
                    contentDescription = "Add photo",
                    icon = { Icon(Icons.Outlined.Image, contentDescription = null) },
                    onClick = onAddPhoto
                )
                if (canCapturePhoto) {
                    ComposerHeaderActionIconButton(
                        contentDescription = "Take photo",
                        icon = { Icon(Icons.Outlined.CameraAlt, contentDescription = null) },
                        onClick = onTakePhoto
                    )
                }
                if (canUseVoiceInput) {
                    ComposerHeaderActionIconButton(
                        contentDescription = "Voice input",
                        icon = { Icon(Icons.Outlined.Mic, contentDescription = null) },
                        onClick = onVoiceInput
                    )
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

@Composable
private fun ComposerHeaderActionIconButton(
    contentDescription: String,
    icon: @Composable () -> Unit,
    onClick: () -> Unit
) {
    FilledTonalIconButton(
        onClick = onClick,
        modifier = Modifier.semantics {
            this.contentDescription = contentDescription
        }
    ) {
        icon()
    }
}
