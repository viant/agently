import Foundation
import AgentlySDK

func deriveAgentlyHostedWorkspaceRestoreState(
    from response: ConversationStateResponse
) -> HostedWorkspaceRestoreState? {
    filterAgentlyHostedWorkspaceRestoreState(
        deriveHostedWorkspaceRestoreState(from: response)
    )
}

func deriveAgentlyHostedWorkspaceRestoreState(
    from response: ConversationStateResponse?,
    streamSnapshot: ConversationStreamSnapshot?
) -> HostedWorkspaceRestoreState? {
    filterAgentlyHostedWorkspaceRestoreState(
        deriveHostedWorkspaceRestoreState(from: response, streamSnapshot: streamSnapshot)
    )
}

func filterAgentlyHostedWorkspaceRestoreState(
    _ restoreState: HostedWorkspaceRestoreState?
) -> HostedWorkspaceRestoreState? {
    guard let restoreState else {
        return nil
    }
    let windows = restoreState.windows.filter(isAgentlyHostedWorkspaceWindow)
    guard !windows.isEmpty else {
        return nil
    }
    let selectedWindowID = restoreState.selectedWindowId
        .flatMap { selected in windows.contains(where: { $0.windowId == selected }) ? selected : nil }
        ?? windows.last?.windowId
    return HostedWorkspaceRestoreState(
        windows: windows,
        selectedWindowId: selectedWindowID?.isEmpty == false ? selectedWindowID : nil
    )
}

private func isAgentlyHostedWorkspaceWindow(_ window: WorkspaceWindowSnapshot) -> Bool {
    window.presentation?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == "hosted" &&
        window.region?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == "chat.top" &&
        window.parentKey?.trimmingCharacters(in: .whitespacesAndNewlines) == "chat/new"
}
