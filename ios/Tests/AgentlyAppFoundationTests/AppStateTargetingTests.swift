import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class AppStateTargetingTests: XCTestCase {
    @MainActor
    func testAppStateSeedsSharedMetadataTargetContext() throws {
        let client = AgentlyClient(
            endpoints: ["appAPI": EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8181")))]
        )

        let state = AppState(client: client, bootstrapBaseURL: "http://localhost:8181")

        XCTAssertEqual(state.metadataTargetContext.platform, "ios")
        XCTAssertEqual(state.metadataTargetContext.surface, "app")
        XCTAssertTrue(state.metadataTargetContext.capabilities.contains("markdown"))
        XCTAssertTrue(state.metadataTargetContext.capabilities.contains("chart"))
    }

    @MainActor
    func testAppStateForgeRuntimeUsesExplicitIOSPlatformTargeting() throws {
        let client = AgentlyClient(
            endpoints: ["appAPI": EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8181")))]
        )

        let state = AppState(client: client, bootstrapBaseURL: "http://localhost:8181")

        let runtime = state.forgeRuntime
        let mirror = Mirror(reflecting: runtime)
        let target = mirror.children.first { $0.label == "targetContext" }?.value as? ForgeTargetContext

        XCTAssertEqual(target?.platform, "ios")
        XCTAssertTrue((target?.formFactor ?? "").isEmpty == false)
        XCTAssertTrue(target?.capabilities.contains("markdown") == true)
    }

    @MainActor
    func testAppleTargetHelpersStayAligned() throws {
        XCTAssertEqual(buildAppleTargetCapabilities(), ["markdown", "chart", "attachments", "camera", "voice"])
        XCTAssertTrue(["phone", "tablet"].contains(detectAppleFormFactor()))
    }

    func testBuildAppleClientQueryContextIncludesUIClientID() {
        let context = buildAppleClientQueryContext(
            formFactor: "phone",
            uiClientID: "ios-ui-123"
        )

        if case .string(let clientID)? = context["uiClientId"] {
            XCTAssertEqual(clientID, "ios-ui-123")
        } else {
            XCTFail("Expected uiClientId string")
        }
        if case .object(let client)? = context["client"],
           case .string(let platform)? = client["platform"] {
            XCTAssertEqual(platform, "ios")
        } else {
            XCTFail("Expected client platform string")
        }
    }

    func testResolvedBootstrapOOBSecretReferencePrefersDeveloperOverrides() {
        XCTAssertEqual(
            resolvedBootstrapOOBSecretReference(
                storedValue: "stored-oob-reference|blowfish://default",
                environmentValue: "env-oob-reference|blowfish://default",
                launchArguments: ["Agently", "--enableDevAuth=1"]
            ),
            "env-oob-reference|blowfish://default"
        )
        XCTAssertEqual(
            resolvedBootstrapOOBSecretReference(
                storedValue: "   ",
                environmentValue: "env-oob-reference|blowfish://default",
                launchArguments: ["Agently", "--enableDevAuth=1"]
            ),
            "env-oob-reference|blowfish://default"
        )
        XCTAssertEqual(
            resolvedBootstrapOOBSecretReference(
                storedValue: "   ",
                environmentValue: "   ",
                launchArguments: ["Agently", "--enableDevAuth=1", "--oobSecretReference=launch-oob-reference|blowfish://default"]
            ),
            "launch-oob-reference|blowfish://default"
        )
        XCTAssertEqual(
            resolvedBootstrapOOBSecretReference(
                storedValue: "stored-oob-reference|blowfish://default",
                environmentValue: "   ",
                launchArguments: ["Agently", "--enableDevAuth=1", "--oobSecretReference=launch-oob-reference|blowfish://default"]
            ),
            "launch-oob-reference|blowfish://default"
        )
    }

    func testResolvedBootstrapOOBSecretReferenceIgnoresOverridesOutsideDevMode() {
        XCTAssertEqual(
            resolvedBootstrapOOBSecretReference(
                storedValue: "stored-oob-reference|blowfish://default",
                environmentValue: "env-oob-reference|blowfish://default",
                launchArguments: ["Agently", "--oobSecretReference=launch-oob-reference|blowfish://default"]
            ),
            "stored-oob-reference|blowfish://default"
        )
    }

    func testResolvedBootstrapActiveConversationIDPrefersEnvironmentOverrideInDevMode() {
        XCTAssertEqual(
            resolvedBootstrapActiveConversationID(
                storedValue: "stored-conversation",
                environmentValue: "env-conversation",
                launchArguments: [],
                developerAuthEnabled: true
            ),
            "env-conversation"
        )
        XCTAssertEqual(
            resolvedBootstrapActiveConversationID(
                storedValue: "stored-conversation",
                environmentValue: "   ",
                launchArguments: []
            ),
            ""
        )
        XCTAssertEqual(
            resolvedBootstrapActiveConversationID(
                storedValue: "stored-conversation",
                environmentValue: nil,
                launchArguments: ["Agently", "--activeConversationID=launch-conversation"],
                developerAuthEnabled: true
            ),
            "launch-conversation"
        )
    }

    func testResolvedBootstrapActiveConversationIDIgnoresOverridesOutsideDevMode() {
        XCTAssertEqual(
            resolvedBootstrapActiveConversationID(
                storedValue: "stored-conversation",
                environmentValue: "env-conversation",
                launchArguments: ["Agently", "--activeConversationID=launch-conversation"],
                developerAuthEnabled: false
            ),
            ""
        )
    }

    func testStartNewConversationLaunchOverrideIsDevOnly() {
        XCTAssertTrue(
            resolvedBootstrapStartsNewConversation(
                launchArguments: ["Agently", "--startNewConversation=1"],
                developerAuthEnabled: true
            )
        )
        XCTAssertFalse(
            resolvedBootstrapStartsNewConversation(
                launchArguments: ["Agently", "--startNewConversation=1"],
                developerAuthEnabled: false
            )
        )
    }

    func testDeveloperInitialWorkspaceRequestIsGenericAndDevOnly() {
        let arguments = [
            "Agently",
            "--initialWorkspaceWindowKey=line",
            "--initialWorkspaceWindowTitle=Line preview",
            "--initialWorkspaceWindowParametersJSON={\"AudienceId\":[7364938]}"
        ]

        XCTAssertNil(resolvedDeveloperInitialWorkspaceRequest(
            launchArguments: arguments,
            developerAuthEnabled: false
        ))

        let request = resolvedDeveloperInitialWorkspaceRequest(
            launchArguments: arguments,
            developerAuthEnabled: true
        )
        XCTAssertEqual(request?.windowKey, "line")
        XCTAssertEqual(request?.windowTitle, "Line preview")
        XCTAssertEqual(request?.parameters["AudienceId"], .array([.number(7_364_938)]))
    }

    func testDeveloperInitialWorkspaceRejectsMalformedParameters() {
        XCTAssertNil(resolvedDeveloperInitialWorkspaceRequest(
            launchArguments: [
                "Agently",
                "--initialWorkspaceWindowKey=order",
                "--initialWorkspaceWindowParametersJSON={invalid"
            ],
            developerAuthEnabled: true
        ))
    }

    func testComposerLookupSearchSupportsIdentifierAndNameFromOneField() throws {
        let entry = try JSONDecoder().decode(LookupRegistryEntry.self, from: Data(#"""
        {
          "name":"order",
          "title":"Order",
          "dataSource":"ad_order_lookup",
          "token":{
            "queryInput":"AdOrderName",
            "resolveInput":"AdOrderId"
          }
        }
        """#.utf8))

        let identifierCandidates = composerLookupSearchInputCandidates(entry: entry, searchQuery: " 2688386 ")
        XCTAssertEqual(identifierCandidates.count, 2)
        XCTAssertEqual(identifierCandidates[0]["AdOrderId"], .string("2688386"))
        XCTAssertEqual(identifierCandidates[1]["AdOrderName"], .string("2688386"))

        let nameCandidates = composerLookupSearchInputCandidates(entry: entry, searchQuery: "Performance")
        XCTAssertEqual(nameCandidates[0]["AdOrderName"], .string("Performance"))
        XCTAssertEqual(nameCandidates[1]["AdOrderId"], .string("Performance"))
    }

    func testResolvedBootstrapAutoOOBSignInHonorsEnvironmentAndLaunchArgs() {
        XCTAssertTrue(
            resolvedBootstrapAutoOOBSignIn(
                environmentValue: "1",
                launchArguments: ["Agently", "--enableDevAuth=1"]
            )
        )
        XCTAssertTrue(
            resolvedBootstrapAutoOOBSignIn(
                environmentValue: nil,
                launchArguments: ["Agently", "--enableDevAuth=1", "--autoOOBSignIn=1"]
            )
        )
        XCTAssertFalse(
            resolvedBootstrapAutoOOBSignIn(
                environmentValue: nil,
                launchArguments: []
            )
        )
    }

    func testResolvedBootstrapAPIBaseURLPrefersDeveloperOverrides() {
        XCTAssertEqual(
            resolvedBootstrapAPIBaseURL(
                storedValue: "http://127.0.0.1:9294",
                environmentValue: "http://127.0.0.1:9292",
                launchArguments: ["Agently", "--enableDevAuth=1"]
            ),
            "http://127.0.0.1:9292"
        )
        XCTAssertEqual(
            resolvedBootstrapAPIBaseURL(
                storedValue: "http://127.0.0.1:9294",
                environmentValue: "   ",
                launchArguments: ["Agently", "--enableDevAuth=1", "--apiBaseURL=http://localhost:9292"]
            ),
            "http://localhost:9292"
        )
        XCTAssertEqual(
            resolvedBootstrapAPIBaseURL(
                storedValue: "http://127.0.0.1:9294",
                environmentValue: "   ",
                launchArguments: []
            ),
            "http://127.0.0.1:9294"
        )
    }

    func testNormalizeAPIBaseURLRepairsSingleSlashScheme() {
        XCTAssertEqual(
            AppSettingsStore.normalizeAPIBaseURL("http:/127.0.0.1:9292"),
            "http://127.0.0.1:9292"
        )
        XCTAssertEqual(
            AppSettingsStore.normalizeAPIBaseURL("https:/steward.agently.viantinc.com/v1/api"),
            "https://steward.agently.viantinc.com"
        )
    }
}
