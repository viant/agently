import XCTest

final class ForecastingPrefillUITests: XCTestCase {
    func testOpenForecastBuilderPromptCanBeSentFromComposer() throws {
        try XCTSkipUnless(
            ProcessInfo.processInfo.environment["AGENTLY_IOS_LIVE_UI_TESTS"] == "1",
            "Set AGENTLY_IOS_LIVE_UI_TESTS=1 to run live local Steward UI verification."
        )

        let baseURL = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_BASE_URL"] ?? "http://127.0.0.1:9292"
        let oobSecret = ProcessInfo.processInfo.environment["AGENTLY_IOS_UI_TEST_OOB_SECRET"]
            ?? "~/.secret/awitas_dsp_ui.enc|blowfish://default"
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
            let backButton = app.buttons["BackButton"]
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
        XCTAssertTrue(forecastingTitle.waitForExistence(timeout: 180), "Forecasting pane did not open")
        RunLoop.current.run(until: Date().addingTimeInterval(45))
    }
}
