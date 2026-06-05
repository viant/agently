import SwiftUI
import AgentlySDK

struct ExecutionInspectorSection: View {
    let state: ConversationStateResponse?

    private var pages: [ExecutionPageState] {
        state?.conversation?.turns
            .last(where: { !($0.execution?.pages.isEmpty ?? true) })?
            .execution?.pages ?? []
    }

    var body: some View {
        if pages.isEmpty {
            ContentUnavailableView(
                "No Execution Details",
                systemImage: "bolt.horizontal.circle",
                description: Text("This conversation does not have execution details for the current turn.")
            )
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 12) {
                    ForEach(Array(pages.enumerated()), id: \.element.id) { index, page in
                        executionPageCard(page, title: "Page \(index + 1)")
                    }
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 12)
            }
        }
    }

    @ViewBuilder
    private func executionPageCard(_ page: ExecutionPageState, title: String) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(title)
                    .font(.headline)
                if let status = page.status?.trimmingCharacters(in: .whitespacesAndNewlines), !status.isEmpty {
                    Text(status.capitalized)
                        .font(.caption.weight(.semibold))
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                        .background(statusTint(status).opacity(0.12), in: Capsule())
                        .foregroundStyle(statusTint(status))
                }
            }

            if let subtitle = pageSubtitle(page) {
                Text(subtitle)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            if let narration = page.narration?.trimmingCharacters(in: .whitespacesAndNewlines), !narration.isEmpty {
                Text(narration)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            if !page.modelSteps.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    ForEach(Array(page.modelSteps.enumerated()), id: \.element.id) { index, step in
                        executionStepRow(
                            title: "Model \(index + 1)",
                            primary: [step.provider, step.model].compactMap { value in
                                let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
                                return trimmed.isEmpty ? nil : trimmed
                            }.joined(separator: " / "),
                            secondary: step.status
                        )
                    }
                }
            }

            if !page.toolSteps.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    ForEach(Array(page.toolSteps.enumerated()), id: \.element.id) { index, step in
                        executionStepRow(
                            title: "Tool \(index + 1)",
                            primary: step.toolName,
                            secondary: step.linkedConversationTitle ?? step.status
                        )
                    }
                }
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.secondary.opacity(0.06), in: RoundedRectangle(cornerRadius: 16))
        .overlay(
            RoundedRectangle(cornerRadius: 16)
                .stroke(Color.black.opacity(0.05), lineWidth: 1)
        )
    }

    @ViewBuilder
    private func executionStepRow(title: String, primary: String?, secondary: String?) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(title)
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
            if let primary, !primary.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                Text(primary)
                    .font(.subheadline.weight(.medium))
            }
            if let secondary, !secondary.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                Text(secondary)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 2)
    }

    private func pageSubtitle(_ page: ExecutionPageState) -> String? {
        let parts: [String] = [
            page.executionRole?.trimmingCharacters(in: .whitespacesAndNewlines),
            page.phase?.trimmingCharacters(in: .whitespacesAndNewlines),
            page.mode?.trimmingCharacters(in: .whitespacesAndNewlines),
            page.iteration.map { "iteration \($0)" }
        ].compactMap { value in
            guard let value, !value.isEmpty else { return nil }
            return value
        }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    private func statusTint(_ status: String) -> Color {
        switch status.lowercased() {
        case "failed", "error":
            return .red
        case "cancelled", "canceled":
            return .orange
        case "completed", "done", "succeeded":
            return .green
        default:
            return .blue
        }
    }
}
