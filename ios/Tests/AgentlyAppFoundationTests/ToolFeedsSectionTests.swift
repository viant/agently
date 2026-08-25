import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class ToolFeedsSectionTests: XCTestCase {
    func testVisibleToolFeedsFiltersDeveloperOnlyAndSortsLabels() {
        let feeds = [
            ActiveFeedState(feedID: "terminal", title: "Terminal", developerOnly: true, itemCount: 2),
            ActiveFeedState(feedID: "plan", title: "Plan", itemCount: 3),
            ActiveFeedState(feedID: "changes", title: "Changes", itemCount: 1),
        ]

        XCTAssertEqual(visibleToolFeeds(feeds).compactMap(\.feedID), ["changes", "plan"])
    }

    func testToolFeedSummaryLinesExtractsUserFacingPlanAndCommandText() {
        let payload: AgentlySDK.JSONValue = .object([
            "output": .object([
                "rows": .array([
                    .object(["step": .string("Inspect changed files"), "status": .string("completed")]),
                    .object(["step": .string("Summarize findings"), "status": .string("running")]),
                ]),
            ]),
        ])

        XCTAssertEqual(toolFeedSummaryLines(payload), ["Inspect changed files", "Completed", "Summarize findings", "Running"])
    }

    func testToolFeedSummaryLinesExtractsResolvedPlanPayload() {
        let payload: AgentlySDK.JSONValue = .object([
            "input": .object([:]),
            "output": .object([
                "explanation": .string("Review complete; ready to report."),
                "plan": .array([
                    .object(["status": .string("completed"), "step": .string("Inspect changed files")]),
                    .object(["status": .string("in_progress"), "step": .string("Summarize findings")]),
                ]),
            ]),
        ])

        XCTAssertEqual(
            toolFeedSummaryLines(payload),
            ["Review complete; ready to report.", "Inspect changed files", "Completed", "Summarize findings", "In Progress"]
        )
    }

    func testToolFeedSummaryLinesExtractsGenericCommandInputAndOutput() {
        let payload: AgentlySDK.JSONValue = .object([
            "output": .object([
                "commands": .array([
                    .object(["input": .string("git status --short"), "output": .string("?? notes.md")]),
                    .object(["input": .string("git diff --stat")]),
                ]),
            ]),
        ])

        XCTAssertEqual(toolFeedSummaryLines(payload), ["$ git status --short", "?? notes.md", "$ git diff --stat"])
    }

    func testToolFeedMetadataDecodesTerminalAndResolvesDeclaredRows() {
        let ui: AgentlySDK.JSONValue = .object([
            "id": .string("commands"),
            "terminal": .object(["dataSourceRef": .string("commands"), "height": .string("240px")]),
        ])
        let sources: AgentlySDK.JSONValue = .object([
            "commands": .object(["source": .string("output.commands")]),
        ])
        let data: AgentlySDK.JSONValue = .object([
            "output": .object([
                "commands": .array([.object(["input": .string("pwd"), "output": .string("/tmp")])]),
            ]),
        ])

        let container = decodedToolFeedContainer(ui)
        XCTAssertEqual(container?.terminal?.dataSourceRef, "commands")
        let rows = toolFeedRows(data: data, dataSources: sources, dataSourceRef: "commands")
        XCTAssertEqual(rows.first?["input"]?.stringValue, "pwd")
        XCTAssertEqual(rows.first?["output"]?.stringValue, "/tmp")
    }

    func testToolFeedFindsNestedMetadataDrivenFilePreview() {
        let ui: AgentlySDK.JSONValue = .object([
            "containers": .array([
                .object([
                    "id": .string("changes"),
                    "dataSourceRef": .string("changes"),
                    "fileBrowser": .object([
                        "dedupeBy": .string("url"),
                        "preview": .object([
                            "kind": .string("codeDiff"),
                            "tool": .string("system_patch-preview"),
                            "defaultMode": .string("diff"),
                            "modes": .array([.string("diff"), .string("current"), .string("prev")]),
                        ]),
                    ]),
                ]),
            ]),
        ])
        let resolved = toolFeedFilePreview(in: decodedToolFeedContainer(ui))
        XCTAssertEqual(resolved?.container.dataSourceRef, "changes")
        XCTAssertEqual(resolved?.browser.dedupeBy, "url")
        XCTAssertEqual(resolved?.browser.preview?.defaultMode, "diff")
        XCTAssertEqual(resolved?.browser.preview?.tool, "system_patch-preview")

        let sources: AgentlySDK.JSONValue = .object([
            "snapshot": .object(["source": .string("output")]),
            "changes": .object([
                "dataSourceRef": .string("snapshot"),
                "selectors": .object(["data": .string("changes")]),
            ]),
        ])
        let data: AgentlySDK.JSONValue = .object([
            "output": .object(["changes": .array([.object(["url": .string("/tmp/a.txt")])])]),
        ])
        XCTAssertEqual(toolFeedRows(data: data, dataSources: sources, dataSourceRef: "changes").first?["url"]?.stringValue, "/tmp/a.txt")
    }

    func testToolFeedFindsGenericDeclaredToolAction() {
        let ui: AgentlySDK.JSONValue = .object([
            "containers": .array([.object([
                "toolbar": .object(["items": .array([.object([
                    "id": .string("apply"), "label": .string("Apply"), "intent": .string("primary"),
                    "on": .array([.object([
                        "event": .string("onClick"), "handler": .string("tool.execute"),
                        "target": .object(["name": .string("system_patch-commit")]),
                    ])]),
                ])])]),
            ])]),
        ])
        let actions = toolFeedActions(in: decodedToolFeedContainer(ui))
        XCTAssertEqual(actions.count, 1)
        XCTAssertEqual(actions.first?.item.intent, "primary")
        if case .object(let target) = actions.first?.execution.target {
            XCTAssertEqual(target["name"]?.stringValue, "system_patch-commit")
        } else { XCTFail("expected tool target") }
    }

    func testMergedToolFeedsKeepsPersistedRowsWhenLiveSnapshotIsEmptyAndLetsLiveWin() {
        let persisted = ActiveFeedState(feedID: "plan", title: "Persisted Plan", itemCount: 1)
        XCTAssertEqual(mergedToolFeeds(live: [], persisted: [persisted]).first?.title, "Persisted Plan")

        let live = ActiveFeedState(feedID: "plan", title: "Live Plan", itemCount: 2)
        let merged = mergedToolFeeds(live: [live], persisted: [persisted])
        XCTAssertEqual(merged.count, 1)
        XCTAssertEqual(merged.first?.title, "Live Plan")
        XCTAssertEqual(merged.first?.itemCount, 2)
    }

    func testToolFeedIconComesFromPresentationNotFeedID() {
        XCTAssertEqual(toolFeedSymbol(FeedPresentation(icon: "chart", accent: "blue")), "chart.bar")
        XCTAssertEqual(toolFeedSymbol(nil), "wrench.and.screwdriver")
    }
}
