package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.StarterTask
import com.viant.agentlysdk.WorkspaceMetadata
import java.util.Locale

internal fun resolveSelectedAgentChoice(
    preferredAgentId: String?,
    metadata: WorkspaceMetadata?
): WorkspaceAgentChoice? {
    val selectedId = resolvePreferredAgentId(preferredAgentId, metadata)?.trim().orEmpty()
    if (selectedId.isBlank()) {
        return null
    }
    return workspaceAgentChoices(metadata).firstOrNull { it.id == selectedId }
        ?: WorkspaceAgentChoice(
            id = selectedId,
            label = selectedId.humanizedAgentLabel()
        )
}

internal fun resolveSelectedAgentLabel(
    preferredAgentId: String?,
    metadata: WorkspaceMetadata?
): String? = resolveSelectedAgentChoice(preferredAgentId, metadata)?.label

internal fun showWorkspaceAgentSelection(metadata: WorkspaceMetadata?): Boolean {
    return workspaceAgentChoices(metadata).size > 1
}

internal fun countStarterTaskAgents(metadata: WorkspaceMetadata?): Int {
    if (metadata == null) return 0
    return metadata.agentInfos.count { info ->
        info.internalAgent != true && info.starterTasks.any { task ->
            task.title?.isNotBlank() == true && task.prompt?.isNotBlank() == true
        }
    }
}

internal fun workspaceStarterTasks(
    preferredAgentId: String?,
    metadata: WorkspaceMetadata?
): List<StarterTask> {
    val selectedId = resolvePreferredAgentId(preferredAgentId, metadata)?.trim().orEmpty()
    if (selectedId.isBlank()) {
        return emptyList()
    }
    val match = metadata?.agentInfos?.firstOrNull { info ->
        val infoId = info.id?.trim().orEmpty()
        infoId.equals(selectedId, ignoreCase = true)
    } ?: return emptyList()
    return match.starterTasks.filter { task ->
        task.prompt?.isNotBlank() == true && task.title?.isNotBlank() == true
    }
}

@Composable
internal fun WorkspaceTaskStartSection(
    metadata: WorkspaceMetadata?,
    preferredAgentId: String,
    onSelectAgent: (String?) -> Unit,
    onSelectStarterTask: (String) -> Unit,
    modifier: Modifier = Modifier
) {
    val agentChoices = remember(metadata) { workspaceAgentChoices(metadata) }
    val selectedAgentLabel = remember(preferredAgentId, metadata) {
        resolveSelectedAgentLabel(preferredAgentId, metadata)
    }
    val starterTasks = remember(preferredAgentId, metadata) {
        workspaceStarterTasks(preferredAgentId, metadata)
    }
    val showAgentSelector = agentChoices.size > 1
    val showAgentScopedCopy = remember(metadata) {
        countStarterTaskAgents(metadata) > 1
    }
    if (selectedAgentLabel.isNullOrBlank() && starterTasks.isEmpty()) {
        return
    }

    Surface(
        color = Color(0xFFF8FAFD),
        border = BorderStroke(1.dp, Color(0xFFDDE4F1)),
        shape = MaterialTheme.shapes.large,
        modifier = modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 18.dp, vertical = 18.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    text = if (showAgentScopedCopy) {
                        selectedAgentLabel?.let { "Start with $it" } ?: "Choose an agent"
                    } else {
                        "Starter tasks"
                    },
                    style = MaterialTheme.typography.titleMedium,
                    color = Color(0xFF101828)
                )
                Text(
                    text = if (showAgentScopedCopy) {
                        "Starter tasks follow the selected public agent, matching the web start flow."
                    } else {
                        "Start with one of the published workspace tasks."
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFF667085)
                )
            }

            if (showAgentSelector) {
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    FilterChip(
                        selected = preferredAgentId.isBlank(),
                        onClick = { onSelectAgent(null) },
                        label = { Text("Workspace Default") }
                    )
                    agentChoices.forEach { choice ->
                        FilterChip(
                            selected = preferredAgentId == choice.id,
                            onClick = { onSelectAgent(choice.id) },
                            label = { Text(choice.label) }
                        )
                    }
                }
            }

            if (starterTasks.isEmpty()) {
                Text(
                    text = "This agent has no published starter tasks yet. You can still begin with your own prompt below.",
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFF667085)
                )
            } else {
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    starterTasks.forEach { task ->
                        ElevatedCard(
                            modifier = Modifier
                                .widthIn(min = 220.dp, max = 280.dp)
                                .clickable {
                                    val prompt = task.prompt?.trim().orEmpty()
                                    if (prompt.isNotBlank()) {
                                        onSelectStarterTask(prompt)
                                    }
                                }
                        ) {
                            Column(
                                modifier = Modifier.padding(horizontal = 14.dp, vertical = 14.dp),
                                verticalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Text(
                                    text = task.title?.trim().orEmpty(),
                                    style = MaterialTheme.typography.titleSmall,
                                    color = Color(0xFF101828),
                                    maxLines = 2,
                                    overflow = TextOverflow.Ellipsis
                                )
                                Text(
                                    text = task.description?.trim().takeUnless { it.isNullOrBlank() }
                                        ?: selectedAgentLabel.orEmpty(),
                                    style = MaterialTheme.typography.bodySmall,
                                    color = Color(0xFF667085),
                                    maxLines = 3,
                                    overflow = TextOverflow.Ellipsis
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

private fun String.humanizedAgentLabel(): String {
    return replace("_", " ")
        .replace("-", " ")
        .trim()
        .split(Regex("\\s+"))
        .filter { it.isNotBlank() }
        .joinToString(" ") { token ->
            token.lowercase(Locale.US).replaceFirstChar { ch ->
                if (ch.isLowerCase()) ch.titlecase(Locale.US) else ch.toString()
            }
        }
        .ifBlank { this }
}
