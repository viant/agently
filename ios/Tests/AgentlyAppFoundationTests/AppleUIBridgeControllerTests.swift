import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class AppleUIBridgeControllerTests: XCTestCase {
    func testFeedDraftBridgeReadsAndPatchesRenderedFeed() async throws {
        let runtime = ForgeRuntime()
        let metadata = WindowMetadata(dataSources: ["items": DataSourceDef()])
        let window = await runtime.openWindowInline(
            key: "feed-plan-conv-1",
            title: "Plan",
            metadata: metadata,
            conversationID: "conv-1",
            presentation: "inline"
        )
        await runtime.setDataSourceCollection(
            windowID: window.id,
            dataSourceRef: "items",
            rows: [["value": .number(1)], ["value": .number(2)]]
        )

        let read = try await handleAppleUIBridgeCommand(
            method: "ui.feed.get",
            params: [
                "conversationId": .string("conv-1"),
                "feedId": .string("plan"),
                "dataSourceRefs": .array([.string("items")])
            ],
            forgeRuntime: runtime,
            baseURL: "http://localhost"
        )
        XCTAssertNotNil(read["dataSources"]?.objectValue?["items"])

        let update = try await handleAppleUIBridgeCommand(
            method: "ui.feed.update",
            params: [
                "conversationId": .string("conv-1"),
                "feedId": .string("plan"),
                "operations": .array([.object([
                    "dataSourceRef": .string("items"),
                    "op": .string("replace"),
                    "path": .string("/collection/1/value"),
                    "value": .number(20)
                ])])
            ],
            forgeRuntime: runtime,
            baseURL: "http://localhost"
        )

        XCTAssertEqual(update["ok"], .bool(true))
        let rows = await runtime.dataSourceCollection(windowID: window.id, dataSourceRef: "items")
        XCTAssertEqual(rows[1]["value"], .number(20))
    }

    func testCanonicalFeedPatchesRecomputeParentsChildrenAndSelectionIdentity() async throws {
        let runtime = ForgeRuntime()
        let metadata = WindowMetadata(dataSources: [
            "root": DataSourceDef(),
            "record": DataSourceDef(),
            "editDraft": DataSourceDef(),
            "publishers": DataSourceDef(),
            "items": DataSourceDef(
                selectionMode: "multi",
                uniqueKey: [DataSourceUniqueKeyDef(field: "id")]
            )
        ])
        let window = await runtime.openWindowInline(
            key: "feed-plan-conv-1",
            title: "Plan",
            metadata: metadata,
            conversationID: "conv-1",
            presentation: "inline"
        )
        let initialRows: [[String: ForgeIOSRuntime.JSONValue]] = [
            ["id": .string("a"), "value": .number(1)],
            ["id": .string("b"), "value": .number(2)],
            ["id": .string("c"), "value": .number(3)]
        ]
        await runtime.setDataSourceCollection(windowID: window.id, dataSourceRef: "items", rows: initialRows)
        await runtime.setDataSourceSelectionState(
            windowID: window.id,
            dataSourceRef: "items",
            selection: SelectionState(selection: [initialRows[1]])
        )
        let definitions: AgentlySDK.JSONValue = .object([
            "root": .object(["source": .string("output")]),
            "record": .object([
                "dataSourceRef": .string("root"),
                "selectors": .object(["data": .string("record")])
            ]),
            "editDraft": .object([
                "dataSourceRef": .string("record"),
                "fields": .object([
                    "title": .string("title"),
                    "window": .object([
                        "transform": .string("dateRange"),
                        "startPath": .string("dates.start"),
                        "endPath": .string("dates.end")
                    ])
                ])
            ]),
            "items": .object([
                "dataSourceRef": .string("record"),
                "selectors": .object(["data": .string("items")]),
                "selectionMode": .string("multi"),
                "uniqueKey": .array([.object(["field": .string("id")])])
            ]),
            "publishers": .object([
                "dataSourceRef": .string("record"),
                "selectors": .object(["data": .string("channels")]),
                "flatten": .object(["sources": .array([.object([
                    "path": .string("publishers"),
                    "parentFields": .object(["Channel": .string("name")]),
                    "values": .object(["InventoryType": .string("Publisher")]),
                    "fields": .object(["Name": .string("name"), "Cost": .string("cost")])
                ])])]),
                "uniqueKey": .array([
                    .object(["field": .string("Channel")]),
                    .object(["field": .string("InventoryType")]),
                    .object(["field": .string("Name")])
                ])
            ])
        ])
        let payload = FeedDataResponse(
            feedID: "plan",
            data: .object([
                "output": .object([
                    "record": .object([
                        "title": .string("Original"),
                        "dates": .object([
                            "start": .object(["year": .number(2026), "month": .number(8), "day": .number(1)]),
                            "end": .object(["year": .number(2026), "month": .number(8), "day": .number(31)])
                        ]),
                        "channels": .array([.object([
                            "name": .string("CTV"),
                            "publishers": .array([.object(["name": .string("One"), "cost": .number(1)])])
                        ])]),
                        "items": .array(initialRows.map { .object($0.mapValues(\.appValue)) })
                    ])
                ])
            ]),
            dataSources: definitions
        )
        _ = await AppleFeedCanonicalRegistry.shared.register(
            forgeRuntime: runtime,
            windowID: window.id,
            payload: payload,
            turnID: "turn-1"
        )

        func operation(_ ref: String, _ op: String, _ path: String, _ value: AgentlySDK.JSONValue? = nil) -> AgentlySDK.JSONValue {
            var object: [String: AgentlySDK.JSONValue] = [
                "dataSourceRef": .string(ref), "op": .string(op), "path": .string(path)
            ]
            if let value { object["value"] = value }
            return .object(object)
        }
        func update(_ operations: [AgentlySDK.JSONValue]) async throws {
            _ = try await handleAppleUIBridgeCommand(
                method: "ui.feed.update",
                params: [
                    "conversationId": .string("conv-1"),
                    "turnId": .string("turn-1"),
                    "feedId": .string("plan"),
                    "operations": .array(operations)
                ],
                forgeRuntime: runtime,
                baseURL: "http://localhost"
            )
        }

        try await update([operation("items", "replace", "/collection/1/value", .number(20))])
        var rows = await runtime.dataSourceCollection(windowID: window.id, dataSourceRef: "items")
        XCTAssertEqual(rows.compactMap { $0["value"]?.intValue }, [1, 20, 3])
        var record = await runtime.formJSONValue(windowID: window.id, dataSourceRef: "record")
        XCTAssertEqual(record["items"]?.arrayValue?[1].objectValue?["value"], .number(20))

        try await update([operation("record", "replace", "/form/items/0/value", .number(10))])
        rows = await runtime.dataSourceCollection(windowID: window.id, dataSourceRef: "items")
        XCTAssertEqual(rows.compactMap { $0["value"]?.intValue }, [10, 20, 3])

        try await update([
            operation("items", "remove", "/collection/2"),
            operation("items", "add", "/collection/-", .object(["id": .string("d"), "value": .number(4)]))
        ])
        rows = await runtime.dataSourceCollection(windowID: window.id, dataSourceRef: "items")
        XCTAssertEqual(rows.compactMap { $0["id"]?.stringValue }, ["a", "b", "d"])

        try await update([operation("editDraft", "replace", "/form/window/start", .string("2026-08-05"))])
        record = await runtime.formJSONValue(windowID: window.id, dataSourceRef: "record")
        XCTAssertEqual(record["title"], .string("Original"))
        XCTAssertEqual(record["dates"]?.objectValue?["start"], .string("2026-08-05"))

        try await update([operation(
            "publishers",
            "add",
            "/collection/-",
            .object([
                "Channel": .string("CTV"),
                "InventoryType": .string("Publisher"),
                "Name": .string("Two"),
                "Cost": .number(2)
            ])
        )])
        record = await runtime.formJSONValue(windowID: window.id, dataSourceRef: "record")
        XCTAssertEqual(
            record["channels"]?.arrayValue?.first?.objectValue?["publishers"]?.arrayValue?.compactMap { $0.objectValue?["name"]?.stringValue },
            ["One", "Two"]
        )

        try await update([operation("items", "replace", "/selection/selection/0/value", .number(25))])
        rows = await runtime.dataSourceCollection(windowID: window.id, dataSourceRef: "items")
        XCTAssertEqual(rows[1]["value"], .number(25))
        let selection = await runtime.dataSourceSelectionState(windowID: window.id, dataSourceRef: "items")
        XCTAssertEqual(selection.selection.first?["value"], .number(25))
        record = await runtime.formJSONValue(windowID: window.id, dataSourceRef: "record")
        XCTAssertEqual(record["items"]?.arrayValue?.count, 3)

        let sameTurn = await AppleFeedCanonicalRegistry.shared.register(
            forgeRuntime: runtime,
            windowID: window.id,
            payload: payload,
            turnID: "turn-1"
        )
        XCTAssertEqual(
            toolFeedRows(data: sameTurn, dataSources: definitions, dataSourceRef: "items")[1]["value"],
            .number(25)
        )
        let nextTurn = await AppleFeedCanonicalRegistry.shared.register(
            forgeRuntime: runtime,
            windowID: window.id,
            payload: payload,
            turnID: "turn-2"
        )
        XCTAssertEqual(
            toolFeedRows(data: nextTurn, dataSources: definitions, dataSourceRef: "items").compactMap { $0["value"]?.intValue },
            [1, 2, 3]
        )
    }

    func testSnapshotDoesNotPublishHostedWindowsWithoutActiveConversation() async {
        let runtime = ForgeRuntime()
        _ = await runtime.openWindow(key: "reportBuilder", title: "Stale report")

        let snapshot = await buildAppleUIBridgeSnapshot(
            activeConversationID: nil,
            selectedWindowID: nil,
            forgeRuntime: runtime
        )

        XCTAssertTrue(snapshot.windows.isEmpty)
    }

    func testHandleSetFormDataMergesGenericWindowFormValues() async throws {
        let runtime = ForgeRuntime()
        let window = await runtime.openWindow(
            key: "generic/report",
            title: "Generic Report",
            parameters: [
                "prefill": .object([
                    "accountId": .number(7)
                ])
            ]
        )
        let result = try await handleAppleUIBridgeCommand(
            method: "ui.window.setFormData",
            params: [
                "windowId": .string(window.id),
                "values": .object([
                    "prefill": .object([
                        "recordId": .number(123)
                    ])
                ])
            ],
            forgeRuntime: runtime,
            baseURL: "http://localhost"
        )
        let windowForm = await runtime.windowFormJSONValue(windowID: window.id)
        let returnedPrefill = result["windowForm"]?.objectValue?["prefill"]?.objectValue

        XCTAssertEqual(result["ok"], .bool(true))
        XCTAssertEqual(result["windowId"], .string(window.id))
        XCTAssertEqual(windowForm["prefill"]?.objectValue?["accountId"], .number(7))
        XCTAssertEqual(windowForm["prefill"]?.objectValue?["recordId"], .number(123))
        XCTAssertEqual(returnedPrefill?["accountId"], .number(7))
        XCTAssertEqual(returnedPrefill?["recordId"], .number(123))
    }

    func testHandleSetFormDataPreservesForecastingPrefillContract() async throws {
        let runtime = ForgeRuntime()
        let openResult = try await handleAppleUIBridgeCommand(
            method: "ui.window.open",
            params: [
                "windowKey": .string("reportBuilder"),
                "windowTitle": .string("Forecasting"),
                "windowId": .string("forecastingCubeBuilder__conv-1"),
                "options": .object([
                    "conversationId": .string("conv-1"),
                    "presentation": .string("hosted"),
                    "region": .string("chat.top")
                ]),
                "parameters": .object([
                    "reportBuilderRef": .string("forecastingCubeBuilder")
                ])
            ],
            forgeRuntime: runtime,
            baseURL: "http://localhost"
        )

        XCTAssertEqual(openResult["ok"], .bool(true))
        XCTAssertEqual(openResult["windowId"], .string("forecastingCubeBuilder__conv-1"))

        let result = try await handleAppleUIBridgeCommand(
            method: "ui.window.setFormData",
            params: [
                "windowId": .string("forecastingCubeBuilder__conv-1"),
                "values": .object([
                    "prefill": .object([
                        "includeCountry": .array([.string("US")]),
                        "includeDealsPmp": .array([.number(90473), .number(90476)]),
                        "includePostalCodeList": .array([.number(70731)]),
                        "scope": .object([
                            "adOrderIds": .array([.number(2664518)]),
                            "audienceIds": .array([.number(7288336)]),
                            "targetKey": .string("audience:7288336")
                        ])
                    ])
                ])
            ],
            forgeRuntime: runtime,
            baseURL: "http://localhost"
        )

        XCTAssertEqual(result["ok"], .bool(true))
        let windowForm = await runtime.windowFormJSONValue(windowID: "forecastingCubeBuilder__conv-1")
        XCTAssertEqual(windowForm["reportBuilderRef"], .string("forecastingCubeBuilder"))
        let prefill = try XCTUnwrap(windowForm["prefill"]?.objectValue)
        XCTAssertEqual(prefill["includeCountry"], .array([.string("US")]))
        XCTAssertEqual(prefill["includeDealsPmp"], .array([.number(90473), .number(90476)]))
        XCTAssertEqual(prefill["includePostalCodeList"], .array([.number(70731)]))
        let scope = try XCTUnwrap(prefill["scope"]?.objectValue)
        XCTAssertEqual(scope["adOrderIds"], .array([.number(2664518)]))
        XCTAssertEqual(scope["audienceIds"], .array([.number(7288336)]))
        XCTAssertEqual(scope["targetKey"], .string("audience:7288336"))

        let returnedPrefill = result["windowForm"]?.objectValue?["prefill"]?.objectValue
        XCTAssertEqual(returnedPrefill?["includeCountry"], .array([.string("US")]))
        XCTAssertEqual(returnedPrefill?["includeDealsPmp"], .array([.number(90473), .number(90476)]))
        XCTAssertEqual(returnedPrefill?["includePostalCodeList"], .array([.number(70731)]))
    }

    func testReportCommandsUseExactNativeMaterializationIdentity() async throws {
        let runtime = ForgeRuntime()
        let window = await runtime.openWindow(
            key: "reportBuilder",
            title: "Report",
            parameters: [
                "reportDefinition": .object([
                    "id": .string("delivery_report"),
                    "documentPatch": .object([
                        "title": .string("Delivery report"),
                        "blocks": .array([.object([
                            "id": .string("spend"),
                            "kind": .string("kpiBlock"),
                            "datasetRef": .string("summary")
                        ])])
                    ])
                ])
            ]
        )
        await runtime.setWindowFormValue(
            windowID: window.id,
            values: [
                "reportDefinition": .object([
                    "id": .string("delivery_report"),
                    "documentPatch": .object([
                        "title": .string("Delivery report"),
                        "blocks": .array([.object([
                            "id": .string("spend"),
                            "kind": .string("kpiBlock"),
                            "datasetRef": .string("summary")
                        ])])
                    ])
                ])
            ],
            replace: false
        )

        let current = try await handleAppleUIBridgeCommand(
            method: "ui.report.getCurrent",
            params: ["windowId": .string(window.id)],
            forgeRuntime: runtime,
            baseURL: "http://localhost"
        )
        XCTAssertEqual(current["ok"], .bool(true))
        XCTAssertEqual(current["canRun"], .bool(true))
        XCTAssertEqual(current["hasCompletedRun"], .bool(false))

        let accepted = try await handleAppleUIBridgeCommand(
            method: "ui.report.run",
            params: ["windowId": .string(window.id)],
            forgeRuntime: runtime,
            baseURL: "http://localhost"
        )
        var requestID = ""
        for _ in 0..<100 where requestID.isEmpty {
            requestID = await runtime.windowFormJSONValue(windowID: window.id)["reportRunRequest"]?
                .objectValue?["id"]?.stringValue ?? ""
            if requestID.isEmpty {
                try await Task.sleep(nanoseconds: 10_000_000)
            }
        }
        XCTAssertFalse(requestID.isEmpty)
        await runtime.setWindowFormValue(
            windowID: window.id,
            values: [
                "reportMaterialization": .object([
                    "id": .string(requestID),
                    "requestId": .string(requestID),
                    "status": .string("completed"),
                    "materialized": .bool(true),
                    "datasetRefs": .array([.string("summary")]),
                    "rowCounts": .object(["summary": .number(1)])
                ])
            ],
            replace: false
        )
        XCTAssertEqual(accepted["ok"], .bool(true))
        XCTAssertEqual(accepted["accepted"], .bool(true))
        XCTAssertEqual(accepted["materialized"], .bool(false))
        XCTAssertEqual(accepted["materializationId"], .string(requestID))
        let completed = try await handleAppleUIBridgeCommand(
            method: "ui.report.getCurrent",
            params: ["windowId": .string(window.id)],
            forgeRuntime: runtime,
            baseURL: "http://localhost"
        )
        XCTAssertEqual(completed["hasCompletedRun"], .bool(true))
    }
}

private extension AgentlySDK.JSONValue {
    var objectValue: [String: AgentlySDK.JSONValue]? {
        guard case .object(let value) = self else { return nil }
        return value
    }
}

private extension ForgeIOSRuntime.JSONValue {
    var objectValue: [String: ForgeIOSRuntime.JSONValue]? {
        guard case .object(let value) = self else { return nil }
        return value
    }

    var stringValue: String? {
        guard case .string(let value) = self else { return nil }
        return value
    }
}
