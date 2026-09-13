import Foundation
import Combine
import AgentlySDK

@MainActor
public final class WorkspaceThemeRuntime: ObservableObject {
    @Published public private(set) var catalog: WorkspaceThemeCatalog?
    @Published public private(set) var themeID = ""
    @Published public private(set) var modePreference = "system"
    @Published public private(set) var diagnostic: String?
    @Published public private(set) var revision = ""

    public var onRefresh: (() async -> Void)?

    private struct Scope: Codable, Equatable {
        var server: String
        var account: String
        var workspace: String
        var persistent: Bool
        var key: String { (try? JSONEncoder().encode([server, account, workspace]).base64EncodedString()) ?? "" }
    }
    private struct Preference: Codable { var themeID: String; var mode: String }
    private struct Cache: Codable { var revision: String; var data: Data }
    private let store: AppSettingsStore
    private var scope: Scope?
    private var generation = 0
    private var memoryPreference: Preference?

    public init(store: AppSettingsStore = AppSettingsStore()) { self.store = store }

    public var selectedTheme: WorkspaceTheme? { catalog?.themes.first { $0.id == themeID } }
    public func effectiveMode(systemMode: String) -> String {
        selectedTheme?.effectiveMode(preference: modePreference, systemMode: systemMode) ?? systemMode
    }
    public func tokens(systemMode: String) -> [String: WorkspaceThemeToken]? {
        selectedTheme?.modes[effectiveMode(systemMode: systemMode)]
    }
    private func serverKey(_ server: String) -> String { "server." + Data(server.utf8).base64EncodedString() }
    private func cacheKey(_ scope: Scope) -> String { "catalog." + scope.key }
    private func preferenceKey(_ scope: Scope) -> String { "preference." + scope.key }

    // Logout removes the endpoint pointer; cached catalog data alone never selects it.
    public func restore(server: String) {
        guard scope == nil,
              let data = store.loadWorkspaceThemeData(serverKey(server)),
              let saved = try? JSONDecoder().decode(Scope.self, from: data), saved.server == server,
              saved.persistent else { return }
        activate(saved)
    }
    private func activate(_ next: Scope) {
        guard scope != next else { return }
        clear()
        scope = next
        guard next.persistent,
              let raw = store.loadWorkspaceThemeData(cacheKey(next)),
              let cached = try? JSONDecoder().decode(Cache.self, from: raw),
              let decoded = try? WorkspaceThemeCatalog.load(cached.data) else { return }
        catalog = decoded; revision = cached.revision
        reconcilePreference()
    }
    private func reconcilePreference() {
        guard let catalog else { themeID = ""; modePreference = "system"; return }
        var preference = memoryPreference
        if let scope, scope.persistent, let raw = store.loadWorkspaceThemeData(preferenceKey(scope)) {
            preference = try? JSONDecoder().decode(Preference.self, from: raw)

        }
        if let preference, preference.themeID.isEmpty || catalog.themes.contains(where: { $0.id == preference.themeID }) {
            memoryPreference = preference
            themeID = preference.themeID
            modePreference = ["system", "light", "dark"].contains(preference.mode) ? preference.mode : catalog.defaultMode
        } else {
            memoryPreference = nil
            themeID = catalog.defaultTheme; modePreference = catalog.defaultMode
            if let scope, scope.persistent { store.saveWorkspaceThemeData(nil, key: preferenceKey(scope)) }
        }
    }
    public func select(themeID: String, mode: String) {
        guard themeID.isEmpty || catalog?.themes.contains(where: { $0.id == themeID }) == true,
              ["system", "light", "dark"].contains(mode) else { return }
        self.themeID = themeID; self.modePreference = mode
        memoryPreference = Preference(themeID: themeID, mode: mode)
        if let scope, scope.persistent {
            store.saveWorkspaceThemeData(try? JSONEncoder().encode(Preference(themeID: themeID, mode: mode)), key: preferenceKey(scope))
        }
    }
    public func refresh(metadata: WorkspaceMetadata, server: String, account: String,
                        fetch: @Sendable (WorkspaceAssetDescriptor) async throws -> Data) async {
        let workspace = metadata.workspaceId ?? ""
        activate(Scope(server: server, account: account, workspace: workspace.isEmpty ? (metadata.workspaceRoot ?? "session") : workspace,
                       persistent: !workspace.isEmpty && !account.isEmpty))
        generation += 1
        let request = generation
        diagnostic = metadata.uiStyleDiagnostics?.joined(separator: " · ")
        guard let asset = metadata.uiThemes else {
            memoryPreference = nil
            catalog = nil; revision = ""; themeID = ""; modePreference = "system"
            if let scope, scope.persistent {
                store.saveWorkspaceThemeData(nil, key: cacheKey(scope))
                store.saveWorkspaceThemeData(nil, key: preferenceKey(scope))
                store.saveWorkspaceThemeData(nil, key: serverKey(server))
            }
            return
        }
        guard asset.isThemeCatalog else {
            diagnostic = "Unsupported workspace theme catalog. Using the previous appearance."
            return
        }
        if asset.revision == revision && catalog != nil { return }
        do {
            let data = try await fetch(asset)
            let decoded = try WorkspaceThemeCatalog.load(data)
            guard request == generation else { return }
            catalog = decoded; revision = asset.revision
            reconcilePreference()
            if let scope, scope.persistent {
                store.saveWorkspaceThemeData(try JSONEncoder().encode(Cache(revision: asset.revision, data: data)), key: cacheKey(scope))
                store.saveWorkspaceThemeData(try JSONEncoder().encode(scope), key: serverKey(server))
            }
        } catch {
            guard request == generation else { return }
            reportRefreshFailure(error)
        }
    }
    public func reportRefreshFailure(_ error: Error) {
        if case AgentlySDKError.httpStatus(let status, _) = error, status == 401 || status == 403 {
            clear(forgetAccount: true)
        }
        diagnostic = catalog == nil ? "Workspace appearance is unavailable. Using the default appearance." : "Workspace appearance could not be refreshed. Using the cached theme."
    }
    public func clear(forgetAccount: Bool = false) {
        generation += 1
        if forgetAccount, let scope { store.saveWorkspaceThemeData(nil, key: serverKey(scope.server)) }
        memoryPreference = nil
        scope = nil; catalog = nil; themeID = ""; modePreference = "system"; revision = ""; diagnostic = nil
    }
}
