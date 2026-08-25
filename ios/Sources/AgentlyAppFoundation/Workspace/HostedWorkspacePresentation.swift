import Foundation
import AgentlySDK

public struct HostedWorkspacePresentation: Equatable {
    public let badgeLabel: String
    public let badgeSymbolName: String
    public let title: String
    public let subtitle: String?
    public let supportingText: String
}

public struct HostedWorkspaceAttachment: Equatable {
    public let turnID: String
    public let presentation: HostedWorkspacePresentation
}

func isHostedWorkspaceOpeningToolName(_ value: String) -> Bool {
    let normalized = value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        .replacingOccurrences(of: ":", with: "/")
    return ["ui/view/open", "ui/window/open", "ui/window/show"].contains(normalized)
}

func hostedWorkspaceAttachment(
    conversationState: ConversationStateResponse?,
    restoreState: HostedWorkspaceRestoreState?
) -> HostedWorkspaceAttachment? {
    guard let presentation = resolveHostedWorkspacePresentation(restoreState: restoreState) else { return nil }
    let turn = conversationState?.conversation?.turns.reversed().first(where: { turn in
        turn.execution?.pages.contains(where: { page in
            page.toolSteps.contains(where: { step in
                let status = step.status?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
                return (status.isEmpty || ["completed", "succeeded", "success", "done"].contains(status))
                    && isHostedWorkspaceOpeningToolName(step.toolName)
            })
        }) == true
    })
    guard let turnID = turn?.turnID.trimmingCharacters(in: .whitespacesAndNewlines), !turnID.isEmpty else { return nil }
    return HostedWorkspaceAttachment(turnID: turnID, presentation: presentation)
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
    let explicitLabel = normalizeHostedWorkspaceText(window.navigation?.label)
    let badgeLabel = explicitLabel ?? humanizeHostedWorkspaceKey(window.windowKey) ?? "Workspace"
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
        badgeSymbolName: hostedWorkspaceSymbol(window.navigation?.icon),
        title: title,
        subtitle: nil,
        supportingText: "Open the \(badgeLabel.lowercased()) workspace."
    )
}

private func hostedWorkspaceSymbol(_ value: String?) -> String {
    switch value?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
    case "chart": return "chart.xyaxis.line"
    case "document": return "doc.text"
    case "application": return "app"
    default: return "rectangle.topthird.inset.filled"
    }
}

private func humanizeHostedWorkspaceKey(_ key: String) -> String? {
    let normalized = key
        .replacingOccurrences(of: "/", with: " ")
        .replacingOccurrences(of: "_", with: " ")
        .replacingOccurrences(
            of: "([a-z0-9])([A-Z])",
            with: "$1 $2",
            options: .regularExpression
        )
        .replacingOccurrences(
            of: "([A-Z]+)([A-Z][a-z])",
            with: "$1 $2",
            options: .regularExpression
        )
        .trimmingCharacters(in: .whitespacesAndNewlines)
    guard !normalized.isEmpty else {
        return nil
    }
    return normalized
        .split(whereSeparator: \.isWhitespace)
        .map { humanizeHostedWorkspaceToken(String($0)) }
        .joined(separator: " ")
}

private func humanizeHostedWorkspaceToken(_ token: String) -> String {
    let trimmed = token.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty else {
        return trimmed
    }
    let hasLowercase = trimmed.rangeOfCharacter(from: .lowercaseLetters) != nil
    guard hasLowercase else {
        return trimmed
    }
    let lower = trimmed.lowercased()
    return lower.prefix(1).uppercased() + lower.dropFirst()
}

private func normalizeHostedWorkspaceText(_ value: String?) -> String? {
    let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    return trimmed.isEmpty ? nil : trimmed
}
