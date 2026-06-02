import XCTest
@testable import AgentlyAppFoundation

final class AppShellBrandingTests: XCTestCase {
    func testResolveWorkspaceBrandTitleFallsBackToAgently() {
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: nil), "Agently")
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: "   "), "Agently")
    }

    func testResolveWorkspaceBrandTitlePrefixesViantForWorkspaceName() {
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: "steward"), "Viant Steward")
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: "metrics_builder"), "Viant Metrics Builder")
    }

    func testResolveWorkspaceBrandTitleDoesNotDoublePrefixViant() {
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: "Viant Steward"), "Viant Steward")
        XCTAssertEqual(resolveWorkspaceBrandTitle(workspaceTitle: "viant"), "Viant")
    }
}
