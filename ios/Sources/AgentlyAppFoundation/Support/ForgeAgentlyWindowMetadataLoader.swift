import Foundation
import AgentlySDK
import ForgeIOSRuntime

func makeForgeAgentlyWindowMetadataLoader(
    client: AgentlyClient,
    targetContext: ForgeTargetContext
) -> @Sendable (String) async throws -> WindowMetadata? {
    return { windowKey in
        let payload = try await client.getForgeWindowMetadata(
            windowKey: windowKey,
            targetContext: AgentlySDK.MetadataTargetContext(
                platform: targetContext.platform,
                formFactor: targetContext.formFactor,
                surface: "app",
                capabilities: targetContext.capabilities
            )
        )
        let data = try JSONEncoder().encode(payload.forgeValue)
        let metadata = try JSONDecoder().decode(WindowMetadata.self, from: data)
        return MetadataResolver.resolve(metadata, for: targetContext)
    }
}
