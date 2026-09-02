import Foundation
import AgentlySDK
import ForgeIOSRuntime

func makeForgeAgentlyWindowMetadataLoader(
    client: AgentlyClient,
    targetContext: ForgeTargetContext
) -> @Sendable (ForgeRuntime.WindowMetadataRequest) async throws -> WindowMetadata? {
    let sdkTarget = AgentlySDK.MetadataTargetContext(
        platform: targetContext.platform,
        formFactor: targetContext.formFactor,
        surface: "app",
        capabilities: targetContext.capabilities
    )
    return { request in
        let complete = try await client.getForgeWindowMetadata(
            windowKey: request.windowKey,
            targetContext: sdkTarget
        )
        let payload: AgentlySDK.JSONValue
        if complete.objectValue?["authorization"] != nil {
            let conversationID = request.conversationID?
                .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            guard !conversationID.isEmpty else {
                throw NSError(
                    domain: "AgentlyAppFoundation.Permission",
                    code: 1,
                    userInfo: [NSLocalizedDescriptionKey: "Authorized Forge window \(request.windowKey) requires a conversation ID"]
                )
            }
            let resource = request.parameters.mapValues(\.appValue)
            payload = try await client.applyPermission(
                windowKey: request.windowKey,
                input: ApplyPermissionInput(
                    conversationId: conversationID,
                    resource: resource,
                    windowParams: resource,
                    targetContext: sdkTarget
                )
            )
        } else {
            payload = complete
        }
        let data = try JSONEncoder().encode(payload.forgeValue)
        let metadata = try JSONDecoder().decode(WindowMetadata.self, from: data)
        return MetadataResolver.resolve(metadata, for: targetContext)
    }
}
