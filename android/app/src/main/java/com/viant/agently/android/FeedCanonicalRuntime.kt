package com.viant.agently.android

import com.viant.agentlysdk.FeedDataResponse
import com.viant.forgeandroid.runtime.FeedPatchOperation
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.JsonUtil
import com.viant.forgeandroid.runtime.SelectionState
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

internal object AndroidFeedCanonicalRegistry {
    private data class Key(val runtime: ForgeRuntime, val windowId: String)
    private data class State(
        var dataSources: JsonObject?,
        var canonical: JsonElement,
        var turnId: String?,
        var dirty: Boolean = false
    )

    private val states = mutableMapOf<Key, State>()

    fun register(runtime: ForgeRuntime, windowId: String, payload: FeedDataResponse, turnId: String? = null): JsonElement {
        val key = Key(runtime, windowId)
        val incomingTurn = turnId?.trim()?.takeIf(String::isNotEmpty)
        val existing = states[key]
        if (existing != null && existing.dirty && (incomingTurn == null || incomingTurn == existing.turnId)) {
            existing.dataSources = payload.dataSources ?: existing.dataSources
            return existing.canonical
        }
        val state = State(
            dataSources = payload.dataSources,
            canonical = payload.data ?: JsonNull,
            turnId = incomingTurn
        )
        states[key] = state
        return state.canonical
    }

    fun clear(runtime: ForgeRuntime, windowId: String) {
        states.remove(Key(runtime, windowId))
    }

    fun has(runtime: ForgeRuntime, windowId: String): Boolean = states.containsKey(Key(runtime, windowId))

    fun apply(
        forgeRuntime: ForgeRuntime,
        windowId: String,
        operations: List<FeedPatchOperation>,
        turnId: String? = null
    ): Set<String> {
        val state = states[Key(forgeRuntime, windowId)] ?: return emptySet()
        val definitions = normalizeFeedDataSources(state.dataSources)
        var canonical = state.canonical
        val canonicalOperations = mutableListOf<Pair<List<String>, FeedPatchOperation>>()
        val directSelectionOperations = mutableListOf<FeedPatchOperation>()
        val synchronizedRefs = mutableSetOf<String>()

        operations.forEach { operation ->
            val definition = definitions[operation.dataSourceRef]
                ?: throw IllegalArgumentException("unknown dataSourceRef: ${operation.dataSourceRef}")
            val viewTokens = parseFeedPointer(operation.path)
            require(viewTokens.isNotEmpty()) { "feed patch path must select a datasource view" }
            val rootPath = canonicalDataSourcePath(
                operation.dataSourceRef,
                definitions,
                canonical
            )
            val relative = when (viewTokens.first()) {
                "form", "collection" -> viewTokens.drop(1)
                "selection" -> selectionCanonicalRelativePath(
                    forgeRuntime,
                    windowId,
                    operation.dataSourceRef,
                    definition,
                    viewTokens.drop(1)
                ) ?: run {
                    directSelectionOperations += operation
                    return@forEach
                }
                else -> throw IllegalArgumentException("unsupported feed patch view: ${viewTokens.first()}")
            }
            if (synchronizedRefs.add(operation.dataSourceRef)) {
                canonical = synchronizeCurrentDataSourceView(
                    canonical,
                    rootPath,
                    forgeRuntime,
                    windowId,
                    operation.dataSourceRef,
                    definition
                )
            }
            val canonicalRoot = valueAtCanonicalPath(canonical, rootPath)
                ?: throw IllegalArgumentException("feed datasource root does not exist: ${operation.dataSourceRef}")
            val flatten = definition["flatten"] as? JsonObject
            if (operation.op == "add" && relative.firstOrNull() == "-" && flatten != null) {
                val (mappedRelative, rawValue) = mapFlattenedFeedAdd(
                    flatten,
                    operation.value,
                    canonicalRoot
                )
                canonicalOperations += (rootPath + mappedRelative) to operation.copy(value = rawValue)
            } else {
                val mappedRelative = mapProjectedFeedRelativePath(
                    definition = definition,
                    viewRelative = relative,
                    canonicalRoot = canonicalRoot,
                    currentRows = forgeRuntime.windowContext(windowId)
                        .contextOrNull(operation.dataSourceRef)?.collection?.peek().orEmpty()
                )
                canonicalOperations += (rootPath + mappedRelative) to operation
            }
        }

        canonicalOperations.forEach { (path, operation) ->
            canonical = patchCanonicalJSON(canonical, path, operation)
        }
        if (canonicalOperations.isNotEmpty()) {
            state.canonical = canonical
            state.dirty = true
            turnId?.trim()?.takeIf(String::isNotEmpty)?.let { state.turnId = it }
            rehydrateCanonicalFeed(forgeRuntime, windowId, state, definitions)
        }
        if (directSelectionOperations.isNotEmpty()) {
            com.viant.forgeandroid.runtime.applyFeedPatchOperations(
                forgeRuntime.windowContext(windowId),
                directSelectionOperations
            )
        }
        return if (canonicalOperations.isNotEmpty()) {
            definitions.keys + directSelectionOperations.map { it.dataSourceRef }
        } else {
            directSelectionOperations.mapTo(linkedSetOf()) { it.dataSourceRef }
        }
    }

    private fun rehydrateCanonicalFeed(
        forgeRuntime: ForgeRuntime,
        windowId: String,
        state: State,
        definitions: Map<String, JsonObject>
    ) {
        val windowContext = forgeRuntime.windowContext(windowId)
        val previousSelections = definitions.keys.associateWith { ref ->
            windowContext.contextOrNull(ref)?.peekSelection()
        }
        val collections = computeFeedCollections(state.dataSources, state.canonical)
        hydrateFeedDataSources(windowContext, collections.collections)
        definitions.forEach { (ref, definition) ->
            val context = windowContext.contextOrNull(ref) ?: return@forEach
            val previous = previousSelections[ref] ?: return@forEach
            val rows = collections.collections[ref].orEmpty()
            context.setSelection(reconcileSelection(previous, rows, uniqueKeyFields(definition)))
        }
    }
}

private fun mapFlattenedFeedAdd(
    flatten: JsonObject,
    projectedValue: Any?,
    canonicalRoot: JsonElement
): Pair<List<String>, Any?> {
    val projected = projectedValue as? Map<*, *>
        ?: throw IllegalArgumentException("flattened add requires an object value")
    val projectedRow = projected.entries.associate { it.key.toString() to it.value }
    val parentValue = JsonUtil.elementToAny(canonicalRoot)
    val parents = asAnyList(parentValue)
    val parentIsArray = parentValue is List<*>
    val sources = flatten["sources"] as? JsonArray ?: throw IllegalArgumentException("flatten.sources are required")
    val parentCandidates: List<Pair<Int?, Any?>> = when {
        parentIsArray && parents.isEmpty() -> listOf(null to null)
        parentIsArray -> parents.mapIndexed { index, parent -> index to parent }
        else -> listOf(null to parentValue)
    }
    for ((parentIndex, parent) in parentCandidates) {
        for (sourceValue in sources) {
            val source = sourceValue as? JsonObject ?: continue
            val constantsMatch = (source["values"] as? JsonObject).orEmpty().all { (field, value) ->
                projectedRow[field] == JsonUtil.elementToAny(value)
            }
            val parentMatches = (source["parentFields"] as? JsonObject).orEmpty().all { (field, path) ->
                projectedRow[field] == selectPath(jsonText(path), parent)
            }
            if (!constantsMatch || !parentMatches) continue
            val sourcePath = jsonText(source["path"])
            val fields = source["fields"] as? JsonObject
            val rawValue: Any? = if (fields != null) {
                if (fields.size == 1 && jsonText(fields.values.first()) == "$") {
                    projectedRow[fields.keys.first()]
                } else {
                    var raw: Any? = linkedMapOf<String, Any?>()
                    fields.forEach { (viewField, mapping) ->
                        if (projectedRow.containsKey(viewField)) {
                            raw = setAnyPath(raw, parseSelectorTokens(jsonText(mapping)), projectedRow[viewField])
                        }
                    }
                    raw
                }
            } else {
                projectedRow.filterKeys { field ->
                    (source["values"] as? JsonObject)?.containsKey(field) != true &&
                        (source["parentFields"] as? JsonObject)?.containsKey(field) != true
                }
            }
            val path = buildList {
                if (parentIndex != null) add(parentIndex.toString())
                if (sourcePath.isNotBlank() && sourcePath != "$") addAll(parseSelectorTokens(sourcePath))
                add("-")
            }
            return path to rawValue
        }
    }
    throw IllegalArgumentException("flattened add does not match a declared parent/source")
}

private fun setAnyPath(root: Any?, tokens: List<String>, value: Any?): Any? {
    if (tokens.isEmpty()) return value
    val map = (root as? Map<*, *>)?.entries?.associateTo(linkedMapOf()) { it.key.toString() to it.value }
        ?: linkedMapOf()
    val token = tokens.first()
    map[token] = setAnyPath(map[token], tokens.drop(1), value)
    return map
}

private fun mapProjectedFeedRelativePath(
    definition: JsonObject,
    viewRelative: List<String>,
    canonicalRoot: JsonElement,
    currentRows: List<Map<String, Any?>>
): List<String> {
    require((definition["aggregate"] as? JsonObject)?.get("countAs") == null) {
        "aggregate feed datasources are read-only"
    }
    (definition["flatten"] as? JsonObject)?.let { flatten ->
        return mapFlattenedFeedRelativePath(
            definition,
            flatten,
            viewRelative,
            canonicalRoot,
            currentRows
        )
    }
    if (canonicalRoot is JsonArray && viewRelative.isNotEmpty()) {
        if (viewRelative.first() == "-" && definition["fields"] == null && definition["exclude"] == null && definition["derive"] == null) {
            return viewRelative
        }
        val viewIndex = viewRelative.first().toIntOrNull()
            ?: throw IllegalArgumentException("feed collection path requires a row index")
        val target = currentRows.getOrNull(viewIndex)
            ?: throw IllegalArgumentException("feed collection index out of bounds: $viewIndex")
        val rawValues = canonicalRoot.map(JsonUtil::elementToAny)
        val rawIndex = rawValues.indexOfFirst { raw ->
            val projected = projectFeedRows(raw, definition).singleOrNull() ?: return@indexOfFirst false
            feedRowsMatch(projected, target, uniqueKeyFields(definition))
        }.takeIf { it >= 0 }
            ?: throw IllegalArgumentException("feed collection row no longer exists in canonical data")
        return listOf(rawIndex.toString()) + mapProjectedFeedFieldPath(definition["fields"] as? JsonObject, viewRelative.drop(1), definition)
    }
    return mapProjectedFeedFieldPath(definition["fields"] as? JsonObject, viewRelative, definition)
}

private fun mapProjectedFeedFieldPath(
    fields: JsonObject?,
    relative: List<String>,
    definition: JsonObject
): List<String> {
    if (relative.isEmpty()) return relative
    val field = relative.first()
    require((definition["derive"] as? JsonObject)?.containsKey(field) != true) {
        "derived feed field is read-only: $field"
    }
    val raw = fields?.get(field) ?: return relative
    val config = raw as? JsonObject
    val transform = jsonText(config?.get("transform")).lowercase()
    val remainder = relative.drop(1)
    val path = when {
        transform == "daterange" && remainder.firstOrNull() == "start" ->
            jsonText(config?.get("startPath")).ifBlank { "start" }
        transform == "daterange" && remainder.firstOrNull() == "end" ->
            jsonText(config?.get("endPath")).ifBlank { "end" }
        raw is JsonPrimitive -> raw.content
        else -> jsonText(config?.get("path")).ifBlank { jsonText(config?.get("selector")).ifBlank { field } }
    }
    val consumed = if (transform == "daterange" && remainder.firstOrNull() in setOf("start", "end")) 1 else 0
    val mapped = if (path.trim() == "$") emptyList() else parseSelectorTokens(path)
    return mapped + remainder.drop(consumed)
}

private fun mapFlattenedFeedRelativePath(
    definition: JsonObject,
    flatten: JsonObject,
    viewRelative: List<String>,
    canonicalRoot: JsonElement,
    currentRows: List<Map<String, Any?>>
): List<String> {
    val viewIndex = viewRelative.firstOrNull()?.toIntOrNull()
        ?: throw IllegalArgumentException("flattened feed path requires a row index")
    val target = currentRows.getOrNull(viewIndex)
        ?: throw IllegalArgumentException("flattened feed index out of bounds: $viewIndex")
    val parentValue = JsonUtil.elementToAny(canonicalRoot)
    val parents = asAnyList(parentValue)
    val parentIsArray = parentValue is List<*>
    val sources = flatten["sources"] as? JsonArray ?: throw IllegalArgumentException("flatten.sources are required")
    for ((parentIndex, parent) in parents.withIndex()) {
        for (sourceValue in sources) {
            val source = sourceValue as? JsonObject ?: continue
            val sourcePath = jsonText(source["path"])
            val selectedChildren = selectPath(sourcePath, parent)
            val children = asAnyList(selectedChildren)
            val childIsArray = selectedChildren is List<*>
            for ((childIndex, child) in children.withIndex()) {
                if (child == null || feedRowExcluded(child, source["exclude"])) continue
                val projected = flattenedProjectedRow(parent, child, source)
                if (!feedRowsMatch(projected, target, uniqueKeyFields(definition))) continue
                val field = viewRelative.getOrNull(1)
                require(field == null || (source["parentFields"] as? JsonObject)?.containsKey(field) != true) {
                    "flattened parent field is read-only: $field"
                }
                require(field == null || (source["values"] as? JsonObject)?.containsKey(field) != true) {
                    "flattened constant field is read-only: $field"
                }
                val prefix = buildList {
                    if (parentIsArray) add(parentIndex.toString())
                    if (sourcePath.trim() != "$" && sourcePath.isNotBlank()) addAll(parseSelectorTokens(sourcePath))
                    if (childIsArray) add(childIndex.toString())
                }
                return prefix + mapProjectedFeedFieldPath(
                    source["fields"] as? JsonObject,
                    viewRelative.drop(1),
                    definition
                )
            }
        }
    }
    throw IllegalArgumentException("flattened feed row no longer exists in canonical data")
}

private fun flattenedProjectedRow(parent: Any?, child: Any?, source: JsonObject): Map<String, Any?> {
    val row: MutableMap<String, Any?> = when {
        source["fields"] is JsonObject ->
            projectFeedFields(child, source["fields"] as JsonObject).toMutableMap()
        child is Map<*, *> -> child.entries.associate { it.key.toString() to it.value }.toMutableMap()
        else -> mutableMapOf("value" to child)
    }
    (source["parentFields"] as? JsonObject)?.forEach { (field, path) ->
        row[field] = selectPath(jsonText(path), parent)
    }
    (source["values"] as? JsonObject)?.forEach { (field, value) ->
        row[field] = JsonUtil.elementToAny(value)
    }
    return row
}

private fun feedRowsMatch(
    candidate: Map<String, Any?>,
    target: Map<String, Any?>,
    uniqueKeys: List<String>
): Boolean = if (uniqueKeys.isNotEmpty()) {
    uniqueKeys.all { candidate[it] == target[it] }
} else {
    candidate == target
}

private fun canonicalDataSourcePath(
    ref: String,
    definitions: Map<String, JsonObject>,
    canonical: JsonElement,
    visiting: MutableSet<String> = mutableSetOf()
): List<String> {
    require(visiting.add(ref)) { "cyclic feed datasource dependency: $ref" }
    val definition = definitions[ref] ?: throw IllegalArgumentException("unknown dataSourceRef: $ref")
    val parent = jsonText(definition["dataSourceRef"])
    val path = if (parent.isBlank()) {
        parseSelectorTokens(jsonText(definition["source"]))
    } else {
        val parentPath = canonicalDataSourcePath(parent, definitions, canonical, visiting)
        val selectorTokens = parseSelectorTokens(feedSelector(definition))
        val parentValue = valueAtCanonicalPath(canonical, parentPath)
        val effectiveSelector = if (
            selectorTokens.size == 1 && selectorTokens.first() in setOf("output", "input") &&
            parentValue is JsonObject && selectorTokens.first() !in parentValue
        ) emptyList() else selectorTokens
        parentPath + effectiveSelector
    }
    visiting.remove(ref)
    if (canonical is JsonObject && "output" !in canonical && "input" !in canonical && path.firstOrNull() in setOf("output", "input")) {
        return path.drop(1)
    }
    return path
}

private fun feedSelector(definition: JsonObject): String {
    val selectors = definition["selectors"] as? JsonObject
    return jsonText(selectors?.get("data")).ifBlank { "output" }
}

private fun synchronizeCurrentDataSourceView(
    canonical: JsonElement,
    rootPath: List<String>,
    forgeRuntime: ForgeRuntime,
    windowId: String,
    ref: String,
    definition: JsonObject
): JsonElement {
    val context = forgeRuntime.windowContext(windowId).contextOrNull(ref) ?: return canonical
    val currentRoot = valueAtCanonicalPath(canonical, rootPath)
    val fields = definition["fields"] as? JsonObject
    if (currentRoot is JsonObject && fields != null) {
        var synchronized = canonical
        context.peekForm().forEach { (field, value) ->
            val config = fields[field] as? JsonObject
            val transform = jsonText(config?.get("transform")).lowercase()
            if (transform == "daterange" && value is Map<*, *>) {
                listOf("start" to "startPath", "end" to "endPath").forEach { (viewField, pathField) ->
                    val mapped = parseSelectorTokens(jsonText(config?.get(pathField)).ifBlank { viewField })
                    synchronized = replaceCanonicalJSON(synchronized, rootPath + mapped, value[viewField].toJsonElementValue())
                }
            } else if (transform != "daterangelabel" && (definition["derive"] as? JsonObject)?.containsKey(field) != true) {
                val mapped = mapProjectedFeedFieldPath(fields, listOf(field), definition)
                synchronized = replaceCanonicalJSON(synchronized, rootPath + mapped, value.toJsonElementValue())
            }
        }
        return synchronized
    }
    if (definition["flatten"] != null || definition["exclude"] != null || definition["aggregate"] != null || definition["derive"] != null) {
        return canonical
    }
    val replacement = when (currentRoot) {
        is JsonArray -> context.collection.peek().toJsonElementValue()
        is JsonObject -> context.peekForm().takeIf { it.isNotEmpty() }?.toJsonElementValue()
            ?: context.collection.peek().singleOrNull()?.toJsonElementValue()
        else -> null
    } ?: return canonical
    return replaceCanonicalJSON(canonical, rootPath, replacement)
}

private fun selectionCanonicalRelativePath(
    forgeRuntime: ForgeRuntime,
    windowId: String,
    ref: String,
    definition: JsonObject,
    relative: List<String>
): List<String>? {
    val context = forgeRuntime.windowContext(windowId).contextOrNull(ref) ?: return null
    val selection = context.peekSelection()
    val (selectedRow, remainder) = when (relative.firstOrNull()) {
        "selection" -> {
            val selectedIndex = relative.getOrNull(1)?.toIntOrNull() ?: return null
            selection.selection.getOrNull(selectedIndex) to relative.drop(2)
        }
        "selected" -> selection.selected to relative.drop(1)
        else -> return null
    }
    val row = selectedRow ?: return null
    val collectionIndex = findRowIndex(context.collection.peek(), row, uniqueKeyFields(definition))
    return collectionIndex?.let { listOf(it.toString()) + remainder }
}

private fun uniqueKeyFields(definition: JsonObject): List<String> {
    return (definition["uniqueKey"] as? JsonArray).orEmpty().mapNotNull { entry ->
        jsonText((entry as? JsonObject)?.get("field")).takeIf(String::isNotBlank)
    }
}

private fun reconcileSelection(
    previous: SelectionState,
    rows: List<Map<String, Any?>>,
    uniqueKeys: List<String>
): SelectionState {
    fun resolve(row: Map<String, Any?>?): Map<String, Any?>? {
        if (row == null) return null
        return findRowIndex(rows, row, uniqueKeys)?.let(rows::get)
    }
    val selectedRows = previous.selection.mapNotNull(::resolve)
    val selected = resolve(previous.selected) ?: selectedRows.lastOrNull()
    val rowIndex = selected?.let { rows.indexOf(it) } ?: -1
    return SelectionState(selected = selected, selection = selectedRows, rowIndex = rowIndex)
}

private fun findRowIndex(
    rows: List<Map<String, Any?>>,
    target: Map<String, Any?>,
    uniqueKeys: List<String>
): Int? {
    val index = if (uniqueKeys.isNotEmpty()) {
        rows.indexOfFirst { row -> uniqueKeys.all { key -> row[key] == target[key] } }
    } else {
        rows.indexOf(target)
    }
    return index.takeIf { it >= 0 }
}

private fun parseFeedPointer(path: String): List<String> {
    require(path.startsWith('/')) { "feed patch path must be an absolute JSON Pointer" }
    return path.split('/').drop(1).map { it.replace("~1", "/").replace("~0", "~") }
}

private fun patchCanonicalJSON(
    current: JsonElement,
    tokens: List<String>,
    operation: FeedPatchOperation
): JsonElement {
    if (tokens.isEmpty()) {
        require(operation.op != "remove") { "cannot remove canonical feed root" }
        require(operation.op == "add" || operation.op == "replace") { "unsupported feed patch op: ${operation.op}" }
        return operation.value.toJsonElementValue()
    }
    val token = tokens.first()
    val remaining = tokens.drop(1)
    return when (current) {
        is JsonObject -> {
            val values = current.toMutableMap()
            if (remaining.isEmpty()) {
                when (operation.op) {
                    "add" -> values[token] = operation.value.toJsonElementValue()
                    "replace" -> {
                        require(token in values) { "feed replace path does not exist: $token" }
                        values[token] = operation.value.toJsonElementValue()
                    }
                    "remove" -> require(values.remove(token) != null) { "feed remove path does not exist: $token" }
                    else -> throw IllegalArgumentException("unsupported feed patch op: ${operation.op}")
                }
            } else {
                val child = values[token] ?: throw IllegalArgumentException("feed patch path does not exist: $token")
                values[token] = patchCanonicalJSON(child, remaining, operation)
            }
            JsonObject(values)
        }
        is JsonArray -> {
            val values = current.toMutableList()
            if (remaining.isEmpty()) {
                when (operation.op) {
                    "add" -> {
                        val index = canonicalArrayIndex(token, values.size, true)
                        values.add(index, operation.value.toJsonElementValue())
                    }
                    "replace" -> values[canonicalArrayIndex(token, values.size, false)] = operation.value.toJsonElementValue()
                    "remove" -> values.removeAt(canonicalArrayIndex(token, values.size, false))
                    else -> throw IllegalArgumentException("unsupported feed patch op: ${operation.op}")
                }
            } else {
                val index = canonicalArrayIndex(token, values.size, false)
                values[index] = patchCanonicalJSON(values[index], remaining, operation)
            }
            JsonArray(values)
        }
        else -> throw IllegalArgumentException("feed patch path traverses a scalar: $token")
    }
}

private fun replaceCanonicalJSON(current: JsonElement, tokens: List<String>, value: JsonElement): JsonElement {
    if (tokens.isEmpty()) return value
    val token = tokens.first()
    val remaining = tokens.drop(1)
    return when (current) {
        is JsonObject -> JsonObject(current.toMutableMap().apply {
            val child = this[token] ?: throw IllegalArgumentException("feed path does not exist: $token")
            this[token] = replaceCanonicalJSON(child, remaining, value)
        })
        is JsonArray -> JsonArray(current.toMutableList().apply {
            val index = canonicalArrayIndex(token, size, false)
            this[index] = replaceCanonicalJSON(this[index], remaining, value)
        })
        else -> throw IllegalArgumentException("feed path traverses a scalar: $token")
    }
}

private fun valueAtCanonicalPath(current: JsonElement, tokens: List<String>): JsonElement? {
    var value: JsonElement = current
    for (token in tokens) {
        value = when (val currentValue = value) {
            is JsonObject -> currentValue[token] ?: return null
            is JsonArray -> currentValue.getOrNull(token.toIntOrNull() ?: return null) ?: return null
            else -> return null
        }
    }
    return value
}

private fun canonicalArrayIndex(token: String, size: Int, allowEnd: Boolean): Int {
    if (allowEnd && token == "-") return size
    val index = token.toIntOrNull() ?: throw IllegalArgumentException("invalid feed array index: $token")
    require(if (allowEnd) index in 0..size else index in 0 until size) { "feed array index out of bounds: $index" }
    return index
}

private fun jsonText(value: JsonElement?): String = (value as? JsonPrimitive)?.content?.trim().orEmpty()

private fun Any?.toJsonElementValue(): JsonElement = when (this) {
    null -> JsonNull
    is JsonElement -> this
    is String -> JsonPrimitive(this)
    is Boolean -> JsonPrimitive(this)
    is Byte -> JsonPrimitive(toInt())
    is Short -> JsonPrimitive(toInt())
    is Int -> JsonPrimitive(this)
    is Long -> JsonPrimitive(this)
    is Float -> JsonPrimitive(this)
    is Double -> JsonPrimitive(this)
    is Number -> JsonPrimitive(toDouble())
    is Map<*, *> -> JsonObject(entries.associate { it.key.toString() to it.value.toJsonElementValue() })
    is Iterable<*> -> JsonArray(map { it.toJsonElementValue() })
    is Array<*> -> JsonArray(map { it.toJsonElementValue() })
    else -> JsonPrimitive(toString())
}
