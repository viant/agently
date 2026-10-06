import XCTest
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class ReportBuilderMetadataSummaryTests: XCTestCase {
    func testSelectedRawVariantCatalogMatchesWebShapeWithoutChangingDefinition() throws {
        let raw = try JSONDecoder().decode(ForgeIOSRuntime.JSONValue.self, from: Data(#"{"view":{"content":{"containers":[{"dashboard":{"reportBuilderRef":"other","reportBuilder":{"dataSources":[]},"reportBuilders":{"selected":{"label":"Performance","reportBuilder":{"opaque":{"preserve":true},"dataSources":[{"id":"active","dataSourceRef":"cube","label":"Active","scope":{"mode":"inherit"},"capabilities":{"preview":true},"request":{"filters":{"orderIds":[7]}},"columnOptions":[{"key":"spend","label":"Spend","kind":"measure","format":"currency"}],"chartFieldOptions":[{"key":"spend","label":"Duplicate"},{"value":"date","label":"Date","kind":"dimension"}],"scopeParamOptions":[{"key":"orderIds","paramPath":"filters.orderIds","multiple":true}]}]}}}}}]}}}"#.utf8))
        let original = raw
        let summary = try XCTUnwrap(nativeReportBuilderMetadataSummary(metadata: raw, form: ["reportBuilderRef": .string("selected")])?.objectValue)
        XCTAssertEqual(summary["builderRef"], .string("selected"))
        XCTAssertEqual(summary["label"], .string("Performance"))
        XCTAssertEqual(Set(summary.keys), Set(["builderRef", "label", "authoringContract", "dataSources"]))
        let source = try XCTUnwrap(summary["dataSources"]?.arrayValue?.first?.objectValue)
        XCTAssertEqual(source["id"], .string("active"))
        XCTAssertEqual(source["dataSourceRef"], .string("cube"))
        XCTAssertEqual(source["scope"], .object(["mode": .string("inherit")]))
        XCTAssertNil(source["request"], "The catalog is a declaration summary, not a fabricated prepared request")
        XCTAssertEqual(source["fields"]?.arrayValue?.count, 2)
        XCTAssertEqual(source["fields"]?.arrayValue?.first?.objectValue?["label"], .string("Spend"))
        XCTAssertEqual(source["scopeParams"]?.arrayValue?.first?.objectValue?["multiple"], .bool(true))
        XCTAssertEqual(raw, original)
    }
}
