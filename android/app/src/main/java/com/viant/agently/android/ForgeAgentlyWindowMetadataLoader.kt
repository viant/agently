package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.fetchForgeWindowMetadata
import com.viant.forgeandroid.runtime.ForgeTargetContext
import com.viant.forgeandroid.runtime.MetadataResolver
import com.viant.forgeandroid.runtime.WindowMetadata
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.decodeFromJsonElement

internal fun makeForgeAgentlyWindowMetadataLoader(
    client: AgentlyClient,
    targetContext: ForgeTargetContext
): suspend (String) -> WindowMetadata? {
    val json = Json { ignoreUnknownKeys = true }
    return { windowKey ->
        val raw = client.fetchForgeWindowMetadata(windowKey)
        val resolved = MetadataResolver.resolve(raw, targetContext) ?: raw
        json.decodeFromJsonElement<WindowMetadata>(resolved)
    }
}
