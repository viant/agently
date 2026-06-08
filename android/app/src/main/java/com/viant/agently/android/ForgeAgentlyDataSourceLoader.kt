package com.viant.agently.android

import android.util.Log
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.FetchDatasourceInput
import com.viant.agentlysdk.fetchDatasource
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.JsonUtil

private const val DATA_SOURCE_LOG_TAG = "ForgeAgentlyDS"

internal fun makeForgeAgentlyDataSourceLoader(
    client: AgentlyClient
): suspend (ForgeRuntime.DataSourceFetchRequest) -> ForgeRuntime.DataSourceFetchResult? {
    return loader@{ request ->
        val service = request.dataSource.service
        val endpoint = service?.endpoint?.trim().orEmpty()
        val uri = service?.uri?.trim().orEmpty()

        if (endpoint.isNotEmpty() && !endpoint.equals("agentlyAPI", ignoreCase = true)) {
            return@loader null
        }
        val datasourceId = extractDatasourceId(uri) ?: return@loader null

        val inputs = request.resolvedInputs.toMutableMap()
        val rawParameters = request.input.parameters
        val nestedInput = rawParameters["input"]
        if (nestedInput != null) {
            inputs["input"] = nestedInput
        }
        rawParameters["page"]?.let { inputs["page"] = it }
        rawParameters
            .filterKeys { it !in setOf("input", "page", "parameters") }
            .forEach { (key, value) ->
                if (inputs[key] == null) {
                    inputs[key] = value
                }
            }
        if (request.input.filter.isNotEmpty()) {
            if (inputs["input"] is Map<*, *>) {
                val inputObject = JsonUtil.asStringMap(inputs["input"]).toMutableMap()
                val queryObject = JsonUtil.asStringMap(inputObject["query"]).toMutableMap()
                queryObject.putAll(request.input.filter)
                inputObject["query"] = queryObject
                inputs["input"] = inputObject
            } else {
                inputs.putAll(request.input.filter)
            }
        }
        request.input.page?.let { inputs["page"] = it }
        request.dataSource.paging?.takeIf { it.enabled != false }?.let { paging ->
            val pageKey = paging.parameters["page"]?.trim().orEmpty()
            val sizeKey = paging.parameters["size"]?.trim().orEmpty()
            if (pageKey.isNotEmpty() && inputs[pageKey] == null) {
                inputs[pageKey] = request.input.page ?: 1
            }
            val size = paging.size
            if (sizeKey.isNotEmpty() && inputs[sizeKey] == null && size != null && size > 0) {
                inputs[sizeKey] = size
            }
        }

        if (datasourceId in setOf("line_header_lookup", "campaign_header_lookup", "order_header_lookup")) {
            Log.d(
                DATA_SOURCE_LOG_TAG,
                "fetch start id=$datasourceId conversation=${request.conversationId.orEmpty()} resolvedInputs=${request.resolvedInputs} inputs=$inputs window=${request.windowId}"
            )
        }

        val response = client.fetchDatasource(
            FetchDatasourceInput(
                id = datasourceId,
                inputs = inputs.takeIf { it.isNotEmpty() }?.mapValues { JsonUtil.anyToElement(it.value) },
                conversationId = request.conversationId?.takeIf { it.isNotBlank() }
            )
        )
        if (datasourceId in setOf("line_header_lookup", "campaign_header_lookup", "order_header_lookup")) {
            val firstRow = response.rows.firstOrNull()
            Log.d(
                DATA_SOURCE_LOG_TAG,
                "fetch done id=$datasourceId rows=${response.rows.size} firstRow=$firstRow metrics=${response.metrics}"
            )
        }
        ForgeRuntime.DataSourceFetchResult(
            rows = response.rows.map { row -> row.mapValues { JsonUtil.elementToAny(it.value) } },
            metrics = response.metrics?.mapValues { JsonUtil.elementToAny(it.value) } ?: emptyMap()
        )
    }
}

private fun extractDatasourceId(uri: String): String? {
    val normalized = uri.trim().substringBefore('?')
    val marker = "/v1/api/datasources/"
    val start = normalized.indexOf(marker)
    if (start == -1) {
        return null
    }
    val suffix = normalized.substring(start + marker.length)
    val id = suffix.substringBefore("/fetch").trim().trim('/')
    return id.ifBlank { null }
}
