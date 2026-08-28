package com.viant.agently.android

import android.util.Log
import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.FetchDatasourceInput
import com.viant.agentlysdk.Schedule
import com.viant.agentlysdk.ScheduleRun
import com.viant.agentlysdk.fetchDatasource
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.JsonUtil

internal fun makeForgeAgentlyDataSourceLoader(
    client: AgentlyClient
): suspend (ForgeRuntime.DataSourceFetchRequest) -> ForgeRuntime.DataSourceFetchResult? {
    return loader@{ request ->
        val startedAt = System.currentTimeMillis()
        val service = request.dataSource.service
        val endpoint = service?.endpoint?.trim().orEmpty()
        val uri = service?.uri?.trim().orEmpty()

        if (endpoint.isNotEmpty() && !endpoint.equals("agentlyAPI", ignoreCase = true)) {
            return@loader null
        }
        if (uri.trimEnd('/') == "/v1/api/agently/scheduler") {
            return@loader loadSchedules(client, request)
        }
        if (uri.trimEnd('/') == "/v1/api/agently/scheduler/run") {
            return@loader loadScheduleRuns(client, request)
        }
        if (uri.trimEnd('/') == "/v1/workspace/metadata") {
            return@loader loadWorkspaceOptions(client, request)
        }
        if (uri.trimEnd('/') == "/v1/workspace/metadata/publicagents") {
            return@loader loadPublicAgents(client)
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

        val response = try {
            if (BuildConfig.DEBUG) {
                runCatching {
                    Log.d(
                        "ForgeDataSource",
                        "fetch start id=$datasourceId conversationId=${request.conversationId.orEmpty()} inputs=${JsonUtil.anyToElement(inputs)}"
                    )
                }
            }
            client.fetchDatasource(
                FetchDatasourceInput(
                    id = datasourceId,
                    inputs = inputs.takeIf { it.isNotEmpty() }?.mapValues { JsonUtil.anyToElement(it.value) },
                    conversationId = request.conversationId?.takeIf { it.isNotBlank() }
                )
            )
        } catch (err: Throwable) {
            // android.util.Log is a throwing stub in local JVM tests. Logging
            // must never replace the real datasource exception being tested.
            runCatching {
                Log.e(
                    "ForgeDataSource",
                    "fetch failed id=$datasourceId conversationId=${request.conversationId.orEmpty()}",
                    err
                )
            }
            throw err
        }
        if (BuildConfig.DEBUG) {
            runCatching {
                Log.d(
                    "ForgeDataSource",
                    "fetch completed id=$datasourceId rows=${response.rows.size} elapsedMs=${System.currentTimeMillis() - startedAt}"
                )
            }
        }
        ForgeRuntime.DataSourceFetchResult(
            rows = response.rows.map { row -> row.mapValues { JsonUtil.elementToAny(it.value) } },
            metrics = response.metrics?.mapValues { JsonUtil.elementToAny(it.value) } ?: emptyMap()
        )
    }
}

private suspend fun loadPublicAgents(client: AgentlyClient): ForgeRuntime.DataSourceFetchResult {
    val rows = client.getPublicAgents().map { info ->
        mapOf("id" to info.id, "name" to info.name, "modelRef" to info.modelRef)
    }
    return ForgeRuntime.DataSourceFetchResult(rows = rows)
}

private suspend fun loadScheduleRuns(
    client: AgentlyClient,
    request: ForgeRuntime.DataSourceFetchRequest
): ForgeRuntime.DataSourceFetchResult {
    val page = (request.input.page ?: 1).coerceAtLeast(1)
    val size = request.dataSource.paging?.size?.takeIf { it > 0 } ?: 10
    val filters = request.input.filter.mapNotNull { (key, value) ->
        value?.toString()?.let { key to it }
    }.toMap()
    val scheduleId = request.resolvedInputs["scheduleId"]?.toString()
        ?: request.input.parameters["scheduleId"]?.toString()
    val response = client.listScheduleRuns(scheduleId, filters, page, size)
    return ForgeRuntime.DataSourceFetchResult(
        rows = response.rows.map(::scheduleRunRow),
        metrics = mapOf("pageCount" to response.pageCount, "totalCount" to response.totalCount)
    )
}

private suspend fun loadSchedules(
    client: AgentlyClient,
    request: ForgeRuntime.DataSourceFetchRequest
): ForgeRuntime.DataSourceFetchResult {
    val query = request.input.filter["name"]?.toString()?.trim().orEmpty()
    val all = client.listSchedules()
        .filter { query.isBlank() || it.name.contains(query, ignoreCase = true) }
    val pageSize = request.dataSource.paging?.size?.takeIf { it > 0 } ?: all.size.coerceAtLeast(1)
    val page = (request.input.page ?: 1).coerceAtLeast(1)
    val from = ((page - 1) * pageSize).coerceAtMost(all.size)
    val rows = all.drop(from).take(pageSize).map(::scheduleRow)
    val pageCount = if (all.isEmpty()) 0 else (all.size + pageSize - 1) / pageSize
    return ForgeRuntime.DataSourceFetchResult(
        rows = rows,
        metrics = mapOf("pageCount" to pageCount, "totalCount" to all.size)
    )
}

private suspend fun loadWorkspaceOptions(
    client: AgentlyClient,
    request: ForgeRuntime.DataSourceFetchRequest
): ForgeRuntime.DataSourceFetchResult {
    val metadata = client.getWorkspaceMetadata()
    val rows = when (request.dataSource.selectors?.data?.trim()) {
        "agentInfos" -> metadata.agentInfos.map { info ->
            mapOf("id" to info.id, "name" to info.name, "modelRef" to info.modelRef)
        }
        "modelInfos" -> metadata.modelInfos.map { info ->
            mapOf("id" to info.id, "name" to info.name)
        }.ifEmpty {
            metadata.models.map { id -> mapOf("id" to id, "name" to id) }
        }
        else -> emptyList()
    }
    return ForgeRuntime.DataSourceFetchResult(rows = rows)
}

private fun scheduleRow(schedule: Schedule): Map<String, Any?> = mapOf(
    "id" to schedule.id,
    "name" to schedule.name,
    "description" to schedule.description,
    "visibility" to schedule.visibility,
    "agentRef" to schedule.agentRef,
    "modelOverride" to schedule.modelOverride,
    "userCredUrl" to schedule.userCredUrl,
    "enabled" to schedule.enabled,
    "startAt" to schedule.startAt,
    "endAt" to schedule.endAt,
    "scheduleType" to schedule.scheduleType,
    "cronExpr" to schedule.cronExpr,
    "intervalSeconds" to schedule.intervalSeconds,
    "timezone" to schedule.timezone,
    "timeoutSeconds" to schedule.timeoutSeconds,
    "taskPromptUri" to schedule.taskPromptUri,
    "taskPrompt" to schedule.taskPrompt,
    "nextRunAt" to schedule.nextRunAt,
    "lastRunAt" to schedule.lastRunAt,
    "lastStatus" to schedule.lastStatus,
    "lastError" to schedule.lastError,
    "createdAt" to schedule.createdAt,
    "updatedAt" to schedule.updatedAt
)

private fun scheduleRunRow(run: ScheduleRun): Map<String, Any?> = mapOf(
    "id" to run.id,
    "scheduleId" to run.scheduleId,
    "conversationId" to run.conversationId,
    "status" to run.status,
    "errorMessage" to run.errorMessage,
    "scheduledFor" to run.scheduledFor,
    "startedAt" to run.startedAt,
    "completedAt" to run.completedAt,
    "createdAt" to run.createdAt,
    "updatedAt" to run.updatedAt
)

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
