#if DEBUG
import XCTest
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class NativeWorkspacePreviewTests: XCTestCase {
    func testPageSizeOverridePreservesTransportAndServerPaging() throws {
        let client: JSONValue = .object(["paginationMode": .string("client"), "paging": .object(["size": .number(10)]), "service": .object(["uri": .string("/fetch")])])
        let changed = previewClientPageSize(1, in: client)
        XCTAssertEqual(changed.objectValue?["paging"]?.objectValue?["size"], .number(1))
        XCTAssertEqual(changed.objectValue?["service"], client.objectValue?["service"])
        let server: JSONValue = .object(["paginationMode": .string("server"), "paging": .object(["size": .number(10)])])
        XCTAssertEqual(previewClientPageSize(1, in: server), server)
    }

    func testNestedSectionSelectionPreservesCompleteWindow() throws {
        let data = Data(#"{"id":"advertiserPhoneRoot","containers":[{"id":"binding","fetchData":true},{"id":"nav","tabs":{},"containers":[{"id":"measurement","tabs":{},"containers":[{"id":"pixels"},{"id":"capi"}]}]}]}"#.utf8)
        let original = try JSONDecoder().decode(JSONValue.self, from: data)
        let selected = previewSelectingSection("capi", in: original).objectValue!
        let children = selected["containers"]!.arrayValue!
        XCTAssertEqual(children.count, 2)
        XCTAssertEqual(children[0], original.objectValue!["containers"]!.arrayValue![0])
        let navigation = children[1].objectValue!
        XCTAssertEqual(navigation["tabs"]?.objectValue?["selectedTabId"], .string("measurement"))
        let nested = navigation["containers"]!.arrayValue![0].objectValue!
        XCTAssertEqual(nested["tabs"]?.objectValue?["selectedTabId"], .string("capi"))
        XCTAssertEqual(previewSelectingSection("missing", in: original), original)
    }
}
#endif
