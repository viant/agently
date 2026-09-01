import SwiftUI
import AgentlySDK

enum StarterTaskLayout {
    case horizontalCards
    case verticalList
}

struct ChatWorkspaceView: View {
    @State private var selectedStarterCategoryID = ""
    let metadata: WorkspaceMetadata?
    let selectedAgentID: String?
    let availableAgents: [WorkspaceAgentOption]
    let onSelectAgent: (String?) -> Void
    let showStarterTasks: Bool
    let showWorkspaceHeader: Bool
    let starterTaskLayout: StarterTaskLayout
    let onSelectStarterTask: (StarterTask) -> Void

    init(
        metadata: WorkspaceMetadata?,
        selectedAgentID: String?,
        availableAgents: [WorkspaceAgentOption],
        onSelectAgent: @escaping (String?) -> Void,
        showStarterTasks: Bool = false,
        showWorkspaceHeader: Bool = true,
        starterTaskLayout: StarterTaskLayout = .horizontalCards,
        onSelectStarterTask: @escaping (StarterTask) -> Void = { _ in }
    ) {
        self.metadata = metadata
        self.selectedAgentID = selectedAgentID
        self.availableAgents = availableAgents
        self.onSelectAgent = onSelectAgent
        self.showStarterTasks = showStarterTasks
        self.showWorkspaceHeader = showWorkspaceHeader
        self.starterTaskLayout = starterTaskLayout
        self.onSelectStarterTask = onSelectStarterTask
    }

    private var resolvedAgentID: String? {
        let explicit = selectedAgentID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if !explicit.isEmpty {
            return explicit
        }
        let metadataDefault = metadata?.defaultAgent?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if !metadataDefault.isEmpty {
            return metadataDefault
        }
        let fallbackDefault = metadata?.defaults?.agent?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return fallbackDefault.isEmpty ? nil : fallbackDefault
    }

    private var resolvedAgentLabel: String {
        if let resolvedAgentID,
           let match = availableAgents.first(where: { $0.id == resolvedAgentID }) {
            return match.displayName
        }
        if let resolvedAgentID, !resolvedAgentID.isEmpty {
            return humanizedAgentLabel(resolvedAgentID)
        }
        return "Workspace Default"
    }

    private var showsAgentSelection: Bool {
        availableAgents.count > 1
    }

    private var starterTaskAgentCount: Int {
        (metadata?.agentInfos ?? []).filter { info in
            info.internalAgent != true && info.starterTasks.contains(where: { task in
                let title = (task.title ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                let prompt = (task.prompt ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                return !title.isEmpty && !prompt.isEmpty
            })
        }.count
    }

    private var workspaceLabel: String {
        let preferred = metadata?.workspaceRoot.flatMap(resolveWorkspaceDisplayTitle)
        let workspaceTitle = resolveWorkspaceBrandTitle(
            workspaceTitle: preferred ?? resolvedAgentLabel,
            fallbackTitle: "Agently"
        )
        return resolveWorkspaceHeaderTitle(
            metadata: metadata,
            workspaceTitle: workspaceTitle
        )
    }

    private var starterTasks: [StarterTask] {
        guard let resolvedAgentID else { return [] }
        let match = metadata?.agentInfos.first(where: { info in
            let agentID = (info.agentID ?? info.name ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            return agentID.caseInsensitiveCompare(resolvedAgentID) == .orderedSame
        })
        return (match?.starterTasks ?? []).filter {
            let title = ($0.title ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            let prompt = ($0.prompt ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            return !title.isEmpty && !prompt.isEmpty
        }
    }

    private var starterTaskCategories: [StarterTaskCategory] {
        guard let resolvedAgentID else { return [] }
        let match = metadata?.agentInfos.first(where: { info in
            let agentID = (info.agentID ?? info.name ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            return agentID.caseInsensitiveCompare(resolvedAgentID) == .orderedSame
        })
        let tasks = starterTasks
        return (match?.starterTaskCategories ?? []).filter { category in
            let id = (category.rawID ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            let title = (category.title ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            return !id.isEmpty && !title.isEmpty && tasks.contains { task in
                (task.categoryID ?? "").trimmingCharacters(in: .whitespacesAndNewlines) == id
            }
        }
    }

    private var activeStarterCategory: StarterTaskCategory? {
        starterTaskCategories.first(where: {
            ($0.rawID ?? "").trimmingCharacters(in: .whitespacesAndNewlines) == selectedStarterCategoryID
        })
    }

    private var activeStarterTasks: [StarterTask] {
        guard let id = activeStarterCategory?.rawID?.trimmingCharacters(in: .whitespacesAndNewlines), !id.isEmpty else {
            return starterTasks
        }
        return starterTasks.filter { ($0.categoryID ?? "").trimmingCharacters(in: .whitespacesAndNewlines) == id }
    }

    private var activeStarterCategoryColor: Color? {
        guard let category = activeStarterCategory,
              let index = starterTaskCategories.firstIndex(where: { $0.id == category.id }) else { return nil }
        return starterCategoryColor(index)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if showWorkspaceHeader {
                workspaceHeader
            }

            if showStarterTasks {
                VStack(alignment: .leading, spacing: 10) {
                    Text(starterTaskAgentCount > 1 ? "Start with an agent prompt" : "Starter tasks")
                        .font(.headline)
                    if starterTasks.isEmpty {
                        Text("This agent has no published starter tasks yet. You can still begin with your own prompt below.")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    } else {
                        if starterTaskCategories.isEmpty {
                            starterTaskList(starterTasks)
                        } else {
                            categorizedStarterTaskList
                        }
                    }
                }
            }
        }
        .padding(.horizontal, 20)
        .padding(.top, 12)
        .padding(.bottom, showStarterTasks ? 12 : 4)
    }

    private var workspaceHeader: some View {
        HStack(alignment: .center, spacing: 12) {
            Label(workspaceLabel, systemImage: "rectangle.topthird.inset.filled")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(.primary)
            Spacer(minLength: 8)
            if showsAgentSelection {
                Menu {
                    Button("Workspace Default") {
                        onSelectAgent(nil)
                    }
                    ForEach(availableAgents) { agent in
                        Button(agent.displayName) {
                            onSelectAgent(agent.id)
                        }
                    }
                } label: {
                    Label(resolvedAgentLabel, systemImage: "person.crop.circle")
                        .font(.subheadline.weight(.semibold))
                }
                .menuStyle(.borderlessButton)
            }
        }
    }

    @ViewBuilder
    private var categorizedStarterTaskList: some View {
        if activeStarterCategory == nil {
            LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 10) {
                ForEach(Array(starterTaskCategories.enumerated()), id: \.element.id) { index, category in
                    let id = (category.rawID ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                    let accent = starterCategoryColor(index)
                    Button {
                        selectedStarterCategoryID = id
                    } label: {
                        HStack(spacing: 10) {
                            Image(systemName: starterCategorySymbol(category.icon))
                                .font(.system(size: 19, weight: .semibold))
                                .frame(width: 32, height: 32)
                                .foregroundStyle(Color.white)
                                .background(accent, in: RoundedRectangle(cornerRadius: 10))
                            VStack(alignment: .leading, spacing: 3) {
                                Text((category.title ?? "").trimmingCharacters(in: .whitespacesAndNewlines))
                                    .font(.subheadline.weight(.semibold))
                                    .lineLimit(2)
                                if let description = category.description?.trimmingCharacters(in: .whitespacesAndNewlines), !description.isEmpty {
                                    Text(description).font(.caption2).foregroundStyle(.secondary).lineLimit(2)
                                }
                            }
                            Spacer(minLength: 0)
                        }
                        .frame(maxWidth: .infinity, minHeight: 72, alignment: .leading)
                        .padding(12)
                        .background(
                            LinearGradient(
                                colors: [Color.white, accent.opacity(0.10)],
                                startPoint: .top,
                                endPoint: .bottom
                            ),
                            in: RoundedRectangle(cornerRadius: 14)
                        )
                        .overlay(RoundedRectangle(cornerRadius: 14).stroke(accent.opacity(0.22)))
                    }
                    .buttonStyle(.plain)
                }
            }
        } else if let category = activeStarterCategory {
            Button {
                selectedStarterCategoryID = ""
            } label: {
                Label("Back to categories", systemImage: "chevron.left")
                    .font(.subheadline.weight(.semibold))
            }
            .buttonStyle(.plain)
            .foregroundStyle(Color.accentColor)
            VStack(alignment: .leading, spacing: 3) {
                Text((category.title ?? "").trimmingCharacters(in: .whitespacesAndNewlines))
                    .font(.subheadline.weight(.semibold))
                if let description = category.description?.trimmingCharacters(in: .whitespacesAndNewlines), !description.isEmpty {
                    Text(description).font(.footnote).foregroundStyle(.secondary)
                }
            }
            starterTaskList(activeStarterTasks, accent: activeStarterCategoryColor)
        }
    }

    @ViewBuilder
    private func starterTaskList(_ tasks: [StarterTask], accent: Color? = nil) -> some View {
        switch starterTaskLayout {
        case .horizontalCards:
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(alignment: .top, spacing: 12) {
                    ForEach(Array(tasks.enumerated()), id: \.offset) { _, task in
                        starterTaskButton(task, width: 220, minHeight: 118, accent: accent)
                    }
                }
            }
        case .verticalList:
            VStack(spacing: 10) {
                ForEach(Array(tasks.enumerated()), id: \.offset) { _, task in
                    starterTaskButton(task, width: nil, minHeight: 96, accent: accent)
                }
            }
        }
    }

    private func starterTaskButton(
        _ task: StarterTask,
        width: CGFloat?,
        minHeight: CGFloat,
        accent: Color? = nil
    ) -> some View {
        Button {
            onSelectStarterTask(task)
        } label: {
            VStack(alignment: .leading, spacing: 6) {
                Text((task.title ?? "").trimmingCharacters(in: .whitespacesAndNewlines))
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.primary)
                    .multilineTextAlignment(.leading)
                    .lineLimit(2)
                Text((task.description ?? resolvedAgentLabel).trimmingCharacters(in: .whitespacesAndNewlines))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.leading)
                    .lineLimit(starterTaskLayout == .verticalList ? 4 : 3)
            }
            .frame(width: width, alignment: .leading)
            .frame(maxWidth: width == nil ? .infinity : nil, minHeight: minHeight, alignment: .topLeading)
            .padding(14)
            .background(
                LinearGradient(
                    colors: [Color.white, accent?.opacity(0.09) ?? Color.secondary.opacity(0.07)],
                    startPoint: .top,
                    endPoint: .bottom
                ),
                in: RoundedRectangle(cornerRadius: 8)
            )
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .stroke(accent?.opacity(0.20) ?? Color.secondary.opacity(0.08), lineWidth: 1)
            )
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier(starterTaskAccessibilityIdentifier(task))
    }
}

private func starterCategorySymbol(_ icon: String?) -> String {
    switch (icon ?? "").trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
    case "route": return "point.topleft.down.to.point.bottomright.curvepath"
    case "chart-line": return "chart.xyaxis.line"
    case "wrench": return "wrench.and.screwdriver"
    case "trend-up": return "chart.line.uptrend.xyaxis"
    case "shield-warning": return "checkmark.shield"
    default: return "sparkles"
    }
}

private func starterCategoryColor(_ index: Int) -> Color {
    let colors: [Color] = [
        Color(red: 0.28, green: 0.45, blue: 0.85),
        Color(red: 0.46, green: 0.33, blue: 0.78),
        Color(red: 0.15, green: 0.53, blue: 0.41),
        Color(red: 0.73, green: 0.42, blue: 0.18),
        Color(red: 0.72, green: 0.30, blue: 0.41),
    ]
    return colors[index % colors.count]
}

private func starterTaskAccessibilityIdentifier(_ task: StarterTask) -> String {
    let title = (task.title ?? "task")
        .lowercased()
        .map { $0.isLetter || $0.isNumber ? $0 : "-" }
    let normalized = String(title)
        .replacingOccurrences(of: "-+", with: "-", options: .regularExpression)
        .trimmingCharacters(in: CharacterSet(charactersIn: "-"))
    return "agently-starter-task-\(normalized.isEmpty ? "task" : normalized)"
}

private func humanizedAgentLabel(_ value: String) -> String {
    let normalized = value
        .replacingOccurrences(of: "_", with: " ")
        .replacingOccurrences(of: "-", with: " ")
        .split(separator: " ")
        .map { token in
            let lower = token.lowercased()
            return lower.prefix(1).uppercased() + lower.dropFirst()
        }
        .joined(separator: " ")
        .trimmingCharacters(in: .whitespacesAndNewlines)
    return normalized.isEmpty ? value : normalized
}

private func resolveWorkspaceDisplayTitle(_ value: String) -> String {
    let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
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
