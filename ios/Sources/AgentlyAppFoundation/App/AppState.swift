import Foundation
import AgentlySDK
import ForgeIOSRuntime
#if canImport(UIKit)
import UIKit
#endif

@MainActor
public final class AppState: ObservableObject {
    @Published public var authState: AuthState = .checking
    @Published public var workspaceMetadata: WorkspaceMetadata?
    @Published public var conversations: [Conversation] = []
    @Published public var activeConversationID: String?
    @Published public var activeTurnID: String?
    @Published public var artifacts: [ArtifactPreview] = []
    @Published public var selectedArtifact: ArtifactPreview?
    @Published public var query: String = ""
    @Published public var bootstrapErrorMessage: String?
    @Published public var artifactErrorMessage: String?
    @Published public var streamErrorMessage: String?
    @Published public var bootstrapBaseURL: String
    @Published public var isStoppingTurn: Bool = false
    @Published public var isRefreshingConversations: Bool = false
    @Published public var isLoadingConversation: Bool = false
    @Published public var isLoadingArtifacts: Bool = false

    public var client: AgentlyClient
    public let forgeRuntime: ForgeRuntime
    public let metadataTargetContext: MetadataTargetContext

    public init(
        client: AgentlyClient,
        bootstrapBaseURL: String,
        forgeRuntime: ForgeRuntime? = nil
    ) {
        self.client = client
        self.bootstrapBaseURL = bootstrapBaseURL
        let formFactor = detectAppleFormFactor()
        self.metadataTargetContext = MetadataTargetContext(
            platform: "ios",
            formFactor: formFactor,
            surface: "app",
            capabilities: buildAppleTargetCapabilities()
        )
        let metadataBaseURL = URL(string: bootstrapBaseURL)
        self.forgeRuntime = forgeRuntime ?? ForgeRuntime(
            targetContext: ForgeTargetContext(
                platform: "ios",
                formFactor: formFactor,
                capabilities: buildAppleTargetCapabilities()
            ),
            windowMetadataBaseURL: metadataBaseURL
        )
    }
}

internal func buildAppleTargetCapabilities() -> [String] {
    ["markdown", "chart", "attachments", "camera", "voice"]
}

internal func detectAppleFormFactor() -> String {
#if canImport(UIKit)
    UIDevice.current.userInterfaceIdiom == .pad ? "tablet" : "phone"
#else
    "phone"
#endif
}

public enum AuthState: Sendable {
    case checking
    case required
    case connectionFailed
    case signedIn
}
