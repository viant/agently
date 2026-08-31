import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class ToolFeedsSectionTests: XCTestCase {
    func testToolFeedSynthesizesRemoteLookupDependenciesFromSharedUI() throws {
        let content = try JSONDecoder().decode(ContentDef.self, from: Data("""
        {"containers":[{"kind":"dashboard.lookupChips","lookup":{"dataSourceRef":"targeting_tree_lookup","drill":{"dataSourceRef":"deal_children"}}}]}
        """.utf8))

        let sources = toolFeedDataSources(declared: nil, content: content)

        XCTAssertEqual(Set(sources.keys), Set(["targeting_tree_lookup", "deal_children"]))
        XCTAssertEqual(sources["targeting_tree_lookup"]?.service?.endpoint, "agentlyAPI")
        XCTAssertEqual(sources["targeting_tree_lookup"]?.service?.uri, "/v1/api/datasources/targeting_tree_lookup/fetch")
        XCTAssertEqual(sources["targeting_tree_lookup"]?.autoFetch, false)
    }

    func testVisibleToolFeedsFiltersDeveloperOnlyAndSortsLabels() {
        let feeds = [
            ActiveFeedState(feedID: "terminal", title: "Terminal", developerOnly: true, itemCount: 2),
            ActiveFeedState(feedID: "plan", title: "Plan", itemCount: 3),
            ActiveFeedState(feedID: "changes", title: "Changes", itemCount: 1),
        ]

        XCTAssertEqual(visibleToolFeeds(feeds).compactMap(\.feedID), ["changes", "plan"])
    }

    func testToolFeedTargetSelectsPlacementAndInlineOwner() {
        let feeds = [
            ActiveFeedState(feedID: "auto", title: "Auto", itemCount: 1),
            ActiveFeedState(feedID: "inline", title: "Inline", presentation: FeedPresentation(target: "inline"), itemCount: 1, turnID: "turn-1"),
            ActiveFeedState(feedID: "workspace", title: "Workspace", presentation: FeedPresentation(target: "workspace"), itemCount: 1),
            ActiveFeedState(feedID: "detached", title: "Detached", presentation: FeedPresentation(target: "detached"), itemCount: 1),
            ActiveFeedState(feedID: "future", title: "Future", presentation: FeedPresentation(target: "future"), itemCount: 1)
        ]

        XCTAssertEqual(toolFeeds(feeds, for: .workspace).compactMap(\.feedID), ["auto", "future", "workspace"])
        XCTAssertEqual(toolFeeds(feeds, for: .detached).compactMap(\.feedID), ["detached"])
        XCTAssertEqual(inlineToolFeeds(feeds, turnID: "turn-1").compactMap(\.feedID), ["inline"])
        XCTAssertTrue(inlineToolFeeds(feeds, turnID: "turn-2").isEmpty)
        XCTAssertEqual(
            suppressedToolFeedReportIDs([
                ActiveFeedState(
                    feedID: "plan",
                    presentation: FeedPresentation(suppressReportIds: [" legacy-plan "])
                )
            ]),
            Set(["legacy-plan"])
        )
    }

    func testInlineFeedAttachesOnlyToFinalAssistantRowForOwningTurn() {
        let items = [
            ChatTranscriptEntry(id: "assistant-early", role: "assistant", markdown: "Working", turnID: "turn-feed"),
            ChatTranscriptEntry(id: "assistant-final", role: "assistant", markdown: "Done", turnID: "turn-feed"),
            ChatTranscriptEntry(id: "user-later", role: "user", markdown: "Unrelated", turnID: "turn-later")
        ]
        let feeds = [
            ActiveFeedState(
                feedID: "plan",
                title: "Plan",
                presentation: FeedPresentation(target: "inline"),
                itemCount: 1,
                turnID: "turn-feed"
            )
        ]

        XCTAssertEqual(inlineToolFeedAttachmentItemIDs(items: items, feeds: feeds), ["plan": "assistant-final"])
    }

    func testNativeFeedProjectionCoversFieldsFlattenExcludeAggregateDeriveAndNumericSelectors() {
        let definitions: AgentlySDK.JSONValue = .object([
            "root": .object(["source": .string("output")]),
            "plan": .object(["dataSourceRef": .string("root"), "selectors": .object(["data": .string("plan")])]),
            "overview": .object([
                "dataSourceRef": .string("plan"),
                "fields": .object([
                    "name": .string("name"),
                    "active": .object(["path": .string("active_flag"), "transform": .string("boolean")]),
                    "flight": .object(["transform": .string("dateRangeLabel"), "startPath": .string("dates.start"), "endPath": .string("dates.end")])
                ])
            ]),
            "publishers": .object([
                "dataSourceRef": .string("plan"), "selectors": .object(["data": .string("channels")]),
                "flatten": .object(["sources": .array([.object([
                    "path": .string("publishers"),
                    "exclude": .object(["field": .string("name"), "equals": .string("TOTAL")]),
                    "parentFields": .object(["channel": .string("name")]),
                    "values": .object(["kind": .string("Publisher")]),
                    "fields": .object(["publisher": .string("name"), "cost": .string("cost")])
                ])])]),
                "uniqueKey": .array([.object(["field": .string("channel")]), .object(["field": .string("publisher")])])
            ]),
            "coverage": .object(["dataSourceRef": .string("publishers"), "aggregate": .object(["countAs": .string("count")])]),
            "segments": .object([
                "dataSourceRef": .string("plan"), "selectors": .object(["data": .string("segments")]),
                "exclude": .object(["field": .string("name"), "equalsIgnoreCase": .string("TOTAL")]),
                "derive": .object(["label": .string("${id}:${name}")])
            ]),
            "secondCode": .object([
                "dataSourceRef": .string("plan"), "selectors": .object(["data": .string("codes[1]")]),
                "flatten": .object(["sources": .array([.object(["path": .string("$"), "fields": .object(["code": .string("$")])])])])
            ])
        ])
        let data: AgentlySDK.JSONValue = .object(["output": .object(["plan": .object([
            "name": .string("Plan A"), "active_flag": .string("true"),
            "dates": .object([
                "start": .object(["year": .number(2026), "month": .number(8), "day": .number(1)]),
                "end": .object(["year": .number(2026), "month": .number(8), "day": .number(31)])
            ]),
            "channels": .array([.object([
                "name": .string("CTV"),
                "publishers": .array([.object(["name": .string("One"), "cost": .number(3)]), .object(["name": .string("TOTAL"), "cost": .number(3)])])
            ])]),
            "segments": .array([.object(["id": .string("s1"), "name": .string("Sports")]), .object(["id": .string("total"), "name": .string("total")])]),
            "codes": .array([.string("CA"), .string("NY")])
        ])])])

        let overview = toolFeedRows(data: data, dataSources: definitions, dataSourceRef: "overview")
        XCTAssertEqual(overview.first?["name"], .string("Plan A"))
        XCTAssertEqual(overview.first?["active"], .bool(true))
        XCTAssertEqual(overview.first?["flight"], .string("2026-08-01 – 2026-08-31"))
        let publishers = toolFeedRows(data: data, dataSources: definitions, dataSourceRef: "publishers")
        XCTAssertEqual(publishers.count, 1)
        XCTAssertEqual(publishers.first?["publisher"], .string("One"))
        XCTAssertEqual(publishers.first?["channel"], .string("CTV"))
        XCTAssertEqual(toolFeedRows(data: data, dataSources: definitions, dataSourceRef: "coverage").first?["count"], .number(1))
        XCTAssertEqual(toolFeedRows(data: data, dataSources: definitions, dataSourceRef: "segments").first?["label"], .string("s1:Sports"))
        XCTAssertEqual(toolFeedRows(data: data, dataSources: definitions, dataSourceRef: "secondCode").first?["code"], .string("NY"))
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

    func testPhoneToolFeedLauncherDefaultsFromTurnActivityAndHonorsOverride() {
        XCTAssertFalse(toolFeedLauncherExpanded(collapsible: true, isTurnActive: false, userOverride: nil))
        XCTAssertTrue(toolFeedLauncherExpanded(collapsible: true, isTurnActive: true, userOverride: nil))
        XCTAssertFalse(toolFeedLauncherExpanded(collapsible: true, isTurnActive: true, userOverride: false))
        XCTAssertTrue(toolFeedLauncherExpanded(collapsible: true, isTurnActive: false, userOverride: true))
        XCTAssertTrue(toolFeedLauncherExpanded(collapsible: false, isTurnActive: false, userOverride: false))
    }
}
