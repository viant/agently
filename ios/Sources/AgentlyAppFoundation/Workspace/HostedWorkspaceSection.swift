import SwiftUI
import AgentlySDK
import ForgeIOSRuntime
import ForgeIOSUI

private typealias SDKJSONValue = AgentlySDK.JSONValue

struct HostedWorkspaceSection: View {
    let restoreState: HostedWorkspaceRestoreState?
    let conversationState: ConversationStateResponse?
    let forgeRuntime: ForgeRuntime?
    let client: AgentlyClient?

    init(
        restoreState: HostedWorkspaceRestoreState? = nil,
        conversationState: ConversationStateResponse?,
        forgeRuntime: ForgeRuntime?,
        client: AgentlyClient? = nil
    ) {
        self.restoreState = restoreState
        self.conversationState = conversationState
        self.forgeRuntime = forgeRuntime
        self.client = client
    }

    @ViewBuilder
    var body: some View {
        let effectiveRestoreState = restoreState ?? conversationState.map { deriveHostedWorkspaceRestoreState(from: $0) } ?? nil
        if let forgeRuntime,
           let effectiveRestoreState {
            HostedWorkspaceWindowView(
                restoreState: effectiveRestoreState,
                forgeRuntime: forgeRuntime,
                client: client
            )
            .padding(.horizontal, 4)
        }
    }
}

private struct HostedWorkspaceWindowView: View {
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass

    let restoreState: HostedWorkspaceRestoreState
    let forgeRuntime: ForgeRuntime
    let client: AgentlyClient?

    @State private var selectedWindowID: String
    @State private var metadata: WindowMetadata?
    @State private var windowContext: WindowContext?
    @State private var errorMessage: String?

    init(restoreState: HostedWorkspaceRestoreState, forgeRuntime: ForgeRuntime, client: AgentlyClient?) {
        self.restoreState = restoreState
        self.forgeRuntime = forgeRuntime
        self.client = client
        _selectedWindowID = State(
            initialValue: restoreState.selectedWindowId
                ?? restoreState.windows.last?.windowId
                ?? ""
        )
    }

    private var selectedWindow: WorkspaceWindowSnapshot? {
        restoreState.windows.first(where: { $0.windowId == selectedWindowID })
            ?? restoreState.windows.last
    }

    var body: some View {
        Group {
            if let selectedWindow,
               let metadata,
               let windowContext,
               let client,
               shouldUseNativeRecommendationBrowser(selectedWindow: selectedWindow, metadata: metadata) {
                RecommendationWorkspaceBrowser(
                    snapshot: selectedWindow,
                    metadata: metadata,
                    forgeRuntime: forgeRuntime,
                    windowContext: windowContext
                )
            } else {
                VStack(alignment: .leading, spacing: 10) {
                    Text("Workspace")
                        .font(.headline)

                    if restoreState.windows.count > 1 {
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(spacing: 8) {
                                ForEach(restoreState.windows, id: \.windowId) { window in
                                    Button {
                                        selectedWindowID = window.windowId
                                    } label: {
                                        Text(window.windowTitle ?? window.windowKey)
                                            .font(.footnote.weight(window.windowId == selectedWindowID ? .semibold : .regular))
                                            .padding(.horizontal, 10)
                                            .padding(.vertical, 6)
                                            .background(
                                                (window.windowId == selectedWindowID ? Color.accentColor.opacity(0.14) : Color.secondary.opacity(0.12)),
                                                in: Capsule()
                                            )
                                    }
                                    .buttonStyle(.plain)
                                }
                            }
                        }
                    }

                    if let errorMessage {
                        Text(errorMessage)
                            .font(.footnote)
                            .foregroundStyle(.red)
                    } else if let metadata, let windowContext {
                        WindowContentView(
                            runtime: forgeRuntime,
                            window: windowContext,
                            metadata: metadata
                        )
                        .frame(maxWidth: .infinity, alignment: .leading)
                    } else {
                        Text("Loading workspace view…")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
        .modifier(HostedWorkspaceChromeModifier(applyChrome: !usesNativeRecommendationBrowser))
        .task(id: selectedWindow?.windowId) {
            guard let selectedWindow else { return }
            await load(selectedWindow)
        }
    }

    private var usesNativeRecommendationBrowser: Bool {
        guard let selectedWindow, let metadata else {
            return false
        }
        return shouldUseNativeRecommendationBrowser(selectedWindow: selectedWindow, metadata: metadata)
    }

    private func shouldUseNativeRecommendationBrowser(
        selectedWindow: WorkspaceWindowSnapshot,
        metadata: WindowMetadata
    ) -> Bool {
        _ = metadata
        let key = selectedWindow.windowKey.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return horizontalSizeClass != .regular
            && (key == "recommendation"
                || key == "recommendationlist"
                || key == "recommendationreview"
                || key.contains("recommendation"))
    }

    @MainActor
    private func load(_ selectedWindow: WorkspaceWindowSnapshot) async {
        let state: ForgeRuntime.WindowState
        if let existing = await forgeRuntime.windows.first(where: { $0.id == selectedWindow.windowId }) {
            state = existing
        } else {
            state = await forgeRuntime.openWindow(
                key: selectedWindow.windowKey,
                title: selectedWindow.windowTitle ?? selectedWindow.windowKey,
                id: selectedWindow.windowId,
                inTab: selectedWindow.inTab ?? true,
                parameters: selectedWindow.parameters?.mapValues(\.forgeValue) ?? [:],
                conversationID: selectedWindow.conversationId,
                presentation: selectedWindow.presentation,
                region: selectedWindow.region,
                parentKey: selectedWindow.parentKey
            )
        }
        windowContext = await forgeRuntime.windowContext(id: state.id)
        try? await Task.sleep(for: .milliseconds(150))
        let latest = await forgeRuntime.windows.first(where: { $0.id == state.id })
        metadata = latest?.metadata
        _ = client
        errorMessage = metadata == nil ? "Workspace metadata did not load." : nil
    }

    private func shouldSelectFirstRow(
        metadata: WindowMetadata,
        dataSourceRef: String,
        selectedWindow: WorkspaceWindowSnapshot
    ) -> Bool {
        let containers = metadata.view?.content?.containers ?? []
        return containers.contains(where: { container in
            container.dataSourceRef == dataSourceRef && container.selectFirst == true
        })
    }

}

private struct HostedWorkspaceChromeModifier: ViewModifier {
    let applyChrome: Bool

    func body(content: Content) -> some View {
        if applyChrome {
            content
                .padding(12)
                .background(Color.secondary.opacity(0.05), in: RoundedRectangle(cornerRadius: 20))
                .overlay(
                    RoundedRectangle(cornerRadius: 20)
                        .stroke(Color.secondary.opacity(0.12), lineWidth: 1)
                )
        } else {
            content
        }
    }
}

func deriveHostedWorkspaceRestoreState(from response: ConversationStateResponse) -> HostedWorkspaceRestoreState? {
    let turns = response.conversation?.turns ?? []
    for turn in turns.reversed() {
        let toolSteps = turn.execution?.pages.flatMap(\.toolSteps) ?? []
        for step in toolSteps.reversed() where (step.status ?? "").lowercased() == "completed" {
            let toolName = normalizeHostedWorkspaceToolName(step.toolName)
            if toolName == "ui/window/list" {
                let windows = hostedWorkspaceWindowsFromListPayload(
                    firstParsedPayload(step.responsePayload, step.content)
                )
                if !windows.isEmpty {
                    let selectedWindowID = selectedWindowIDFromToolSteps(toolSteps, windows)
                    return HostedWorkspaceRestoreState(
                        windows: windows,
                        selectedWindowId: selectedWindowID.isEmpty ? nil : selectedWindowID
                    )
                }
            }
            if toolName == "ui/view/open" {
                let windows = hostedWorkspaceWindowsFromViewOpenStep(step)
                if !windows.isEmpty {
                    let responsePayload = firstParsedPayload(step.responsePayload, step.content)
                    let selectedWindowID = responsePayload?.objectValue?["selectedWindowId"]?.stringValue
                        ?? windows.last?.windowId
                    return HostedWorkspaceRestoreState(
                        windows: windows,
                        selectedWindowId: selectedWindowID?.isEmpty == false ? selectedWindowID : nil
                    )
                }
            }
        }
    }
    return nil
}

private func hostedWorkspaceWindowsFromListPayload(_ raw: SDKJSONValue?) -> [WorkspaceWindowSnapshot] {
    guard let payload = raw,
          let items = payload.objectValue?["items"]?.arrayValue else {
        return []
    }
    return items.compactMap { normalizeHostedWorkspaceWindow($0.objectValue) }
}

private func hostedWorkspaceWindowsFromViewOpenStep(_ step: ToolStepState) -> [WorkspaceWindowSnapshot] {
    let responsePayload = firstParsedPayload(step.responsePayload, step.content)
    if let items = responsePayload?.objectValue?["items"]?.arrayValue, !items.isEmpty {
        return items.compactMap { normalizeHostedWorkspaceWindow($0.objectValue) }
    }
    let requestPayload = firstParsedPayload(step.requestPayload, nil)
    let merged: [String: SDKJSONValue] = [
        "windowId": responsePayload?.objectValue?["windowId"] ?? .string(""),
        "conversationId": responsePayload?.objectValue?["conversationId"] ?? .string(""),
        "windowKey": responsePayload?.objectValue?["windowKey"] ?? requestPayload?.objectValue?["id"] ?? .string(""),
        "windowTitle": responsePayload?.objectValue?["windowTitle"] ?? .string(""),
        "presentation": responsePayload?.objectValue?["presentation"] ?? .string(""),
        "region": responsePayload?.objectValue?["region"] ?? .string(""),
        "parentKey": responsePayload?.objectValue?["parentKey"] ?? .string(""),
        "parameters": requestPayload?.objectValue?["parameters"] ?? .object([:])
    ]
    return [normalizeHostedWorkspaceWindow(merged)].compactMap { $0 }
}

private func normalizeHostedWorkspaceWindow(_ raw: [String: SDKJSONValue]?) -> WorkspaceWindowSnapshot? {
    guard let raw else { return nil }
    let presentation = raw["presentation"]?.stringValue?.lowercased() ?? ""
    let region = raw["region"]?.stringValue?.lowercased() ?? ""
    let parentKey = raw["parentKey"]?.stringValue ?? ""
    let windowID = raw["windowId"]?.stringValue ?? ""
    let windowKey = raw["windowKey"]?.stringValue ?? ""
    guard !windowID.isEmpty, !windowKey.isEmpty else { return nil }
    guard presentation == "hosted", region == "chat.top", parentKey == "chat/new" else { return nil }
    return WorkspaceWindowSnapshot(
        windowId: windowID,
        conversationId: raw["conversationId"]?.stringValue,
        windowKey: windowKey,
        windowTitle: raw["windowTitle"]?.stringValue?.isEmpty == false ? raw["windowTitle"]?.stringValue : windowKey,
        presentation: raw["presentation"]?.stringValue,
        region: raw["region"]?.stringValue,
        parentKey: parentKey,
        inTab: true,
        parameters: raw["parameters"]?.objectValue
    )
}

private func selectedWindowIDFromToolSteps(
    _ toolSteps: [ToolStepState],
    _ windows: [WorkspaceWindowSnapshot]
) -> String {
    let windowIDs = Set(windows.map(\.windowId))
    for step in toolSteps.reversed() where (step.status ?? "").lowercased() == "completed" {
        let toolName = normalizeHostedWorkspaceToolName(step.toolName)
        if toolName == "ui/window/show" {
            let requestPayload = firstParsedPayload(step.requestPayload, nil)
            let windowID = requestPayload?.objectValue?["windowId"]?.stringValue ?? ""
            if windowIDs.contains(windowID) {
                return windowID
            }
        }
        if toolName == "ui/window/list" {
            let responsePayload = firstParsedPayload(step.responsePayload, step.content)
            let focusedWindowID = responsePayload?.objectValue?["focusedWindowId"]?.stringValue ?? ""
            if windowIDs.contains(focusedWindowID) {
                return focusedWindowID
            }
        }
    }
    return ""
}

private func firstParsedPayload(_ raw: SDKJSONValue?, _ rawText: String?) -> SDKJSONValue? {
    if let raw {
        return parsePayload(raw)
    }
    if let rawText {
        return parsePayload(.string(rawText))
    }
    return nil
}

private func parsePayload(_ raw: SDKJSONValue) -> SDKJSONValue? {
    switch raw {
    case .string(let value):
        let trimmed = value.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines)
        guard !trimmed.isEmpty, let data = trimmed.data(using: String.Encoding.utf8) else { return nil }
        return try? JSONDecoder.agently().decode(SDKJSONValue.self, from: data)
    case .object(let object):
        if let inlineBody = object["inlineBody"]?.stringValue ?? object["InlineBody"]?.stringValue,
           !inlineBody.isEmpty,
           let data = inlineBody.data(using: String.Encoding.utf8),
           let decoded = try? JSONDecoder.agently().decode(SDKJSONValue.self, from: data) {
            return decoded
        }
        return raw
    default:
        return raw
    }
}

private func normalizeHostedWorkspaceToolName(_ raw: String?) -> String {
    String(raw ?? "")
        .trimmingCharacters(in: CharacterSet.whitespacesAndNewlines)
        .lowercased()
        .replacingOccurrences(of: ":", with: "/")
}

private extension SDKJSONValue {
    var stringValue: String? {
        if case .string(let value) = self {
            return value
        }
        return nil
    }

    var objectValue: [String: SDKJSONValue]? {
        if case .object(let value) = self {
            return value
        }
        return nil
    }

    var arrayValue: [SDKJSONValue]? {
        if case .array(let value) = self {
            return value
        }
        return nil
    }
}
