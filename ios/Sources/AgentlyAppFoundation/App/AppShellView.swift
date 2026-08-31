import SwiftUI
import AgentlySDK

public struct AppShellView: View {
    @ObservedObject private var runtime: AppRuntime
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @State private var isShowingSettings = false
    @State private var isShowingAutomation = false
    @State private var conversationSearchText = ""
    @State private var compactNavigationPath: [String] = []
    @State private var compactUserReturnedToListConversationID: String?
    @State private var compactShowsStarterSurface = true
    @State private var compactHistoryRequested = false
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
                if isHomeSurfaceVisible {
                    Button {
                        compactUserReturnedToListConversationID = nil
                        compactNavigationPath = []
                        conversationSearchText = ""
                        compactShowsStarterSurface = true
                        compactHistoryRequested = false
                        runtime.startNewConversation()
                    } label: {
                        AppleToolbarActionIcon(
                            systemImage: "square.and.pencil",
                            color: Color(red: 0.10, green: 0.45, blue: 0.95)
                        )
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("New Chat")
                    .accessibilityIdentifier("agently-new-chat")

                    Button {
                        compactUserReturnedToListConversationID = normalizedCompactConversationID(runtime.state.activeConversationID)
                        compactNavigationPath = []
                        conversationSearchText = ""
                        compactShowsStarterSurface = false
                        compactHistoryRequested = true
                        if horizontalSizeClass == .regular {
                            regularColumnVisibility = .all
                        }
                        Task { await runtime.refreshConversationList() }
                    } label: {
                        AppleToolbarActionIcon(
                            systemImage: "clock.arrow.circlepath",
                            color: Color(red: 0.88, green: 0.54, blue: 0.12)
                        )
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("History")
                    .accessibilityIdentifier("agently-home-history")
                } else {
                    Button {
                    compactUserReturnedToListConversationID = nil
                    compactNavigationPath = []
                    conversationSearchText = ""
                    compactShowsStarterSurface = true
                    compactHistoryRequested = false
                    if horizontalSizeClass == .regular {
                        regularColumnVisibility = .detailOnly
                    }
                    runtime.startNewConversation()
                } label: {
                    AppleToolbarActionIcon(
                        systemImage: "house.fill",
                        color: Color(red: 0.10, green: 0.45, blue: 0.95)
                    )
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("Home")
                    .accessibilityIdentifier("agently-home")
                }

                if shouldShowToolbarHistory {
                    Button {
                    compactUserReturnedToListConversationID = normalizedCompactConversationID(runtime.state.activeConversationID)
                    compactNavigationPath = []
                    conversationSearchText = ""
                    compactShowsStarterSurface = false
                    compactHistoryRequested = true
                    if horizontalSizeClass == .regular {
                        regularColumnVisibility = .all
                    }
                    Task { await runtime.refreshConversationList() }
                } label: {
                    AppleToolbarActionIcon(
                        systemImage: "clock.arrow.circlepath",
                        color: Color(red: 0.88, green: 0.54, blue: 0.12)
                    )
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("History")
                    .accessibilityIdentifier("agently-history")
                }

                if isHistorySurfaceVisible {
                    Button {
                        Task { await runtime.refreshConversationList() }
                    } label: {
                        AppleToolbarActionIcon(
                            systemImage: "arrow.clockwise",
                            color: Color(red: 0.04, green: 0.61, blue: 0.60),
                            isLoading: runtime.state.isRefreshingConversations
                        )
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("Refresh conversations")
                    .accessibilityIdentifier("agently-history-refresh")
                    .disabled(runtime.state.isRefreshingConversations)
                }

                Button {
                    isShowingAutomation = true
                } label: {
                    AppleToolbarActionIcon(
                        systemImage: "clock",
                        color: Color(red: 0.04, green: 0.53, blue: 0.49)
                    )
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Automation")
                .accessibilityIdentifier("agently-automation")

                Button {
                    isShowingSettings = true
                } label: {
                    AppleToolbarActionIcon(
                        systemImage: "gearshape.fill",
                        color: Color(red: 0.49, green: 0.32, blue: 0.88)
                    )
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Settings")
            }
        }
        .settingsSheet(
            isPresented: $isShowingSettings,
            runtime: runtime,
            restoreConversationID: settingsRestoreConversationID,
            selectInitialConversation: settingsShouldRestoreConversation
        )
        .automationPresentation(isPresented: $isShowingAutomation) {
            AutomationWorkspaceScreen(
                forgeRuntime: runtime.state.forgeRuntime,
                client: runtime.state.client,
                onOpenConversation: { conversationID in
                    isShowingAutomation = false
                    compactShowsStarterSurface = false
                    compactHistoryRequested = false
                    Task { await runtime.selectConversation(conversationID) }
                }
            )
        }
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
                showsHistorySurfaceOverride: false,
                onRefresh: {
                    await runtime.refreshConversationList()
                },
                onSelectConversation: { conversationID in
                    compactShowsStarterSurface = false
                    compactHistoryRequested = false
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
                        compactHistoryRequested = false
                        runtime.composerRuntime.query = prompt
                    }
                },
                onRequestDeleteConversation: { conversation in
                    pendingConversationDeletion = conversation
                },
                onNewChat: {
                    compactShowsStarterSurface = true
                    compactHistoryRequested = false
                    runtime.startNewConversation()
                },
                onShowHistory: {
                    compactShowsStarterSurface = false
                    compactHistoryRequested = true
                    regularColumnVisibility = .all
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
                showsHistorySurfaceOverride: compactHistoryRequested,
                onRefresh: {
                    await runtime.refreshConversationList()
                },
                onSelectConversation: { conversationID in
                    let normalized = conversationID.trimmingCharacters(in: .whitespacesAndNewlines)
                    if !normalized.isEmpty {
                        compactUserReturnedToListConversationID = nil
                        compactNavigationPath = [normalized]
                    }
                    compactShowsStarterSurface = false
                    compactHistoryRequested = false
                    Task { await runtime.selectConversation(conversationID) }
                },
                onSelectAgent: { agentID in
                    runtime.selectPreferredAgent(agentID)
                },
                onSelectStarterTask: { task in
                    let prompt = (task.prompt ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                    if !prompt.isEmpty {
                        compactShowsStarterSurface = true
                        compactHistoryRequested = false
                        runtime.composerRuntime.query = prompt
                    }
                },
                onRequestDeleteConversation: { conversation in
                    pendingConversationDeletion = conversation
                },
                onNewChat: {
                    compactUserReturnedToListConversationID = nil
                    compactNavigationPath = []
                    compactShowsStarterSurface = true
                    compactHistoryRequested = false
                    runtime.startNewConversation()
                },
                onShowHistory: {
                    compactUserReturnedToListConversationID = normalizedCompactConversationID(runtime.state.activeConversationID)
                    compactNavigationPath = []
                    compactShowsStarterSurface = false
                    compactHistoryRequested = true
                    Task { await runtime.refreshConversationList() }
                },
                onSend: { Task { await runtime.sendCurrentQuery() } }
            )
            .navigationDestination(for: String.self) { conversationID in
                CompactConversationDestination(
                    conversationID: conversationID,
                    runtime: runtime,
                    onReturnToList: {
                        compactUserReturnedToListConversationID = normalizedCompactConversationID(runtime.state.activeConversationID)
                            ?? normalizedCompactConversationID(conversationID)
                        compactShowsStarterSurface = false
                        compactHistoryRequested = true
                        compactNavigationPath = []
                    }
                )
            }
            .task(id: runtime.state.activeConversationID) {
                syncCompactNavigationPath()
            }
            .onChange(of: compactNavigationPath) { _, newValue in
                if newValue.isEmpty, runtime.state.activeConversationID != nil {
                    compactUserReturnedToListConversationID = normalizedCompactConversationID(runtime.state.activeConversationID)
                    compactShowsStarterSurface = false
                    compactHistoryRequested = true
                } else if !newValue.isEmpty {
                    compactUserReturnedToListConversationID = nil
                    compactHistoryRequested = false
                }
            }
        }
    }

    private func syncCompactNavigationPath() {
        let nextPath = resolvedCompactNavigationPath(
            activeConversationID: runtime.state.activeConversationID,
            navigationPath: compactNavigationPath,
            userReturnedToListConversationID: compactUserReturnedToListConversationID
        )
        if nextPath != compactNavigationPath {
            compactNavigationPath = nextPath
        }
        if nextPath.isEmpty, normalizedCompactConversationID(runtime.state.activeConversationID) == nil {
            compactUserReturnedToListConversationID = nil
        }
    }

    private var conversationsWorkspaceTitle: String? {
        runtime.state.workspaceMetadata?.workspaceRoot?.workspaceDisplayTitle
            ?? runtime.state.workspaceMetadata?.defaultAgent?.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var shouldShowToolbarHistory: Bool {
        guard !isHomeSurfaceVisible else { return false }
        if horizontalSizeClass == .compact {
            return !compactNavigationPath.isEmpty
        }
        return regularColumnVisibility == .detailOnly
    }

    private var isHomeSurfaceVisible: Bool {
        if horizontalSizeClass == .compact {
            return compactNavigationPath.isEmpty
                && !compactHistoryRequested
                && compactShowsStarterSurface
        }
        return compactShowsStarterSurface && runtime.state.activeConversationID == nil
    }

    private var isHistorySurfaceVisible: Bool {
        if horizontalSizeClass == .compact {
            return compactNavigationPath.isEmpty && compactHistoryRequested
        }
        guard !compactShowsStarterSurface else { return false }
        return regularColumnVisibility == .all
    }

    private var settingsShouldRestoreConversation: Bool {
        if horizontalSizeClass == .compact {
            return !compactNavigationPath.isEmpty
        }
        return regularColumnVisibility == .detailOnly
    }

    private var settingsRestoreConversationID: String? {
        settingsShouldRestoreConversation
            ? normalizedCompactConversationID(runtime.state.activeConversationID)
            : nil
    }
}

private extension View {
    @ViewBuilder
    func automationPresentation<Content: View>(
        isPresented: Binding<Bool>,
        @ViewBuilder content: @escaping () -> Content
    ) -> some View {
        #if os(iOS)
        self.fullScreenCover(isPresented: isPresented, content: content)
        #else
        self.sheet(isPresented: isPresented, content: content)
        #endif
    }
}

struct AppleToolbarActionIcon: View {
    let systemImage: String
    let color: Color
    var isLoading = false

    var body: some View {
        ZStack {
            Circle()
                .fill(
                    LinearGradient(
                        colors: [color.opacity(0.20), color.opacity(0.08)],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                )
            Circle()
                .stroke(
                    LinearGradient(
                        colors: [Color.white.opacity(0.92), color.opacity(0.32)],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    ),
                    lineWidth: 1
                )
            Circle()
                .fill(Color.white.opacity(0.42))
                .frame(width: 18, height: 8)
                .blur(radius: 3)
                .offset(x: -5, y: -8)
            if isLoading {
                ProgressView()
                    .controlSize(.small)
                    .tint(color)
            } else {
                Image(systemName: systemImage)
                    .font(.system(size: 15, weight: .semibold))
                    .symbolRenderingMode(.hierarchical)
                    .foregroundStyle(color)
            }
        }
        .frame(width: 34, height: 34)
        .shadow(color: color.opacity(0.20), radius: 5, x: 0, y: 3)
        .contentShape(Circle())
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
    let metadata: WorkspaceMetadata?

    var body: some View {
        let displayTitle = resolveWorkspaceBrandTitle(workspaceTitle: workspaceTitle)
        let headerTitle = resolveWorkspaceHeaderTitle(metadata: metadata, workspaceTitle: displayTitle)
        Text(headerTitle)
            .font(.headline.weight(.semibold))
            .foregroundStyle(.primary)
        .fixedSize(horizontal: true, vertical: false)
        .accessibilityLabel(headerTitle)
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
    let showsHistorySurfaceOverride: Bool
    let onRefresh: () async -> Void
    let onSelectConversation: (String) -> Void
    let onSelectAgent: (String?) -> Void
    let onSelectStarterTask: (StarterTask) -> Void
    let onRequestDeleteConversation: (Conversation) -> Void
    let onNewChat: () -> Void
    let onShowHistory: () -> Void
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
            return !showsHistorySurfaceOverride
        }
        return false
    }

    var body: some View {
        VStack(spacing: 0) {
            AppBrandView(
                workspaceTitle: workspaceTitle,
                metadata: metadata
            )
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 20)
                .padding(.top, 10)
                .padding(.bottom, 6)

            if !showsCompactStarterTasks {
                HStack(spacing: 10) {
                    Image(systemName: "magnifyingglass")
                        .foregroundStyle(.secondary)
                    TextField("Search conversations", text: $searchText)
                    if !searchText.isEmpty {
                        Button {
                            searchText = ""
                        } label: {
                            Image(systemName: "xmark.circle.fill")
                                .foregroundStyle(.secondary)
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel("Clear conversation search")
                    }
                }
                .padding(.horizontal, 14)
                .frame(minHeight: 44)
                .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 14))
                .padding(.horizontal, 16)
                .padding(.bottom, 8)
                .accessibilityIdentifier("agently-history-filter")
            }

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
                        isActive: conversation.id == activeConversationID,
                        onDelete: { onRequestDeleteConversation(conversation) }
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
                    ConversationRowView(
                        conversation: conversation,
                        isActive: conversation.id == activeConversationID,
                        onDelete: { onRequestDeleteConversation(conversation) }
                    )
                    .contentShape(Rectangle())
                    .onTapGesture { onSelectConversation(conversation.id) }
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
                    ConversationRowView(
                        conversation: conversation,
                        isActive: conversation.id == activeConversationID,
                        onDelete: { onRequestDeleteConversation(conversation) }
                    )
                    .contentShape(Rectangle())
                    .onTapGesture { onSelectConversation(conversation.id) }
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
        .navigationTitle(showsCompactStarterTasks ? "Home" : "History")
        .modifier(ConversationListNavigationTitleMode(useInlineTitle: usesNavigationDestination))
        .refreshable {
            await onRefresh()
        }
        .task(id: showsCompactStarterTasks) {
            if !showsCompactStarterTasks {
                await onRefresh()
            }
        }
    }

    private var compactStarterSurface: some View {
        VStack(spacing: 12) {
            ScrollView {
                ChatWorkspaceView(
                    metadata: metadata,
                    selectedAgentID: selectedAgentID,
                    availableAgents: availableAgents,
                    onSelectAgent: onSelectAgent,
                    showStarterTasks: true,
                    showWorkspaceHeader: false,
                    starterTaskLayout: .verticalList,
                    onSelectStarterTask: onSelectStarterTask
                )
                .padding(.horizontal, 16)
                .padding(.top, 6)
                .padding(.bottom, 8)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)

            ComposerScreen(
                runtime: composerRuntime,
                isSending: isSending,
                density: .compact,
                onSend: onSend
            )
            .padding(.horizontal, 16)
            .padding(.bottom, 10)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottom)
    }

}

private struct ConversationListNavigationTitleMode: ViewModifier {
    let useInlineTitle: Bool

    func body(content: Content) -> some View {
        #if os(iOS)
        if useInlineTitle {
            content.navigationBarTitleDisplayMode(.inline)
        } else {
            content
        }
        #else
        content
        #endif
    }
}

private struct CompactConversationDestination: View {
    let conversationID: String
    @ObservedObject var runtime: AppRuntime
    let onReturnToList: () -> Void

    private var isActiveConversationLoaded: Bool {
        runtime.state.activeConversationID?.trimmingCharacters(in: .whitespacesAndNewlines) == conversationID
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Button(action: onReturnToList) {
                    HStack(spacing: 8) {
                        AppleToolbarActionIcon(
                            systemImage: "chevron.left",
                            color: Color(red: 0.35, green: 0.40, blue: 0.85)
                        )
                        Text("Conversations")
                            .font(.subheadline.weight(.semibold))
                    }
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("agently-conversations-back")
                Spacer()
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 6)

            Group {
                if isActiveConversationLoaded {
                    ChatScreens(
                        runtime: runtime,
                        onReturnToConversationList: onReturnToList
                    )
                        .id(conversationID)
                } else {
                    ConversationLoadingDetailView()
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .navigationTitle("Conversation")
        .modifier(ConversationListNavigationTitleMode(useInlineTitle: true))
        .navigationBarBackButtonHidden(true)
        .task(id: conversationID) {
            if !isActiveConversationLoaded {
                await runtime.selectConversation(conversationID)
            }
        }
    }
}

private struct ConversationLoadingDetailView: View {
    var body: some View {
        VStack(spacing: 14) {
            ProgressView()
                .controlSize(.large)
            Text("Opening conversation")
                .font(.headline)
            Text("Loading the selected workspace thread.")
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding()
    }
}

private struct ConversationRowView: View {
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @State private var isPointerHovering = false
    let conversation: Conversation
    let isActive: Bool
    let onDelete: () -> Void

    var body: some View {
        Group {
        if horizontalSizeClass == .compact {
            HStack(spacing: 12) {
                VStack(alignment: .leading, spacing: 5) {
                    Text(conversation.title ?? "Untitled Conversation")
                        .font(.body.weight(isActive ? .semibold : .regular))
                        .lineLimit(1)
                    if let relativeLastActivity, !relativeLastActivity.isEmpty {
                        Text(relativeLastActivity)
                            .font(.caption.weight(.medium))
                            .foregroundStyle(.secondary)
                    }
                }
                Spacer(minLength: 8)
                trailingAction
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .background(
                RoundedRectangle(cornerRadius: 16)
                    .fill(isActive ? Color.blue.opacity(0.07) : Color.secondary.opacity(0.04))
            )
            .overlay(
                RoundedRectangle(cornerRadius: 16)
                    .stroke(isActive ? Color.blue.opacity(0.30) : Color.secondary.opacity(0.10), lineWidth: 1)
            )
        } else {
            HStack(alignment: .center, spacing: 10) {
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
        if isPointerHovering {
            deleteButton
        }
            }
            }
        }
        .onHover { hovering in
            withAnimation(.easeInOut(duration: 0.12)) {
                isPointerHovering = hovering
            }
        }
    }

    @ViewBuilder
    private var trailingAction: some View {
        if isPointerHovering {
            deleteButton
        } else {
            AppleToolbarActionIcon(
                systemImage: "eye.fill",
                color: Color(red: 0.10, green: 0.45, blue: 0.95)
            )
            .accessibilityHidden(true)
        }
    }

    private var deleteButton: some View {
        Button(role: .destructive, action: onDelete) {
            AppleToolbarActionIcon(
                systemImage: "trash.fill",
                color: Color(red: 0.70, green: 0.14, blue: 0.12)
            )
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Delete conversation")
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
    let lowercased = normalized.lowercased()
    if lowercased == "viant" || lowercased.hasPrefix("viant ") {
        return normalized
    }
    return "Viant \(normalized)"
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

internal func resolveWorkspaceHeaderTitle(
    metadata: WorkspaceMetadata?,
    workspaceTitle: String,
    fallbackTitle: String = "Agently"
) -> String {
    let configuredAppName = metadata?.appName?.trimmingCharacters(in: .whitespacesAndNewlines)
    if let configuredAppName, !configuredAppName.isEmpty {
        return configuredAppName
    }
    let defaultAppName = metadata?.defaults?.appName?.trimmingCharacters(in: .whitespacesAndNewlines)
    if let defaultAppName, !defaultAppName.isEmpty {
        return defaultAppName
    }
    let workspace = workspaceTitle.trimmingCharacters(in: .whitespacesAndNewlines)
    return workspace.isEmpty ? fallbackTitle : workspace
}

internal func normalizedCompactConversationID(_ conversationID: String?) -> String? {
    let normalized = conversationID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    return normalized.isEmpty ? nil : normalized
}

internal func resolvedCompactNavigationPath(
    activeConversationID: String?,
    navigationPath: [String],
    userReturnedToListConversationID: String?
) -> [String] {
    guard let activeConversationID = normalizedCompactConversationID(activeConversationID) else {
        return []
    }
    if navigationPath.isEmpty,
       normalizedCompactConversationID(userReturnedToListConversationID) == activeConversationID {
        return []
    }
    if navigationPath.count == 1, navigationPath.last == activeConversationID {
        return navigationPath
    }
    return [activeConversationID]
}

private extension View {
    func settingsSheet(
        isPresented: Binding<Bool>,
        runtime: AppRuntime,
        restoreConversationID: String?,
        selectInitialConversation: Bool
    ) -> some View {
        sheet(isPresented: isPresented) {
            NavigationStack {
                SettingsScreen(
                    runtime: runtime.settingsRuntime,
                    workspaceRoot: runtime.state.workspaceMetadata?.workspaceRoot,
                    workspaceDefaultAgentID: runtime.state.workspaceMetadata?.defaultAgent,
                    availableAgents: runtime.availableAgentOptions,
                    agentAutoSelectionEnabled: runtime.state.workspaceMetadata?.capabilities?.agentAutoSelection == true,
                    oauthProviderLabels: runtime.authRuntime.authProviders.map { ($0.name ?? $0.type).trimmingCharacters(in: .whitespacesAndNewlines) }.filter { !$0.isEmpty },
                    oauthScopes: runtime.authRuntime.oauthScopes,
                    authSessionID: runtime.authRuntime.lastAuthSessionID
                ) {
                    Task {
                        isPresented.wrappedValue = false
                        await runtime.applySettingsAndReload(
                            restoreConversationID: restoreConversationID,
                            selectInitialConversation: selectInitialConversation
                        )
                    }
                }
            }
        }
    }
}
