package com.viant.agently.android

import kotlinx.serialization.json.*

/** Portable tokens only. Workspace CSS is never consumed by native clients. */
internal data class WorkspaceTheme(
    val id: String,
    val label: String,
    val fallbackMode: String,
    val modes: Map<String, JsonObject>,
) {
    fun effectiveMode(preference: String, systemMode: String): String {
        val requested = if (preference == "system") systemMode else preference
        return if (modes.containsKey(requested)) requested else fallbackMode
    }
}

internal data class WorkspaceThemeCatalog(
    val defaultTheme: String,
    val defaultMode: String,
    val themes: List<WorkspaceTheme>,
    val fonts: List<com.viant.agentlysdk.WorkspaceFontFamily> = emptyList(),
) {
    companion object {
        private val dimensions = mapOf(
            "typography.size" to 8.0..72.0,
            "control.minHeight" to 16.0..128.0,
            "control.radius" to 0.0..64.0,
            "control.paddingInline" to 0.0..64.0,
        )
        private val roleDimensions = listOf("caption", "small", "body", "section", "heading", "title", "metric", "display", "code")
            .flatMap { role -> listOf("typography.$role.size" to 8.0..96.0, "typography.$role.lineHeight" to 8.0..144.0) }.toMap()
        private val colors = setOf("surface", "text", "control.background", "control.foreground",
            "control.border", "focus.color", "button.background", "button.foreground",
            "disabled.background", "disabled.foreground", "validation.border")
        private val optionalColors = setOf(
            "canvas", "surface.subtle", "surface.raised", "text.secondary", "text.muted", "text.inverse",
            "border", "border.strong", "interaction.foreground", "interaction.hover",
            "interaction.active", "interaction.selectedBackground",
            "status.info.background", "status.info.foreground", "status.info.border",
            "status.success.background", "status.success.foreground", "status.success.border",
            "status.warning.background", "status.warning.foreground", "status.warning.border",
            "status.danger.background", "status.danger.foreground", "status.danger.border",
            "data.categorical.1", "data.categorical.2", "data.categorical.3",
            "data.categorical.4", "data.categorical.5", "data.categorical.6", "data.categorical.7", "data.categorical.8",
            "data.sequential.1", "data.sequential.2", "data.sequential.3",
            "data.sequential.4", "data.sequential.5",
        )

        fun load(source: String): WorkspaceThemeCatalog {
            require(source.toByteArray(Charsets.UTF_8).size <= 512 * 1024)
            val root = Json.parseToJsonElement(source).jsonObject
            require(root["version"]?.jsonPrimitive?.isString == false && root["paletteVersion"]?.jsonPrimitive?.isString == false)
            require(root["version"]?.jsonPrimitive?.int == 1 && root["paletteVersion"]?.jsonPrimitive?.int == 1)
            val defaultTheme = root.getValue("defaultTheme").jsonPrimitive.content
            val defaultMode = root.getValue("defaultMode").jsonPrimitive.content
            require(defaultMode in setOf("light", "dark", "system"))
            val themes = root.getValue("themes").jsonArray.map { entry ->
                val obj = entry.jsonObject
                val id = obj.getValue("id").jsonPrimitive.content
                val label = obj.getValue("label").jsonPrimitive.content
                val fallback = obj.getValue("fallbackMode").jsonPrimitive.content
                require(id.matches(Regex("^[a-z][a-z0-9-]{0,63}$")) && id != "forge-default")
                require(label.isNotBlank() && label.toByteArray(Charsets.UTF_8).size <= 256)
                val modes = obj.getValue("modes").jsonObject.mapValues { it.value.jsonObject }
                require(modes.isNotEmpty() && modes.keys.all { it == "light" || it == "dark" } && fallback in modes)
                modes.values.forEach { tokens ->
                    val required = colors + dimensions.keys + "typography.family"
                    require(tokens.keys.containsAll(required) && (tokens.keys - required).all { it in optionalColors || it in roleDimensions })
                    tokens.forEach { (key, value) ->
                        val primitive = value.jsonPrimitive
                        when {
                            key in dimensions || key in roleDimensions -> {
                                require(!primitive.isString)
                                val number = primitive.double
                                require(number.isFinite() && number in (dimensions[key] ?: roleDimensions.getValue(key)))
                            }
                            key == "typography.family" -> require(primitive.isString && primitive.content in setOf("system", "workspace-primary"))
                            else -> require(primitive.isString && primitive.content.matches(Regex("^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$")))
                        }
                    }
                }
                WorkspaceTheme(id, label, fallback, modes)
            }
            require(themes.size <= 32 && themes.map { it.id }.distinct().size == themes.size)
            require(themes.any { it.id == defaultTheme })
            val fonts = root["fonts"]?.let { Json { ignoreUnknownKeys = true }.decodeFromJsonElement<List<com.viant.agentlysdk.WorkspaceFontFamily>>(it) }.orEmpty()
            require(fonts.size <= 8 && fonts.sumOf { it.faces.size } <= 64)
            return WorkspaceThemeCatalog(defaultTheme, defaultMode, themes, fonts)
        }
    }
}
