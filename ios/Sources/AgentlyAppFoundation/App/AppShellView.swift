import SwiftUI
import AgentlySDK

public struct AppShellView: View {
    @ObservedObject private var runtime: AppRuntime
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @State private var isShowingSettings = false
    @State private var conversationSearchText = ""

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
            ToolbarItemGroup(placement: ToolbarItemPlacement.primaryShellToolbarPlacement) {
                Button {
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
            ToolbarItem(placement: .primaryAction) {
                Button {
                    isShowingSettings = true
                } label: {
                    Label("Settings", systemImage: "gearshape")
                }
            }
        }
        .settingsSheet(isPresented: $isShowingSettings, runtime: runtime)
    }

    private var regularShell: some View {
        NavigationSplitView {
            ConversationListView(
                conversations: runtime.state.conversations,
                activeConversationID: runtime.state.activeConversationID,
                searchText: $conversationSearchText,
                isRefreshing: runtime.state.isRefreshingConversations,
                onRefresh: {
                    await runtime.refreshConversationList()
                },
                onSelectConversation: { conversationID in
                    Task { await runtime.selectConversation(conversationID) }
                }
            )
        } detail: {
            if runtime.state.activeConversationID == nil {
                EmptyConversationDetailView(workspaceTitle: runtime.state.workspaceMetadata?.workspaceRoot?.workspaceDisplayTitle)
            } else {
                ChatScreens(runtime: runtime)
                    .navigationTitle(runtime.state.workspaceMetadata?.workspaceRoot?.workspaceDisplayTitle ?? "Workspace")
            }
        }
        .navigationSplitViewStyle(.balanced)
    }

    private var compactShell: some View {
        NavigationStack {
            ConversationListView(
                conversations: runtime.state.conversations,
                activeConversationID: runtime.state.activeConversationID,
                searchText: $conversationSearchText,
                isRefreshing: runtime.state.isRefreshingConversations,
                onRefresh: {
                    await runtime.refreshConversationList()
                },
                onSelectConversation: { conversationID in
                    Task { await runtime.selectConversation(conversationID) }
                }
            )
            .navigationDestination(for: String.self) { conversationID in
                ChatScreens(runtime: runtime)
                    .navigationTitle(runtime.state.workspaceMetadata?.workspaceRoot?.workspaceDisplayTitle ?? "Workspace")
                    .task(id: conversationID) {
                        if runtime.state.activeConversationID != conversationID {
                            await runtime.selectConversation(conversationID)
                        }
                    }
            }
        }
    }
}

private extension ToolbarItemPlacement {
    static var primaryShellToolbarPlacement: ToolbarItemPlacement {
        #if os(iOS)
        .topBarLeading
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

private struct ConversationListView: View {
    let conversations: [Conversation]
    let activeConversationID: String?
    @Binding var searchText: String
    let isRefreshing: Bool
    let onRefresh: () async -> Void
    let onSelectConversation: (String) -> Void

    private var trimmedSearchText: String {
        searchText.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var filteredConversations: [Conversation] {
        let base = conversations.sorted(by: Self.isConversation(_:newerThan:))
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

    var body: some View {
        Group {
            if conversations.isEmpty, isRefreshing {
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
            } else {
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
                }
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

    private static func isConversation(_ lhs: Conversation, newerThan rhs: Conversation) -> Bool {
        let lhsDate = parsedActivityDate(from: lhs.lastActivity)
        let rhsDate = parsedActivityDate(from: rhs.lastActivity)

        switch (lhsDate, rhsDate) {
        case let (lhsDate?, rhsDate?):
            if lhsDate != rhsDate {
                return lhsDate > rhsDate
            }
        case (_?, nil):
            return true
        case (nil, _?):
            return false
        case (nil, nil):
            break
        }

        let lhsTitle = lhs.title?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let rhsTitle = rhs.title?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if lhsTitle != rhsTitle {
            return lhsTitle.localizedCaseInsensitiveCompare(rhsTitle) == .orderedAscending
        }

        return lhs.id < rhs.id
    }

    private static func parsedActivityDate(from rawValue: String?) -> Date? {
        guard let rawValue = rawValue?.trimmingCharacters(in: .whitespacesAndNewlines),
              !rawValue.isEmpty else {
            return nil
        }

        let fractionalFormatter = ISO8601DateFormatter()
        fractionalFormatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = fractionalFormatter.date(from: rawValue) {
            return date
        }

        let fallbackFormatter = ISO8601DateFormatter()
        fallbackFormatter.formatOptions = [.withInternetDateTime]
        return fallbackFormatter.date(from: rawValue)
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
        guard let rawValue = conversation.lastActivity?.trimmingCharacters(in: .whitespacesAndNewlines),
              !rawValue.isEmpty else {
            return nil
        }

        let fractionalFormatter = ISO8601DateFormatter()
        fractionalFormatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = fractionalFormatter.date(from: rawValue) {
            return date
        }

        let fallbackFormatter = ISO8601DateFormatter()
        fallbackFormatter.formatOptions = [.withInternetDateTime]
        return fallbackFormatter.date(from: rawValue)
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

    var body: some View {
        ContentUnavailableView(
            workspaceTitle ?? "Workspace Ready",
            systemImage: "text.bubble",
            description: Text("Choose a conversation from the sidebar or create one by sending a query once the backend is connected.")
        )
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

private extension View {
    func settingsSheet(isPresented: Binding<Bool>, runtime: AppRuntime) -> some View {
        sheet(isPresented: isPresented) {
            NavigationStack {
                SettingsScreen(runtime: runtime.settingsRuntime) {
                    Task {
                        isPresented.wrappedValue = false
                        await runtime.applySettingsAndReload()
                    }
                }
            }
        }
    }
}
