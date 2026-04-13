package com.viant.agently.android

import com.viant.agentlysdk.FeedDataResponse
import com.viant.forgeandroid.runtime.ContainerDef
import com.viant.forgeandroid.runtime.ContentDef
import com.viant.forgeandroid.runtime.DataSourceDef
import com.viant.forgeandroid.runtime.DataSourceContext
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.JsonUtil
import com.viant.forgeandroid.runtime.SelectionState
import com.viant.forgeandroid.runtime.ViewDef
import com.viant.forgeandroid.runtime.WindowContext
import com.viant.forgeandroid.runtime.WindowMetadata
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

internal val feedRuntimeJson = Json { ignoreUnknownKeys = true }

internal data class FeedCollections(
    val rootDataSource: String?,
    val collections: Map<String, List<Map<String, Any?>>>
)

internal fun buildFeedWindowMetadata(payload: FeedDataResponse): WindowMetadata {
    val dataSources = decodeFeedDataSources(payload.dataSources)
    val ui = payload.ui ?: error("Feed ${payload.feedId ?: payload.title ?: "unknown"} is missing ui metadata")
    val content = decodeFeedContent(ui)
    return WindowMetadata(
        namespace = "agently.android.feed",
        dataSources = dataSources,
        view = ViewDef(
            content = content
        )
    )
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

internal fun wireFeedWindow(runtime: ForgeRuntime, windowId: String, payload: FeedDataResponse) {
    val windowContext = runtime.windowContext(windowId)
    val collections = computeFeedCollections(payload.dataSources, payload.data)
    hydrateFeedDataSources(windowContext, collections.collections)
    selectInitialFeedRoot(windowContext, collections)
}

private fun hydrateFeedDataSources(
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
    return asFeedRows(selectPath(jsonString(def["source"]), rootAny))
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
    return asFeedRows(selectPath(selector, parentRoot))
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
    val tokens = parseSelectorTokens(selector)
    if (tokens.isEmpty()) {
        return root
    }
    resolveDirectFeedChannel(root, tokens)?.let { return it }
    val effectiveTokens = stripImplicitFeedChannelPrefix(root, tokens)
    return walkSelectorPath(root, effectiveTokens)
}

private fun parseSelectorTokens(selector: String?): List<String> {
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
