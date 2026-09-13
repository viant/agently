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
    @Test fun rejectsInvalidValues() {
        val source = fixture()
        listOf(source.replace("\"version\": 1", "\"version\": 2"),
            source.replace("\"control.minHeight\": 36", "\"control.minHeight\": \"36px\""),
            source.replace("#1b2230", "red; display:none")).forEach {
            assertThrows(IllegalArgumentException::class.java) { WorkspaceThemeCatalog.load(it) }
        }
    }
}
