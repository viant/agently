package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.width
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.FetchDatasourceInput
import com.viant.agentlysdk.LookupRegistryEntry
import com.viant.agentlysdk.fetchDatasource
import com.viant.agentlysdk.AgentlyClient
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.delay
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonPrimitive

@Composable
internal fun ComposerLookupDialog(
    client: AgentlyClient,
    occurrence: ComposerLookupOccurrence,
    currentSelection: ComposerLookupSelection? = null,
    activeConversationId: String? = null,
    onDismiss: () -> Unit,
    onSelect: (Map<String, JsonElement>) -> Unit,
    onClear: (() -> Unit)? = null
) {
    var searchText by remember(occurrence.key) { mutableStateOf("") }
    var rows by remember(occurrence.key) { mutableStateOf<List<Map<String, JsonElement>>>(emptyList()) }
    var loading by remember(occurrence.key) { mutableStateOf(false) }
    var error by remember(occurrence.key) { mutableStateOf<String?>(null) }

    LaunchedEffect(occurrence.key, searchText) {
        loading = true
        error = null
        try {
            // Avoid issuing and then canceling a network request for every key
            // event while the user is still typing an order name or ID.
            if (searchText.isNotBlank()) delay(250)
            rows = loadComposerLookupRows(client, occurrence.entry, searchText, activeConversationId)
        } catch (cancelled: CancellationException) {
            // A new search replaces the previous LaunchedEffect. Cancellation
            // is normal control flow and must never surface as a lookup error.
            throw cancelled
        } catch (err: Throwable) {
            rows = emptyList()
            error = composerLookupErrorMessage(err)
        }
        loading = false
    }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(occurrence.title) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                OutlinedTextField(
                    value = searchText,
                    onValueChange = { searchText = it },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                    label = { Text("Search ${occurrence.title.lowercase()}") }
                )
                if (loading) {
                    Box(
                        modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        CircularProgressIndicator()
                    }
                }
                error?.takeIf { it.isNotBlank() }?.let {
                    Text(it, color = Color(0xFFB42318), style = MaterialTheme.typography.bodySmall)
                }
                Surface(
                    modifier = Modifier.fillMaxWidth(),
                    shape = MaterialTheme.shapes.medium,
                    color = Color.White,
                    border = BorderStroke(1.dp, Color(0xFFD0D5DD)),
                    tonalElevation = 0.dp
                ) {
                    Column {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .background(Color(0xFFF2F4F7))
                                .padding(horizontal = 12.dp, vertical = 9.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text(
                                occurrence.title,
                                modifier = Modifier.weight(1f),
                                style = MaterialTheme.typography.labelMedium,
                                color = Color(0xFF344054),
                                fontWeight = FontWeight.SemiBold
                            )
                            Text(
                                "ID",
                                style = MaterialTheme.typography.labelSmall,
                                color = Color(0xFF667085),
                                fontWeight = FontWeight.SemiBold,
                                textAlign = TextAlign.End
                            )
                        }
                        HorizontalDivider(color = Color(0xFFD0D5DD))
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .heightIn(max = 320.dp)
                                .verticalScroll(rememberScrollState())
                        ) {
                            rows.forEachIndexed { index, row ->
                                val selected = currentSelection?.token == runCatching {
                                    composerLookupSelection(occurrence, row).token
                                }.getOrNull()
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .background(if (selected) Color(0xFFEAF2FF) else Color.White)
                                        .clickable { onSelect(row) }
                                        .padding(horizontal = 12.dp, vertical = 11.dp),
                                    verticalAlignment = Alignment.CenterVertically
                                ) {
                                    Text(
                                        composerLookupRowLabel(row, occurrence.entry),
                                        modifier = Modifier.weight(1f),
                                        style = MaterialTheme.typography.bodyMedium,
                                        color = Color(0xFF101828),
                                        fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Normal,
                                        maxLines = 2,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    Spacer(Modifier.width(10.dp))
                                    Text(
                                        composerLookupRowSecondaryText(row) ?: "—",
                                        style = MaterialTheme.typography.bodySmall,
                                        color = if (selected) Color(0xFF175CD3) else Color(0xFF667085),
                                        textAlign = TextAlign.End,
                                        maxLines = 2
                                    )
                                }
                                if (index != rows.lastIndex) {
                                    HorizontalDivider(color = Color(0xFFEAECF0))
                                }
                            }
                            if (!loading && rows.isEmpty() && error == null) {
                                Text(
                                    "No matching results.",
                                    modifier = Modifier.padding(14.dp),
                                    color = Color(0xFF667085)
                                )
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) {
                Text("Close")
            }
        },
        dismissButton = {
            onClear?.let {
                TextButton(onClick = it) {
                    Text("Clear")
                }
            }
        }
    )
}

internal fun composerLookupErrorMessage(error: Throwable): String {
    val detail = error.message.orEmpty().trim()
    if (detail.contains("timeout", ignoreCase = true) ||
        detail.contains("timed out", ignoreCase = true)
    ) {
        return "The lookup service did not respond. Try again."
    }
    return detail.ifBlank { "Unable to load lookup results. Try again." }
}

internal suspend fun loadComposerLookupRows(
    client: AgentlyClient,
    entry: LookupRegistryEntry,
    searchText: String,
    activeConversationId: String? = null
): List<Map<String, JsonElement>> {
    val inputs = composerLookupSearchInputs(entry, searchText)
    val conversationId = activeConversationId?.trim()?.takeIf { it.isNotBlank() }
    return client.fetchDatasource(
        FetchDatasourceInput(
            id = entry.dataSource,
            inputs = inputs,
            conversationId = conversationId
        )
    ).rows
}

/**
 * Named lookup metadata exposes a text-search input and, when supported, an
 * exact identifier input. Web's order picker offers both filters; on mobile a
 * single field chooses the right one so typing an order ID does not search the
 * order-name column.
 */
internal fun composerLookupSearchInputs(
    entry: LookupRegistryEntry,
    searchText: String
): Map<String, JsonElement>? {
    val value = searchText.trim()
    if (value.isBlank()) return null

    val queryKey = entry.token?.queryInput?.trim().orEmpty()
    val resolveKey = entry.token?.resolveInput?.trim().orEmpty()
    val looksLikeIdentifier = value.all(Char::isDigit)
    val inputKey = when {
        looksLikeIdentifier && resolveKey.isNotBlank() -> resolveKey
        queryKey.isNotBlank() -> queryKey
        resolveKey.isNotBlank() -> resolveKey
        else -> return null
    }
    return mapOf(inputKey to JsonPrimitive(value))
}
