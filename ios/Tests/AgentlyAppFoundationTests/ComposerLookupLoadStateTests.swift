import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class ComposerLookupLoadStateTests: XCTestCase {
    func testSupersededCancellationCannotReplaceCurrentRowsOrLoadingState() {
        var state = ComposerLookupLoadState()
        let old = state.begin()
        let current = state.begin()
        state.complete(.failure(URLError(.cancelled)), generation: old)
        XCTAssertTrue(state.isLoading)
        XCTAssertNil(state.errorMessage)
        state.complete(.success([["campaignId": .number(563259)]]), generation: current)
        state.complete(.failure(CancellationError()), generation: old)
        XCTAssertEqual(state.rows.count, 1)
        XCTAssertFalse(state.isLoading)
        XCTAssertNil(state.errorMessage)
        state.complete(.success([["campaignId": .number(1)]]), generation: old)
        XCTAssertEqual(state.rows.first?["campaignId"], .number(563259))
    }

    func testExpectedCancellationIsSilentButGenuineNetworkFailureRemainsVisible() {
        var state = ComposerLookupLoadState()
        var request = state.begin()
        state.complete(.success([["id": .number(7)]]), generation: request)
        request = state.begin()
        state.complete(.failure(URLError(.cancelled)), generation: request)
        XCTAssertNil(state.errorMessage)
        XCTAssertEqual(state.rows.count, 1)
        request = state.begin()
        state.complete(.failure(URLError(.notConnectedToInternet)), generation: request)
        XCTAssertEqual(state.errorMessage, URLError(.notConnectedToInternet).localizedDescription)
        XCTAssertTrue(state.rows.isEmpty)
        XCTAssertFalse(state.isLoading)
    }

    func testCancelledTaskDoesNotPublishEvenIfTransportReturnsSuccess() {
        var state = ComposerLookupLoadState()
        let request = state.begin()
        state.complete(.success([["id": .number(7)]]), generation: request, taskCancelled: true)
        XCTAssertTrue(state.rows.isEmpty)
        XCTAssertNil(state.errorMessage)
    }

    func testSecondaryTextUsesDeclaredIdentityInsteadOfGuessingEntityFieldNames() throws {
        let entry = try JSONDecoder().decode(LookupRegistryEntry.self, from: Data(#"{"name":"campaign","dataSource":"lookup","token":{"store":"${campaignId}","display":"${campaignName}"}}"#.utf8))
        let row: [String: JSONValue] = ["campaignId": .number(563259), "campaignName": .string("Daybright Coffee"), "id": .number(999)]
        XCTAssertEqual(composerLookupRowSecondaryText(row: row, entry: entry), "563259")
        XCTAssertEqual(composerLookupRowSecondaryText(row: row), "999")
        XCTAssertEqual(composerLookupRowSecondaryText(row: ["id": .number(7), "groupName": .string("Group")], entry: entry), "Group • 7")
        XCTAssertNil(composerLookupRowSecondaryText(row: ["campaignId": .number(563259)]))
    }
}
