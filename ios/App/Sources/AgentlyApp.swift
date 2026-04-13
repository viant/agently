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
        configuration.timeoutIntervalForRequest = 8
        configuration.timeoutIntervalForResource = 20
        configuration.waitsForConnectivity = false
        return AgentlyClient(
            endpoints: [
                "appAPI": EndpointConfig(baseURL: baseURL)
            ],
            session: URLSession(configuration: configuration)
        )
    }

    static func resolvedBaseURLString(
        settings: AppSettingsStore = AppSettingsStore(),
        configuredBaseURL: String? = nil
    ) -> String {
        let candidate = AppSettingsStore.normalizeAPIBaseURL(
            configuredBaseURL ?? settings.loadAPIBaseURL()
        )
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
        return "http://127.0.0.1:8080"
    }
}
