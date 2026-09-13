package com.viant.agently.android

import com.viant.agentlysdk.WorkspaceAssetDescriptor
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.WorkspaceThemeRequestException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.json.*
import java.security.MessageDigest

internal interface WorkspaceThemeStorage {
    fun read(key: String): String?
    fun write(key: String, value: String?)
}
internal data class WorkspaceThemeState(
    val catalog: WorkspaceThemeCatalog? = null,
    val revision: String = "",
    val themeId: String = "",
    val modePreference: String = "system",
    val diagnostic: String? = null,
) {
    val selectedTheme get() = catalog?.themes?.find { it.id == themeId }
    fun effectiveMode(systemMode: String) = selectedTheme?.effectiveMode(modePreference, systemMode) ?: systemMode
    fun tokens(systemMode: String) = selectedTheme?.modes?.get(effectiveMode(systemMode))
}

/** Main-thread lifecycle owner. Fetch suspends without blocking the UI. */
internal class WorkspaceThemeRuntime(private val storage: WorkspaceThemeStorage) {
    private data class Scope(val server: String, val account: String, val workspace: String, val persistent: Boolean) {
        val key get() = hash(JsonArray(listOf(server, account, workspace).map(::JsonPrimitive)).toString())
        fun json() = buildJsonObject { put("server", server); put("account", account); put("workspace", workspace) }.toString()
    }
    private data class Preference(val theme: String, val mode: String)
    private val mutable = MutableStateFlow(WorkspaceThemeState())
    val state = mutable.asStateFlow()
    private var scope: Scope? = null
    private var generation = 0
    private var preference: Preference? = null

    private fun read(key: String) = runCatching { storage.read(key) }.getOrNull()
    private fun write(key: String, value: String?) { runCatching { storage.write(key, value) } }
    private fun serverKey(server: String) = "server." + hash(server)
    private fun activate(next: Scope) {
        if (scope == next) return
        clear(); scope = next
        if (!next.persistent) return
        runCatching {
            val raw = read("catalog." + next.key) ?: return@runCatching
            val record = Json.parseToJsonElement(raw).jsonObject
            val catalog = WorkspaceThemeCatalog.load(record.getValue("data").jsonPrimitive.content)
            mutable.value = WorkspaceThemeState(catalog = catalog, revision = record.getValue("revision").jsonPrimitive.content)
            reconcile()
        }
    }
    fun restore(server: String) {
        if (scope != null) return
        runCatching {
            val saved = Json.parseToJsonElement(read(serverKey(server)) ?: return@runCatching).jsonObject
            val account = saved.getValue("account").jsonPrimitive.content
            val workspace = saved.getValue("workspace").jsonPrimitive.content
            if (saved.getValue("server").jsonPrimitive.content == server && account.isNotBlank() && workspace.isNotBlank()) activate(Scope(server, account, workspace, true))
        }
    }
    private fun reconcile() {
        val catalog = mutable.value.catalog ?: return
        scope?.takeIf { it.persistent }?.let { current ->
            read("preference." + current.key)?.let { raw -> runCatching {
                val saved = Json.parseToJsonElement(raw).jsonObject
                preference = Preference(saved.getValue("theme").jsonPrimitive.content, saved.getValue("mode").jsonPrimitive.content)
            } }
        }
        val saved = preference?.takeIf { (it.theme.isEmpty() || catalog.themes.any { theme -> theme.id == it.theme }) && it.mode in setOf("system", "light", "dark") }
        if (saved == null) {
            preference = null
            scope?.takeIf { it.persistent }?.let { write("preference." + it.key, null) }
        }
        mutable.value = mutable.value.copy(themeId = saved?.theme ?: catalog.defaultTheme, modePreference = saved?.mode ?: catalog.defaultMode)
    }
    fun select(theme: String, mode: String) {
        if ((theme.isNotEmpty() && mutable.value.catalog?.themes?.none { it.id == theme } != false) || mode !in setOf("system", "light", "dark")) return
        preference = Preference(theme, mode)
        mutable.value = mutable.value.copy(themeId = theme, modePreference = mode)
        scope?.takeIf { it.persistent }?.let { write("preference." + it.key, buildJsonObject { put("theme", theme); put("mode", mode) }.toString()) }
    }
    suspend fun refresh(metadata: WorkspaceMetadata, server: String, account: String, fetch: suspend (WorkspaceAssetDescriptor) -> String) {
        val workspace = metadata.workspaceId.orEmpty()
        activate(Scope(server, account, workspace.ifBlank { metadata.workspaceRoot ?: "session" }, workspace.isNotBlank() && account.isNotBlank()))
        val request = ++generation
        mutable.value = mutable.value.copy(diagnostic = metadata.uiStyleDiagnostics.takeIf { it.isNotEmpty() }?.joinToString(" · "))
        val asset = metadata.uiThemes
        if (asset == null) {
            scope?.takeIf { it.persistent }?.let { write("catalog." + it.key, null); write("preference." + it.key, null); write(serverKey(server), null) }
            preference = null; mutable.value = WorkspaceThemeState(); return
        }
        if (!asset.isThemeCatalog) { mutable.value = mutable.value.copy(diagnostic = "Unsupported workspace theme catalog. Using the previous appearance."); return }
        if (asset.revision == mutable.value.revision && mutable.value.catalog != null) return
        try {
            val raw = fetch(asset)
            val catalog = WorkspaceThemeCatalog.load(raw)
            if (request != generation) return
            mutable.value = mutable.value.copy(catalog = catalog, revision = asset.revision)
            reconcile()
            scope?.takeIf { it.persistent }?.let {
                write("catalog." + it.key, buildJsonObject { put("revision", asset.revision); put("data", raw) }.toString())
                write(serverKey(server), it.json())
            }
        } catch (cancelled: CancellationException) { throw cancelled }
        catch (error: Exception) {
            if (request != generation) return
            if (error is WorkspaceThemeRequestException && error.statusCode in setOf(401, 403)) clear(forgetAccount = true)
            mutable.value = mutable.value.copy(diagnostic = if (mutable.value.catalog == null) "Workspace appearance is unavailable. Using the default appearance." else "Workspace appearance could not be refreshed. Using the cached theme.")
        }
    }
    fun clear(forgetAccount: Boolean = false) {
        generation++
        if (forgetAccount) scope?.let { write(serverKey(it.server), null) }
        scope = null; preference = null; mutable.value = WorkspaceThemeState()
    }
    companion object {
        private fun hash(value: String) = MessageDigest.getInstance("SHA-256").digest(value.toByteArray(Charsets.UTF_8)).joinToString("") { "%02x".format(it) }
    }
}
