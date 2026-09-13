import SwiftUI
import ForgeIOSUI

private struct WorkspaceThemeRuntimeKey: EnvironmentKey { static let defaultValue: WorkspaceThemeRuntime? = nil }
extension EnvironmentValues {
    public var workspaceThemeRuntime: WorkspaceThemeRuntime? {
        get { self[WorkspaceThemeRuntimeKey.self] }
        set { self[WorkspaceThemeRuntimeKey.self] = newValue }
    }
}

@MainActor
public struct WorkspaceThemePresentation: ViewModifier {
    @ObservedObject var runtime: WorkspaceThemeRuntime
    @Environment(\.colorScheme) private var systemScheme
    public init(runtime: WorkspaceThemeRuntime) { self.runtime = runtime }

    private var preferredScheme: ColorScheme? {
        guard let theme = runtime.selectedTheme else { return nil }
        if runtime.modePreference == "system", theme.modes["light"] != nil, theme.modes["dark"] != nil { return nil }
        return runtime.effectiveMode(systemMode: systemScheme == .dark ? "dark" : "light") == "dark" ? .dark : .light
    }
    public func body(content: Content) -> some View {
        let appearance = forgeThemeAppearance(runtime.tokens(systemMode: systemScheme == .dark ? "dark" : "light"))
        content
            .environment(\.workspaceThemeRuntime, runtime)
            .environment(\.forgeThemeAppearance, appearance)
            .preferredColorScheme(preferredScheme)
            .tint(appearance?.focus)
    }
}

func forgeThemeAppearance(_ tokens: [String: WorkspaceThemeToken]?) -> ForgeThemeAppearance? {
    guard let tokens else { return nil }
    func dimension(_ key: String) -> CGFloat {
        guard case .number(let value) = tokens[key] else { return 0 }
        return CGFloat(value)
    }
    func color(_ key: String) -> Color {
        guard case .text(let hex) = tokens[key], let value = UInt64(hex.dropFirst(), radix: 16) else { return .clear }
        let rgba = hex.count == 9 ? value : (value << 8) | 255
        return Color(.sRGB, red: Double((rgba >> 24) & 255) / 255,
                     green: Double((rgba >> 16) & 255) / 255,
                     blue: Double((rgba >> 8) & 255) / 255, opacity: Double(rgba & 255) / 255)
    }
    var appearance = ForgeThemeAppearance(surface: color("surface"), text: color("text"),
        controlBackground: color("control.background"), controlForeground: color("control.foreground"),
        controlBorder: color("control.border"), focus: color("focus.color"),
        buttonBackground: color("button.background"), buttonForeground: color("button.foreground"),
        disabledBackground: color("disabled.background"), disabledForeground: color("disabled.foreground"),
        validationBorder: color("validation.border"), fontSize: dimension("typography.size"),
        controlHeight: dimension("control.minHeight"), radius: dimension("control.radius"), paddingInline: dimension("control.paddingInline"))
    if tokens["lookup.background"] != nil { appearance.lookupBackground = color("lookup.background") }
    if tokens["lookup.border"] != nil { appearance.lookupBorder = color("lookup.border") }
    if tokens["required.background"] != nil { appearance.requiredBackground = color("required.background") }
    if tokens["required.border"] != nil { appearance.requiredBorder = color("required.border") }
    return appearance
}

struct WorkspaceThemeSettingsSection: View {
    @ObservedObject var runtime: WorkspaceThemeRuntime
    @Environment(\.colorScheme) private var systemScheme
    @State private var refreshing = false
    var body: some View {
        Section("Workspace Appearance") {
            if let catalog = runtime.catalog {
                Picker("Theme", selection: Binding(get: { runtime.themeID }, set: { runtime.select(themeID: $0, mode: runtime.modePreference) })) {
                    Text("Default").tag("")
                    ForEach(catalog.themes, id: \.id) { theme in Text(theme.label).tag(theme.id) }
                }
                Picker("Color mode", selection: Binding(get: { runtime.modePreference }, set: { runtime.select(themeID: runtime.themeID, mode: $0) })) {
                    Text("System").tag("system")
                    Text("Light").tag("light").disabled(runtime.selectedTheme?.modes["light"] == nil)
                    Text("Dark").tag("dark").disabled(runtime.selectedTheme?.modes["dark"] == nil)
                }.disabled(runtime.selectedTheme == nil)
                Text("Currently \(runtime.effectiveMode(systemMode: systemScheme == .dark ? "dark" : "light"))")
                    .font(.footnote).foregroundStyle(.secondary)
            } else {
                Text("No workspace theme is loaded.").foregroundStyle(.secondary)
            }
            if let diagnostic = runtime.diagnostic, !diagnostic.isEmpty { Text(diagnostic).font(.footnote).foregroundStyle(.secondary) }
            Button(refreshing ? "Refreshing…" : "Reload appearance") {
                Task { refreshing = true; await runtime.onRefresh?(); refreshing = false }
            }.disabled(refreshing || runtime.onRefresh == nil)
        }
    }
}
