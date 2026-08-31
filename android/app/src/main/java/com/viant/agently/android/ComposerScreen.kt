package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
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
import androidx.compose.material.icons.outlined.ArrowUpward
import androidx.compose.material.icons.outlined.CameraAlt
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.Image
import androidx.compose.material.icons.outlined.KeyboardHide
import androidx.compose.material.icons.outlined.Mic
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.OffsetMapping
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.input.TransformedText
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.IntOffset
import kotlin.math.abs
import kotlin.math.roundToInt

internal val ComposerInputFill = Color.White
internal val ComposerInputBorder = Color(0xFFB9DBBD)

@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun PhoneComposerDock(
    loading: Boolean,
    activeConversationId: String?,
    agentLabel: String?,
    query: String,
    onQueryChange: (String) -> Unit,
    onClearComposer: () -> Unit,
    composerAttachments: List<ComposerAttachmentDraft>,
    canCapturePhoto: Boolean,
    canUseVoiceInput: Boolean,
    voiceInputState: ComposerVoiceInputState = ComposerVoiceInputState.Idle,
    composerCursorPosition: Int = query.length,
    onComposerCursorPositionChange: (Int) -> Unit = {},
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
    val compactConversationDock = true
    var composerExpanded by remember(activeConversationId) {
        mutableStateOf(query.isNotBlank() || composerAttachments.isNotEmpty())
    }
    LaunchedEffect(query, composerAttachments.size, voiceInputState.phase) {
        if (query.isNotBlank() || composerAttachments.isNotEmpty() ||
            voiceInputState.phase != ComposerVoicePhase.Idle
        ) {
            composerExpanded = true
        }
    }
    val density = LocalDensity.current
    val focusManager = LocalFocusManager.current
    val keyboardController = LocalSoftwareKeyboardController.current
    val composerFocusRequester = remember { FocusRequester() }
    fun hideKeyboard() {
        focusManager.clearFocus(force = true)
        keyboardController?.hide()
    }
    LaunchedEffect(composerExpanded) {
        if (composerExpanded) {
            kotlinx.coroutines.yield()
            composerFocusRequester.requestFocus()
            keyboardController?.show()
        }
    }
    val unresolvedRequiredLookup = firstUnresolvedRequiredComposerLookup(
        lookupOccurrences,
        lookupSelections
    )
    val sendLabel = composerSendButtonLabel(unresolvedRequiredLookup)
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            // The activity already resizes for the IME. Applying imePadding here
            // subtracts the keyboard twice and pushes the composer up the screen;
            // retain only the system navigation inset for edge-to-edge layouts.
            .navigationBarsPadding()
            .onGloballyPositioned { coordinates ->
                onMeasuredHeight(with(density) { coordinates.size.height.toDp() })
            },
        color = Color(0xFFFDFDFE),
        border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
        shape = RoundedCornerShape(if (compactConversationDock) 24.dp else 28.dp),
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
                    Surface(
                        onClick = {
                            focusManager.clearFocus(force = true)
                            composerExpanded = true
                        },
                        color = ComposerInputFill,
                        border = BorderStroke(1.dp, ComposerInputBorder),
                        shape = RoundedCornerShape(20.dp),
                        modifier = Modifier.weight(1f)
                    ) {
                        Text(
                            "Type your message…",
                            modifier = Modifier.padding(horizontal = 14.dp, vertical = 12.dp),
                            style = MaterialTheme.typography.bodyMedium,
                            color = Color(0xFF667085)
                        )
                    }
                    if (canUseVoiceInput) {
                        PhoneToolbarAction(
                            icon = Icons.Outlined.Mic,
                            contentDescription = "Voice input",
                            accent = Color(0xFFDF5B78),
                            onClick = {
                                composerExpanded = true
                                onVoiceInput()
                            }
                        )
                    }
                }
            } else if (compactConversationDock) {
                if (voiceInputState.phase != ComposerVoicePhase.Idle) {
                    ComposerVoiceStatus(
                        state = voiceInputState,
                        onStop = {
                            if (voiceInputState.phase == ComposerVoicePhase.Listening ||
                                voiceInputState.phase == ComposerVoicePhase.Processing
                            ) {
                                onVoiceInput()
                            }
                        }
                    )
                }
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Surface(
                        modifier = Modifier.weight(1f),
                        color = ComposerInputFill,
                        border = BorderStroke(1.dp, ComposerInputBorder),
                        shape = RoundedCornerShape(20.dp)
                    ) {
                        Column(
                            modifier = Modifier.padding(6.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            InlineLookupPromptField(
                                value = query,
                                onValueChange = onQueryChange,
                                selectionPosition = composerCursorPosition,
                                onSelectionChange = onComposerCursorPositionChange,
                                placeholder = "Type your message…",
                                occurrences = lookupOccurrences,
                                selections = lookupSelections,
                                onLookupClick = onLookupClick,
                                modifier = Modifier.fillMaxWidth().focusRequester(composerFocusRequester),
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
                                    imeAction = ImeAction.Default
                                ),
                                keyboardActions = KeyboardActions(onDone = { hideKeyboard() }),
                            )
                        }
                    }
                    PhoneToolbarAction(
                        icon = Icons.Outlined.ArrowUpward,
                        contentDescription = sendLabel,
                        onClick = {
                            hideKeyboard()
                            composerExpanded = false
                            onRunQuery()
                        },
                        accent = Color(0xFF383BD8),
                        enabled = composerSendEnabled(
                            loading = loading,
                            hasContent = query.isNotBlank() || composerAttachments.isNotEmpty(),
                            unresolvedRequiredLookup = unresolvedRequiredLookup,
                        ),
                        loading = loading
                    )
                }
                // Keep the full action row available above the IME. Hiding it
                // while typing made photo, camera, voice, clear, and collapse
                // disappear exactly when the user was composing a message.
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)
                ) {
                        if (query.isNotBlank() || composerAttachments.isNotEmpty() || lookupSelections.isNotEmpty()) {
                            CompactComposerIconButton(
                                contentDescription = "Clear composer",
                                accent = Color(0xFFBD4A59),
                                icon = { Icon(Icons.Outlined.Close, contentDescription = null) },
                                onClick = {
                                    hideKeyboard()
                                    onClearComposer()
                                }
                            )
                        }
                        CompactComposerIconButton(
                            contentDescription = "Collapse composer",
                            accent = Color(0xFF5C667A),
                            icon = { Icon(Icons.Outlined.ExpandMore, contentDescription = null) },
                            onClick = {
                                hideKeyboard()
                                composerExpanded = false
                            }
                        )
                        CompactComposerIconButton(
                            contentDescription = "Add photo",
                            accent = Color(0xFF1A73F0),
                            icon = { Icon(Icons.Outlined.Image, contentDescription = "Add photo") },
                            onClick = onAddPhoto
                        )
                        if (canCapturePhoto) {
                            CompactComposerIconButton(
                                contentDescription = "Take photo",
                                accent = Color(0xFFE08A1E),
                                icon = { Icon(Icons.Outlined.CameraAlt, contentDescription = "Take photo") },
                                onClick = onTakePhoto
                            )
                        }
                        if (canUseVoiceInput) {
                            CompactComposerIconButton(
                                contentDescription = if (
                                    voiceInputState.phase == ComposerVoicePhase.Listening ||
                                    voiceInputState.phase == ComposerVoicePhase.Processing
                                ) {
                                    "Stop voice input"
                                } else {
                                    "Voice input"
                                },
                                accent = Color(0xFFDF5B78),
                                icon = { Icon(Icons.Outlined.Mic, contentDescription = "Voice input") },
                                // This must use the same start path as the working
                                // collapsed red mic. The action row is only visible
                                // once the IME is down, so clearing focus again can
                                // race the recognition callback and lose insertion.
                                onClick = onVoiceInput
                            )
                        }
                }
            } else {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)
                ) {
                    if (query.isNotBlank() || composerAttachments.isNotEmpty() || lookupSelections.isNotEmpty()) {
                        CompactComposerIconButton(
                            contentDescription = "Clear composer",
                            accent = Color(0xFFBD4A59),
                            icon = { Icon(Icons.Outlined.Close, contentDescription = null) },
                            onClick = onClearComposer
                        )
                    }
                    CompactComposerIconButton(
                        contentDescription = "Add photo",
                        accent = Color(0xFF1A73F0),
                        icon = { Icon(Icons.Outlined.Image, contentDescription = "Add photo") },
                        onClick = onAddPhoto
                    )
                    if (canCapturePhoto) {
                        CompactComposerIconButton(
                            contentDescription = "Take photo",
                            accent = Color(0xFFE08A1E),
                            icon = { Icon(Icons.Outlined.CameraAlt, contentDescription = "Take photo") },
                            onClick = onTakePhoto
                        )
                    }
                    if (canUseVoiceInput) {
                        CompactComposerIconButton(
                            contentDescription = "Voice input",
                            accent = Color(0xFFDF5B78),
                            icon = { Icon(Icons.Outlined.Mic, contentDescription = "Voice input") },
                            onClick = onVoiceInput
                        )
                    }
                    CompactComposerIconButton(
                        contentDescription = "Hide keyboard",
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
            if (!compactConversationDock && lookupOccurrences.isNotEmpty()) {
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
                        imeAction = ImeAction.Default
                    ),
                    keyboardActions = KeyboardActions(onDone = { hideKeyboard() }),
                    visualTransformation = composerLookupVisualTransformation(lookupOccurrences),
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

@Composable
private fun ComposerVoiceStatus(
    state: ComposerVoiceInputState,
    onStop: () -> Unit
) {
    val active = state.phase == ComposerVoicePhase.Listening ||
        state.phase == ComposerVoicePhase.Processing
    val containerColor = if (state.phase == ComposerVoicePhase.Error) {
        Color(0xFFFFF4E5)
    } else {
        Color(0xFFEAF3FF)
    }
    val contentColor = if (state.phase == ComposerVoicePhase.Error) {
        Color(0xFF8A4B08)
    } else {
        Color(0xFF145DA0)
    }
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = containerColor,
        shape = RoundedCornerShape(16.dp),
        onClick = if (active) onStop else ({})
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 10.dp),
            horizontalArrangement = Arrangement.spacedBy(10.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Icon(
                Icons.Outlined.Mic,
                contentDescription = null,
                tint = contentColor,
                modifier = Modifier.size((20 + (state.level * 6f)).dp)
            )
            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(2.dp)
            ) {
                Text(
                    state.partialText.ifBlank { state.message },
                    style = MaterialTheme.typography.bodyMedium,
                    color = contentColor,
                    fontWeight = FontWeight.Medium
                )
                if (active) {
                    Text(
                        if (state.phase == ComposerVoicePhase.Processing) {
                            "Adding speech to your message"
                        } else {
                            "Speak naturally · tap to stop"
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = contentColor.copy(alpha = 0.78f)
                    )
                }
            }
        }
    }
}

private data class ComposerInlineLookupTarget(
    val occurrence: ComposerLookupOccurrence,
    val transformedRange: IntRange,
    val label: String
)

private data class ComposerInlineLookupDisplay(
    val source: String,
    val text: AnnotatedString,
    val offsetMapping: OffsetMapping,
    val targets: List<ComposerInlineLookupTarget>
)

@Composable
private fun InlineLookupPromptField(
    value: String,
    onValueChange: (String) -> Unit,
    selectionPosition: Int,
    onSelectionChange: (Int) -> Unit,
    placeholder: String,
    occurrences: List<ComposerLookupOccurrence>,
    selections: Map<String, ComposerLookupSelection>,
    onLookupClick: (ComposerLookupOccurrence) -> Unit,
    modifier: Modifier = Modifier,
    minLines: Int = 1,
    maxLines: Int = 6,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default,
    keyboardActions: KeyboardActions = KeyboardActions.Default
) {
    var editorValue by remember {
        mutableStateOf(
            TextFieldValue(
                text = value,
                selection = TextRange(selectionPosition.coerceIn(0, value.length))
            )
        )
    }
    LaunchedEffect(value, selectionPosition) {
        if (editorValue.text != value) {
            val requestedPosition = selectionPosition
                .takeIf { it in 1..value.length }
                ?: value.length
            editorValue = TextFieldValue(
                text = value,
                selection = TextRange(requestedPosition)
            )
            onSelectionChange(requestedPosition)
        }
    }
    val display = remember(value, occurrences, selections) {
        buildComposerInlineLookupDisplay(value, occurrences, selections)
    }
    var textLayout by remember(value, display.text) { mutableStateOf<TextLayoutResult?>(null) }
    val hitTargets = remember(textLayout, display.targets) {
        val layout = textLayout ?: return@remember emptyList()
        display.targets.flatMap { target ->
            composerInlineLookupRects(layout, target.transformedRange).map { rect -> target to rect }
        }
    }
    val density = LocalDensity.current
    Box(modifier = modifier.padding(horizontal = 8.dp, vertical = 6.dp)) {
        BasicTextField(
            value = editorValue,
            onValueChange = { updated ->
                editorValue = updated
                onSelectionChange(updated.selection.end)
                if (updated.text != value) {
                    onValueChange(updated.text)
                }
            },
            modifier = Modifier.fillMaxWidth(),
            textStyle = MaterialTheme.typography.bodyLarge.copy(color = Color(0xFF182230)),
            cursorBrush = SolidColor(MaterialTheme.colorScheme.primary),
            minLines = minLines,
            maxLines = maxLines,
            keyboardOptions = keyboardOptions,
            keyboardActions = keyboardActions,
            visualTransformation = ComposerInlineLookupVisualTransformation(display),
            onTextLayout = { textLayout = it },
            decorationBox = { innerTextField ->
                Box {
                    if (value.isEmpty()) {
                        Text(
                            placeholder,
                            style = MaterialTheme.typography.bodyLarge,
                            color = Color(0xFF667085)
                        )
                    }
                    innerTextField()
                }
            }
        )
        hitTargets.forEach { (target, rect) ->
            Box(
                modifier = Modifier
                    .offset {
                        IntOffset(
                            x = rect.left.roundToInt(),
                            y = rect.top.roundToInt()
                        )
                    }
                    .size(
                        width = with(density) { rect.width.toDp() },
                        height = with(density) { rect.height.toDp() }
                    )
                    .semantics { contentDescription = "Select ${target.occurrence.title}" }
                    .clickable { onLookupClick(target.occurrence) }
            )
        }
    }
}

private class ComposerInlineLookupVisualTransformation(
    private val display: ComposerInlineLookupDisplay
) : VisualTransformation {
    override fun filter(text: AnnotatedString): TransformedText =
        if (text.text == display.source) {
            TransformedText(display.text, display.offsetMapping)
        } else {
            TransformedText(text, OffsetMapping.Identity)
        }
}

private fun buildComposerInlineLookupDisplay(
    source: String,
    occurrences: List<ComposerLookupOccurrence>,
    selections: Map<String, ComposerLookupSelection>
): ComposerInlineLookupDisplay {
    if (source.isEmpty() || occurrences.isEmpty()) {
        return ComposerInlineLookupDisplay(
            source = source,
            text = AnnotatedString(source),
            offsetMapping = OffsetMapping.Identity,
            targets = emptyList()
        )
    }
    val builder = AnnotatedString.Builder()
    val originalToTransformed = IntArray(source.length + 1)
    val transformedToOriginal = mutableListOf(0)
    val targets = mutableListOf<ComposerInlineLookupTarget>()
    var sourceOffset = 0
    var transformedOffset = 0

    fun appendSourceCharacter(index: Int) {
        originalToTransformed[index] = transformedOffset
        builder.append(source[index])
        transformedOffset += 1
        transformedToOriginal.add(index + 1)
    }

    occurrences
        .sortedBy { it.displayRange.first }
        .forEach { occurrence ->
            val replacementRange = composerInlineLookupReplacementRange(source, occurrence)
            val start = replacementRange.first.coerceIn(sourceOffset, source.length)
            val endExclusive = (replacementRange.last + 1).coerceIn(start, source.length)
            while (sourceOffset < start) {
                appendSourceCharacter(sourceOffset)
                sourceOffset += 1
            }
            val replacementStart = transformedOffset
            val selection = selections[occurrence.key]
            val label = composerInlineLookupLabel(occurrence, selection)
            originalToTransformed[start] = replacementStart
            val background = if (selection == null) Color(0xFFDCEBFF) else Color(0xFFDDF4E5)
            val foreground = if (selection == null) Color(0xFF175CD3) else Color(0xFF087443)
            val styleStart = builder.length
            builder.append(label)
            builder.addStyle(
                SpanStyle(
                    color = foreground,
                    background = background,
                    fontWeight = FontWeight.SemiBold
                ),
                styleStart,
                builder.length
            )
            repeat(label.length) {
                transformedOffset += 1
                transformedToOriginal.add(start)
            }
            if (label.isNotEmpty()) {
                transformedToOriginal[transformedOffset] = endExclusive
                targets += ComposerInlineLookupTarget(
                    occurrence = occurrence,
                    transformedRange = replacementStart until transformedOffset,
                    label = label
                )
            }
            for (index in start + 1 until endExclusive) {
                originalToTransformed[index] = replacementStart
            }
            originalToTransformed[endExclusive] = transformedOffset
            sourceOffset = endExclusive
        }
    while (sourceOffset < source.length) {
        appendSourceCharacter(sourceOffset)
        sourceOffset += 1
    }
    originalToTransformed[source.length] = transformedOffset

    val offsetMapping = object : OffsetMapping {
        override fun originalToTransformed(offset: Int): Int =
            originalToTransformed[offset.coerceIn(0, source.length)]

        override fun transformedToOriginal(offset: Int): Int =
            transformedToOriginal[offset.coerceIn(0, transformedToOriginal.lastIndex)]
    }
    return ComposerInlineLookupDisplay(
        source = source,
        text = builder.toAnnotatedString(),
        offsetMapping = offsetMapping,
        targets = targets
    )
}

internal fun composerInlineLookupDisplayText(
    source: String,
    occurrences: List<ComposerLookupOccurrence>,
    selections: Map<String, ComposerLookupSelection> = emptyMap()
): String = buildComposerInlineLookupDisplay(source, occurrences, selections).text.text

internal fun composerInlineLookupLabel(
    occurrence: ComposerLookupOccurrence,
    selection: ComposerLookupSelection?
): String {
    val selectedLabel = selection?.let(::composerLookupSelectionLabel)?.trim().orEmpty()
    if (selectedLabel.isEmpty()) {
        return occurrence.title
    }
    val identifier = selectedLabel.substringBefore(" · ").trim()
    if (identifier != selectedLabel &&
        identifier.matches(Regex("""[A-Za-z0-9][A-Za-z0-9_./:-]*"""))
    ) {
        return identifier
    }
    return if (selectedLabel.length <= 24) selectedLabel else selectedLabel.take(23).trimEnd() + "…"
}

private fun composerInlineLookupReplacementRange(
    source: String,
    occurrence: ComposerLookupOccurrence
): IntRange {
    val tokenStart = occurrence.displayRange.first.coerceIn(0, source.length)
    val tokenEnd = (occurrence.displayRange.last + 1).coerceIn(tokenStart, source.length)
    val entityWords = setOf(
        occurrence.name.lowercase(),
        occurrence.title.lowercase()
    )

    var afterWhitespace = tokenEnd
    while (afterWhitespace < source.length && source[afterWhitespace].isWhitespace()) {
        afterWhitespace += 1
    }
    var followingWordEnd = afterWhitespace
    while (followingWordEnd < source.length &&
        (source[followingWordEnd].isLetterOrDigit() || source[followingWordEnd] in "_-")
    ) {
        followingWordEnd += 1
    }
    val followingWord = source.substring(afterWhitespace, followingWordEnd).lowercase()
    if (followingWord.isNotEmpty() && followingWord in entityWords) {
        return tokenStart until followingWordEnd
    }

    // Remove whitespace authored between a lookup and punctuation so the
    // inline representation reads naturally (for example, "Line.").
    if (afterWhitespace > tokenEnd &&
        afterWhitespace < source.length &&
        source[afterWhitespace] in ".,;:!?"
    ) {
        return tokenStart until afterWhitespace
    }
    return tokenStart until tokenEnd
}

private fun composerInlineLookupRects(
    layout: TextLayoutResult,
    transformedRange: IntRange
): List<Rect> {
    if (transformedRange.isEmpty() || layout.layoutInput.text.isEmpty()) {
        return emptyList()
    }
    val end = transformedRange.last.coerceAtMost(layout.layoutInput.text.lastIndex)
    if (transformedRange.first > end) {
        return emptyList()
    }
    val rows = mutableListOf<Rect>()
    for (offset in transformedRange.first..end) {
        val box = layout.getBoundingBox(offset)
        val current = rows.lastOrNull()
        if (current != null && abs(current.top - box.top) < 1f) {
            rows[rows.lastIndex] = Rect(
                left = minOf(current.left, box.left),
                top = minOf(current.top, box.top),
                right = maxOf(current.right, box.right),
                bottom = maxOf(current.bottom, box.bottom)
            )
        } else {
            rows += box
        }
    }
    return rows
}

internal fun composerLookupVisualTransformation(
    occurrences: List<ComposerLookupOccurrence>
): VisualTransformation =
    if (occurrences.isEmpty()) {
        VisualTransformation.None
    } else {
        ComposerLookupVisualTransformation(occurrences.map { it.displayRange })
    }

/**
 * Keeps authored lookup directives in the stored prompt while removing them
 * from the user-facing editor. The adjacent lookup control is the visible and
 * editable representation, matching the web composer's token model.
 */
internal class ComposerLookupVisualTransformation(
    private val authoredRanges: List<IntRange>
) : VisualTransformation {
    override fun filter(text: AnnotatedString): TransformedText {
        val source = text.text
        if (source.isEmpty() || authoredRanges.isEmpty()) {
            return TransformedText(text, OffsetMapping.Identity)
        }
        val hidden = BooleanArray(source.length)
        authoredRanges.forEach { authoredRange ->
            val start = authoredRange.first.coerceIn(0, source.length)
            val endExclusive = (authoredRange.last + 1).coerceIn(start, source.length)
            for (index in start until endExclusive) {
                hidden[index] = true
            }

            var trailing = endExclusive
            while (trailing < source.length && source[trailing].isWhitespace()) {
                hidden[trailing] = true
                trailing += 1
            }
            if (trailing < source.length && source[trailing] in ".,;:!?") {
                var leading = start - 1
                while (leading >= 0 && source[leading].isWhitespace()) {
                    hidden[leading] = true
                    leading -= 1
                }
            }
        }

        val displayed = StringBuilder(source.length)
        val originalToTransformed = IntArray(source.length + 1)
        val transformedToOriginal = mutableListOf(0)
        var transformedOffset = 0
        for (originalOffset in source.indices) {
            originalToTransformed[originalOffset] = transformedOffset
            if (hidden[originalOffset]) {
                transformedToOriginal[transformedOffset] = originalOffset + 1
            } else {
                displayed.append(source[originalOffset])
                transformedOffset += 1
                transformedToOriginal.add(originalOffset + 1)
            }
        }
        originalToTransformed[source.length] = transformedOffset

        val offsetMapping = object : OffsetMapping {
            override fun originalToTransformed(offset: Int): Int =
                originalToTransformed[offset.coerceIn(0, source.length)]

            override fun transformedToOriginal(offset: Int): Int =
                transformedToOriginal[offset.coerceIn(0, transformedToOriginal.lastIndex)]
        }
        return TransformedText(AnnotatedString(displayed.toString()), offsetMapping)
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

internal fun composerSendEnabled(
    loading: Boolean,
    hasContent: Boolean,
    unresolvedRequiredLookup: ComposerLookupOccurrence?,
): Boolean = !loading && hasContent && unresolvedRequiredLookup == null

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
    accent: Color = Color(0xFF5C667A),
    icon: @Composable () -> Unit,
    onClick: () -> Unit
) {
    FilledTonalIconButton(
        onClick = onClick,
        modifier = Modifier.semantics { this.contentDescription = contentDescription },
        colors = IconButtonDefaults.filledTonalIconButtonColors(
            containerColor = accent.copy(alpha = 0.12f),
            contentColor = accent
        )
    ) {
        icon()
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
