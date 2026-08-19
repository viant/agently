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
import androidx.compose.material3.Card
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.DeleteSweep
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.RestartAlt
import androidx.compose.material.icons.outlined.Save
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
                    SettingsIconAction(
                        label = "Back",
                        icon = Icons.AutoMirrored.Outlined.ArrowBack,
                        accent = Color(0xFF5965D8),
                        onClick = onBack,
                        modifier = Modifier.weight(1f)
                    )
                    SettingsIconAction(
                        label = "Refresh",
                        icon = Icons.Outlined.Refresh,
                        accent = Color(0xFF0A9B98),
                        onClick = onRefreshWorkspace,
                        enabled = !loading,
                        loading = loading,
                        modifier = Modifier.weight(1f)
                    )
                    SettingsIconAction(
                        label = "Save",
                        icon = Icons.Outlined.Save,
                        accent = Color(0xFF16835D),
                        onClick = saveSettings,
                        enabled = !loading,
                        modifier = Modifier.weight(1f)
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
                        singleLine = true
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
                    Text("Developer OOB Sign-In", style = MaterialTheme.typography.titleMedium)
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

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            SettingsIconAction(
                label = "Reset Overrides",
                icon = Icons.Outlined.RestartAlt,
                accent = Color(0xFFE08A1E),
                onClick = onResetAppOverrides,
                enabled = !loading,
                modifier = Modifier.weight(1f)
            )
            SettingsIconAction(
                label = "Save & Apply",
                icon = Icons.Outlined.CheckCircle,
                accent = Color(0xFF16835D),
                onClick = saveSettings,
                enabled = !loading,
                modifier = Modifier.weight(1f)
            )
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
    Column(
        modifier = modifier,
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        PhoneToolbarAction(
            icon = icon,
            contentDescription = label,
            onClick = onClick,
            accent = accent,
            enabled = enabled,
            loading = loading
        )
        Text(
            text = label,
            style = MaterialTheme.typography.labelSmall,
            color = if (enabled || loading) accent else Color(0xFF98A2B3),
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}
