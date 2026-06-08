package com.viant.agently.android

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.Goal

@Composable
internal fun GoalSummaryCard(goal: Goal) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                Text("Goal", style = MaterialTheme.typography.titleSmall)
                Text(
                    goal.status.replace('_', ' ').replaceFirstChar { it.uppercase() },
                    style = MaterialTheme.typography.labelMedium,
                    color = Color(0xFF667085)
                )
            }
            Text(goal.objective, style = MaterialTheme.typography.bodyMedium)
            val reason = listOfNotNull(goal.statusReason, goal.pauseReason)
                .map { it.trim() }
                .firstOrNull { it.isNotEmpty() }
            if (!reason.isNullOrBlank()) {
                Text(reason, style = MaterialTheme.typography.bodySmall, color = Color(0xFF667085))
            }
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Metric("Tokens", goal.tokenBudget?.let { "${goal.tokensUsed ?: 0}/$it" } ?: "${goal.tokensUsed ?: 0}")
                Metric("Time", "${goal.timeUsedSeconds ?: 0}s")
            }
        }
    }
}

@Composable
private fun Metric(title: String, value: String) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(title, style = MaterialTheme.typography.labelSmall, color = Color(0xFF667085))
        Text(value, style = MaterialTheme.typography.bodySmall)
    }
}
