import Foundation
import AgentlySDK
import ForgeIOSRuntime

func makeForgeAgentlyWindowMetadataLoader(
    client: AgentlyClient,
    targetContext: ForgeTargetContext
) -> @Sendable (String) async throws -> WindowMetadata? {
    return { windowKey in
        let payload = try await client.getForgeWindowMetadata(windowKey: windowKey)
        let data = try JSONEncoder().encode(payload.forgeValue)
        let metadata = try JSONDecoder().decode(WindowMetadata.self, from: data)
        return MetadataResolver.resolve(metadata, for: targetContext)
    }
}
