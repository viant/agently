package com.viant.agently.android

import com.viant.agentlysdk.LookupRegistryEntry
import com.viant.agentlysdk.LookupTokens
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.longOrNull
import java.util.Locale

internal data class ComposerLookupOccurrence(
    val key: String,
    val name: String,
    val title: String,
    val required: Boolean,
    val displayRange: IntRange,
    val entry: LookupRegistryEntry
)

internal data class ComposerLookupSelection(
    val token: String,
    val label: String
)

internal class ComposerLookupUnresolvedRequiredException(title: String) :
    IllegalStateException("Resolve required lookup $title before sending.")

internal data class ComposerLookupSubmissionResolution(
    val resolvedQuery: String?,
    val unresolvedRequiredLookup: ComposerLookupOccurrence?
)

internal fun resolveComposerLookupSubmission(
    query: String,
    registry: List<LookupRegistryEntry>,
    selections: Map<String, ComposerLookupSelection>
): ComposerLookupSubmissionResolution {
    val occurrences = parseComposerLookupOccurrences(query, registry)
    val unresolved = firstUnresolvedRequiredComposerLookup(occurrences, selections)
    if (unresolved != null) {
        return ComposerLookupSubmissionResolution(
            resolvedQuery = null,
            unresolvedRequiredLookup = unresolved
        )
    }
    return ComposerLookupSubmissionResolution(
        resolvedQuery = resolveComposerQuery(query, registry, selections),
        unresolvedRequiredLookup = null
    )
}

internal fun parseComposerLookupOccurrences(
    query: String,
    registry: List<LookupRegistryEntry>
): List<ComposerLookupOccurrence> {
    if (query.isEmpty() || registry.isEmpty()) {
        return emptyList()
    }
    val registryByName = registry.associateBy { it.name.lowercase(Locale.US) }
    val counters = mutableMapOf<String, Int>()
    return Regex("""/([a-zA-Z][a-zA-Z0-9_-]*)\b""").findAll(query).mapNotNull { match ->
        val rawName = match.groupValues[1].lowercase(Locale.US)
        val entry = registryByName[rawName] ?: return@mapNotNull null
        val occurrence = counters[rawName] ?: 0
        counters[rawName] = occurrence + 1
        ComposerLookupOccurrence(
            key = "$rawName#$occurrence",
            name = rawName,
            title = composerLookupTitle(entry),
            required = entry.required == true,
            displayRange = match.range,
            entry = entry
        )
    }.toList()
}

internal fun resolveComposerQuery(
    query: String,
    registry: List<LookupRegistryEntry>,
    selections: Map<String, ComposerLookupSelection>
): String {
    val occurrences = parseComposerLookupOccurrences(query, registry)
    if (occurrences.isEmpty()) {
        return query
    }
    val stored = StringBuilder(query)
    occurrences.asReversed().forEach { occurrence ->
        val selection = selections[occurrence.key]
        if (selection == null) {
            if (occurrence.required) {
                throw ComposerLookupUnresolvedRequiredException(occurrence.title)
            }
            return@forEach
        }
        stored.replace(occurrence.displayRange.first, occurrence.displayRange.last + 1, selection.token)
    }
    return LookupTokens.flattenStored(stored.toString(), registry)
}

internal fun pruneComposerLookupSelections(
    query: String,
    registry: List<LookupRegistryEntry>,
    selections: Map<String, ComposerLookupSelection>
): Map<String, ComposerLookupSelection> {
    val validKeys = parseComposerLookupOccurrences(query, registry).mapTo(mutableSetOf()) { it.key }
    return selections.filterKeys { it in validKeys }
}

internal fun composerLookupSelection(
    occurrence: ComposerLookupOccurrence,
    row: Map<String, JsonElement>
): ComposerLookupSelection {
    val token = LookupTokens.serializeToken(occurrence.entry, row.mapValues { jsonElementToLookupValue(it.value) })
    val label = LookupTokens.parseTokens(token).firstOrNull()?.label ?: occurrence.title
    return ComposerLookupSelection(token = token, label = label)
}

internal fun composerLookupRowLabel(row: Map<String, JsonElement>, entry: LookupRegistryEntry): String {
    val template = entry.token?.display?.trim().takeUnless { it.isNullOrBlank() }
        ?: entry.display?.trim().takeUnless { it.isNullOrBlank() }
        ?: "\${name}"
    val rendered = composerLookupApplyTemplate(template, row)
    if (rendered.isNotBlank()) {
        return rendered
    }
    return jsonElementDisplayString(row["name"])
        ?: jsonElementDisplayString(row["id"])
        ?: "Select"
}

internal fun composerLookupRowSecondaryText(row: Map<String, JsonElement>): String? {
    val group = jsonElementDisplayString(row["groupName"]).orEmpty()
    val identifier = jsonElementDisplayString(row["entityId"])
        ?: jsonElementDisplayString(row["id"])
        ?: ""
    return listOf(group, identifier)
        .filter { it.isNotBlank() }
        .joinToString(" • ")
        .takeIf { it.isNotBlank() }
}

private fun composerLookupTitle(entry: LookupRegistryEntry): String {
    entry.title?.trim()?.takeIf { it.isNotBlank() }?.let { return it }
    return entry.name
        .replace("_", " ")
        .replace("-", " ")
        .trim()
        .split(Regex("\\s+"))
        .filter { it.isNotBlank() }
        .joinToString(" ") { token ->
            token.lowercase(Locale.US).replaceFirstChar { ch ->
                if (ch.isLowerCase()) ch.titlecase(Locale.US) else ch.toString()
            }
        }
        .ifBlank { entry.name }
}

private fun composerLookupApplyTemplate(template: String, row: Map<String, JsonElement>): String {
    return Regex("""\$\{(\w+)}""").replace(template) { match ->
        jsonElementDisplayString(row[match.groupValues[1]]).orEmpty()
    }
}

private fun jsonElementDisplayString(value: JsonElement?): String? {
    return when (value) {
        is JsonPrimitive -> value.contentOrNull ?: value.toString()
        null -> null
        else -> value.toString()
    }?.trim()?.takeIf { it.isNotBlank() }
}

private fun jsonElementToLookupValue(value: JsonElement): Any? {
    return when (value) {
        JsonNull -> null
        is JsonPrimitive -> {
            value.booleanOrNull
                ?: value.longOrNull
                ?: value.doubleOrNull
                ?: value.contentOrNull
        }
        is JsonObject -> value.mapValues { jsonElementToLookupValue(it.value) }
        else -> value.toString()
    }
}
