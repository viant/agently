import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class ColdReportFormMergeTests: XCTestCase {
    private var document: JSONValue { .object(["title": .string("Report"), "blocks": .array([.object(["id": .string("kpi"), "datasetRef": .string("summary")])])]) }
    private var durable: [String: JSONValue] { ["reportBuilderRef": .string("builder"), "prefill": .object(["orderIds": .array([.number(7)])]), "reportDefinition": .object(["documentPatch": document])] }
    private var local: [String: JSONValue] {
        var form = durable
        form["reportBuilder:builder"] = .object(["activeDynamicFilterKeys": .array([.string("orderIds")]), "dynamicFilterValues": .object(["orderIds": .string("7")]), "opaque": .string("keep")])
        form["reportBuilder:sibling"] = .object(["opaque": .string("discard")])
        return form
    }
    func testSameSelectedDocumentRetainsMissingHookFieldsButNotSibling() throws {
        let merged = try XCTUnwrap(AppRuntime.mergeWindowForm(base: durable, overlay: local))
        XCTAssertEqual(merged["reportBuilder:builder"], local["reportBuilder:builder"])
        XCTAssertNil(merged["reportBuilder:sibling"])
    }
    func testDifferentDocumentDiscardsLocalBuilderState() throws {
        var changed = durable
        changed["reportDefinition"] = .object(["documentPatch": .object(["title": .string("New"), "blocks": .array([.object(["id": .string("other"), "datasetRef": .string("summary")])])])])
        let merged = try XCTUnwrap(AppRuntime.mergeWindowForm(base: changed, overlay: local))
        XCTAssertNil(merged["reportBuilder:builder"])
    }
    func testExplicitDurablePrefillAndOpaqueStateEditsAlwaysWin() throws {
        var edited = durable
        edited["prefill"] = .object(["orderIds": .array([.number(9)])])
        edited["reportBuilder:builder"] = .object(["opaque": .string("edited"), "dynamicFilterValues": .object([:]), "activeDynamicFilterKeys": .array([])])
        let merged = try XCTUnwrap(AppRuntime.mergeWindowForm(base: edited, overlay: local))
        XCTAssertEqual(merged["prefill"], edited["prefill"])
        XCTAssertEqual(merged["reportBuilder:builder"]?.objectValue?["opaque"], .string("edited"))
        XCTAssertEqual(merged["reportBuilder:builder"]?.objectValue?["dynamicFilterValues"], .object([:]))
        XCTAssertEqual(merged["reportBuilder:builder"]?.objectValue?["activeDynamicFilterKeys"], .array([]))
    }
}
