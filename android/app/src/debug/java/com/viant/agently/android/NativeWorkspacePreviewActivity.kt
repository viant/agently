package com.viant.agently.android

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.forgeandroid.runtime.*
import com.viant.forgeandroid.ui.ForgeRoot
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.*
import okhttp3.OkHttpClient
import okhttp3.Request

/** Debug source set only. Renders workspace metadata with the shipping Compose renderer. */
class NativeWorkspacePreviewActivity : ComponentActivity() {
    private val previewRequest = MutableStateFlow<PreviewRequest?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        ActionHookRuntime.initialize(applicationContext)
        previewRequest.value = intent.toPreviewRequest()
        setContent {
            val request by previewRequest.collectAsState()
            request?.let { current ->
                key(current) {
                    NativeWorkspacePreview(current.baseUrl, current.windowKey, current.parameters, current.sectionId)
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        previewRequest.value = intent.toPreviewRequest()
    }
}

private data class PreviewRequest(
    val baseUrl: String,
    val windowKey: String,
    val parameters: String,
    val sectionId: String,
)

private data class PreviewAccessOptions(val roles: Set<String> = emptySet(), val features: Set<String> = emptySet())

private fun Intent.toPreviewRequest(): PreviewRequest {
    val baseUrl = (getStringExtra("baseUrl") ?: "http://127.0.0.1:8118").trimEnd('/')
    require(java.net.URI(baseUrl).host in setOf("127.0.0.1", "localhost", "10.0.2.2")) {
        "Native preview only supports a local synthetic preview server"
    }
    return PreviewRequest(
        baseUrl = baseUrl,
        windowKey = getStringExtra("window") ?: "advertiser",
        parameters = getStringExtra("parameters") ?: "{}",
        sectionId = getStringExtra("section")?.trim().orEmpty(),
    )
}

@Composable
private fun NativeWorkspacePreview(baseUrl: String, windowKey: String, parameters: String, sectionId: String) {
    val androidContext = LocalContext.current
    val scope = rememberCoroutineScope()
    val json = remember { Json { ignoreUnknownKeys = true } }
    val http = remember { OkHttpClient() }
    val theme = remember { WorkspaceThemeRuntime(object : WorkspaceThemeStorage {
        override fun read(key: String): String? = null
        override fun write(key: String, value: String?) = Unit
    }) }
    var error by remember { mutableStateOf<String?>(null) }
    var accessOptions by remember { mutableStateOf(PreviewAccessOptions()) }
    var selectedRoles by remember { mutableStateOf<Set<String>>(emptySet()) }
    var selectedFeatures by remember { mutableStateOf<Set<String>>(emptySet()) }
    var accessInitialized by remember { mutableStateOf(false) }
    suspend fun get(path: String): String = withContext(Dispatchers.IO) {
        http.newCall(Request.Builder().url(baseUrl + path).build()).execute().use {
            check(it.isSuccessful) { "Preview HTTP ${it.code}: $path" }
            it.body?.string() ?: error("Empty preview response")
        }
    }
    val runtime = remember {
        val target = buildForgeTargetContext("phone")
        ForgeRuntime(mapOf("preview" to EndpointConfig(baseUrl)), scope, target).also { runtime ->
            runtime.registerExternalURLHandler { href -> androidContext.startActivity(Intent(Intent.ACTION_VIEW, android.net.Uri.parse(href))) }
            runtime.registerWindowMetadataRequestLoader { request ->
                try {
                    val encoded = java.net.URLEncoder.encode(request.windowKey, "UTF-8")
                    val response = json.parseToJsonElement(
                        get("/api/windows/$encoded?platform=android&formFactor=phone&surface=app")
                    ).jsonObject.getValue("data")
                    val options = collectPreviewAccessOptions(response)
                    val baseline = previewPrincipalAccess(response)
                    withContext(Dispatchers.Main) {
                        accessOptions = options
                        if (!accessInitialized) {
                            selectedRoles = baseline.roles
                            selectedFeatures = baseline.features
                            accessInitialized = true
                        }
                    }
                    val raw = if (sectionId.isBlank()) response else selectPreviewSection(response, sectionId).first
                    decodeForgeWindowMetadata(raw, target)
                } catch (failure: Exception) {
                    error = failure.message
                    throw failure
                }
            }
        }
    }
    LaunchedEffect(runtime) {
        try {
            val metadata = json.decodeFromString<WorkspaceMetadata>(get("/api/workspace"))
            theme.refresh(metadata, baseUrl, "native-preview") { get(it.href) }
            val args = json.parseToJsonElement(parameters).jsonObject.mapValues { JsonUtil.elementToAny(it.value) }
            runtime.openWindow(windowKey, parameters = args)
        } catch (failure: Exception) { error = failure.message }
    }
    WorkspaceThemeHost(theme) {
        Surface(Modifier.fillMaxSize()) {
            Column(Modifier.fillMaxSize().safeDrawingPadding()) {
                Row(
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
                    verticalAlignment = androidx.compose.ui.Alignment.CenterVertically
                ) {
                    Text("Native preview · synthetic workspace", style = MaterialTheme.typography.labelSmall, modifier = Modifier.weight(1f))
                    PreviewAccessMenu(accessOptions, selectedRoles, selectedFeatures) { roles, features ->
                        selectedRoles = roles
                        selectedFeatures = features
                        runtime.windows.value.forEach { runtime.updatePreviewPrincipal(it.windowId, roles, features) }
                    }
                }
                error?.let { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(12.dp)) }
                Box(Modifier.weight(1f)) { ForgeRoot(runtime) }
            }
        }
    }
}

@Composable
private fun PreviewAccessMenu(
    options: PreviewAccessOptions,
    roles: Set<String>,
    features: Set<String>,
    onChange: (Set<String>, Set<String>) -> Unit
) {
    if (options.roles.isEmpty() && options.features.isEmpty()) return
    var expanded by remember { mutableStateOf(false) }
    Box {
        TextButton(onClick = { expanded = true }) { Text("Access ${roles.size + features.size}") }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            fun toggle(kind: String, value: String, checked: Boolean) {
                if (kind == "role") onChange(if (checked) roles + value else roles - value, features)
                else onChange(roles, if (checked) features + value else features - value)
            }
            options.roles.sorted().forEach { value ->
                DropdownMenuItem(
                    text = { Text(value) },
                    leadingIcon = { Checkbox(checked = value in roles, onCheckedChange = null) },
                    onClick = { toggle("role", value, value !in roles) }
                )
            }
            options.features.sorted().forEach { value ->
                DropdownMenuItem(
                    text = { Text(value) },
                    leadingIcon = { Checkbox(checked = value in features, onCheckedChange = null) },
                    onClick = { toggle("feature", value, value !in features) }
                )
            }
        }
    }
}

private fun previewPrincipalAccess(element: JsonElement): PreviewAccessOptions {
    val principal = element.jsonObject["authorizationSnapshot"]?.jsonObject?.get("principal")?.jsonObject
    fun values(key: String) = (principal?.get(key) as? JsonArray).orEmpty()
        .mapNotNull { (it as? JsonPrimitive)?.contentOrNull?.trim()?.takeIf(String::isNotBlank) }
        .toSet()
    return PreviewAccessOptions(values("roles"), values("features"))
}

private fun collectPreviewAccessOptions(element: JsonElement): PreviewAccessOptions {
    val baseline = previewPrincipalAccess(element)
    val roles = baseline.roles.toMutableSet()
    val features = baseline.features.toMutableSet()
    fun add(target: MutableSet<String>, value: JsonElement?) {
        when (value) {
            is JsonPrimitive -> value.contentOrNull?.trim()?.takeIf(String::isNotBlank)?.let(target::add)
            is JsonArray -> value.forEach { add(target, it) }
            else -> Unit
        }
    }
    fun visit(value: JsonElement) {
        when (value) {
            is JsonArray -> value.forEach(::visit)
            is JsonObject -> {
                add(roles, value["roles"])
                add(features, value["features"])
                val field = (value["field"] as? JsonPrimitive)?.contentOrNull.orEmpty()
                val target = when {
                    field.endsWith("roles") -> roles
                    field.endsWith("features") -> features
                    else -> null
                }
                if (target != null) listOf("contains", "containsAny", "containsAll", "equals", "in", "value", "values").forEach { add(target, value[it]) }
                value.values.forEach(::visit)
            }
            else -> Unit
        }
    }
    visit(element)
    return PreviewAccessOptions(roles, features)
}

/** Debug-only deterministic tab selection for screenshot and orientation QA. */
internal fun selectPreviewSection(element: JsonElement, sectionId: String): Pair<JsonElement, Boolean> {
    return when (element) {
        is JsonArray -> {
            var found = false
            val children = element.map { child ->
                val (next, contains) = selectPreviewSection(child, sectionId)
                found = found || contains
                next
            }
            JsonArray(children) to found
        }
        is JsonObject -> {
            var found = (element["id"] as? JsonPrimitive)?.content == sectionId
            val next = element.mapValues { (_, child) ->
                val (resolved, contains) = selectPreviewSection(child, sectionId)
                found = found || contains
                resolved
            }.toMutableMap()
            val containers = next["containers"] as? JsonArray
            val tabs = next["tabs"] as? JsonObject
            if (containers != null && tabs != null) {
                val selected = containers.firstOrNull { child ->
                    selectPreviewSection(child, sectionId).second
                } as? JsonObject
                val selectedId = (selected?.get("id") as? JsonPrimitive)?.content
                if (!selectedId.isNullOrBlank()) {
                    next["tabs"] = JsonObject(tabs + ("defaultSelectedTabId" to JsonPrimitive(selectedId)))
                }
            }
            JsonObject(next) to found
        }
        else -> element to false
    }
}
