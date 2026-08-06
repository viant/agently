import XCTest

final class ForecastingPrefillUITests: XCTestCase {
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
