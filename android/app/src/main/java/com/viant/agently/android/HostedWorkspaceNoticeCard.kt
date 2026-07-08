package com.viant.agently.android

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp

@Composable
internal fun HostedWorkspaceNoticeCard(
    notice: HostedWorkspaceEventNotice,
    modifier: Modifier = Modifier
) {
    Surface(
        color = Color(0xFFFFFBEB),
        border = BorderStroke(1.dp, Color(0xFFFEDF89)),
        shape = MaterialTheme.shapes.large,
        modifier = modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 14.dp, vertical = 12.dp)
        ) {
            Text(
                text = "Workspace view unavailable",
                style = MaterialTheme.typography.labelLarge,
                color = Color(0xFF93370D)
            )
            Text(
                text = notice.message,
                style = MaterialTheme.typography.bodySmall,
                color = Color(0xFF7A2E0E)
            )
        }
    }
}
