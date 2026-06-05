import SwiftUI
import AgentlySDK
import ForgeIOSRuntime
import ForgeIOSUI

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
        .modifier(HostedWorkspaceChromeModifier(applyChrome: true))
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
        ZStack {
            HStack {
                controlButtons
                Spacer(minLength: 0)
            }

            Text(selectedWindow?.windowTitle ?? selectedWindow?.windowKey ?? "Workspace")
                .font(.title3.weight(.semibold))
                .foregroundStyle(.primary)
                .lineLimit(1)
        }
    }

    private var controlRow: some View {
        HStack {
            controlButtons
            Spacer(minLength: 0)
        }
    }

    private var controlButtons: some View {
        HStack(spacing: 6) {
            workspaceControlDot(
                color: Color(red: 0.93, green: 0.36, blue: 0.31),
                accessibilityLabel: "Close workspace"
            ) {
                displayMode = .closed
            }
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
            .frame(width: 18, height: 18)
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

    @MainActor
    private func load(_ selectedWindow: WorkspaceWindowSnapshot) async {
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
