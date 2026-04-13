import Foundation

@MainActor
public final class SettingsRuntime: ObservableObject {
    @Published public var apiBaseURL: String = ""
    @Published public var preferredAgentID: String = ""

    private let store: AppSettingsStore

    public static let localPresets: [(title: String, value: String)] = [
        ("Local 8080", "http://127.0.0.1:8080"),
        ("Localhost 8080", "http://localhost:8080")
    ]

    public init(store: AppSettingsStore = AppSettingsStore()) {
        self.store = store
        self.apiBaseURL = store.loadAPIBaseURL()
        self.preferredAgentID = store.loadPreferredAgentID()
    }

    public func save() {
        apiBaseURL = normalizedAPIBaseURL
        store.saveAPIBaseURL(apiBaseURL)
        store.savePreferredAgentID(preferredAgentID.trimmingCharacters(in: .whitespacesAndNewlines))
    }

    public var normalizedAPIBaseURL: String {
        AppSettingsStore.normalizeAPIBaseURL(apiBaseURL)
    }

    public func applyPreset(_ value: String) {
        apiBaseURL = AppSettingsStore.normalizeAPIBaseURL(value)
    }

    public func clearAPIBaseURL() {
        apiBaseURL = ""
    }
}
