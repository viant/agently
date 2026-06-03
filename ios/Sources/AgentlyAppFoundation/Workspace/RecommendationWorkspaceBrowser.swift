import SwiftUI
import AgentlySDK
import ForgeIOSRuntime
import ForgeIOSUI

struct HostedWorkspaceBrowser: View {
    let snapshot: WorkspaceWindowSnapshot
    let metadata: WindowMetadata
    let forgeRuntime: ForgeRuntime
    let windowContext: WindowContext

    @State private var rows: [[String: AppJSONValue]] = []
    @State private var dataInfo: [String: AppJSONValue] = [:]
    @State private var currentPage: Int = 1
    @State private var selectedRowID: String?
    @State private var isLoading = false
    @State private var isRollingBack = false
    @State private var errorMessage: String?
    @State private var reloadToken = 0

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if let errorMessage, rows.isEmpty {
                recommendationErrorState(errorMessage)
            } else {
                listHeader
                recommendationList
                paginationBar
            }
        }
        .task(id: taskKey) {
            await loadPage()
        }
    }

    private var taskKey: String {
        "\(snapshot.windowId):\(currentPage):\(reloadToken)"
    }

    private var primaryDataSourceRef: String? {
        if let tableRef = tableContainer?.dataSourceRef, !tableRef.isEmpty {
            return tableRef
        }
        if metadata.dataSources.keys.contains("recommendation") {
            return "recommendation"
        }
        if metadata.dataSources.keys.contains("recommendation_list") {
            return "recommendation_list"
        }
        return metadata.dataSources.keys.sorted().first
    }

    private var primaryDataSource: DataSourceDef? {
        guard let primaryDataSourceRef else { return nil }
        return metadata.dataSources[primaryDataSourceRef]
    }

    private var tableContainer: ContainerDef? {
        metadata.view?.content?.containers.first(where: { $0.table != nil })
    }

    private var detailRootContainer: ContainerDef? {
        metadata.view?.content?.containers.first(where: { !$0.containers.isEmpty && $0.id == "recommendationDetailPane" })
            ?? metadata.view?.content?.containers.first(where: { !$0.containers.isEmpty })
    }

    private var factItems: [ItemDef] {
        detailRootContainer?.containers
            .first(where: { $0.id == "recommendationFactsPane" || ($0.title ?? "").localizedCaseInsensitiveContains("detail") })?
            .items
            .filter { ($0.type ?? "label").localizedCaseInsensitiveCompare("button") != .orderedSame } ?? []
    }

    private var copyItems: [ItemDef] {
        detailRootContainer?.containers
            .first(where: { $0.id == "recommendationCopyPane" || ($0.title ?? "").localizedCaseInsensitiveContains("change") })?
            .items ?? []
    }

    private var rollbackItem: ItemDef? {
        detailRootContainer?.containers
            .flatMap(\.items)
            .first(where: { ($0.type ?? "").localizedCaseInsensitiveCompare("button") == .orderedSame && !($0.on.isEmpty) })
    }

    private var pageSize: Int {
        max(1, primaryDataSource?.paging?.size ?? 20)
    }

    private var pageParameterName: String {
        let configured = primaryDataSource?.paging?.parameters["page"]?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return configured.isEmpty ? "page" : configured
    }

    private var sizeParameterName: String {
        let configured = primaryDataSource?.paging?.parameters["size"]?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return configured.isEmpty ? "size" : configured
    }

    private var pageCount: Int {
        if let explicit = integerValue(forAnyOf: ["pageCount"], in: dataInfo), explicit > 0 {
            return explicit
        }
        if let total = integerValue(forAnyOf: ["totalCount", "recordCount"], in: dataInfo), total > 0 {
            return max(1, Int(ceil(Double(total) / Double(pageSize))))
        }
        return max(1, currentPage)
    }

    private var totalCount: Int? {
        integerValue(forAnyOf: ["totalCount", "recordCount"], in: dataInfo)
    }

    private var hasNextPage: Bool {
        if let hasMore = booleanValue(forAnyOf: ["hasMore"], in: dataInfo) {
            return hasMore
        }
        return currentPage < pageCount
    }

    private var selectedRow: [String: AppJSONValue]? {
        guard let selectedRowID else {
            return rows.first
        }
        return rows.first(where: { recommendationIdentity(for: $0) == selectedRowID }) ?? rows.first
    }

    @ViewBuilder
    private var listHeader: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(snapshot.windowTitle?.trimmingCharacters(in: .whitespacesAndNewlines).nilIfEmpty ?? "Records")
                .font(.title3.weight(.semibold))
            HStack(spacing: 8) {
                if let totalCount {
                    Text(itemCountLabel(totalCount))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                } else {
                    Text(itemCountLabel(rows.count))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                if isLoading {
                    ProgressView()
                        .controlSize(.small)
                }
            }
        }
        .padding(.horizontal, 2)
    }

    private var recommendationList: some View {
        ScrollView(.vertical, showsIndicators: true) {
            LazyVStack(alignment: .leading, spacing: 12) {
                if let errorMessage, !rows.isEmpty {
                    Text(errorMessage)
                        .font(.footnote)
                        .foregroundStyle(.red)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(12)
                        .background(Color.red.opacity(0.08), in: RoundedRectangle(cornerRadius: 14))
                }

                if rows.isEmpty {
                    Text(isLoading ? "Loading items…" : "No items found.")
                        .font(.body)
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(16)
                        .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 16))
                } else {
                    ForEach(Array(rows.enumerated()), id: \.offset) { _, row in
                        NavigationLink(destination: detailScreen(for: row)) {
                            RecommendationRowCard(
                                row: row,
                                isSelected: recommendationIdentity(for: row) == selectedRowID
                            )
                            .padding(14)
                            .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 18))
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
            .padding(.vertical, 4)
        }
    }

    private func detailScreen(for row: [String: AppJSONValue]) -> some View {
        RecommendationDetailScreen(
            row: row,
            factItems: factItems,
            copyItems: copyItems,
            rollbackTitle: rollbackItem?.properties["text"]?.stringValue
                ?? rollbackItem?.label
                ?? rollbackItem?.title
                ?? "Rollback",
            canRollback: canRollback(row: row),
            isRollingBack: isRollingBack,
            onAppear: {
                Task {
                    await selectRow(row)
                }
            },
            onRollback: canRollback(row: row) ? {
                Task {
                    await rollbackSelectedRecommendation(row)
                }
            } : nil
        )
    }

    @ViewBuilder
    private var paginationBar: some View {
        HStack(spacing: 12) {
            Button {
                currentPage = max(1, currentPage - 1)
            } label: {
                Image(systemName: "chevron.left")
                    .font(.footnote.weight(.semibold))
                    .frame(width: 28, height: 28)
            }
            .buttonStyle(.plain)
            .background(Color.secondary.opacity(0.10), in: Circle())
            .foregroundStyle(currentPage <= 1 || isLoading ? .tertiary : .primary)
            .disabled(isLoading || currentPage <= 1)

            Spacer(minLength: 0)

            Text(pageCount > 1 ? "Page \(currentPage) of \(pageCount)" : "Page 1 of 1")
                .font(.footnote.weight(.semibold))
                .foregroundStyle(.secondary)

            Spacer(minLength: 0)

            Button {
                currentPage += 1
            } label: {
                Image(systemName: "chevron.right")
                    .font(.footnote.weight(.semibold))
                    .frame(width: 28, height: 28)
            }
            .buttonStyle(.plain)
            .background(Color.secondary.opacity(0.10), in: Circle())
            .foregroundStyle(!hasNextPage || isLoading ? .tertiary : .primary)
            .disabled(isLoading || !hasNextPage)
        }
        .padding(.vertical, 4)
    }

    @ViewBuilder
    private func recommendationErrorState(_ message: String) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Workspace data is unavailable right now.")
                .font(.headline)
            Text(message)
                .font(.footnote)
                .foregroundStyle(.secondary)
            Button("Retry") {
                reloadToken += 1
            }
            .buttonStyle(.borderedProminent)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 16))
    }

    @MainActor
    private func loadPage() async {
        guard let primaryDataSourceRef,
              let primaryDataSource else {
            errorMessage = "Workspace datasource metadata is incomplete."
            return
        }

        isLoading = true
        errorMessage = nil

        var inputs = snapshot.parameters ?? [:]
        if primaryDataSource.paging?.enabled != false {
            inputs[pageParameterName] = .number(Double(currentPage))
            inputs[sizeParameterName] = .number(Double(pageSize))
        }
        await forgeRuntime.setDataSourceInputParameters(
            windowID: windowContext.windowID,
            dataSourceRef: primaryDataSourceRef,
            parameters: inputs.mapValues(\.forgeValue),
            fetch: false
        )
        await forgeRuntime.refreshDataSourceCollection(
            windowID: windowContext.windowID,
            dataSourceRef: primaryDataSourceRef
        )
        let preparedRows = await forgeRuntime.dataSourceCollection(
            windowID: windowContext.windowID,
            dataSourceRef: primaryDataSourceRef
        ).map { $0.mapValues(\.appValue) }
        rows = preparedRows
        dataInfo = await forgeRuntime.dataSourceMetrics(
            windowID: windowContext.windowID,
            dataSourceRef: primaryDataSourceRef
        ).mapValues(\.appValue)

        let preferredRow = preferredRow(for: preparedRows)
        if let preferredRow {
            selectedRowID = recommendationIdentity(for: preferredRow)
            await selectRow(preferredRow)
        } else {
            selectedRowID = nil
        }

        isLoading = false
    }

    private func preferredRow(for rows: [[String: AppJSONValue]]) -> [String: AppJSONValue]? {
        if let selectedRowID,
           let matching = rows.first(where: { recommendationIdentity(for: $0) == selectedRowID }) {
            return matching
        }

        if let pinned = snapshot.parameters?["recommendation_id"]?.stringOrNumberValue ?? snapshot.parameters?["recommendationId"]?.stringOrNumberValue,
           let matching = rows.first(where: { row in
               recommendationValue(in: row, for: "recommendation_id")?.stringOrNumberValue == pinned
           }) {
            return matching
        }

        return rows.first
    }

    private func selectRow(_ row: [String: AppJSONValue]) async {
        guard let primaryDataSourceRef else { return }
        let rowIndex = rows.firstIndex(where: { recommendationIdentity(for: $0) == recommendationIdentity(for: row) }) ?? 0
        selectedRowID = recommendationIdentity(for: row)
        await forgeRuntime.setDataSourceSelection(
            windowID: windowContext.windowID,
            dataSourceRef: primaryDataSourceRef,
            selected: row.mapValues(\.forgeValue),
            rowIndex: rowIndex
        )
    }

    private func rollbackSelectedRecommendation(_ row: [String: AppJSONValue]) async {
        guard let rollbackExecution = rollbackItem?.on.first,
              let primaryDataSourceRef else {
            return
        }

        isRollingBack = true
        errorMessage = nil
        await selectRow(row)
        _ = await forgeRuntime.execute(
            rollbackExecution,
            context: ExecutionContext(windowID: windowContext.windowID, dataSourceRef: primaryDataSourceRef)
        )
        isRollingBack = false
        reloadToken += 1
    }

    private func canRollback(row: [String: AppJSONValue]) -> Bool {
        rollbackItem != nil
            && recommendationValue(in: row, for: "apply_status")?.stringValue?.uppercased() == "APPLIED"
    }

    private func recommendationIdentity(for row: [String: AppJSONValue]) -> String {
        recommendationValue(in: row, for: "recommendation_id")?.stringOrNumberValue
            ?? row["id"]?.stringOrNumberValue
            ?? UUID().uuidString
    }

    private func integerValue(forAnyOf keys: [String], in payload: [String: AppJSONValue]) -> Int? {
        for key in keys {
            if let value = payload[key]?.intValue {
                return value
            }
        }
        return nil
    }

    private func itemCountLabel(_ count: Int) -> String {
        count == 1 ? "1 item" : "\(count) items"
    }

    private func booleanValue(forAnyOf keys: [String], in payload: [String: AppJSONValue]) -> Bool? {
        for key in keys {
            if let value = payload[key]?.boolValue {
                return value
            }
        }
        return nil
    }
}

private struct RecommendationRowCard: View {
    let row: [String: AppJSONValue]
    let isSelected: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .top, spacing: 8) {
                RecommendationStatusBadge(status: recommendationValue(in: row, for: "apply_status")?.stringValue ?? "UNKNOWN")
                Spacer(minLength: 8)
                Text("ID \(recommendationValue(in: row, for: "recommendation_id")?.stringOrNumberValue ?? "—")")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 4)
                    .background(Color.secondary.opacity(0.08), in: Capsule())
            }

            VStack(alignment: .leading, spacing: 8) {
                recommendationSummaryRow(
                    label: "Ad Order",
                    value: recommendationValue(in: row, for: "ad_order_id")?.stringOrNumberValue ?? "—"
                )
                recommendationSummaryRow(
                    label: "Audience",
                    value: recommendationValue(in: row, for: "audience_id")?.stringOrNumberValue ?? "—"
                )
                if let recommendedAt = recommendationValue(in: row, for: "recommended_at")?.displayText, !recommendedAt.isEmpty {
                    Divider()
                    Text("Recommended \(recommendedAt)")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 2)
        .contentShape(Rectangle())
        .overlay(alignment: .leading) {
            if isSelected {
                RoundedRectangle(cornerRadius: 2)
                    .fill(Color.accentColor)
                    .frame(width: 3)
                    .padding(.vertical, 10)
            }
        }
    }

    @ViewBuilder
    private func recommendationSummaryRow(label: String, value: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Text(label)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(.secondary)
            Spacer(minLength: 8)
            Text(value)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(.primary)
                .multilineTextAlignment(.trailing)
        }
    }
}

private struct RecommendationStatusBadge: View {
    let status: String

    var body: some View {
        Text(status.replacingOccurrences(of: "_", with: " "))
            .font(.caption.weight(.semibold))
            .foregroundStyle(foregroundColor)
            .padding(.horizontal, 10)
            .padding(.vertical, 5)
            .background(backgroundColor, in: Capsule())
    }

    private var normalized: String {
        status.trimmingCharacters(in: .whitespacesAndNewlines).uppercased()
    }

    private var foregroundColor: Color {
        switch normalized {
        case "APPLIED":
            return Color.green.opacity(0.9)
        case "APPROVED":
            return Color.orange.opacity(0.9)
        case "REJECT", "REJECTED":
            return Color.red.opacity(0.9)
        default:
            return Color.blue.opacity(0.9)
        }
    }

    private var backgroundColor: Color {
        switch normalized {
        case "APPLIED":
            return Color.green.opacity(0.12)
        case "APPROVED":
            return Color.orange.opacity(0.14)
        case "REJECT", "REJECTED":
            return Color.red.opacity(0.12)
        default:
            return Color.blue.opacity(0.10)
        }
    }
}

private struct RecommendationDetailScreen: View {
    let row: [String: AppJSONValue]
    let factItems: [ItemDef]
    let copyItems: [ItemDef]
    let rollbackTitle: String
    let canRollback: Bool
    let isRollingBack: Bool
    let onAppear: () -> Void
    let onRollback: (() -> Void)?

    @State private var showsRollbackConfirmation = false

    var body: some View {
        recommendationDetailBody
        .navigationTitle(recommendationValue(in: row, for: "recommendation_id")?.stringOrNumberValue.map { "Recommendation \($0)" } ?? "Recommendation")
        .task {
            onAppear()
        }
        .alert("Rollback recommendation?", isPresented: $showsRollbackConfirmation) {
            Button("Cancel", role: .cancel) {}
            if let onRollback {
                Button(rollbackTitle, role: .destructive) {
                    onRollback()
                }
            }
        } message: {
            Text("This will request a rollback for the selected recommendation.")
        }
    }

    @ViewBuilder
    private var recommendationDetailBody: some View {
        #if os(iOS)
        List {
            recommendationDetailContent
        }
        .listStyle(.insetGrouped)
        .navigationBarTitleDisplayMode(.inline)
        #else
        List {
            recommendationDetailContent
        }
        .listStyle(.automatic)
        #endif
    }

    @ViewBuilder
    private var recommendationDetailContent: some View {
        Section("Summary") {
            RecommendationStatusBadge(status: recommendationValue(in: row, for: "apply_status")?.stringValue ?? "UNKNOWN")

            ForEach(visibleFactItems, id: \.id) { item in
                if let key = item.dataKey,
                   let value = recommendationValue(in: row, for: key)?.displayText,
                   !value.isEmpty {
                    LabeledContent(item.displayLabel, value: value)
                }
            }
        }

        if !markdownSections.isEmpty {
            ForEach(Array(markdownSections.enumerated()), id: \.offset) { _, section in
                Section(section.title) {
                    MarkdownRenderer(markdown: section.body)
                        .padding(.vertical, 4)
                }
            }
        }

        if canRollback {
            Section {
                Button(role: .destructive) {
                    showsRollbackConfirmation = true
                } label: {
                    if isRollingBack {
                        HStack(spacing: 8) {
                            ProgressView()
                                .controlSize(.small)
                            Text("Processing…")
                        }
                    } else {
                        Text(rollbackTitle)
                    }
                }
                .disabled(isRollingBack)
            }
        }
    }

    private var visibleFactItems: [ItemDef] {
        factItems.filter { item in
            guard let key = item.dataKey else { return false }
            return recommendationValue(in: row, for: key) != nil
        }
    }

    private var markdownSections: [(title: String, body: String)] {
        copyItems.compactMap { item in
            guard let key = item.dataKey,
                  let body = recommendationValue(in: row, for: key)?.displayText,
                  !body.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
                return nil
            }
            return (item.displayLabel, body == "—" ? "No \(item.displayLabel.lowercased()) available." : body)
        }
    }
}

private extension ItemDef {
    var dataKey: String? {
        let candidates = [field, dataField, bindingPath, id]
            .compactMap { $0?.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
        return candidates.first
    }

    var displayLabel: String {
        label?.trimmingCharacters(in: .whitespacesAndNewlines).nilIfEmpty
            ?? title?.trimmingCharacters(in: .whitespacesAndNewlines).nilIfEmpty
            ?? id?.replacingOccurrences(of: "_", with: " ").capitalized
            ?? "Field"
    }
}

private extension AgentlySDK.JSONValue {
    var stringValue: String? {
        if case .string(let value) = self {
            return value
        }
        return nil
    }

    var intValue: Int? {
        switch self {
        case .number(let value):
            return Int(value)
        case .string(let value):
            return Int(value)
        default:
            return nil
        }
    }

    var boolValue: Bool? {
        switch self {
        case .bool(let value):
            return value
        case .string(let value):
            return Bool(value)
        default:
            return nil
        }
    }

    var stringOrNumberValue: String? {
        switch self {
        case .string(let value):
            return value
        case .number(let value):
            if value.rounded(.towardZero) == value {
                return String(Int(value))
            }
            return String(value)
        default:
            return nil
        }
    }

    var displayText: String {
        switch self {
        case .string(let value):
            return Self.formatDisplayString(value)
        case .number(let value):
            if value.rounded(.towardZero) == value {
                return String(Int(value))
            }
            return String(value)
        case .bool(let value):
            return value ? "Yes" : "No"
        case .null:
            return "—"
        case .array(let values):
            return values.map(\.displayText).joined(separator: ", ")
        case .object(let values):
            return values
                .sorted { $0.key < $1.key }
                .map { "\($0.key): \($0.value.displayText)" }
                .joined(separator: "\n")
        }
    }

    private static func formatDisplayString(_ value: String) -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return "—" }
        if let formatted = formatDate(trimmed) {
            return formatted
        }
        return trimmed
    }

    private static func formatDate(_ value: String) -> String? {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = formatter.date(from: value) ?? {
            let fallback = ISO8601DateFormatter()
            fallback.formatOptions = [.withInternetDateTime]
            return fallback.date(from: value)
        }() {
            return date.formatted(date: .abbreviated, time: .shortened)
        }
        return nil
    }
}

private extension String {
    var nilIfEmpty: String? {
        isEmpty ? nil : self
    }
}

private func recommendationValue(
    in row: [String: AppJSONValue],
    for key: String
) -> AppJSONValue? {
    if let exact = row[key] {
        return exact
    }
    let trimmed = key.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty else {
        return nil
    }
    let candidates = [
        recommendationCamelCase(trimmed),
        recommendationSnakeCase(trimmed)
    ].filter { !$0.isEmpty && $0 != trimmed }

    for candidate in candidates {
        if let value = row[candidate] {
            return value
        }
    }
    return nil
}

private func recommendationCamelCase(_ key: String) -> String {
    let parts = key.split(separator: "_").map(String.init)
    guard let head = parts.first else { return key }
    return ([head.lowercased()] + parts.dropFirst().map { part in
        let lower = part.lowercased()
        return lower.prefix(1).uppercased() + lower.dropFirst()
    }).joined()
}

private func recommendationSnakeCase(_ key: String) -> String {
    guard key.contains(where: \.isUppercase) else { return key }
    var output = ""
    for character in key {
        if character.isUppercase {
            if !output.isEmpty {
                output.append("_")
            }
            output.append(character.lowercased())
        } else {
            output.append(character)
        }
    }
    return output
}

func extractDatasourceID(from uri: String) -> String? {
    let trimmed = uri.trimmingCharacters(in: .whitespacesAndNewlines)
    guard let range = trimmed.range(of: "/v1/api/datasources/") else {
        return nil
    }
    let remainder = trimmed[range.upperBound...]
    let head = remainder.split(separator: "/").first.map(String.init) ?? ""
    return head.isEmpty ? nil : head
}
