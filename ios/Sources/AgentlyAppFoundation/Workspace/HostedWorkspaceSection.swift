import SwiftUI
import AgentlySDK
import ForgeIOSRuntime
import ForgeIOSUI

private typealias SDKJSONValue = AgentlySDK.JSONValue

struct HostedWorkspaceSection: View {
    let restoreState: HostedWorkspaceRestoreState?
    let conversationState: ConversationStateResponse?
    let forgeRuntime: ForgeRuntime?
    let showTitle: Bool
    @Binding var displayMode: HostedWorkspaceDisplayMode

    init(
        restoreState: HostedWorkspaceRestoreState? = nil,
        conversationState: ConversationStateResponse?,
        forgeRuntime: ForgeRuntime?,
        showTitle: Bool = true,
        displayMode: Binding<HostedWorkspaceDisplayMode> = .constant(.standard)
    ) {
        self.restoreState = restoreState
        self.conversationState = conversationState
        self.forgeRuntime = forgeRuntime
        self.showTitle = showTitle
        self._displayMode = displayMode
    }

    @ViewBuilder
    var body: some View {
        let effectiveRestoreState = restoreState ?? conversationState.map { deriveHostedWorkspaceRestoreState(from: $0) } ?? nil
        if let forgeRuntime,
           let effectiveRestoreState {
            HostedWorkspaceWindowView(
                restoreState: effectiveRestoreState,
                forgeRuntime: forgeRuntime,
                showTitle: showTitle,
                displayMode: $displayMode
            )
            .padding(.horizontal, 4)
        }
    }
}

private struct HostedWorkspaceWindowView: View {
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass

    let restoreState: HostedWorkspaceRestoreState
    let forgeRuntime: ForgeRuntime
    let showTitle: Bool
    @Binding var displayMode: HostedWorkspaceDisplayMode

    @State private var selectedWindowID: String
    @State private var metadata: WindowMetadata?
    @State private var windowContext: WindowContext?
    @State private var errorMessage: String?

    init(
        restoreState: HostedWorkspaceRestoreState,
        forgeRuntime: ForgeRuntime,
        showTitle: Bool,
        displayMode: Binding<HostedWorkspaceDisplayMode>
    ) {
        self.restoreState = restoreState
        self.forgeRuntime = forgeRuntime
        self.showTitle = showTitle
        self._displayMode = displayMode
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
               shouldUseNativeHostedBrowser(metadata: metadata) {
                HostedWorkspaceBrowser(
                    snapshot: selectedWindow,
                    metadata: metadata,
                    forgeRuntime: forgeRuntime,
                    windowContext: windowContext
                )
            } else {
                VStack(alignment: .leading, spacing: 10) {
                    if showTitle {
                        headerRow
                    } else if displayMode != .minimized {
                        controlRow
                    }

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

                    if displayMode != .minimized {
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
                .background(
                    GeometryReader { proxy in
                        Color.clear
                            .preference(key: HostedWorkspaceContentHeightPreferenceKey.self, value: proxy.size.height)
                    }
                )
            }
        }
        .modifier(HostedWorkspaceChromeModifier(applyChrome: !usesNativeRecommendationBrowser))
        .onChange(of: restoreSelectionKey) { _, _ in
            selectedWindowID = restoreState.selectedWindowId
                ?? restoreState.windows.last?.windowId
                ?? ""
            metadata = nil
            windowContext = nil
            errorMessage = nil
        }
        .task(id: selectedWindow?.windowId) {
            guard let selectedWindow else { return }
            await load(selectedWindow)
        }
    }

    private var headerRow: some View {
        HStack(alignment: .center, spacing: 12) {
            Text(selectedWindow?.windowTitle ?? selectedWindow?.windowKey ?? "Workspace")
                .font(.title3.weight(.semibold))
                .foregroundStyle(.primary)
                .lineLimit(1)

            Spacer(minLength: 0)

            HStack(spacing: 10) {
                controlButtons
            }
        }
    }

    private var controlRow: some View {
        HStack {
            Spacer(minLength: 0)
            controlButtons
        }
    }

    private var controlButtons: some View {
        HStack(spacing: 10) {
                workspaceControlDot(
                    color: Color(red: 0.95, green: 0.76, blue: 0.24),
                    accessibilityLabel: displayMode == .minimized ? "Restore workspace" : "Minimize workspace"
                ) {
                    displayMode = displayMode == .minimized ? .standard : .minimized
                }
                workspaceControlDot(
                    color: Color(red: 0.20, green: 0.76, blue: 0.44),
                    accessibilityLabel: displayMode == .expanded ? "Restore workspace size" : "Expand workspace"
                ) {
                    displayMode = displayMode == .expanded ? .standard : .expanded
                }
                workspaceControlDot(
                    color: Color(red: 0.93, green: 0.36, blue: 0.31),
                    accessibilityLabel: "Close workspace"
                ) {
                    displayMode = .closed
                }
        }
    }

    private func workspaceControlDot(
        color: Color,
        accessibilityLabel: String,
        action: @escaping () -> Void
    ) -> some View {
        Button(action: action) {
            ZStack {
                Circle()
                    .fill(color)
                    .frame(width: 12, height: 12)
                    .overlay(
                        Circle()
                            .stroke(Color.black.opacity(0.08), lineWidth: 0.5)
                    )
            }
            .frame(width: 44, height: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(accessibilityLabel)
    }

    private var restoreSelectionKey: String {
        let selected = restoreState.selectedWindowId ?? ""
        let windows = restoreState.windows.map(\.windowId).joined(separator: "|")
        return "\(selected)#\(windows)"
    }

    private var usesNativeRecommendationBrowser: Bool {
        guard let metadata else {
            return false
        }
        return shouldUseNativeHostedBrowser(metadata: metadata)
    }

    private func shouldUseNativeHostedBrowser(
        metadata: WindowMetadata
    ) -> Bool {
        guard horizontalSizeClass != .regular else {
            return false
        }
        let containers = metadata.view?.content?.containers ?? []
        let hasTable = containers.contains { $0.table != nil }
        let hasNestedDetail = containers.contains { !$0.containers.isEmpty }
        return hasTable && hasNestedDetail
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
                workspaceSharePct: selectedWindow.workspaceSharePct,
                workspaceMinHeight: selectedWindow.workspaceMinHeight,
                parentKey: selectedWindow.parentKey
            )
        }
        if let windowForm = selectedWindow.windowForm?.mapValues(\.forgeValue), !windowForm.isEmpty {
            await forgeRuntime.setWindowFormValue(
                windowID: state.id,
                values: windowForm,
                replace: true
            )
        }
        windowContext = await forgeRuntime.windowContext(id: state.id)
        try? await Task.sleep(for: .milliseconds(150))
        let latest = await forgeRuntime.windows.first(where: { $0.id == state.id })
        metadata = latest?.metadata
        errorMessage = metadata == nil ? "Workspace metadata did not load." : nil
    }

}

struct HostedWorkspaceContentHeightPreferenceKey: PreferenceKey {
    static var defaultValue: CGFloat = 0

    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = max(value, nextValue())
    }
}

private struct HostedWorkspaceChromeModifier: ViewModifier {
    let applyChrome: Bool

    func body(content: Content) -> some View {
        if applyChrome {
            content
                .padding(10)
                .background(Color(.systemBackground), in: RoundedRectangle(cornerRadius: 22))
                .overlay(
                    RoundedRectangle(cornerRadius: 22)
                        .stroke(Color.black.opacity(0.06), lineWidth: 1)
                )
                .shadow(color: Color.black.opacity(0.04), radius: 18, x: 0, y: 8)
        } else {
            content
        }
    }
}

enum HostedWorkspaceDisplayMode {
    case standard
    case expanded
    case minimized
    case closed
}

func deriveHostedWorkspaceRestoreState(from response: ConversationStateResponse) -> HostedWorkspaceRestoreState? {
    let turns = response.conversation?.turns ?? []
    guard let lastTurn = turns.last else {
        return nil
    }
    let toolSteps = lastTurn.execution?.pages.flatMap(\.toolSteps) ?? []
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
        "parameters": requestPayload?.objectValue?["parameters"] ?? .object([:]),
        "windowForm": responsePayload?.objectValue?["windowForm"] ?? .object([:])
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
        workspaceSharePct: raw["workspaceSharePct"]?.intValue,
        workspaceMinHeight: raw["workspaceMinHeight"]?.intValue,
        inTab: true,
        parameters: raw["parameters"]?.objectValue,
        windowForm: raw["windowForm"]?.objectValue
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
    var candidates: [SDKJSONValue] = []
    if let raw {
        candidates.append(raw)
    }
    if let rawText,
       !rawText.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).isEmpty {
        candidates.append(.string(rawText))
    }
    for candidate in candidates {
        guard let parsed = parsePayload(candidate) else {
            continue
        }
        if isPayloadEnvelope(parsed) {
            continue
        }
        return parsed
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

private func isPayloadEnvelope(_ value: SDKJSONValue) -> Bool {
    guard let object = value.objectValue else {
        return false
    }
    let hasInlineBody = object["inlineBody"]?.stringValue != nil || object["InlineBody"]?.stringValue != nil
    let hasCompression = object["compression"]?.stringValue != nil || object["Compression"]?.stringValue != nil
    let hasDirectWorkspaceShape = object["items"] != nil || object["windowId"] != nil || object["focusedWindowId"] != nil
    return (hasInlineBody || hasCompression) && !hasDirectWorkspaceShape
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

    var intValue: Int? {
        switch self {
        case .number(let value):
            return Int(value)
        case .string(let value):
            return Int(value)
        default:
            return nil
        }
    }
}
