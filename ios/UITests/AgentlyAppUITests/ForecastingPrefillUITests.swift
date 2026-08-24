import XCTest

final class ForecastingPrefillUITests: XCTestCase {
    func testPhysicalCausalEvidenceKeepsAuthoredTableGrid() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_PHYSICAL_REPORT_TESTS"] == "1"
                || ProcessInfo.processInfo.environment["AGENTLY_IOS_PHYSICAL_STABILITY_TESTS"] == "1",
            "Enable the physical iOS report test explicitly."
        )
        let conversationID = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_ACTIVE_CONVERSATION_ID"]
            ?? "8f5785be-143a-4ca7-a367-bf5629abc4af"
        let app = XCUIApplication()
        app.launchArguments = ["--activeConversationID=\(conversationID)"]
        app.launch()

        let openReport = app.buttons["agently-open-report"].firstMatch
        XCTAssertTrue(openReport.waitForExistence(timeout: 60), "Open report action did not appear")
        openReport.tap()

        let sectionSelector = app.descendants(matching: .any)["forge-report-runtime-section-selector"]
        XCTAssertTrue(sectionSelector.waitForExistence(timeout: 30), "Report section selector did not appear")
        let selectedSection = app.buttons["Overview"].firstMatch
        XCTAssertTrue(selectedSection.waitForExistence(timeout: 10), "Overview section menu did not appear")
        selectedSection.tap()
        let causalEvidence = app.buttons["Causal evidence"].firstMatch
        XCTAssertTrue(causalEvidence.waitForExistence(timeout: 10), "Causal evidence menu item did not appear")
        causalEvidence.tap()

        let incidentTable = app.descendants(matching: .any)["forge-report-runtime-table-causal_incident_table"]
        XCTAssertTrue(incidentTable.waitForExistence(timeout: 30), "Current-versus-previous authored table did not render")
        let screenshot = XCTAttachment(screenshot: app.screenshot())
        screenshot.name = "iOS Causal evidence authored table"
        screenshot.lifetime = .keepAlways
        add(screenshot)
    }

    func testPhysicalHistoryRefreshFindsLatestOrderConversation() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_PHYSICAL_STABILITY_TESTS"] == "1",
            "Enable the physical iOS stability test explicitly."
        )
        let query = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_HISTORY_QUERY"] ?? "101"
        let app = XCUIApplication()
        app.launch()
        XCTAssertTrue(returnToPhysicalHome(app), "Home did not appear")
        let history = app.buttons["agently-home-history"]
        XCTAssertTrue(history.waitForExistence(timeout: 30), "History action did not appear on Home")
        history.tap()

        let refresh = app.buttons["agently-history-refresh"]
        XCTAssertTrue(refresh.waitForExistence(timeout: 30), "History refresh action did not appear")
        if refresh.isEnabled { refresh.tap() }

        let search = app.textFields.firstMatch
        XCTAssertTrue(search.waitForExistence(timeout: 30), "History search field did not appear")
        search.tap()
        search.typeText(query)
        let matchingConversation = app.buttons
            .matching(NSPredicate(format: "label CONTAINS[c] %@", query))
            .firstMatch
        XCTAssertTrue(matchingConversation.waitForExistence(timeout: 45), "Latest conversation did not match History filter \(query)")

        let screenshot = XCTAttachment(screenshot: app.screenshot())
        screenshot.name = "Refreshed iOS History filtered by \(query)"
        screenshot.lifetime = .keepAlways
        add(screenshot)
    }

    func testPhysicalComposerRemainsStableWhileTypingSingleLine() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_PHYSICAL_STABILITY_TESTS"] == "1",
            "Enable the physical iOS stability test explicitly."
        )
        let app = XCUIApplication()
        app.launch()
        XCTAssertTrue(returnToPhysicalHome(app), "Home did not appear")
        let newChat = app.buttons["agently-new-chat"]
        XCTAssertTrue(newChat.waitForExistence(timeout: 30))
        newChat.tap()
        let expand = app.buttons["agently-composer-expand"]
        XCTAssertTrue(expand.waitForExistence(timeout: 20))
        expand.tap()
        let editor = app.textViews["agently-composer-editor"]
        XCTAssertTrue(editor.waitForExistence(timeout: 20))
        editor.tap()

        var verticalPositions: [CGFloat] = []
        for character in "stable composer input" {
            editor.typeText(String(character))
            verticalPositions.append(editor.frame.minY)
        }
        let spread = (verticalPositions.max() ?? 0) - (verticalPositions.min() ?? 0)
        XCTAssertLessThanOrEqual(spread, 2, "Composer moved vertically while typing a single rendered line")
    }

    func testPhysicalTroubleshootStarterOrderSelectionAndSendRemainStable() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_PHYSICAL_STABILITY_TESTS"] == "1",
            "Enable the physical iOS stability test explicitly."
        )
        let orderID = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_ORDER_ID"] ?? "2692101"
        let app = XCUIApplication()
        app.launch()
        XCTAssertTrue(returnToPhysicalHome(app), "Home did not appear")

        let starter = app.buttons["agently-starter-task-troubleshoot-ad-order"]
        XCTAssertTrue(starter.waitForExistence(timeout: 30), "Troubleshoot starter task did not appear")
        starter.tap()

        let search = app.searchFields.firstMatch
        let send = app.buttons["agently-composer-send"]
        if !search.waitForExistence(timeout: 8) {
            XCTAssertTrue(send.waitForExistence(timeout: 20), "Starter composer send action did not appear")
            send.tap()
        }
        XCTAssertTrue(search.waitForExistence(timeout: 30), "Order lookup search did not appear")
        search.tap()
        search.typeText(orderID)

        let orderRow = app.descendants(matching: .any)
            .matching(NSPredicate(format: "label CONTAINS[c] %@", orderID))
            .firstMatch
        XCTAssertTrue(orderRow.waitForExistence(timeout: 60), "Requested order did not appear")
        orderRow.tap()

        XCTAssertTrue(send.waitForExistence(timeout: 15), "Send action did not return after order selection")
        XCTAssertTrue(send.isEnabled, "Send action was disabled after order selection")
        send.tap()

        RunLoop.current.run(until: Date().addingTimeInterval(90))
        XCTAssertEqual(app.state, .runningForeground, "Agently terminated during troubleshooting response streaming")
        XCTAssertTrue(app.buttons["agently-home"].exists, "Home action disappeared during troubleshooting")
    }

    private func returnToPhysicalHome(_ app: XCUIApplication) -> Bool {
        let newChat = app.buttons["agently-new-chat"]
        if newChat.waitForExistence(timeout: 45) { return true }
        for _ in 0..<5 {
            let candidates = [
                app.buttons["agently-home"],
                app.buttons["BackButton"],
                app.buttons["agently-conversations-back"],
                app.buttons.matching(NSPredicate(format: "label ==[c] 'Conversations'")).firstMatch
            ]
            guard let action = candidates.first(where: { $0.exists && $0.isHittable }) else { break }
            action.tap()
            if newChat.waitForExistence(timeout: 8) { return true }
        }
        return newChat.exists
    }

    func testNormalAuthRequiredScreenIsQuietWithoutDevAuth() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_AUTH_SCREEN_LIVE_TESTS"] == "1"
                || ProcessInfo.processInfo.environment["AGENTLY_IOS_LIVE_UI_TESTS"] == "1",
            "Set AGENTLY_IOS_AUTH_SCREEN_LIVE_TESTS=1 or AGENTLY_IOS_LIVE_UI_TESTS=1 to run live local auth-screen verification."
        )

        let app = XCUIApplication()
        app.launch()

        let continueButton = app.buttons["workspace-selection-continue"]
        if continueButton.waitForExistence(timeout: 15) {
            continueButton.tap()
        }

        let authMessage = app.staticTexts["This workspace requires authorization."]
        XCTAssertTrue(authMessage.waitForExistence(timeout: 60), "Quiet required-auth message did not appear")
        XCTAssertTrue(app.buttons["Sign in"].waitForExistence(timeout: 5), "Sign in action did not appear")
        XCTAssertTrue(app.buttons["Workspace settings"].waitForExistence(timeout: 5), "Workspace settings action did not appear")

        let bannedLabels = [
            "Use developer session",
            "Hide developer session sign-in",
            "Session ID or token",
            "OOB",
            "Use saved",
            "Open workspace sign-in",
            "Developer OOB",
            "Developer Connection",
            "Sign-In Helpers",
            "OOB Secret Reference"
        ]
        for label in bannedLabels {
            XCTAssertFalse(app.staticTexts[label].exists, "Unexpected auth noise appeared: \(label)")
            XCTAssertFalse(app.buttons[label].exists, "Unexpected auth noise appeared: \(label)")
            XCTAssertFalse(app.textFields[label].exists, "Unexpected auth noise appeared: \(label)")
        }

        let screenshot = XCTAttachment(screenshot: app.screenshot())
        screenshot.name = "Quiet required auth screen"
        screenshot.lifetime = .keepAlways
        add(screenshot)
    }

    func testSignInActivatesSecureAuthenticationWindow() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_AUTH_SCREEN_LIVE_TESTS"] == "1"
                || ProcessInfo.processInfo.environment["AGENTLY_IOS_LIVE_UI_TESTS"] == "1",
            "Enable live iOS UI tests to run auth-window verification."
        )

        let baseURL = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_BASE_URL"]
            ?? "https://steward.agently.viantinc.com"
        let app = XCUIApplication()
        app.launchArguments = [
            "--enableDevAuth=1",
            "--apiBaseURL=\(baseURL)"
        ]
        app.launch()

        let signInButton = app.buttons["Sign in"]
        XCTAssertTrue(signInButton.waitForExistence(timeout: 30), "Sign in action did not appear")
        let enabled = NSPredicate(format: "isEnabled == true")
        expectation(for: enabled, evaluatedWith: signInButton)
        waitForExpectations(timeout: 10)
        signInButton.tap()

        let cancelButton = app.buttons["Cancel"]
        XCTAssertTrue(
            cancelButton.waitForExistence(timeout: 15),
            "Tapping Sign in did not activate the secure authentication window"
        )
        let screenshot = XCTAttachment(screenshot: app.screenshot())
        screenshot.name = "Secure authentication window"
        screenshot.lifetime = .keepAlways
        add(screenshot)
        cancelButton.tap()
    }

    func testOpenForecastBuilderPromptCanBeSentFromComposer() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_LIVE_UI_TESTS"] == "1",
            "Set AGENTLY_IOS_LIVE_UI_TESTS=1 to run live local Steward UI verification."
        )

        let baseURL = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_BASE_URL"] ?? "http://127.0.0.1:9292"
        let oobSecret = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_OOB_SECRET"]
            ?? ""
        try XCTSkipUnless(
            !oobSecret.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
            "Set AGENTLY_IOS_UI_TEST_OOB_SECRET to run live local OOB UI verification."
        )
        let activeConversationID = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_ACTIVE_CONVERSATION_ID"] ?? ""

        let app = XCUIApplication()
        let uiBridgeClientID = "ios-ui-test-\(UUID().uuidString)"
        app.launchArguments = [
            "--enableDevAuth=1",
            "--apiBaseURL=\(baseURL)",
            "--oobSecretReference=\(oobSecret)",
            "--autoOOBSignIn=1",
            "--uiBridgeClientID=\(uiBridgeClientID)"
        ]
        if !activeConversationID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            app.launchArguments.append("--activeConversationID=\(activeConversationID)")
        }
        app.launch()

        var newChatButton = app.buttons["agently-new-chat"]
        for _ in 0..<3 where !newChatButton.waitForExistence(timeout: 10) {
            let backButton = preferredConversationBackButton(in: app)
            if backButton.waitForExistence(timeout: 5) {
                backButton.tap()
                newChatButton = app.buttons["agently-new-chat"]
            } else {
                break
            }
        }
        XCTAssertTrue(newChatButton.waitForExistence(timeout: 60), "New Chat button did not appear")
        newChatButton.tap()

        let editor = app.textViews["agently-composer-editor"]
        XCTAssertTrue(editor.waitForExistence(timeout: 45), "Composer editor did not appear")

        let prompt = "open forecast builder for line 7288336"
        editor.tap()
        editor.typeText(prompt)

        let sendButton = app.buttons["agently-composer-send"]
        XCTAssertTrue(sendButton.waitForExistence(timeout: 5), "Composer send button did not appear")
        XCTAssertTrue(sendButton.isEnabled, "Composer send button should enable after typing")
        sendButton.tap()

        let composerConsumedPrompt = NSPredicate { _, _ in
            let value = editor.value as? String ?? ""
            return !value.localizedCaseInsensitiveContains(prompt)
        }
        expectation(for: composerConsumedPrompt, evaluatedWith: editor)
        waitForExpectations(timeout: 10)

        let forecastingTitle = app.staticTexts["Forecasting"].firstMatch
        XCTAssertTrue(forecastingTitle.waitForExistence(timeout: 300), "Forecasting pane did not open")
        XCTAssertTrue(
            waitForReportBuilderFilterBody(in: app),
            "Forecasting filter body did not render predicate-derived controls"
        )
        let screenshot = XCTAttachment(screenshot: app.screenshot())
        screenshot.name = "Forecasting predicate filters"
        screenshot.lifetime = .keepAlways
        add(screenshot)
        RunLoop.current.run(until: Date().addingTimeInterval(45))
    }

    func testPhoneShellCanReturnToConversationsAndHideKeyboard() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_LIVE_UI_TESTS"] == "1",
            "Set AGENTLY_IOS_LIVE_UI_TESTS=1 to run live local Steward UI verification."
        )

        let baseURL = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_BASE_URL"] ?? "http://127.0.0.1:9292"
        let oobSecret = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_OOB_SECRET"] ?? ""
        try XCTSkipUnless(
            !oobSecret.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
            "Set AGENTLY_IOS_UI_TEST_OOB_SECRET to run live local OOB UI verification."
        )

        let app = XCUIApplication()
        app.launchArguments = [
            "--enableDevAuth=1",
            "--apiBaseURL=\(baseURL)",
            "--oobSecretReference=\(oobSecret)",
            "--autoOOBSignIn=1",
            "--uiBridgeClientID=ios-ui-shell-\(UUID().uuidString)"
        ]
        app.launch()

        let continueButton = app.buttons["workspace-selection-continue"]
        if continueButton.waitForExistence(timeout: 15) {
            continueButton.tap()
        }

        var newChatButton = app.buttons["agently-new-chat"]
        if !newChatButton.waitForExistence(timeout: 10) {
            let backButton = preferredConversationBackButton(in: app)
            if backButton.waitForExistence(timeout: 5) {
                backButton.tap()
                newChatButton = app.buttons["agently-new-chat"]
            }
        }
        XCTAssertTrue(newChatButton.waitForExistence(timeout: 90), "New Chat button did not appear")
        XCTAssertTrue(app.navigationBars["Conversations"].waitForExistence(timeout: 10), "Back navigation did not return to Conversations")
        newChatButton.tap()

        let expandComposerButton = app.buttons["agently-composer-expand"]
        XCTAssertTrue(expandComposerButton.waitForExistence(timeout: 30), "Collapsed composer activation did not appear")
        expandComposerButton.tap()

        let editor = app.textViews["agently-composer-editor"]
        XCTAssertTrue(editor.waitForExistence(timeout: 30), "Composer editor did not appear")
        editor.tap()
        editor.typeText("hello")

        let hideKeyboardButton = app.buttons["agently-composer-hide-keyboard"]
        XCTAssertTrue(hideKeyboardButton.waitForExistence(timeout: 10), "Hide Keyboard action did not appear after focusing composer")
        hideKeyboardButton.tap()
        XCTAssertFalse(hideKeyboardButton.waitForExistence(timeout: 5), "Hide Keyboard action should disappear after keyboard dismissal")
        XCTAssertTrue(app.buttons["agently-new-chat"].waitForExistence(timeout: 10), "New chat action should remain reachable")

        let screenshot = XCTAttachment(screenshot: app.screenshot())
        screenshot.name = "Phone shell conversations and keyboard"
        screenshot.lifetime = .keepAlways
        add(screenshot)
    }

    func testLivePhoneEntityWorkspaceViewsMatchAndroid() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_LIVE_ENTITY_WORKSPACE_TESTS"] == "1"
                || ProcessInfo.processInfo.environment["AGENTLY_IOS_LIVE_UI_TESTS"] == "1",
            "Enable the live campaign/order/line workspace test explicitly."
        )
        let baseURL = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_BASE_URL"] ?? ""
        let oobSecret = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_OOB_SECRET"] ?? ""
        let lineID = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_LINE_ID"] ?? "7364938"
        let activeConversationID = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_ACTIVE_CONVERSATION_ID"] ?? ""
        try XCTSkipUnless(!baseURL.isEmpty && !oobSecret.isEmpty)

        let app = XCUIApplication()
        app.launchArguments = [
            "--enableDevAuth=1",
            "--apiBaseURL=\(baseURL)",
            "--oobSecretReference=\(oobSecret)",
            "--autoOOBSignIn=1",
            "--uiBridgeClientID=ios-ui-entity-workspace-\(UUID().uuidString)",
            "--initialWorkspaceWindowKey=line",
            "--initialWorkspaceWindowTitle=Line preview",
            "--initialWorkspaceWindowParametersJSON={\"AudienceId\":[\(lineID)]}"
        ]
        if !activeConversationID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            app.launchArguments.append("--activeConversationID=\(activeConversationID)")
        }
        app.launch()

        let continueButton = app.buttons["workspace-selection-continue"]
        if continueButton.waitForExistence(timeout: 15) {
            continueButton.tap()
        }

        let orderLink = app.buttons["↑ Order"]
        let campaignLink = app.buttons["↑ Campaign"]
        XCTAssertTrue(orderLink.waitForExistence(timeout: 60), "Line workspace did not expose its order parent")
        XCTAssertTrue(campaignLink.waitForExistence(timeout: 10), "Line workspace did not expose its campaign parent")
        XCTAssertTrue(app.staticTexts["Range"].exists, "Line workspace did not expose the compact range control")
        XCTAssertTrue(app.staticTexts["Resolution"].exists, "Line workspace did not expose the compact resolution control")
        XCTAssertTrue(app.buttons["Delivery"].exists, "Line workspace did not expose the Delivery tab")
        XCTAssertTrue(app.buttons["KPIs"].exists, "Line workspace did not expose the KPIs tab")
        XCTAssertTrue(app.buttons["Summary"].exists, "Line workspace did not expose the Summary tab")

        let rangeControl = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Range,")).firstMatch
        let resolutionControl = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Resolution,")).firstMatch
        XCTAssertTrue(rangeControl.exists && resolutionControl.exists, "Compact selector buttons did not render")
        XCTAssertLessThanOrEqual(
            abs(rangeControl.frame.minY - resolutionControl.frame.minY),
            4,
            "Range and Resolution should share one compact phone row"
        )

        let dailyResolution = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", "Daily")).firstMatch
        if dailyResolution.exists && dailyResolution.isHittable {
            dailyResolution.tap()
            let hourlyResolution = app.buttons["Hourly"]
            XCTAssertTrue(hourlyResolution.waitForExistence(timeout: 5), "Hourly resolution option did not appear")
            hourlyResolution.tap()
        }
        let lineScreenshot = XCTAttachment(screenshot: app.screenshot())
        lineScreenshot.name = "iOS phone line workspace"
        lineScreenshot.lifetime = .keepAlways
        add(lineScreenshot)

        if app.staticTexts["Unable to load chart data"].exists {
            throw XCTSkip("Steward datasource is unavailable; shell parity was verified but hierarchy navigation requires data.")
        }

        orderLink.tap()
        XCTAssertTrue(app.buttons["Lines"].waitForExistence(timeout: 90), "Order workspace did not expose child lines")
        XCTAssertTrue(app.buttons["Pacing"].exists, "Order workspace did not expose the Pacing tab")
        let orderScreenshot = XCTAttachment(screenshot: app.screenshot())
        orderScreenshot.name = "iOS phone order workspace"
        orderScreenshot.lifetime = .keepAlways
        add(orderScreenshot)

        let orderCampaignLink = app.buttons["↑ Campaign"]
        XCTAssertTrue(orderCampaignLink.waitForExistence(timeout: 30), "Order workspace did not expose its campaign parent")
        orderCampaignLink.tap()
        XCTAssertTrue(app.buttons["Orders"].waitForExistence(timeout: 90), "Campaign workspace did not expose child orders")
        XCTAssertTrue(app.buttons["Viewability"].exists, "Campaign workspace did not expose the Viewability tab")
        let campaignScreenshot = XCTAttachment(screenshot: app.screenshot())
        campaignScreenshot.name = "iOS phone campaign workspace"
        campaignScreenshot.lifetime = .keepAlways
        add(campaignScreenshot)
    }

    func testLiveAuthoredReportSectionsRenderOnPhone() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_LIVE_UI_TESTS"] == "1",
            "Set AGENTLY_IOS_LIVE_UI_TESTS=1 to run live Steward UI verification."
        )
        let baseURL = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_BASE_URL"] ?? ""
        let oobSecret = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_OOB_SECRET"] ?? ""
        let conversationID = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_ACTIVE_CONVERSATION_ID"] ?? ""
        try XCTSkipUnless(!baseURL.isEmpty && !oobSecret.isEmpty && !conversationID.isEmpty)

        let app = XCUIApplication()
        app.launchArguments = [
            "--enableDevAuth=1",
            "--apiBaseURL=\(baseURL)",
            "--oobSecretReference=\(oobSecret)",
            "--autoOOBSignIn=1",
            "--activeConversationID=\(conversationID)",
            "--uiBridgeClientID=ios-ui-report-\(UUID().uuidString)"
        ]
        app.launch()

        XCTAssertTrue(
            app.staticTexts["Order 2676237 Delivery Troubleshoot"].firstMatch.waitForExistence(timeout: 90),
            "Live authored report did not render"
        )
        XCTAssertTrue(app.buttons["agently-composer-expand"].waitForExistence(timeout: 10))
        XCTAssertFalse(app.buttons["Photos"].exists, "Collapsed one-line composer should not show media actions")
        XCTAssertFalse(app.buttons["Attach"].exists, "Collapsed one-line composer should not show attachment actions")
        let selector = app.descendants(matching: .any)["forge-report-runtime-section-selector"].firstMatch
        XCTAssertTrue(selector.waitForExistence(timeout: 30), "Compact report section selector did not render")
        selector.tap()
        let causalEvidence = app.descendants(matching: .any)
            .matching(NSPredicate(format: "label == %@", "Causal evidence"))
            .firstMatch
        XCTAssertTrue(causalEvidence.waitForExistence(timeout: 10), "Causal Evidence section was not offered")
        causalEvidence.tap()
        XCTAssertTrue(
            app.buttons.matching(
                NSPredicate(format: "identifier == %@ AND label CONTAINS[c] %@", "forge-report-runtime-section-selector", "Causal evidence")
            ).firstMatch.waitForExistence(timeout: 10),
            "Causal Evidence did not become the selected full report view"
        )

        let screenshot = XCTAttachment(screenshot: app.screenshot())
        screenshot.name = "Order 2676237 Causal Evidence iPhone"
        screenshot.lifetime = .keepAlways
        add(screenshot)
    }

    func testLivePerformanceReportCanSwitchBetweenChartAndTable() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_LIVE_UI_TESTS"] == "1",
            "Set AGENTLY_IOS_LIVE_UI_TESTS=1 to run live Steward UI verification."
        )
        let baseURL = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_BASE_URL"] ?? ""
        let oobSecret = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_OOB_SECRET"] ?? ""
        let conversationID = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_ACTIVE_CONVERSATION_ID"] ?? ""
        try XCTSkipUnless(!baseURL.isEmpty && !oobSecret.isEmpty && !conversationID.isEmpty)

        let app = XCUIApplication()
        app.launchArguments = [
            "--enableDevAuth=1",
            "--apiBaseURL=\(baseURL)",
            "--oobSecretReference=\(oobSecret)",
            "--autoOOBSignIn=1",
            "--activeConversationID=\(conversationID)",
            "--uiBridgeClientID=ios-ui-chart-table-\(UUID().uuidString)"
        ]
        app.launch()

        let authoredSectionSelector = app.descendants(matching: .any)["forge-report-runtime-section-selector"].firstMatch
        if authoredSectionSelector.waitForExistence(timeout: 30) {
            authoredSectionSelector.tap()
            let trendSection = app.descendants(matching: .any)
                .matching(NSPredicate(format: "label == %@", "Trend and bid funnel"))
                .firstMatch
            XCTAssertTrue(trendSection.waitForExistence(timeout: 10), "Trend and bid funnel section was not offered")
            trendSection.tap()
            XCTAssertTrue(
                app.descendants(matching: .any)["forge-report-runtime-chart-daily_spend_trend"].firstMatch
                    .waitForExistence(timeout: 30),
                "Authored daily spend chart did not render"
            )
            XCTAssertTrue(
                app.descendants(matching: .any)["forge-report-runtime-table-daily_delivery_table"].firstMatch
                    .waitForExistence(timeout: 30),
                "Authored daily delivery table did not render"
            )
            let screenshot = XCTAttachment(screenshot: app.screenshot())
            screenshot.name = "Performance report authored chart and table parity"
            screenshot.lifetime = .keepAlways
            add(screenshot)
            return
        }

        let modePicker = app.descendants(matching: .any)["forge-report-builder-view-mode"].firstMatch
        let hideFilters = app.buttons["Hide Filters"]
        if hideFilters.waitForExistence(timeout: 30) {
            hideFilters.tap()
        }
        if !modePicker.waitForExistence(timeout: 10) {
            let spendByDatePreset = app.buttons.matching(
                NSPredicate(format: "label CONTAINS[c] %@", "Overview · Spend by Date")
            ).firstMatch
            XCTAssertTrue(spendByDatePreset.waitForExistence(timeout: 30), "Default Spend by Date chart preset did not render")
            spendByDatePreset.tap()
        }
        XCTAssertTrue(modePicker.waitForExistence(timeout: 90), "Report view selector did not restore or apply")
        XCTAssertTrue(app.descendants(matching: .any)["forge-report-builder-chart"].firstMatch.waitForExistence(timeout: 30), "Chart did not render")

        let tableSegment = app.buttons["Table"]
        XCTAssertTrue(tableSegment.waitForExistence(timeout: 10), "Table segment did not render")
        tableSegment.tap()
        XCTAssertTrue(app.descendants(matching: .any)["forge-report-builder-table"].firstMatch.waitForExistence(timeout: 30), "Table did not render after switching")

        let chartSegment = app.buttons["Chart"]
        XCTAssertTrue(chartSegment.waitForExistence(timeout: 10), "Chart segment did not render")
        chartSegment.tap()
        XCTAssertTrue(app.descendants(matching: .any)["forge-report-builder-chart"].firstMatch.waitForExistence(timeout: 30), "Chart did not return after switching")

        let screenshot = XCTAttachment(screenshot: app.screenshot())
        screenshot.name = "Performance report chart and table parity"
        screenshot.lifetime = .keepAlways
        add(screenshot)
    }

    func testLiveAuthoredReportPDFShowsProgressAndOpensQuickLook() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_LIVE_UI_TESTS"] == "1",
            "Set AGENTLY_IOS_LIVE_UI_TESTS=1 to run live Steward UI verification."
        )
        let baseURL = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_BASE_URL"] ?? ""
        let oobSecret = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_OOB_SECRET"] ?? ""
        let conversationID = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_ACTIVE_CONVERSATION_ID"] ?? ""
        try XCTSkipUnless(!baseURL.isEmpty && !oobSecret.isEmpty && !conversationID.isEmpty)

        let app = XCUIApplication()
        app.launchArguments = [
            "--enableDevAuth=1",
            "--apiBaseURL=\(baseURL)",
            "--oobSecretReference=\(oobSecret)",
            "--autoOOBSignIn=1",
            "--activeConversationID=\(conversationID)",
            "--uiBridgeClientID=ios-ui-pdf-\(UUID().uuidString)"
        ]
        app.launch()

        let openPDF = app.buttons["forge-report-runtime-open-pdf"]
        XCTAssertTrue(openPDF.waitForExistence(timeout: 90), "Open PDF action did not render")
        openPDF.tap()
        let preparingPDF = NSPredicate(format: "label CONTAINS[c] %@", "Preparing PDF")
        expectation(for: preparingPDF, evaluatedWith: openPDF)
        waitForExpectations(timeout: 5)

        let deadline = Date().addingTimeInterval(180)
        var dismissPreview: XCUIElement?
        while Date() < deadline {
            // iOS 26 Quick Look exposes this as a lowercase `close` label and
            // a stable overlay accessibility identifier. Older releases use
            // Done or Close, so retain all variants.
            for candidate in [
                app.buttons["QLOverlayDoneButtonAccessibilityIdentifier"],
                app.buttons["close"],
                app.buttons["Done"],
                app.buttons["Close"]
            ] where candidate.exists {
                dismissPreview = candidate
                break
            }
            if dismissPreview != nil { break }
            RunLoop.current.run(until: Date().addingTimeInterval(1))
        }
        XCTAssertNotNil(dismissPreview, "Go-backed PDF export did not open Quick Look")

        let screenshot = XCTAttachment(screenshot: app.screenshot())
        screenshot.name = "Go-backed report PDF Quick Look"
        screenshot.lifetime = .keepAlways
        add(screenshot)
        dismissPreview?.tap()
    }

    private func waitForReportBuilderFilterBody(in app: XCUIApplication) -> Bool {
        let requiredIdentifiers = [
            "forge-report-builder-filter-summary",
            "forge-report-builder-static-filter-dateRange",
            "forge-report-builder-dynamic-filters",
            "forge-report-builder-dynamic-family-inventory",
            "forge-report-builder-add-line-inventory"
        ]
        let deadline = Date().addingTimeInterval(120)
        var observedIdentifiers = Set<String>()
        while Date() < deadline {
            observedIdentifiers.formUnion(app.visibleReportBuilderIdentifiers(matching: requiredIdentifiers))
            if requiredIdentifiers.allSatisfy({ observedIdentifiers.contains($0) }) {
                return true
            }
            app.swipeUp()
            RunLoop.current.run(until: Date().addingTimeInterval(1))
        }
        app.swipeDown()
        RunLoop.current.run(until: Date().addingTimeInterval(1))
        observedIdentifiers.formUnion(app.visibleReportBuilderIdentifiers(matching: requiredIdentifiers))
        return requiredIdentifiers.allSatisfy { observedIdentifiers.contains($0) }
    }

    private func preferredConversationBackButton(in app: XCUIApplication) -> XCUIElement {
        let contentBackButton = app.buttons["agently-conversations-back"]
        if contentBackButton.exists {
            return contentBackButton
        }
        return app.buttons["BackButton"]
    }
}

private extension XCUIApplication {
    func visibleReportBuilderIdentifiers(matching expectedIdentifiers: [String]) -> Set<String> {
        let visibleFrame = windows.firstMatch.frame
        let elements = descendants(matching: .any).allElementsBoundByIndex
        var matched = Set<String>()
        for element in elements {
            guard element.exists, element.frame.intersects(visibleFrame) else { continue }
            let identifier = element.identifier.trimmingCharacters(in: .whitespacesAndNewlines)
            if expectedIdentifiers.contains(identifier) {
                matched.insert(identifier)
            }
        }
        return matched
    }
}
