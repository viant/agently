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
                client: client,
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

            ToolFeedsSection(
                feeds: mergedToolFeeds(live: streamSnapshot?.feeds ?? [], persisted: conversationState?.feeds ?? []),
                conversationID: conversationState?.conversation?.conversationID,
                client: client,
                forgeRuntime: forgeRuntime
            )

            ToolFeedsSection(
                feeds: mergedToolFeeds(live: streamSnapshot?.feeds ?? [], persisted: conversationState?.feeds ?? []),
                conversationID: conversationState?.conversation?.conversationID,
                client: client,
                placement: .detached,
                sectionTitle: "Feed apps",
                forgeRuntime: forgeRuntime
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
                    },
                    feeds: mergedToolFeeds(live: streamSnapshot?.feeds ?? [], persisted: conversationState?.feeds ?? []),
                    forgeRuntime: forgeRuntime
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
                },
                feeds: mergedToolFeeds(live: streamSnapshot?.feeds ?? [], persisted: conversationState?.feeds ?? []),
                forgeRuntime: forgeRuntime
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
    let toolDetails: [TurnProgressToolDetail]
    let tokenUsage: String?
    let tokenDetails: TurnProgressTokenDetails?
    let isWaitingForUser: Bool
    let canStop: Bool

    init(
        title: String,
        detail: String,
        activity: String,
        toolProgress: String?,
        toolDetails: [TurnProgressToolDetail] = [],
        tokenUsage: String?,
        tokenDetails: TurnProgressTokenDetails? = nil,
        isWaitingForUser: Bool = false,
        canStop: Bool
    ) {
        self.title = title
        self.detail = detail
        self.activity = activity
        self.toolProgress = toolProgress
        self.toolDetails = toolDetails
        self.tokenUsage = tokenUsage
        self.tokenDetails = tokenDetails
        self.isWaitingForUser = isWaitingForUser
        self.canStop = canStop
    }
}

struct TurnProgressToolDetail: Equatable, Identifiable {
    let id: String
    let name: String
    let status: String
}

struct TurnProgressTokenModel: Equatable, Identifiable {
    let id: String
    let label: String
    let total: Int
    let input: Int?
    let output: Int?
    let cachedInput: Int?
    let reasoning: Int?
    let embedding: Int?
}

struct TurnProgressTokenDetails: Equatable {
    let scope: String
    let total: Int
    let input: Int
    let output: Int
    let cachedInput: Int
    let reasoning: Int
    let embedding: Int
    let models: [TurnProgressTokenModel]
}

func progressStatusAttributedText(_ markdown: String) -> AttributedString {
    let options = AttributedString.MarkdownParsingOptions(
        interpretedSyntax: .inlineOnlyPreservingWhitespace
    )
    let lines = markdown
        .split(whereSeparator: { $0.isNewline })
        .map { String($0).trimmingCharacters(in: .whitespacesAndNewlines) }
        .filter { !$0.isEmpty }
    var result = AttributedString()
    for (index, rawLine) in lines.enumerated() {
        if index > 0 { result.append(AttributedString("\n")) }
        let headingRange = rawLine.range(of: #"^#{1,6}\s+"#, options: .regularExpression)
        var normalized = headingRange.map { rawLine.replacingCharacters(in: $0, with: "") } ?? rawLine
        if normalized.hasPrefix("- ") || normalized.hasPrefix("* ") {
            normalized = "• " + normalized.dropFirst(2).trimmingCharacters(in: .whitespaces)
        }
        let source = headingRange == nil
            ? normalized
            : "**\(normalized.replacingOccurrences(of: "**", with: ""))**"
        result.append((try? AttributedString(markdown: source, options: options)) ?? AttributedString(normalized))
    }
    return result
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
        let tokenDetails = progressTokenDetails(groups: [], snapshot: streamSnapshot, conversationState: conversationState)
        return TurnProgressPresentation(
            title: "Stopping request",
            detail: "Stopping the current turn.",
            activity: "Stopping",
            toolProgress: nil,
            tokenUsage: formattedTokenUsage(tokenDetails),
            tokenDetails: tokenDetails,
            canStop: false
        )
    }
    guard let activeID else {
        let tokenDetails = progressTokenDetails(groups: [], snapshot: streamSnapshot, conversationState: conversationState)
        return TurnProgressPresentation(
            title: "Sending request",
            detail: "Connecting to the workspace.",
            activity: "Connecting",
            toolProgress: nil,
            tokenUsage: formattedTokenUsage(tokenDetails),
            tokenDetails: tokenDetails,
            canStop: false
        )
    }

    let groups = streamSnapshot?.liveExecutionGroupsByID.values
        .filter { ($0.turnID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? "") == activeID }
        .sorted(by: executionGroupComesBefore) ?? []
    let toolDetails = progressToolDetails(groups)
    let persistedActiveStatus = conversationState?.conversation?.turns.reversed().first(where: {
        $0.turnID.trimmingCharacters(in: .whitespacesAndNewlines) == activeID
    })?.status
    let waitingForUser = streamSnapshot?.pendingElicitation != nil || isWaitingForUserStatus(persistedActiveStatus)
    if waitingForUser {
        let tokenDetails = progressTokenDetails(groups: groups, snapshot: streamSnapshot, conversationState: conversationState)
        return TurnProgressPresentation(
            title: "Needs your input",
            detail: "Review the requested action to continue.",
            activity: "Needs your input",
            toolProgress: explicitToolProgress(groups),
            toolDetails: toolDetails,
            tokenUsage: formattedTokenUsage(tokenDetails),
            tokenDetails: tokenDetails,
            isWaitingForUser: true,
            canStop: false
        )
    }
    let activeTools = groups.flatMap(\.toolSteps).filter { step in
        isActiveExecutionStatus(step.status)
    }
    let activeModel = groups.reversed().lazy.flatMap(\.modelSteps).first(where: { step in
        isActiveExecutionStatus(step.status)
    })
    let plannerStatus = streamSnapshot?.plannerByTurnID[activeID]?.status?
        .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let activity: String
    if activeTools.count == 1 {
        activity = userFacingToolActivity(activeTools[0].toolName) ?? "Using tool"
    } else if activeTools.count > 1 {
        activity = "Calling tools"
    } else {
        activity = activeAssistantHasContent(streamSnapshot, activeTurnID: activeID)
            ? "Writing response"
            : (activeModel != nil ? "Thinking" : (!plannerStatus.isEmpty ? "Planning" : "Thinking"))
    }
    let fallbackDetail: String
    switch activity {
    case "Writing response": fallbackDetail = "Preparing the response for display."
    case "Planning": fallbackDetail = "Preparing the execution plan."
    case "Thinking": fallbackDetail = activeModel != nil
        ? "The model is analyzing the request."
        : "Planning the next step."
    default: fallbackDetail = "Using a workspace tool."
    }
    let tokenDetails = progressTokenDetails(groups: groups, snapshot: streamSnapshot, conversationState: conversationState)
    return TurnProgressPresentation(
        title: "Working on your request",
        detail: fallbackDetail,
        activity: activity,
        toolProgress: explicitToolProgress(groups),
        toolDetails: toolDetails,
        tokenUsage: formattedTokenUsage(tokenDetails),
        tokenDetails: tokenDetails,
        canStop: true
    )
}

private func progressTokenDetails(
    groups: [LiveExecutionGroup],
    snapshot: ConversationStreamSnapshot?,
    conversationState: ConversationStateResponse?
) -> TurnProgressTokenDetails? {
    let modelRows = groups.flatMap(\.modelSteps).compactMap { step -> (LiveModelStepState, ModelUsageState)? in
        guard let usage = step.usage, (usage.totalTokens ?? 0) > 0 else { return nil }
        return (step, usage)
    }
    if !modelRows.isEmpty {
        let models = modelRows.map { step, usage in
            TurnProgressTokenModel(
                id: step.modelCallID,
                label: [step.provider, step.model].compactMap { $0 }.joined(separator: "/"),
                total: usage.totalTokens ?? 0,
                input: usage.inputTokens,
                output: usage.outputTokens,
                cachedInput: usage.cachedInputTokens,
                reasoning: usage.reasoningTokens,
                embedding: usage.embeddingTokens
            )
        }.sorted { $0.total > $1.total }
        return TurnProgressTokenDetails(
            scope: "turn",
            total: modelRows.reduce(0) { $0 + ($1.1.totalTokens ?? 0) },
            input: modelRows.reduce(0) { $0 + ($1.1.inputTokens ?? 0) },
            output: modelRows.reduce(0) { $0 + ($1.1.outputTokens ?? 0) },
            cachedInput: modelRows.reduce(0) { $0 + ($1.1.cachedInputTokens ?? 0) },
            reasoning: modelRows.reduce(0) { $0 + ($1.1.reasoningTokens ?? 0) },
            embedding: modelRows.reduce(0) { $0 + ($1.1.embeddingTokens ?? 0) },
            models: models
        )
    }
    let conversationUsage = conversationState?.usage
    let input = snapshot?.usage?.inputTokens ?? conversationUsage?.totalInputTokens ?? 0
    let output = snapshot?.usage?.outputTokens ?? conversationUsage?.totalOutputTokens ?? 0
    let embedding = snapshot?.usage?.embeddingTokens ?? conversationUsage?.totalEmbeddingTokens ?? 0
    let total = snapshot?.usage?.totalTokens ?? conversationUsage?.totalTokens ?? (input + output + embedding)
    guard total > 0 else { return nil }
    let models = (conversationUsage?.models ?? []).enumerated().map { index, model in
        let role = model.executionRole?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let modelLabel = [model.provider, model.model]
            .compactMap { $0?.isEmpty == false ? $0 : nil }
            .joined(separator: "/")
        return TurnProgressTokenModel(
            id: "\(model.provider ?? ""):\(model.model):\(role):\(index)",
            label: role.isEmpty ? modelLabel : "\(modelLabel) · \(role)",
            total: model.totalTokens ?? ((model.inputTokens ?? 0) + (model.outputTokens ?? 0)),
            input: model.inputTokens,
            output: model.outputTokens,
            cachedInput: model.cachedInputTokens,
            reasoning: model.reasoningTokens,
            embedding: nil
        )
    }.sorted { $0.total > $1.total }
    return TurnProgressTokenDetails(
        scope: "conversation",
        total: total,
        input: input,
        output: output,
        cachedInput: conversationUsage?.totalCachedInputTokens ?? 0,
        reasoning: conversationUsage?.totalReasoningTokens ?? 0,
        embedding: embedding,
        models: models
    )
}

private func formattedTokenUsage(_ details: TurnProgressTokenDetails?) -> String? {
    guard let details else { return nil }
    let suffix = details.scope == "turn" ? "turn tokens" : "total tokens"
    return "\(details.total.formatted(.number.grouping(.automatic))) \(suffix)"
}

private func explicitToolProgress(_ groups: [LiveExecutionGroup]) -> String? {
    var statuses: [String: String] = [:]
    var identityComplete = true
    for group in groups {
        for planned in group.toolCallsPlanned {
            let id = planned.toolCallID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            if id.isEmpty { identityComplete = false } else if statuses[id] == nil { statuses[id] = "queued" }
        }
        for step in group.toolSteps {
            let id = step.toolCallID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            if id.isEmpty {
                identityComplete = false
            } else {
                statuses[id] = step.status?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
            }
        }
    }
    guard identityComplete, !statuses.isEmpty else { return nil }
    let doneStatuses = Set(["completed", "done", "success", "succeeded"])
    let failedStatuses = Set(["canceled", "cancelled", "declined", "error", "failed", "terminated", "timed_out", "timeout"])
    let queuedStatuses = Set(["open", "pending", "planned", "queued", "waiting"])
    let done = statuses.values.filter(doneStatuses.contains).count
    let failed = statuses.values.filter(failedStatuses.contains).count
    let queued = statuses.values.filter(queuedStatuses.contains).count
    let active = statuses.count - done - failed - queued
    var parts = ["\(done)/\(statuses.count) done"]
    if active > 0 { parts.append("\(active) active") }
    if queued > 0 { parts.append("\(queued) queued") }
    if failed > 0 { parts.append("\(failed) failed") }
    return parts.joined(separator: " · ")
}

private func progressToolDetails(_ groups: [LiveExecutionGroup]) -> [TurnProgressToolDetail] {
    var rows: [String: TurnProgressToolDetail] = [:]
    for group in groups {
        for planned in group.toolCallsPlanned {
            let id = planned.toolCallID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            guard !id.isEmpty else { continue }
            rows[id] = TurnProgressToolDetail(
                id: id,
                name: nonEmptyProgressText(planned.toolName, fallback: "Tool"),
                status: rows[id]?.status ?? "queued"
            )
        }
        for step in group.toolSteps {
            let id = step.toolCallID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            guard !id.isEmpty else { continue }
            rows[id] = TurnProgressToolDetail(
                id: id,
                name: nonEmptyProgressText(step.toolName, fallback: rows[id]?.name ?? "Tool"),
                status: nonEmptyProgressText(step.status?.lowercased(), fallback: rows[id]?.status ?? "running")
            )
        }
    }
    return rows.values.sorted {
        if $0.status != $1.status { return $0.status < $1.status }
        return $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending
    }
}

private func nonEmptyProgressText(_ value: String?, fallback: String) -> String {
    let text = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    return text.isEmpty ? fallback : text
}

private func executionGroupComesBefore(_ lhs: LiveExecutionGroup, _ rhs: LiveExecutionGroup) -> Bool {
    let left = (lhs.sequence ?? Int.min, lhs.iteration ?? Int.min)
    let right = (rhs.sequence ?? Int.min, rhs.iteration ?? Int.min)
    return left < right
}

private func isActiveExecutionStatus(_ status: String?) -> Bool {
    let normalized = status?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
    return normalized.isEmpty || ["active", "executing", "in_progress", "processing", "starting", "started", "running", "streaming"].contains(normalized)
}

private func isWaitingForUserStatus(_ status: String?) -> Bool {
    let normalized = status?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
    return ["blocked", "eliciting", "waiting_for_user"].contains(normalized)
}

private func isTerminalExecutionStatus(_ status: String?) -> Bool {
    let normalized = status?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
    return ["completed", "succeeded", "success", "failed", "canceled", "cancelled"].contains(normalized)
}

private func isPendingTurnStatus(_ status: String?) -> Bool {
    let normalized = status?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
    return ["queued", "pending", "starting", "running", "streaming", "processing", "waiting", "waiting_for_model", "waiting_for_tool", "waiting_for_user", "blocked", "eliciting"].contains(normalized)
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
    @State private var disclosure: TurnProgressDisclosure?

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            content(now: context.date)
        }
        .onChange(of: "\(presentation.title)|\(presentation.activity)") { _, _ in
            phaseStartedAt = Date()
        }
    }

    private func content(now: Date) -> some View {
        HStack(alignment: .center, spacing: 8) {
            if presentation.isWaitingForUser {
                Image(systemName: "exclamationmark.circle.fill")
                    .foregroundStyle(Color.orange)
                    .accessibilityLabel("Needs your input")
            } else {
                ProgressView()
                    .controlSize(.small)
                    .accessibilityLabel("Request in progress")
            }
            VStack(alignment: .leading, spacing: 3) {
                Text(presentation.title)
                    .font(.footnote.weight(.semibold))
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 5) {
                        Text(activityLabel(now: now))
                            .foregroundStyle(Color.blue)
                            .padding(.horizontal, 7)
                            .padding(.vertical, 2)
                            .background(Color.blue.opacity(0.10), in: Capsule())
                        if let toolProgress = presentation.toolProgress {
                            Button(toolProgress) { disclosure = .tools }
                                .buttonStyle(.plain)
                                .foregroundStyle(Color.purple)
                                .padding(.horizontal, 7)
                                .padding(.vertical, 2)
                                .background(Color.purple.opacity(0.10), in: Capsule())
                        }
                        if let tokenUsage = presentation.tokenUsage {
                            Button(tokenUsage) { disclosure = .tokens }
                                .buttonStyle(.plain)
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
        .popover(item: $disclosure) { selected in
            switch selected {
            case .tools:
                VStack(alignment: .leading, spacing: 8) {
                    Text("Tool progress").font(.headline)
                    if presentation.toolDetails.isEmpty {
                        Text("Tool identities are not available yet.").foregroundStyle(.secondary)
                    } else {
                        ForEach(presentation.toolDetails) { tool in
                            HStack {
                                Text(userFacingToolActivity(tool.name) ?? tool.name)
                                Spacer()
                                Text(tool.status.replacingOccurrences(of: "_", with: " ").capitalized)
                                    .fontWeight(.semibold)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
                .padding()
                .frame(minWidth: 280)
            case .tokens:
                if let details = presentation.tokenDetails {
                VStack(alignment: .leading, spacing: 8) {
                    Text(details.scope == "turn" ? "This turn" : "Conversation total").font(.headline)
                    tokenDetailRow("Total", details.total)
                    tokenDetailRow("Input", details.input)
                    tokenDetailRow("Output", details.output)
                    if details.cachedInput > 0 { tokenDetailRow("Cached input", details.cachedInput) }
                    if details.reasoning > 0 { tokenDetailRow("Reasoning", details.reasoning) }
                    if details.embedding > 0 { tokenDetailRow("Embedding", details.embedding) }
                    ForEach(details.models) { model in
                        VStack(alignment: .leading, spacing: 4) {
                            Divider()
                            Text(model.label.isEmpty ? "Model" : model.label).font(.subheadline.weight(.semibold))
                            tokenDetailRow("Total", model.total)
                            tokenDetailRow("Input", model.input)
                            tokenDetailRow("Output", model.output)
                            tokenDetailRow("Cached input", model.cachedInput)
                            tokenDetailRow("Reasoning", model.reasoning)
                            tokenDetailRow("Embedding", model.embedding)
                        }
                    }
                }
                .padding()
                .frame(minWidth: 260)
                } else {
                    Text("Token details are not available.").padding()
                }
            }
        }
    }

    @ViewBuilder
    private func tokenDetailRow(_ label: String, _ value: Int) -> some View {
        HStack {
            Text(label)
            Spacer()
            Text(value.formatted(.number.grouping(.automatic))).fontWeight(.semibold)
        }
    }

    @ViewBuilder
    private func tokenDetailRow(_ label: String, _ value: Int?) -> some View {
        HStack {
            Text(label)
            Spacer()
            if let value {
                Text(value.formatted(.number.grouping(.automatic))).fontWeight(.semibold)
            } else {
                Text("Not reported").foregroundStyle(.secondary)
            }
        }
    }

    private func activityLabel(now: Date) -> String {
        let seconds = max(0, Int(now.timeIntervalSince(phaseStartedAt)))
        guard seconds >= 5 else { return presentation.activity }
        let elapsed = seconds < 60 ? "\(seconds)s" : "\(seconds / 60)m \(seconds % 60)s"
        return "\(presentation.activity) · \(elapsed)"
    }
}

private enum TurnProgressDisclosure: String, Identifiable {
    case tools
    case tokens
    var id: String { rawValue }
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
