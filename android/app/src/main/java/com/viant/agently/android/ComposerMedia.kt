package com.viant.agently.android

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.net.Uri
import android.provider.MediaStore
import android.provider.OpenableColumns
import android.speech.RecognizerIntent
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
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
    val launchPhotoPicker: () -> Unit,
    val launchCameraCapture: () -> Unit,
    val launchVoiceInput: () -> Unit,
    val removeAttachment: (String) -> Unit
)

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
    val supportsCameraCapture = remember(packageManager) {
        packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY) ||
            Intent(MediaStore.ACTION_IMAGE_CAPTURE).resolveActivity(packageManager) != null
    }
    val supportsVoiceInput = remember(packageManager) {
        Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH).resolveActivity(packageManager) != null
    }
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
    val speechLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.StartActivityForResult()
    ) { activityResult ->
        val speechText = activityResult.data
            ?.getStringArrayListExtra(RecognizerIntent.EXTRA_RESULTS)
            ?.firstOrNull()
            ?.trim()
            .orEmpty()
        if (speechText.isNotBlank()) {
            onQueryChange(listOf(query.trim(), speechText).filter { it.isNotBlank() }.joinToString("\n"))
        } else {
            onError("Voice input did not return any text. Speech services may be unavailable on this device.")
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
            runCatching {
                speechLauncher.launch(createSpeechRecognizerIntent())
            }.onFailure { err ->
                onError("Voice input is unavailable: ${err.message ?: err}")
            }
        } else {
            onError("Microphone permission is required for voice input.")
        }
    }

    return ComposerMediaController(
        canCapturePhoto = supportsCameraCapture,
        canUseVoiceInput = supportsVoiceInput,
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
            if (!supportsVoiceInput) {
                onError("Voice input is unavailable on this device.")
                return@launchVoice
            }
            val hasRecordAudioPermission =
                ContextCompat.checkSelfPermission(context, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED
            if (hasRecordAudioPermission) {
                runCatching {
                    speechLauncher.launch(createSpeechRecognizerIntent())
                }.onFailure { err ->
                    onError("Voice input is unavailable: ${err.message ?: err}")
                }
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
    }
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
