import SwiftUI
import AgentlySDK
import ForgeIOSRuntime

private enum PhoneWorkspacePaneSelection: Hashable {
    case workspace
    case conversation
}

public struct PhoneWorkspaceScreen: View {
    @State private var isArtifactSectionExpanded = false
    @State private var isApprovalSectionExpanded = true
    @State private var selectedPane: PhoneWorkspacePaneSelection = .conversation
    @State private var hadWorkspaceSurface = false
    @State private var isHostedWorkspacePresented = false
    @State private var lastPresentedHostedWindowID: String?
    let metadata: WorkspaceMetadata?
    let selectedAgentID: String?
    let availableAgents: [WorkspaceAgentOption]
    let hostedWorkspaceRestoreStateOverride: HostedWorkspaceRestoreState?
    let conversationState: ConversationStateResponse?
    let transcript: [ChatTranscriptEntry]
    let client: AgentlyClient
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
        hostedWorkspaceRestoreState: HostedWorkspaceRestoreState? = nil,
        conversationState: ConversationStateResponse? = nil,
        transcript: [ChatTranscriptEntry],
        client: AgentlyClient,
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
        self.hostedWorkspaceRestoreStateOverride = hostedWorkspaceRestoreState
        self.conversationState = conversationState
        self.transcript = transcript
        self.client = client
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
                onSelectAgent: onSelectAgent,
                showStarterTasks: transcript.isEmpty && !isLoadingConversation,
                onSelectStarterTask: { task in
                    let prompt = (task.prompt ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                    if !prompt.isEmpty {
                        composerRuntime.query = prompt
                    }
                }
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
            if hasHostedWorkspace {
                hostedWorkspaceShortcut
                    .padding(.horizontal)
                    .padding(.top, 12)
                    .padding(.bottom, 12)
            }
            if showsPanePicker {
                Picker("Content", selection: $selectedPane) {
                    Text("Workspace").tag(PhoneWorkspacePaneSelection.workspace)
                    Text("Conversation").tag(PhoneWorkspacePaneSelection.conversation)
                }
                .pickerStyle(.segmented)
                .padding(.horizontal)
                .padding(.top, 12)
                .padding(.bottom, 12)
            }
            Group {
                switch visiblePane {
                case .workspace:
                    workspacePane
                case .conversation:
                    conversationPane
                }
            }
            ComposerScreen(runtime: composerRuntime, isSending: isSending, onSend: onSend)
        }
        .onAppear {
            syncPaneSelection(for: hasWorkspaceSurface)
            syncHostedWorkspacePresentation(for: hostedWorkspaceWindowID)
        }
        .onChange(of: hasWorkspaceSurface) { _, newValue in
            syncPaneSelection(for: newValue)
        }
        .onChange(of: hostedWorkspaceWindowID) { _, newValue in
            syncHostedWorkspacePresentation(for: newValue)
        }
        .navigationDestination(isPresented: $isHostedWorkspacePresented) {
            hostedWorkspaceDestination
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

    private var hasHostedWorkspace: Bool {
        hostedWorkspaceRestoreState != nil
    }

    private var hasWorkspaceSurface: Bool {
        hasHostedWorkspace || !approvals.isEmpty || !artifacts.isEmpty || isLoadingArtifacts
    }

    private var showsPanePicker: Bool {
        !hasHostedWorkspace && hasWorkspaceSurface && !transcript.isEmpty
    }

    private var visiblePane: PhoneWorkspacePaneSelection {
        if hasHostedWorkspace {
            return .conversation
        }
        return hasWorkspaceSurface ? selectedPane : .conversation
    }

    @ViewBuilder
    private var workspacePane: some View {
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
                    ArtifactLoadingView()
                }

                if !hasHostedWorkspace && approvals.isEmpty && artifacts.isEmpty && !isLoadingArtifacts {
                    ContentUnavailableView(
                        "Workspace Ready",
                        systemImage: "rectangle.topthird.inset.filled",
                        description: Text("Recommendation views and workspace approvals will appear here.")
                    )
                    .frame(maxWidth: .infinity)
                    .padding(.top, 24)
                }
            }
            .padding(.horizontal)
            .padding(.bottom, 12)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    @ViewBuilder
    private var hostedWorkspaceShortcut: some View {
        Button {
            isHostedWorkspacePresented = true
        } label: {
            HStack(spacing: 10) {
                Image(systemName: "rectangle.topthird.inset.filled")
                    .font(.headline)
                VStack(alignment: .leading, spacing: 2) {
                    Text(hostedWorkspaceTitle ?? "Open Workspace")
                        .font(.headline)
                    Text("Open the active workspace view")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Image(systemName: "chevron.right")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(.secondary)
            }
            .padding(14)
            .background(Color.secondary.opacity(0.05), in: RoundedRectangle(cornerRadius: 16))
            .overlay(
                RoundedRectangle(cornerRadius: 16)
                    .stroke(Color.secondary.opacity(0.12), lineWidth: 1)
            )
        }
        .buttonStyle(.plain)
    }

    @ViewBuilder
    private var hostedWorkspaceDestination: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
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
                HostedWorkspaceSection(
                    restoreState: hostedWorkspaceRestoreState,
                    conversationState: conversationState,
                    forgeRuntime: forgeRuntime,
                    showTitle: false,
                    displayMode: .constant(.standard)
                )
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
                    ArtifactLoadingView()
                }
            }
            .padding(.horizontal)
            .padding(.vertical, 12)
        }
        .navigationTitle(hostedWorkspaceTitle ?? "Workspace")
        .modifier(InlineNavigationTitleDisplayMode())
    }

    @ViewBuilder
    private var conversationPane: some View {
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

    private var hostedWorkspaceRestoreState: HostedWorkspaceRestoreState? {
        if let hostedWorkspaceRestoreStateOverride {
            return hostedWorkspaceRestoreStateOverride
        }
        guard let conversationState else {
            return nil
        }
        return deriveHostedWorkspaceRestoreState(from: conversationState)
    }

    private var hostedWorkspaceWindowID: String? {
        guard let restoreState = hostedWorkspaceRestoreState else {
            return nil
        }
        return restoreState.selectedWindowId ?? restoreState.windows.last?.windowId
    }

    private var hostedWorkspaceTitle: String? {
        guard let restoreState = hostedWorkspaceRestoreState else {
            return nil
        }
        let targetWindowID = hostedWorkspaceWindowID
        return restoreState.windows.first(where: { $0.windowId == targetWindowID })?.windowTitle
            ?? restoreState.windows.last?.windowTitle
    }

    private func syncPaneSelection(for hasWorkspaceSurface: Bool) {
        if hasHostedWorkspace {
            selectedPane = .conversation
            hadWorkspaceSurface = hasWorkspaceSurface
            return
        }
        if hasWorkspaceSurface && !hadWorkspaceSurface {
            selectedPane = .workspace
        }
        if !hasWorkspaceSurface {
            selectedPane = .conversation
        }
        hadWorkspaceSurface = hasWorkspaceSurface
    }

    private func syncHostedWorkspacePresentation(for windowID: String?) {
        let normalized = windowID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !normalized.isEmpty else {
            isHostedWorkspacePresented = false
            return
        }
        guard normalized != lastPresentedHostedWindowID else {
            return
        }
        lastPresentedHostedWindowID = normalized
        isHostedWorkspacePresented = true
    }
}

private struct InlineNavigationTitleDisplayMode: ViewModifier {
    func body(content: Content) -> some View {
#if os(iOS)
        content.navigationBarTitleDisplayMode(.inline)
#else
        content
#endif
    }
}
