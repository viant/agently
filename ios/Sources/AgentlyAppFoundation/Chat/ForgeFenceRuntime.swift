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
                    metadata: metadata
                )
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

    private var renderTaskKey: String {
        let blockCount = payload.blocks.count
        let dataKeys = dataStore.keys.sorted().joined(separator: "|")
        return "\(payload.title ?? "forge-ui"):\(payload.subtitle ?? ""):\(blockCount):\(dataKeys)"
    }

    private func openInlineWindow() async {
        guard let metadata = try? buildTranscriptForgeWindowMetadata(payload: payload, dataStore: dataStore) else {
            return
        }
        let state = await runtime.openWindowInline(
            key: "transcript-\(UUID().uuidString)",
            title: payload.title ?? "Forge content",
            metadata: metadata
        )
        windowID = state.id
        windowContext = await runtime.windowContext(id: state.id)
        await hydrateTranscriptDataSources(windowID: state.id, dataStore: dataStore)
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
    let containers = try payload.blocks.compactMap(adaptForgeBlock)
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

private func adaptForgeBlock(_ block: JSONValue) throws -> ContainerDef? {
    guard let object = block.objectValue else { return nil }
    let kind = object["kind"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    switch kind {
    case "planner.table":
        let id = object["id"]?.stringValue ?? "planner-table"
        let title = object["title"]?.stringValue
        let dataSourceRef = object["dataSourceRef"]?.stringValue
        let columns = object["columns"]?.arrayValue?.compactMap { column -> String? in
            guard let columnObject = column.objectValue else { return nil }
            return columnObject["key"]?.stringValue ?? columnObject["id"]?.stringValue
        } ?? []
        return ContainerDef(
            id: id,
            title: title,
            kind: "table",
            dataSourceRef: dataSourceRef,
            table: TableDef(title: title, columns: columns)
        )
    case "dashboard.summary":
        let metrics = object["metrics"]?.arrayValue?.compactMap { metric -> DashboardMetricDef? in
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
        } ?? []
        return ContainerDef(
            id: object["id"]?.stringValue ?? "dashboard-summary",
            title: object["title"]?.stringValue,
            subtitle: object["subtitle"]?.stringValue,
            kind: "dashboard.summary",
            dataSourceRef: object["dataSourceRef"]?.stringValue,
            metrics: metrics
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
            id: object["id"]?.stringValue ?? "dashboard-filters",
            title: object["title"]?.stringValue,
            subtitle: object["subtitle"]?.stringValue,
            kind: "dashboard.filters",
            dashboard: DashboardDef(filters: DashboardFiltersDef(items: items))
        )
    case "dashboard.timeline":
        let chartType = object["chartType"]?.stringValue ?? "bar"
        let xKey = object["dateField"]?.stringValue ?? object["timeColumn"]?.stringValue ?? object["groupBy"]?.stringValue ?? object["seriesColumn"]?.stringValue ?? "label"
        let valueKeys = object["series"]?.arrayValue?.compactMap(\.stringValue)
            ?? [object["valueColumn"]?.stringValue].compactMap { $0 }
        return ContainerDef(
            id: object["id"]?.stringValue ?? "dashboard-timeline",
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
