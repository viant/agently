package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.WorkspaceMetadata

@Composable
internal fun SettingsScreen(
    configuredAppApiBaseUrl: String,
    currentAppApiBaseUrl: String,
    metadata: WorkspaceMetadata?,
    currentPreferredAgentId: String,
    savedLoginConfig: SavedLoginConfig,
    loading: Boolean,
    error: String?,
    onBack: () -> Unit,
    onRefreshWorkspace: () -> Unit,
    onSave: (String, String, SavedLoginConfig) -> Unit,
    onResetAppOverrides: () -> Unit,
    onClearAuthSecrets: () -> Unit
) {
    var endpointDraft by remember(currentAppApiBaseUrl) { mutableStateOf(currentAppApiBaseUrl) }
    var preferredAgentDraft by remember(currentPreferredAgentId) { mutableStateOf(currentPreferredAgentId) }
    var idpUsernameDraft by remember(savedLoginConfig) { mutableStateOf(savedLoginConfig.username) }
    var idpPasswordDraft by remember(savedLoginConfig) { mutableStateOf(savedLoginConfig.password) }
    val discoveredAgents = remember(metadata) { workspaceAgentChoices(metadata) }
    val saveSettings = {
        onSave(
            endpointDraft,
            preferredAgentDraft,
            SavedLoginConfig(
                username = idpUsernameDraft.trim(),
                password = idpPasswordDraft
            )
        )
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .navigationBarsPadding()
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        Surface(
            color = Color(0xFFF8FAFD),
            border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
            shape = MaterialTheme.shapes.large,
            modifier = Modifier.fillMaxWidth()
        ) {
            Column(
                modifier = Modifier.padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Text("Settings", style = MaterialTheme.typography.headlineSmall)
                Text(
                    "Configure the phone client, inspect the discovered workspace, and choose which agent new conversations should use by default.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = Color(0xFF667085)
                )
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    OutlinedButton(onClick = onBack, modifier = Modifier.weight(1f)) {
                        Text("Back")
                    }
                    OutlinedButton(
                        onClick = onRefreshWorkspace,
                        enabled = !loading,
                        modifier = Modifier.weight(1f)
                    ) {
                        Text("Refresh")
                    }
                    Button(
                        onClick = saveSettings,
                        enabled = !loading,
                        modifier = Modifier.weight(1f)
                    ) {
                        Text("Save")
                    }
                }
                error?.let {
                    Card(modifier = Modifier.fillMaxWidth()) {
                        Text(
                            "Error: $it",
                            color = Color(0xFFB42318),
                            style = MaterialTheme.typography.bodySmall,
                            modifier = Modifier.padding(14.dp)
                        )
                    }
                }
            }
        }

        Card(modifier = Modifier.fillMaxWidth()) {
            Column(
                modifier = Modifier.padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Text("App Endpoint", style = MaterialTheme.typography.titleMedium)
                OutlinedTextField(
                    value = endpointDraft,
                    onValueChange = { endpointDraft = it },
                    label = { Text("Agently / MCP host") },
                    placeholder = { Text(configuredAppApiBaseUrl) },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true
                )
                Text(
                    "Build default: $configuredAppApiBaseUrl",
                    style = MaterialTheme.typography.labelSmall,
                    color = Color(0xFF667085)
                )
            }
        }

        Card(modifier = Modifier.fillMaxWidth()) {
            Column(
                modifier = Modifier.padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Text("Workspace", style = MaterialTheme.typography.titleMedium)
                Text(
                    metadata?.workspaceRoot ?: "Workspace not discovered yet.",
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFF667085),
                    maxLines = 3,
                    overflow = TextOverflow.Ellipsis
                )
                metadata?.version?.takeIf { it.isNotBlank() }?.let {
                    Text("Version $it", style = MaterialTheme.typography.labelSmall, color = Color(0xFF667085))
                }
                Text(
                    "Workspace default agent: ${metadata?.defaultAgent ?: metadata?.defaults?.agent ?: "n/a"}",
                    style = MaterialTheme.typography.bodySmall
                )
                Text(
                    "App default agent: ${resolvePreferredAgentId(preferredAgentDraft, metadata) ?: "n/a"}",
                    style = MaterialTheme.typography.bodySmall
                )
                if (discoveredAgents.isEmpty()) {
                    Text(
                        "No agent list published yet. The app will fall back to the workspace default agent.",
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                } else {
                    Row(
                        modifier = Modifier.horizontalScroll(rememberScrollState()),
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        FilterChip(
                            selected = preferredAgentDraft.isBlank(),
                            onClick = { preferredAgentDraft = "" },
                            label = { Text("Workspace Default") }
                        )
                        discoveredAgents.forEach { choice ->
                            FilterChip(
                                selected = preferredAgentDraft == choice.id,
                                onClick = { preferredAgentDraft = choice.id },
                                label = { Text(choice.label) }
                            )
                        }
                    }
                }
            }
        }

        Card(modifier = Modifier.fillMaxWidth()) {
            Column(
                modifier = Modifier.padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Text("OAuth Sign-In Helper", style = MaterialTheme.typography.titleMedium)
                Text(
                    "These values are stored encrypted on-device and only help autofill the OAuth web login. Agently still authenticates through OAuth.",
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFF667085)
                )
                OutlinedTextField(
                    value = idpUsernameDraft,
                    onValueChange = { idpUsernameDraft = it },
                    label = { Text("IDP username") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true
                )
                OutlinedTextField(
                    value = idpPasswordDraft,
                    onValueChange = { idpPasswordDraft = it },
                    label = { Text("IDP password") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true
                )
                OutlinedButton(
                    onClick = onClearAuthSecrets,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text("Clear Saved Auth")
                }
            }
        }

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            OutlinedButton(
                onClick = onResetAppOverrides,
                enabled = !loading,
                modifier = Modifier.weight(1f)
            ) {
                Text("Reset Overrides")
            }
            Button(
                onClick = saveSettings,
                enabled = !loading,
                modifier = Modifier.weight(1f)
            ) {
                Text("Save & Apply")
            }
        }
    }
}
