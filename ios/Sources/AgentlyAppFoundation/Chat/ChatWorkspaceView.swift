import SwiftUI
import AgentlySDK

struct ChatWorkspaceView: View {
    let metadata: WorkspaceMetadata?
    let selectedAgentID: String?
    let availableAgents: [WorkspaceAgentOption]
    let onSelectAgent: (String?) -> Void
    let showStarterTasks: Bool
    let onSelectStarterTask: (StarterTask) -> Void

    init(
        metadata: WorkspaceMetadata?,
        selectedAgentID: String?,
        availableAgents: [WorkspaceAgentOption],
        onSelectAgent: @escaping (String?) -> Void,
        showStarterTasks: Bool = false,
        onSelectStarterTask: @escaping (StarterTask) -> Void = { _ in }
    ) {
        self.metadata = metadata
        self.selectedAgentID = selectedAgentID
        self.availableAgents = availableAgents
        self.onSelectAgent = onSelectAgent
        self.showStarterTasks = showStarterTasks
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
        return resolveWorkspaceBrandTitle(
            workspaceTitle: preferred ?? resolvedAgentLabel,
            fallbackTitle: "Agently"
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

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
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

            if showStarterTasks {
                VStack(alignment: .leading, spacing: 10) {
                    Text(starterTaskAgentCount > 1 ? "Start with an agent prompt" : "Starter tasks")
                        .font(.headline)
                    if starterTasks.isEmpty {
                        Text("This agent has no published starter tasks yet. You can still begin with your own prompt below.")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    } else {
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(alignment: .top, spacing: 12) {
                                ForEach(Array(starterTasks.enumerated()), id: \.offset) { _, task in
                                    Button {
                                        onSelectStarterTask(task)
                                    } label: {
                                        VStack(alignment: .leading, spacing: 6) {
                                            Text((task.title ?? "").trimmingCharacters(in: .whitespacesAndNewlines))
                                                .font(.subheadline.weight(.semibold))
                                                .foregroundStyle(.primary)
                                                .multilineTextAlignment(.leading)
                                            Text((task.description ?? resolvedAgentLabel).trimmingCharacters(in: .whitespacesAndNewlines))
                                                .font(.footnote)
                                                .foregroundStyle(.secondary)
                                                .multilineTextAlignment(.leading)
                                                .lineLimit(3)
                                        }
                                        .frame(width: 220, alignment: .leading)
                                        .padding(14)
                                        .background(Color.secondary.opacity(0.07), in: RoundedRectangle(cornerRadius: 18))
                                    }
                                    .buttonStyle(.plain)
                                }
                            }
                        }
                    }
                }
            }
        }
        .padding(.horizontal, 20)
        .padding(.top, 12)
        .padding(.bottom, showStarterTasks ? 12 : 4)
    }
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
