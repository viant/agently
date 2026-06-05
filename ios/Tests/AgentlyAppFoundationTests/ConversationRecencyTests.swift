import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class ConversationRecencyTests: XCTestCase {
    func testParseConversationActivityDateHandlesGoMonotonicSuffix() {
        let raw = "2026-06-02 11:44:30.288943 -0700 PDT m=+9154.487875251"
        let parsed = parseConversationActivityDate(raw)

        XCTAssertNotNil(parsed)
    }

    func testSortedRecentConversationsUsesParsedActivityDate() {
        let older = Conversation(
            id: "older",
            title: "open workspace items",
            createdAt: "2026-06-02 11:42:01.751982 -0700 PDT m=+9005.950440334",
            lastActivity: "2026-06-02 11:44:30.288943 -0700 PDT m=+9154.487875251"
        )
        let newer = Conversation(
            id: "newer",
            title: "Open workspace items",
            createdAt: "2026-06-02 12:01:51.245322 -0700 PDT m=+10195.412337501",
            lastActivity: "2026-06-02 13:42:02.259476 -0700 PDT m=+16206.420227876"
        )

        let sorted = sortedRecentConversations([older, newer])

        XCTAssertEqual(sorted.first?.id, "newer")
    }

    func testMergeConversationIntoRecentListInjectsMissingActiveConversation() {
        let existing = Conversation(
            id: "existing",
            title: "Open workspace items",
            createdAt: "2026-06-02 13:31:23.337963 -0700 PDT m=+15567.496967543",
            lastActivity: "2026-06-02 13:32:09.609359 -0700 PDT m=+15613.768971084"
        )
        let missingActive = Conversation(
            id: "active",
            title: "Policy review request",
            createdAt: "2026-06-02 11:42:01.751982 -0700 PDT m=+9005.950440334",
            lastActivity: "2026-06-02 11:44:30.288943 -0700 PDT m=+9154.487875251"
        )

        let merged = mergeConversationIntoRecentList([existing], conversation: missingActive)

        XCTAssertEqual(merged.map(\.id), ["existing", "active"])
    }
}
