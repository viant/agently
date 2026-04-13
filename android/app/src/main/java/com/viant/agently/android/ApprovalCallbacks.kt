package com.viant.agently.android

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray

internal suspend fun executeApprovalCallbacks(
    meta: com.viant.agentlysdk.ApprovalMeta?,
    event: String,
    payload: com.viant.agentlysdk.ApprovalCallbackPayload
): com.viant.agentlysdk.ApprovalCallbackPayload {
    val callbacks = meta?.forge?.callbacks.orEmpty()
    if (callbacks.isEmpty()) {
        return payload
    }
    var current = payload
    callbacks.forEach { callback ->
        val callbackEvent = callback.event?.trim()?.lowercase().orEmpty()
        if (callbackEvent.isNotEmpty() && callbackEvent != event.trim().lowercase()) {
            return@forEach
        }
        val handlerName = callback.handler?.trim().orEmpty()
        val result = runApprovalCallbackHandler(handlerName, current)
        current = mergeApprovalCallbackPayload(current, result)
    }
    return current
}

private fun runApprovalCallbackHandler(
    handlerName: String,
    payload: com.viant.agentlysdk.ApprovalCallbackPayload
): com.viant.agentlysdk.ApprovalCallbackResult? {
    return when (handlerName) {
        "approval.filterEnvNames" -> filterEnvNamesApprovalCallback(payload)
        else -> null
    }
}

private fun filterEnvNamesApprovalCallback(
    payload: com.viant.agentlysdk.ApprovalCallbackPayload
): com.viant.agentlysdk.ApprovalCallbackResult {
    val originalArgs = payload.originalArgs as? JsonObject
    val editedFields = payload.editedFields as? JsonObject
    val requested = (originalArgs?.get("names") as? JsonArray).orEmpty().mapNotNull { (it as? JsonPrimitive)?.content }
    val selected = ((editedFields?.get("names") as? JsonArray).orEmpty().mapNotNull { (it as? JsonPrimitive)?.content }).toSet()
    val normalized = if (requested.isEmpty()) {
        selected.toList()
    } else {
        requested.filter { selected.contains(it) }
    }
    return com.viant.agentlysdk.ApprovalCallbackResult(
        editedFields = JsonObject(
            mapOf("names" to buildJsonArray { normalized.forEach { add(JsonPrimitive(it)) } })
        )
    )
}

private fun mergeApprovalCallbackPayload(
    base: com.viant.agentlysdk.ApprovalCallbackPayload,
    result: com.viant.agentlysdk.ApprovalCallbackResult?
): com.viant.agentlysdk.ApprovalCallbackPayload {
    if (result == null) {
        return base
    }
    val mergedEditedFields = mergeJsonObjects(base.editedFields, result.editedFields)
    return base.copy(
        editedFields = mergedEditedFields,
        action = result.action?.takeIf { it.isNotBlank() } ?: base.action
    )
}

private fun mergeJsonObjects(base: JsonElement?, override: JsonElement?): JsonElement? {
    val baseObject = base as? JsonObject
    val overrideObject = override as? JsonObject
    return when {
        baseObject == null -> override
        overrideObject == null -> base
        else -> JsonObject(baseObject.toMap() + overrideObject.toMap())
    }
}
