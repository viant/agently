import SwiftUI
import AgentlySDK
import ForgeIOSRuntime

public struct PhoneWorkspaceScreen: View {
    @State private var isArtifactSectionExpanded = false
    @State private var isApprovalSectionExpanded = true
    let metadata: WorkspaceMetadata?
    let selectedAgentID: String?
    let availableAgents: [WorkspaceAgentOption]
    let transcript: [ChatTranscriptEntry]
    let artifacts: [ArtifactPreview]
    let composerRuntime: ComposerRuntime
    let isSending: Bool
    let isLoadingConversation: Bool
    let isLoadingArtifacts: Bool
    let queryError: String?
    let activeTurnID: String?
    let isStoppingTurn: Bool
    let streamError: String?
    let approvals: [PendingToolApproval]
    let decidingApprovalID: String?
    let approvalError: String?
    let pendingElicitation: PendingElicitation?
    let isResolvingElicitation: Bool
    let elicitationError: String?
    let artifactError: String?
    let onSend: () -> Void
    let onCancelTurn: () -> Void
    let onRetryStreaming: () -> Void
    let onSelectArtifact: (ArtifactPreview) -> Void
    let onDecision: (PendingToolApproval, String, [String: AppJSONValue]) -> Void
    let onResolveElicitation: (String, [String: AppJSONValue]) -> Void
    let onDismissElicitation: () -> Void
    let onSelectAgent: (String?) -> Void
    let forgeRuntime: ForgeRuntime?

    public init(
        metadata: WorkspaceMetadata?,
        selectedAgentID: String? = nil,
        availableAgents: [WorkspaceAgentOption] = [],
        transcript: [ChatTranscriptEntry],
        artifacts: [ArtifactPreview] = [],
        composerRuntime: ComposerRuntime,
        isSending: Bool = false,
        isLoadingConversation: Bool = false,
        isLoadingArtifacts: Bool = false,
        queryError: String? = nil,
        activeTurnID: String? = nil,
        isStoppingTurn: Bool = false,
        streamError: String? = nil,
        approvals: [PendingToolApproval] = [],
        decidingApprovalID: String? = nil,
        approvalError: String? = nil,
        pendingElicitation: PendingElicitation? = nil,
        isResolvingElicitation: Bool = false,
        elicitationError: String? = nil,
        artifactError: String? = nil,
        onSend: @escaping () -> Void,
        onCancelTurn: @escaping () -> Void = {},
        onRetryStreaming: @escaping () -> Void = {},
        onSelectArtifact: @escaping (ArtifactPreview) -> Void = { _ in },
        onDecision: @escaping (PendingToolApproval, String, [String: AppJSONValue]) -> Void = { _, _, _ in },
        onResolveElicitation: @escaping (String, [String: AppJSONValue]) -> Void = { _, _ in },
        onDismissElicitation: @escaping () -> Void = {},
        onSelectAgent: @escaping (String?) -> Void = { _ in },
        forgeRuntime: ForgeRuntime? = nil
    ) {
        self.metadata = metadata
        self.selectedAgentID = selectedAgentID
        self.availableAgents = availableAgents
        self.transcript = transcript
        self.artifacts = artifacts
        self.composerRuntime = composerRuntime
        self.isSending = isSending
        self.isLoadingConversation = isLoadingConversation
        self.isLoadingArtifacts = isLoadingArtifacts
        self.queryError = queryError
        self.activeTurnID = activeTurnID
        self.isStoppingTurn = isStoppingTurn
        self.streamError = streamError
        self.approvals = approvals
        self.decidingApprovalID = decidingApprovalID
        self.approvalError = approvalError
        self.pendingElicitation = pendingElicitation
        self.isResolvingElicitation = isResolvingElicitation
        self.elicitationError = elicitationError
        self.artifactError = artifactError
        self.onSend = onSend
        self.onCancelTurn = onCancelTurn
        self.onRetryStreaming = onRetryStreaming
        self.onSelectArtifact = onSelectArtifact
        self.onDecision = onDecision
        self.onResolveElicitation = onResolveElicitation
        self.onDismissElicitation = onDismissElicitation
        self.onSelectAgent = onSelectAgent
        self.forgeRuntime = forgeRuntime
    }

    public var body: some View {
        VStack(spacing: 0) {
            ChatWorkspaceView(
                metadata: metadata,
                selectedAgentID: selectedAgentID,
                availableAgents: availableAgents,
                onSelectAgent: onSelectAgent
            )
            WorkspaceStatusSection(
                isSending: isSending,
                isLoadingArtifacts: isLoadingArtifacts,
                activeTurnID: activeTurnID,
                isStoppingTurn: isStoppingTurn,
                decidingApprovalID: decidingApprovalID,
                isResolvingElicitation: isResolvingElicitation,
                queryError: queryError,
                streamError: streamError,
                approvalError: approvalError,
                elicitationError: elicitationError,
                artifactError: artifactError,
                onCancelTurn: onCancelTurn,
                onRetryStreaming: onRetryStreaming
            )
            if !artifacts.isEmpty {
                WorkspaceAccessorySection(
                    title: "Artifacts",
                    count: artifacts.count,
                    isExpanded: $isArtifactSectionExpanded
                ) {
                    ArtifactListView(previews: artifacts, onSelect: onSelectArtifact)
                }
                    .padding(.horizontal)
                    .padding(.bottom, 12)
            } else if isLoadingArtifacts {
                ArtifactLoadingView()
                    .padding(.horizontal)
                    .padding(.bottom, 12)
            }
            if !approvals.isEmpty {
                WorkspaceAccessorySection(
                    title: "Approvals",
                    count: approvals.count,
                    isExpanded: $isApprovalSectionExpanded
                ) {
                    ApprovalListView(
                        approvals: approvals,
                        decidingApprovalID: decidingApprovalID,
                        forgeRuntime: forgeRuntime,
                        onDecision: onDecision
                    )
                }
                .padding(.horizontal)
                .padding(.bottom, 12)
            }
            if transcript.isEmpty, isLoadingConversation {
                WorkspaceLoadingView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if transcript.isEmpty {
                ContentUnavailableView(
                    "No Messages Yet",
                    systemImage: "ellipsis.message",
                    description: Text("Ask the workspace a question to begin a conversation.")
                )
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                TranscriptScreen(
                    items: transcript,
                    onReusePrompt: { prompt in
                        composerRuntime.query = prompt
                    },
                    onReuseAndSendPrompt: isSending ? nil : { prompt in
                        composerRuntime.query = prompt
                        onSend()
                    }
                )
            }
            ComposerScreen(runtime: composerRuntime, isSending: isSending, onSend: onSend)
        }
        .sheet(isPresented: Binding(
            get: { pendingElicitation != nil },
            set: { if !$0 { onDismissElicitation() } }
        )) {
            ElicitationOverlay(
                pending: pendingElicitation,
                errorMessage: elicitationError,
                isResolving: isResolvingElicitation,
                forgeRuntime: forgeRuntime,
                onResolve: onResolveElicitation,
                onDismiss: onDismissElicitation
            )
        }
    }
}
