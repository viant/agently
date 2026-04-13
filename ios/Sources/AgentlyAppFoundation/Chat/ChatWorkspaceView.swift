import SwiftUI
import AgentlySDK

struct ChatWorkspaceView: View {
    let metadata: WorkspaceMetadata?
    let selectedAgentID: String?
    let availableAgents: [WorkspaceAgentOption]
    let onSelectAgent: (String?) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            VStack(alignment: .leading, spacing: 6) {
                Text(workspaceDisplayTitle)
                    .font(.title3.weight(.semibold))
                    .lineLimit(2)
                if let workspaceRoot = metadata?.workspaceRoot,
                   workspaceRoot != workspaceDisplayTitle {
                    Text(workspaceRoot)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                }
                Text(metadataSummary)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }

            WorkspaceMetadataChipRow(
                chips: metadataChips
            )

            if !availableAgents.isEmpty {
                AgentSelectionSection(
                    agents: availableAgents,
                    selectedAgentID: selectedAgentID,
                    onSelectAgent: onSelectAgent
                )
            }
        }
        .padding(.horizontal)
        .padding(.top, 12)
        .padding(.bottom, 8)
    }

    private var workspaceDisplayTitle: String {
        metadata?.workspaceRoot?.workspaceDisplayTitle ?? "Workspace loading"
    }

    private var metadataSummary: String {
        guard let metadata else {
            return "Loading workspace metadata"
        }

        let availableAgentCount = max(metadata.agentInfos.count, metadata.agents.count)
        let availableModelCount = max(metadata.modelInfos.count, metadata.models.count)
        if availableAgentCount > 0 && availableModelCount > 0 {
            return "\(availableAgentCount) available agent\(availableAgentCount == 1 ? "" : "s")" +
                " • \(availableModelCount) model\(availableModelCount == 1 ? "" : "s")"
        }
        if availableAgentCount > 0 {
            return "\(availableAgentCount) available agent\(availableAgentCount == 1 ? "" : "s")"
        }
        if availableModelCount > 0 {
            return "\(availableModelCount) available model\(availableModelCount == 1 ? "" : "s")"
        }

        return "No active agent metadata yet"
    }

    private var metadataChips: [WorkspaceMetadataChip] {
        guard let metadata else {
            return [
                WorkspaceMetadataChip(
                    title: "Status",
                    value: "Connecting",
                    systemImage: "dot.radiowaves.left.and.right"
                )
            ]
        }

        var chips: [WorkspaceMetadataChip] = []

        if let defaultAgent = metadata.defaultAgent ?? metadata.defaults?.agent, !defaultAgent.isEmpty {
            chips.append(WorkspaceMetadataChip(title: "Agent", value: defaultAgent, systemImage: "person.crop.square"))
        }

        if let defaultModel = metadata.defaultModel ?? metadata.defaults?.model, !defaultModel.isEmpty {
            chips.append(WorkspaceMetadataChip(title: "Model", value: defaultModel, systemImage: "cpu"))
        }

        if let defaultEmbedder = metadata.defaultEmbedder ?? metadata.defaults?.embedder, !defaultEmbedder.isEmpty {
            chips.append(WorkspaceMetadataChip(title: "Embedder", value: defaultEmbedder, systemImage: "sparkles.rectangle.stack"))
        }

        let availableAgentCount = max(metadata.agentInfos.count, metadata.agents.count)
        if availableAgentCount > 0 {
            chips.append(
                WorkspaceMetadataChip(
                    title: "Agents",
                    value: "\(availableAgentCount)",
                    systemImage: "person.3"
                )
            )
        }

        let availableModelCount = max(metadata.modelInfos.count, metadata.models.count)
        if availableModelCount > 0 {
            chips.append(
                WorkspaceMetadataChip(
                    title: "Models",
                    value: "\(availableModelCount)",
                    systemImage: "circle.grid.2x2"
                )
            )
        }

        if metadata.capabilities?.toolAutoSelection == true || metadata.defaults?.autoSelectTools == true {
            chips.append(
                WorkspaceMetadataChip(
                    title: "Tools",
                    value: "Auto Select",
                    systemImage: "wand.and.stars"
                )
            )
        }

        if let version = metadata.version, !version.isEmpty {
            chips.append(
                WorkspaceMetadataChip(
                    title: "Version",
                    value: version,
                    systemImage: "number"
                )
            )
        }

        return chips
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

private struct WorkspaceMetadataChip: Hashable {
    let title: String
    let value: String
    let systemImage: String
}

private struct AgentSelectionSection: View {
    let agents: [WorkspaceAgentOption]
    let selectedAgentID: String?
    let onSelectAgent: (String?) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Active Agent")
                .font(.footnote.weight(.semibold))
                .foregroundStyle(.secondary)
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    Button {
                        onSelectAgent(nil)
                    } label: {
                        agentChip(
                            title: "Workspace Default",
                            subtitle: nil,
                            isSelected: selectedAgentID == nil || selectedAgentID?.isEmpty == true
                        )
                    }
                    .buttonStyle(.plain)

                    ForEach(agents) { agent in
                        Button {
                            onSelectAgent(agent.id)
                        } label: {
                            agentChip(
                                title: agent.displayName,
                                subtitle: agent.modelRef,
                                isSelected: selectedAgentID == agent.id
                            )
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        }
    }

    private func agentChip(title: String, subtitle: String?, isSelected: Bool) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title)
                .font(.caption.weight(.semibold))
                .foregroundStyle(isSelected ? .white : .primary)
            if let subtitle, !subtitle.isEmpty {
                Text(subtitle)
                    .font(.caption2)
                    .foregroundStyle(isSelected ? Color.white.opacity(0.8) : .secondary)
                    .lineLimit(1)
            }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 8)
        .background(isSelected ? Color.accentColor : Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
    }
}

private struct WorkspaceMetadataChipRow: View {
    let chips: [WorkspaceMetadataChip]

    var body: some View {
        if !chips.isEmpty {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    ForEach(chips, id: \.self) { chip in
                        Label {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(chip.title)
                                    .font(.caption2)
                                    .foregroundStyle(.secondary)
                                Text(chip.value)
                                    .font(.caption.weight(.semibold))
                                    .lineLimit(1)
                            }
                        } icon: {
                            Image(systemName: chip.systemImage)
                                .foregroundStyle(.secondary)
                        }
                        .padding(.horizontal, 10)
                        .padding(.vertical, 8)
                        .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
                    }
                }
            }
        }
    }
}
