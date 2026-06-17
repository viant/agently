import Foundation

public struct WorkspaceEndpointOption: Identifiable, Hashable, Sendable {
    public let title: String
    public let subtitle: String
    public let value: String

    public var id: String { value }

    public init(title: String, subtitle: String, value: String) {
        self.title = title
        self.subtitle = subtitle
        self.value = AppSettingsStore.normalizeAPIBaseURL(value)
    }
}

@MainActor
public final class SettingsRuntime: ObservableObject {
    @Published public var apiBaseURL: String = ""
    @Published public var preferredAgentID: String = ""
    @Published public var oobSecretReference: String = ""
    @Published public private(set) var hasWorkspaceEndpointSelection: Bool = false

    private let store: AppSettingsStore

    public static let workspacePresets: [WorkspaceEndpointOption] = [
        WorkspaceEndpointOption(
            title: "Steward",
            subtitle: "Viant Steward workspace",
            value: "https://steward.agently.viantinc.com/"
        ),
        WorkspaceEndpointOption(
            title: "Localhost 9191",
            subtitle: "Local Agently server on this Mac",
            value: "http://localhost:9191"
        ),
        WorkspaceEndpointOption(
            title: "Loopback 9191",
            subtitle: "Local Agently server via loopback",
            value: "http://127.0.0.1:9191"
        )
    ]

    public static let localPresets: [(title: String, value: String)] = [
        ("Loopback 9191", "http://127.0.0.1:9191"),
        ("Localhost 9191", "http://localhost:9191")
    ]

    public init(store: AppSettingsStore = AppSettingsStore()) {
        self.store = store
        let resolvedBaseURL = resolvedBootstrapAPIBaseURL(
            storedValue: store.loadAPIBaseURL(),
            environmentValue: ProcessInfo.processInfo.environment["AGENTLY_API_BASE_URL"],
            launchArguments: CommandLine.arguments
        )
        self.apiBaseURL = resolvedBaseURL
        self.hasWorkspaceEndpointSelection = !resolvedBaseURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        self.preferredAgentID = store.loadPreferredAgentID()
        self.oobSecretReference = resolvedBootstrapOOBSecretReference(
            storedValue: store.loadOOBSecretReference(),
            environmentValue: ProcessInfo.processInfo.environment["AGENTLY_IOS_OOB_SECRET_REF"],
            launchArguments: CommandLine.arguments
        )
    }

    public func save() {
        apiBaseURL = normalizedAPIBaseURL
        store.saveAPIBaseURL(apiBaseURL)
        hasWorkspaceEndpointSelection = !apiBaseURL.isEmpty
        store.savePreferredAgentID(preferredAgentID.trimmingCharacters(in: .whitespacesAndNewlines))
        store.saveOOBSecretReference(oobSecretReference)
    }

    public var normalizedAPIBaseURL: String {
        AppSettingsStore.normalizeAPIBaseURL(apiBaseURL)
    }

    public func applyPreset(_ value: String) {
        apiBaseURL = AppSettingsStore.normalizeAPIBaseURL(value)
    }

    public func selectWorkspaceEndpoint(_ option: WorkspaceEndpointOption) {
        apiBaseURL = option.value
        save()
    }

    public func clearAPIBaseURL() {
        apiBaseURL = ""
        hasWorkspaceEndpointSelection = false
    }

    public var selectedWorkspacePreset: WorkspaceEndpointOption? {
        let normalized = normalizedAPIBaseURL
        return Self.workspacePresets.first { $0.value == normalized }
    }
}

internal func resolvedBootstrapOOBSecretReference(
    storedValue: String,
    environmentValue: String?,
    launchArguments: [String]
) -> String {
    let stored = storedValue.trimmingCharacters(in: .whitespacesAndNewlines)
    if !stored.isEmpty {
        return stored
    }
    let environmentOverride = environmentValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    if !environmentOverride.isEmpty {
        return environmentOverride
    }
    let launchOverrideArgument = launchArguments.first { $0.hasPrefix("--oobSecretReference=") }
    let launchOverride = launchOverrideArgument
        .flatMap { $0.split(separator: "=", maxSplits: 1).last.map(String.init) }?
        .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    return launchOverride
}

internal func resolvedBootstrapAPIBaseURL(
    storedValue: String,
    environmentValue: String?,
    launchArguments: [String]
) -> String {
    let normalizedStored = AppSettingsStore.normalizeAPIBaseURL(storedValue)
    let normalizedEnvironment = AppSettingsStore.normalizeAPIBaseURL(environmentValue ?? "")

    if developerAuthFeaturesEnabled(
        environmentValue: ProcessInfo.processInfo.environment["AGENTLY_ENABLE_DEV_AUTH"],
        launchArguments: launchArguments
    ) {
        if !normalizedEnvironment.isEmpty {
            return normalizedEnvironment
        }
        let launchOverrideArgument = launchArguments.first { $0.hasPrefix("--apiBaseURL=") }
        let launchOverride = launchOverrideArgument
            .flatMap { $0.split(separator: "=", maxSplits: 1).last.map(String.init) }?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let normalizedLaunch = AppSettingsStore.normalizeAPIBaseURL(launchOverride)
        if !normalizedLaunch.isEmpty {
            return normalizedLaunch
        }
    }

    return normalizedStored
}

internal func resolvedBootstrapAutoOOBSignIn(
    environmentValue: String?,
    launchArguments: [String]
) -> Bool {
    guard developerAuthFeaturesEnabled(
        environmentValue: ProcessInfo.processInfo.environment["AGENTLY_ENABLE_DEV_AUTH"],
        launchArguments: launchArguments
    ) else {
        return false
    }
    let normalized = environmentValue?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
    if ["1", "true", "yes", "on"].contains(normalized) {
        return true
    }
    return launchArguments.contains("--autoOOBSignIn=1")
        || launchArguments.contains("--autoOOBSignIn=true")
}

internal func developerAuthFeaturesEnabled() -> Bool {
    developerAuthFeaturesEnabled(
        environmentValue: ProcessInfo.processInfo.environment["AGENTLY_ENABLE_DEV_AUTH"],
        launchArguments: CommandLine.arguments
    )
}

internal func developerAuthFeaturesEnabled(
    environmentValue: String?,
    launchArguments: [String]
) -> Bool {
    let normalized = environmentValue?
        .trimmingCharacters(in: .whitespacesAndNewlines)
        .lowercased() ?? ""
    if ["1", "true", "yes", "on"].contains(normalized) {
        return true
    }
    return launchArguments.contains("--enableDevAuth=1")
        || launchArguments.contains("--enableDevAuth=true")
}
