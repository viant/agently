package com.viant.agently.android

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject

private val hiddenRouterPayloadKeys = setOf(
    "appendToolBundles",
    "suggestedProfileId",
    "templateId",
    "classification",
    "prompting",
    "directAction",
    "scope",
    "clarificationNeeded",
    "clarificationQuestion"
)

private val visibleContentJson = Json { ignoreUnknownKeys = true }

internal fun sanitizeVisibleAssistantText(raw: String?): String? {
    val trimmed = raw?.trim().orEmpty()
    if (trimmed.isEmpty()) return null
    val stripped = stripHiddenRouterPayload(trimmed)
    return stripped.takeIf { it.isNotEmpty() }
}

private fun stripHiddenRouterPayload(text: String): String {
    if (isHiddenRouterPayloadJson(text)) return ""
    val start = text.indexOf('{')
    val end = text.lastIndexOf('}')
    if (start < 0 || end <= start) return text
    val candidate = text.substring(start, end + 1).trim()
    if (!isHiddenRouterPayloadJson(candidate)) return text
    return text.substring(0, start).trim()
}

private fun isHiddenRouterPayloadJson(text: String): Boolean {
    val payloadObject = runCatching {
        visibleContentJson.parseToJsonElement(text).jsonObject
    }.getOrNull() ?: return false
    return payloadObject.keys.any(hiddenRouterPayloadKeys::contains)
}
