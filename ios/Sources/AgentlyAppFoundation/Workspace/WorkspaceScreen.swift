import SwiftUI
import AgentlySDK
import Foundation
import ForgeIOSRuntime

public struct WorkspaceScreen: View {
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
        approvals: [PendingToolApproval],
        decidingApprovalID: String? = nil,
        approvalError: String? = nil,
        pendingElicitation: PendingElicitation?,
        isResolvingElicitation: Bool = false,
        elicitationError: String? = nil,
        artifactError: String? = nil,
        onSend: @escaping () -> Void,
        onCancelTurn: @escaping () -> Void = {},
        onRetryStreaming: @escaping () -> Void = {},
        onSelectArtifact: @escaping (ArtifactPreview) -> Void = { _ in },
        onDecision: @escaping (PendingToolApproval, String, [String: AppJSONValue]) -> Void,
        onResolveElicitation: @escaping (String, [String: AppJSONValue]) -> Void,
        onDismissElicitation: @escaping () -> Void,
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
            HStack(alignment: .top, spacing: 20) {
                mainPane
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)

                WorkspaceSidebar(
                    artifacts: artifacts,
                    approvals: approvals,
                    decidingApprovalID: decidingApprovalID,
                    isLoadingArtifacts: isLoadingArtifacts,
                    isArtifactSectionExpanded: $isArtifactSectionExpanded,
                    isApprovalSectionExpanded: $isApprovalSectionExpanded,
                    forgeRuntime: forgeRuntime,
                    onSelectArtifact: onSelectArtifact,
                    onDecision: onDecision
                )
                .frame(minWidth: 300, idealWidth: 340, maxWidth: 340, maxHeight: .infinity, alignment: .top)
            }
            .padding(.horizontal, 20)
            .padding(.bottom, 16)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
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

    private var mainPane: some View {
        VStack(spacing: 16) {
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

            transcriptCard
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)

            ComposerScreen(runtime: composerRuntime, isSending: isSending, onSend: onSend)
                .padding(.horizontal, 4)
        }
    }

    private var transcriptCard: some View {
        Group {
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
        }
        .padding(.vertical, 8)
        .background(Color.secondary.opacity(0.05), in: RoundedRectangle(cornerRadius: 20))
        .overlay(
            RoundedRectangle(cornerRadius: 20)
                .stroke(Color.secondary.opacity(0.12), lineWidth: 1)
        )
    }

    @ViewBuilder
    private var transcriptContent: some View {
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
    }
}

private struct WorkspaceSidebar: View {
    let artifacts: [ArtifactPreview]
    let approvals: [PendingToolApproval]
    let decidingApprovalID: String?
    let isLoadingArtifacts: Bool
    @Binding var isArtifactSectionExpanded: Bool
    @Binding var isApprovalSectionExpanded: Bool
    let forgeRuntime: ForgeRuntime?
    let onSelectArtifact: (ArtifactPreview) -> Void
    let onDecision: (PendingToolApproval, String, [String: AppJSONValue]) -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
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
                }

                if !artifacts.isEmpty {
                    WorkspaceAccessorySection(
                        title: "Artifacts",
                        count: artifacts.count,
                        isExpanded: $isArtifactSectionExpanded
                    ) {
                        ArtifactListView(previews: artifacts, onSelect: onSelectArtifact)
                    }
                } else if isLoadingArtifacts {
                    WorkspaceAccessorySection(
                        title: "Artifacts",
                        count: 0,
                        isExpanded: $isArtifactSectionExpanded
                    ) {
                        ArtifactLoadingView()
                    }
                }

                if approvals.isEmpty && artifacts.isEmpty && !isLoadingArtifacts {
                    ContentUnavailableView(
                        "No Workspace Context",
                        systemImage: "sidebar.right",
                        description: Text("Approvals and artifacts for this conversation will appear here.")
                    )
                    .frame(maxWidth: .infinity)
                    .padding(.top, 32)
                }
            }
            .padding(16)
        }
        .background(Color.secondary.opacity(0.05), in: RoundedRectangle(cornerRadius: 20))
        .overlay(
            RoundedRectangle(cornerRadius: 20)
                .stroke(Color.secondary.opacity(0.12), lineWidth: 1)
        )
    }
}

struct WorkspaceAccessorySection<Content: View>: View {
    let title: String
    let count: Int
    @Binding var isExpanded: Bool
    @ViewBuilder let content: Content

    init(
        title: String,
        count: Int,
        isExpanded: Binding<Bool>,
        @ViewBuilder content: () -> Content
    ) {
        self.title = title
        self.count = count
        self._isExpanded = isExpanded
        self.content = content()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Button {
                withAnimation(.easeInOut(duration: 0.2)) {
                    isExpanded.toggle()
                }
            } label: {
                HStack(spacing: 8) {
                    Image(systemName: isExpanded ? "chevron.down" : "chevron.right")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.secondary)
                    Text(title)
                        .font(.headline)
                        .foregroundStyle(.primary)
                    Text("\(count)")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                        .background(Color.secondary.opacity(0.12), in: Capsule())
                    Spacer()
                    Text(isExpanded ? "Hide" : "Show")
                        .font(.footnote.weight(.medium))
                        .foregroundStyle(.secondary)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)

            if isExpanded {
                content
            }
        }
        .padding(12)
        .background(Color.secondary.opacity(0.06), in: RoundedRectangle(cornerRadius: 14))
    }
}

struct WorkspaceStatusSection: View {
    let isSending: Bool
    let isLoadingArtifacts: Bool
    let activeTurnID: String?
    let isStoppingTurn: Bool
    let decidingApprovalID: String?
    let isResolvingElicitation: Bool
    let queryError: String?
    let streamError: String?
    let approvalError: String?
    let elicitationError: String?
    let artifactError: String?
    let onCancelTurn: () -> Void
    let onRetryStreaming: () -> Void

    var body: some View {
        VStack(spacing: 8) {
            if isLoadingArtifacts {
                WorkspaceBanner(
                    title: "Loading artifacts",
                    message: "Refreshing files for the active conversation.",
                    tint: .secondary
                )
            }
            if isSending, activeTurnID == nil, !isStoppingTurn {
                WorkspaceBanner(
                    title: "Sending query",
                    message: "Waiting for the workspace to accept the latest request.",
                    tint: .blue
                )
            }
            if let activeTurnID, !activeTurnID.isEmpty {
                WorkspaceBanner(
                    title: isStoppingTurn ? "Stopping turn" : "Streaming response",
                    message: isStoppingTurn
                        ? "Waiting for the workspace to cancel the current turn."
                        : "The assistant is still working on the current turn.",
                    tint: .blue,
                    actionTitle: isStoppingTurn ? nil : "Stop",
                    action: isStoppingTurn ? nil : onCancelTurn
                )
            }
            if decidingApprovalID != nil {
                WorkspaceBanner(
                    title: "Submitting approval",
                    message: "Waiting for the workspace to apply the latest approval decision.",
                    tint: .orange
                )
            }
            if isResolvingElicitation {
                WorkspaceBanner(
                    title: "Submitting elicitation",
                    message: "Waiting for the workspace to process the current elicitation response.",
                    tint: .orange
                )
            }
            if let queryError, !queryError.isEmpty {
                WorkspaceBanner(
                    title: "Query failed",
                    message: queryError,
                    tint: .red
                )
            }
            if let streamError, !streamError.isEmpty {
                WorkspaceBanner(
                    title: "Live updates unavailable",
                    message: streamError,
                    tint: .orange,
                    actionTitle: "Retry Live",
                    action: onRetryStreaming
                )
            }
            if let approvalError, !approvalError.isEmpty {
                WorkspaceBanner(
                    title: "Approval action failed",
                    message: approvalError,
                    tint: .orange
                )
            }
            if let elicitationError, !elicitationError.isEmpty {
                WorkspaceBanner(
                    title: "Elicitation action failed",
                    message: elicitationError,
                    tint: .orange
                )
            }
            if let artifactError, !artifactError.isEmpty {
                WorkspaceBanner(
                    title: "Artifact refresh failed",
                    message: artifactError,
                    tint: .orange
                )
            }
        }
        .padding(.horizontal, 4)
    }
}

struct WorkspaceLoadingView: View {
    var body: some View {
        ContentUnavailableView {
            Label("Loading Conversation", systemImage: "bubble.left.and.bubble.right")
        } description: {
            Text("Refreshing transcript, approvals, and artifacts for the selected conversation.")
        } actions: {
            ProgressView()
        }
    }
}

struct ArtifactLoadingView: View {
    var body: some View {
        HStack(spacing: 10) {
            ProgressView()
            Text("Loading artifacts")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            Spacer()
        }
        .padding(12)
        .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
    }
}

struct WorkspaceBanner: View {
    let title: String
    let message: String
    let tint: Color
    var actionTitle: String? = nil
    var action: (() -> Void)? = nil

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .firstTextBaseline) {
                Text(title)
                    .font(.footnote.weight(.semibold))
                Spacer()
                if let actionTitle, let action {
                    Button(actionTitle, action: action)
                        .buttonStyle(.borderedProminent)
                        .controlSize(.small)
                }
            }
            Text(message)
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(tint.opacity(0.1), in: RoundedRectangle(cornerRadius: 12))
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(tint.opacity(0.2), lineWidth: 1)
        )
    }
}
