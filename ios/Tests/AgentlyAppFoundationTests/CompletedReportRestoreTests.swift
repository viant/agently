import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class CompletedReportRestoreTests: XCTestCase {
    func testRestoreProofIncludesOnlySelectedMetadataDeclaredEmptyDefaults() throws {
        var (form, run) = try fixture()
        let metadata = #"{"reportBuilders":{"other":{"reportBuilder":{"request":{"baseParameters":{"filters":{"foreign":""}}},"dataSources":[]}},"builder":{"reportBuilder":{"request":{"baseParameters":{"filters":{"status":"","nonempty":"behind"}}},"dataSources":[{"id":"summary","dataSourceRef":"cube","scope":{"mode":"inherit"},"request":{"limit":1,"filters":{}}}]}}}}"#
        form["__agentlyWindowMetadata"] = try JSONDecoder().decode(JSONValue.self, from: Data(metadata.utf8))
        let restored = try XCTUnwrap(restoredCompletedReportForm(form, run: run, conversationID: "conv"))
        XCTAssertEqual(restored["reportValidatedRestore"]?.objectValue?["inheritedEmptyDefaults"], .object(["status": .string("")]))
    }

    func testRestoresMatchingDataWithoutReplacingEditedLayoutOrDispatchingRequest() throws {
        let (form, run) = try fixture()
        let restored = try XCTUnwrap(restoredCompletedReportForm(form, run: run, conversationID: "conv"))
        XCTAssertEqual(restored["reportDefinition"], form["reportDefinition"])
        guard case .array(let datasets) = restored["reportStaticDatasets"] else { return XCTFail("Missing restored datasets") }
        XCTAssertEqual(datasets.count, 1)
        XCTAssertNil(restored["reportRunRequest"])
        XCTAssertEqual(restored["executeOnOpen"], .bool(false))
    }
    func testRejectsAnotherConversationChangedScopeAndDatasetRequest() throws {
        let (form, run) = try fixture()
        XCTAssertNil(restoredCompletedReportForm(form, run: run, conversationID: "other"))
        var changed = form
        changed["prefill"] = .object(["orderIds": .array([.number(99)])])
        XCTAssertNil(restoredCompletedReportForm(changed, run: run, conversationID: "conv"))
        let metadata = #"{"reportBuilders":{"builder":{"reportBuilder":{"dataSources":[{"id":"summary","dataSourceRef":"cube","scope":{"mode":"inherit"},"request":{"limit":2,"filters":{}}}]}}}}"#
        changed = form
        changed["__agentlyWindowMetadata"] = try JSONDecoder().decode(JSONValue.self, from: Data(metadata.utf8))
        XCTAssertNil(restoredCompletedReportForm(changed, run: run, conversationID: "conv"))
    }
    func testOnlyAbsentSnapshotRequestUsesMatchingLiveDeclaration() throws {
        let (form, run) = try fixture()
        let live = try XCTUnwrap(form["__agentlyWindowMetadata"])
        var sparse = form
        sparse["__agentlyWindowMetadata"] = try JSONDecoder().decode(JSONValue.self, from: Data(#"{"reportBuilders":{"builder":{"reportBuilder":{"dataSources":[{"id":"summary","dataSourceRef":"cube","scope":{"mode":"inherit"}}]}}}}"#.utf8))
        XCTAssertNotNil(restoredCompletedReportForm(sparse, run: run, conversationID: "conv", liveMetadata: live))
        sparse["__agentlyWindowMetadata"] = try JSONDecoder().decode(JSONValue.self, from: Data(#"{"reportBuilders":{"builder":{"reportBuilder":{"dataSources":[{"id":"summary","dataSourceRef":"cube","scope":{"mode":"inherit"},"request":{}}]}}}}"#.utf8))
        XCTAssertNil(restoredCompletedReportForm(sparse, run: run, conversationID: "conv", liveMetadata: live))
    }

    func testRestoresCompiledChartLabelsOnlyForMatchingUneditedChartSpec() throws {
        var (form, run) = try fixture()
        let chart: JSONValue = .object(["kind": .string("chartBlock"), "id": .string("chart"), "datasetRef": .string("summary"), "chartSpec": .object(["type": .string("line"), "xField": .string("date"), "yFields": .array([.string("spend")])])])
        form["reportDefinition"] = .object(["documentPatch": .object(["blocks": .array([chart])])])
        var encoded = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(run)) as? [String: Any])
        var spec = try XCTUnwrap(encoded["reportSpec"] as? [String: Any])
        let model: [String: Any] = ["type": "line", "yAxis": ["format": "currency"], "series": ["values": [["value": "spend", "label": "Spend"]]]]
        spec["blocks"] = [["kind": "chartBlock", "id": "chart", "datasetRef": "summary", "chartSpec": ["type": "line", "xField": "date", "yFields": ["spend"], "title": "Saved title"], "chartModel": model]]
        encoded["reportSpec"] = spec
        run = try JSONDecoder().decode(ReportRun.self, from: JSONSerialization.data(withJSONObject: encoded))
        let restored = try XCTUnwrap(restoredCompletedReportForm(form, run: run, conversationID: "conv"))
        let definition = try XCTUnwrap(restored["reportDefinition"]?.objectValue?["documentPatch"]?.objectValue)
        guard case .array(let blocks) = definition["blocks"] else { return XCTFail("Missing blocks") }
        XCTAssertNotNil(blocks.first?.objectValue?["chartModel"])
        var changedChart = try XCTUnwrap(chart.objectValue)
        changedChart["chartSpec"] = .object(["type": .string("bar"), "xField": .string("date"), "yFields": .array([.string("spend")])])
        form["reportDefinition"] = .object(["documentPatch": .object(["blocks": .array([.object(changedChart)])])])
        let changed = try XCTUnwrap(restoredCompletedReportForm(form, run: run, conversationID: "conv"))
        XCTAssertEqual(changed["reportDefinition"], form["reportDefinition"])
    }

    private func fixture() throws -> ([String: JSONValue], ReportRun) {
        let form = #"{"reportBuilderRef":"builder","prefill":{"orderIds":[7]},"executeOnOpen":true,"reportRunRequest":{"id":"old"},"reportDefinition":{"documentPatch":{"title":"Edited current layout","blocks":[{"id":"metric","datasetRef":"summary"}]}},"__agentlyWindowMetadata":{"reportBuilders":{"builder":{"reportBuilder":{"dataSources":[{"id":"summary","dataSourceRef":"cube","scope":{"mode":"inherit"},"request":{"limit":1,"filters":{}}}]}}}}}"#
        let run = #"{"reportRunId":"run","ownerId":"owner","conversationId":"conv","builderRef":"builder","status":"completed","revision":2,"effectiveParams":{"filters":{"orderIds":[7]}},"reportSpec":{"source":{"containerId":"reportBuilder"},"datasets":[{"id":"summary","dataSourceRef":"cube","request":{"limit":1,"filters":{"orderIds":[7]}}}]},"reportFill":{"source":{"containerId":"reportBuilder"},"datasets":[{"id":"summary","dataSourceRef":"cube","request":{"limit":1,"filters":{"orderIds":[7]}},"rows":[{"spend":309}]}]}}"#
        return (try JSONDecoder().decode([String: JSONValue].self, from: Data(form.utf8)), try JSONDecoder().decode(ReportRun.self, from: Data(run.utf8)))
    }
}
