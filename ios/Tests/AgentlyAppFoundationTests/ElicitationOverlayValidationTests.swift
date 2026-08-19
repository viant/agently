import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class ElicitationOverlayValidationTests: XCTestCase {
    func testFallbackElicitationRequiresAndTrimsResponse() {
        XCTAssertFalse(fallbackElicitationCanSubmit("   "))
        XCTAssertTrue(fallbackElicitationCanSubmit("ad order"))
        XCTAssertEqual(
            fallbackElicitationPayload("  ad order  "),
            ["response": .string("ad order")]
        )
    }

    func testForgePayloadValidationRequiresMissingRequiredFields() {
        let field = makeField(name: "workspace", title: "Workspace", isRequired: true)

        let message = schemaPayloadValidationMessage(
            fields: [field],
            payload: [:],
            parseStructuredJSON: parseJSON
        )

        XCTAssertEqual(message, "Complete required fields: Workspace")
    }

    func testForgePayloadValidationAcceptsRequiredDefaultPayload() {
        let field = makeField(name: "workspace", title: "Workspace", isRequired: true)

        let message = schemaPayloadValidationMessage(
            fields: [field],
            payload: ["workspace": .string("Primary")],
            parseStructuredJSON: parseJSON
        )

        XCTAssertNil(message)
    }

    func testForgePayloadValidationAppliesFieldConstraints() {
        let field = makeField(
            name: "code",
            title: "Code",
            isRequired: true,
            pattern: "^[a-z]+$"
        )

        let message = schemaPayloadValidationMessage(
            fields: [field],
            payload: ["code": .string("ABC")],
            parseStructuredJSON: parseJSON
        )

        XCTAssertNotNil(message)
    }

    func testElicitationTerminalStatusMessageRecognizesResolvedCanceledAndTimedOutStates() {
        XCTAssertEqual(
            elicitationTerminalStatusMessage(" resolved "),
            "This elicitation has already been resolved."
        )
        XCTAssertEqual(
            elicitationTerminalStatusMessage("cancelled"),
            "This elicitation has been canceled."
        )
        XCTAssertEqual(
            elicitationTerminalStatusMessage("timed-out"),
            "This elicitation has timed out."
        )
        XCTAssertNil(elicitationTerminalStatusMessage("pending"))
        XCTAssertNil(elicitationTerminalStatusMessage(nil))
    }

    private func parseJSON(_ text: String) -> AppJSONValue? {
        guard let data = text.data(using: .utf8) else { return nil }
        return try? JSONDecoder().decode(AppJSONValue.self, from: data)
    }

    private func makeField(
        name: String,
        title: String,
        isRequired: Bool,
        pattern: String? = nil
    ) -> SchemaField {
        SchemaField(
            name: name,
            title: title,
            description: nil,
            example: nil,
            placeholder: title,
            kind: .text,
            defaultTextValue: "",
            defaultBooleanValue: false,
            isRequired: isRequired,
            options: [],
            constValue: nil,
            negatedConstraint: nil,
            allowsNull: false,
            jsonContainer: nil,
            arrayItemConstraint: nil,
            prefixItemConstraints: [],
            containsConstraint: nil,
            objectPropertyConstraints: [:],
            objectRequiredProperties: [],
            allowsAdditionalProperties: true,
            additionalPropertyConstraint: nil,
            alternativeConstraints: [],
            conjunctiveConstraints: [],
            pattern: pattern,
            minLength: nil,
            maxLength: nil,
            minItems: nil,
            maxItems: nil,
            minContains: nil,
            maxContains: nil,
            uniqueItems: false,
            minProperties: nil,
            maxProperties: nil,
            minimum: nil,
            maximum: nil,
            exclusiveMinimum: nil,
            exclusiveMaximum: nil,
            multipleOf: nil
        )
    }
}
