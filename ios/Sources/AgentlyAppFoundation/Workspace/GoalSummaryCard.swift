import SwiftUI
import AgentlySDK

struct GoalSummaryCard: View {
    let goal: Goal

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Goal")
                    .font(.headline)
                Spacer(minLength: 0)
                Text(goal.status.replacingOccurrences(of: "_", with: " ").capitalized)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.secondary)
            }
            Text(goal.objective)
                .font(.subheadline)
                .foregroundStyle(.primary)
            if let reason = nonEmpty(goal.statusReason) ?? nonEmpty(goal.pauseReason) {
                Text(reason)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            HStack(spacing: 12) {
                if let tokenBudget = goal.tokenBudget {
                    metric("Tokens", "\(goal.tokensUsed ?? 0)/\(tokenBudget)")
                } else {
                    metric("Tokens", "\(goal.tokensUsed ?? 0)")
                }
                metric("Time", "\(goal.timeUsedSeconds ?? 0)s")
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }

    @ViewBuilder
    private func metric(_ title: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title)
                .font(.caption2)
                .foregroundStyle(.secondary)
            Text(value)
                .font(.caption.weight(.medium))
                .foregroundStyle(.primary)
        }
    }

    private func nonEmpty(_ value: String?) -> String? {
        let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return trimmed.isEmpty ? nil : trimmed
    }
}
