package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Card
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.DeleteSweep
import androidx.compose.material.icons.outlined.MoreVert
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.RestartAlt
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.Alignment
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
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
    authSessionId: String?,
    loading: Boolean,
    error: String?,
    onBack: () -> Unit,
    onRefreshWorkspace: () -> Unit,
    onSave: (String, String, SavedLoginConfig) -> Unit,
    onResetAppOverrides: () -> Unit,
    onClearAuthSecrets: () -> Unit
) {
    val developerAuthEnabled = BuildConfig.DEBUG
    var endpointDraft by remember(currentAppApiBaseUrl) { mutableStateOf(currentAppApiBaseUrl) }
    var preferredAgentDraft by remember(currentPreferredAgentId) { mutableStateOf(currentPreferredAgentId) }
    var oobSecretRefDraft by remember(savedLoginConfig) { mutableStateOf(savedLoginConfig.oobSecretRef) }
    var actionsMenuExpanded by remember { mutableStateOf(false) }
    val endpointValid = isValidWorkspaceBaseUrl(endpointDraft)
    val discoveredAgents = remember(metadata) { workspaceAgentChoices(metadata) }
    val saveSettings = {
        onSave(
            endpointDraft,
            preferredAgentDraft,
            SavedLoginConfig(
                oobSecretRef = oobSecretRefDraft.trim()
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
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text("Settings", style = MaterialTheme.typography.titleLarge)
                    }
                    SettingsIconAction(
                        label = "Back",
                        icon = Icons.Outlined.Close,
                        accent = Color(0xFF5965D8),
                        onClick = onBack
                    )
                    Box {
                        SettingsIconAction(
                            label = "More settings actions",
                            icon = Icons.Outlined.MoreVert,
                            accent = Color(0xFF7D52D9),
                            onClick = { actionsMenuExpanded = true }
                        )
                        DropdownMenu(
                            expanded = actionsMenuExpanded,
                            onDismissRequest = { actionsMenuExpanded = false }
                        ) {
                            DropdownMenuItem(
                                text = { Text("Refresh workspace") },
                                leadingIcon = { androidx.compose.material3.Icon(Icons.Outlined.Refresh, contentDescription = null) },
                                enabled = !loading,
                                onClick = {
                                    actionsMenuExpanded = false
                                    onRefreshWorkspace()
                                }
                            )
                            DropdownMenuItem(
                                text = { Text("Reset overrides") },
                                leadingIcon = { androidx.compose.material3.Icon(Icons.Outlined.RestartAlt, contentDescription = null) },
                                enabled = !loading && isValidWorkspaceBaseUrl(configuredAppApiBaseUrl),
                                onClick = {
                                    actionsMenuExpanded = false
                                    onResetAppOverrides()
                                }
                            )
                        }
                    }
                    SettingsIconAction(
                        label = "Save",
                        icon = Icons.Outlined.Check,
                        accent = Color(0xFF16835D),
                        onClick = saveSettings,
                        enabled = !loading && endpointValid
                    )
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
                Text("Workspace Endpoint", style = MaterialTheme.typography.titleMedium)
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    workspaceEndpointOptions.forEach { option ->
                        FilterChip(
                            selected = normalizeApiBaseUrl(endpointDraft) == option.value,
                            onClick = { endpointDraft = option.value },
                            label = { Text(option.title) }
                        )
                    }
                    if (selectedWorkspaceEndpointOption(endpointDraft) == null &&
                        normalizeApiBaseUrl(endpointDraft).isNotBlank()
                    ) {
                        FilterChip(
                            selected = true,
                            onClick = {},
                            label = { Text("Custom") }
                        )
                    }
                }
                Text(
                    normalizeApiBaseUrl(endpointDraft).ifBlank { configuredAppApiBaseUrl },
                    style = MaterialTheme.typography.labelSmall,
                    color = Color(0xFF667085)
                )
                if (developerAuthEnabled) {
                    OutlinedTextField(
                        value = endpointDraft,
                        onValueChange = { endpointDraft = it },
                        label = { Text("Developer endpoint") },
                        placeholder = { Text(configuredAppApiBaseUrl) },
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true,
                        isError = !endpointValid,
                        supportingText = if (!endpointValid) {
                            { Text("Use a complete http:// or https:// URL.") }
                        } else {
                            null
                        }
                    )
                    Text(
                        "Build default: $configuredAppApiBaseUrl",
                        style = MaterialTheme.typography.labelSmall,
                        color = Color(0xFF667085)
                    )
                }
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

        if (developerAuthEnabled) {
            Card(modifier = Modifier.fillMaxWidth()) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    Text("Sign-In Helpers", style = MaterialTheme.typography.titleMedium)
                    Text(
                        "Optional OOB reference for developer verification builds.",
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                    authSessionId?.takeIf { it.isNotBlank() }?.let {
                        Text(
                            "Session ID: $it",
                            style = MaterialTheme.typography.bodySmall,
                            color = Color(0xFF475467)
                        )
                    }
                    OutlinedTextField(
                        value = oobSecretRefDraft,
                        onValueChange = { oobSecretRefDraft = it },
                        label = { Text("OOB secret reference") },
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                    SettingsIconAction(
                        label = "Clear Saved Auth",
                        icon = Icons.Outlined.DeleteSweep,
                        accent = Color(0xFFD34B5F),
                        onClick = onClearAuthSecrets,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            }
        }

    }
}

@Composable
private fun SettingsIconAction(
    label: String,
    icon: ImageVector,
    accent: Color,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    loading: Boolean = false
) {
    Box(modifier = modifier, contentAlignment = Alignment.CenterEnd) {
        PhoneToolbarAction(
            icon = icon,
            contentDescription = label,
            onClick = onClick,
            accent = accent,
            enabled = enabled,
            loading = loading
        )
    }
}
