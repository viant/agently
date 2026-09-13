import Foundation
import XCTest
@testable import AgentlyAppFoundation

final class WorkspaceThemeTests: XCTestCase {
    private func fixture() throws -> Data {
        var root = URL(fileURLWithPath: #filePath)
        for _ in 0..<5 { root.deleteLastPathComponent() }
        return try Data(contentsOf: root.appendingPathComponent("agently-core/protocol/ui/theme/testdata/baseline.json"))
    }
    func testSharedFixture() throws {
        let catalog = try WorkspaceThemeCatalog.load(fixture())
        let theme = try XCTUnwrap(catalog.themes.first)
        XCTAssertEqual(theme.modes["light"]?["control.minHeight"], .number(36))
        XCTAssertEqual(theme.modes["dark"]?["surface"], .text("#1b2230"))
        XCTAssertEqual(theme.effectiveMode(preference: "system", systemMode: "dark"), "dark")
        XCTAssertEqual(theme.effectiveMode(preference: "light", systemMode: "dark"), "light")
        XCTAssertEqual(theme.effectiveMode(preference: "unknown", systemMode: "dark"), "light")
    }
    func testRejectsInvalidCatalog() throws {
        let source = String(decoding: try fixture(), as: UTF8.self)
        for changed in [source.replacingOccurrences(of: "\"version\": 1", with: "\"version\": 2"),
                        source.replacingOccurrences(of: "\"control.minHeight\": 36", with: "\"control.minHeight\": \"36px\""),
                        source.replacingOccurrences(of: "#1b2230", with: "red; display:none")] {
            XCTAssertThrowsError(try WorkspaceThemeCatalog.load(Data(changed.utf8)))
        }
    }

    func testOptionalInputColorsRemainValidated() throws {
        var catalog = try XCTUnwrap(JSONSerialization.jsonObject(with: fixture()) as? [String: Any])
        var themes = try XCTUnwrap(catalog["themes"] as? [[String: Any]])
        var theme = themes[0]
        var modes = try XCTUnwrap(theme["modes"] as? [String: [String: Any]])
        modes["light"]?["lookup.background"] = "#f1f8f2"
        modes["light"]?["lookup.border"] = "#bfd7c4"
        modes["light"]?["required.background"] = "#fff3f4"
        modes["light"]?["required.border"] = "#d58f98"
        theme["modes"] = modes
        themes[0] = theme
        catalog["themes"] = themes
        let valid = try WorkspaceThemeCatalog.load(JSONSerialization.data(withJSONObject: catalog))
        XCTAssertEqual(valid.themes[0].modes["light"]?["required.border"], .text("#d58f98"))
        modes["light"]?["lookup.background"] = "not-a-color"
        theme["modes"] = modes
        themes[0] = theme
        catalog["themes"] = themes
        XCTAssertThrowsError(try WorkspaceThemeCatalog.load(JSONSerialization.data(withJSONObject: catalog)))
    }
}
