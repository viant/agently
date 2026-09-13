package com.viant.agently.android

import com.viant.agentlysdk.WorkspaceAssetDescriptor
import com.viant.agentlysdk.WorkspaceMetadata
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import java.io.File
import java.io.IOException

class WorkspaceThemeRuntimeTest {
    private class Memory : WorkspaceThemeStorage {
        val values = mutableMapOf<String, String>()
        override fun read(key: String) = values[key]
        override fun write(key: String, value: String?) { if (value == null) values.remove(key) else values[key] = value }
    }
    private fun fixture() = generateSequence(File(requireNotNull(System.getProperty("user.dir")))) { it.parentFile }
        .map { File(it, "agently-core/protocol/ui/theme/testdata/baseline.json") }.first { it.isFile }.readText()
    private fun metadata(char: Char = 'a', workspace: String = "workspace"): WorkspaceMetadata {
        val revision = char.toString().repeat(64)
        return WorkspaceMetadata(workspaceId = workspace, uiThemes = WorkspaceAssetDescriptor(1, revision, "/v1/workspace/ui/themes/$revision.json"))
    }
    @Test fun selectionSurvivesOfflineRestartAndLogoutClearsPointer() = runBlocking {
        val storage = Memory(); val runtime = WorkspaceThemeRuntime(storage); val json = fixture()
        runtime.refresh(metadata(), "server", "user") { json }
        runtime.select("baseline", "dark")
        val restored = WorkspaceThemeRuntime(storage); restored.restore("server")
        assertEquals("dark", restored.state.value.effectiveMode("light"))
        restored.refresh(metadata('b'), "server", "user") { throw IOException("offline") }
        assertEquals("baseline", restored.state.value.themeId)
        assertTrue(restored.state.value.diagnostic!!.contains("cached"))
        restored.clear(forgetAccount = true)
        val loggedOut = WorkspaceThemeRuntime(storage); loggedOut.restore("server")
        assertNull(loggedOut.state.value.catalog)
    }
    @Test fun otherAccountsAndWorkspacesCannotReuseCachedAppearance() = runBlocking {
        val runtime = WorkspaceThemeRuntime(Memory()); val json = fixture()
        runtime.refresh(metadata(), "server", "one") { json }
        runtime.refresh(metadata(), "server", "two") { throw IOException("offline") }
        assertNull(runtime.state.value.catalog)
        runtime.refresh(metadata(workspace = "other"), "server", "one") { throw IOException("offline") }
        assertNull(runtime.state.value.catalog)
    }
    @Test fun removedThemesAndManifestResetSelection() = runBlocking {
        val storage = Memory(); val runtime = WorkspaceThemeRuntime(storage); val json = fixture()
        runtime.refresh(metadata(), "server", "user") { json }
        runtime.select("baseline", "dark")
        runtime.refresh(metadata('b'), "server", "user") { json.replace("\"baseline\"", "\"renamed\"") }
        assertEquals("renamed", runtime.state.value.themeId)
        assertEquals("system", runtime.state.value.modePreference)
        runtime.refresh(WorkspaceMetadata(workspaceId = "workspace"), "server", "user") { error("must not fetch CSS") }
        assertNull(runtime.state.value.catalog)
        val restored = WorkspaceThemeRuntime(storage); restored.restore("server")
        assertNull(restored.state.value.catalog)
    }
    @Test fun lateResponseCannotRestoreClearedWorkspace() = runBlocking {
        val runtime = WorkspaceThemeRuntime(Memory()); val json = fixture()
        val started = CompletableDeferred<Unit>(); val response = CompletableDeferred<String>()
        val pending = async { runtime.refresh(metadata(), "server", "user") { started.complete(Unit); response.await() } }
        started.await(); runtime.clear(); response.complete(json); pending.await()
        assertNull(runtime.state.value.catalog)
    }
    @Test fun unavailableStorageKeepsInMemoryDefaultSelection() = runBlocking {
        val storage = object : WorkspaceThemeStorage {
            override fun read(key: String): String? = throw SecurityException()
            override fun write(key: String, value: String?) { throw SecurityException() }
        }
        val runtime = WorkspaceThemeRuntime(storage); val json = fixture()
        runtime.refresh(metadata(), "server", "user") { json }
        runtime.select("", "system")
        runtime.refresh(metadata('b'), "server", "user") { json }
        assertEquals("", runtime.state.value.themeId)
    }
}
