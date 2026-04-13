package com.viant.agently.android

import com.viant.agentlysdk.PendingToolApproval
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Test

class ApprovalRuntimeTest {

    @Test
    fun `buildApprovalRefreshState trims stale edits and clears decided approval`() {
        val state = buildApprovalRefreshState(
            approvalEdits = mapOf(
                "approval-1" to mapOf("enabled" to JsonPrimitive(true)),
                "approval-2" to mapOf("enabled" to JsonPrimitive(false)),
                "approval-stale" to mapOf("enabled" to JsonPrimitive(true))
            ),
            pendingApprovals = listOf(
                PendingToolApproval(id = "approval-1", toolName = "tool/one", status = "pending"),
                PendingToolApproval(id = "approval-2", toolName = "tool/two", status = "pending")
            ),
            clearedApprovalId = "approval-2"
        )

        assertEquals(listOf("approval-1", "approval-2"), state.pendingApprovals.map { it.id })
        assertEquals(listOf("approval-1"), state.approvalEdits.keys.toList())
    }
}
