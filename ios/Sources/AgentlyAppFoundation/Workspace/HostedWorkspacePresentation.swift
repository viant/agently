import Foundation
import AgentlySDK

struct HostedWorkspacePresentation: Equatable {
    let badgeLabel: String
    let badgeSymbolName: String
    let title: String
    let subtitle: String?
    let supportingText: String
}

func resolveHostedWorkspacePresentation(
    restoreState: HostedWorkspaceRestoreState?
) -> HostedWorkspacePresentation? {
    guard let restoreState else {
        return nil
    }
    let selectedID = restoreState.selectedWindowId
    let window = restoreState.windows.first(where: { $0.windowId == selectedID }) ?? restoreState.windows.last
    return resolveHostedWorkspacePresentation(window: window)
}

func resolveHostedWorkspacePresentation(
    window: WorkspaceWindowSnapshot?
) -> HostedWorkspacePresentation? {
    guard let window else {
        return nil
    }
    let badgeLabel = humanizeHostedWorkspaceKey(window.windowKey) ?? "Workspace"
    let normalizedTitle = normalizeHostedWorkspaceText(window.windowTitle)
    let title: String
    if let normalizedTitle,
       normalizedTitle.caseInsensitiveCompare(window.windowKey) != .orderedSame {
        title = normalizedTitle
    } else {
        title = badgeLabel
    }
    return HostedWorkspacePresentation(
        badgeLabel: badgeLabel,
        badgeSymbolName: "rectangle.topthird.inset.filled",
        title: title,
        subtitle: nil,
        supportingText: ""
    )
}

private func humanizeHostedWorkspaceKey(_ key: String) -> String? {
    let normalized = key
        .replacingOccurrences(of: "/", with: " ")
        .replacingOccurrences(of: "_", with: " ")
        .trimmingCharacters(in: .whitespacesAndNewlines)
    guard !normalized.isEmpty else {
        return nil
    }
    return normalized
        .split(whereSeparator: \.isWhitespace)
        .map { token in
            let lower = token.lowercased()
            return lower.prefix(1).uppercased() + lower.dropFirst()
        }
        .joined(separator: " ")
}

private func normalizeHostedWorkspaceText(_ value: String?) -> String? {
    let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    return trimmed.isEmpty ? nil : trimmed
}
