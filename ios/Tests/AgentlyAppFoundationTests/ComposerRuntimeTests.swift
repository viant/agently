import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class ComposerRuntimeTests: XCTestCase {
    @MainActor
    func testRequestFocusAdvancesFocusRequestID() {
        let runtime = ComposerRuntime()

        XCTAssertEqual(runtime.focusRequestID, 0)

        runtime.requestFocus()
        XCTAssertEqual(runtime.focusRequestID, 1)

        runtime.requestFocus()
        XCTAssertEqual(runtime.focusRequestID, 2)
    }

    @MainActor
    func testLookupOccurrencesParseRegisteredSlashTokensOnly() async throws {
        let runtime = ComposerRuntime()
        try await configureLookupRuntime(runtime)

        runtime.query = "Troubleshoot /order and ignore /unknown"

        let occurrences = runtime.lookupOccurrences
        XCTAssertEqual(occurrences.count, 1)
        XCTAssertEqual(occurrences[0].key, "order#0")
        XCTAssertEqual(occurrences[0].title, "Order")
        XCTAssertEqual(occurrences[0].required, true)
        XCTAssertEqual(String(runtime.query[occurrences[0].displayRange]), "/order")
    }

    @MainActor
    func testResolvedQueryRequiresRequiredLookupSelection() async throws {
        let runtime = ComposerRuntime()
        try await configureLookupRuntime(runtime)
        runtime.query = "Troubleshoot /order"

        XCTAssertThrowsError(try runtime.resolvedQuery()) { error in
            guard case ComposerLookupError.unresolvedRequired("Order") = error else {
                XCTFail("unexpected error \(error)")
                return
            }
        }
    }

    @MainActor
    func testResolvedQueryUsesSelectedLookupToken() async throws {
        let runtime = ComposerRuntime()
        try await configureLookupRuntime(runtime)
        runtime.query = "Troubleshoot /order"
        let occurrence = try XCTUnwrap(runtime.lookupOccurrences.first)

        runtime.setLookupSelection(
            for: occurrence,
            row: [
                "id": .string("fixture-order-1"),
                "name": .string("Fixture Order")
            ]
        )

        XCTAssertEqual(runtime.selectionForLookup(occurrence)?.label, "Fixture Order")
        XCTAssertEqual(try runtime.resolvedQuery(), "Troubleshoot order fixture-order-1")
    }

    @MainActor
    func testComposerEditorProjectionHidesRawLookupTokenInsideInput() async throws {
        let runtime = ComposerRuntime()
        try await configureLookupRuntime(runtime)
        runtime.query = "Troubleshoot /order for delivery issues"

        let projection = ComposerEditorProjection(
            source: runtime.query,
            occurrences: runtime.lookupOccurrences
        )

        XCTAssertEqual(projection.display, "Troubleshoot for delivery issues")
        XCTAssertFalse(projection.display.contains("/order"))
        XCTAssertEqual(projection.sourceOffset(forDisplayOffset: 13), 20)
    }

    @MainActor
    func testChangingQueryPrunesStaleLookupSelections() async throws {
        let runtime = ComposerRuntime()
        try await configureLookupRuntime(runtime)
        runtime.query = "Troubleshoot /order"
        let occurrence = try XCTUnwrap(runtime.lookupOccurrences.first)
        runtime.setLookupSelection(
            for: occurrence,
            row: [
                "id": .string("fixture-order-1"),
                "name": .string("Fixture Order")
            ]
        )
        XCTAssertNotNil(runtime.selectionForLookup(occurrence))

        runtime.query = "Troubleshoot delivery"

        XCTAssertNil(runtime.selectionForLookup(occurrence))
        XCTAssertEqual(try runtime.resolvedQuery(), "Troubleshoot delivery")
    }

    func testLookupControlLabelMakesUnresolvedActionExplicit() {
        XCTAssertEqual(
            composerLookupControlLabel(title: "Order", selection: nil),
            "Select Order"
        )
        XCTAssertEqual(
            composerLookupControlLabel(
                title: "Order",
                selection: ComposerLookupSelection(token: #"@{order:fixture-order-1 "Fixture Order"}"#, label: "Fixture Order")
            ),
            "Fixture Order"
        )
    }

    func testComposerEditorHeightStartsCompactAndExpandsForPromptLength() {
        XCTAssertEqual(
            composerEditorHeight(query: "", density: .compact, horizontalSizeClass: .compact),
            54
        )
        XCTAssertEqual(
            composerEditorHeight(query: "", density: .regular, horizontalSizeClass: .regular),
            82
        )

        let longPrompt = """
        Troubleshoot ad order delivery issues and identify the primary causal blocker family such as setup, supply, bid competitiveness, or change pressure.
        """
        XCTAssertGreaterThan(
            composerEditorHeight(query: longPrompt, density: .compact, horizontalSizeClass: .compact),
            54
        )
        XCTAssertGreaterThan(
            composerEditorHeight(query: longPrompt, density: .regular, horizontalSizeClass: .regular),
            82
        )
    }

    @MainActor
    func testRecognizedTextIsInsertedAtCaretAndPreservesSuffix() {
        let runtime = ComposerRuntime()
        runtime.query = "Troubleshoot  and summarize delivery"

        let cursor = runtime.insertRecognizedText("order 2674628", atUTF16Offset: 13)

        XCTAssertEqual(runtime.query, "Troubleshoot order 2674628 and summarize delivery")
        XCTAssertEqual(cursor, 26)
    }

    @MainActor
    func testRecognizedTextPopulatesBlankComposer() {
        let runtime = ComposerRuntime()

        let cursor = runtime.insertRecognizedText(" troubleshoot order 2674628 ", atUTF16Offset: 0)

        XCTAssertEqual(runtime.query, "troubleshoot order 2674628")
        XCTAssertEqual(cursor, 26)
    }

    @MainActor
    func testLookupRowsLoaderReceivesTrimmedConversationID() async throws {
        let runtime = ComposerRuntime()
        let entry = try fixtureOrderLookupEntry()
        await runtime.configureLookupSupport(
            contextID: "fixture-agent",
            conversationID: " conv-1 ",
            registryLoader: { _, _ in [entry] },
            rowsLoader: { receivedEntry, searchQuery, conversationID in
                XCTAssertEqual(receivedEntry.name, "order")
                XCTAssertEqual(searchQuery, "fixture")
                XCTAssertEqual(conversationID, "conv-1")
                return [["id": .string("fixture-order-1")]]
            }
        )
        runtime.query = "Troubleshoot /order"
        let occurrence = try XCTUnwrap(runtime.lookupOccurrences.first)

        let rows = try await runtime.loadLookupRows(for: occurrence, query: "fixture")

        XCTAssertEqual(rows.first?["id"], .string("fixture-order-1"))
    }

    @MainActor
    private func configureLookupRuntime(_ runtime: ComposerRuntime) async throws {
        let entry = try fixtureOrderLookupEntry()
        await runtime.configureLookupSupport(
            contextID: "fixture-agent",
            registryLoader: { _, _ in [entry] },
            rowsLoader: nil
        )
    }

    private func fixtureOrderLookupEntry() throws -> LookupRegistryEntry {
        let data = """
        {
          "name": "order",
          "title": "Order",
          "dataSource": "order_lookup",
          "required": true,
          "token": {
            "store": "${id}",
            "display": "${name}",
            "modelForm": "order ${id}",
            "queryInput": "q"
          }
        }
        """.data(using: .utf8)!
        return try JSONDecoder().decode(LookupRegistryEntry.self, from: data)
    }
}
