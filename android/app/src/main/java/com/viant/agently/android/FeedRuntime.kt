package com.viant.agently.android

import com.viant.agentlysdk.FeedDataResponse
import com.viant.forgeandroid.runtime.ContainerDef
import com.viant.forgeandroid.runtime.ContentDef
import com.viant.forgeandroid.runtime.DataSourceDef
import com.viant.forgeandroid.runtime.DataSourceContext
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.JsonUtil
import com.viant.forgeandroid.runtime.SelectionState
import com.viant.forgeandroid.runtime.ServiceDef
import com.viant.forgeandroid.runtime.ViewDef
import com.viant.forgeandroid.runtime.WindowContext
import com.viant.forgeandroid.runtime.WindowMetadata
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

internal val feedRuntimeJson = Json { ignoreUnknownKeys = true }

internal data class FeedCollections(
    val rootDataSource: String?,
    val collections: Map<String, List<Map<String, Any?>>>
)

internal fun buildFeedWindowMetadata(payload: FeedDataResponse): WindowMetadata {
    val ui = payload.ui ?: error("Feed ${payload.feedId ?: payload.title ?: "unknown"} is missing ui metadata")
    val content = decodeFeedContent(ui)
    val dataSources = decodeFeedDataSources(payload.dataSources).toMutableMap()
    referencedFeedLookupDataSources(content).forEach { ref ->
        dataSources.putIfAbsent(
            ref,
            DataSourceDef(
                service = ServiceDef(endpoint = "agentlyAPI", uri = "/v1/api/datasources/$ref/fetch"),
                autoFetch = false
            )
        )
    }
    return WindowMetadata(
        namespace = "agently.android.feed",
        dataSources = dataSources,
        view = ViewDef(
            content = content
        )
    )
}

private fun referencedFeedLookupDataSources(content: ContentDef): Set<String> {
    val refs = linkedSetOf<String>()
    fun visit(container: ContainerDef) {
        val lookup = container.lookup
        (lookup?.get("dataSourceRef") as? JsonPrimitive)?.contentOrNull?.trim()?.takeIf(String::isNotEmpty)?.let(refs::add)
        val drill = lookup?.get("drill") as? JsonObject
        (drill?.get("dataSourceRef") as? JsonPrimitive)?.contentOrNull?.trim()?.takeIf(String::isNotEmpty)?.let(refs::add)
        container.containers.forEach(::visit)
    }
    content.containers.forEach(::visit)
    return refs
}

private fun decodeFeedDataSources(rawDataSources: JsonObject?): Map<String, DataSourceDef> {
    return normalizeFeedDataSources(rawDataSources).mapValues { (_, def) ->
        sanitizeFeedDataSource(feedRuntimeJson.decodeFromString<DataSourceDef>(def.toString()))
    }
}

private fun sanitizeFeedDataSource(def: DataSourceDef): DataSourceDef {
    return if (def.service == null) def.copy(paging = null) else def
}

private fun decodeFeedContent(ui: JsonObject): ContentDef {
    return decodeFeedContentObject(ui) ?: wrapFeedContainer(
        decodeFeedContainer(ui)
            ?: error("Feed ui must decode as either ContentDef or ContainerDef")
    )
}

private fun decodeFeedContentObject(ui: JsonObject): ContentDef? {
    val content = runCatching { feedRuntimeJson.decodeFromString<ContentDef>(ui.toString()) }.getOrNull() ?: return null
    if ("containers" !in ui) {
        return null
    }
    require(content.containers.isNotEmpty()) { "Feed content must contain at least one container" }
    return content
}

private fun decodeFeedContainer(ui: JsonObject): ContainerDef? {
    return runCatching { feedRuntimeJson.decodeFromString<ContainerDef>(ui.toString()) }.getOrNull()
}

private fun wrapFeedContainer(container: ContainerDef): ContentDef {
    return ContentDef(containers = listOf(container))
}

internal fun wireFeedWindow(
    runtime: ForgeRuntime,
    windowId: String,
    payload: FeedDataResponse,
    turnId: String? = null
) {
    val windowContext = runtime.windowContext(windowId)
    runtime.registerFeedPatchHandler { targetWindowId, operation ->
        if (!AndroidFeedCanonicalRegistry.has(runtime, targetWindowId)) {
            false
        } else {
            AndroidFeedCanonicalRegistry.apply(runtime, targetWindowId, listOf(operation))
            true
        }
    }
    val effectiveData = AndroidFeedCanonicalRegistry.register(runtime, windowId, payload, turnId)
    val collections = computeFeedCollections(payload.dataSources, effectiveData)
    hydrateFeedDataSources(windowContext, collections.collections)
    selectInitialFeedRoot(windowContext, collections)
}

internal fun hydrateFeedDataSources(
    windowContext: WindowContext,
    collections: Map<String, List<Map<String, Any?>>>
) {
    collections.forEach { (dataSourceRef, rows) ->
        val context = windowContext.contextOrNull(dataSourceRef) ?: return@forEach
        context.collection.set(rows)
        context.control.set(context.control.peek().copy(loading = false, error = null))
        if (rows.size == 1) {
            context.setForm(rows.first())
        }
    }
}

private fun selectInitialFeedRoot(
    windowContext: WindowContext,
    collections: FeedCollections
) {
    val rootRef = collections.rootDataSource ?: return
    val rootRows = collections.collections[rootRef].orEmpty()
    if (rootRows.isEmpty()) {
        return
    }
    val rootContext = windowContext.contextOrNull(rootRef) ?: return
    if (!shouldAutoSelectFeedRoot(rootContext)) {
        return
    }
    rootContext.setSelection(SelectionState(selected = rootRows.first(), rowIndex = 0))
}

private fun shouldAutoSelectFeedRoot(
    context: DataSourceContext
): Boolean {
    return (context.dataSource.selectionMode ?: "single") != "none" &&
        context.peekSelection().selected == null
}

internal fun computeFeedCollections(
    rawDataSources: JsonObject?,
    feedData: JsonElement?
): FeedCollections {
    val dataSources = normalizeFeedDataSources(rawDataSources)
    val rootAny = feedData?.let(JsonUtil::elementToAny)
    val rootRef = resolveRootFeedDataSource(dataSources)
    val collections = seedFeedCollections(dataSources, rootAny, rootRef)
    resolveDerivedFeedCollections(dataSources, collections)
    fillMissingFeedCollections(dataSources, collections)
    return FeedCollections(rootDataSource = rootRef, collections = collections)
}

private fun seedFeedCollections(
    dataSources: Map<String, JsonObject>,
    rootAny: Any?,
    rootRef: String?
): LinkedHashMap<String, List<Map<String, Any?>>> {
    val collections = linkedMapOf<String, List<Map<String, Any?>>>()
    dataSources.forEach { (name, def) ->
        topLevelFeedRows(def, rootAny)?.let { rows ->
            collections[name] = rows
        }
    }
    seedRootFeedCollection(collections, rootRef, rootAny)
    return collections
}

private fun topLevelFeedRows(
    def: JsonObject,
    rootAny: Any?
): List<Map<String, Any?>>? {
    if (!isTopLevelSource(def)) {
        return null
    }
    return projectFeedRows(selectPath(jsonString(def["source"]), rootAny), def)
}

private fun seedRootFeedCollection(
    collections: LinkedHashMap<String, List<Map<String, Any?>>>,
    rootRef: String?,
    rootAny: Any?
) {
    if (!rootRef.isNullOrBlank()) {
        collections.putIfAbsent(rootRef, asFeedRows(rootAny))
    }
}

private fun resolveDerivedFeedCollections(
    dataSources: Map<String, JsonObject>,
    collections: LinkedHashMap<String, List<Map<String, Any?>>>
) {
    val pending = dataSources.keys.toMutableSet()
    var changed = true
    while (pending.isNotEmpty() && changed) {
        changed = false
        val iterator = pending.iterator()
        while (iterator.hasNext()) {
            val name = iterator.next()
            when (val rows = resolveFeedCollection(name, dataSources, collections)) {
                null -> continue
                else -> {
                    collections.putIfAbsent(name, rows)
                    iterator.remove()
                    changed = true
                }
            }
        }
    }
}

private fun resolveFeedCollection(
    name: String,
    dataSources: Map<String, JsonObject>,
    collections: Map<String, List<Map<String, Any?>>>
): List<Map<String, Any?>>? {
    if (collections.containsKey(name)) {
        return collections[name]
    }
    val def = dataSources[name] ?: JsonObject(emptyMap())
    val parent = parentDataSourceRef(def)
    if (parent == null) {
        return emptyList()
    }
    val parentRows = collections[parent] ?: return null
    val selector = resolveFeedSelector(def)
    val parentRoot = feedParentRoot(parentRows)
    return projectFeedRows(selectPath(selector, parentRoot), def)
}

private fun resolveFeedSelector(def: JsonObject): String {
    return selectorsData(def) ?: "output"
}

private fun feedParentRoot(rows: List<Map<String, Any?>>): Any? {
    return rows.singleOrNull() ?: rows
}

private fun fillMissingFeedCollections(
    dataSources: Map<String, JsonObject>,
    collections: LinkedHashMap<String, List<Map<String, Any?>>>
) {
    dataSources.keys.forEach { name ->
        collections.putIfAbsent(name, emptyList())
    }
}

private fun isTopLevelSource(def: JsonObject): Boolean {
    val source = jsonString(def["source"])
    val parent = jsonString(def["dataSourceRef"])
    return source.isNotBlank() && parent.isBlank()
}

internal fun normalizeFeedDataSources(rawDataSources: JsonObject?): Map<String, JsonObject> {
    val normalized = rawDataSources.toNormalizedFeedDataSources()
    appendMissingParentDataSources(normalized)
    return normalized
}

internal fun resolveRootFeedDataSource(dataSources: Map<String, JsonObject>): String? {
    val explicit = dataSources.entries.firstOrNull { (_, value) -> isExplicitRootDataSource(value) }
    if (explicit != null) {
        return explicit.key
    }
    val firstTopLevel = dataSources.entries.firstOrNull { (_, value) -> hasNoParentDataSource(value) }
    return firstTopLevel?.key ?: dataSources.keys.firstOrNull()
}

private fun JsonObject?.toNormalizedFeedDataSources(): LinkedHashMap<String, JsonObject> {
    val normalized = linkedMapOf<String, JsonObject>()
    this.orEmpty().forEach { (name, value) ->
        normalized[name] = value as? JsonObject
            ?: error("Feed data source '$name' must be a JSON object")
    }
    return normalized
}

private fun appendMissingParentDataSources(
    dataSources: LinkedHashMap<String, JsonObject>
) {
    dataSources.values
        .mapNotNull(::parentDataSourceRef)
        .forEach { parent ->
            dataSources.putIfAbsent(parent, JsonObject(emptyMap()))
        }
}

private fun isExplicitRootDataSource(def: JsonObject): Boolean {
    val source = jsonString(def["source"]).lowercase()
    return hasNoParentDataSource(def) && (source == "output" || source == "input")
}

private fun hasNoParentDataSource(def: JsonObject): Boolean {
    return parentDataSourceRef(def) == null
}

private fun parentDataSourceRef(def: JsonObject): String? {
    return jsonString(def["dataSourceRef"]).takeIf(String::isNotBlank)
}

internal fun selectPath(selector: String?, root: Any?): Any? {
    if (selector?.trim() == "$") return root
    val tokens = parseSelectorTokens(selector)
    if (tokens.isEmpty()) {
        return root
    }
    resolveDirectFeedChannel(root, tokens)?.let { return it }
    val effectiveTokens = stripImplicitFeedChannelPrefix(root, tokens)
    return walkSelectorPath(root, effectiveTokens)
}

internal fun parseSelectorTokens(selector: String?): List<String> {
    val input = selector?.trim().orEmpty()
    if (input.isEmpty()) {
        return emptyList()
    }
    val tokens = mutableListOf<String>()
    val current = StringBuilder()
    var index = 0
    while (index < input.length) {
        when (val ch = input[index]) {
            '.' -> {
                flushSelectorToken(current, tokens)
                index++
            }
            '[' -> {
                flushSelectorToken(current, tokens)
                index = consumeBracketSelectorToken(input, index, tokens)
            }
            else -> {
                current.append(ch)
                index++
            }
        }
    }
    flushSelectorToken(current, tokens)
    return tokens
}

private fun flushSelectorToken(current: StringBuilder, tokens: MutableList<String>) {
    if (current.isEmpty()) {
        return
    }
    val token = current.toString().trim()
    if (token.isNotEmpty()) {
        tokens += token
    }
    current.clear()
}

private fun consumeBracketSelectorToken(
    input: String,
    startIndex: Int,
    tokens: MutableList<String>
): Int {
    val closingIndex = input.indexOf(']', startIndex + 1)
    if (closingIndex == -1) {
        return input.length
    }
    val token = input.substring(startIndex + 1, closingIndex).trim()
    if (token.isNotEmpty()) {
        tokens += token
    }
    return closingIndex + 1
}

private fun resolveDirectFeedChannel(root: Any?, tokens: List<String>): Any? {
    if (tokens.size != 1) {
        return null
    }
    val token = tokens.first()
    if (token != "output" && token != "input") {
        return null
    }
    return if (root is Map<*, *> && root.containsKey(token)) root[token] else root
}

private fun stripImplicitFeedChannelPrefix(root: Any?, tokens: List<String>): List<String> {
    if (root !is Map<*, *>) {
        return tokens
    }
    if (root.containsKey("output") || root.containsKey("input")) {
        return tokens
    }
    val first = tokens.firstOrNull()
    return when (first) {
        "output", "input" -> tokens.drop(1)
        else -> tokens
    }
}

private fun walkSelectorPath(root: Any?, tokens: List<String>): Any? {
    var current: Any? = root
    tokens.forEach { token ->
        current = nextSelectorValue(current, token) ?: return null
    }
    return current
}

private fun nextSelectorValue(current: Any?, token: String): Any? {
    return when (current) {
        is List<*> -> token.toIntOrNull()?.let { index ->
            if (index in current.indices) current[index] else null
        }
        is Map<*, *> -> current[token]
        else -> null
    }
}

private fun selectorsData(def: JsonObject): String? {
    val selectors = def["selectors"] as? JsonObject ?: return null
    return jsonString(selectors["data"]).takeIf { it.isNotBlank() }
}

private fun asFeedRows(value: Any?): List<Map<String, Any?>> {
    return when (value) {
        null -> emptyList()
        is List<*> -> value.mapNotNull(::toFeedRow)
        else -> listOfNotNull(toFeedRow(value))
    }
}

internal fun projectFeedRows(value: Any?, definition: JsonObject): List<Map<String, Any?>> {
    val flattened = (definition["flatten"] as? JsonObject)?.let { flattenFeedRows(value, it) }
    var rows = flattened ?: projectFeedFieldRows(value, definition["fields"] as? JsonObject)
    rows = filterFeedRows(rows, definition["exclude"])
    rows = deduplicateFeedRows(rows, definition["uniqueKey"] as? JsonArray)
    rows = deriveFeedRows(rows, definition["derive"] as? JsonObject)
    val countAs = jsonString((definition["aggregate"] as? JsonObject)?.get("countAs"))
    return if (countAs.isNotBlank()) listOf(mapOf(countAs to rows.size)) else rows
}

private fun projectFeedFieldRows(value: Any?, fields: JsonObject?): List<Map<String, Any?>> {
    if (fields == null) return asFeedRows(value)
    return when (value) {
        is List<*> -> value.map { item -> projectFeedFields(item, fields) }
        else -> listOf(projectFeedFields(value, fields))
    }
}

internal fun projectFeedFields(root: Any?, fields: JsonObject): Map<String, Any?> {
    val result = linkedMapOf<String, Any?>()
    fields.forEach { (name, raw) ->
        val config = raw as? JsonObject
        val path = when (raw) {
            is JsonPrimitive -> raw.content
            else -> jsonString(config?.get("path")).ifBlank { jsonString(config?.get("selector")).ifBlank { name } }
        }
        val transform = jsonString(config?.get("transform")).lowercase()
        var selected: Any? = selectPath(path, root)
        selected = when (transform) {
            "daterange", "daterangelabel" -> {
                val start = projectFeedDate(selectPath(jsonString(config?.get("startPath")).ifBlank { "start" }, root))
                val end = projectFeedDate(selectPath(jsonString(config?.get("endPath")).ifBlank { "end" }, root))
                if (transform == "daterangelabel") listOf(start, end).filter(String::isNotBlank).joinToString(" – ")
                else mapOf("start" to start, "end" to end)
            }
            "dateparts" -> projectFeedDate(selected)
            "boolean" -> when (selected) {
                true, 1, 1L, "1" -> true
                is String -> selected.equals("true", ignoreCase = true)
                else -> false
            }
            else -> selected
        }
        result[name] = selected
    }
    return result
}

private fun projectFeedDate(value: Any?): String {
    val map = value as? Map<*, *> ?: return value?.toString().orEmpty()
    val year = (map["year"] as? Number)?.toInt() ?: return ""
    val month = (map["month"] as? Number)?.toInt()
        ?: (map["monthIndex"] as? Number)?.toInt()?.plus(1)
        ?: return ""
    val day = (map["day"] as? Number)?.toInt() ?: return ""
    return "%04d-%02d-%02d".format(year, month, day)
}

private fun flattenFeedRows(value: Any?, config: JsonObject): List<Map<String, Any?>> {
    val sources = config["sources"] as? JsonArray ?: return emptyList()
    val output = mutableListOf<Map<String, Any?>>()
    asAnyList(value).forEach { parent ->
        sources.forEach sourceLoop@{ sourceValue ->
            val source = sourceValue as? JsonObject ?: return@sourceLoop
            val children = asAnyList(selectPath(jsonString(source["path"]), parent))
            children.forEach childLoop@{ child ->
                if (child == null || feedRowExcluded(child, source["exclude"])) return@childLoop
                val row: MutableMap<String, Any?> = when {
                    source["fields"] is JsonObject ->
                        projectFeedFields(child, source["fields"] as JsonObject).toMutableMap()
                    child is Map<*, *> -> child.entries.associate { it.key.toString() to it.value }.toMutableMap()
                    else -> mutableMapOf("value" to child)
                }
                (source["parentFields"] as? JsonObject)?.forEach { (field, path) ->
                    row[field] = selectPath(jsonString(path), parent)
                }
                (source["values"] as? JsonObject)?.forEach { (field, constant) ->
                    row[field] = JsonUtil.elementToAny(constant)
                }
                output += row
            }
        }
    }
    return output
}

private fun filterFeedRows(rows: List<Map<String, Any?>>, exclude: JsonElement?): List<Map<String, Any?>> {
    val rules = when (exclude) {
        is JsonArray -> exclude.toList()
        is JsonObject -> listOf(exclude)
        else -> emptyList()
    }
    if (rules.isEmpty()) return rows
    return rows.filterNot { row -> rules.any { feedRowExcluded(row, it) } }
}

internal fun feedRowExcluded(row: Any?, rawRule: JsonElement?): Boolean {
    val rule = rawRule as? JsonObject ?: return false
    val path = jsonString(rule["field"]).ifBlank { jsonString(rule["path"]) }
    val actual = selectPath(path, row)
    rule["equalsIgnoreCase"]?.let { expected ->
        return actual?.toString()?.trim()?.lowercase() == jsonString(expected).lowercase()
    }
    if ("equals" in rule) return actual == JsonUtil.elementToAny(rule["equals"] ?: JsonNull)
    return false
}

private fun deduplicateFeedRows(rows: List<Map<String, Any?>>, rawKeys: JsonArray?): List<Map<String, Any?>> {
    val keys = rawKeys.orEmpty().mapNotNull { key ->
        when (key) {
            is JsonObject -> jsonString(key["field"])
            is JsonPrimitive -> key.content.trim()
            else -> ""
        }.takeIf(String::isNotBlank)
    }
    if (keys.isEmpty()) return rows
    val seen = mutableSetOf<List<Any?>>()
    return rows.filter { row -> seen.add(keys.map(row::get)) }
}

private fun deriveFeedRows(rows: List<Map<String, Any?>>, derive: JsonObject?): List<Map<String, Any?>> {
    if (derive == null) return rows
    val expression = Regex("\\$\\{([^}]+)}")
    return rows.map { source ->
        val row = source.toMutableMap()
        derive.forEach { (field, templateValue) ->
            val template = jsonString(templateValue)
            row[field] = expression.replace(template) { match ->
                selectPath(match.groupValues[1].trim(), row)?.toString().orEmpty()
            }
        }
        row
    }
}

internal fun asAnyList(value: Any?): List<Any?> = when (value) {
    null -> emptyList()
    is List<*> -> value
    else -> listOf(value)
}

private fun toFeedRow(value: Any?): Map<String, Any?>? {
    return when (value) {
        null -> null
        is Map<*, *> -> value.entries.associate { it.key.toString() to it.value }
        else -> mapOf("value" to value)
    }
}

private fun jsonString(value: JsonElement?): String {
    return (value as? JsonPrimitive)?.content?.trim().orEmpty()
}
