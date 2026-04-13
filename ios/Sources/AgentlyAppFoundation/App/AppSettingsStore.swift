import Foundation

public final class AppSettingsStore {
    private let defaults: UserDefaults
    private let apiBaseURLKey = "agently.ios.settings.apiBaseURL"
    private let preferredAgentIDKey = "agently.ios.settings.preferredAgentID"
    private let activeConversationIDKey = "agently.ios.settings.activeConversationID"

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    public func loadAPIBaseURL() -> String {
        defaults.string(forKey: apiBaseURLKey) ?? ""
    }

    public func saveAPIBaseURL(_ value: String) {
        let normalized = Self.normalizeAPIBaseURL(value)
        if normalized.isEmpty {
            defaults.removeObject(forKey: apiBaseURLKey)
        } else {
            defaults.set(normalized, forKey: apiBaseURLKey)
        }
    }

    public func loadPreferredAgentID() -> String {
        defaults.string(forKey: preferredAgentIDKey) ?? ""
    }

    public func savePreferredAgentID(_ value: String) {
        defaults.set(value, forKey: preferredAgentIDKey)
    }

    public func loadActiveConversationID() -> String {
        defaults.string(forKey: activeConversationIDKey) ?? ""
    }

    public func saveActiveConversationID(_ value: String?) {
        let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if trimmed.isEmpty {
            defaults.removeObject(forKey: activeConversationIDKey)
        } else {
            defaults.set(trimmed, forKey: activeConversationIDKey)
        }
    }

    public static func normalizeAPIBaseURL(_ value: String) -> String {
        var candidate = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !candidate.isEmpty else { return "" }
        if !candidate.contains("://") {
            candidate = "http://\(candidate)"
        }
        guard var components = URLComponents(string: candidate) else {
            return candidate.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        }
        let path = components.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        switch path.lowercased() {
        case "", ".":
            components.path = ""
        case "v1", "v1/api":
            components.path = ""
        default:
            components.path = "/" + path
        }
        components.query = nil
        components.fragment = nil
        let normalized = components.string ?? candidate
        return normalized.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
    }
}
