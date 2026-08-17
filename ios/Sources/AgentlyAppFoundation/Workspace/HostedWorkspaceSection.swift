import SwiftUI
import AgentlySDK
import ForgeIOSRuntime
import ForgeIOSUI
import OSLog

private let hostedWorkspaceDidOpenNotification = Notification.Name("forgeHostedWorkspaceDidOpen")
private let hostedWorkspaceLogger = Logger(
    subsystem: "com.viant.agently.ios",
    category: "HostedWorkspace"
)

struct HostedWorkspaceSection: View {
    let restoreState: HostedWorkspaceRestoreState?
    let forgeRuntime: ForgeRuntime?
    let showTitle: Bool
    @Binding var displayMode: HostedWorkspaceDisplayMode

    init(
        restoreState: HostedWorkspaceRestoreState? = nil,
        forgeRuntime: ForgeRuntime?,
        showTitle: Bool = true,
        displayMode: Binding<HostedWorkspaceDisplayMode> = .constant(.standard)
    ) {
        self.restoreState = restoreState
        self.forgeRuntime = forgeRuntime
        self.showTitle = showTitle
        self._displayMode = displayMode
    }

    @ViewBuilder
    var body: some View {
        if let forgeRuntime,
           let effectiveRestoreState = restoreState {
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
    let restoreState: HostedWorkspaceRestoreState
    let forgeRuntime: ForgeRuntime
    let showTitle: Bool
    @Binding var displayMode: HostedWorkspaceDisplayMode

    @State private var selectedWindowID: String
    @State private var activeWindowSnapshot: WorkspaceWindowSnapshot?
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
        let initialWindow = restoreState.windows.first(where: { $0.windowId == restoreState.selectedWindowId })
            ?? restoreState.windows.last
        _selectedWindowID = State(initialValue: initialWindow?.windowId ?? "")
        _activeWindowSnapshot = State(initialValue: initialWindow)
    }

    private var selectedWindow: WorkspaceWindowSnapshot? {
        activeWindowSnapshot
            ?? restoreState.windows.first(where: { $0.windowId == selectedWindowID })
            ?? restoreState.windows.last
    }

    private var presentation: HostedWorkspacePresentation? {
        resolveHostedWorkspacePresentation(window: selectedWindow)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            if showTitle || presentation != nil {
                headerRow
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
        .modifier(HostedWorkspaceChromeModifier(applyChrome: true))
        .onChange(of: restoreSelectionKey) { _, _ in
            let restoredWindow = restoreState.windows.first(where: { $0.windowId == restoreState.selectedWindowId })
                ?? restoreState.windows.last
            selectedWindowID = restoredWindow?.windowId ?? ""
            activeWindowSnapshot = restoredWindow
            metadata = nil
            windowContext = nil
            errorMessage = nil
        }
        .onReceive(NotificationCenter.default.publisher(for: hostedWorkspaceDidOpenNotification)) { notification in
            let snapshot: WorkspaceWindowSnapshot?
            if let state = notification.userInfo?["state"] as? ForgeRuntime.WindowState {
                snapshot = workspaceWindowSnapshot(from: state)
            } else {
                snapshot = notification.userInfo?["snapshot"] as? WorkspaceWindowSnapshot
            }
            guard let snapshot else {
                return
            }
            Task { @MainActor in
                applyHostedWorkspaceUpdate(snapshot)
            }
        }
        .task(id: selectedWindowLoadKey) {
            guard let selectedWindow else { return }
            await load(selectedWindow)
        }
        .task(id: selectedWindowFormKey) {
            guard let selectedWindow,
                  await forgeRuntime.windows.contains(where: { $0.id == selectedWindow.windowId }) else {
                return
            }
            await applyRestoredWindowForm(selectedWindow)
        }
    }

    private var headerRow: some View {
        HStack(alignment: .top, spacing: 12) {
            VStack(alignment: .leading, spacing: showTitle ? 8 : 6) {
                if let presentation {
                    HStack(spacing: 8) {
                        Label(presentation.badgeLabel, systemImage: presentation.badgeSymbolName)
                            .font(.caption.weight(.semibold))
                            .padding(.horizontal, 10)
                            .padding(.vertical, 6)
                            .background(
                                hostedWorkspaceAccentColor().opacity(0.12),
                                in: Capsule()
                            )
                            .foregroundStyle(hostedWorkspaceAccentColor())
                    }

                    Text(presentation.title)
                        .font(showTitle ? .title3.weight(.semibold) : .headline.weight(.semibold))
                        .foregroundStyle(.primary)
                        .lineLimit(2)

                    if let subtitle = presentation.subtitle {
                        Text(subtitle)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                            .lineLimit(2)
                    }

                    if showTitle {
                        Text(presentation.supportingText)
                            .font(.caption)
                            .foregroundStyle(.tertiary)
                    }
                } else {
                    Text(selectedWindow?.windowTitle ?? selectedWindow?.windowKey ?? "Workspace")
                        .font(showTitle ? .title3.weight(.semibold) : .headline.weight(.semibold))
                        .foregroundStyle(.primary)
                        .lineLimit(2)
                }
            }
            Spacer(minLength: 0)
            if showTitle {
                controlButtons
            }
        }
    }

    private var controlButtons: some View {
        HStack(spacing: 6) {
            workspaceControlButton(
                systemName: "xmark",
                accessibilityLabel: "Close workspace"
            ) {
                displayMode = .closed
            }
            workspaceControlButton(
                systemName: displayMode == .minimized ? "rectangle" : "rectangle.compress.vertical",
                accessibilityLabel: displayMode == .minimized ? "Restore workspace" : "Minimize workspace"
            ) {
                displayMode = displayMode == .minimized ? .standard : .minimized
            }
            workspaceControlButton(
                systemName: displayMode == .expanded ? "arrow.down.right.and.arrow.up.left" : "arrow.up.left.and.arrow.down.right",
                accessibilityLabel: displayMode == .expanded ? "Restore workspace size" : "Expand workspace"
            ) {
                displayMode = displayMode == .expanded ? .standard : .expanded
            }
        }
    }

    private func workspaceControlButton(
        systemName: String,
        accessibilityLabel: String,
        action: @escaping () -> Void
    ) -> some View {
        Button(action: action) {
            Image(systemName: systemName)
                .font(.footnote.weight(.semibold))
                .frame(width: 30, height: 30)
                .background(Color.secondary.opacity(0.10), in: RoundedRectangle(cornerRadius: 10))
                .foregroundStyle(.primary)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(accessibilityLabel)
    }

    private var restoreSelectionKey: String {
        let selected = restoreState.selectedWindowId ?? ""
        let windows = restoreState.windows.map { window in
            [window.windowId, window.windowKey].joined(separator: "@")
        }.joined(separator: "|")
        return "\(selected)#\(windows)"
    }

    private var selectedWindowLoadKey: String {
        guard let selectedWindow else { return "" }
        return [
            selectedWindow.windowId,
            selectedWindow.windowKey,
            selectedWindow.windowTitle ?? "",
            hostedWorkspaceJSONSignature(selectedWindow.parameters)
        ].joined(separator: "#")
    }

    private var selectedWindowFormKey: String {
        guard let selectedWindow else { return "" }
        return "\(selectedWindow.windowId)#\(hostedWorkspaceJSONSignature(selectedWindow.windowForm))"
    }

    @MainActor
    private func applyHostedWorkspaceUpdate(_ snapshot: WorkspaceWindowSnapshot) {
        guard shouldApplyHostedWorkspaceUpdate(snapshot) else {
            return
        }
        selectedWindowID = snapshot.windowId
        activeWindowSnapshot = selectedWindow.map {
            mergeHostedWorkspaceRuntimeSnapshot(current: $0, incoming: snapshot)
        } ?? snapshot
        metadata = nil
        windowContext = nil
        errorMessage = nil
    }

    private func shouldApplyHostedWorkspaceUpdate(_ snapshot: WorkspaceWindowSnapshot) -> Bool {
        guard let current = selectedWindow else {
            return false
        }
        if snapshot.windowId == current.windowId {
            return true
        }
        let currentPresentation = normalizedHostedField(current.presentation)
        let snapshotPresentation = normalizedHostedField(snapshot.presentation)
        guard currentPresentation == "hosted", snapshotPresentation == "hosted" else {
            return false
        }
        let currentRegion = normalizedHostedField(current.region)
        let snapshotRegion = normalizedHostedField(snapshot.region)
        let currentConversationID = normalizedHostedField(current.conversationId)
        let snapshotConversationID = normalizedHostedField(snapshot.conversationId)
        guard !currentRegion.isEmpty,
              currentRegion == snapshotRegion,
              !currentConversationID.isEmpty,
              currentConversationID == snapshotConversationID else {
            return false
        }
        let currentParentKey = normalizedHostedField(current.parentKey)
        let snapshotParentKey = normalizedHostedField(snapshot.parentKey)
        return currentParentKey.isEmpty || snapshotParentKey.isEmpty || currentParentKey == snapshotParentKey
    }

    @MainActor
    private func load(_ selectedWindow: WorkspaceWindowSnapshot) async {
        activeWindowSnapshot = selectedWindow
        let snapshotMetadata = decodeHostedWorkspaceMetadata(selectedWindow.windowForm?["__agentlyWindowMetadata"])
        let state = await forgeRuntime.openWindow(
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
        if let liveMetadata = state.metadata,
           let snapshotMetadata,
           let mergedMetadata = mergeHostedWorkspaceMetadata(
                live: liveMetadata,
                snapshot: snapshotMetadata
           ) {
            _ = await forgeRuntime.updateWindowInline(
                id: state.id,
                title: state.title,
                metadata: mergedMetadata,
                resolveMetadata: false
            )
        }
        await applyRestoredWindowForm(selectedWindow)
        let restoredForm = await forgeRuntime.windowFormJSONValue(windowID: state.id)
        hostedWorkspaceLogger.info(
            "Restored window form entries=\(restoredForm.count, privacy: .public) authoredReport=\(hostedWorkspaceContainsAuthoredReport(restoredForm), privacy: .public)"
        )
        windowContext = await forgeRuntime.windowContext(id: state.id)
        try? await Task.sleep(for: .milliseconds(150))
        let latest = await forgeRuntime.windows.first(where: { $0.id == state.id })
        metadata = latest?.metadata
        errorMessage = metadata == nil ? "Workspace metadata did not load." : nil
    }

    @MainActor
    private func applyRestoredWindowForm(_ selectedWindow: WorkspaceWindowSnapshot) async {
        let restoredWindowForm = selectedWindow.windowForm?
            .filter { $0.key != "__agentlyWindowMetadata" }
            .mapValues(\.forgeValue)
        guard let windowForm = restoredWindowForm, !windowForm.isEmpty else { return }
        await forgeRuntime.setWindowFormValue(
            windowID: selectedWindow.windowId,
            values: windowForm,
            replace: true,
            bumpPrefillRevision: false
        )
    }

}

private func decodeHostedWorkspaceMetadata(_ value: AgentlySDK.JSONValue?) -> WindowMetadata? {
    guard let value,
          let data = try? JSONEncoder().encode(value.forgeValue) else {
        return nil
    }
    return try? JSONDecoder().decode(WindowMetadata.self, from: data)
}

private func mergeHostedWorkspaceMetadata(
    live: WindowMetadata,
    snapshot: WindowMetadata
) -> WindowMetadata? {
    let snapshotContainers = snapshot.view?.content?.containers ?? []
    let publishedContainer = snapshotContainers.first { container in
        let direct = container.dashboard?.reportBuilder?.dataSources ?? []
        let variants = container.dashboard?.reportBuilders.values.flatMap {
            $0.reportBuilder?.dataSources ?? []
        } ?? []
        return !direct.isEmpty || !variants.isEmpty
    }
    let builderRef = publishedContainer?.dashboard?.reportBuilderRef?
        .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let publishedSources = publishedContainer?.dashboard?.reportBuilder?.dataSources
        ?? publishedContainer?.dashboard?.reportBuilders[builderRef]?.reportBuilder?.dataSources
        ?? []
    guard !publishedSources.isEmpty,
          let liveData = try? JSONEncoder().encode(live),
          let sourcesData = try? JSONEncoder().encode(publishedSources),
          let liveValue = try? JSONDecoder().decode(ForgeIOSRuntime.JSONValue.self, from: liveData),
          let sourcesValue = try? JSONDecoder().decode(ForgeIOSRuntime.JSONValue.self, from: sourcesData),
          var liveObject = liveValue.objectValue,
          var view = liveObject["view"]?.objectValue,
          var content = view["content"]?.objectValue,
          var containers = content["containers"]?.arrayValue else {
        return nil
    }
    for index in containers.indices {
        guard var container = containers[index].objectValue else {
            continue
        }
        if var variants = container["reportBuilders"]?.objectValue,
           var variant = variants[builderRef]?.objectValue,
           var reportBuilder = variant["reportBuilder"]?.objectValue {
            reportBuilder["dataSources"] = sourcesValue
            variant["reportBuilder"] = .object(reportBuilder)
            variants[builderRef] = .object(variant)
            container["reportBuilders"] = .object(variants)
        }
        guard var dashboard = container["dashboard"]?.objectValue else {
            containers[index] = .object(container)
            continue
        }
        if var direct = dashboard["reportBuilder"]?.objectValue {
            direct["dataSources"] = sourcesValue
            dashboard["reportBuilder"] = .object(direct)
        }
        if var variants = dashboard["reportBuilders"]?.objectValue {
            let selectedRef = builderRef.isEmpty
                ? dashboard["reportBuilderRef"]?.stringValue ?? ""
                : builderRef
            if var variant = variants[selectedRef]?.objectValue,
               var reportBuilder = variant["reportBuilder"]?.objectValue {
                reportBuilder["dataSources"] = sourcesValue
                variant["reportBuilder"] = .object(reportBuilder)
                variants[selectedRef] = .object(variant)
                dashboard["reportBuilders"] = .object(variants)
            }
        }
        container["dashboard"] = .object(dashboard)
        containers[index] = .object(container)
    }
    content["containers"] = .array(containers)
    view["content"] = .object(content)
    liveObject["view"] = .object(view)
    guard let mergedData = try? JSONEncoder().encode(ForgeIOSRuntime.JSONValue.object(liveObject)) else {
        return nil
    }
    return try? JSONDecoder().decode(WindowMetadata.self, from: mergedData)
}

private func normalizedHostedField(_ value: String?) -> String {
    String(value ?? "")
        .trimmingCharacters(in: .whitespacesAndNewlines)
        .lowercased()
}

private func hostedWorkspaceJSONSignature(_ value: [String: AgentlySDK.JSONValue]?) -> String {
    guard let value else { return "" }
    return value.keys.sorted().map { key in
        "\(key)=\(hostedWorkspaceJSONSignature(value[key]))"
    }.joined(separator: "&")
}

private func hostedWorkspaceJSONSignature(_ value: AgentlySDK.JSONValue?) -> String {
    guard let value else { return "" }
    switch value {
    case .null:
        return "null"
    case .bool(let value):
        return value ? "true" : "false"
    case .number(let value):
        return String(value)
    case .string(let value):
        return value
    case .array(let values):
        return "[" + values.map { hostedWorkspaceJSONSignature($0) }.joined(separator: ",") + "]"
    case .object(let values):
        return "{" + hostedWorkspaceJSONSignature(values) + "}"
    }
}

private func hostedWorkspaceContainsAuthoredReport(_ form: [String: ForgeIOSRuntime.JSONValue]) -> Bool {
    if !(form["reportDocument"]?.objectValue?["blocks"]?.arrayValue ?? []).isEmpty ||
        !(form["reportDocumentBlocks"]?.arrayValue ?? []).isEmpty {
        return true
    }
    return form.values.compactMap(\.objectValue).contains { value in
        !(value["reportDocument"]?.objectValue?["blocks"]?.arrayValue ?? []).isEmpty ||
            !(value["reportDocumentBlocks"]?.arrayValue ?? []).isEmpty
    }
}

private func workspaceWindowSnapshot(from state: ForgeRuntime.WindowState) -> WorkspaceWindowSnapshot {
    WorkspaceWindowSnapshot(
        windowId: state.id,
        conversationId: state.conversationID,
        windowKey: state.key,
        windowTitle: state.title,
        presentation: state.presentation,
        region: state.region,
        parentKey: state.parentKey,
        workspaceSharePct: state.workspaceSharePct,
        workspaceMinHeight: state.workspaceMinHeight,
        inTab: state.inTab,
        parameters: state.parameters.mapValues(\.appValue),
        windowForm: nil
    )
}

func hostedWorkspaceAccentColor() -> Color {
    .accentColor
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
                .background(Color.agentlySystemBackground, in: RoundedRectangle(cornerRadius: 22))
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
