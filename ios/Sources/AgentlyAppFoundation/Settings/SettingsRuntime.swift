import Foundation

@MainActor
public final class SettingsRuntime: ObservableObject {
    @Published public var apiBaseURL: String = ""
    @Published public var preferredAgentID: String = ""
    @Published public var oobSecretReference: String = ""

    private let store: AppSettingsStore

    public static let localPresets: [(title: String, value: String)] = [
        ("Steward 9191", "http://127.0.0.1:9191"),
        ("Localhost 9191", "http://localhost:9191")
    ]

    public init(store: AppSettingsStore = AppSettingsStore()) {
        self.store = store
        self.apiBaseURL = store.loadAPIBaseURL()
        self.preferredAgentID = store.loadPreferredAgentID()
        self.oobSecretReference = resolvedBootstrapOOBSecretReference(
            storedValue: store.loadOOBSecretReference(),
            environmentValue: ProcessInfo.processInfo.environment["AGENTLY_IOS_OOB_SECRET_REF"]
        )
    }

    public func save() {
        apiBaseURL = normalizedAPIBaseURL
        store.saveAPIBaseURL(apiBaseURL)
        store.savePreferredAgentID(preferredAgentID.trimmingCharacters(in: .whitespacesAndNewlines))
        store.saveOOBSecretReference(oobSecretReference)
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

internal func resolvedBootstrapOOBSecretReference(
    storedValue: String,
    environmentValue: String?
) -> String {
    guard developerAuthFeaturesEnabled() else {
        return storedValue.trimmingCharacters(in: .whitespacesAndNewlines)
    }
    let stored = storedValue.trimmingCharacters(in: .whitespacesAndNewlines)
    if !stored.isEmpty {
        return stored
    }
    return environmentValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
}

internal func developerAuthFeaturesEnabled() -> Bool {
#if DEBUG
    true
#else
    ProcessInfo.processInfo.environment["AGENTLY_ENABLE_DEV_AUTH"] == "1"
#endif
}
