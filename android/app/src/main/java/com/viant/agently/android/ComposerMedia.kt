package com.viant.agently.android

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.net.Uri
import android.os.Bundle
import android.provider.MediaStore
import android.provider.OpenableColumns
import android.speech.RecognitionListener
import android.speech.RecognizerIntent
import android.speech.SpeechRecognizer
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.ContextCompat
import java.io.ByteArrayOutputStream
import java.util.Locale
import java.util.UUID

internal data class ComposerAttachmentDraft(
    val id: String = UUID.randomUUID().toString(),
    val name: String,
    val mimeType: String,
    val bytes: ByteArray,
    val source: String
)

internal data class ComposerMediaController(
    val canCapturePhoto: Boolean,
    val canUseVoiceInput: Boolean,
    val voiceInputState: ComposerVoiceInputState,
    val composerCursorPosition: Int,
    val updateComposerCursorPosition: (Int) -> Unit,
    val launchPhotoPicker: () -> Unit,
    val launchCameraCapture: () -> Unit,
    val launchVoiceInput: () -> Unit,
    val removeAttachment: (String) -> Unit
)

internal enum class ComposerVoicePhase {
    Idle,
    Listening,
    Processing,
    Error
}

internal data class ComposerVoiceInputState(
    val phase: ComposerVoicePhase = ComposerVoicePhase.Idle,
    val message: String = "",
    val partialText: String = "",
    val level: Float = 0f
) {
    companion object {
        val Idle = ComposerVoiceInputState()
    }
}

@Composable
internal fun rememberComposerMediaController(
    attachments: List<ComposerAttachmentDraft>,
    onAttachmentsChange: (List<ComposerAttachmentDraft>) -> Unit,
    query: String,
    onQueryChange: (String) -> Unit,
    onError: (String) -> Unit
): ComposerMediaController {
    val context = LocalContext.current
    val packageManager = context.packageManager
    val latestQuery by rememberUpdatedState(query)
    val latestOnQueryChange by rememberUpdatedState(onQueryChange)
    var composerCursorPosition by remember { mutableIntStateOf(query.length) }
    val latestComposerCursorPosition by rememberUpdatedState(composerCursorPosition)
    val supportsCameraCapture = remember(packageManager) {
        packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY) ||
            Intent(MediaStore.ACTION_IMAGE_CAPTURE).resolveActivity(packageManager) != null
    }
    val supportsVoiceInput = remember(context) {
        SpeechRecognizer.isRecognitionAvailable(context)
    }
    val speechRecognizer = remember(context, supportsVoiceInput) {
        if (!supportsVoiceInput) {
            null
        } else {
            runCatching { SpeechRecognizer.createSpeechRecognizer(context) }.getOrNull()
        }
    }
    var voiceInputState by remember { mutableStateOf(ComposerVoiceInputState.Idle) }
    val latestVoiceInputState by rememberUpdatedState(voiceInputState)
    var voiceCancelRequested by remember { mutableStateOf(false) }
    val photoPickerRequest = remember {
        PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)
    }

    fun appendAttachment(draft: ComposerAttachmentDraft) {
        onAttachmentsChange(attachments + draft)
    }

    val imagePickerLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.PickVisualMedia()
    ) { uri ->
        if (uri == null) {
            return@rememberLauncherForActivityResult
        }
        runCatching {
            readAttachmentFromUri(context, uri, fallbackName = "image-${System.currentTimeMillis()}.jpg")
        }.onSuccess { draft ->
            appendAttachment(draft)
        }.onFailure { err ->
            onError("Unable to attach image: ${err.message ?: err}")
        }
    }
    val cameraLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.TakePicturePreview()
    ) { bitmap ->
        if (bitmap == null) {
            return@rememberLauncherForActivityResult
        }
        runCatching {
            bitmapToAttachment(bitmap)
        }.onSuccess { draft ->
            appendAttachment(draft)
        }.onFailure { err ->
            onError("Unable to capture photo: ${err.message ?: err}")
        }
    }
    fun startVoiceRecognition() {
        val recognizer = speechRecognizer
        if (recognizer == null) {
            voiceInputState = ComposerVoiceInputState(
                phase = ComposerVoicePhase.Error,
                message = "Voice input is unavailable on this device."
            )
            return
        }
        voiceCancelRequested = false
        voiceInputState = ComposerVoiceInputState(
            phase = ComposerVoicePhase.Listening,
            message = "Starting microphone…"
        )
        runCatching {
            recognizer.startListening(createSpeechRecognizerIntent())
        }.onFailure { err ->
            voiceInputState = ComposerVoiceInputState(
                phase = ComposerVoicePhase.Error,
                message = "Unable to start voice input: ${err.message ?: err}"
            )
        }
    }

    fun commitVoiceTranscript(speechText: String): Boolean {
        if (speechText.isBlank()) {
            return false
        }
        val insertion = insertVoiceTranscript(
            existingQuery = latestQuery,
            speechText = speechText,
            cursorPosition = latestComposerCursorPosition
        )
        composerCursorPosition = insertion.cursorPosition
        latestOnQueryChange(insertion.text)
        return true
    }

    DisposableEffect(speechRecognizer) {
        val listener = object : RecognitionListener {
            override fun onReadyForSpeech(params: Bundle?) {
                voiceInputState = ComposerVoiceInputState(
                    phase = ComposerVoicePhase.Listening,
                    message = "Listening…"
                )
            }

            override fun onBeginningOfSpeech() {
                voiceInputState = voiceInputState.copy(
                    phase = ComposerVoicePhase.Listening,
                    message = "Listening…"
                )
            }

            override fun onRmsChanged(rmsdB: Float) {
                if (voiceInputState.phase == ComposerVoicePhase.Listening) {
                    voiceInputState = voiceInputState.copy(level = ((rmsdB + 2f) / 12f).coerceIn(0f, 1f))
                }
            }

            override fun onBufferReceived(buffer: ByteArray?) = Unit

            override fun onEndOfSpeech() {
                voiceInputState = voiceInputState.copy(
                    phase = ComposerVoicePhase.Processing,
                    message = "Transcribing…",
                    level = 0f
                )
            }

            override fun onError(error: Int) {
                if (voiceCancelRequested) {
                    voiceCancelRequested = false
                    voiceInputState = ComposerVoiceInputState.Idle
                    return
                }
                if ((error == SpeechRecognizer.ERROR_NO_MATCH ||
                        error == SpeechRecognizer.ERROR_SPEECH_TIMEOUT) &&
                    commitVoiceTranscript(latestVoiceInputState.partialText)
                ) {
                    voiceInputState = ComposerVoiceInputState.Idle
                    return
                }
                voiceInputState = ComposerVoiceInputState(
                    phase = ComposerVoicePhase.Error,
                    message = voiceRecognitionErrorMessage(error)
                )
            }

            override fun onResults(results: Bundle?) {
                val speechText = speechRecognitionText(results)
                voiceInputState = ComposerVoiceInputState.Idle
                if (!commitVoiceTranscript(speechText)) {
                    voiceInputState = ComposerVoiceInputState(
                        phase = ComposerVoicePhase.Error,
                        message = "I didn't catch that. Tap the microphone to try again."
                    )
                }
            }

            override fun onPartialResults(partialResults: Bundle?) {
                val partialText = speechRecognitionText(partialResults)
                voiceInputState = voiceInputState.copy(
                    phase = ComposerVoicePhase.Listening,
                    message = if (partialText.isBlank()) "Listening…" else "Hearing",
                    partialText = partialText
                )
            }

            override fun onEvent(eventType: Int, params: Bundle?) = Unit
        }
        speechRecognizer?.setRecognitionListener(listener)
        onDispose {
            speechRecognizer?.cancel()
            speechRecognizer?.destroy()
        }
    }
    val cameraPermissionLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestPermission()
    ) { granted ->
        if (granted) {
            cameraLauncher.launch(null)
        } else {
            onError("Camera permission is required to take a photo.")
        }
    }
    val audioPermissionLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestPermission()
    ) { granted ->
        if (granted) {
            startVoiceRecognition()
        } else {
            voiceInputState = ComposerVoiceInputState(
                phase = ComposerVoicePhase.Error,
                message = "Microphone permission is required for voice input."
            )
            onError("Microphone permission is required for voice input.")
        }
    }

    return ComposerMediaController(
        canCapturePhoto = supportsCameraCapture,
        canUseVoiceInput = supportsVoiceInput && speechRecognizer != null,
        voiceInputState = voiceInputState,
        composerCursorPosition = composerCursorPosition,
        updateComposerCursorPosition = { position ->
            composerCursorPosition = position.coerceIn(0, query.length)
        },
        launchPhotoPicker = {
            imagePickerLauncher.launch(photoPickerRequest)
        },
        launchCameraCapture = launchCamera@{
            if (!supportsCameraCapture) {
                onError("Camera capture is unavailable on this device.")
                return@launchCamera
            }
            val hasCameraPermission =
                ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED
            if (hasCameraPermission) {
                cameraLauncher.launch(null)
            } else {
                cameraPermissionLauncher.launch(Manifest.permission.CAMERA)
            }
        },
        launchVoiceInput = launchVoice@{
            if (!supportsVoiceInput || speechRecognizer == null) {
                onError("Voice input is unavailable on this device.")
                return@launchVoice
            }
            if (voiceInputState.phase == ComposerVoicePhase.Listening ||
                voiceInputState.phase == ComposerVoicePhase.Processing
            ) {
                voiceCancelRequested = true
                speechRecognizer.cancel()
                voiceInputState = ComposerVoiceInputState.Idle
                return@launchVoice
            }
            val hasRecordAudioPermission =
                ContextCompat.checkSelfPermission(context, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED
            if (hasRecordAudioPermission) {
                startVoiceRecognition()
            } else {
                audioPermissionLauncher.launch(Manifest.permission.RECORD_AUDIO)
            }
        },
        removeAttachment = { attachmentId ->
            onAttachmentsChange(attachments.filterNot { it.id == attachmentId })
        }
    )
}

internal fun buildUserComposerMarkdown(
    prompt: String,
    attachments: List<ComposerAttachmentDraft>
): String {
    if (attachments.isEmpty()) {
        return prompt
    }
    val attachmentLines = attachments.joinToString("\n") { attachment ->
        "- ${attachment.source}: ${attachment.name} (${attachment.mimeType}, ${formatSizeLabel(attachment.bytes.size.toLong())})"
    }
    return buildString {
        append(prompt)
        append("\n\n")
        append("Attached:\n")
        append(attachmentLines)
    }
}

internal fun buildConversationTitle(
    prompt: String,
    attachments: List<ComposerAttachmentDraft>
): String {
    val trimmedPrompt = prompt.trim()
    if (attachments.isEmpty()) {
        return trimmedPrompt.take(80)
    }
    if (trimmedPrompt.isNotBlank() && trimmedPrompt != "Please analyze the attached file(s).") {
        return trimmedPrompt.take(80)
    }
    val attachmentSummary = attachments
        .take(2)
        .joinToString(", ") { it.name.ifBlank { it.source.lowercase() } }
        .take(80)
    val remainingCount = (attachments.size - 2).coerceAtLeast(0)
    return when {
        attachments.size == 1 && attachmentSummary.isNotBlank() -> attachmentSummary
        attachmentSummary.isNotBlank() && remainingCount > 0 -> "$attachmentSummary +$remainingCount more".take(80)
        attachmentSummary.isNotBlank() -> attachmentSummary
        else -> trimmedPrompt.take(80)
    }
}

internal fun createSpeechRecognizerIntent(): Intent {
    return Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH).apply {
        putExtra(
            RecognizerIntent.EXTRA_LANGUAGE_MODEL,
            RecognizerIntent.LANGUAGE_MODEL_FREE_FORM
        )
        putExtra(RecognizerIntent.EXTRA_PROMPT, "Speak your message")
        putExtra(RecognizerIntent.EXTRA_PARTIAL_RESULTS, true)
        putExtra(RecognizerIntent.EXTRA_MAX_RESULTS, 1)
    }
}

internal fun mergeVoiceTranscript(existingQuery: String, speechText: String): String =
    listOf(existingQuery.trim(), speechText.trim())
        .filter { it.isNotBlank() }
        .joinToString("\n")

internal data class VoiceTranscriptInsertion(
    val text: String,
    val cursorPosition: Int
)

internal fun insertVoiceTranscript(
    existingQuery: String,
    speechText: String,
    cursorPosition: Int
): VoiceTranscriptInsertion {
    val transcript = speechText.trim()
    if (transcript.isEmpty()) {
        return VoiceTranscriptInsertion(existingQuery, cursorPosition.coerceIn(0, existingQuery.length))
    }
    val cursor = cursorPosition.coerceIn(0, existingQuery.length)
    val prefix = existingQuery.substring(0, cursor)
    val suffix = existingQuery.substring(cursor)
    val leadingSeparator = if (prefix.isNotEmpty() && !prefix.last().isWhitespace()) " " else ""
    val trailingSeparator = if (suffix.isNotEmpty() && !suffix.first().isWhitespace()) " " else ""
    val inserted = leadingSeparator + transcript + trailingSeparator
    return VoiceTranscriptInsertion(
        text = prefix + inserted + suffix,
        cursorPosition = prefix.length + leadingSeparator.length + transcript.length
    )
}

internal fun speechRecognitionText(results: Bundle?): String =
    results
        ?.getStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION)
        ?.firstOrNull()
        ?.trim()
        .orEmpty()

internal fun voiceRecognitionErrorMessage(error: Int): String = when (error) {
    SpeechRecognizer.ERROR_NO_MATCH,
    SpeechRecognizer.ERROR_SPEECH_TIMEOUT -> "I didn't catch that. Tap the microphone to try again."
    SpeechRecognizer.ERROR_NETWORK,
    SpeechRecognizer.ERROR_NETWORK_TIMEOUT -> "Voice recognition needs a network connection."
    SpeechRecognizer.ERROR_AUDIO -> "The microphone could not be read. Tap to try again."
    SpeechRecognizer.ERROR_RECOGNIZER_BUSY -> "Voice recognition is busy. Wait a moment and try again."
    SpeechRecognizer.ERROR_INSUFFICIENT_PERMISSIONS -> "Microphone permission is required for voice input."
    else -> "Voice input stopped. Tap the microphone to try again."
}

internal fun formatSizeLabel(sizeBytes: Long): String {
    if (sizeBytes < 1024) {
        return "${sizeBytes} B"
    }
    val kib = sizeBytes / 1024.0
    if (kib < 1024) {
        return String.format(Locale.US, "%.1f KB", kib)
    }
    val mib = kib / 1024.0
    return String.format(Locale.US, "%.1f MB", mib)
}

private fun bitmapToAttachment(bitmap: Bitmap): ComposerAttachmentDraft {
    val output = ByteArrayOutputStream()
    check(bitmap.compress(Bitmap.CompressFormat.JPEG, 92, output)) {
        "Unable to encode captured image."
    }
    val timestamp = System.currentTimeMillis()
    return ComposerAttachmentDraft(
        name = "camera-$timestamp.jpg",
        mimeType = "image/jpeg",
        bytes = output.toByteArray(),
        source = "Camera"
    )
}

private fun readAttachmentFromUri(
    context: android.content.Context,
    uri: Uri,
    fallbackName: String
): ComposerAttachmentDraft {
    val resolver = context.contentResolver
    val mimeType = resolver.getType(uri)?.takeIf { it.isNotBlank() } ?: "application/octet-stream"
    val bytes = resolver.openInputStream(uri)?.use { it.readBytes() }
        ?: error("Unable to read selected file.")
    val displayName = resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)
        ?.use { cursor ->
            if (cursor.moveToFirst()) {
                val columnIndex = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                if (columnIndex >= 0) cursor.getString(columnIndex) else null
            } else {
                null
            }
        }
        ?: uri.lastPathSegment?.substringAfterLast('/')
        ?: fallbackName
    return ComposerAttachmentDraft(
        name = displayName,
        mimeType = mimeType,
        bytes = bytes,
        source = "Photo"
    )
}
