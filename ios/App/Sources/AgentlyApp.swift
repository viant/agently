import SwiftUI
import AgentlyAppFoundation
import AgentlySDK

@main
struct AgentlyApp: App {
    @StateObject private var runtime = AppBootstrap.makeRuntime()

    var body: some Scene {
        WindowGroup {
            AppContent(runtime: runtime)
                .task {
                    guard runtime.settingsRuntime.hasWorkspaceEndpointSelection else { return }
                    await runtime.bootstrap()
                }
                .onOpenURL { url in
                    Task {
                        if await runtime.authRuntime.handleOAuthCallback(url) {
                            await runtime.bootstrap()
                        }
                    }
                }
        }
    }
}

enum AppBootstrap {
    @MainActor
    static func makeRuntime() -> AppRuntime {
        let settings = AppSettingsStore()
        let startupBaseURL = resolvedBaseURLString(settings: settings, configuredBaseURL: settings.loadAPIBaseURL())
        let clientFactory: @Sendable (String) -> AgentlyClient = { configuredBaseURL in
            makeClient(settings: settings, configuredBaseURL: configuredBaseURL)
        }
        return AppRuntime(
            client: clientFactory(startupBaseURL),
            startupBaseURL: startupBaseURL,
            settingsStore: settings,
            clientFactory: clientFactory
        )
    }

    static func makeClient(
        settings: AppSettingsStore = AppSettingsStore(),
        configuredBaseURL: String? = nil
    ) -> AgentlyClient {
        let baseURLString = resolvedBaseURLString(
            settings: settings,
            configuredBaseURL: configuredBaseURL ?? settings.loadAPIBaseURL()
        )
        let baseURL = URL(string: baseURLString) ?? URL(string: defaultBaseURLString())!
        let configuration = URLSessionConfiguration.default
        configuration.timeoutIntervalForRequest = 300
        configuration.timeoutIntervalForResource = 300
        configuration.waitsForConnectivity = false
        if developerOverridesEnabled(), let proxy = resolvedHTTPProxy() {
            configuration.connectionProxyDictionary = [
                "HTTPEnable": 1,
                "HTTPProxy": proxy.host,
                "HTTPPort": proxy.port,
                "HTTPSEnable": 1,
                "HTTPSProxy": proxy.host,
                "HTTPSPort": proxy.port,
            ]
        }
        return AgentlyClient(
            endpoints: [
                "appAPI": EndpointConfig(baseURL: baseURL)
            ],
            session: URLSession(configuration: configuration),
            sessionCookieStore: AgentlyPersistentSessionCookieStore(
                namespace: sessionCookieNamespace(for: baseURL)
            )
        )
    }

    static func sessionCookieNamespace(for baseURL: URL) -> String {
        let host = baseURL.host?.lowercased() ?? "default"
        let port = baseURL.port.map { ":\($0)" } ?? ""
        let scheme = baseURL.scheme?.lowercased() ?? "http"
        return "\(scheme)://\(host)\(port)"
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: ":", with: "_")
    }

    static func resolvedBaseURLString(
        settings: AppSettingsStore = AppSettingsStore(),
        configuredBaseURL: String? = nil
    ) -> String {
        let storedValue = AppSettingsStore.normalizeAPIBaseURL(configuredBaseURL ?? settings.loadAPIBaseURL())
        let environmentValue = AppSettingsStore.normalizeAPIBaseURL(
            ProcessInfo.processInfo.environment["AGENTLY_API_BASE_URL"] ?? ""
        )
        let launchOverrideArgument = CommandLine.arguments.first { $0.hasPrefix("--apiBaseURL=") }
        let launchOverride = launchOverrideArgument
            .flatMap { $0.split(separator: "=", maxSplits: 1).last.map(String.init) }?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let launchValue = AppSettingsStore.normalizeAPIBaseURL(launchOverride)

        let developerOverridesEnabled = developerOverridesEnabled()

        let candidate: String
        if developerOverridesEnabled {
            if !environmentValue.isEmpty {
                candidate = environmentValue
            } else if !launchValue.isEmpty {
                candidate = launchValue
            } else {
                candidate = storedValue
            }
        } else {
            candidate = storedValue
        }
        if !candidate.isEmpty {
            return candidate
        }
        return defaultBaseURLString()
    }

    private static func defaultBaseURLString() -> String {
        let candidate = AppSettingsStore.normalizeAPIBaseURL(
            ProcessInfo.processInfo.environment["AGENTLY_API_BASE_URL"] ?? ""
        )
        if !candidate.isEmpty {
            return candidate
        }
        return "http://127.0.0.1:9292"
    }

    private static func developerOverridesEnabled() -> Bool {
        let normalized = ProcessInfo.processInfo.environment["AGENTLY_ENABLE_DEV_AUTH"]?
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased() ?? ""
        if ["1", "true", "yes", "on"].contains(normalized) {
            return true
        }
        return CommandLine.arguments.contains("--enableDevAuth=1")
            || CommandLine.arguments.contains("--enableDevAuth=true")
    }

    private static func resolvedHTTPProxy() -> (host: String, port: Int)? {
        let argument = CommandLine.arguments.first { $0.hasPrefix("--httpProxy=") }
        let raw = argument
            .flatMap { $0.split(separator: "=", maxSplits: 1).last.map(String.init) }
            ?? ProcessInfo.processInfo.environment["AGENTLY_IOS_HTTP_PROXY"]
            ?? ""
        guard let url = URL(string: raw), let host = url.host, let port = url.port else { return nil }
        return (host, port)
    }
}
