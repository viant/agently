import SwiftUI

/// Small native adoption surface for the shared contract. This does not change
/// the default app theme or introduce workspace networking/selection policy.
public struct WorkspaceThemeBaseline: View {
    private let theme: WorkspaceTheme
    private let mode: String
    @State private var text = ""
    @ScaledMetric private var fontScale = 1.0

    public init(theme: WorkspaceTheme, preference: String, systemMode: String) {
        self.theme = theme
        self.mode = theme.effectiveMode(preference: preference, systemMode: systemMode)
    }

    private var tokens: [String: WorkspaceThemeToken] { theme.modes[mode] ?? [:] }
    private func dimension(_ key: String, fallback: Double) -> Double {
        if case .number(let n) = tokens[key] { return n }
        return fallback
    }
    private func color(_ key: String, fallback: Color) -> Color {
        guard case .text(let hex) = tokens[key], let value = UInt64(hex.dropFirst(), radix: 16) else { return fallback }
        let rgba = hex.count == 9 ? value : (value << 8) | 255
        return Color(.sRGB, red: Double((rgba >> 24) & 255) / 255,
                     green: Double((rgba >> 16) & 255) / 255,
                     blue: Double((rgba >> 8) & 255) / 255, opacity: Double(rgba & 255) / 255)
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text(theme.label).font(.headline)
            Text("Customer name")
            TextField("Customer name", text: $text)
                .textFieldStyle(.plain)
                .padding(.horizontal, dimension("control.paddingInline", fallback: 10))
                .frame(minHeight: max(44, dimension("control.minHeight", fallback: 36)))
                .foregroundStyle(color("control.foreground", fallback: .primary))
                .background(color("control.background", fallback: .clear))
                .clipShape(RoundedRectangle(cornerRadius: dimension("control.radius", fallback: 4)))
                .overlay(RoundedRectangle(cornerRadius: dimension("control.radius", fallback: 4))
                    .stroke(color("control.border", fallback: .secondary)))
                .accessibilityIdentifier("workspace-theme-input")
            Button("Clear") { text = "" }
                .buttonStyle(.plain)
                .padding(.horizontal, dimension("control.paddingInline", fallback: 10))
                .frame(minHeight: max(44, dimension("control.minHeight", fallback: 36)))
                .foregroundStyle(color("button.foreground", fallback: .white))
                .background(color("button.background", fallback: .accentColor))
                .clipShape(RoundedRectangle(cornerRadius: dimension("control.radius", fallback: 4)))
                .accessibilityIdentifier("workspace-theme-button")
        }
        .font(.system(size: dimension("typography.size", fallback: 14) * fontScale))
        .padding()
        .foregroundStyle(color("text", fallback: .primary))
        .background(color("surface", fallback: .clear))
        .tint(color("focus.color", fallback: .accentColor))
        .environment(\.colorScheme, mode == "dark" ? .dark : .light)
    }
}
