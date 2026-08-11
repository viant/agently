package com.viant.agently.android

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.ExecutionPageState
import com.viant.agentlysdk.ModelStepState
import com.viant.agentlysdk.ToolStepState
import kotlinx.coroutines.launch

@Composable
internal fun ExecutionInspectorSection(
    state: ConversationStateResponse?,
    client: AgentlyClient
) {
    val turn = state?.conversation?.turns
        ?.lastOrNull { it.execution?.pages?.isNotEmpty() == true }
        ?: return
    val pages = turn.execution?.pages.orEmpty()
    if (pages.isEmpty()) {
        return
    }
    val payloadTitles = remember(state) { state?.let(::executionPayloadTitles).orEmpty() }
    var payloadPreviews by remember(state) { mutableStateOf<Map<String, ArtifactPreview>>(emptyMap()) }
    var loadingPayloadId by remember(state) { mutableStateOf<String?>(null) }
    var payloadError by remember(state) { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    val loadPayload: (String) -> Unit = { payloadId ->
        if (payloadPreviews[payloadId] == null && loadingPayloadId != payloadId) {
            loadingPayloadId = payloadId
            payloadError = null
            scope.launch {
                runCatching {
                    val downloaded = client.downloadPayload(payloadId, 1L * 1024 * 1024)
                    buildPayloadPreview(
                        payloadId = payloadId,
                        title = payloadTitles[payloadId] ?: "Execution payload",
                        downloaded = downloaded,
                        maxInflatedBytes = 512 * 1024
                    )
                }.onSuccess { preview ->
                    // Retain only the two most recently inspected payloads.
                    payloadPreviews = (payloadPreviews + (payloadId to preview))
                        .entries
                        .toList()
                        .takeLast(2)
                        .associate { it.key to it.value }
                }.onFailure {
                    payloadError = it.message?.takeIf(String::isNotBlank) ?: "Unable to load payload preview."
                }
                loadingPayloadId = null
            }
        }
    }
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Text("Execution", style = MaterialTheme.typography.titleMedium)
            pages.forEachIndexed { index, page ->
                val payloads = payloadPreviewSources(page, payloadTitles)
                ExecutionPageCard(
                    title = "Page ${index + 1}",
                    subtitle = buildPageSubtitle(page),
                    payloads = payloads,
                    payloadPreviews = payloadPreviews,
                    loadingPayloadId = loadingPayloadId,
                    payloadError = payloadError,
                    onLoadPayload = loadPayload,
                    content = { onSelectPayload ->
                        page.modelSteps.forEachIndexed { modelIndex, step ->
                            ModelStepCard(
                                index = modelIndex + 1,
                                step = step,
                                onSelectPayload = onSelectPayload
                            )
                        }
                        page.toolSteps.forEachIndexed { toolIndex, step ->
                            ToolStepCard(
                                index = toolIndex + 1,
                                step = step,
                                onSelectPayload = onSelectPayload
                            )
                        }
                    }
                )
            }
        }
    }
}

@Composable
private fun ExecutionPageCard(
    title: String,
    subtitle: String?,
    payloads: List<PayloadPreviewSource>,
    payloadPreviews: Map<String, ArtifactPreview>,
    loadingPayloadId: String?,
    payloadError: String?,
    onLoadPayload: (String) -> Unit,
    content: @Composable ((String) -> Unit) -> Unit
) {
    var selectedPayloadId by remember(payloads.map { it.id }) {
        mutableStateOf<String?>(null)
    }
    val selectedPreview = selectedPayloadId?.let(payloadPreviews::get)
    Surface(
        color = MaterialTheme.colorScheme.surfaceVariant,
        shape = MaterialTheme.shapes.medium,
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Text(title, style = MaterialTheme.typography.labelLarge, fontWeight = FontWeight.SemiBold)
            subtitle?.takeIf { it.isNotBlank() }?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = Color(0xFF667085))
            }
            selectedPreview?.let { preview ->
                InlineArtifactPreviewSection(
                    preview = preview,
                    onClose = { selectedPayloadId = null }
                )
            }
            if (selectedPayloadId == loadingPayloadId) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator()
                    Text("Loading payload preview…", style = MaterialTheme.typography.bodySmall)
                }
            } else if (selectedPayloadId != null && selectedPreview == null && !payloadError.isNullOrBlank()) {
                Text(payloadError, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            }
            content { payloadId ->
                selectedPayloadId = payloadId
                onLoadPayload(payloadId)
            }
        }
    }
}

@Composable
private fun ModelStepCard(
    index: Int,
    step: ModelStepState,
    onSelectPayload: (String) -> Unit
) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            "Model step $index · ${step.model ?: step.provider ?: step.modelCallId.take(12)}",
            style = MaterialTheme.typography.labelMedium,
            fontWeight = FontWeight.Medium
        )
        PayloadChipRow(
            requestPayloadId = step.requestPayloadId,
            providerRequestPayloadId = step.providerRequestPayloadId,
            responsePayloadId = step.responsePayloadId,
            providerResponsePayloadId = step.providerResponsePayloadId,
            prefix = "llm.request",
            onSelectPayload = onSelectPayload
        )
    }
}

@Composable
private fun ToolStepCard(
    index: Int,
    step: ToolStepState,
    onSelectPayload: (String) -> Unit
) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            "Tool step $index · ${step.toolName}",
            style = MaterialTheme.typography.labelMedium,
            fontWeight = FontWeight.Medium
        )
        PayloadChipRow(
            requestPayloadId = step.requestPayloadId,
            providerRequestPayloadId = null,
            responsePayloadId = step.responsePayloadId,
            providerResponsePayloadId = null,
            prefix = "tool",
            onSelectPayload = onSelectPayload
        )
    }
}

@Composable
private fun PayloadChipRow(
    requestPayloadId: String?,
    providerRequestPayloadId: String?,
    responsePayloadId: String?,
    providerResponsePayloadId: String?,
    prefix: String,
    onSelectPayload: (String) -> Unit
) {
    Row(
        modifier = Modifier.horizontalScroll(rememberScrollState()),
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        payloadChip(requestPayloadId, "$prefix.request", onSelectPayload)
        payloadChip(providerRequestPayloadId, "$prefix.providerRequest", onSelectPayload)
        payloadChip(responsePayloadId, "$prefix.response", onSelectPayload)
        payloadChip(providerResponsePayloadId, "$prefix.providerResponse", onSelectPayload)
    }
}

@Composable
private fun payloadChip(
    payloadId: String?,
    title: String,
    onSelectPayload: (String) -> Unit
) {
    val id = payloadId?.trim().orEmpty()
    if (id.isBlank()) {
        return
    }
    AssistChip(
        onClick = { onSelectPayload(id) },
        label = { Text(title) }
    )
}

private fun buildPageSubtitle(page: ExecutionPageState): String? {
    val parts = buildList {
        page.mode?.takeIf { it.isNotBlank() }?.let { add(it) }
        page.status?.takeIf { it.isNotBlank() }?.let { add(it) }
        page.iteration?.let { add("iteration $it") }
    }
    return parts.takeIf { it.isNotEmpty() }?.joinToString(" · ")
}

private data class PayloadPreviewSource(
    val id: String,
    val title: String
)

private fun payloadPreviewSources(
    page: ExecutionPageState,
    payloadTitles: Map<String, String>
): List<PayloadPreviewSource> {
    val result = mutableListOf<PayloadPreviewSource>()
    page.modelSteps.forEach { step ->
        addPayloadSource(result, step.requestPayloadId, payloadTitles)
        addPayloadSource(result, step.providerRequestPayloadId, payloadTitles)
        addPayloadSource(result, step.responsePayloadId, payloadTitles)
        addPayloadSource(result, step.providerResponsePayloadId, payloadTitles)
    }
    page.toolSteps.forEach { step ->
        addPayloadSource(result, step.requestPayloadId, payloadTitles)
        addPayloadSource(result, step.responsePayloadId, payloadTitles)
    }
    return result
}

private fun addPayloadSource(
    result: MutableList<PayloadPreviewSource>,
    payloadId: String?,
    payloadTitles: Map<String, String>
) {
    val id = payloadId?.trim().orEmpty()
    if (id.isBlank()) return
    if (result.none { it.id == id }) {
        result += PayloadPreviewSource(id = id, title = payloadTitles[id] ?: "Execution payload")
    }
}
