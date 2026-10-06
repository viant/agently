package com.viant.agently.android

import com.viant.forgeandroid.runtime.nativeReportMetadataConfiguration
import kotlinx.serialization.json.*

private const val REPORT_AUTHORING_CONTRACT="Use dataSources[].id as report block datasetRef. dataSources[].dataSourceRef is the underlying execution source. Dataset scope mode inherit follows the active report filters; relativeDateRange overrides them with its declared window."

/** Read-only port of Forge web snapshot.js's reportBuilder authoring catalog. */
internal fun nativeReportAuthoringSummary(metadata:JsonObject,form:JsonObject):JsonObject? {
    fun text(value:JsonElement?)=(value as? JsonPrimitive)?.takeUnless { it==JsonNull }?.content?.trim().orEmpty()
    var defaultRef=""
    var selectedLabel=""
    var contentTitle=""
    val requested=text(form["reportBuilderRef"])
    fun locate(value:JsonElement) {
        when(value) {
            is JsonArray -> value.forEach(::locate)
            is JsonObject -> {
                val dashboard=value["dashboard"] as? JsonObject
                if(defaultRef.isEmpty()) defaultRef=text(dashboard?.get("reportBuilderRef")).ifBlank { text(value["reportBuilderRef"]) }
                val ref=requested.ifBlank { defaultRef }
                val variants=(value["reportBuilders"] ?: dashboard?.get("reportBuilders")) as? JsonObject
                val selected=variants?.get(ref) as? JsonObject
                if(selected?.get("reportBuilder") is JsonObject && selectedLabel.isEmpty()) {
                    selectedLabel=text(selected["label"]);if(contentTitle.isEmpty()) contentTitle=text(value["title"])
                }
                value.filterKeys { it!="reportBuilders" }.values.forEach(::locate)
            }
            else -> Unit
        }
    }
    locate(metadata)
    val builder=requested.ifBlank { defaultRef }
    val config=nativeReportMetadataConfiguration(metadata,builder)?.first ?: return null
    fun fields(value:List<JsonElement>):JsonArray {
        val seen=linkedSetOf<String>()
        return JsonArray(value.mapNotNull { raw ->
            val option=raw as? JsonObject ?: return@mapNotNull null
            val key=text(option["key"]).ifBlank { text(option["value"]) }
            if(key.isBlank() || !seen.add(key)) return@mapNotNull null
            buildJsonObject {
                put("key",key);put("label",text(option["label"]).ifBlank { key })
                for(name in listOf("kind","format","paramPath","startParamPath","endParamPath")) put(name,text(option[name]).takeIf(String::isNotBlank)?.let(::JsonPrimitive)?:JsonNull)
                for(name in listOf("required","multiple")) if(option[name]==JsonPrimitive(true)) put(name,true)
            }
        }.take(100))
    }
    fun safe(value:JsonElement,depth:Int=0,maxDepth:Int):JsonElement = when {
        depth>maxDepth -> JsonPrimitive("[MaxDepth]")
        value is JsonPrimitive && value.isString && value.content.length>500 -> JsonPrimitive(value.content.take(500)+"…")
        value is JsonArray && value.size>100 -> buildJsonObject { put("items",JsonArray(value.take(100).map { safe(it,depth+1,maxDepth) }));put("truncated",true);put("total",value.size) }
        value is JsonArray -> JsonArray(value.map { safe(it,depth+1,maxDepth) })
        value is JsonObject -> JsonObject(value.entries.take(30).associate { it.key to safe(it.value,depth+1,maxDepth) }+if(value.size>30) mapOf("truncated" to JsonPrimitive(true),"totalKeys" to JsonPrimitive(value.size)) else emptyMap())
        else -> value
    }
    val sources=(config["dataSources"] as? JsonArray).orEmpty().mapNotNull { raw ->
        val source=raw as? JsonObject ?: return@mapNotNull null
        val id=text(source["id"]).takeIf(String::isNotBlank)?:return@mapNotNull null
        buildJsonObject {
            put("id",id);put("dataSourceRef",text(source["dataSourceRef"]).takeIf(String::isNotBlank)?.let(::JsonPrimitive)?:JsonNull)
            put("label",text(source["label"]).ifBlank { id })
            for(name in listOf("description","kindLabel")) put(name,text(source[name]).takeIf(String::isNotBlank)?.let(::JsonPrimitive)?:JsonNull)
            source["capabilities"]?.takeIf { it is JsonObject || it is JsonArray }?.let { put("capabilities",safe(it,maxDepth=2)) }
            source["scope"]?.takeIf { it is JsonObject || it is JsonArray }?.let { put("scope",safe(it,maxDepth=4)) }
            put("fields",fields(listOf("columnOptions","chartFieldOptions","valueFieldOptions","secondaryFieldOptions").flatMap { (source[it] as? JsonArray).orEmpty() }))
            put("scopeParams",fields((source["scopeParamOptions"] as? JsonArray).orEmpty()))
        }
    }.take(50)
    return buildJsonObject {
        put("builderRef",builder.takeIf(String::isNotBlank)?.let(::JsonPrimitive)?:JsonNull)
        put("label",selectedLabel.ifBlank { text(config["label"]) }.ifBlank { contentTitle }.ifBlank { builder }.takeIf(String::isNotBlank)?.let(::JsonPrimitive)?:JsonNull)
        put("authoringContract",REPORT_AUTHORING_CONTRACT);put("dataSources",JsonArray(sources))
    }
}

internal fun nativeMetadataWithReportAuthoringSummary(metadata:JsonObject,form:JsonObject):JsonObject = nativeReportAuthoringSummary(metadata,form)?.let { JsonObject(metadata+("reportBuilder" to it)) } ?: metadata
