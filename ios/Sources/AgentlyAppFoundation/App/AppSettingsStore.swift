import Foundation
import AgentlySDK

public final class AppSettingsStore {
    private let defaults: UserDefaults
    private let apiBaseURLKey = "agently.ios.settings.apiBaseURL"
    private let preferredAgentIDKey = "agently.ios.settings.preferredAgentID"
    private let activeConversationIDKey = "agently.ios.settings.activeConversationID"
    private let oobSecretReferenceKey = "agently.ios.settings.oobSecretReference"
    private let hostedWorkspaceRestoreStatePrefix = "agently.ios.settings.hostedWorkspaceRestoreState."

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

    public func loadOOBSecretReference() -> String {
        defaults.string(forKey: oobSecretReferenceKey) ?? ""
    }

    public func saveOOBSecretReference(_ value: String) {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty {
            defaults.removeObject(forKey: oobSecretReferenceKey)
        } else {
            defaults.set(trimmed, forKey: oobSecretReferenceKey)
        }
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

    public func loadHostedWorkspaceRestoreState(conversationID: String) -> HostedWorkspaceRestoreState? {
        let key = hostedWorkspaceRestoreStateKey(conversationID: conversationID)
        guard let data = defaults.data(forKey: key) else {
            return nil
        }
        return try? JSONDecoder.agently().decode(HostedWorkspaceRestoreState.self, from: data)
    }

    public func saveHostedWorkspaceRestoreState(
        _ value: HostedWorkspaceRestoreState?,
        conversationID: String
    ) {
        let key = hostedWorkspaceRestoreStateKey(conversationID: conversationID)
        let normalizedConversationID = conversationID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedConversationID.isEmpty else {
            defaults.removeObject(forKey: key)
            return
        }
        guard let value,
              !value.windows.isEmpty,
              let data = try? JSONEncoder.agently().encode(value) else {
            defaults.removeObject(forKey: key)
            return
        }
        defaults.set(data, forKey: key)
    }

    private func hostedWorkspaceRestoreStateKey(conversationID: String) -> String {
        hostedWorkspaceRestoreStatePrefix + conversationID.trimmingCharacters(in: .whitespacesAndNewlines)
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
