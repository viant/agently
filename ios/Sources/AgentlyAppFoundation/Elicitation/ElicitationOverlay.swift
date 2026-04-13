import SwiftUI
import AgentlySDK
import ForgeIOSRuntime
import ForgeIOSUI

public struct ElicitationOverlay: View {
    let pending: PendingElicitation?
    let errorMessage: String?
    let isResolving: Bool
    /// When provided, schema-form elicitations render via Forge's
    /// SchemaBasedFormRenderer instead of the built-in field editor.
    let forgeRuntime: ForgeRuntime?
    let onResolve: (String, [String: AppJSONValue]) -> Void
    let onDismiss: () -> Void
    @State private var fieldValues: [String: String] = [:]
    @State private var booleanValues: [String: Bool] = [:]
    @State private var forgeFormPayload: [String: AppJSONValue] = [:]

    public init(
        pending: PendingElicitation?,
        errorMessage: String? = nil,
        isResolving: Bool = false,
        forgeRuntime: ForgeRuntime? = nil,
        onResolve: @escaping (String, [String: AppJSONValue]) -> Void,
        onDismiss: @escaping () -> Void
    ) {
        self.pending = pending
        self.errorMessage = errorMessage
        self.isResolving = isResolving
        self.forgeRuntime = forgeRuntime
        self.onResolve = onResolve
        self.onDismiss = onDismiss
    }

    public var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 16) {
                Text(pending?.message ?? "Input Required")
                    .font(.headline)
                if let url = pending?.url, !url.isEmpty {
                    Text(url)
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                    if let destination = URL(string: url) {
                        Link("Open Link", destination: destination)
                            .font(.footnote)
                    }
                }
                if approvalMode {
                    Text("Review the requested action and choose how to continue.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                } else if formFields.isEmpty {
                    Text("Submit to continue this workflow.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                } else if let runtime = forgeRuntime,
                          let schema = cleanedSchema {
                    // Forge-driven form path
                    ElicitationForgeForm(
                        schema: schema,
                        elicitationID: pending?.elicitationID ?? "elicitation",
                        forgeRuntime: runtime,
                        onPayloadChange: { forgeFormPayload = $0 }
                    )
                } else {
                    // Built-in field editor fallback
                    Form {
                        ForEach(formFields, id: \.name) { field in
                            Section(field.title) {
                                fieldEditor(for: field)
                            }
                        }
                    }
                    .formStyle(.grouped)
                }
                if let errorMessage, !errorMessage.isEmpty {
                    Text(errorMessage)
                        .font(.footnote)
                        .foregroundStyle(.red)
                } else if let validationMessage {
                    Text(validationMessage)
                        .font(.footnote)
                        .foregroundStyle(.orange)
                }
                if isResolving {
                    Label("Submitting elicitation response...", systemImage: "hourglass")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                VStack(spacing: 12) {
                    if approvalMode {
                        HStack {
                            Button(approvalRejectLabel) { onResolve("decline", [:]) }
                                .disabled(isResolving)
                                .buttonStyle(.bordered)
                            Button(approvalCancelLabel) { onResolve("cancel", [:]) }
                                .disabled(isResolving)
                                .buttonStyle(.bordered)
                            Spacer()
                            Button(approvalAcceptLabel) {
                                // Use Forge form payload when runtime is active,
                                // fall back to hand-built form payload.
                                let payload = forgeRuntime != nil
                                    ? forgeFormPayload : approvalPayload()
                                onResolve("accept", payload)
                            }
                            .disabled(isResolving || validationMessage != nil)
                            .buttonStyle(.borderedProminent)
                        }
                    } else {
                        Button("Submit") {
                            let payload = forgeRuntime != nil ? forgeFormPayload : formPayload()
                            onResolve("accept", payload)
                        }
                        .disabled(isResolving || (forgeRuntime == nil && validationMessage != nil))
                        .buttonStyle(.borderedProminent)
                        Button("Cancel") {
                            onResolve("cancel", [:])
                        }
                        .disabled(isResolving)
                        .buttonStyle(.bordered)
                    }
                }
            }
            .padding()
            .navigationTitle("Elicitation")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close", action: onDismiss)
                        .disabled(isResolving)
                }
            }
        }
        .onAppear {
            seedDefaults()
        }
    }

    private var approvalMode: Bool {
        approvalMeta != nil
    }

    private var approvalMeta: ApprovalMeta? {
        ApprovalMetadataSupport.extractToolApprovalMeta(requestedSchemaObject)
    }

    private var approvalAcceptLabel: String {
        nonBlank(approvalMeta?.acceptLabel) ?? "Approve"
    }

    private var approvalRejectLabel: String {
        nonBlank(approvalMeta?.rejectLabel) ?? "Decline"
    }

    private var approvalCancelLabel: String {
        nonBlank(approvalMeta?.cancelLabel) ?? "Cancel"
    }

    /// Schema with `_`-prefixed meta fields stripped, ready for form rendering.
    private var cleanedSchema: ForgeJSONValue? {
        guard let schema = requestedSchemaObject, !approvalMode else { return nil }
        // Strip _-prefixed meta fields from properties
        if let propsValue = schema["properties"],
           case .object(let props) = propsValue {
            let cleaned = props.filter { !$0.key.hasPrefix("_") }
            guard !cleaned.isEmpty else { return nil }
            var result = schema
            result["properties"] = AppJSONValue.object(cleaned)
            return AppJSONValue.object(result).forgeValue
        }
        // No properties object — return schema as-is if it has non-meta keys
        let visible = schema.filter { !$0.key.hasPrefix("_") }
        return visible.isEmpty ? nil : AppJSONValue.object(visible).forgeValue
    }

    private var requestedSchemaObject: [String: AppJSONValue]? {
        guard case .object(let object)? = pending?.requestedSchema else { return nil }
        return object
    }

    private var formFields: [SchemaField] {
        guard let schema = requestedSchemaObject else { return [] }
        let properties = schemaProperties(in: schema)
        let requiredNames = requiredFieldNames(in: schema)
        return properties.compactMap { key, value in
            guard !key.hasPrefix("_"), case .object(let fieldSchema) = value else { return nil }
            let title = jsonString(fieldSchema["title"]) ?? key
            let description = jsonString(fieldSchema["description"])
            let example = stringArray(fieldSchema["examples"]).first
            let typeNames = schemaTypeNames(fieldSchema["type"])
            let format = jsonString(fieldSchema["format"])?.lowercased()
            let defaultValue = fieldSchema["default"]
            let options = stringArray(fieldSchema["enum"])
            let constValue = fieldSchema["const"]
            let negatedConstraint = negatedConstraint(from: fieldSchema["not"])
            let arrayItemConstraint = arrayItemConstraint(from: fieldSchema["items"])
            let prefixItemConstraints = prefixItemConstraints(from: fieldSchema["prefixItems"])
            let containsConstraint = containsConstraint(from: fieldSchema["contains"])
            let objectPropertyConstraints = objectPropertyConstraints(from: fieldSchema["properties"])
            let objectRequiredProperties = requiredFieldNames(in: fieldSchema)
            let allowsAdditionalProperties = additionalPropertiesAllowed(in: fieldSchema)
            let additionalPropertyConstraint = additionalPropertyConstraint(from: fieldSchema["additionalProperties"])
            let alternativeConstraints = alternativeConstraints(from: fieldSchema["oneOf"], fallback: fieldSchema["anyOf"])
            let conjunctiveConstraints = conjunctiveConstraints(from: fieldSchema["allOf"])
            return SchemaField(
                name: key,
                title: title,
                description: description,
                example: example,
                placeholder: schemaPlaceholder(
                    title: title,
                    description: description,
                    example: example,
                    options: options
                ),
                kind: schemaFieldKind(typeNames: typeNames, format: format, options: options),
                defaultTextValue: defaultTextValueText(defaultValue),
                defaultBooleanValue: jsonBool(defaultValue) ?? false,
                isRequired: requiredNames.contains(key),
                options: options,
                constValue: constValue,
                negatedConstraint: negatedConstraint,
                allowsNull: typeNames.contains("null"),
                jsonContainer: jsonContainerKind(typeNames: typeNames),
                arrayItemConstraint: arrayItemConstraint,
                prefixItemConstraints: prefixItemConstraints,
                containsConstraint: containsConstraint,
                objectPropertyConstraints: objectPropertyConstraints,
                objectRequiredProperties: objectRequiredProperties,
                allowsAdditionalProperties: allowsAdditionalProperties,
                additionalPropertyConstraint: additionalPropertyConstraint,
                alternativeConstraints: alternativeConstraints,
                conjunctiveConstraints: conjunctiveConstraints,
                pattern: jsonString(fieldSchema["pattern"]),
                minLength: jsonInteger(fieldSchema["minLength"]),
                maxLength: jsonInteger(fieldSchema["maxLength"]),
                minItems: jsonInteger(fieldSchema["minItems"]),
                maxItems: jsonInteger(fieldSchema["maxItems"]),
                minContains: jsonInteger(fieldSchema["minContains"]),
                maxContains: jsonInteger(fieldSchema["maxContains"]),
                uniqueItems: jsonBool(fieldSchema["uniqueItems"]) ?? false,
                minProperties: jsonInteger(fieldSchema["minProperties"]),
                maxProperties: jsonInteger(fieldSchema["maxProperties"]),
                minimum: jsonNumber(fieldSchema["minimum"]),
                maximum: jsonNumber(fieldSchema["maximum"]),
                exclusiveMinimum: jsonNumber(fieldSchema["exclusiveMinimum"]),
                exclusiveMaximum: jsonNumber(fieldSchema["exclusiveMaximum"]),
                multipleOf: jsonNumber(fieldSchema["multipleOf"])
            )
        }
        .sorted { $0.name < $1.name }
    }

    private var validationMessage: String? {
        let invalidFieldTitles = formFields
            .compactMap(fieldValidationMessage(for:))

        if !invalidFieldTitles.isEmpty {
            return invalidFieldTitles.joined(separator: " ")
        }

        let missingTitles = formFields
            .filter(\.isRequired)
            .filter { !fieldHasValue($0) }
            .map(\.title)

        guard !missingTitles.isEmpty else { return nil }
        return "Complete required fields: \(missingTitles.joined(separator: ", "))"
    }

    private func binding(for name: String) -> Binding<String> {
        Binding(
            get: { fieldValues[name] ?? "" },
            set: { fieldValues[name] = $0 }
        )
    }

    private func booleanBinding(for name: String) -> Binding<Bool> {
        Binding(
            get: { booleanValues[name] ?? false },
            set: { booleanValues[name] = $0 }
        )
    }

    private func formPayload() -> [String: AppJSONValue] {
        let pairs: [(String, AppJSONValue)] = formFields.compactMap { field in
            payloadEntry(for: field)
        }
        return Dictionary(uniqueKeysWithValues: pairs)
    }

    private func approvalPayload() -> [String: AppJSONValue] {
        let pairs: [(String, AppJSONValue)] = formFields.compactMap { field in
            payloadEntry(for: field)
        }
        let editedFields = Dictionary(uniqueKeysWithValues: pairs)
        return editedFields.isEmpty ? [:] : ["editedFields": .object(editedFields)]
    }

    private func seedDefaults() {
        guard fieldValues.isEmpty else { return }
        fieldValues = Dictionary(uniqueKeysWithValues: formFields.map { field in
            (field.name, field.defaultTextValue)
        })
        booleanValues = Dictionary(uniqueKeysWithValues: formFields.compactMap { field in
            guard field.kind == .boolean else { return nil }
            return (field.name, field.defaultBooleanValue)
        })
    }

    private func schemaProperties(in schema: [String: AppJSONValue]) -> [String: AppJSONValue] {
        if case .object(let properties)? = schema["properties"] {
            return properties
        }
        return [:]
    }

    private func requiredFieldNames(in schema: [String: AppJSONValue]) -> Set<String> {
        guard case .array(let values)? = schema["required"] else { return [] }
        let names = values.compactMap { value -> String? in
            guard case .string(let string) = value else { return nil }
            return string
        }
        return Set(names)
    }

    private func jsonString(_ value: AppJSONValue?) -> String? {
        guard case .string(let string)? = value else { return nil }
        return string
    }

    private func nonBlank(_ value: String?) -> String? {
        guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !trimmed.isEmpty else {
            return nil
        }
        return trimmed
    }

    private func jsonBool(_ value: AppJSONValue?) -> Bool? {
        guard case .bool(let bool)? = value else { return nil }
        return bool
    }

    private func jsonNumber(_ value: AppJSONValue?) -> Double? {
        guard case .number(let number)? = value else { return nil }
        return number
    }

    private func jsonInteger(_ value: AppJSONValue?) -> Int? {
        guard case .number(let number)? = value else { return nil }
        let rounded = number.rounded(.towardZero)
        guard rounded == number else { return nil }
        return Int(rounded)
    }

    private func stringArray(_ value: AppJSONValue?) -> [String] {
        guard case .array(let values)? = value else { return [] }
        return values.compactMap { item in
            guard case .string(let string) = item else { return nil }
            return string
        }
    }

    private func schemaTypeNames(_ value: AppJSONValue?) -> [String] {
        switch value {
        case .string(let typeName):
            return [typeName.lowercased()]
        case .array(let values):
            return values.compactMap { item in
                guard case .string(let typeName) = item else { return nil }
                return typeName.lowercased()
            }
        default:
            return []
        }
    }

    private func schemaPlaceholder(
        title: String,
        description: String?,
        example: String?,
        options: [String]
    ) -> String {
        if !options.isEmpty {
            return "Select \(title)"
        }
        if let description, !description.isEmpty {
            return description
        }
        if let example, !example.isEmpty {
            return "Example: \(example)"
        }
        return "Enter \(title)"
    }

    private func schemaFieldKind(typeNames: [String], format: String?, options: [String]) -> SchemaFieldKind {
        if !options.isEmpty {
            return .choice
        }

        if typeNames.contains("boolean") {
            return .boolean
        }
        if typeNames.contains("number") {
            return .number
        }
        if typeNames.contains("integer") {
            return .integer
        }

        switch format {
        case "textarea", "multiline":
            return .multiline
        case "date":
            return .date
        case "date-time", "datetime":
            return .dateTime
        case "email":
            return .email
        case "uri", "url":
            return .url
        default:
            break
        }

        if typeNames.contains("array") || typeNames.contains("object") {
            return .multiline
        }

        return .text
    }

    private func jsonContainerKind(typeNames: [String]) -> JSONContainerKind? {
        if typeNames.contains("object") {
            return .object
        }
        if typeNames.contains("array") {
            return .array
        }
        return nil
    }

    private func arrayItemConstraint(from value: AppJSONValue?) -> ArrayItemConstraint? {
        guard case .object(let itemSchema)? = value else { return nil }
        let typeNames = schemaTypeNames(itemSchema["type"])
        let format = jsonString(itemSchema["format"])?.lowercased()
        let options = stringArray(itemSchema["enum"])
        let constValue = itemSchema["const"]
        let negatedConstraint = negatedConstraint(from: itemSchema["not"])
        return ArrayItemConstraint(
            typeNames: typeNames,
            format: format,
            options: options,
            constValue: constValue,
            negatedConstraint: negatedConstraint,
            arrayItemConstraint: arrayItemConstraint(from: itemSchema["items"]),
            prefixItemConstraints: prefixItemConstraints(from: itemSchema["prefixItems"]),
            containsConstraint: containsConstraint(from: itemSchema["contains"]),
            objectPropertyConstraints: objectPropertyConstraints(from: itemSchema["properties"]),
            objectRequiredProperties: requiredFieldNames(in: itemSchema),
            allowsAdditionalProperties: additionalPropertiesAllowed(in: itemSchema),
            additionalPropertyConstraint: additionalPropertyConstraint(from: itemSchema["additionalProperties"]),
            alternativeConstraints: alternativeConstraints(from: itemSchema["oneOf"], fallback: itemSchema["anyOf"]),
            conjunctiveConstraints: conjunctiveConstraints(from: itemSchema["allOf"]),
            pattern: jsonString(itemSchema["pattern"]),
            minLength: jsonInteger(itemSchema["minLength"]),
            maxLength: jsonInteger(itemSchema["maxLength"]),
            minItems: jsonInteger(itemSchema["minItems"]),
            maxItems: jsonInteger(itemSchema["maxItems"]),
            minContains: jsonInteger(itemSchema["minContains"]),
            maxContains: jsonInteger(itemSchema["maxContains"]),
            uniqueItems: jsonBool(itemSchema["uniqueItems"]) ?? false,
            minProperties: jsonInteger(itemSchema["minProperties"]),
            maxProperties: jsonInteger(itemSchema["maxProperties"]),
            minimum: jsonNumber(itemSchema["minimum"]),
            maximum: jsonNumber(itemSchema["maximum"]),
            exclusiveMinimum: jsonNumber(itemSchema["exclusiveMinimum"]),
            exclusiveMaximum: jsonNumber(itemSchema["exclusiveMaximum"]),
            multipleOf: jsonNumber(itemSchema["multipleOf"])
        )
    }

    private func objectPropertyConstraints(from value: AppJSONValue?) -> [String: ObjectPropertyConstraint] {
        guard case .object(let properties)? = value else { return [:] }
        return properties.reduce(into: [String: ObjectPropertyConstraint]()) { result, entry in
            let (name, propertyValue) = entry
            guard case .object(let propertySchema) = propertyValue else { return }
            let typeNames = schemaTypeNames(propertySchema["type"])
            let format = jsonString(propertySchema["format"])?.lowercased()
            let options = stringArray(propertySchema["enum"])
            let constValue = propertySchema["const"]
            let negatedConstraint = negatedConstraint(from: propertySchema["not"])
            result[name] = ObjectPropertyConstraint(
                name: name,
                typeNames: typeNames,
                format: format,
                options: options,
                constValue: constValue,
                negatedConstraint: negatedConstraint,
                arrayItemConstraint: arrayItemConstraint(from: propertySchema["items"]),
                prefixItemConstraints: prefixItemConstraints(from: propertySchema["prefixItems"]),
                containsConstraint: containsConstraint(from: propertySchema["contains"]),
                objectPropertyConstraints: objectPropertyConstraints(from: propertySchema["properties"]),
                objectRequiredProperties: requiredFieldNames(in: propertySchema),
                allowsAdditionalProperties: additionalPropertiesAllowed(in: propertySchema),
                additionalPropertyConstraint: additionalPropertyConstraint(from: propertySchema["additionalProperties"]),
                alternativeConstraints: alternativeConstraints(from: propertySchema["oneOf"], fallback: propertySchema["anyOf"]),
                conjunctiveConstraints: conjunctiveConstraints(from: propertySchema["allOf"]),
                pattern: jsonString(propertySchema["pattern"]),
                minLength: jsonInteger(propertySchema["minLength"]),
                maxLength: jsonInteger(propertySchema["maxLength"]),
                minItems: jsonInteger(propertySchema["minItems"]),
                maxItems: jsonInteger(propertySchema["maxItems"]),
                minContains: jsonInteger(propertySchema["minContains"]),
                maxContains: jsonInteger(propertySchema["maxContains"]),
                uniqueItems: jsonBool(propertySchema["uniqueItems"]) ?? false,
                minProperties: jsonInteger(propertySchema["minProperties"]),
                maxProperties: jsonInteger(propertySchema["maxProperties"]),
                minimum: jsonNumber(propertySchema["minimum"]),
                maximum: jsonNumber(propertySchema["maximum"]),
                exclusiveMinimum: jsonNumber(propertySchema["exclusiveMinimum"]),
                exclusiveMaximum: jsonNumber(propertySchema["exclusiveMaximum"]),
                multipleOf: jsonNumber(propertySchema["multipleOf"])
            )
        }
    }

    private func additionalPropertiesAllowed(in schema: [String: AppJSONValue]) -> Bool {
        guard let value = schema["additionalProperties"] else { return true }
        if case .bool(let allowed) = value {
            return allowed
        }
        return true
    }

    private func additionalPropertyConstraint(from value: AppJSONValue?) -> ArrayItemConstraint? {
        guard case .object(let schema)? = value else { return nil }
        let typeNames = schemaTypeNames(schema["type"])
        let format = jsonString(schema["format"])?.lowercased()
        let options = stringArray(schema["enum"])
        let constValue = schema["const"]
        let negatedConstraint = negatedConstraint(from: schema["not"])
        return ArrayItemConstraint(
            typeNames: typeNames,
            format: format,
            options: options,
            constValue: constValue,
            negatedConstraint: negatedConstraint,
            arrayItemConstraint: arrayItemConstraint(from: schema["items"]),
            prefixItemConstraints: prefixItemConstraints(from: schema["prefixItems"]),
            containsConstraint: containsConstraint(from: schema["contains"]),
            objectPropertyConstraints: objectPropertyConstraints(from: schema["properties"]),
            objectRequiredProperties: requiredFieldNames(in: schema),
            allowsAdditionalProperties: additionalPropertiesAllowed(in: schema),
            additionalPropertyConstraint: additionalPropertyConstraint(from: schema["additionalProperties"]),
            alternativeConstraints: alternativeConstraints(from: schema["oneOf"], fallback: schema["anyOf"]),
            conjunctiveConstraints: conjunctiveConstraints(from: schema["allOf"]),
            pattern: jsonString(schema["pattern"]),
            minLength: jsonInteger(schema["minLength"]),
            maxLength: jsonInteger(schema["maxLength"]),
            minItems: jsonInteger(schema["minItems"]),
            maxItems: jsonInteger(schema["maxItems"]),
            minContains: jsonInteger(schema["minContains"]),
            maxContains: jsonInteger(schema["maxContains"]),
            uniqueItems: jsonBool(schema["uniqueItems"]) ?? false,
            minProperties: jsonInteger(schema["minProperties"]),
            maxProperties: jsonInteger(schema["maxProperties"]),
            minimum: jsonNumber(schema["minimum"]),
            maximum: jsonNumber(schema["maximum"]),
            exclusiveMinimum: jsonNumber(schema["exclusiveMinimum"]),
            exclusiveMaximum: jsonNumber(schema["exclusiveMaximum"]),
            multipleOf: jsonNumber(schema["multipleOf"])
        )
    }

    private func alternativeConstraints(from primary: AppJSONValue?, fallback: AppJSONValue?) -> [ArrayItemConstraint] {
        let source = primary ?? fallback
        guard case .array(let values)? = source else { return [] }
        return values.compactMap { value in
            guard case .object(let schema) = value else { return nil }
            let typeNames = schemaTypeNames(schema["type"])
            let format = jsonString(schema["format"])?.lowercased()
            let options = stringArray(schema["enum"])
            let constValue = schema["const"]
            let negatedConstraint = negatedConstraint(from: schema["not"])
            return ArrayItemConstraint(
                typeNames: typeNames,
                format: format,
                options: options,
                constValue: constValue,
                negatedConstraint: negatedConstraint,
                arrayItemConstraint: arrayItemConstraint(from: schema["items"]),
                prefixItemConstraints: prefixItemConstraints(from: schema["prefixItems"]),
                containsConstraint: containsConstraint(from: schema["contains"]),
                objectPropertyConstraints: objectPropertyConstraints(from: schema["properties"]),
                objectRequiredProperties: requiredFieldNames(in: schema),
                allowsAdditionalProperties: additionalPropertiesAllowed(in: schema),
                additionalPropertyConstraint: additionalPropertyConstraint(from: schema["additionalProperties"]),
                alternativeConstraints: alternativeConstraints(from: schema["oneOf"], fallback: schema["anyOf"]),
                conjunctiveConstraints: conjunctiveConstraints(from: schema["allOf"]),
                pattern: jsonString(schema["pattern"]),
                minLength: jsonInteger(schema["minLength"]),
                maxLength: jsonInteger(schema["maxLength"]),
                minItems: jsonInteger(schema["minItems"]),
                maxItems: jsonInteger(schema["maxItems"]),
                minContains: jsonInteger(schema["minContains"]),
                maxContains: jsonInteger(schema["maxContains"]),
                uniqueItems: jsonBool(schema["uniqueItems"]) ?? false,
                minProperties: jsonInteger(schema["minProperties"]),
                maxProperties: jsonInteger(schema["maxProperties"]),
                minimum: jsonNumber(schema["minimum"]),
                maximum: jsonNumber(schema["maximum"]),
                exclusiveMinimum: jsonNumber(schema["exclusiveMinimum"]),
                exclusiveMaximum: jsonNumber(schema["exclusiveMaximum"]),
                multipleOf: jsonNumber(schema["multipleOf"])
            )
        }
    }

    private func conjunctiveConstraints(from value: AppJSONValue?) -> [ArrayItemConstraint] {
        guard case .array(let values)? = value else { return [] }
        return values.compactMap { value in
            guard case .object(let schema) = value else { return nil }
            let typeNames = schemaTypeNames(schema["type"])
            let format = jsonString(schema["format"])?.lowercased()
            let options = stringArray(schema["enum"])
            let constValue = schema["const"]
            let negatedConstraint = negatedConstraint(from: schema["not"])
            return ArrayItemConstraint(
                typeNames: typeNames,
                format: format,
                options: options,
                constValue: constValue,
                negatedConstraint: negatedConstraint,
                arrayItemConstraint: arrayItemConstraint(from: schema["items"]),
                prefixItemConstraints: prefixItemConstraints(from: schema["prefixItems"]),
                containsConstraint: containsConstraint(from: schema["contains"]),
                objectPropertyConstraints: objectPropertyConstraints(from: schema["properties"]),
                objectRequiredProperties: requiredFieldNames(in: schema),
                allowsAdditionalProperties: additionalPropertiesAllowed(in: schema),
                additionalPropertyConstraint: additionalPropertyConstraint(from: schema["additionalProperties"]),
                alternativeConstraints: alternativeConstraints(from: schema["oneOf"], fallback: schema["anyOf"]),
                conjunctiveConstraints: conjunctiveConstraints(from: schema["allOf"]),
                pattern: jsonString(schema["pattern"]),
                minLength: jsonInteger(schema["minLength"]),
                maxLength: jsonInteger(schema["maxLength"]),
                minItems: jsonInteger(schema["minItems"]),
                maxItems: jsonInteger(schema["maxItems"]),
                minContains: jsonInteger(schema["minContains"]),
                maxContains: jsonInteger(schema["maxContains"]),
                uniqueItems: jsonBool(schema["uniqueItems"]) ?? false,
                minProperties: jsonInteger(schema["minProperties"]),
                maxProperties: jsonInteger(schema["maxProperties"]),
                minimum: jsonNumber(schema["minimum"]),
                maximum: jsonNumber(schema["maximum"]),
                exclusiveMinimum: jsonNumber(schema["exclusiveMinimum"]),
                exclusiveMaximum: jsonNumber(schema["exclusiveMaximum"]),
                multipleOf: jsonNumber(schema["multipleOf"])
            )
        }
    }

    private func prefixItemConstraints(from value: AppJSONValue?) -> [ArrayItemConstraint] {
        guard case .array(let values)? = value else { return [] }
        return values.compactMap { item in
            guard case .object(let schema) = item else { return nil }
            let typeNames = schemaTypeNames(schema["type"])
            let format = jsonString(schema["format"])?.lowercased()
            let options = stringArray(schema["enum"])
            let constValue = schema["const"]
            let negatedConstraint = negatedConstraint(from: schema["not"])
            return ArrayItemConstraint(
                typeNames: typeNames,
                format: format,
                options: options,
                constValue: constValue,
                negatedConstraint: negatedConstraint,
                arrayItemConstraint: arrayItemConstraint(from: schema["items"]),
                prefixItemConstraints: prefixItemConstraints(from: schema["prefixItems"]),
                containsConstraint: containsConstraint(from: schema["contains"]),
                objectPropertyConstraints: objectPropertyConstraints(from: schema["properties"]),
                objectRequiredProperties: requiredFieldNames(in: schema),
                allowsAdditionalProperties: additionalPropertiesAllowed(in: schema),
                additionalPropertyConstraint: additionalPropertyConstraint(from: schema["additionalProperties"]),
                alternativeConstraints: alternativeConstraints(from: schema["oneOf"], fallback: schema["anyOf"]),
                conjunctiveConstraints: conjunctiveConstraints(from: schema["allOf"]),
                pattern: jsonString(schema["pattern"]),
                minLength: jsonInteger(schema["minLength"]),
                maxLength: jsonInteger(schema["maxLength"]),
                minItems: jsonInteger(schema["minItems"]),
                maxItems: jsonInteger(schema["maxItems"]),
                minContains: jsonInteger(schema["minContains"]),
                maxContains: jsonInteger(schema["maxContains"]),
                uniqueItems: jsonBool(schema["uniqueItems"]) ?? false,
                minProperties: jsonInteger(schema["minProperties"]),
                maxProperties: jsonInteger(schema["maxProperties"]),
                minimum: jsonNumber(schema["minimum"]),
                maximum: jsonNumber(schema["maximum"]),
                exclusiveMinimum: jsonNumber(schema["exclusiveMinimum"]),
                exclusiveMaximum: jsonNumber(schema["exclusiveMaximum"]),
                multipleOf: jsonNumber(schema["multipleOf"])
            )
        }
    }

    private func containsConstraint(from value: AppJSONValue?) -> ArrayItemConstraint? {
        guard case .object(let schema)? = value else { return nil }
        let typeNames = schemaTypeNames(schema["type"])
        let format = jsonString(schema["format"])?.lowercased()
        let options = stringArray(schema["enum"])
        let constValue = schema["const"]
        let negatedConstraint = negatedConstraint(from: schema["not"])
        return ArrayItemConstraint(
            typeNames: typeNames,
            format: format,
            options: options,
            constValue: constValue,
            negatedConstraint: negatedConstraint,
            arrayItemConstraint: arrayItemConstraint(from: schema["items"]),
            prefixItemConstraints: prefixItemConstraints(from: schema["prefixItems"]),
            containsConstraint: containsConstraint(from: schema["contains"]),
            objectPropertyConstraints: objectPropertyConstraints(from: schema["properties"]),
            objectRequiredProperties: requiredFieldNames(in: schema),
            allowsAdditionalProperties: additionalPropertiesAllowed(in: schema),
            additionalPropertyConstraint: additionalPropertyConstraint(from: schema["additionalProperties"]),
            alternativeConstraints: alternativeConstraints(from: schema["oneOf"], fallback: schema["anyOf"]),
            conjunctiveConstraints: conjunctiveConstraints(from: schema["allOf"]),
            pattern: jsonString(schema["pattern"]),
            minLength: jsonInteger(schema["minLength"]),
            maxLength: jsonInteger(schema["maxLength"]),
            minItems: jsonInteger(schema["minItems"]),
            maxItems: jsonInteger(schema["maxItems"]),
            minContains: jsonInteger(schema["minContains"]),
            maxContains: jsonInteger(schema["maxContains"]),
            uniqueItems: jsonBool(schema["uniqueItems"]) ?? false,
            minProperties: jsonInteger(schema["minProperties"]),
            maxProperties: jsonInteger(schema["maxProperties"]),
            minimum: jsonNumber(schema["minimum"]),
            maximum: jsonNumber(schema["maximum"]),
            exclusiveMinimum: jsonNumber(schema["exclusiveMinimum"]),
            exclusiveMaximum: jsonNumber(schema["exclusiveMaximum"]),
            multipleOf: jsonNumber(schema["multipleOf"])
        )
    }

    private func negatedConstraint(from value: AppJSONValue?) -> ArrayItemConstraint? {
        guard case .object(let schema)? = value else { return nil }
        let typeNames = schemaTypeNames(schema["type"])
        let format = jsonString(schema["format"])?.lowercased()
        let options = stringArray(schema["enum"])
        let constValue = schema["const"]
        return ArrayItemConstraint(
            typeNames: typeNames,
            format: format,
            options: options,
            constValue: constValue,
            negatedConstraint: negatedConstraint(from: schema["not"]),
            arrayItemConstraint: arrayItemConstraint(from: schema["items"]),
            prefixItemConstraints: prefixItemConstraints(from: schema["prefixItems"]),
            containsConstraint: containsConstraint(from: schema["contains"]),
            objectPropertyConstraints: objectPropertyConstraints(from: schema["properties"]),
            objectRequiredProperties: requiredFieldNames(in: schema),
            allowsAdditionalProperties: additionalPropertiesAllowed(in: schema),
            additionalPropertyConstraint: additionalPropertyConstraint(from: schema["additionalProperties"]),
            alternativeConstraints: alternativeConstraints(from: schema["oneOf"], fallback: schema["anyOf"]),
            conjunctiveConstraints: conjunctiveConstraints(from: schema["allOf"]),
            pattern: jsonString(schema["pattern"]),
            minLength: jsonInteger(schema["minLength"]),
            maxLength: jsonInteger(schema["maxLength"]),
            minItems: jsonInteger(schema["minItems"]),
            maxItems: jsonInteger(schema["maxItems"]),
            minContains: jsonInteger(schema["minContains"]),
            maxContains: jsonInteger(schema["maxContains"]),
            uniqueItems: jsonBool(schema["uniqueItems"]) ?? false,
            minProperties: jsonInteger(schema["minProperties"]),
            maxProperties: jsonInteger(schema["maxProperties"]),
            minimum: jsonNumber(schema["minimum"]),
            maximum: jsonNumber(schema["maximum"]),
            exclusiveMinimum: jsonNumber(schema["exclusiveMinimum"]),
            exclusiveMaximum: jsonNumber(schema["exclusiveMaximum"]),
            multipleOf: jsonNumber(schema["multipleOf"])
        )
    }

    private func defaultTextValueText(_ value: AppJSONValue?) -> String {
        switch value {
        case .string(let string):
            return string
        case .number(let number):
            if number.rounded(.towardZero) == number {
                return String(Int(number))
            }
            return String(number)
        case .bool(let bool):
            return bool ? "true" : "false"
        case .object, .array:
            return prettyPrintedJSONText(value)
        default:
            return ""
        }
    }

    private func prettyPrintedJSONText(_ value: AppJSONValue?) -> String {
        guard let value else { return "" }
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        guard let data = try? encoder.encode(value),
              let string = String(data: data, encoding: .utf8) else {
            return ""
        }
        return string
    }

    private func parseStructuredJSON(_ text: String) -> AppJSONValue? {
        guard let data = text.data(using: .utf8) else { return nil }
        return try? JSONDecoder().decode(AppJSONValue.self, from: data)
    }

    private func fieldValidationMessage(for field: SchemaField) -> String? {
        let trimmed = (fieldValues[field.name] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        return SchemaFieldValidator(
            field: field,
            text: trimmed,
            parseStructuredJSON: parseStructuredJSON
        ).validationMessage()
    }

    private func fieldHasValue(_ field: SchemaField) -> Bool {
        switch field.kind {
        case .boolean:
            return true
        case .choice:
            let trimmed = (fieldValues[field.name] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            return !trimmed.isEmpty
        case .number, .integer, .multiline, .text, .email, .url, .date, .dateTime:
            let trimmed = (fieldValues[field.name] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            return !trimmed.isEmpty
        }
    }

    @ViewBuilder
    private func fieldEditor(for field: SchemaField) -> some View {
        switch field.kind {
        case .boolean:
            VStack(alignment: .leading, spacing: 8) {
                Toggle(fieldLabel(for: field), isOn: booleanBinding(for: field.name))
                fieldHelp(for: field)
            }
        case .choice:
            VStack(alignment: .leading, spacing: 8) {
                Picker(fieldLabel(for: field), selection: binding(for: field.name)) {
                    if !field.isRequired || field.allowsNull {
                        Text("None").tag("")
                    }
                    ForEach(field.options, id: \.self) { option in
                        Text(option).tag(option)
                    }
                }
                .pickerStyle(.menu)
                fieldHelp(for: field)
            }
        case .multiline:
            VStack(alignment: .leading, spacing: 8) {
                TextField(fieldPlaceholder(for: field), text: binding(for: field.name), axis: .vertical)
                    .lineLimit(3...8)
                    .autocorrectionDisabled()
                fieldHelp(for: field)
            }
        case .number:
            VStack(alignment: .leading, spacing: 8) {
                platformTextField(for: field)
                fieldHelp(for: field)
            }
        case .integer:
            VStack(alignment: .leading, spacing: 8) {
                platformTextField(for: field)
                fieldHelp(for: field)
            }
        case .email:
            VStack(alignment: .leading, spacing: 8) {
                platformTextField(for: field)
                fieldHelp(for: field)
            }
        case .url:
            VStack(alignment: .leading, spacing: 8) {
                platformTextField(for: field)
                fieldHelp(for: field)
            }
        case .date:
            VStack(alignment: .leading, spacing: 8) {
                platformTextField(for: field)
                fieldHelp(for: field)
            }
        case .dateTime:
            VStack(alignment: .leading, spacing: 8) {
                platformTextField(for: field)
                fieldHelp(for: field)
            }
        case .text:
            VStack(alignment: .leading, spacing: 8) {
                platformTextField(for: field)
                fieldHelp(for: field)
            }
        }
    }

    private func fieldLabel(for field: SchemaField) -> String {
        field.isRequired ? "\(field.title) *" : field.title
    }

    private func fieldPlaceholder(for field: SchemaField) -> String {
        if field.isRequired {
            return "\(field.placeholder) (required)"
        }
        if field.allowsNull {
            return "\(field.placeholder) (optional)"
        }
        return field.placeholder
    }

    @ViewBuilder
    private func fieldHelp(for field: SchemaField) -> some View {
        if let validationMessage = fieldValidationMessage(for: field) {
            Text(validationMessage)
                .font(.footnote)
                .foregroundStyle(.orange)
        } else if let description = field.description, !description.isEmpty {
            Text(description)
                .font(.footnote)
                .foregroundStyle(.secondary)
        } else if let example = field.example, !example.isEmpty {
            Text("Example: \(example)")
                .font(.footnote)
                .foregroundStyle(.secondary)
        } else if field.allowsNull && !field.isRequired {
            Text("Optional")
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    @ViewBuilder
    private func platformTextField(for field: SchemaField) -> some View {
        #if os(iOS)
        switch field.kind {
        case .number:
            TextField(fieldPlaceholder(for: field), text: binding(for: field.name))
                .autocorrectionDisabled()
                .keyboardType(.decimalPad)
        case .integer:
            TextField(fieldPlaceholder(for: field), text: binding(for: field.name))
                .autocorrectionDisabled()
                .keyboardType(.numberPad)
        case .email:
            TextField(fieldPlaceholder(for: field), text: binding(for: field.name))
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .keyboardType(.emailAddress)
                .textContentType(.emailAddress)
        case .url:
            TextField(fieldPlaceholder(for: field), text: binding(for: field.name))
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .keyboardType(.URL)
                .textContentType(.URL)
        case .date:
            TextField(fieldPlaceholder(for: field), text: binding(for: field.name))
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .textContentType(.none)
        case .dateTime:
            TextField(fieldPlaceholder(for: field), text: binding(for: field.name))
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .textContentType(.none)
        case .text, .multiline, .choice, .boolean:
            TextField(fieldPlaceholder(for: field), text: binding(for: field.name))
                .autocorrectionDisabled()
        }
        #else
        TextField(fieldPlaceholder(for: field), text: binding(for: field.name))
            .autocorrectionDisabled()
        #endif
    }

    private func payloadEntry(for field: SchemaField) -> (String, AppJSONValue)? {
        switch field.kind {
        case .boolean:
            return (field.name, .bool(booleanValues[field.name] ?? field.defaultBooleanValue))
        case .number:
            let trimmed = (fieldValues[field.name] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !trimmed.isEmpty else { return field.allowsNull ? (field.name, .null) : nil }
            guard let number = Double(trimmed) else { return (field.name, .string(trimmed)) }
            return (field.name, .number(number))
        case .integer:
            let trimmed = (fieldValues[field.name] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !trimmed.isEmpty else { return field.allowsNull ? (field.name, .null) : nil }
            guard let integer = Int(trimmed) else { return (field.name, .string(trimmed)) }
            return (field.name, .number(Double(integer)))
        case .choice, .multiline, .text, .email, .url, .date, .dateTime:
            let trimmed = (fieldValues[field.name] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !trimmed.isEmpty else { return field.allowsNull ? (field.name, .null) : nil }
            if let jsonContainer = field.jsonContainer,
               let structured = parseStructuredJSON(trimmed),
               jsonContainer.matches(structured) {
                return (field.name, structured)
            }
            return (field.name, .string(trimmed))
        }
    }
}

private struct SchemaField {
    let name: String
    let title: String
    let description: String?
    let example: String?
    let placeholder: String
    let kind: SchemaFieldKind
    let defaultTextValue: String
    let defaultBooleanValue: Bool
    let isRequired: Bool
    let options: [String]
    let constValue: AppJSONValue?
    let negatedConstraint: ArrayItemConstraint?
    let allowsNull: Bool
    let jsonContainer: JSONContainerKind?
    let arrayItemConstraint: ArrayItemConstraint?
    let prefixItemConstraints: [ArrayItemConstraint]
    let containsConstraint: ArrayItemConstraint?
    let objectPropertyConstraints: [String: ObjectPropertyConstraint]
    let objectRequiredProperties: Set<String>
    let allowsAdditionalProperties: Bool
    let additionalPropertyConstraint: ArrayItemConstraint?
    let alternativeConstraints: [ArrayItemConstraint]
    let conjunctiveConstraints: [ArrayItemConstraint]
    let pattern: String?
    let minLength: Int?
    let maxLength: Int?
    let minItems: Int?
    let maxItems: Int?
    let minContains: Int?
    let maxContains: Int?
    let uniqueItems: Bool
    let minProperties: Int?
    let maxProperties: Int?
    let minimum: Double?
    let maximum: Double?
    let exclusiveMinimum: Double?
    let exclusiveMaximum: Double?
    let multipleOf: Double?
}

private struct SchemaFieldValidator {
    let field: SchemaField
    let text: String
    let parseStructuredJSON: (String) -> AppJSONValue?

    func validationMessage() -> String? {
        guard !text.isEmpty else { return nil }
        if !field.alternativeConstraints.isEmpty {
            guard let value = inputJSONValue() else {
                return "\(field.title) must match one of the allowed schema alternatives."
            }
            let matchesAlternative = field.alternativeConstraints.contains { constraint in
                constraint.validationMessage(for: value, path: field.title) == nil
            }
            if !matchesAlternative {
                return "\(field.title) must match one of the allowed schema alternatives."
            }
        }
        if !field.conjunctiveConstraints.isEmpty {
            guard let value = inputJSONValue() else {
                return "\(field.title) must satisfy all required schema constraints."
            }
            for constraint in field.conjunctiveConstraints {
                if let message = constraint.validationMessage(for: value, path: field.title) {
                    return message
                }
            }
        }
        if let negatedConstraint = field.negatedConstraint {
            guard let value = inputJSONValue() else {
                return nil
            }
            if negatedConstraint.validationMessage(for: value, path: field.title) == nil {
                return "\(field.title) must not match the disallowed schema."
            }
        }
        if let constValidationMessage = constValidationMessage() {
            return constValidationMessage
        }
        return patternValidationMessage()
            ?? structuredValidationMessage()
            ?? lengthValidationMessage()
            ?? formatValidationMessage()
            ?? numericTypeValidationMessage()
            ?? numericRangeValidationMessage()
    }

    private func inputJSONValue() -> AppJSONValue? {
        switch field.kind {
        case .boolean:
            return .bool(false)
        case .number:
            if let number = Double(text) {
                return .number(number)
            }
            return .string(text)
        case .integer:
            if let integer = Int(text) {
                return .number(Double(integer))
            }
            return .string(text)
        case .choice, .multiline, .text, .email, .url, .date, .dateTime:
            if let jsonContainer = field.jsonContainer,
               let parsed = parseStructuredJSON(text),
               jsonContainer.matches(parsed) {
                return parsed
            }
            return .string(text)
        }
    }

    private func patternValidationMessage() -> String? {
        guard let pattern = field.pattern, !pattern.isEmpty else { return nil }
        do {
            let regex = try NSRegularExpression(pattern: pattern)
            let range = NSRange(text.startIndex..<text.endIndex, in: text)
            let match = regex.firstMatch(in: text, options: [], range: range)
            return match?.range == range ? nil : "\(field.title) does not match the expected format."
        } catch {
            return nil
        }
    }

    private func constValidationMessage() -> String? {
        guard let constValue = field.constValue,
              let inputValue = inputJSONValue() else { return nil }
        return canonicalJSONKey(for: inputValue) == canonicalJSONKey(for: constValue)
            ? nil
            : "\(field.title) must match the required constant value."
    }

    private func structuredValidationMessage() -> String? {
        guard let jsonContainer = field.jsonContainer else { return nil }
        guard let parsed = parseStructuredJSON(text) else {
            return "\(field.title) must be valid JSON."
        }
        guard jsonContainer.matches(parsed) else {
            return "\(field.title) must be a JSON \(jsonContainer.label)."
        }

        switch parsed {
        case .array(let values):
            if let minItems = field.minItems, values.count < minItems {
                return "\(field.title) must have at least \(minItems) item\(minItems == 1 ? "" : "s")."
            }
            if let maxItems = field.maxItems, values.count > maxItems {
                return "\(field.title) must have at most \(maxItems) item\(maxItems == 1 ? "" : "s")."
            }
            if let containsValidationMessage = containsValidationMessage(for: values) {
                return containsValidationMessage
            }
            if field.uniqueItems, !hasUniqueItems(values) {
                return "\(field.title) must not contain duplicate items."
            }
            if let itemValidationMessage = arrayItemValidationMessage(
                values: values,
                prefixConstraints: field.prefixItemConstraints,
                trailingConstraint: field.arrayItemConstraint
            ) {
                return itemValidationMessage
            }
        case .object(let properties):
            if let minProperties = field.minProperties, properties.count < minProperties {
                return "\(field.title) must have at least \(minProperties) field\(minProperties == 1 ? "" : "s")."
            }
            if let maxProperties = field.maxProperties, properties.count > maxProperties {
                return "\(field.title) must have at most \(maxProperties) field\(maxProperties == 1 ? "" : "s")."
            }
            if let propertyValidationMessage = objectPropertyValidationMessage(properties: properties) {
                return propertyValidationMessage
            }
        default:
            break
        }

        return nil
    }

    private func arrayItemValidationMessage(
        values: [AppJSONValue],
        prefixConstraints: [ArrayItemConstraint],
        trailingConstraint: ArrayItemConstraint?
    ) -> String? {
        for (index, value) in values.enumerated() {
            let constraint = index < prefixConstraints.count ? prefixConstraints[index] : trailingConstraint
            guard let constraint else { continue }
            if let message = constraint.validationMessage(
                for: value,
                path: "\(field.title) item \(index + 1)"
            ) {
                return message
            }
        }
        return nil
    }

    private func containsValidationMessage(for values: [AppJSONValue]) -> String? {
        guard let containsConstraint = field.containsConstraint else { return nil }
        let matchCount = values.filter {
            containsConstraint.validationMessage(for: $0, path: field.title) == nil
        }.count
        let minimumMatches = field.minContains ?? 1
        if matchCount < minimumMatches {
            return "\(field.title) must contain at least \(minimumMatches) matching item\(minimumMatches == 1 ? "" : "s")."
        }
        if let maxContains = field.maxContains, matchCount > maxContains {
            return "\(field.title) must contain at most \(maxContains) matching item\(maxContains == 1 ? "" : "s")."
        }
        return nil
    }

    private func objectPropertyValidationMessage(
        properties: [String: AppJSONValue]
    ) -> String? {
        let missingRequired = field.objectRequiredProperties
            .subtracting(properties.keys)
            .sorted()
        if !missingRequired.isEmpty {
            return "\(field.title) is missing required field\(missingRequired.count == 1 ? "" : "s"): \(missingRequired.joined(separator: ", "))."
        }
        if !field.allowsAdditionalProperties {
            let unknownProperties = Set(properties.keys)
                .subtracting(field.objectPropertyConstraints.keys)
                .sorted()
            if !unknownProperties.isEmpty {
                return "\(field.title) contains unsupported field\(unknownProperties.count == 1 ? "" : "s"): \(unknownProperties.joined(separator: ", "))."
            }
        }
        if let additionalPropertyConstraint = field.additionalPropertyConstraint {
            let unknownProperties = Set(properties.keys)
                .subtracting(field.objectPropertyConstraints.keys)
                .sorted()
            for name in unknownProperties {
                guard let value = properties[name] else { continue }
                if let message = additionalPropertyConstraint.validationMessage(
                    for: value,
                    path: "\(field.title).\(name)"
                ) {
                    return message
                }
            }
        }

        for (name, constraint) in field.objectPropertyConstraints.sorted(by: { $0.key < $1.key }) {
            guard let value = properties[name] else { continue }
            if let message = constraint.validationMessage(
                for: value,
                path: "\(field.title).\(name)"
            ) {
                return message
            }
        }

        return nil
    }

    private func hasUniqueItems(_ values: [AppJSONValue]) -> Bool {
        var seen = Set<String>()
        for value in values {
            let key = canonicalJSONKey(for: value)
            if !seen.insert(key).inserted {
                return false
            }
        }
        return true
    }

    private func canonicalJSONKey(for value: AppJSONValue) -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        guard let data = try? encoder.encode(value),
              let string = String(data: data, encoding: .utf8) else {
            return String(describing: value)
        }
        return string
    }

    private func lengthValidationMessage() -> String? {
        if let minLength = field.minLength, text.count < minLength {
            return "\(field.title) must be at least \(minLength) characters."
        }
        if let maxLength = field.maxLength, text.count > maxLength {
            return "\(field.title) must be at most \(maxLength) characters."
        }
        return nil
    }

    private func formatValidationMessage() -> String? {
        switch field.kind {
        case .email:
            let parts = text.split(separator: "@", omittingEmptySubsequences: false)
            guard parts.count == 2,
                  !parts[0].isEmpty,
                  !parts[1].isEmpty,
                  parts[1].contains(".") else {
                return "\(field.title) must be a valid email address."
            }
            return nil
        case .url:
            guard let url = URL(string: text),
                  let scheme = url.scheme?.lowercased(),
                  (scheme == "http" || scheme == "https"),
                  url.host?.isEmpty == false else {
                return "\(field.title) must be a valid http or https URL."
            }
            return nil
        case .date:
            let formatter = DateFormatter()
            formatter.calendar = Calendar(identifier: .iso8601)
            formatter.locale = Locale(identifier: "en_US_POSIX")
            formatter.timeZone = TimeZone(secondsFromGMT: 0)
            formatter.dateFormat = "yyyy-MM-dd"
            return formatter.date(from: text) == nil ? "\(field.title) must use YYYY-MM-DD." : nil
        case .dateTime:
            let primary = ISO8601DateFormatter()
            primary.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if primary.date(from: text) != nil {
                return nil
            }
            let fallback = ISO8601DateFormatter()
            fallback.formatOptions = [.withInternetDateTime]
            return fallback.date(from: text) == nil ? "\(field.title) must be a valid ISO 8601 date-time." : nil
        case .boolean, .choice, .multiline, .text, .number, .integer:
            return nil
        }
    }

    private func numericTypeValidationMessage() -> String? {
        switch field.kind {
        case .number:
            return Double(text) == nil ? "\(field.title) must be a valid number." : nil
        case .integer:
            return Int(text) == nil ? "\(field.title) must be a whole number." : nil
        case .boolean, .choice, .multiline, .text, .email, .url, .date, .dateTime:
            return nil
        }
    }

    private func numericRangeValidationMessage() -> String? {
        switch field.kind {
        case .number, .integer:
            guard let number = Double(text) else { return nil }
            if let minimum = field.minimum, number < minimum {
                return "\(field.title) must be at least \(displayNumber(minimum))."
            }
            if let exclusiveMinimum = field.exclusiveMinimum, number <= exclusiveMinimum {
                return "\(field.title) must be greater than \(displayNumber(exclusiveMinimum))."
            }
            if let maximum = field.maximum, number > maximum {
                return "\(field.title) must be at most \(displayNumber(maximum))."
            }
            if let exclusiveMaximum = field.exclusiveMaximum, number >= exclusiveMaximum {
                return "\(field.title) must be less than \(displayNumber(exclusiveMaximum))."
            }
            if let multipleOf = field.multipleOf,
               !isMultiple(number, of: multipleOf) {
                return "\(field.title) must be a multiple of \(displayNumber(multipleOf))."
            }
            return nil
        case .boolean, .choice, .multiline, .text, .email, .url, .date, .dateTime:
            return nil
        }
    }

    private func isMultiple(_ number: Double, of divisor: Double) -> Bool {
        guard divisor != 0 else { return true }
        let quotient = number / divisor
        return abs(quotient.rounded() - quotient) < 0.000_000_1
    }

    private func displayNumber(_ value: Double) -> String {
        if value.rounded(.towardZero) == value {
            return String(Int(value))
        }
        return String(value)
    }
}

private final class ArrayItemConstraint {
    let typeNames: [String]
    let format: String?
    let options: [String]
    let constValue: AppJSONValue?
    let negatedConstraint: ArrayItemConstraint?
    let arrayItemConstraint: ArrayItemConstraint?
    let prefixItemConstraints: [ArrayItemConstraint]
    let containsConstraint: ArrayItemConstraint?
    let objectPropertyConstraints: [String: ObjectPropertyConstraint]
    let objectRequiredProperties: Set<String>
    let allowsAdditionalProperties: Bool
    let additionalPropertyConstraint: ArrayItemConstraint?
    let alternativeConstraints: [ArrayItemConstraint]
    let conjunctiveConstraints: [ArrayItemConstraint]
    let pattern: String?
    let minLength: Int?
    let maxLength: Int?
    let minItems: Int?
    let maxItems: Int?
    let minContains: Int?
    let maxContains: Int?
    let uniqueItems: Bool
    let minProperties: Int?
    let maxProperties: Int?
    let minimum: Double?
    let maximum: Double?
    let exclusiveMinimum: Double?
    let exclusiveMaximum: Double?
    let multipleOf: Double?

    init(
        typeNames: [String],
        format: String?,
        options: [String],
        constValue: AppJSONValue?,
        negatedConstraint: ArrayItemConstraint?,
        arrayItemConstraint: ArrayItemConstraint?,
        prefixItemConstraints: [ArrayItemConstraint],
        containsConstraint: ArrayItemConstraint?,
        objectPropertyConstraints: [String: ObjectPropertyConstraint],
        objectRequiredProperties: Set<String>,
        allowsAdditionalProperties: Bool,
        additionalPropertyConstraint: ArrayItemConstraint?,
        alternativeConstraints: [ArrayItemConstraint],
        conjunctiveConstraints: [ArrayItemConstraint],
        pattern: String?,
        minLength: Int?,
        maxLength: Int?,
        minItems: Int?,
        maxItems: Int?,
        minContains: Int?,
        maxContains: Int?,
        uniqueItems: Bool,
        minProperties: Int?,
        maxProperties: Int?,
        minimum: Double?,
        maximum: Double?,
        exclusiveMinimum: Double?,
        exclusiveMaximum: Double?,
        multipleOf: Double?
    ) {
        self.typeNames = typeNames
        self.format = format
        self.options = options
        self.constValue = constValue
        self.negatedConstraint = negatedConstraint
        self.arrayItemConstraint = arrayItemConstraint
        self.prefixItemConstraints = prefixItemConstraints
        self.containsConstraint = containsConstraint
        self.objectPropertyConstraints = objectPropertyConstraints
        self.objectRequiredProperties = objectRequiredProperties
        self.allowsAdditionalProperties = allowsAdditionalProperties
        self.additionalPropertyConstraint = additionalPropertyConstraint
        self.alternativeConstraints = alternativeConstraints
        self.conjunctiveConstraints = conjunctiveConstraints
        self.pattern = pattern
        self.minLength = minLength
        self.maxLength = maxLength
        self.minItems = minItems
        self.maxItems = maxItems
        self.minContains = minContains
        self.maxContains = maxContains
        self.uniqueItems = uniqueItems
        self.minProperties = minProperties
        self.maxProperties = maxProperties
        self.minimum = minimum
        self.maximum = maximum
        self.exclusiveMinimum = exclusiveMinimum
        self.exclusiveMaximum = exclusiveMaximum
        self.multipleOf = multipleOf
    }

    func matches(_ value: AppJSONValue) -> Bool {
        validationMessage(for: value, path: "value") == nil
    }

    func validationMessage(for value: AppJSONValue, path: String) -> String? {
        if !alternativeConstraints.isEmpty {
            let matchesAlternative = alternativeConstraints.contains { constraint in
                constraint.validationMessage(for: value, path: path) == nil
            }
            if matchesAlternative {
                return nil
            }
            return "\(path) must match one of the allowed schema alternatives."
        }
        for constraint in conjunctiveConstraints {
            if let message = constraint.validationMessage(for: value, path: path) {
                return message
            }
        }
        if let negatedConstraint,
           negatedConstraint.validationMessage(for: value, path: path) == nil {
            return "\(path) must not match the disallowed schema."
        }

        if !options.isEmpty {
            guard case .string(let string) = value else { return "\(path) must be \(expectedDescription)." }
            return options.contains(string) ? nil : "\(path) must be \(expectedDescription)."
        }
        if let constValue,
           canonicalJSONKey(for: value) != canonicalJSONKey(for: constValue) {
            return "\(path) must match the required constant value."
        }

        if let message = patternValidationMessage(for: value, path: path)
            ?? lengthValidationMessage(for: value, path: path)
            ?? numericRangeValidationMessage(for: value, path: path) {
            return message
        }

        if typeNames.isEmpty {
            return nestedValidationMessage(for: value, path: path)
        }

        if case .null = value {
            return typeNames.contains("null") ? nil : "\(path) must be \(expectedDescription)."
        }

        for typeName in typeNames {
            switch typeName {
            case "string":
                if stringMatches(value) { return nestedValidationMessage(for: value, path: path) }
            case "number":
                if case .number = value { return nestedValidationMessage(for: value, path: path) }
            case "integer":
                if integerMatches(value) { return nestedValidationMessage(for: value, path: path) }
            case "boolean":
                if case .bool = value { return nestedValidationMessage(for: value, path: path) }
            case "object":
                if case .object = value { return nestedValidationMessage(for: value, path: path) }
            case "array":
                if case .array = value { return nestedValidationMessage(for: value, path: path) }
            default:
                break
            }
        }

        return "\(path) must be \(expectedDescription)."
    }

    var expectedDescription: String {
        if !options.isEmpty {
            return "one of: \(options.joined(separator: ", "))"
        }

        let typeDescription = typeNames
            .map(typeLabel(for:))
            .joined(separator: " or ")

        guard let format else { return typeDescription.isEmpty ? "a valid value" : typeDescription }

        switch format {
        case "email":
            return "a valid email address"
        case "uri", "url":
            return "a valid http or https URL"
        case "date":
            return "a date in YYYY-MM-DD format"
        case "date-time", "datetime":
            return "a valid ISO 8601 date-time"
        default:
            return typeDescription.isEmpty ? "a valid value" : typeDescription
        }
    }

    private func stringMatches(_ value: AppJSONValue) -> Bool {
        guard case .string(let string) = value else { return false }

        switch format {
        case "email":
            let parts = string.split(separator: "@", omittingEmptySubsequences: false)
            return parts.count == 2 && !parts[0].isEmpty && !parts[1].isEmpty && parts[1].contains(".")
        case "uri", "url":
            guard let url = URL(string: string),
                  let scheme = url.scheme?.lowercased(),
                  (scheme == "http" || scheme == "https"),
                  url.host?.isEmpty == false else {
                return false
            }
            return true
        case "date":
            let formatter = DateFormatter()
            formatter.calendar = Calendar(identifier: .iso8601)
            formatter.locale = Locale(identifier: "en_US_POSIX")
            formatter.timeZone = TimeZone(secondsFromGMT: 0)
            formatter.dateFormat = "yyyy-MM-dd"
            return formatter.date(from: string) != nil
        case "date-time", "datetime":
            let primary = ISO8601DateFormatter()
            primary.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if primary.date(from: string) != nil {
                return true
            }
            let fallback = ISO8601DateFormatter()
            fallback.formatOptions = [.withInternetDateTime]
            return fallback.date(from: string) != nil
        default:
            return true
        }
    }

    private func integerMatches(_ value: AppJSONValue) -> Bool {
        guard case .number(let number) = value else { return false }
        return number.rounded(.towardZero) == number
    }

    private func typeLabel(for typeName: String) -> String {
        switch typeName {
        case "string":
            return "a string"
        case "number":
            return "a number"
        case "integer":
            return "a whole number"
        case "boolean":
            return "a boolean"
        case "object":
            return "a JSON object"
        case "array":
            return "a JSON array"
        case "null":
            return "null"
        default:
            return typeName
        }
    }

    private func patternValidationMessage(for value: AppJSONValue, path: String) -> String? {
        guard let pattern, !pattern.isEmpty, case .string(let string) = value else { return nil }
        do {
            let regex = try NSRegularExpression(pattern: pattern)
            let range = NSRange(string.startIndex..<string.endIndex, in: string)
            let match = regex.firstMatch(in: string, options: [], range: range)
            return match?.range == range ? nil : "\(path) does not match the expected format."
        } catch {
            return nil
        }
    }

    private func lengthValidationMessage(for value: AppJSONValue, path: String) -> String? {
        guard case .string(let string) = value else { return nil }
        if let minLength, string.count < minLength {
            return "\(path) must be at least \(minLength) characters."
        }
        if let maxLength, string.count > maxLength {
            return "\(path) must be at most \(maxLength) characters."
        }
        return nil
    }

    private func numericRangeValidationMessage(for value: AppJSONValue, path: String) -> String? {
        let number: Double
        switch value {
        case .number(let parsed):
            number = parsed
        default:
            return nil
        }
        if let minimum, number < minimum {
            return "\(path) must be at least \(displayNumber(minimum))."
        }
        if let exclusiveMinimum, number <= exclusiveMinimum {
            return "\(path) must be greater than \(displayNumber(exclusiveMinimum))."
        }
        if let maximum, number > maximum {
            return "\(path) must be at most \(displayNumber(maximum))."
        }
        if let exclusiveMaximum, number >= exclusiveMaximum {
            return "\(path) must be less than \(displayNumber(exclusiveMaximum))."
        }
        if let multipleOf, !isMultiple(number, of: multipleOf) {
            return "\(path) must be a multiple of \(displayNumber(multipleOf))."
        }
        return nil
    }

    private func isMultiple(_ number: Double, of divisor: Double) -> Bool {
        guard divisor != 0 else { return true }
        let quotient = number / divisor
        return abs(quotient.rounded() - quotient) < 0.000_000_1
    }

    private func displayNumber(_ value: Double) -> String {
        if value.rounded(.towardZero) == value {
            return String(Int(value))
        }
        return String(value)
    }

    private func hasUniqueItems(_ values: [AppJSONValue]) -> Bool {
        var seen = Set<String>()
        for value in values {
            let key = canonicalJSONKey(for: value)
            if !seen.insert(key).inserted {
                return false
            }
        }
        return true
    }

    private func canonicalJSONKey(for value: AppJSONValue) -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        guard let data = try? encoder.encode(value),
              let string = String(data: data, encoding: .utf8) else {
            return String(describing: value)
        }
        return string
    }

    private func nestedValidationMessage(for value: AppJSONValue, path: String) -> String? {
        switch value {
        case .array(let values):
            if let minItems, values.count < minItems {
                return "\(path) must have at least \(minItems) item\(minItems == 1 ? "" : "s")."
            }
            if let maxItems, values.count > maxItems {
                return "\(path) must have at most \(maxItems) item\(maxItems == 1 ? "" : "s")."
            }
            if let containsConstraint {
                let matchCount = values.filter {
                    containsConstraint.validationMessage(for: $0, path: path) == nil
                }.count
                let minimumMatches = minContains ?? 1
                if matchCount < minimumMatches {
                    return "\(path) must contain at least \(minimumMatches) matching item\(minimumMatches == 1 ? "" : "s")."
                }
                if let maxContains, matchCount > maxContains {
                    return "\(path) must contain at most \(maxContains) matching item\(maxContains == 1 ? "" : "s")."
                }
            }
            if uniqueItems, !hasUniqueItems(values) {
                return "\(path) must not contain duplicate items."
            }
            for (index, item) in values.enumerated() {
                let constraint = index < prefixItemConstraints.count ? prefixItemConstraints[index] : arrayItemConstraint
                guard let constraint else { continue }
                if let message = constraint.validationMessage(
                    for: item,
                    path: "\(path) item \(index + 1)"
                ) {
                    return message
                }
            }
            return nil
        case .object(let properties):
            if let minProperties, properties.count < minProperties {
                return "\(path) must have at least \(minProperties) field\(minProperties == 1 ? "" : "s")."
            }
            if let maxProperties, properties.count > maxProperties {
                return "\(path) must have at most \(maxProperties) field\(maxProperties == 1 ? "" : "s")."
            }
            let missingRequired = objectRequiredProperties
                .subtracting(properties.keys)
                .sorted()
            if !missingRequired.isEmpty {
                return "\(path) is missing required field\(missingRequired.count == 1 ? "" : "s"): \(missingRequired.joined(separator: ", "))."
            }
            if !allowsAdditionalProperties {
                let unknownProperties = Set(properties.keys)
                    .subtracting(objectPropertyConstraints.keys)
                    .sorted()
                if !unknownProperties.isEmpty {
                    return "\(path) contains unsupported field\(unknownProperties.count == 1 ? "" : "s"): \(unknownProperties.joined(separator: ", "))."
                }
            }
            if let additionalPropertyConstraint {
                let unknownProperties = Set(properties.keys)
                    .subtracting(objectPropertyConstraints.keys)
                    .sorted()
                for name in unknownProperties {
                    guard let propertyValue = properties[name] else { continue }
                    if let message = additionalPropertyConstraint.validationMessage(
                        for: propertyValue,
                        path: "\(path).\(name)"
                    ) {
                        return message
                    }
                }
            }
            for (name, constraint) in objectPropertyConstraints.sorted(by: { $0.key < $1.key }) {
                guard let propertyValue = properties[name] else { continue }
                if let message = constraint.validationMessage(
                    for: propertyValue,
                    path: "\(path).\(name)"
                ) {
                    return message
                }
            }
            return nil
        default:
            return nil
        }
    }
}

private final class ObjectPropertyConstraint {
    let name: String
    let typeNames: [String]
    let format: String?
    let options: [String]
    let constValue: AppJSONValue?
    let negatedConstraint: ArrayItemConstraint?
    let arrayItemConstraint: ArrayItemConstraint?
    let prefixItemConstraints: [ArrayItemConstraint]
    let containsConstraint: ArrayItemConstraint?
    let objectPropertyConstraints: [String: ObjectPropertyConstraint]
    let objectRequiredProperties: Set<String>
    let allowsAdditionalProperties: Bool
    let additionalPropertyConstraint: ArrayItemConstraint?
    let alternativeConstraints: [ArrayItemConstraint]
    let conjunctiveConstraints: [ArrayItemConstraint]
    let pattern: String?
    let minLength: Int?
    let maxLength: Int?
    let minItems: Int?
    let maxItems: Int?
    let minContains: Int?
    let maxContains: Int?
    let uniqueItems: Bool
    let minProperties: Int?
    let maxProperties: Int?
    let minimum: Double?
    let maximum: Double?
    let exclusiveMinimum: Double?
    let exclusiveMaximum: Double?
    let multipleOf: Double?

    init(
        name: String,
        typeNames: [String],
        format: String?,
        options: [String],
        constValue: AppJSONValue?,
        negatedConstraint: ArrayItemConstraint?,
        arrayItemConstraint: ArrayItemConstraint?,
        prefixItemConstraints: [ArrayItemConstraint],
        containsConstraint: ArrayItemConstraint?,
        objectPropertyConstraints: [String: ObjectPropertyConstraint],
        objectRequiredProperties: Set<String>,
        allowsAdditionalProperties: Bool,
        additionalPropertyConstraint: ArrayItemConstraint?,
        alternativeConstraints: [ArrayItemConstraint],
        conjunctiveConstraints: [ArrayItemConstraint],
        pattern: String?,
        minLength: Int?,
        maxLength: Int?,
        minItems: Int?,
        maxItems: Int?,
        minContains: Int?,
        maxContains: Int?,
        uniqueItems: Bool,
        minProperties: Int?,
        maxProperties: Int?,
        minimum: Double?,
        maximum: Double?,
        exclusiveMinimum: Double?,
        exclusiveMaximum: Double?,
        multipleOf: Double?
    ) {
        self.name = name
        self.typeNames = typeNames
        self.format = format
        self.options = options
        self.constValue = constValue
        self.negatedConstraint = negatedConstraint
        self.arrayItemConstraint = arrayItemConstraint
        self.prefixItemConstraints = prefixItemConstraints
        self.containsConstraint = containsConstraint
        self.objectPropertyConstraints = objectPropertyConstraints
        self.objectRequiredProperties = objectRequiredProperties
        self.allowsAdditionalProperties = allowsAdditionalProperties
        self.additionalPropertyConstraint = additionalPropertyConstraint
        self.alternativeConstraints = alternativeConstraints
        self.conjunctiveConstraints = conjunctiveConstraints
        self.pattern = pattern
        self.minLength = minLength
        self.maxLength = maxLength
        self.minItems = minItems
        self.maxItems = maxItems
        self.minContains = minContains
        self.maxContains = maxContains
        self.uniqueItems = uniqueItems
        self.minProperties = minProperties
        self.maxProperties = maxProperties
        self.minimum = minimum
        self.maximum = maximum
        self.exclusiveMinimum = exclusiveMinimum
        self.exclusiveMaximum = exclusiveMaximum
        self.multipleOf = multipleOf
    }

    func matches(_ value: AppJSONValue) -> Bool {
        ArrayItemConstraint(
            typeNames: typeNames,
            format: format,
            options: options,
            constValue: constValue,
            negatedConstraint: negatedConstraint,
            arrayItemConstraint: arrayItemConstraint,
            prefixItemConstraints: prefixItemConstraints,
            containsConstraint: containsConstraint,
            objectPropertyConstraints: objectPropertyConstraints,
            objectRequiredProperties: objectRequiredProperties,
            allowsAdditionalProperties: allowsAdditionalProperties,
            additionalPropertyConstraint: additionalPropertyConstraint,
            alternativeConstraints: alternativeConstraints,
            conjunctiveConstraints: conjunctiveConstraints,
            pattern: pattern,
            minLength: minLength,
            maxLength: maxLength,
            minItems: minItems,
            maxItems: maxItems,
            minContains: minContains,
            maxContains: maxContains,
            uniqueItems: uniqueItems,
            minProperties: minProperties,
            maxProperties: maxProperties,
            minimum: minimum,
            maximum: maximum,
            exclusiveMinimum: exclusiveMinimum,
            exclusiveMaximum: exclusiveMaximum,
            multipleOf: multipleOf
        ).matches(value)
    }

    var expectedDescription: String {
        ArrayItemConstraint(
            typeNames: typeNames,
            format: format,
            options: options,
            constValue: constValue,
            negatedConstraint: negatedConstraint,
            arrayItemConstraint: arrayItemConstraint,
            prefixItemConstraints: prefixItemConstraints,
            containsConstraint: containsConstraint,
            objectPropertyConstraints: objectPropertyConstraints,
            objectRequiredProperties: objectRequiredProperties,
            allowsAdditionalProperties: allowsAdditionalProperties,
            additionalPropertyConstraint: additionalPropertyConstraint,
            alternativeConstraints: alternativeConstraints,
            conjunctiveConstraints: conjunctiveConstraints,
            pattern: pattern,
            minLength: minLength,
            maxLength: maxLength,
            minItems: minItems,
            maxItems: maxItems,
            minContains: minContains,
            maxContains: maxContains,
            uniqueItems: uniqueItems,
            minProperties: minProperties,
            maxProperties: maxProperties,
            minimum: minimum,
            maximum: maximum,
            exclusiveMinimum: exclusiveMinimum,
            exclusiveMaximum: exclusiveMaximum,
            multipleOf: multipleOf
        ).expectedDescription
    }

    func validationMessage(for value: AppJSONValue, path: String) -> String? {
        ArrayItemConstraint(
            typeNames: typeNames,
            format: format,
            options: options,
            constValue: constValue,
            negatedConstraint: negatedConstraint,
            arrayItemConstraint: arrayItemConstraint,
            prefixItemConstraints: prefixItemConstraints,
            containsConstraint: containsConstraint,
            objectPropertyConstraints: objectPropertyConstraints,
            objectRequiredProperties: objectRequiredProperties,
            allowsAdditionalProperties: allowsAdditionalProperties,
            additionalPropertyConstraint: additionalPropertyConstraint,
            alternativeConstraints: alternativeConstraints,
            conjunctiveConstraints: conjunctiveConstraints,
            pattern: pattern,
            minLength: minLength,
            maxLength: maxLength,
            minItems: minItems,
            maxItems: maxItems,
            minContains: minContains,
            maxContains: maxContains,
            uniqueItems: uniqueItems,
            minProperties: minProperties,
            maxProperties: maxProperties,
            minimum: minimum,
            maximum: maximum,
            exclusiveMinimum: exclusiveMinimum,
            exclusiveMaximum: exclusiveMaximum,
            multipleOf: multipleOf
        ).validationMessage(for: value, path: path)
    }
}

private enum SchemaFieldKind {
    case text
    case multiline
    case choice
    case boolean
    case number
    case integer
    case email
    case url
    case date
    case dateTime
}

// MARK: - Forge-driven elicitation form

/// Renders a schema-based elicitation form using SchemaBasedFormRenderer.
/// Reports payload changes upward so the parent can collect values on Submit.
private struct ElicitationForgeForm: View {
    let schema: ForgeJSONValue
    let elicitationID: String
    let forgeRuntime: ForgeRuntime
    let onPayloadChange: ([String: AppJSONValue]) -> Void

    var body: some View {
        let formDef = SchemaBasedFormDef(
            id: "elicitationForm",
            dataSourceRef: "elicitationForm",
            schema: schema,
            showSubmit: false
        )
        let container = ContainerDef(
            id: "elicitationForm",
            dataSourceRef: "elicitationForm",
            schemaBasedForm: formDef
        )
        // onChange fires on every field edit so forgeFormPayload stays current.
        // onSubmit is wired too in case showSubmit is ever enabled upstream.
        SchemaBasedFormRenderer(
            container: container,
            onChange: { values in
                onPayloadChange(values.mapValues { $0.appValue })
            },
            onSubmit: { values in
                onPayloadChange(values.mapValues { $0.appValue })
            }
        )
    }
}

private enum JSONContainerKind {
    case object
    case array

    var label: String {
        switch self {
        case .object:
            return "object"
        case .array:
            return "array"
        }
    }

    func matches(_ value: AppJSONValue) -> Bool {
        switch (self, value) {
        case (.object, .object), (.array, .array):
            return true
        default:
            return false
        }
    }
}
