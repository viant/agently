import Foundation
import AgentlySDK
import ForgeIOSRuntime

struct HostedWorkspaceEventNotice: Equatable {
    let invalidWorkspaceID: String
    let availableWorkspaceIDs: [String]

    var message: String {
        let available = availableWorkspaceIDs.joined(separator: ", ")
        if available.isEmpty {
            return "Workspace view \"\(invalidWorkspaceID)\" is not published for this workspace."
        }
        return "Workspace view \"\(invalidWorkspaceID)\" is not published for this workspace. Available views: \(available)."
    }
}

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
    let windows = restoreState.windows
        .filter(isAgentlyHostedWorkspaceWindow)
        .map(seedHostedWorkspaceWindowForm)
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

private func seedHostedWorkspaceWindowForm(_ window: WorkspaceWindowSnapshot) -> WorkspaceWindowSnapshot {
    guard let parameters = window.parameters, !parameters.isEmpty else {
        return window
    }
    let seededWindowForm = mergeHostedWindowJSONObjects(
        base: parameters,
        overlay: window.windowForm ?? [:]
    )
    return WorkspaceWindowSnapshot(
        windowId: window.windowId,
        conversationId: window.conversationId,
        windowKey: window.windowKey,
        windowTitle: window.windowTitle,
        presentation: window.presentation,
        region: window.region,
        parentKey: window.parentKey,
        workspaceSharePct: window.workspaceSharePct,
        workspaceMinHeight: window.workspaceMinHeight,
        inTab: window.inTab,
        parameters: window.parameters,
        windowForm: seededWindowForm
    )
}

private func mergeHostedWindowJSONObjects(
    base: [String: AgentlySDK.JSONValue],
    overlay: [String: AgentlySDK.JSONValue]
) -> [String: AgentlySDK.JSONValue] {
    var merged = base
    for (key, value) in overlay {
        if case .object(let baseObject)? = merged[key],
           case .object(let overlayObject) = value {
            merged[key] = .object(mergeHostedWindowJSONObjects(base: baseObject, overlay: overlayObject))
        } else {
            merged[key] = value
        }
    }
    return merged
}

func hostedWorkspaceEventNotice(from events: [UIEvent]) -> HostedWorkspaceEventNotice? {
    for event in events.reversed() {
        guard case .object(let payload)? = event.detail?["payload"],
              case .string(let invalidID)? = payload["invalidWorkspaceId"] else {
            continue
        }
        let trimmedInvalidID = invalidID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedInvalidID.isEmpty else { continue }
        let availableIDs: [String]
        if case .array(let values)? = payload["availableWorkspaceIds"] {
            availableIDs = values.compactMap { value in
                guard case .string(let id) = value else { return nil }
                let trimmed = id.trimmingCharacters(in: .whitespacesAndNewlines)
                return trimmed.isEmpty ? nil : trimmed
            }
        } else {
            availableIDs = []
        }
        return HostedWorkspaceEventNotice(
            invalidWorkspaceID: trimmedInvalidID,
            availableWorkspaceIDs: availableIDs
        )
    }
    return nil
}

func shouldApplyHostedWorkspaceNotice(activeConversationID: String?, targetConversationID: String?) -> Bool {
    let active = activeConversationID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let target = targetConversationID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    return !active.isEmpty && active == target
}

func shouldReuseExistingHostedWorkspaceWindow(
    _ existing: ForgeRuntime.WindowState,
    for selected: WorkspaceWindowSnapshot
) -> Bool {
    existing.metadata != nil &&
        existing.id == selected.windowId &&
        existing.key == selected.windowKey &&
        existing.conversationID == selected.conversationId &&
        existing.presentation == selected.presentation &&
        existing.region == selected.region &&
        existing.parentKey == selected.parentKey
}
