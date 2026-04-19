package com.viant.agently.android

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.GeneratedFileEntry

@Composable
internal fun ConversationArtifactsSection(
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
internal fun ArtifactPreviewSection(
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
                    Text("Preview", style = MaterialTheme.typography.titleMedium)
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
            ArtifactPreviewBody(preview = preview)
        }
    }
}

@Composable
internal fun InlineArtifactPreviewSection(
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
            ArtifactPreviewBody(preview = preview)
        }
    }
}

@Composable
private fun ArtifactPreviewBody(preview: ArtifactPreview) {
    if (preview.text != null) {
        Surface(
            color = MaterialTheme.colorScheme.surfaceVariant,
            shape = MaterialTheme.shapes.medium,
            modifier = Modifier.fillMaxWidth()
        ) {
            Text(
                preview.text,
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 220.dp)
                    .verticalScroll(rememberScrollState())
                    .padding(12.dp)
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
