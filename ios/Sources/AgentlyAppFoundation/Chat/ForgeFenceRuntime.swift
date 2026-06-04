import SwiftUI
import ForgeIOSRuntime
import ForgeIOSUI

private let forgeUIFence = "forge-ui"
private let forgeDataFence = "forge-data"

enum TranscriptContentPart: Equatable {
    case markdown(String)
    case forgeUI(ForgeUIPayload, [String: MaterializedForgeDataBlock])
}

struct ForgeDataBlock: Codable, Equatable {
    let version: Int?
    let id: String?
    let format: String?
    let mode: String?
    let data: JSONValue?
}

struct ForgeUIPayload: Codable, Equatable {
    let version: Int?
    let title: String?
    let subtitle: String?
    let blocks: [JSONValue]
}

struct MaterializedForgeDataBlock: Equatable {
    let id: String
    let rows: JSONValue
}

private struct FenceMatch {
    let range: Range<String.Index>
    let kind: String
    let body: String
}

struct TranscriptMessageContent: View {
    let markdown: String

    var body: some View {
        let parts = parseTranscriptContentParts(markdown)
        VStack(alignment: .leading, spacing: 10) {
            ForEach(Array(parts.enumerated()), id: \.offset) { _, part in
                switch part {
                case .markdown(let text):
                    if !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        MarkdownRenderer(markdown: text)
                            .textSelection(.enabled)
                    }
                case .forgeUI(let payload, let dataStore):
                    TranscriptForgeUIView(payload: payload, dataStore: dataStore)
                }
            }
        }
    }
}

private struct TranscriptForgeUIView: View {
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass

    let payload: ForgeUIPayload
    let dataStore: [String: MaterializedForgeDataBlock]

    @State private var windowID: String?
    @State private var windowContext: WindowContext?
    private let runtime = ForgeRuntime()

    var body: some View {
        Group {
            if let metadata = try? buildTranscriptForgeWindowMetadata(payload: payload, dataStore: dataStore),
               let windowContext {
                WindowContentView(
                    runtime: runtime,
                    window: windowContext,
                    metadata: metadata,
                    scrollEnabled: true,
                    contentPadding: 4
                )
                .environment(\.forgePresentationDensity, .compact)
                .frame(maxHeight: inlineDashboardMaxHeight)
                .clipped()
            } else {
                VStack(alignment: .leading, spacing: 6) {
                    if let title = payload.title, !title.isEmpty {
                        Text(title).font(.headline)
                    }
                    Text("Loading interactive content…")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(12)
                .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
            }
        }
        .task(id: renderTaskKey) {
            windowID = nil
            windowContext = nil
            await openInlineWindow()
        }
    }

    private var inlineDashboardMaxHeight: CGFloat {
        horizontalSizeClass == .regular ? 420 : 340
    }

    private var renderTaskKey: String {
        let blockCount = payload.blocks.count
        let dataKeys = dataStore.keys.sorted().joined(separator: "|")
        return "\(payload.title ?? "forge-ui"):\(payload.subtitle ?? ""):\(blockCount):\(dataKeys)"
    }

    private func openInlineWindow() async {
        let normalizedStore = buildNormalizedTranscriptDataStore(payload: payload, dataStore: dataStore)
        guard let metadata = try? buildTranscriptForgeWindowMetadata(payload: payload, dataStore: normalizedStore) else {
            return
        }
        let state = await runtime.openWindowInline(
            key: "transcript-\(UUID().uuidString)",
            title: payload.title ?? "Forge content",
            metadata: metadata
        )
        windowID = state.id
        await hydrateTranscriptDataSources(windowID: state.id, dataStore: normalizedStore)
        windowContext = await runtime.windowContext(id: state.id)
    }

    private func hydrateTranscriptDataSources(windowID: String, dataStore: [String: MaterializedForgeDataBlock]) async {
        for (dataSourceRef, block) in dataStore {
            let rows = materializedRows(from: block.rows)
            await runtime.setDataSourceCollection(windowID: windowID, dataSourceRef: dataSourceRef, rows: rows)
            if rows.count == 1, let first = rows.first {
                await runtime.setDataSourceMetrics(windowID: windowID, dataSourceRef: dataSourceRef, values: first)
            }
        }
    }
}

func parseTranscriptContentParts(_ markdown: String) -> [TranscriptContentPart] {
    guard !markdown.isEmpty else { return [] }
    let matches = findForgeFenceMatches(in: markdown)
    guard !matches.isEmpty else { return [.markdown(markdown)] }

    var parts: [TranscriptContentPart] = []
    var dataBlocks: [ForgeDataBlock] = []
    var cursor = markdown.startIndex

    for match in matches {
        if cursor < match.range.lowerBound {
            appendMarkdownPart(&parts, String(markdown[cursor..<match.range.lowerBound]))
        }
        let rawFence = String(markdown[match.range])
        switch match.kind {
        case forgeDataFence:
            if let block = try? JSONDecoder().decode(ForgeDataBlock.self, from: Data(match.body.utf8)),
               let id = block.id?.trimmingCharacters(in: .whitespacesAndNewlines),
               !id.isEmpty {
                dataBlocks.append(block)
            } else {
                appendMarkdownPart(&parts, rawFence)
            }
        case forgeUIFence:
            if let payload = try? JSONDecoder().decode(ForgeUIPayload.self, from: Data(match.body.utf8)) {
                parts.append(.forgeUI(payload, materializeForgeDataBlocks(dataBlocks)))
            } else {
                appendMarkdownPart(&parts, rawFence)
            }
        default:
            appendMarkdownPart(&parts, rawFence)
        }
        cursor = match.range.upperBound
    }

    if cursor < markdown.endIndex {
        appendMarkdownPart(&parts, String(markdown[cursor..<markdown.endIndex]))
    }

    return parts.isEmpty ? [.markdown(markdown)] : parts
}

func buildTranscriptForgeWindowMetadata(
    payload: ForgeUIPayload,
    dataStore: [String: MaterializedForgeDataBlock]
) throws -> WindowMetadata {
    let refs = Set(dataStore.keys)
    let dataSources = Dictionary(uniqueKeysWithValues: refs.map { ($0, DataSourceDef()) })
    let containers = try payload.blocks.enumerated().compactMap { index, block in
        try adaptForgeBlock(block, index: index)
    }
    return WindowMetadata(
        namespace: "agently.ios.transcript",
        view: ViewDef(
            content: ContentDef(
                containers: [
                    ContainerDef(
                        id: "transcript-root",
                        title: payload.title,
                        subtitle: payload.subtitle,
                        containers: containers
                    )
                ]
            )
        ),
        dataSources: dataSources
    )
}

private func adaptForgeBlock(_ block: JSONValue, index: Int) throws -> ContainerDef? {
    guard let object = block.objectValue else { return nil }
    let kind = object["kind"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    switch kind {
    case "planner.table":
        let id = object["id"]?.stringValue ?? "planner-table-\(index)"
        let title = object["title"]?.stringValue
        let dataSourceRef = object["dataSourceRef"]?.stringValue
        let columns = transcriptColumns(from: object)
        return ContainerDef(
            id: id,
            title: title,
            kind: "table",
            dataSourceRef: dataSourceRef,
            table: TableDef(title: title, columns: columns)
        )
    case "dashboard.table":
        let id = object["id"]?.stringValue ?? "dashboard-table-\(index)"
        let title = object["title"]?.stringValue
        let dataSourceRef = resolveTranscriptDataSourceRef(from: object)
        let columns = transcriptColumns(from: object)
        return ContainerDef(
            id: id,
            title: title,
            kind: "table",
            dataSourceRef: dataSourceRef,
            table: TableDef(title: title, columns: columns)
        )
    case "dashboard.summary":
        let metrics = transcriptSummaryMetrics(from: object)
        return ContainerDef(
            id: object["id"]?.stringValue ?? "dashboard-summary-\(index)",
            title: object["title"]?.stringValue,
            subtitle: object["subtitle"]?.stringValue,
            kind: "dashboard.summary",
            dataSourceRef: resolveTranscriptDataSourceRef(from: object),
            metrics: metrics
        )
    case "dashboard.report":
        let sectionValues = object["sections"]?.arrayValue ?? []
        let sectionsPayload = sectionValues.compactMap { section -> JSONValue? in
            guard let sectionObject = section.objectValue else { return nil }
            let body = (sectionObject["body"]?.arrayValue ?? [])
                .compactMap(\.stringValue)
                .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
                .filter { !$0.isEmpty }
            return .object([
                "id": sectionObject["id"] ?? .null,
                "title": sectionObject["title"] ?? .null,
                "body": .array(body.map(JSONValue.string)),
                "tone": sectionObject["tone"] ?? .null
            ])
        }
        let sectionData = try JSONEncoder().encode(sectionsPayload)
        let sections = try JSONDecoder().decode([DashboardReportSectionDef].self, from: sectionData)
        return ContainerDef(
            id: object["id"]?.stringValue ?? "dashboard-report-\(index)",
            title: object["title"]?.stringValue,
            subtitle: object["subtitle"]?.stringValue,
            kind: "dashboard.report",
            sections: sections
        )
    case "dashboard.kpiTable":
        let id = object["id"]?.stringValue ?? "dashboard-kpi-table-\(index)"
        let title = object["title"]?.stringValue
        let dataSourceRef = resolveTranscriptDataSourceRef(from: object)
        let columns = transcriptColumns(from: object)
        return ContainerDef(
            id: id,
            title: title,
            kind: "table",
            dataSourceRef: dataSourceRef,
            table: TableDef(title: title, columns: columns)
        )
    case "dashboard.filters":
        let items = object["items"]?.arrayValue?.compactMap { item -> DashboardFilterItemDef? in
            guard let itemObject = item.objectValue else { return nil }
            let options = itemObject["options"]?.arrayValue?.compactMap { option -> DashboardFilterOptionDef? in
                guard let optionObject = option.objectValue else { return nil }
                return DashboardFilterOptionDef(
                    label: optionObject["label"]?.stringValue,
                    value: optionObject["value"]?.stringValue,
                    defaultValue: optionObject["default"]?.boolValue
                )
            } ?? []
            return DashboardFilterItemDef(
                id: itemObject["id"]?.stringValue,
                label: itemObject["label"]?.stringValue,
                field: itemObject["field"]?.stringValue,
                multiple: itemObject["multiple"]?.boolValue,
                options: options
            )
        } ?? []
        return ContainerDef(
            id: object["id"]?.stringValue ?? "dashboard-filters-\(index)",
            title: object["title"]?.stringValue,
            subtitle: object["subtitle"]?.stringValue,
            kind: "dashboard.filters",
            dashboard: DashboardDef(filters: DashboardFiltersDef(items: items))
        )
    case "dashboard.dimensions":
        let fieldData = try JSONEncoder().encode([
            object["dimension"] ?? .null,
            object["metric"] ?? .null
        ])
        let decodedFields = try JSONDecoder().decode([DashboardFieldDef].self, from: fieldData)
        return ContainerDef(
            id: object["id"]?.stringValue ?? "dashboard-dimensions-\(index)",
            title: object["title"]?.stringValue,
            subtitle: object["subtitle"]?.stringValue,
            kind: "dashboard.dimensions",
            dataSourceRef: resolveTranscriptDataSourceRef(from: object),
            dimension: decodedFields.first,
            metric: decodedFields.dropFirst().first,
            viewModes: object["viewModes"]?.arrayValue?.compactMap(\.stringValue) ?? [],
            limit: object["limit"]?.intValue,
            orderBy: object["orderBy"]?.stringValue
        )
    case "dashboard.messages":
        let items = (object["items"]?.arrayValue ?? []).compactMap { entry -> ItemDef? in
            guard let itemObject = entry.objectValue else { return nil }
            return ItemDef(
                id: itemObject["id"]?.stringValue,
                label: itemObject["label"]?.stringValue,
                title: itemObject["title"]?.stringValue,
                body: itemObject["body"]?.stringValue,
                severity: itemObject["severity"]?.stringValue
            )
        }
        return ContainerDef(
            id: object["id"]?.stringValue ?? "dashboard-messages-\(index)",
            title: object["title"]?.stringValue,
            subtitle: object["subtitle"]?.stringValue,
            kind: "dashboard.messages",
            items: items
        )
    case "dashboard.timeline":
        let chartType = object["chartType"]?.stringValue ?? "bar"
        let xKey = object["dateField"]?.stringValue ?? object["timeColumn"]?.stringValue ?? object["groupBy"]?.stringValue ?? object["seriesColumn"]?.stringValue ?? "label"
        let valueKeys = object["series"]?.arrayValue?.compactMap { series -> String? in
            if let key = series.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines), !key.isEmpty {
                return key
            }
            guard let seriesObject = series.objectValue else { return nil }
            return nonEmpty(seriesObject["key"]?.stringValue)
                ?? nonEmpty(seriesObject["id"]?.stringValue)
                ?? nonEmpty(seriesObject["value"]?.stringValue)
        }
            ?? [object["valueColumn"]?.stringValue].compactMap { $0 }
        return ContainerDef(
            id: object["id"]?.stringValue ?? "dashboard-timeline-\(index)",
            title: object["title"]?.stringValue,
            subtitle: object["subtitle"]?.stringValue,
            kind: "chart",
            dataSourceRef: object["dataSourceRef"]?.stringValue,
            chart: ChartDef(
                kind: "chart",
                title: object["title"]?.stringValue,
                type: chartType,
                xKey: xKey,
                valueKey: valueKeys.first,
                nameKey: xKey,
                series: valueKeys
            )
        )
    default:
        return nil
    }
}

private func buildNormalizedTranscriptDataStore(
    payload: ForgeUIPayload,
    dataStore: [String: MaterializedForgeDataBlock]
) -> [String: MaterializedForgeDataBlock] {
    var result = dataStore
    for block in payload.blocks {
        guard let object = block.objectValue else { continue }
        guard let synthetic = synthesizeTranscriptDataBlock(from: object) else { continue }
        result[synthetic.id] = synthetic
    }
    return result
}

private func synthesizeTranscriptDataBlock(from object: [String: JSONValue]) -> MaterializedForgeDataBlock? {
    let kind = object["kind"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    guard kind == "dashboard.summary" else { return nil }
    let dataSourceRef = syntheticTranscriptDataSourceRef(for: object)
    let items = object["items"]?.arrayValue ?? []
    var row: [String: JSONValue] = [:]
    for item in items {
        guard let itemObject = item.objectValue else { continue }
        let key = transcriptFieldKey(
            itemObject["key"]?.stringValue
                ?? itemObject["label"]?.stringValue
                ?? itemObject["id"]?.stringValue
                ?? "value"
        )
        row[key] = itemObject["value"] ?? .null
    }
    guard !row.isEmpty else { return nil }
    return MaterializedForgeDataBlock(id: dataSourceRef, rows: .array([.object(row)]))
}

private func transcriptSummaryMetrics(from object: [String: JSONValue]) -> [DashboardMetricDef] {
    if let declared = object["metrics"]?.arrayValue, !declared.isEmpty {
        return declared.compactMap { metric -> DashboardMetricDef? in
            if let name = metric.stringValue {
                return DashboardMetricDef(id: name, label: titleize(name), selector: name, format: nil)
            }
            guard let metricObject = metric.objectValue else { return nil }
            let selector = metricObject["selector"]?.stringValue ?? metricObject["key"]?.stringValue
            return DashboardMetricDef(
                id: metricObject["id"]?.stringValue ?? selector,
                label: metricObject["label"]?.stringValue ?? titleize(selector ?? ""),
                selector: selector,
                format: metricObject["format"]?.stringValue
            )
        }
    }
    return (object["items"]?.arrayValue ?? []).compactMap { item -> DashboardMetricDef? in
        guard let itemObject = item.objectValue else { return nil }
        let rawKey = itemObject["key"]?.stringValue
            ?? itemObject["label"]?.stringValue
            ?? itemObject["id"]?.stringValue
            ?? "value"
        let key = transcriptFieldKey(rawKey)
        return DashboardMetricDef(
            id: key,
            label: itemObject["label"]?.stringValue ?? titleize(key),
            selector: key,
            format: itemObject["format"]?.stringValue
        )
    }
}

private func resolveTranscriptDataSourceRef(from object: [String: JSONValue]) -> String? {
    nonEmpty(object["dataSourceRef"]?.stringValue)
        ?? nonEmpty(object["dataSource"]?.stringValue)
        ?? syntheticTranscriptDataSourceRef(for: object)
}

private func transcriptColumns(from object: [String: JSONValue]) -> [ColumnDef] {
    (object["columns"]?.arrayValue ?? []).compactMap { column in
        guard let columnObject = column.objectValue else { return nil }
        guard let key = nonEmpty(columnObject["key"]?.stringValue)
            ?? nonEmpty(columnObject["id"]?.stringValue) else {
            return nil
        }
        return ColumnDef(
            id: key,
            name: key,
            label: nonEmpty(columnObject["label"]?.stringValue) ?? titleize(key),
            type: nonEmpty(columnObject["type"]?.stringValue),
            format: nonEmpty(columnObject["format"]?.stringValue),
            link: columnObject["link"]?.objectValue.flatMap { linkObject in
                LinkDef(href: nonEmpty(linkObject["href"]?.stringValue))
            }
        )
    }
}

private func syntheticTranscriptDataSourceRef(for object: [String: JSONValue]) -> String {
    let base = nonEmpty(object["id"]?.stringValue)
        ?? nonEmpty(object["title"]?.stringValue)
        ?? "transcript-block"
    return "inline_\(transcriptFieldKey(base))"
}

private func nonEmpty(_ value: String?) -> String? {
    guard let value else { return nil }
    let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
    return trimmed.isEmpty ? nil : trimmed
}

private func transcriptFieldKey(_ value: String) -> String {
    let allowed = value
        .trimmingCharacters(in: .whitespacesAndNewlines)
        .replacingOccurrences(of: #"[^A-Za-z0-9]+"#, with: "_", options: .regularExpression)
        .trimmingCharacters(in: CharacterSet(charactersIn: "_"))
    guard !allowed.isEmpty else { return "value" }
    let parts = allowed.lowercased().split(separator: "_").map(String.init)
    guard let head = parts.first else { return "value" }
    return ([head] + parts.dropFirst().map { $0.prefix(1).uppercased() + $0.dropFirst() }).joined()
}

private func findForgeFenceMatches(in markdown: String) -> [FenceMatch] {
    let pattern = try! NSRegularExpression(
        pattern: "```(forge-data|forge-ui)(?:\\r?\\n|(?=[\\[{]))(.*?)```",
        options: [.dotMatchesLineSeparators, .caseInsensitive]
    )
    let nsRange = NSRange(markdown.startIndex..<markdown.endIndex, in: markdown)
    return pattern.matches(in: markdown, options: [], range: nsRange).compactMap { match in
        guard
            let range = Range(match.range, in: markdown),
            let kindRange = Range(match.range(at: 1), in: markdown),
            let bodyRange = Range(match.range(at: 2), in: markdown)
        else {
            return nil
        }
        return FenceMatch(
            range: range,
            kind: String(markdown[kindRange]).lowercased(),
            body: String(markdown[bodyRange]).trimmingCharacters(in: .whitespacesAndNewlines)
        )
    }
}

private func appendMarkdownPart(_ parts: inout [TranscriptContentPart], _ text: String) {
    guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
    if case .markdown(let existing)? = parts.last {
        parts[parts.count - 1] = .markdown(existing + text)
    } else {
        parts.append(.markdown(text))
    }
}

private func materializeForgeDataBlocks(_ blocks: [ForgeDataBlock]) -> [String: MaterializedForgeDataBlock] {
    var store: [String: MaterializedForgeDataBlock] = [:]
    for block in blocks {
        guard let id = block.id?.trimmingCharacters(in: .whitespacesAndNewlines), !id.isEmpty else { continue }
        store[id] = MaterializedForgeDataBlock(id: id, rows: block.data ?? .null)
    }
    return store
}

private func materializedRows(from value: JSONValue) -> [[String: JSONValue]] {
    switch value {
    case .array(let entries):
        return entries.compactMap(\.objectValue)
    case .object(let object):
        return [object]
    default:
        return []
    }
}

private func titleize(_ value: String) -> String {
    value
        .replacingOccurrences(of: "_", with: " ")
        .replacingOccurrences(of: "-", with: " ")
        .split(separator: " ")
        .map { token in token.prefix(1).uppercased() + token.dropFirst().lowercased() }
        .joined(separator: " ")
}
