package com.viant.agently.android

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.FetchDatasourceInput
import com.viant.agentlysdk.LookupRegistryEntry
import com.viant.agentlysdk.fetchDatasource
import com.viant.agentlysdk.AgentlyClient
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonPrimitive

@Composable
internal fun ComposerLookupDialog(
    client: AgentlyClient,
    occurrence: ComposerLookupOccurrence,
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
            rows = loadComposerLookupRows(client, occurrence.entry, searchText, activeConversationId)
        } catch (err: Throwable) {
            rows = emptyList()
            error = err.message ?: "Lookup failed."
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
                    placeholder = { Text("Search") }
                )
                if (loading) {
                    CircularProgressIndicator()
                }
                error?.takeIf { it.isNotBlank() }?.let {
                    Text(it, color = Color(0xFFB42318), style = MaterialTheme.typography.bodySmall)
                }
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(max = 320.dp)
                        .verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    rows.forEach { row ->
                        Button(
                            onClick = { onSelect(row) },
                            modifier = Modifier.fillMaxWidth()
                        ) {
                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(vertical = 2.dp),
                                verticalArrangement = Arrangement.spacedBy(2.dp)
                            ) {
                                Text(
                                    composerLookupRowLabel(row, occurrence.entry),
                                    modifier = Modifier.fillMaxWidth()
                                )
                                composerLookupRowSecondaryText(row)?.let { secondary ->
                                    Text(
                                        secondary,
                                        style = MaterialTheme.typography.bodySmall,
                                        color = Color(0xFF667085),
                                        modifier = Modifier.fillMaxWidth()
                                    )
                                }
                            }
                        }
                    }
                    if (!loading && rows.isEmpty() && error == null) {
                        Text("No matching results.", color = Color(0xFF667085))
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

internal suspend fun loadComposerLookupRows(
    client: AgentlyClient,
    entry: LookupRegistryEntry,
    searchText: String,
    activeConversationId: String? = null
): List<Map<String, JsonElement>> {
    val queryKey = entry.token?.queryInput?.trim().orEmpty()
    val inputs = if (queryKey.isNotBlank() && searchText.trim().isNotBlank()) {
        mapOf(queryKey to JsonPrimitive(searchText.trim()))
    } else {
        null
    }
    val conversationId = activeConversationId?.trim()?.takeIf { it.isNotBlank() }
    return client.fetchDatasource(
        FetchDatasourceInput(
            id = entry.dataSource,
            inputs = inputs,
            conversationId = conversationId
        )
    ).rows
}
