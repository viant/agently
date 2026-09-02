package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.ApplyPermissionInput
import com.viant.agentlysdk.MetadataTargetContext
import com.viant.agentlysdk.applyPermission
import com.viant.agentlysdk.fetchForgeWindowMetadata
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.ForgeTargetContext
import com.viant.forgeandroid.runtime.JsonUtil
import com.viant.forgeandroid.runtime.MetadataResolver
import com.viant.forgeandroid.runtime.WindowMetadata
import com.viant.forgeandroid.runtime.normalizeWindowMetadataJson
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject

internal fun makeForgeAgentlyWindowMetadataLoader(
    client: AgentlyClient,
    targetContext: ForgeTargetContext
): suspend (ForgeRuntime.WindowMetadataRequest) -> WindowMetadata? {
    val json = Json { ignoreUnknownKeys = true }
    val sdkTarget = MetadataTargetContext(
        platform = targetContext.platform,
        formFactor = targetContext.formFactor,
        surface = "app",
        capabilities = targetContext.capabilities.toList().sorted()
    )
    return { request ->
        val complete = client.fetchForgeWindowMetadata(request.windowKey, sdkTarget)
        val raw = if ((complete as? JsonObject)?.containsKey("authorization") == true) {
            val conversationId = request.conversationId?.trim().orEmpty()
            check(conversationId.isNotEmpty()) {
                "Authorized Forge window ${request.windowKey} requires a conversation ID"
            }
            val resource = request.parameters.mapValues { (_, value) -> JsonUtil.anyToElement(value) }
            client.applyPermission(
                request.windowKey,
                ApplyPermissionInput(
                    conversationId = conversationId,
                    resource = resource,
                    windowParams = resource,
                    targetContext = sdkTarget
                )
            )
        } else {
            complete
        }
        val normalized = normalizeWindowMetadataJson(normalizeWindowMetadataCollections(raw))
        val metadata = json.decodeFromJsonElement<WindowMetadata>(normalized)
        val metadataJson = json.encodeToJsonElement(metadata)
        val resolved = MetadataResolver.resolve(metadataJson, targetContext) ?: metadataJson
        json.decodeFromJsonElement<WindowMetadata>(normalizeWindowMetadataJson(resolved))
    }
}

private fun normalizeWindowMetadataCollections(element: JsonElement): JsonElement {
    return when (element) {
        is JsonArray -> JsonArray(element.map(::normalizeWindowMetadataCollections))
        is JsonObject -> {
            val normalized = linkedMapOf<String, JsonElement>()
            val listKeys = setOf(
                "dialogs",
                "on",
                "actions",
                "parameters",
                "args",
                "containers",
                "metrics",
                "checks",
                "rows",
                "sections",
                "items",
                "viewModes",
                "measures",
                "dimensions",
                "staticFilters",
                "dynamicFilterGroups",
                "dynamicFilterFamilies",
                "defaultChartSpecs",
                "supportedTypes",
                "options",
                "fields",
                "enum",
                "uniqueKey",
                "values"
            )
            val mapKeys = setOf(
                "dataSources",
                "dataSource",
                "params",
                "targetOverrides",
                "filterBindings",
                "properties",
                "style",
                "dataInfoSelectors",
                "selectors",
                "reportBuilders"
            )
            for ((key, value) in element) {
                val replacement = when (key) {
                    in listKeys ->
                        if (value is JsonNull) JsonArray(emptyList()) else normalizeWindowMetadataCollections(value)
                    in mapKeys ->
                        if (value is JsonNull) JsonObject(emptyMap()) else normalizeWindowMetadataCollections(value)
                    else -> normalizeWindowMetadataCollections(value)
                }
                normalized[key] = replacement
            }
            JsonObject(normalized)
        }
        else -> element
    }
}
