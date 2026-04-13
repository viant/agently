import Foundation
import AgentlySDK
import ForgeIOSRuntime

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

    public init(
        client: AgentlyClient,
        bootstrapBaseURL: String,
        forgeRuntime: ForgeRuntime? = nil
    ) {
        self.client = client
        self.bootstrapBaseURL = bootstrapBaseURL
        let metadataBaseURL = URL(string: bootstrapBaseURL)
        self.forgeRuntime = forgeRuntime ?? ForgeRuntime(windowMetadataBaseURL: metadataBaseURL)
    }
}

public enum AuthState: Sendable {
    case checking
    case required
    case connectionFailed
    case signedIn
}
