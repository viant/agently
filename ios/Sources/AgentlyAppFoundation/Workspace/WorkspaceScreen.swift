import SwiftUI
import AgentlySDK
import Foundation
import ForgeIOSRuntime
import ForgeIOSUI

public struct WorkspaceScreen: View {
    private enum DetailPane: String, CaseIterable, Identifiable {
        case transcript
        case execution

        var id: String { rawValue }

        var title: String {
            switch self {
            case .transcript:
                return "Transcript"
            case .execution:
                return "Execution details"
            }
        }
    }

    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @State private var isArtifactSectionExpanded = false
    @State private var isApprovalSectionExpanded = true
    @State private var hostedWorkspaceDisplayMode: HostedWorkspaceDisplayMode = .standard
    @State private var hostedWorkspaceMeasuredHeight: CGFloat = 0
    @State private var hostedWindowContentMeasuredHeight: CGFloat = 0
    @State private var transcriptMeasuredHeight: CGFloat = 0
    @State private var selectedDetailPane: DetailPane = .transcript
    let metadata: WorkspaceMetadata?
    let selectedAgentID: String?
    let availableAgents: [WorkspaceAgentOption]
    let activeGoal: Goal?
    let hostedWorkspaceRestoreState: HostedWorkspaceRestoreState?
    let conversationState: ConversationStateResponse?
    let streamSnapshot: ConversationStreamSnapshot?
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
        activeGoal: Goal? = nil,
        hostedWorkspaceRestoreState: HostedWorkspaceRestoreState? = nil,
        conversationState: ConversationStateResponse? = nil,
        streamSnapshot: ConversationStreamSnapshot? = nil,
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
        self.activeGoal = activeGoal
        self.hostedWorkspaceRestoreState = hostedWorkspaceRestoreState
        self.conversationState = conversationState
        self.streamSnapshot = streamSnapshot
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
        let showsSidebar = !approvals.isEmpty || !artifacts.isEmpty
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
            if let activeGoal {
                GoalSummaryCard(goal: activeGoal)
                    .padding(.horizontal)
                    .padding(.top, 12)
            }
            HStack(alignment: .top, spacing: 20) {
                mainPane
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)

                if showsSidebar {
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
        .onChange(of: hostedWorkspaceIdentity) { _, _ in
            hostedWorkspaceDisplayMode = defaultHostedWorkspaceDisplayMode
            hostedWorkspaceMeasuredHeight = 0
            hostedWindowContentMeasuredHeight = 0
            transcriptMeasuredHeight = 0
            selectedDetailPane = .transcript
        }
        .onAppear {
            if showsHostedWorkspace && hostedWorkspaceDisplayMode == .standard {
                hostedWorkspaceDisplayMode = defaultHostedWorkspaceDisplayMode
            }
        }
        .onPreferenceChange(HostedWorkspaceContentHeightPreferenceKey.self) { newHeight in
            guard newHeight > 0 else { return }
            hostedWorkspaceMeasuredHeight = newHeight
        }
        .onPreferenceChange(WindowContentHeightPreferenceKey.self) { newHeight in
            guard newHeight > 0 else { return }
            hostedWindowContentMeasuredHeight = newHeight
        }
        .onPreferenceChange(TranscriptContentHeightPreferenceKey.self) { newHeight in
            guard newHeight > 0 else { return }
            transcriptMeasuredHeight = newHeight
        }
    }

    private var mainPane: some View {
        let usesWorkspaceFocusedLayout = horizontalSizeClass == .regular &&
            showsHostedWorkspace &&
            hostedWorkspaceDisplayMode == .expanded
        return VStack(spacing: 16) {
            WorkspaceStatusSection(
                isSending: isSending,
                conversationState: conversationState,
                streamSnapshot: streamSnapshot,
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

            GeometryReader { proxy in
                let availableHeight = max(proxy.size.height, 0)
                let layoutPlan = resolveHostedWorkspaceLayoutPlan(
                    availableHeight: availableHeight,
                    showsHostedWorkspace: showsHostedWorkspace,
                    displayMode: hostedWorkspaceDisplayMode,
                    isRegularWidth: horizontalSizeClass == .regular,
                    transcriptMeasuredHeight: transcriptMeasuredHeight,
                    hostedWorkspaceMeasuredHeight: hostedWorkspaceMeasuredHeight,
                    hostedWindowContentMeasuredHeight: hostedWindowContentMeasuredHeight,
                    activeWindow: activeHostedWorkspaceWindow
                )

                VStack(spacing: 16) {
                    if showsHostedWorkspace {
                        HostedWorkspaceSection(
                            restoreState: hostedWorkspaceRestoreState,
                            forgeRuntime: forgeRuntime,
                            displayMode: $hostedWorkspaceDisplayMode
                        )
                        .frame(
                            maxWidth: .infinity,
                            minHeight: usesWorkspaceFocusedLayout ? availableHeight : layoutPlan.workspaceHeight,
                            maxHeight: usesWorkspaceFocusedLayout ? availableHeight : layoutPlan.workspaceHeight,
                            alignment: .topLeading
                        )
                        .clipped()
                    }

                    if !usesWorkspaceFocusedLayout {
                        if hasExecutionDetails {
                            Picker("Conversation detail", selection: $selectedDetailPane) {
                                ForEach(DetailPane.allCases) { pane in
                                    Text(pane.title).tag(pane)
                                }
                            }
                            .pickerStyle(.segmented)
                            .padding(.horizontal, 4)
                        }

                        activeDetailCard
                            .frame(maxWidth: .infinity, minHeight: layoutPlan.transcriptHeight, maxHeight: layoutPlan.transcriptHeight, alignment: .topLeading)
                    }
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
            .agentlyDismissKeyboardOnInteraction()

            if !usesWorkspaceFocusedLayout {
                ComposerScreen(
                    runtime: composerRuntime,
                    isSending: isSending,
                    density: hostedWorkspaceDisplayMode == .minimized ? .compact : .regular,
                    onSend: onSend
                )
                    .padding(.horizontal, 4)
            }
        }
    }

    private var hasHostedWorkspace: Bool {
        hostedWorkspaceRestoreState != nil
    }

    private var showsHostedWorkspace: Bool {
        hasHostedWorkspace && hostedWorkspaceDisplayMode != .closed
    }

    private var hostedWorkspaceIdentity: String {
        let windowIdentity = hostedWorkspaceRestoreState?.selectedWindowId
            ?? hostedWorkspaceRestoreState?.windows.last?.windowId
            ?? conversationState?.conversation?.conversationID
            ?? "none"
        let turnIdentity = conversationState?.conversation?.turns.last?.turnID ?? "no-turn"
        return "\(windowIdentity)#\(turnIdentity)"
    }

    private var defaultHostedWorkspaceDisplayMode: HostedWorkspaceDisplayMode {
        resolveDefaultHostedWorkspaceDisplayMode(isRegularWidth: horizontalSizeClass == .regular)
    }

    private var activeHostedWorkspaceWindow: WorkspaceWindowSnapshot? {
        resolveActiveHostedWorkspaceWindow(
            restoreState: hostedWorkspaceRestoreState
        )
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
					client: client,
                    conversationID: conversationState?.conversation?.conversationID,
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
    private var activeDetailCard: some View {
        switch selectedDetailPane {
        case .transcript:
            transcriptCard
        case .execution:
            executionCard
        }
    }

    private var executionCard: some View {
        ExecutionInspectorSection(state: conversationState)
            .padding(.vertical, 8)
            .background(Color.secondary.opacity(0.05), in: RoundedRectangle(cornerRadius: 20))
            .overlay(
                RoundedRectangle(cornerRadius: 20)
                    .stroke(Color.secondary.opacity(0.12), lineWidth: 1)
            )
    }

    private var hasExecutionDetails: Bool {
        conversationState?.conversation?.turns.contains(where: { !($0.execution?.pages.isEmpty ?? true) }) == true
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
				client: client,
                conversationID: conversationState?.conversation?.conversationID,
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

struct TurnProgressPresentation: Equatable {
    let title: String
    let detail: String
    let activity: String
    let toolProgress: String?
    let tokenUsage: String?
    let canStop: Bool
}

func turnProgressPresentation(
    isSending: Bool,
    activeTurnID: String?,
    isStoppingTurn: Bool,
    conversationState: ConversationStateResponse?,
    streamSnapshot: ConversationStreamSnapshot?
) -> TurnProgressPresentation? {
    let streamedTurnID = streamSnapshot?.activeTurnID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let fallbackTurnID = activeTurnID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let persistedPendingTurnID = conversationState?.conversation?.turns.reversed().first(where: { turn in
        isPendingTurnStatus(turn.status)
    })?.turnID.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let activeID: String? = !streamedTurnID.isEmpty
        ? streamedTurnID
        : (!fallbackTurnID.isEmpty
            ? fallbackTurnID
            : (!persistedPendingTurnID.isEmpty ? persistedPendingTurnID : nil))
    guard isSending || activeID != nil || isStoppingTurn else { return nil }
    if isStoppingTurn {
        return TurnProgressPresentation(
            title: "Stopping request",
            detail: "Stopping the current turn.",
            activity: "Stopping",
            toolProgress: nil,
            tokenUsage: formattedConversationTokenUsage(streamSnapshot, conversationState: conversationState),
            canStop: false
        )
    }
    guard let activeID else {
        return TurnProgressPresentation(
            title: "Sending request",
            detail: "Connecting to the workspace.",
            activity: "Connecting",
            toolProgress: nil,
            tokenUsage: formattedConversationTokenUsage(streamSnapshot, conversationState: conversationState),
            canStop: false
        )
    }

    let groups = streamSnapshot?.liveExecutionGroupsByID.values
        .filter { ($0.turnID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? "") == activeID }
        .sorted(by: executionGroupComesBefore) ?? []
    let latestGroup = groups.last
    let narrationCandidates = [
        latestGroup?.narration,
        streamSnapshot?.bufferedMessages.reversed().first(where: { message in
            message.role.lowercased() == "assistant" &&
                message.turnID?.trimmingCharacters(in: .whitespacesAndNewlines) == activeID &&
                message.narration?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
        })?.narration,
        conversationState?.conversation?.turns.reversed().first(where: { turn in
            turn.turnID == activeID || isPendingTurnStatus(turn.status)
        })?.assistant?.narration?.content
    ]
    let narration = narrationCandidates
        .compactMap(sanitizeAssistantTranscriptText)
        .first(where: { !$0.isEmpty })

    let liveToolName = groups.reversed().lazy.flatMap(\.toolSteps).first(where: { step in
        isActiveExecutionStatus(step.status)
    })?.toolName
    let plannedToolName = latestGroup?.toolCallsPlanned.reversed().compactMap(\.toolName).first
    let bufferedToolName = streamSnapshot?.bufferedMessages.reversed().first(where: { message in
        message.turnID?.trimmingCharacters(in: .whitespacesAndNewlines) == activeID &&
            message.toolName?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
    })?.toolName
    let activeModel = groups.reversed().lazy.flatMap(\.modelSteps).first(where: { step in
        isActiveExecutionStatus(step.status)
    })
    let plannerStatus = streamSnapshot?.plannerByTurnID[activeID]?.status?
        .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let activity = userFacingToolActivity(liveToolName ?? plannedToolName ?? bufferedToolName)
        ?? (activeAssistantHasContent(streamSnapshot, activeTurnID: activeID)
            ? "Writing response"
            : (activeModel != nil ? "Thinking" : (!plannerStatus.isEmpty ? "Planning" : "Thinking")))
    let fallbackDetail: String
    switch activity {
    case "Writing response": fallbackDetail = "Preparing the response for display."
    case "Planning": fallbackDetail = "Preparing the execution plan."
    case "Thinking": fallbackDetail = activeModel != nil
        ? "The model is analyzing the request."
        : "Planning the next step."
    default: fallbackDetail = "Using a workspace tool."
    }
    let toolSteps = groups.flatMap(\.toolSteps)
    let plannedToolCount = groups.reduce(0) { $0 + $1.toolCallsPlanned.count }
    let toolTotal = max(toolSteps.count, plannedToolCount)
    let toolCompleted = min(toolTotal, toolSteps.filter { isTerminalExecutionStatus($0.status) }.count)
    return TurnProgressPresentation(
        title: "Working on your request",
        detail: narration ?? fallbackDetail,
        activity: activity,
        toolProgress: toolTotal > 0 ? "Tools \(toolCompleted)/\(toolTotal)" : nil,
        tokenUsage: formattedConversationTokenUsage(streamSnapshot, conversationState: conversationState),
        canStop: true
    )
}

private func formattedConversationTokenUsage(
    _ snapshot: ConversationStreamSnapshot?,
    conversationState: ConversationStateResponse?
) -> String? {
    let streamedTotal = snapshot?.usage?.totalTokens ?? 0
    let persistedTotal = (conversationState?.usage?.totalInputTokens ?? 0) +
        (conversationState?.usage?.totalOutputTokens ?? 0)
    let total = streamedTotal > 0 ? streamedTotal : persistedTotal
    guard total > 0 else { return nil }
    return "\(total.formatted(.number.grouping(.automatic))) tokens"
}

private func executionGroupComesBefore(_ lhs: LiveExecutionGroup, _ rhs: LiveExecutionGroup) -> Bool {
    let left = (lhs.sequence ?? Int.min, lhs.iteration ?? Int.min)
    let right = (rhs.sequence ?? Int.min, rhs.iteration ?? Int.min)
    return left < right
}

private func isActiveExecutionStatus(_ status: String?) -> Bool {
    let normalized = status?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
    return normalized.isEmpty || ["queued", "pending", "starting", "started", "running", "streaming", "processing"].contains(normalized)
}

private func isTerminalExecutionStatus(_ status: String?) -> Bool {
    let normalized = status?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
    return ["completed", "succeeded", "success", "failed", "canceled", "cancelled"].contains(normalized)
}

private func isPendingTurnStatus(_ status: String?) -> Bool {
    let normalized = status?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
    return ["queued", "pending", "starting", "running", "streaming", "processing", "waiting", "waiting_for_model", "waiting_for_tool"].contains(normalized)
}

private func activeAssistantHasContent(_ snapshot: ConversationStreamSnapshot?, activeTurnID: String) -> Bool {
    snapshot?.bufferedMessages.contains(where: { message in
        message.role.lowercased() == "assistant" &&
            message.turnID?.trimmingCharacters(in: .whitespacesAndNewlines) == activeTurnID &&
            sanitizeAssistantTranscriptText(message.content)?.isEmpty == false
    }) == true
}

func userFacingToolActivity(_ rawName: String?) -> String? {
    let raw = rawName?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    guard !raw.isEmpty else { return nil }
    let lower = raw.lowercased()
    if lower.contains("reporting") && lower.contains("export") { return "Preparing report export" }
    if lower.contains("reporting") { return "Reporting" }
    if lower.contains("diagnostic") { return "Delivery diagnostics" }
    if lower.contains("forecast") { return "Forecasting" }
    if lower.contains("hierarchy") { return "Loading hierarchy" }
    if lower.contains("resource") { return "Reading workspace" }
    let leaf = raw.split(whereSeparator: { $0 == "/" || $0 == ":" }).last.map(String.init) ?? raw
    let spaced = leaf
        .replacingOccurrences(of: "_", with: " ")
        .replacingOccurrences(of: "-", with: " ")
        .replacingOccurrences(of: "([a-z0-9])([A-Z])", with: "$1 $2", options: .regularExpression)
        .trimmingCharacters(in: .whitespacesAndNewlines)
    return spaced.isEmpty ? "Workspace tool" : spaced.capitalized
}

private struct TurnProgressBanner: View {
    let presentation: TurnProgressPresentation
    let onStop: () -> Void
    @State private var phaseStartedAt = Date()

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            content(now: context.date)
        }
        .onChange(of: "\(presentation.title)|\(presentation.detail)|\(presentation.activity)") { _, _ in
            phaseStartedAt = Date()
        }
    }

    private func content(now: Date) -> some View {
        HStack(alignment: .center, spacing: 8) {
            ProgressView()
                .controlSize(.small)
                .accessibilityLabel("Request in progress")
            VStack(alignment: .leading, spacing: 3) {
                Text(presentation.title)
                    .font(.footnote.weight(.semibold))
                Text(presentation.detail)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 5) {
                        Text(activityLabel(now: now))
                            .foregroundStyle(Color.blue)
                            .padding(.horizontal, 7)
                            .padding(.vertical, 2)
                            .background(Color.blue.opacity(0.10), in: Capsule())
                        if let toolProgress = presentation.toolProgress {
                            Text(toolProgress)
                                .foregroundStyle(Color.purple)
                                .padding(.horizontal, 7)
                                .padding(.vertical, 2)
                                .background(Color.purple.opacity(0.10), in: Capsule())
                        }
                        if let tokenUsage = presentation.tokenUsage {
                            Text(tokenUsage)
                                .foregroundStyle(.secondary)
                                .padding(.horizontal, 7)
                                .padding(.vertical, 2)
                                .background(Color.secondary.opacity(0.08), in: Capsule())
                        }
                    }
                    .font(.caption2.weight(.semibold))
                }
            }
            Spacer(minLength: 4)
            if presentation.canStop {
                Button(action: onStop) {
                    Image(systemName: "stop.fill")
                        .font(.caption.weight(.bold))
                        .foregroundStyle(Color.red)
                        .frame(width: 30, height: 30)
                        .background(Color.red.opacity(0.10), in: Circle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Stop current request")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 10)
        .padding(.vertical, 8)
        .background(Color.blue.opacity(0.08), in: RoundedRectangle(cornerRadius: 14))
        .overlay(RoundedRectangle(cornerRadius: 14).stroke(Color.blue.opacity(0.20), lineWidth: 1))
    }

    private func activityLabel(now: Date) -> String {
        let seconds = max(0, Int(now.timeIntervalSince(phaseStartedAt)))
        guard seconds >= 5 else { return presentation.activity }
        let elapsed = seconds < 60 ? "\(seconds)s" : "\(seconds / 60)m \(seconds % 60)s"
        return "\(presentation.activity) · \(elapsed)"
    }
}

struct WorkspaceStatusSection: View {
    let isSending: Bool
    let conversationState: ConversationStateResponse?
    let streamSnapshot: ConversationStreamSnapshot?
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
            if let progress = turnProgressPresentation(
                isSending: isSending,
                activeTurnID: activeTurnID,
                isStoppingTurn: isStoppingTurn,
                conversationState: conversationState,
                streamSnapshot: streamSnapshot
            ) {
                TurnProgressBanner(presentation: progress, onStop: onCancelTurn)
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
