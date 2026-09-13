import Foundation

/// Portable catalog values. CSS is intentionally absent from this contract.
public enum WorkspaceThemeToken: Decodable, Equatable, Sendable {
    case text(String)
    case number(Double)

    public init(from decoder: Decoder) throws {
        let value = try decoder.singleValueContainer()
        if let text = try? value.decode(String.self) { self = .text(text) }
        else { self = .number(try value.decode(Double.self)) }
    }
}

public struct WorkspaceThemeCatalog: Decodable, Sendable {
    public let version: Int
    public let paletteVersion: Int
    public let defaultTheme: String
    public let defaultMode: String
    public let themes: [WorkspaceTheme]

    public static func load(_ data: Data) throws -> WorkspaceThemeCatalog {
        guard data.count <= 512 * 1024 else { throw WorkspaceThemeError.invalidCatalog }
        let catalog = try JSONDecoder().decode(Self.self, from: data)
        guard catalog.version == 1, catalog.paletteVersion == 1,
              ["light", "dark", "system"].contains(catalog.defaultMode),
              catalog.themes.count <= 32,
              Set(catalog.themes.map(\.id)).count == catalog.themes.count,
              catalog.themes.contains(where: { $0.id == catalog.defaultTheme }) else {
            throw WorkspaceThemeError.invalidCatalog
        }
        for theme in catalog.themes { try theme.validate() }
        return catalog
    }
}

public enum WorkspaceThemeError: Error { case invalidCatalog, invalidTokens }

public struct WorkspaceTheme: Decodable, Sendable {
    public let id: String
    public let label: String
    public let fallbackMode: String
    public let modes: [String: [String: WorkspaceThemeToken]]

    public func effectiveMode(preference: String, systemMode: String) -> String {
        let mode = preference == "system" ? systemMode : preference
        return modes[mode] == nil ? fallbackMode : mode
    }

    private static let dimensions: [String: ClosedRange<Double>] = [
        "typography.size": 8...72, "control.minHeight": 16...128,
        "control.radius": 0...64, "control.paddingInline": 0...64,
    ]
    private static let colors: Set<String> = [
        "surface", "text", "control.background", "control.foreground", "control.border",
        "focus.color", "button.background", "button.foreground", "disabled.background",
        "disabled.foreground", "validation.border",
    ]
    private static let optionalColors: Set<String> = [
        "lookup.background", "lookup.border", "required.background", "required.border",
    ]

    fileprivate func validate() throws {
        guard id.range(of: "^[a-z][a-z0-9-]{0,63}$", options: .regularExpression) != nil,
              id != "forge-default", !label.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              label.utf8.count <= 256, !modes.isEmpty, modes[fallbackMode] != nil,
              Set(modes.keys).isSubset(of: ["light", "dark"]) else {
            throw WorkspaceThemeError.invalidCatalog
        }
        let expected = Self.colors.union(Self.dimensions.keys).union(["typography.family"])
        for tokens in modes.values {
            let keys = Set(tokens.keys)
            guard expected.isSubset(of: keys), keys.isSubset(of: expected.union(Self.optionalColors)) else {
                throw WorkspaceThemeError.invalidTokens
            }
            for (key, token) in tokens {
                if let range = Self.dimensions[key] {
                    guard case .number(let n) = token, n.isFinite, range.contains(n) else {
                        throw WorkspaceThemeError.invalidTokens
                    }
                } else if key == "typography.family" {
                    guard token == .text("system") else { throw WorkspaceThemeError.invalidTokens }
                } else {
                    guard case .text(let color) = token,
                          color.range(of: "^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$", options: .regularExpression) != nil else {
                        throw WorkspaceThemeError.invalidTokens
                    }
                }
            }
        }
    }
}
