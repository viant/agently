package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.ApprovalCallbackPayload
import com.viant.agentlysdk.ApprovalMeta
import com.viant.agentlysdk.DecideToolApprovalInput
import com.viant.agentlysdk.ListPendingToolApprovalsInput
import com.viant.agentlysdk.PendingToolApproval
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject

internal data class ApprovalDecisionRequest(
    val id: String,
    val action: String,
    val editedFields: Map<String, JsonElement>
)

internal data class ApprovalRefreshState(
    val pendingApprovals: List<PendingToolApproval>,
    val approvalEdits: Map<String, Map<String, JsonElement>>
)

internal suspend fun buildApprovalDecisionRequest(
    approval: PendingToolApproval,
    action: String,
    approvalJson: Json,
    approvalEdits: Map<String, Map<String, JsonElement>>
): ApprovalDecisionRequest {
    val meta: ApprovalMeta? = decodeApprovalMeta(approval.metadata, approvalJson)
    val callbackPayload = executeApprovalCallbacks(
        meta = meta,
        event = action,
        payload = ApprovalCallbackPayload(
            approval = meta,
            editedFields = approvalEdits[approval.id]?.let { JsonObject(it) },
            originalArgs = approval.arguments
        )
    )
    val resolvedAction = callbackPayload.action?.takeIf { it.isNotBlank() } ?: action
    val resolvedEditedFields = when (val edited = callbackPayload.editedFields) {
        is JsonObject -> edited.toMap()
        else -> approvalEdits[approval.id] ?: emptyMap()
    }
    return ApprovalDecisionRequest(
        id = approval.id,
        action = resolvedAction,
        editedFields = resolvedEditedFields
    )
}

internal suspend fun submitApprovalDecision(
    client: AgentlyClient,
    decision: ApprovalDecisionRequest
) {
    client.decideToolApproval(
        DecideToolApprovalInput(
            id = decision.id,
            action = decision.action,
            editedFields = decision.editedFields
        )
    )
}

internal suspend fun loadPendingApprovals(
    client: AgentlyClient,
    conversationId: String?
): List<PendingToolApproval> {
    if (conversationId.isNullOrBlank()) {
        return emptyList()
    }
    return client.listPendingToolApprovals(
        ListPendingToolApprovalsInput(
            conversationId = conversationId,
            status = "pending",
            limit = 20
        )
    )
}

internal fun buildApprovalRefreshState(
    approvalEdits: Map<String, Map<String, JsonElement>>,
    pendingApprovals: List<PendingToolApproval>,
    clearedApprovalId: String? = null
): ApprovalRefreshState {
    val trimmedApprovalEdits = trimApprovalEdits(approvalEdits, pendingApprovals)
    return ApprovalRefreshState(
        pendingApprovals = pendingApprovals,
        approvalEdits = if (clearedApprovalId == null) {
            trimmedApprovalEdits
        } else {
            trimmedApprovalEdits - clearedApprovalId
        }
    )
}

internal suspend fun refreshApprovalState(
    client: AgentlyClient,
    conversationId: String?,
    approvalEdits: Map<String, Map<String, JsonElement>>,
    clearedApprovalId: String? = null
): ApprovalRefreshState {
    val pendingApprovals = loadPendingApprovals(
        client = client,
        conversationId = conversationId
    )
    return buildApprovalRefreshState(
        approvalEdits = approvalEdits,
        pendingApprovals = pendingApprovals,
        clearedApprovalId = clearedApprovalId
    )
}

internal fun updateApprovalEdit(
    approvalEdits: Map<String, Map<String, JsonElement>>,
    approvalId: String,
    fieldName: String,
    value: JsonElement
): Map<String, Map<String, JsonElement>> {
    return approvalEdits.toMutableMap().apply {
        val nextFields = (this[approvalId] ?: emptyMap()).toMutableMap()
        nextFields[fieldName] = value
        this[approvalId] = nextFields
    }
}
