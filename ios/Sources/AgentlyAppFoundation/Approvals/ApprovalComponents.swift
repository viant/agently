import SwiftUI
import AgentlySDK
import ForgeIOSRuntime
import ForgeIOSUI

let approvalReservedFormKeys: Set<String> = [
    "approvalSchemaJSON",
    "approval",
    "originalArgs",
    "editedFields"
]

// MARK: - Approval list

public struct ApprovalListView: View {
    let approvals: [PendingToolApproval]
    let decidingApprovalID: String?
    let forgeRuntime: ForgeRuntime?
    /// Carries the approval, the action string, and any edited field values.
    let onDecision: (PendingToolApproval, String, [String: AppJSONValue]) -> Void

    public init(
        approvals: [PendingToolApproval],
        decidingApprovalID: String? = nil,
        forgeRuntime: ForgeRuntime? = nil,
        onDecision: @escaping (PendingToolApproval, String, [String: AppJSONValue]) -> Void
    ) {
        self.approvals = approvals
        self.decidingApprovalID = decidingApprovalID
        self.forgeRuntime = forgeRuntime
        self.onDecision = onDecision
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            ForEach(approvals) { approval in
                ApprovalCard(
                    approval: approval,
                    isSubmitting: decidingApprovalID == approval.id,
                    forgeRuntime: forgeRuntime,
                    onDecision: { action, fields in
                        onDecision(approval, action, fields)
                    }
                )
            }
        }
    }
}

// MARK: - Single approval card

/// Owns per-approval state: edited fields and Forge window.
private struct ApprovalCard: View {
    let approval: PendingToolApproval
    let isSubmitting: Bool
    let forgeRuntime: ForgeRuntime?
    let onDecision: (String, [String: AppJSONValue]) -> Void

    @State private var editedFields: [String: AppJSONValue] = [:]

    var body: some View {
        let meta = parsedApprovalMeta(approval)
        VStack(alignment: .leading, spacing: 12) {
            // Header
            VStack(alignment: .leading, spacing: 4) {
                Text(trimmedApprovalText(meta?.title) ?? trimmedApprovalText(approval.title) ?? humanizeApprovalKey(approval.toolName))
                    .font(.headline)
                Text(trimmedApprovalText(meta?.message) ?? "Review the proposed change before approving it.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }

            if isSubmitting {
                Label("Submitting...", systemImage: "hourglass")
                    .font(.footnote).foregroundStyle(.secondary)
            }

            // Editors — Forge-driven when runtime and editors are available,
            // raw JSON fallback otherwise.
            if let meta, let editors = meta.editors, !editors.isEmpty,
               let runtime = forgeRuntime {
                ApprovalForgeEditors(
                    approval: approval,
                    meta: meta,
                    editors: editors,
                    forgeRuntime: runtime,
                    onEditedFieldsChange: { editedFields = $0 }
                )
            } else {
                ApprovalArgumentsPreviewView(preview: buildApprovalArgumentsPreview(approval.arguments))
            }

            if let errorMessage = trimmedApprovalText(approval.errorMessage) {
                Text(approvalFailureMessage(errorMessage))
                    .font(.footnote)
                    .foregroundStyle(Color.red)
                    .padding(10)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color.red.opacity(0.08), in: RoundedRectangle(cornerRadius: 10))
            }

            // Keep the mobile decision surface focused on the two user outcomes.
            let acceptLabel = meta?.acceptLabel ?? "Approve"
            let rejectLabel = meta?.rejectLabel ?? "Reject"
            HStack(spacing: 10) {
                Button(acceptLabel) { onDecision("approve", editedFields) }
                    .disabled(isSubmitting)
                    .buttonStyle(.borderedProminent)
                Button(rejectLabel) { onDecision("reject", editedFields) }
                    .disabled(isSubmitting)
                    .buttonStyle(.bordered)
            }
        }
        .padding()
        .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
    }
}

struct ApprovalPreviewField: Equatable {
    let label: String
    let value: String
}

struct ApprovalArgumentsPreview: Equatable {
    var summary: [ApprovalPreviewField] = []
    var rowsLabel: String = "Items"
    var rows: [[ApprovalPreviewField]] = []
    var totalRows: Int = 0
}

func buildApprovalArgumentsPreview(_ arguments: AppJSONValue?) -> ApprovalArgumentsPreview {
    guard case .object(let root) = arguments else { return ApprovalArgumentsPreview() }
    let collection = root.first { _, value in
        guard case .array(let values) = value else { return false }
        return values.contains { if case .object = $0 { return true }; return false }
    }
    let rowValues: [AppJSONValue]
    if let collection, case .array(let values) = collection.value { rowValues = values } else { rowValues = [] }
    let rowsContainRationale = rowValues.contains { value in
        guard case .object(let row) = value else { return false }
        return row.keys.contains { $0.caseInsensitiveCompare("rationale") == .orderedSame }
    }

    var summary: [ApprovalPreviewField] = root.compactMap { key, value in
        guard !isApprovalTechnicalField(key), let display = approvalPreviewValue(value) else { return nil }
        return ApprovalPreviewField(label: humanizeApprovalKey(key), value: display)
    }
    if let nested = root.values.compactMap({ value -> [String: AppJSONValue]? in
        if case .object(let object) = value { return object }
        return nil
    }).first {
        for (key, value) in nested where summary.count < 6 && !isApprovalTechnicalField(key) {
            if let display = approvalPreviewValue(value) {
                summary.append(ApprovalPreviewField(label: humanizeApprovalKey(key), value: display))
            }
        }
    }
    if rowsContainRationale {
        summary.removeAll { $0.label.caseInsensitiveCompare("Change Reason") == .orderedSame }
    }
    summary = Array(summary.prefix(6))
    let summaryLabels = Set(summary.map(\.label))
    let rows = rowValues.compactMap { value -> [ApprovalPreviewField]? in
        guard case .object(let row) = value else { return nil }
        return Array(row.compactMap { key, value in
            let label = humanizeApprovalKey(key)
            guard !isApprovalTechnicalField(key), !summaryLabels.contains(label), let display = approvalPreviewValue(value) else { return nil }
            return ApprovalPreviewField(label: label, value: display)
        }.prefix(6))
    }
    let rowsLabel = collection.map { humanizeApprovalKey($0.key) } ?? "Items"
    return ApprovalArgumentsPreview(
        summary: summary,
        rowsLabel: rowsLabel,
        rows: Array(rows.prefix(6)),
        totalRows: rowValues.count
    )
}

private struct ApprovalArgumentsPreviewView: View {
    let preview: ApprovalArgumentsPreview

    var body: some View {
        if !preview.summary.isEmpty || !preview.rows.isEmpty {
            VStack(alignment: .leading, spacing: 12) {
                ForEach(Array(preview.summary.enumerated()), id: \.offset) { _, field in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(field.label).font(.caption).foregroundStyle(.secondary)
                        Text(field.value).font(.subheadline)
                    }
                }
                if !preview.rows.isEmpty {
                    Text("\(preview.rowsLabel) (\(preview.totalRows))")
                        .font(.subheadline.weight(.semibold))
                    ScrollView(.horizontal, showsIndicators: true) {
                        let columns = Array(Set(preview.rows.flatMap { $0.map(\.label) })).sorted()
                        VStack(alignment: .leading, spacing: 0) {
                            approvalPreviewRow(columns.map { ApprovalPreviewField(label: $0, value: $0) }, isHeader: true)
                            ForEach(Array(preview.rows.enumerated()), id: \.offset) { index, row in
                                let values = Dictionary(uniqueKeysWithValues: row.map { ($0.label, $0.value) })
                                approvalPreviewRow(columns.map { ApprovalPreviewField(label: $0, value: values[$0] ?? "") }, isHeader: false)
                                    .background(index.isMultiple(of: 2) ? Color.clear : Color.secondary.opacity(0.04))
                            }
                        }
                    }
                    if preview.totalRows > preview.rows.count {
                        Text("+\(preview.totalRows - preview.rows.count) more")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            .padding(12)
            .background(Color.secondary.opacity(0.05), in: RoundedRectangle(cornerRadius: 10))
        }
    }

    private func approvalPreviewRow(_ fields: [ApprovalPreviewField], isHeader: Bool) -> some View {
        HStack(spacing: 0) {
            ForEach(fields, id: \.label) { field in
                Text(field.value)
                    .font(isHeader ? .caption.weight(.semibold) : .caption)
                    .foregroundStyle(isHeader ? Color.primary : Color.secondary)
                    .lineLimit(3)
                    .frame(width: field.value.count > 48 ? 240 : 132, alignment: .leading)
                    .padding(8)
            }
        }
        .background(isHeader ? Color.blue.opacity(0.08) : Color.clear)
    }
}

func approvalFailureMessage(_ errorMessage: String?) -> String {
    let detail = errorMessage?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    if detail.localizedCaseInsensitiveContains("EOFException") || detail.caseInsensitiveCompare("EOF") == .orderedSame || detail.localizedCaseInsensitiveContains("unexpected end of stream") {
        return "The connection ended before the server confirmed approval. Your selection was kept; try again."
    }
    if detail.localizedCaseInsensitiveContains("status=403") || detail.localizedCaseInsensitiveContains("http 403") || detail.localizedCaseInsensitiveContains("forbidden") {
        return "The platform did not authorize this recommendation update (403). Your selection was kept; review access or retry."
    }
    if detail.hasPrefix("{") || detail.localizedCaseInsensitiveContains("body={") {
        return "The server rejected this approval. Your selection was kept; try again or review permissions."
    }
    let concise = detail.split(whereSeparator: \.isNewline).first.map(String.init)?.prefix(180) ?? ""
    return concise.isEmpty ? "Approval did not complete. Your selection was kept; try again." : "Approval did not complete: \(concise)"
}

private func approvalPreviewValue(_ value: AppJSONValue) -> String? {
    switch value {
    case .string(let value): return trimmedApprovalText(value)
    case .number(let value):
        return value.rounded() == value ? String(format: "%.0f", value) : value.description
    case .bool(let value): return value.description
    case .null, .array, .object: return nil
    }
}

private func trimmedApprovalText(_ value: String?) -> String? {
    let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    return trimmed.isEmpty ? nil : trimmed
}

private func isApprovalTechnicalField(_ key: String) -> Bool {
    let normalized = key.lowercased().replacingOccurrences(of: "_", with: "").replacingOccurrences(of: "-", with: "")
    return ["timeoutms", "selected", "id", "mode", "queuetitle", "selectordirection", "targetfield"].contains(normalized)
}

private func humanizeApprovalKey(_ key: String) -> String {
    key.replacingOccurrences(of: "([a-z0-9])([A-Z])", with: "$1 $2", options: .regularExpression)
        .replacingOccurrences(of: "_", with: " ")
        .replacingOccurrences(of: "-", with: " ")
        .split(whereSeparator: \.isWhitespace)
        .map { $0.lowercased().capitalized }
        .joined(separator: " ")
}

// MARK: - Forge-driven editor section

private struct ApprovalForgeEditors: View {
    let approval: PendingToolApproval
    let meta: ApprovalMeta
    let editors: [ApprovalEditor]
    let forgeRuntime: ForgeRuntime
    let onEditedFieldsChange: ([String: AppJSONValue]) -> Void

    @State private var windowState: ForgeRuntime.WindowState?

    var body: some View {
        Group {
            if let state = windowState,
               let metadata = state.metadata {
                ForEach(renderedContainers(from: metadata)) { container in
                    SchemaBasedFormRenderer(
                        container: container,
                        seedValues: buildApprovalEditorSeed(meta: meta, approval: approval, editors: editors),
                        onChange: { values in
                            let merged = buildApprovalEditorSeed(meta: meta, approval: approval, editors: editors).merging(values) { _, new in new }
                            onEditedFieldsChange(extractApprovalEditedFields(from: merged))
                        }
                    )
                }
            } else {
                // Loading placeholder
                ProgressView()
                    .frame(maxWidth: .infinity, alignment: .center)
                    .padding(.vertical, 8)
            }
        }
        .task(id: approval.id) {
            guard windowState == nil else { return }
            let state: ForgeRuntime.WindowState
            if let remoteWindowRef = remoteWindowRef {
                state = await forgeRuntime.openWindow(
                    key: remoteWindowRef,
                    title: meta.title ?? approval.toolName
                )
            } else {
                let metadata = buildApprovalWindow()
                state = await forgeRuntime.openWindowInline(
                    key: "approval-\(approval.id)",
                    title: meta.title ?? approval.toolName,
                    metadata: metadata
                )
            }
            // Seed initial values so form starts pre-filled
            await forgeRuntime.setFormValue(
                windowID: state.id,
                dataSourceRef: approvalDataSourceRef(meta),
                values: buildApprovalEditorSeed(meta: meta, approval: approval, editors: editors)
            )
            windowState = state
        }
        .onDisappear {
            if let id = windowState?.id {
                Task { await forgeRuntime.closeWindow(id: id) }
                windowState = nil
            }
        }
    }

    private var remoteWindowRef: String? {
        let trimmed = meta.forge?.windowRef?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return trimmed.isEmpty ? nil : trimmed
    }

    private func renderedContainers(from metadata: WindowMetadata) -> [ContainerDef] {
        let containers = metadata.view?.content?.containers ?? []
        guard let targetContainerRef = meta.forge?.containerRef?.trimmingCharacters(in: .whitespacesAndNewlines),
              !targetContainerRef.isEmpty else {
            return containers
        }
        let filtered = containers.filter { $0.id == targetContainerRef }
        return filtered.isEmpty ? containers : filtered
    }

    private func buildApprovalWindow() -> WindowMetadata {
        let schema = buildApprovalForgeSchema(editors: editors)
        let dataSourceRef = approvalDataSourceRef(meta)
        let formDef = SchemaBasedFormDef(
            id: dataSourceRef,
            dataSourceRef: dataSourceRef,
            schema: schema,
            showSubmit: false
        )
        let container = ContainerDef(
            id: {
                let trimmed = meta.forge?.containerRef?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
                return trimmed.isEmpty ? dataSourceRef : trimmed
            }(),
            title: meta.message,
            dataSourceRef: dataSourceRef,
            schemaBasedForm: formDef
        )
        return WindowMetadata(
            view: ViewDef(content: ContentDef(containers: [container])),
            dataSources: [dataSourceRef: DataSourceDef(uri: nil, method: nil)]
        )
    }

}

// MARK: - Helpers

func buildApprovalForgeSchema(editors: [ApprovalEditor]) -> ForgeJSONValue? {
    guard !editors.isEmpty else { return nil }
    var properties: [String: ForgeJSONValue] = [:]
    for editor in editors {
        let kind = editor.kind?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        let isRadio = kind == "radio_list" || kind == "radio"
        let isMultiSelect = !isRadio
        var prop: [String: ForgeJSONValue] = [
            "title": .string(editor.label ?? editor.name)
        ]
        if let desc = editor.description { prop["description"] = .string(desc) }
        if let options = editor.options, !options.isEmpty {
            let defaults = options.filter { $0.selected == true }.map { ForgeJSONValue.string($0.id) }
            prop["type"] = .string(isRadio ? "string" : "array")
            prop["enum"] = .array(options.map { .string($0.id) })
            prop["x-ui-widget"] = .string(isMultiSelect ? "multiSelect" : "radio")
            if isRadio {
                prop["default"] = defaults.first ?? .string("")
            } else {
                prop["default"] = .array(defaults)
            }
        } else {
            prop["type"] = .string("string")
        }
        properties[editor.name] = .object(prop)
    }
    return .object(["type": .string("object"), "properties": .object(properties)])
}

func buildApprovalEditorSeed(
    meta: ApprovalMeta,
    approval: PendingToolApproval,
    editors: [ApprovalEditor]
) -> [String: ForgeJSONValue] {
    var seed: [String: ForgeJSONValue] = [:]
    if let schema = buildApprovalForgeSchema(editors: editors),
       let schemaString = schema.jsonString {
        seed["approvalSchemaJSON"] = .string(schemaString)
    }
    if let approvalMetaValue = meta.appJSONValue?.forgeValue {
        seed["approval"] = approvalMetaValue
    }
    if let args = approval.arguments?.forgeValue {
        seed["originalArgs"] = args
    }
    var editedFields: [String: ForgeJSONValue] = [:]
    for editor in editors {
        if let defaultOpt = editor.options?.first(where: { $0.selected == true }) {
            seed[editor.name] = .string(defaultOpt.id)
            editedFields[editor.name] = .string(defaultOpt.id)
        }
    }
    if let args = approval.arguments, case .object(let obj) = args {
        let editorNames = Set(editors.map(\.name))
        for (k, v) in obj where editorNames.contains(k) {
            seed[k] = v.forgeValue
            editedFields[k] = v.forgeValue
        }
    }
    if !editedFields.isEmpty {
        seed["editedFields"] = .object(editedFields)
    }
    return seed
}

func extractApprovalEditedFields(from formState: [String: ForgeJSONValue]) -> [String: AppJSONValue] {
    if case .object(let explicitEditedFields)? = formState["editedFields"], !explicitEditedFields.isEmpty {
        let liveFields = formState.filter { key, _ in !approvalReservedFormKeys.contains(key) }
        if !liveFields.isEmpty {
            return liveFields.mapValues { $0.appValue }
        }
        return explicitEditedFields.mapValues { $0.appValue }
    }
    return formState
        .filter { key, _ in !approvalReservedFormKeys.contains(key) }
        .mapValues { $0.appValue }
}

private func parsedApprovalMeta(_ approval: PendingToolApproval) -> ApprovalMeta? {
    ApprovalMetadataSupport.parsedApprovalMeta(approval)
}

private func approvalDataSourceRef(_ meta: ApprovalMeta) -> String {
    ApprovalMetadataSupport.approvalDataSourceRef(meta)
}

extension ApprovalMeta {
    var appJSONValue: AppJSONValue? {
        guard let data = try? JSONEncoder.agently().encode(self),
              let decoded = try? JSONDecoder.agently().decode(AppJSONValue.self, from: data) else {
            return nil
        }
        return decoded
    }
}

extension ForgeJSONValue {
    var jsonString: String? {
        guard let data = try? JSONEncoder().encode(self),
              let string = String(data: data, encoding: .utf8) else {
            return nil
        }
        return string
    }
}

private struct ApprovalMetaRow: View {
    let label: String
    let value: String
    var body: some View {
        LabeledContent(label, value: value)
            .font(.caption).foregroundStyle(.secondary)
    }
}

private struct ApprovalJSONSection: View {
    let title: String
    let value: AppJSONValue
    @State private var isExpanded = true

    var body: some View {
        DisclosureGroup(isExpanded: $isExpanded) {
            ScrollView(.horizontal, showsIndicators: false) {
                Text(value.prettyPrinted)
                    .font(.system(.caption, design: .monospaced))
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(12)
                    .background(Color.secondary.opacity(0.06),
                                in: RoundedRectangle(cornerRadius: 10))
            }
            .padding(.top, 4)
        } label: {
            Text(title).font(.subheadline.weight(.semibold))
        }
    }
}

private extension AppJSONValue {
    var prettyPrinted: String {
        guard let object = try? jsonObject(),
              JSONSerialization.isValidJSONObject(object),
              let data = try? JSONSerialization.data(
                withJSONObject: object, options: [.prettyPrinted, .sortedKeys]),
              let string = String(data: data, encoding: .utf8) else {
            return rawScalarDescription
        }
        return string
    }

    func jsonObject() throws -> Any {
        let data = try JSONEncoder.agently().encode(self)
        return try JSONSerialization.jsonObject(with: data)
    }

    var rawScalarDescription: String {
        switch self {
        case .string(let v): return v
        case .number(let v): return v.description
        case .bool(let v): return v.description
        case .null: return "null"
        case .array, .object: return String(describing: self)
        }
    }
}
