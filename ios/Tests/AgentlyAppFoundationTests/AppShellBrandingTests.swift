import XCTest
@testable import AgentlyAppFoundation

final class AppShellBrandingTests: XCTestCase {
    func testResolveWorkspaceBrandTitleFallsBackToAgently() {
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: nil), "Agently")
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: "   "), "Agently")
    }

    func testResolveWorkspaceBrandTitlePrefixesViantForWorkspaceName() {
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: "workspace"), "Viant Workspace")
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: "metrics_builder"), "Viant Metrics Builder")
    }

    func testResolveWorkspaceBrandTitleDoesNotDoublePrefixViant() {
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: "Viant Workspace"), "Viant Workspace")
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: "viant"), "Viant")
    }
}
