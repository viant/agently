import Foundation
import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

@MainActor
final class WorkspaceThemeRuntimeTests: XCTestCase {
    private func fixture() throws -> Data {
        var root = URL(fileURLWithPath: #filePath)
        for _ in 0..<5 { root.deleteLastPathComponent() }
        return try Data(contentsOf: root.appendingPathComponent("agently-core/protocol/ui/theme/testdata/baseline.json"))
    }
    private func store() -> AppSettingsStore {
        let name = "workspace-theme-tests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: name)!
        addTeardownBlock { defaults.removePersistentDomain(forName: name) }
        return AppSettingsStore(defaults: defaults)
    }
    private func metadata(_ char: Character = "a", workspace: String = "workspace") -> WorkspaceMetadata {
        let revision = String(repeating: String(char), count: 64)
        return WorkspaceMetadata(workspaceId: workspace, uiThemes: WorkspaceAssetDescriptor(revision: revision, href: "/v1/workspace/ui/themes/\(revision).json"))
    }
    func testSelectionSurvivesOfflineRestartAndLogoutClearsPointer() async throws {
        let store = store(), data = try fixture()
        let runtime = WorkspaceThemeRuntime(store: store)
        await runtime.refresh(metadata: metadata(), server: "https://one", account: "user") { _ in data }
        runtime.select(themeID: "baseline", mode: "dark")
        let restored = WorkspaceThemeRuntime(store: store)
        restored.restore(server: "https://one")
        XCTAssertEqual(restored.effectiveMode(systemMode: "light"), "dark")
        await restored.refresh(metadata: metadata("b"), server: "https://one", account: "user") { _ in throw URLError(.notConnectedToInternet) }
        XCTAssertEqual(restored.themeID, "baseline")
        XCTAssertTrue(restored.diagnostic?.contains("cached") == true)
        restored.clear(forgetAccount: true)
        let loggedOut = WorkspaceThemeRuntime(store: store)
        loggedOut.restore(server: "https://one")
        XCTAssertNil(loggedOut.catalog)
    }
    func testWorkspaceAndAccountChangesDoNotReuseAppearanceOnFailure() async throws {
        let runtime = WorkspaceThemeRuntime(store: store()), data = try fixture()
        await runtime.refresh(metadata: metadata(), server: "https://one", account: "user-a") { _ in data }
        runtime.select(themeID: "baseline", mode: "dark")
        await runtime.refresh(metadata: metadata(), server: "https://one", account: "user-b") { _ in throw URLError(.notConnectedToInternet) }
        XCTAssertNil(runtime.catalog)
        await runtime.refresh(metadata: metadata(workspace: "other"), server: "https://one", account: "user-a") { _ in throw URLError(.notConnectedToInternet) }
        XCTAssertNil(runtime.catalog)
    }
    func testRemovedThemeAndManifestResetPreference() async throws {
        let store = store(), data = try fixture()
        let runtime = WorkspaceThemeRuntime(store: store)
        await runtime.refresh(metadata: metadata(), server: "https://one", account: "user") { _ in data }
        runtime.select(themeID: "baseline", mode: "dark")
        let renamed = Data(String(decoding: data, as: UTF8.self).replacingOccurrences(of: "\"baseline\"", with: "\"renamed\"").utf8)
        await runtime.refresh(metadata: metadata("b"), server: "https://one", account: "user") { _ in renamed }
        XCTAssertEqual(runtime.themeID, "renamed")
        XCTAssertEqual(runtime.modePreference, "system")
        await runtime.refresh(metadata: WorkspaceMetadata(workspaceId: "workspace"), server: "https://one", account: "user") { _ in XCTFail("CSS-only workspace must not request a catalog"); return data }
        XCTAssertNil(runtime.catalog)
        let restored = WorkspaceThemeRuntime(store: store); restored.restore(server: "https://one")
        XCTAssertNil(restored.catalog)
    }
    func testSessionOnlyDefaultSelectionSurvivesRefresh() async throws {
        let runtime = WorkspaceThemeRuntime(store: store()), data = try fixture()
        await runtime.refresh(metadata: metadata(), server: "https://one", account: "") { _ in data }
        runtime.select(themeID: "", mode: "system")
        await runtime.refresh(metadata: metadata("b"), server: "https://one", account: "") { _ in data }
        XCTAssertEqual(runtime.themeID, "")
    }
    func testLateCatalogCannotRestoreClearedWorkspace() async throws {
        let runtime = WorkspaceThemeRuntime(store: store()), data = try fixture()
        let started = expectation(description: "fetch started")
        let gate = ThemeFetchGate()
        let metadata = metadata()
        let task = Task { await runtime.refresh(metadata: metadata, server: "https://one", account: "user") { _ in
            started.fulfill(); return await gate.wait()
        } }
        await fulfillment(of: [started], timeout: 2)
        runtime.clear()
        await gate.release(data)
        await task.value
        XCTAssertNil(runtime.catalog)
    }
}
private actor ThemeFetchGate {
    var value: Data?
    var continuation: CheckedContinuation<Data, Never>?
    func wait() async -> Data {
        if let value { return value }
        return await withCheckedContinuation { continuation = $0 }
    }
    func release(_ value: Data) { self.value = value; continuation?.resume(returning: value); continuation = nil }
}
