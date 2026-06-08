import SwiftUI
import AgentlySDK

public struct AppShellView: View {
    @ObservedObject private var runtime: AppRuntime
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @State private var isShowingSettings = false
    @State private var conversationSearchText = ""
    @State private var compactNavigationPath: [String] = []
    @State private var compactShowsStarterSurface = false
    @State private var regularColumnVisibility: NavigationSplitViewVisibility = .all
    @State private var pendingConversationDeletion: Conversation?

    public init(runtime: AppRuntime) {
        self.runtime = runtime
    }

    public var body: some View {
        Group {
            if horizontalSizeClass == .compact {
                compactShell
            } else {
                regularShell
            }
        }
        .toolbar {
            ToolbarItemGroup(placement: ToolbarItemPlacement.actionShellToolbarPlacement) {
                Button {
                    isShowingSettings = true
                } label: {
                    Label("Settings", systemImage: "gearshape")
                }

                Button {
                    compactNavigationPath = []
                    conversationSearchText = ""
                    compactShowsStarterSurface = true
                    if horizontalSizeClass == .regular {
                        regularColumnVisibility = .detailOnly
                    }
                    runtime.startNewConversation()
                } label: {
                    Label("New Chat", systemImage: "square.and.pencil")
                }

                Button {
                    Task { await runtime.refreshConversationList() }
                } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
            }
        }
        .settingsSheet(isPresented: $isShowingSettings, runtime: runtime)
        .alert(
            "Delete Conversation?",
            isPresented: Binding(
                get: { pendingConversationDeletion != nil },
                set: { if !$0 { pendingConversationDeletion = nil } }
            ),
            presenting: pendingConversationDeletion
        ) { conversation in
            Button("Delete", role: .destructive) {
                let conversationID = conversation.id
                pendingConversationDeletion = nil
                Task { await runtime.deleteConversation(conversationID: conversationID) }
            }
            Button("Cancel", role: .cancel) {
                pendingConversationDeletion = nil
            }
        } message: { conversation in
            let title = conversation.title?.trimmingCharacters(in: .whitespacesAndNewlines)
            if let title, !title.isEmpty {
                Text("Delete \"\(title)\" and its conversation history? This action can’t be undone.")
            } else {
                Text("Delete this conversation and its history? This action can’t be undone.")
            }
        }
    }

    private var regularShell: some View {
        NavigationSplitView(columnVisibility: $regularColumnVisibility) {
            ConversationListView(
                conversations: runtime.state.conversations,
                activeConversationID: runtime.state.activeConversationID,
                selection: nil,
                usesNavigationDestination: false,
                searchText: $conversationSearchText,
                isRefreshing: runtime.state.isRefreshingConversations,
                workspaceTitle: conversationsWorkspaceTitle,
                metadata: runtime.state.workspaceMetadata,
                selectedAgentID: runtime.selectedAgentOption?.id,
                availableAgents: runtime.availableAgentOptions,
                composerRuntime: runtime.composerRuntime,
                isSending: runtime.isQueryBusy,
                showsStarterSurfaceOverride: compactShowsStarterSurface,
                onRefresh: {
                    await runtime.refreshConversationList()
                },
                onSelectConversation: { conversationID in
                    compactShowsStarterSurface = false
                    regularColumnVisibility = .detailOnly
                    Task { await runtime.selectConversation(conversationID) }
                },
                onSelectAgent: { agentID in
                    runtime.selectPreferredAgent(agentID)
                },
                onSelectStarterTask: { task in
                    let prompt = (task.prompt ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                    if !prompt.isEmpty {
                        compactShowsStarterSurface = true
                        runtime.composerRuntime.query = prompt
                    }
                },
                onRequestDeleteConversation: { conversation in
                    pendingConversationDeletion = conversation
                },
                onSend: { Task { await runtime.sendCurrentQuery() } }
            )
        } detail: {
            if runtime.state.activeConversationID == nil {
                EmptyConversationDetailView(
                    workspaceTitle: runtime.state.workspaceMetadata?.workspaceRoot?.workspaceDisplayTitle,
                    metadata: runtime.state.workspaceMetadata,
                    selectedAgentID: runtime.selectedAgentOption?.id,
                    availableAgents: runtime.availableAgentOptions,
                    composerRuntime: runtime.composerRuntime,
                    isSending: runtime.isQueryBusy,
                    onSelectAgent: { runtime.selectPreferredAgent($0) },
                    onSelectStarterTask: { task in
                        let prompt = (task.prompt ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                        if !prompt.isEmpty {
                            runtime.composerRuntime.query = prompt
                        }
                    },
                    onSend: { Task { await runtime.sendCurrentQuery() } }
                )
            } else {
                ChatScreens(runtime: runtime)
                    .id(runtime.state.activeConversationID ?? "chat-empty")
                    .task(id: runtime.state.activeConversationID) {
                        if let conversationID = runtime.state.activeConversationID,
                           !conversationID.isEmpty {
                            await runtime.selectConversation(conversationID)
                        }
                    }
            }
        }
        .navigationSplitViewStyle(.prominentDetail)
        .id("split-\(runtime.state.activeConversationID ?? "none")")
        .task(id: runtime.state.activeConversationID) {
            let hasActiveConversation = !(runtime.state.activeConversationID?
                .trimmingCharacters(in: .whitespacesAndNewlines)
                .isEmpty ?? true)
            if hasActiveConversation, regularColumnVisibility == .all {
                regularColumnVisibility = .detailOnly
            } else if !hasActiveConversation {
                regularColumnVisibility = .all
            }
        }
    }

    private var compactShell: some View {
        NavigationStack(path: $compactNavigationPath) {
            ConversationListView(
                conversations: runtime.state.conversations,
                activeConversationID: runtime.state.activeConversationID,
                selection: nil,
                usesNavigationDestination: true,
                searchText: $conversationSearchText,
                isRefreshing: runtime.state.isRefreshingConversations,
                workspaceTitle: conversationsWorkspaceTitle,
                metadata: runtime.state.workspaceMetadata,
                selectedAgentID: runtime.selectedAgentOption?.id,
                availableAgents: runtime.availableAgentOptions,
                composerRuntime: runtime.composerRuntime,
                isSending: runtime.isQueryBusy,
                showsStarterSurfaceOverride: compactShowsStarterSurface,
                onRefresh: {
                    await runtime.refreshConversationList()
                },
                onSelectConversation: { conversationID in
                    compactShowsStarterSurface = false
                    Task { await runtime.selectConversation(conversationID) }
                },
                onSelectAgent: { agentID in
                    runtime.selectPreferredAgent(agentID)
                },
                onSelectStarterTask: { task in
                    let prompt = (task.prompt ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                    if !prompt.isEmpty {
                        compactShowsStarterSurface = true
                        runtime.composerRuntime.query = prompt
                    }
                },
                onRequestDeleteConversation: { conversation in
                    pendingConversationDeletion = conversation
                },
                onSend: { Task { await runtime.sendCurrentQuery() } }
            )
            .navigationDestination(for: String.self) { conversationID in
                ChatScreens(runtime: runtime)
                    .id(conversationID)
                    .task(id: conversationID) {
                        if runtime.state.activeConversationID != conversationID {
                            await runtime.selectConversation(conversationID)
                        }
                    }
            }
            .task(id: runtime.state.activeConversationID) {
                syncCompactNavigationPath()
            }
            .onChange(of: compactNavigationPath) { _, newValue in
                if newValue.isEmpty, runtime.state.activeConversationID != nil {
                    compactShowsStarterSurface = false
                }
            }
        }
    }

    private func syncCompactNavigationPath() {
        let activeConversationID = runtime.state.activeConversationID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if activeConversationID.isEmpty {
            if !compactNavigationPath.isEmpty {
                compactNavigationPath = []
            }
            return
        }
        if compactNavigationPath.last != activeConversationID || compactNavigationPath.count != 1 {
            compactNavigationPath = [activeConversationID]
        }
    }

    private var conversationsWorkspaceTitle: String? {
        runtime.state.workspaceMetadata?.workspaceRoot?.workspaceDisplayTitle
            ?? runtime.state.workspaceMetadata?.defaultAgent?.trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

private extension ToolbarItemPlacement {
    static var actionShellToolbarPlacement: ToolbarItemPlacement {
        #if os(iOS)
        .topBarTrailing
        #else
        .automatic
        #endif
    }
}

private extension SearchFieldPlacement {
    static var conversationSearchPlacement: SearchFieldPlacement {
        #if os(iOS)
        .navigationBarDrawer(displayMode: .automatic)
        #else
        .automatic
        #endif
    }
}

private struct AppBrandView: View {
    let workspaceTitle: String?
    let brandLabel: String

    var body: some View {
        let displayTitle = resolveWorkspaceBrandTitle(workspaceTitle: workspaceTitle)
        HStack(spacing: 8) {
            Text(brandLabel)
                .font(.caption.weight(.black))
                .tracking(0.4)
                .foregroundStyle(Color(red: 0.86, green: 0.12, blue: 0.17))
            Text(displayTitle)
                .font(.headline.weight(.semibold))
                .foregroundStyle(.primary)
        }
        .fixedSize(horizontal: true, vertical: false)
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(brandLabel) \(displayTitle)")
    }
}

private struct ConversationListView: View {
    let conversations: [Conversation]
    let activeConversationID: String?
    let selection: Binding<String?>?
    let usesNavigationDestination: Bool
    @Binding var searchText: String
    let isRefreshing: Bool
    let workspaceTitle: String?
    let metadata: WorkspaceMetadata?
    let selectedAgentID: String?
    let availableAgents: [WorkspaceAgentOption]
    let composerRuntime: ComposerRuntime
    let isSending: Bool
    let showsStarterSurfaceOverride: Bool
    let onRefresh: () async -> Void
    let onSelectConversation: (String) -> Void
    let onSelectAgent: (String?) -> Void
    let onSelectStarterTask: (StarterTask) -> Void
    let onRequestDeleteConversation: (Conversation) -> Void
    let onSend: () -> Void

    private var trimmedSearchText: String {
        searchText.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var filteredConversations: [Conversation] {
        let base = sortedRecentConversations(conversations)
        guard !trimmedSearchText.isEmpty else { return base }
        let query = trimmedSearchText.lowercased()
        return base.filter { conversation in
            let haystacks = [
                conversation.title,
                conversation.summary,
                conversation.agentID,
                conversation.lastActivity,
                conversation.stage
            ]
            return haystacks
                .compactMap { $0?.lowercased() }
                .contains { $0.contains(query) }
        }
    }

    private var showsCompactStarterTasks: Bool {
        guard usesNavigationDestination, trimmedSearchText.isEmpty else {
            return false
        }
        if showsStarterSurfaceOverride {
            return true
        }
        return activeConversationID?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true
    }

    var body: some View {
        VStack(spacing: 0) {
            AppBrandView(
                workspaceTitle: workspaceTitle,
                brandLabel: resolveWorkspaceBrandLabel(metadata: metadata)
            )
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 20)
                .padding(.top, 10)
                .padding(.bottom, 6)

            if showsCompactStarterTasks {
                compactStarterSurface
            } else if conversations.isEmpty, isRefreshing {
                ContentUnavailableView {
                    Label("Loading Conversations", systemImage: "arrow.triangle.2.circlepath")
                } description: {
                    Text("Fetching the latest workspace conversations.")
                } actions: {
                    ProgressView()
                }
            } else if conversations.isEmpty {
                ContentUnavailableView(
                    "No Conversations Yet",
                    systemImage: "bubble.left.and.bubble.right",
                    description: Text("Connect to a workspace and send a query to create your first conversation.")
                )
            } else if filteredConversations.isEmpty {
                ContentUnavailableView {
                    Label("No Matching Conversations", systemImage: "magnifyingglass")
                } description: {
                    Text("Try a different search term for the current conversation list.")
                } actions: {
                    Button("Clear Search") {
                        searchText = ""
                    }
                    .buttonStyle(.bordered)
                }
            } else if let selection {
                List(filteredConversations, selection: selection) { conversation in
                    ConversationRowView(
                        conversation: conversation,
                        isActive: conversation.id == activeConversationID
                    )
                    .tag(Optional(conversation.id))
                    .contentShape(Rectangle())
                    .onTapGesture {
                        onSelectConversation(conversation.id)
                    }
                    .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                        Button(role: .destructive) {
                            onRequestDeleteConversation(conversation)
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                }
                .id("selected-list-\(activeConversationID ?? "none")")
            } else if usesNavigationDestination {
                List(filteredConversations, id: \.id) { conversation in
                    NavigationLink(value: conversation.id) {
                        ConversationRowView(
                            conversation: conversation,
                            isActive: conversation.id == activeConversationID
                        )
                    }
                    .simultaneousGesture(TapGesture().onEnded {
                        onSelectConversation(conversation.id)
                    })
                    .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                        Button(role: .destructive) {
                            onRequestDeleteConversation(conversation)
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                }
                .id("nav-list-\(activeConversationID ?? "none")")
            } else {
                List(filteredConversations, id: \.id) { conversation in
                    Button {
                        onSelectConversation(conversation.id)
                    } label: {
                        ConversationRowView(
                            conversation: conversation,
                            isActive: conversation.id == activeConversationID
                        )
                    }
                    .buttonStyle(.plain)
                    .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                        Button(role: .destructive) {
                            onRequestDeleteConversation(conversation)
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                }
                .id("plain-list-\(activeConversationID ?? "none")")
            }
        }
        .navigationTitle("Conversations")
        .searchable(
            text: $searchText,
            placement: SearchFieldPlacement.conversationSearchPlacement,
            prompt: "Search conversations"
        )
        .refreshable {
            await onRefresh()
        }
    }

    private var compactStarterSurface: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                ComposerScreen(
                    runtime: composerRuntime,
                    isSending: isSending,
                    onSend: onSend
                )
                .background(Color.secondary.opacity(0.05), in: RoundedRectangle(cornerRadius: 18))
                .overlay(
                    RoundedRectangle(cornerRadius: 18)
                        .stroke(Color.secondary.opacity(0.12), lineWidth: 1)
                )

                ChatWorkspaceView(
                    metadata: metadata,
                    selectedAgentID: selectedAgentID,
                    availableAgents: availableAgents,
                    onSelectAgent: onSelectAgent,
                    showStarterTasks: true,
                    onSelectStarterTask: onSelectStarterTask
                )
            }
            .padding(.horizontal, 16)
            .padding(.bottom, 18)
        }
    }

}

private struct ConversationRowView: View {
    let conversation: Conversation
    let isActive: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(conversation.title ?? "Untitled Conversation")
                    .font(.body.weight(isActive ? .semibold : .regular))
                    .lineLimit(1)
                Spacer(minLength: 8)
                if let relativeLastActivity, !relativeLastActivity.isEmpty {
                    Text(relativeLastActivity)
                        .font(.caption2.weight(.medium))
                        .foregroundStyle(.tertiary)
                        .lineLimit(1)
                }
            }

            if let summary = conversation.summary?.trimmingCharacters(in: .whitespacesAndNewlines),
               !summary.isEmpty {
                Text(summary)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }

            if !metadataChips.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 6) {
                        ForEach(metadataChips) { chip in
                            ConversationMetadataChip(chip: chip)
                        }
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 4)
    }

    private var metadataChips: [ConversationMetadataChipModel] {
        var chips: [ConversationMetadataChipModel] = []

        if let stage = conversation.stage?.trimmingCharacters(in: .whitespacesAndNewlines),
           !stage.isEmpty {
            chips.append(
                ConversationMetadataChipModel(
                    id: "stage-\(stage.lowercased())",
                    title: stage.capitalized,
                    tint: stageTint(for: stage)
                )
            )
        }

        if let agentID = conversation.agentID?.trimmingCharacters(in: .whitespacesAndNewlines),
           !agentID.isEmpty {
            chips.append(
                ConversationMetadataChipModel(
                    id: "agent-\(agentID)",
                    title: agentID,
                    tint: .secondary
                )
            )
        }

        if let absoluteLastActivity, !absoluteLastActivity.isEmpty {
            chips.append(
                ConversationMetadataChipModel(
                    id: "last-activity",
                    title: absoluteLastActivity,
                    tint: .gray
                )
            )
        }

        return chips
    }

    private var relativeLastActivity: String? {
        guard let date = parsedLastActivityDate else { return nil }
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .short
        return formatter.localizedString(for: date, relativeTo: Date())
    }

    private var absoluteLastActivity: String? {
        guard let date = parsedLastActivityDate else {
            return conversation.lastActivity?.trimmingCharacters(in: .whitespacesAndNewlines)
        }

        return DateFormatter.localizedString(from: date, dateStyle: .medium, timeStyle: .short)
    }

    private var parsedLastActivityDate: Date? {
        parseConversationActivityDate(conversation.lastActivity ?? conversation.createdAt)
    }

    private func stageTint(for rawStage: String) -> Color {
        switch rawStage.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
        case "running", "active":
            return .blue
        case "waiting", "pending":
            return .orange
        case "failed", "error":
            return .red
        case "completed", "done":
            return .green
        default:
            return .secondary
        }
    }
}

private struct ConversationMetadataChipModel: Identifiable {
    let id: String
    let title: String
    let tint: Color
}

private struct ConversationMetadataChip: View {
    let chip: ConversationMetadataChipModel

    var body: some View {
        Text(chip.title)
            .font(.caption2.weight(.medium))
            .foregroundStyle(chip.tint)
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(chip.tint.opacity(0.12), in: Capsule())
    }
}

private struct EmptyConversationDetailView: View {
    let workspaceTitle: String?
    let metadata: WorkspaceMetadata?
    let selectedAgentID: String?
    let availableAgents: [WorkspaceAgentOption]
    let composerRuntime: ComposerRuntime
    let isSending: Bool
    let onSelectAgent: (String?) -> Void
    let onSelectStarterTask: (StarterTask) -> Void
    let onSend: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            ChatWorkspaceView(
                metadata: metadata,
                selectedAgentID: selectedAgentID,
                availableAgents: availableAgents,
                onSelectAgent: onSelectAgent,
                showStarterTasks: true,
                onSelectStarterTask: onSelectStarterTask
            )
            ContentUnavailableView(
                workspaceTitle ?? "Workspace Ready",
                systemImage: "text.bubble",
                description: Text("Choose a conversation from the sidebar or create one by sending a query once the backend is connected.")
            )
            ComposerScreen(
                runtime: composerRuntime,
                isSending: isSending,
                onSend: onSend
            )
            .background(Color.secondary.opacity(0.05), in: RoundedRectangle(cornerRadius: 18))
            .overlay(
                RoundedRectangle(cornerRadius: 18)
                    .stroke(Color.secondary.opacity(0.12), lineWidth: 1)
            )
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }
}

private extension String {
    var workspaceDisplayTitle: String {
        let trimmed = trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return "Workspace" }
        let normalized = trimmed.hasSuffix("/") ? String(trimmed.dropLast()) : trimmed
        let url = URL(fileURLWithPath: normalized)
        let candidate = url.lastPathComponent
        if candidate.isEmpty {
            return trimmed
        }
        if candidate.hasPrefix(".") {
            let parent = url.deletingLastPathComponent().lastPathComponent
            if !parent.isEmpty {
                return parent
            }
        }
        return candidate
    }
}

internal func resolveWorkspaceBrandTitle(
    workspaceTitle: String?,
    fallbackTitle: String = "Agently"
) -> String {
    let trimmed = workspaceTitle?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    guard !trimmed.isEmpty else {
        return fallbackTitle
    }
    let normalized = trimmed
        .replacingOccurrences(of: "_", with: " ")
        .replacingOccurrences(of: "-", with: " ")
        .split(separator: " ")
        .map { token in
            let lower = token.lowercased()
            return lower.prefix(1).uppercased() + lower.dropFirst()
        }
        .joined(separator: " ")
        .trimmingCharacters(in: .whitespacesAndNewlines)
    guard !normalized.isEmpty else {
        return fallbackTitle
    }
    let stripped = normalized.replacingOccurrences(
        of: #"^viant\s+"#,
        with: "",
        options: [.regularExpression, .caseInsensitive]
    ).trimmingCharacters(in: .whitespacesAndNewlines)
    return stripped.isEmpty ? fallbackTitle : stripped
}

internal func resolveWorkspaceBrandLabel(
    metadata: WorkspaceMetadata?,
    fallbackLabel: String = "Agently"
) -> String {
    let explicit = metadata?.appName?.trimmingCharacters(in: .whitespacesAndNewlines)
    if let explicit, !explicit.isEmpty {
        return explicit
    }
    let defaultLabel = metadata?.defaults?.appName?.trimmingCharacters(in: .whitespacesAndNewlines)
    if let defaultLabel, !defaultLabel.isEmpty {
        return defaultLabel
    }
    return fallbackLabel
}

private extension View {
    func settingsSheet(isPresented: Binding<Bool>, runtime: AppRuntime) -> some View {
        sheet(isPresented: isPresented) {
            NavigationStack {
                SettingsScreen(
                    runtime: runtime.settingsRuntime,
                    workspaceRoot: runtime.state.workspaceMetadata?.workspaceRoot,
                    workspaceDefaultAgentID: runtime.state.workspaceMetadata?.defaultAgent,
                    availableAgents: runtime.availableAgentOptions,
                    agentAutoSelectionEnabled: runtime.state.workspaceMetadata?.capabilities?.agentAutoSelection == true,
                    oauthProviderLabels: runtime.authRuntime.authProviders.map { ($0.name ?? $0.type).trimmingCharacters(in: .whitespacesAndNewlines) }.filter { !$0.isEmpty },
                    oauthScopes: runtime.authRuntime.oauthScopes
                ) {
                    Task {
                        isPresented.wrappedValue = false
                        await runtime.applySettingsAndReload()
                    }
                }
            }
        }
    }
}
