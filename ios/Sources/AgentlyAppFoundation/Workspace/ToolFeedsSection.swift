import SwiftUI
import AgentlySDK
import ForgeIOSRuntime
import ForgeIOSUI

func toolFeedSymbol(_ presentation: FeedPresentation?) -> String {
    switch presentation?.icon?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
    case "list", "checklist": return "list.bullet.clipboard"
    case "terminal", "console": return "terminal"
    case "changes", "refresh": return "arrow.triangle.2.circlepath"
    case "chart", "report": return "chart.bar"
    case "database", "data": return "cylinder"
    case "document", "file": return "doc.text"
    case "folder", "explorer": return "folder"
    default: return "wrench.and.screwdriver"
    }
}

func toolFeedAccent(_ presentation: FeedPresentation?) -> Color {
    let raw = presentation?.accent?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let named: [String: Color] = [
        "blue": Color(red: 0.10, green: 0.45, blue: 0.94),
        "orange": Color(red: 0.88, green: 0.54, blue: 0.12),
        "purple": Color(red: 0.49, green: 0.32, blue: 0.85),
        "teal": Color(red: 0.04, green: 0.61, blue: 0.60),
        "pink": Color(red: 0.87, green: 0.36, blue: 0.47),
    ]
    if let color = named[raw.lowercased()] { return color }
    let hex = raw.hasPrefix("#") ? String(raw.dropFirst()) : raw
    if hex.count == 6, let value = UInt64(hex, radix: 16) {
        return Color(
            red: Double((value >> 16) & 0xff) / 255,
            green: Double((value >> 8) & 0xff) / 255,
            blue: Double(value & 0xff) / 255
        )
    }
    return Color(red: 0.35, green: 0.40, blue: 0.85)
}

func visibleToolFeeds(_ feeds: [ActiveFeedState]) -> [ActiveFeedState] {
    feeds
        .filter { $0.developerOnly != true && !($0.feedID ?? "").trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
        .sorted {
            ($0.title ?? $0.feedID ?? "").localizedCaseInsensitiveCompare($1.title ?? $1.feedID ?? "") == .orderedAscending
        }
}

enum AppleFeedPlacement: String {
    case inline
    case workspace
    case detached
}

func appleFeedPlacement(_ feed: ActiveFeedState) -> AppleFeedPlacement {
    switch feed.presentation?.target?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
    case "inline": return .inline
    case "detached": return .detached
    default: return .workspace
    }
}

func toolFeeds(
    _ feeds: [ActiveFeedState],
    for placement: AppleFeedPlacement
) -> [ActiveFeedState] {
    visibleToolFeeds(feeds).filter { appleFeedPlacement($0) == placement }
}

func inlineToolFeeds(_ feeds: [ActiveFeedState], turnID: String?) -> [ActiveFeedState] {
    let owner = turnID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    guard !owner.isEmpty else { return [] }
    return toolFeeds(feeds, for: .inline).filter {
        $0.turnID?.trimmingCharacters(in: .whitespacesAndNewlines) == owner
    }
}

func suppressedToolFeedReportIDs(_ feeds: [ActiveFeedState]) -> Set<String> {
    Set(visibleToolFeeds(feeds).flatMap { feed in
        feed.presentation?.suppressReportIds.compactMap {
            let trimmed = $0.trimmingCharacters(in: .whitespacesAndNewlines)
            return trimmed.isEmpty ? nil : trimmed
        } ?? []
    })
}

func mergedToolFeeds(live: [ActiveFeedState], persisted: [ActiveFeedState]) -> [ActiveFeedState] {
    var rows: [String: ActiveFeedState] = [:]
    for feed in persisted + live {
        let id = (feed.feedID ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        if !id.isEmpty { rows[id] = feed }
    }
    return Array(rows.values)
}

func toolFeedLauncherExpanded(
    collapsible: Bool,
    isTurnActive: Bool,
    userOverride: Bool?
) -> Bool {
    guard collapsible else { return true }
    return userOverride ?? isTurnActive
}

func toolFeedSummaryLines(_ value: AgentlySDK.JSONValue?, limit: Int = 8) -> [String] {
    guard let value, limit > 0 else { return [] }
    var lines: [String] = []
    func append(_ text: String) {
        let normalized = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalized.isEmpty, lines.count < limit else { return }
        let preview = normalized.count > 480 ? String(normalized.prefix(480)) + "…" : normalized
        if !lines.contains(preview) { lines.append(preview) }
    }
    func walk(_ node: AgentlySDK.JSONValue, depth: Int = 0) {
        guard lines.count < limit, depth < 6 else { return }
        switch node {
        case .string(let text): append(text)
        case .number(let number): append(number.formatted())
        case .bool(let flag): append(flag ? "Yes" : "No")
        case .null: break
        case .array(let values): values.forEach { walk($0, depth: depth + 1) }
        case .object(let object):
            let preferred = ["explanation", "step", "title", "name", "command", "message", "path", "status", "input", "output"]
            for key in preferred {
                guard let child = object[key] else { continue }
                if case .string(let text) = child {
                    if key == "status" {
                        append(text.replacingOccurrences(of: "_", with: " ").capitalized)
                    } else if key == "input" {
                        append("$ " + text)
                    } else {
                        append(text)
                    }
                } else if case .object = child {
                    walk(child, depth: depth + 1)
                } else if case .array = child {
                    walk(child, depth: depth + 1)
                }
            }
            for key in object.keys.sorted() where !preferred.contains(key) {
                guard let child = object[key] else { continue }
                if case .object = child { walk(child, depth: depth + 1) }
                if case .array = child { walk(child, depth: depth + 1) }
            }
        }
    }
    walk(value)
    return lines
}

func decodedToolFeedContainer(_ value: AgentlySDK.JSONValue?) -> ContainerDef? {
    guard let value,
          let data = try? JSONEncoder().encode(value) else { return nil }
    return try? JSONDecoder().decode(ContainerDef.self, from: data)
}

func decodedToolFeedContent(_ value: AgentlySDK.JSONValue?) -> ContentDef? {
    guard let value, let data = try? JSONEncoder().encode(value) else { return nil }
    if let content = try? JSONDecoder().decode(ContentDef.self, from: data), !content.containers.isEmpty {
        return content
    }
    return (try? JSONDecoder().decode(ContainerDef.self, from: data)).map {
        ContentDef(containers: [$0])
    }
}

func decodedToolFeedDataSources(_ value: AgentlySDK.JSONValue?) -> [String: DataSourceDef] {
    guard let definitions = value?.objectValue else { return [:] }
    return definitions.reduce(into: [:]) { result, entry in
        let decoded = (try? JSONEncoder().encode(entry.value))
            .flatMap { try? JSONDecoder().decode(DataSourceDef.self, from: $0) }
        // Projection-only declarations (fields/flatten/derive) still need a
        // runtime context even when they contain no Forge transport metadata.
        guard let decoded else {
            result[entry.key] = DataSourceDef(autoFetch: false)
            return
        }
        if decoded.service == nil {
            result[entry.key] = DataSourceDef(
                selectionMode: decoded.selectionMode,
                autoSelect: decoded.autoSelect,
                autoFetch: false,
                uniqueKey: decoded.uniqueKey,
                selectors: decoded.selectors,
                params: decoded.params,
                parameters: decoded.parameters,
                uri: decoded.uri,
                method: decoded.method,
                on: decoded.on,
                target: decoded.target,
                targetOverrides: decoded.targetOverrides
            )
        } else {
            result[entry.key] = decoded
        }
    }
}

func toolFeedDataSources(
    declared value: AgentlySDK.JSONValue?,
    content: ContentDef
) -> [String: DataSourceDef] {
    var result = decodedToolFeedDataSources(value)
    for reference in referencedToolFeedLookupDataSources(content) where result[reference] == nil {
        result[reference] = DataSourceDef(
            service: DataSourceServiceDef(
                endpoint: "agentlyAPI",
                uri: "/v1/api/datasources/\(reference)/fetch"
            ),
            autoFetch: false
        )
    }
    return result
}

func referencedToolFeedLookupDataSources(_ content: ContentDef) -> Set<String> {
    var result = Set<String>()
    func appendReference(_ value: ForgeIOSRuntime.JSONValue?) {
        let reference = value?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if !reference.isEmpty { result.insert(reference) }
    }
    func visit(_ container: ContainerDef) {
        let lookup = container.lookup?.objectValue
        appendReference(lookup?["dataSourceRef"])
        appendReference(lookup?["drill"]?.objectValue?["dataSourceRef"])
        container.containers.forEach(visit)
    }
    content.containers.forEach(visit)
    return result
}

struct ToolFeedFilePreview {
    let container: ContainerDef
    let browser: FileBrowserDef
}

struct ToolFeedAction: Identifiable {
    let id: String
    let item: ToolbarItemDef
    let execution: ExecutionDef
}

func toolFeedActions(in container: ContainerDef?) -> [ToolFeedAction] {
    guard let container else { return [] }
    var result: [ToolFeedAction] = []
    for item in container.toolbar?.items ?? [] {
        let baseID = item.id ?? item.label ?? "action"
        for execution in item.on where execution.event == "onClick" && execution.action == "tool.execute" {
            result.append(ToolFeedAction(id: baseID + "-tool.execute", item: item, execution: execution))
        }
    }
    for child in container.containers { result.append(contentsOf: toolFeedActions(in: child)) }
    return result
}

func toolFeedFilePreview(in container: ContainerDef?) -> ToolFeedFilePreview? {
    guard let container else { return nil }
    if let browser = container.fileBrowser, browser.preview != nil {
        return ToolFeedFilePreview(container: container, browser: browser)
    }
    for child in container.containers {
        if let match = toolFeedFilePreview(in: child) { return match }
    }
    return nil
}

func toolFeedRows(
    data: AgentlySDK.JSONValue?,
    dataSources: AgentlySDK.JSONValue?,
    dataSourceRef: String?
) -> [[String: ForgeIOSRuntime.JSONValue]] {
    guard let data else { return [] }
    let ref = dataSourceRef?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    func select(_ path: String, from root: AgentlySDK.JSONValue) -> AgentlySDK.JSONValue? { selectToolFeedValue(path, from: root) }
    var visiting = Set<String>()
    var resolved: [String: AgentlySDK.JSONValue] = [:]
    func resolve(_ name: String, sources: [String: AgentlySDK.JSONValue]) -> AgentlySDK.JSONValue? {
        if let cached = resolved[name] { return cached }
        guard !visiting.contains(name), case .object(let definition) = sources[name] else { return nil }
        visiting.insert(name)
        defer { visiting.remove(name) }
        let selected: AgentlySDK.JSONValue?
        if case .string(let sourcePath) = definition["source"] {
            selected = select(sourcePath, from: data)
        } else if case .string(let parentName) = definition["dataSourceRef"],
                  let parent = resolve(parentName, sources: sources) {
            let parentRoot: AgentlySDK.JSONValue
            if case .array(let values) = parent, values.count == 1 { parentRoot = values[0] }
            else { parentRoot = parent }
            let selector: String
            if case .object(let selectors) = definition["selectors"], case .string(let value) = selectors["data"] { selector = value }
            else { selector = "output" }
            selected = select(selector, from: parentRoot)
        } else {
            selected = nil
        }
        let rows = projectAppleFeedRows(selected, definition: definition)
        let value = AgentlySDK.JSONValue.array(rows.map { .object($0) })
        resolved[name] = value
        return value
    }
    var selected = data
    if !ref.isEmpty, let dataSources, case .object(let sources) = dataSources {
        selected = resolve(ref, sources: sources) ?? data
    }
    guard let encoded = try? JSONEncoder().encode(selected),
          let bridged = try? JSONDecoder().decode(ForgeIOSRuntime.JSONValue.self, from: encoded) else { return [] }
    switch bridged {
    case .array(let values): return values.compactMap(\.objectValue)
    case .object(let row): return [row]
    default: return []
    }
}

func projectAppleFeedRows(
    _ value: AgentlySDK.JSONValue?,
    definition: [String: AgentlySDK.JSONValue]
) -> [[String: AgentlySDK.JSONValue]] {
    var rows: [[String: AgentlySDK.JSONValue]]
    if let flatten = definition["flatten"]?.objectValue {
        rows = flattenAppleFeedRows(value, config: flatten)
    } else if let fields = definition["fields"]?.objectValue {
        rows = appleFeedValues(value).map { projectAppleFeedFields($0, fields: fields) }
    } else {
        rows = appleFeedValues(value).compactMap { $0.objectValue ?? ["value": $0] }
    }
    rows = filterAppleFeedRows(rows, exclude: definition["exclude"])
    rows = deduplicateAppleFeedRows(rows, uniqueKey: definition["uniqueKey"])
    rows = deriveAppleFeedRows(rows, derive: definition["derive"]?.objectValue)
    if case .object(let aggregate) = definition["aggregate"],
       case .string(let countAs) = aggregate["countAs"],
       !countAs.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
        return [[countAs: .number(Double(rows.count))]]
    }
    return rows
}

func projectAppleFeedFields(
    _ root: AgentlySDK.JSONValue,
    fields: [String: AgentlySDK.JSONValue]
) -> [String: AgentlySDK.JSONValue] {
    var result: [String: AgentlySDK.JSONValue] = [:]
    for (name, raw) in fields {
        let config = raw.objectValue
        let path: String
        if case .string(let direct) = raw { path = direct }
        else if case .string(let configured) = config?["path"] { path = configured }
        else if case .string(let selector) = config?["selector"] { path = selector }
        else { path = name }
        let transform = appleFeedString(config?["transform"]).lowercased()
        var selected = selectToolFeedValue(path, from: root) ?? .null
        switch transform {
        case "daterange", "daterangelabel":
            let startPath = appleFeedString(config?["startPath"]).isEmpty ? "start" : appleFeedString(config?["startPath"])
            let endPath = appleFeedString(config?["endPath"]).isEmpty ? "end" : appleFeedString(config?["endPath"])
            let start = projectAppleFeedDate(selectToolFeedValue(startPath, from: root))
            let end = projectAppleFeedDate(selectToolFeedValue(endPath, from: root))
            selected = transform == "daterangelabel"
                ? .string([start, end].filter { !$0.isEmpty }.joined(separator: " – "))
                : .object(["start": .string(start), "end": .string(end)])
        case "dateparts":
            selected = .string(projectAppleFeedDate(selected))
        case "boolean":
            switch selected {
            case .bool(let value): selected = .bool(value)
            case .number(let value): selected = .bool(value == 1)
            case .string(let value): selected = .bool(value == "1" || value.lowercased() == "true")
            default: selected = .bool(false)
            }
        default:
            break
        }
        result[name] = selected
    }
    return result
}

private func projectAppleFeedDate(_ value: AgentlySDK.JSONValue?) -> String {
    guard case .object(let object) = value,
          case .number(let year) = object["year"],
          case .number(let day) = object["day"] else {
        if case .string(let text) = value { return text }
        return ""
    }
    let month: Double
    if case .number(let direct) = object["month"] { month = direct }
    else if case .number(let index) = object["monthIndex"] { month = index + 1 }
    else { return "" }
    return String(format: "%04d-%02d-%02d", Int(year), Int(month), Int(day))
}

private func flattenAppleFeedRows(
    _ value: AgentlySDK.JSONValue?,
    config: [String: AgentlySDK.JSONValue]
) -> [[String: AgentlySDK.JSONValue]] {
    guard case .array(let sources) = config["sources"] else { return [] }
    var output: [[String: AgentlySDK.JSONValue]] = []
    for parent in appleFeedValues(value) {
        for sourceValue in sources {
            guard case .object(let source) = sourceValue else { continue }
            for child in appleFeedValues(selectToolFeedValue(appleFeedString(source["path"]), from: parent)) {
                if appleFeedRowExcluded(child, rule: source["exclude"]) { continue }
                var row = source["fields"]?.objectValue.map { projectAppleFeedFields(child, fields: $0) }
                    ?? child.objectValue
                    ?? ["value": child]
                if case .object(let parentFields) = source["parentFields"] {
                    for (field, path) in parentFields {
                        row[field] = selectToolFeedValue(appleFeedString(path), from: parent) ?? .null
                    }
                }
                if case .object(let values) = source["values"] {
                    for (field, constant) in values { row[field] = constant }
                }
                output.append(row)
            }
        }
    }
    return output
}

private func filterAppleFeedRows(
    _ rows: [[String: AgentlySDK.JSONValue]],
    exclude: AgentlySDK.JSONValue?
) -> [[String: AgentlySDK.JSONValue]] {
    let rules: [AgentlySDK.JSONValue]
    if case .array(let values) = exclude { rules = values }
    else if let exclude { rules = [exclude] }
    else { rules = [] }
    return rows.filter { row in !rules.contains { appleFeedRowExcluded(.object(row), rule: $0) } }
}

func appleFeedRowExcluded(
    _ row: AgentlySDK.JSONValue,
    rule: AgentlySDK.JSONValue?
) -> Bool {
    guard case .object(let object) = rule else { return false }
    let path = appleFeedString(object["field"]).isEmpty ? appleFeedString(object["path"]) : appleFeedString(object["field"])
    let actual = selectToolFeedValue(path, from: row)
    if let expected = object["equalsIgnoreCase"] {
        return appleFeedScalarText(actual).trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            == appleFeedScalarText(expected).trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }
    if let expected = object["equals"] { return actual == expected }
    return false
}

private func deduplicateAppleFeedRows(
    _ rows: [[String: AgentlySDK.JSONValue]],
    uniqueKey: AgentlySDK.JSONValue?
) -> [[String: AgentlySDK.JSONValue]] {
    guard case .array(let entries) = uniqueKey else { return rows }
    let fields = entries.compactMap { entry -> String? in
        if case .object(let object) = entry { return appleFeedString(object["field"]) }
        return appleFeedString(entry)
    }.filter { !$0.isEmpty }
    guard !fields.isEmpty else { return rows }
    var seen = Set<String>()
    return rows.filter { row in
        let identity = fields.map { field in
            guard let value = row[field],
                  let data = try? JSONEncoder().encode(value) else { return "null" }
            return data.base64EncodedString()
        }.joined(separator: "|")
        return seen.insert(identity).inserted
    }
}

private func deriveAppleFeedRows(
    _ rows: [[String: AgentlySDK.JSONValue]],
    derive: [String: AgentlySDK.JSONValue]?
) -> [[String: AgentlySDK.JSONValue]] {
    guard let derive, !derive.isEmpty else { return rows }
    let regex = try? NSRegularExpression(pattern: #"\$\{([^}]+)\}"#)
    return rows.map { source in
        var row = source
        for (field, templateValue) in derive {
            let template = appleFeedString(templateValue)
            let range = NSRange(template.startIndex..<template.endIndex, in: template)
            var rendered = template
            for match in (regex?.matches(in: template, range: range) ?? []).reversed() {
                guard let expressionRange = Range(match.range(at: 1), in: template),
                      let replacementRange = Range(match.range(at: 0), in: rendered) else { continue }
                let replacement = appleFeedScalarText(selectToolFeedValue(String(template[expressionRange]), from: .object(row)))
                rendered.replaceSubrange(replacementRange, with: replacement)
            }
            row[field] = .string(rendered)
        }
        return row
    }
}

func appleFeedValues(_ value: AgentlySDK.JSONValue?) -> [AgentlySDK.JSONValue] {
    guard let value else { return [] }
    if case .array(let values) = value { return values }
    return [value]
}

func appleFeedString(_ value: AgentlySDK.JSONValue?) -> String {
    guard case .string(let string) = value else { return "" }
    return string
}

private func appleFeedScalarText(_ value: AgentlySDK.JSONValue?) -> String {
    switch value {
    case .string(let value): return value
    case .number(let value): return value.rounded() == value ? String(Int(value)) : String(value)
    case .bool(let value): return value ? "true" : "false"
    default: return ""
    }
}

struct ToolFeedsSection: View {
    let feeds: [ActiveFeedState]
    let conversationID: String?
    let client: AgentlyClient
    let collapsible: Bool
    let isTurnActive: Bool
    let placement: AppleFeedPlacement
    let sectionTitle: String
    let forgeRuntime: ForgeRuntime?

    @State private var selectedFeedID = ""
    @State private var payload: FeedDataResponse?
    @State private var isLoading = false
    @State private var errorMessage: String?
    @State private var isFeedSheetPresented = false
    @State private var expansionOverrides: [String: Bool] = [:]

    init(
        feeds: [ActiveFeedState],
        conversationID: String?,
        client: AgentlyClient,
        collapsible: Bool = false,
        isTurnActive: Bool = false,
        placement: AppleFeedPlacement = .workspace,
        sectionTitle: String = "Tool feeds",
        forgeRuntime: ForgeRuntime? = nil
    ) {
        self.feeds = feeds
        self.conversationID = conversationID
        self.client = client
        self.collapsible = collapsible
        self.isTurnActive = isTurnActive
        self.placement = placement
        self.sectionTitle = sectionTitle
        self.forgeRuntime = forgeRuntime
    }

    private var visibleFeeds: [ActiveFeedState] { toolFeeds(feeds, for: placement) }
    private var selectedFeed: ActiveFeedState? { visibleFeeds.first { $0.feedID == selectedFeedID } ?? visibleFeeds.first }
    private var effectiveConversationID: String {
        conversationID?.trimmingCharacters(in: .whitespacesAndNewlines)
            ?? visibleFeeds.first?.conversationID?.trimmingCharacters(in: .whitespacesAndNewlines)
            ?? ""
    }
    private var isLauncherExpanded: Bool {
        toolFeedLauncherExpanded(
            collapsible: collapsible,
            isTurnActive: isTurnActive,
            userOverride: expansionOverrides[effectiveConversationID]
        )
    }

    var body: some View {
        if !visibleFeeds.isEmpty, !effectiveConversationID.isEmpty {
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 10) {
                    Text(sectionTitle).font(.headline)
                    if collapsible && !isLauncherExpanded {
                        Text("\(visibleFeeds.count)")
                            .font(.caption2.weight(.bold))
                            .foregroundStyle(.secondary)
                            .padding(.horizontal, 7)
                            .padding(.vertical, 3)
                            .background(Color.secondary.opacity(0.10), in: Capsule())
                    }
                    Spacer()
                    if collapsible {
                        Button {
                            withAnimation(.easeInOut(duration: 0.18)) {
                                expansionOverrides[effectiveConversationID] = !isLauncherExpanded
                            }
                        } label: {
                            Image(systemName: isLauncherExpanded ? "chevron.up" : "chevron.down")
                                .font(.caption.weight(.bold))
                                .frame(width: 28, height: 28)
                                .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .foregroundStyle(.secondary)
                        .accessibilityLabel(isLauncherExpanded ? "Collapse tool feeds" : "Expand tool feeds")
                    }
                    Button("Open") { isFeedSheetPresented = true }
                        .font(.caption.weight(.semibold))
                }
                if isLauncherExpanded {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 8) {
                            ForEach(visibleFeeds) { feed in
                                Button {
                                    selectedFeedID = feed.feedID ?? ""
                                    isFeedSheetPresented = true
                                } label: {
                                    HStack(spacing: 5) {
                                        Image(systemName: toolFeedSymbol(feed.presentation))
                                        Text(feed.title ?? feed.feedID ?? "Feed")
                                        if (feed.itemCount ?? 0) > 0 {
                                            Text("\(feed.itemCount ?? 0)").font(.caption2.weight(.bold))
                                        }
                                    }
                                    .font(.caption.weight(.semibold))
                                    .foregroundStyle(toolFeedAccent(feed.presentation))
                                    .padding(.horizontal, 10)
                                    .padding(.vertical, 6)
                                    .background(toolFeedAccent(feed.presentation).opacity(selectedFeedID == feed.feedID ? 0.16 : 0.08), in: Capsule())
                                    .overlay(Capsule().stroke(toolFeedAccent(feed.presentation).opacity(0.22), lineWidth: 1))
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    }
                    .transition(.opacity.combined(with: .move(edge: .top)))
                }
            }
            .padding(12)
            .background(Color.secondary.opacity(0.045), in: RoundedRectangle(cornerRadius: 16))
            .overlay(RoundedRectangle(cornerRadius: 16).stroke(Color.secondary.opacity(0.12), lineWidth: 1))
            .padding(.horizontal)
            .task(id: "\(effectiveConversationID)#\(selectedFeedID)#\(visibleFeeds.map { $0.updatedAt ?? 0 })#\(isFeedSheetPresented)") {
                if selectedFeedID.isEmpty { selectedFeedID = visibleFeeds.first?.feedID ?? "" }
                guard isFeedSheetPresented, !selectedFeedID.isEmpty else { return }
                isLoading = true
                errorMessage = nil
                do {
                    payload = try await client.getFeedData(feedID: selectedFeedID, conversationID: effectiveConversationID)
                } catch {
                    payload = nil
                    errorMessage = "Unable to load this feed."
                }
                isLoading = false
            }
            .onChange(of: visibleFeeds.map { $0.feedID ?? "" }) { _, ids in
                if !ids.contains(selectedFeedID) { selectedFeedID = ids.first ?? "" }
            }
            .sheet(isPresented: $isFeedSheetPresented) {
                NavigationStack {
                    ScrollView {
                        VStack(alignment: .leading, spacing: 14) {
                            ScrollView(.horizontal, showsIndicators: false) {
                                HStack(spacing: 8) {
                                    ForEach(visibleFeeds) { feed in
                                        Button(feed.title ?? feed.feedID ?? "Feed") {
                                            selectedFeedID = feed.feedID ?? ""
                                        }
                                        .buttonStyle(.borderedProminent)
                                        .tint(toolFeedAccent(feed.presentation).opacity(selectedFeedID == feed.feedID ? 1 : 0.55))
                                    }
                                }
                            }
                            feedDetailContent
                        }
                        .padding()
                    }
                    .navigationTitle(sectionTitle)
                    .toolbar {
                        ToolbarItem(placement: .confirmationAction) {
                            Button("Done") { isFeedSheetPresented = false }
                        }
                    }
                }
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
            }
        }
    }

    @ViewBuilder
    private var feedDetailContent: some View {
        if isLoading {
            ProgressView().controlSize(.small)
        } else if let resolved = toolFeedFilePreview(in: decodedToolFeedContainer(payload?.ui)) {
            VStack(alignment: .leading, spacing: 12) {
                FilePreviewBrowser(
                    rows: toolFeedRows(
                        data: payload?.data ?? selectedFeed?.data,
                        dataSources: payload?.dataSources,
                        dataSourceRef: resolved.browser.dataSourceRef ?? resolved.container.dataSourceRef
                    ),
                    config: resolved.browser,
                    loadText: { uri in try await client.downloadWorkspaceFile(uri: uri) },
                    loadPreview: { uri in
                        guard let tool = resolved.browser.preview?.tool, !tool.isEmpty else {
                            throw NSError(domain: "ToolFeedPreview", code: 1)
                        }
                        let raw = try await client.executeTool(
                            name: tool,
                            args: ["url": .string(uri)],
                            conversationID: effectiveConversationID
                        )
                        guard let data = raw.data(using: .utf8),
                              let value = try? JSONDecoder().decode(AgentlySDK.JSONValue.self, from: data),
                              case .object(let object) = value else {
                            return FilePreviewContent()
                        }
                        func text(_ key: String) -> String {
                            if case .string(let value) = object[key] { return value }
                            return ""
                        }
                        return FilePreviewContent(
                            current: text("current"), previous: text("previous"), diff: text("diff")
                        )
                    }
                )
                ToolFeedActionBar(
                    actions: toolFeedActions(in: decodedToolFeedContainer(payload?.ui)),
                    client: client,
                    conversationID: effectiveConversationID
                )
            }
        } else if let container = decodedToolFeedContainer(payload?.ui),
                  let terminal = container.terminal {
            TerminalRenderer(
                container: container,
                terminal: terminal,
                rows: toolFeedRows(
                    data: payload?.data ?? selectedFeed?.data,
                    dataSources: payload?.dataSources,
                    dataSourceRef: terminal.dataSourceRef ?? container.dataSourceRef
                )
            )
        } else if let payload, let forgeRuntime,
                  let content = decodedToolFeedContent(payload.ui) {
            NativeToolFeedView(
                payload: payload,
                feed: selectedFeed,
                conversationID: effectiveConversationID,
                content: content,
                forgeRuntime: forgeRuntime
            )
        } else {
            let lines = toolFeedSummaryLines(payload?.data ?? selectedFeed?.data)
            if lines.isEmpty {
                if let errorMessage {
                    Text(errorMessage).font(.footnote).foregroundStyle(Color.red)
                } else {
                    Text("No feed details are available yet.").font(.footnote).foregroundStyle(.secondary)
                }
            } else {
                VStack(alignment: .leading, spacing: 8) {
                    ForEach(Array(lines.enumerated()), id: \.offset) { _, line in
                        HStack(alignment: .firstTextBaseline, spacing: 8) {
                            Image(systemName: "checkmark")
                                .font(.caption2.weight(.bold))
                                .foregroundStyle(Color.green)
                            Text(line).font(.body)
                        }
                    }
                    if errorMessage != nil {
                        Text("Showing the latest saved feed; live refresh is temporarily unavailable.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
    }

}

struct InlineToolFeedSurface: View {
    let feed: ActiveFeedState
    let conversationID: String
    let client: AgentlyClient
    let forgeRuntime: ForgeRuntime

    @State private var payload: FeedDataResponse?
    @State private var errorMessage: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Image(systemName: toolFeedSymbol(feed.presentation))
                Text(feed.title ?? feed.feedID ?? "Feed").font(.headline)
                Spacer()
                if (feed.itemCount ?? 0) > 0 {
                    Text("\(feed.itemCount ?? 0)").font(.caption.weight(.bold)).foregroundStyle(.secondary)
                }
            }
            if let payload, let content = decodedToolFeedContent(payload.ui) {
                NativeToolFeedView(
                    payload: payload,
                    feed: feed,
                    conversationID: conversationID,
                    content: content,
                    forgeRuntime: forgeRuntime
                )
            } else if let errorMessage {
                Text(errorMessage).font(.footnote).foregroundStyle(Color.red)
            } else {
                ProgressView().controlSize(.small)
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.secondary.opacity(0.045), in: RoundedRectangle(cornerRadius: 16))
        .overlay(RoundedRectangle(cornerRadius: 16).stroke(toolFeedAccent(feed.presentation).opacity(0.22), lineWidth: 1))
        .task(id: "\(conversationID)#\(feed.feedID ?? "")") {
            guard let feedID = feed.feedID?.trimmingCharacters(in: .whitespacesAndNewlines), !feedID.isEmpty else { return }
            do {
                payload = try await Task.detached(priority: .userInitiated) {
                    try await client.getFeedData(feedID: feedID, conversationID: conversationID)
                }.value
                errorMessage = nil
            } catch {
                payload = nil
                errorMessage = "Unable to load this feed."
            }
        }
    }
}

private struct NativeToolFeedView: View {
    let payload: FeedDataResponse
    let feed: ActiveFeedState?
    let conversationID: String
    let content: ContentDef
    let forgeRuntime: ForgeRuntime

    @State private var window: ForgeRuntime.WindowState?
    @State private var windowContext: WindowContext?
    @State private var errorMessage: String?

    var body: some View {
        Group {
            if window != nil, let windowContext {
                VStack(alignment: .leading, spacing: 12) {
                    ForEach(content.containers) { container in
                        ContainerRenderer(runtime: forgeRuntime, window: windowContext, container: container)
                    }
                }
                .environment(\.forgePresentationDensity, .compact)
            } else if let errorMessage {
                Text(errorMessage).font(.footnote).foregroundStyle(Color.red)
            } else {
                ProgressView().controlSize(.small)
            }
        }
        .task(id: "\(conversationID)#\(payload.feedID ?? feed?.feedID ?? "")") {
            await openAndHydrate()
        }
    }

    private func openAndHydrate() async {
        let feedID = (payload.feedID ?? feed?.feedID ?? "unknown").trimmingCharacters(in: .whitespacesAndNewlines)
        let hydratedPayload = FeedDataResponse(
            feedID: payload.feedID ?? feed?.feedID,
            title: payload.title ?? feed?.title,
            developerOnly: payload.developerOnly ?? feed?.developerOnly,
            presentation: payload.presentation ?? feed?.presentation,
            data: payload.data ?? feed?.data,
            dataSources: payload.dataSources,
            ui: payload.ui
        )
        let key = "feed-\(feedID)-\(conversationID)"
        let metadata = WindowMetadata(
            view: ViewDef(content: content),
            dataSources: toolFeedDataSources(declared: payload.dataSources, content: content)
        )
        let existing = await forgeRuntime.windows.first(where: { $0.key == key && $0.conversationID == conversationID })
        let state: ForgeRuntime.WindowState
        if let existing,
           let updated = await forgeRuntime.updateWindowInline(id: existing.id, title: payload.title ?? feed?.title ?? feedID, metadata: metadata) {
            state = updated
        } else {
            state = await forgeRuntime.openWindowInline(
                key: key,
                title: payload.title ?? feed?.title ?? feedID,
                metadata: metadata,
                conversationID: conversationID,
                presentation: payload.presentation?.target ?? feed?.presentation?.target ?? "auto"
            )
        }
        let effectiveData = await AppleFeedCanonicalRegistry.shared.register(
            forgeRuntime: forgeRuntime,
            windowID: state.id,
            payload: hydratedPayload,
            turnID: feed?.turnID
        )
        await forgeRuntime.registerFeedPatchHandler { windowID, operation in
            guard await AppleFeedCanonicalRegistry.shared.contains(
                forgeRuntime: forgeRuntime,
                windowID: windowID
            ) else { return false }
            _ = try await AppleFeedCanonicalRegistry.shared.apply(
                forgeRuntime: forgeRuntime,
                windowID: windowID,
                operations: [operation]
            )
            return true
        }
        for ref in metadata.dataSources.keys {
            let rows = toolFeedRows(data: effectiveData, dataSources: payload.dataSources, dataSourceRef: ref)
            await forgeRuntime.setDataSourceCollection(windowID: state.id, dataSourceRef: ref, rows: rows)
            if rows.count == 1 {
                await forgeRuntime.setDataSourceForm(windowID: state.id, dataSourceRef: ref, values: rows[0])
            }
        }
        window = state
        windowContext = await forgeRuntime.windowContext(id: state.id)
        errorMessage = nil
    }
}

private struct ToolFeedActionBar: View {
    let actions: [ToolFeedAction]
    let client: AgentlyClient
    let conversationID: String
    @State private var busyID: String?
    @State private var errorMessage: String?

    var body: some View {
        if !actions.isEmpty {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 8) {
                    Spacer()
                    ForEach(actions) { action in
                        actionButton(action)
                    }
                }
                if let errorMessage { Text(errorMessage).font(.caption).foregroundStyle(Color.red) }
            }
        }
    }

    @ViewBuilder private func actionButton(_ action: ToolFeedAction) -> some View {
        let button = Button {
            Task { await execute(action) }
        } label: {
            HStack(spacing: 6) {
                if busyID == action.id { ProgressView().controlSize(.small) }
                Text(action.item.label ?? action.item.id ?? "Action")
            }
        }
        .tint(action.item.intent == "primary" ? Color.blue : Color.secondary)
        .disabled(busyID != nil)
        if action.item.appearance == "minimal" { button.buttonStyle(.borderless) }
        else { button.buttonStyle(.borderedProminent) }
    }

    private func execute(_ action: ToolFeedAction) async {
        guard case .object(let target) = action.execution.target,
              case .string(let name) = target["name"], !name.isEmpty else { return }
        busyID = action.id
        errorMessage = nil
        do {
            let args: [String: AgentlySDK.JSONValue]
            if let value = target["arguments"],
               let data = try? JSONEncoder().encode(value),
               let decoded = try? JSONDecoder().decode([String: AgentlySDK.JSONValue].self, from: data) { args = decoded }
            else { args = [:] }
            _ = try await client.executeTool(name: name, args: args, conversationID: conversationID)
        } catch { errorMessage = error.localizedDescription }
        busyID = nil
    }
}
