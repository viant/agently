package com.viant.agently.android

import java.io.File
import org.junit.Assert.*
import org.junit.Test

class WorkspaceThemeTest {
    private fun fixture(): String = generateSequence(File(requireNotNull(System.getProperty("user.dir")))) { it.parentFile }
        .map { File(it, "agently-core/protocol/ui/theme/testdata/baseline.json") }
        .first { it.isFile }.readText()

    @Test fun sharedFixtureResolvesBothModes() {
        val catalog = WorkspaceThemeCatalog.load(fixture())
        val theme = catalog.themes.single()
        assertEquals("baseline", catalog.defaultTheme)
        assertEquals("dark", theme.effectiveMode("system", "dark"))
        assertEquals("light", theme.effectiveMode("light", "dark"))
        assertEquals("light", theme.effectiveMode("unknown", "dark"))
        assertEquals("36", theme.modes.getValue("light").getValue("control.minHeight").toString())
        assertEquals("\"#1b2230\"", theme.modes.getValue("dark").getValue("surface").toString())
    }
    @Test fun acceptsCanonicalWorkspaceFontRoleAndTypographyRoles() {
        val source = fixture().replace("\"typography.family\": \"system\"", "\"typography.family\": \"workspace-primary\"")
            .replace("\"typography.size\": 14", "\"typography.size\": 14, \"typography.body.size\": 14, \"typography.body.lineHeight\": 20, \"typography.display.size\": 32, \"typography.display.lineHeight\": 40")
        val theme = WorkspaceThemeCatalog.load(source).themes.single()
        assertEquals("\"workspace-primary\"", theme.modes.getValue("light").getValue("typography.family").toString())
        assertEquals("20", theme.modes.getValue("light").getValue("typography.body.lineHeight").toString())
        assertThrows(IllegalArgumentException::class.java) { WorkspaceThemeCatalog.load(source.replace("workspace-primary", "unregistered-font")) }
        assertThrows(IllegalArgumentException::class.java) { WorkspaceThemeCatalog.load(source.replace("\"typography.body.lineHeight\": 20", "\"typography.body.lineHeight\": 200")) }
    }
    @Test fun rejectsInvalidValues() {
        val source = fixture()
        listOf(source.replace("\"version\": 1", "\"version\": 2"),
            source.replace("\"control.minHeight\": 36", "\"control.minHeight\": \"36px\""),
            source.replace("#1b2230", "red; display:none")).forEach {
            assertThrows(IllegalArgumentException::class.java) { WorkspaceThemeCatalog.load(it) }
        }
    }
    @Test fun acceptsValidatedOptionalSemanticColors() {
        val source = fixture()
        val extended = source.replace(
            "\"text\": \"#171b26\"",
            "\"text\": \"#171b26\", \"text.secondary\": \"#3a4460\", \"interaction.foreground\": \"#1a3a8f\", \"status.danger.foreground\": \"#ae0020\", \"data.categorical.1\": \"#1a3a8f\"",
        )
        val catalog = WorkspaceThemeCatalog.load(extended)
        assertEquals("\"#1a3a8f\"", catalog.themes.single().modes.getValue("light").getValue("interaction.foreground").toString())
        assertThrows(IllegalArgumentException::class.java) {
            WorkspaceThemeCatalog.load(extended.replace("#ae0020", "red"))
        }
    }
}
