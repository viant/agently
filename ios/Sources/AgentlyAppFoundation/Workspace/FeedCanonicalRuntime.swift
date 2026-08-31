import Foundation
import AgentlySDK
import ForgeIOSRuntime

actor AppleFeedCanonicalRegistry {
    static let shared = AppleFeedCanonicalRegistry()

    private struct Key: Hashable {
        let runtimeID: ObjectIdentifier
        let windowID: String
    }

    private struct State {
        var dataSources: [String: AgentlySDK.JSONValue]
        var canonical: AgentlySDK.JSONValue
        var turnID: String?
        var dirty: Bool
    }

    private var states: [Key: State] = [:]

    func register(
        forgeRuntime: ForgeRuntime,
        windowID: String,
        payload: FeedDataResponse,
        turnID: String? = nil
    ) -> AgentlySDK.JSONValue {
        let key = Key(runtimeID: ObjectIdentifier(forgeRuntime), windowID: windowID)
        let incomingTurn = normalizedToolFeedID(turnID)
        if var existing = states[key],
           existing.dirty,
           incomingTurn == nil || incomingTurn == existing.turnID {
            if let definitions = payload.dataSources?.objectValue, !definitions.isEmpty {
                existing.dataSources = definitions
            }
            states[key] = existing
            return existing.canonical
        }
        let state = State(
            dataSources: payload.dataSources?.objectValue ?? [:],
            canonical: payload.data ?? .null,
            turnID: incomingTurn,
            dirty: false
        )
        states[key] = state
        return state.canonical
    }

    func clear(forgeRuntime: ForgeRuntime, windowID: String) {
        states.removeValue(forKey: Key(runtimeID: ObjectIdentifier(forgeRuntime), windowID: windowID))
    }

    func contains(forgeRuntime: ForgeRuntime, windowID: String) -> Bool {
        states[Key(runtimeID: ObjectIdentifier(forgeRuntime), windowID: windowID)] != nil
    }

    func apply(
        forgeRuntime: ForgeRuntime,
        windowID: String,
        operations: [ForgeIOSRuntime.FeedPatchOperation],
        turnID: String? = nil
    ) async throws -> Set<String> {
        let key = Key(runtimeID: ObjectIdentifier(forgeRuntime), windowID: windowID)
        guard var state = states[key] else { return [] }
        var canonical = state.canonical
        var canonicalOperations: [([String], ForgeIOSRuntime.FeedPatchOperation)] = []
        var directSelectionOperations: [ForgeIOSRuntime.FeedPatchOperation] = []
        var synchronizedRefs = Set<String>()

        for operation in operations {
            guard let definition = state.dataSources[operation.dataSourceRef]?.objectValue else {
                throw FeedDraftRuntimeError.unknownDataSource(operation.dataSourceRef)
            }
            let viewTokens = try toolFeedPointerTokens(operation.path)
            guard let view = viewTokens.first else {
                throw FeedDraftRuntimeError.invalidPointer(operation.path)
            }
            let rootPath = try canonicalToolFeedDataSourcePath(
                operation.dataSourceRef,
                definitions: state.dataSources,
                canonical: canonical
            )
            let relative: [String]
            switch view {
            case "form", "collection":
                relative = Array(viewTokens.dropFirst())
            case "selection":
                guard let resolved = await selectionCanonicalRelativePath(
                    forgeRuntime: forgeRuntime,
                    windowID: windowID,
                    ref: operation.dataSourceRef,
                    definition: definition,
                    relative: Array(viewTokens.dropFirst())
                ) else {
                    directSelectionOperations.append(operation)
                    continue
                }
                relative = resolved
            default:
                throw FeedDraftRuntimeError.unsupportedView(view)
            }
            if synchronizedRefs.insert(operation.dataSourceRef).inserted {
                canonical = await synchronizeCurrentDataSourceView(
                    canonical: canonical,
                    rootPath: rootPath,
                    forgeRuntime: forgeRuntime,
                    windowID: windowID,
                    ref: operation.dataSourceRef,
                    definition: definition
                )
            }
            guard let canonicalRoot = toolFeedValue(at: rootPath, in: canonical) else {
                throw FeedDraftRuntimeError.missingPath(operation.dataSourceRef)
            }
            let currentRows = await forgeRuntime.dataSourceCollection(
                windowID: windowID,
                dataSourceRef: operation.dataSourceRef
            ).map { $0.mapValues(\.appValue) }
            if operation.op == "add", relative.first == "-", case .object(let flatten) = definition["flatten"] {
                let mapped = try mapFlattenedAppleFeedAdd(
                    flatten: flatten,
                    projectedValue: operation.value?.appValue,
                    canonicalRoot: canonicalRoot
                )
                canonicalOperations.append((
                    rootPath + mapped.path,
                    ForgeIOSRuntime.FeedPatchOperation(
                        dataSourceRef: operation.dataSourceRef,
                        op: operation.op,
                        path: operation.path,
                        value: mapped.value.forgeValue
                    )
                ))
            } else {
                let mappedRelative = try mapProjectedAppleFeedRelativePath(
                    definition: definition,
                    viewRelative: relative,
                    canonicalRoot: canonicalRoot,
                    currentRows: currentRows
                )
                canonicalOperations.append((rootPath + mappedRelative, operation))
            }
        }

        for (path, operation) in canonicalOperations {
            canonical = try patchCanonicalToolFeedValue(canonical, tokens: path, operation: operation)
        }
        if !canonicalOperations.isEmpty {
            state.canonical = canonical
            state.dirty = true
            if let incomingTurn = normalizedToolFeedID(turnID) { state.turnID = incomingTurn }
            states[key] = state
            try await rehydrateCanonicalFeed(
                forgeRuntime: forgeRuntime,
                windowID: windowID,
                state: state
            )
        }
        if !directSelectionOperations.isEmpty {
            _ = try await forgeRuntime.applyFeedPatchOperations(
                windowID: windowID,
                operations: directSelectionOperations
            )
        }
        if !canonicalOperations.isEmpty {
            return Set(state.dataSources.keys).union(directSelectionOperations.map(\.dataSourceRef))
        }
        return Set(directSelectionOperations.map(\.dataSourceRef))
    }

    private func rehydrateCanonicalFeed(
        forgeRuntime: ForgeRuntime,
        windowID: String,
        state: State
    ) async throws {
        var previousSelections: [String: SelectionState] = [:]
        for ref in state.dataSources.keys {
            previousSelections[ref] = await forgeRuntime.dataSourceSelectionState(
                windowID: windowID,
                dataSourceRef: ref
            )
        }
        let sourceValue = AgentlySDK.JSONValue.object(state.dataSources)
        for (ref, rawDefinition) in state.dataSources {
            let rows = toolFeedRows(
                data: state.canonical,
                dataSources: sourceValue,
                dataSourceRef: ref
            )
            await forgeRuntime.setDataSourceCollection(windowID: windowID, dataSourceRef: ref, rows: rows)
            if rows.count == 1 {
                await forgeRuntime.setDataSourceForm(windowID: windowID, dataSourceRef: ref, values: rows[0])
            }
            let definition = rawDefinition.objectValue ?? [:]
            let reconciled = reconcileToolFeedSelection(
                previousSelections[ref] ?? SelectionState(),
                rows: rows,
                uniqueKeys: toolFeedUniqueKeyFields(definition)
            )
            await forgeRuntime.setDataSourceSelectionState(
                windowID: windowID,
                dataSourceRef: ref,
                selection: reconciled
            )
            if rows.count == 1 {
                await forgeRuntime.setDataSourceForm(windowID: windowID, dataSourceRef: ref, values: rows[0])
            }
        }
    }
}

private func mapFlattenedAppleFeedAdd(
    flatten: [String: AgentlySDK.JSONValue],
    projectedValue: AgentlySDK.JSONValue?,
    canonicalRoot: AgentlySDK.JSONValue
) throws -> (path: [String], value: AgentlySDK.JSONValue) {
    guard case .object(let projected) = projectedValue,
          case .array(let sources) = flatten["sources"] else {
        throw FeedDraftRuntimeError.invalidViewShape("flattened add requires an object")
    }
    let parents = appleFeedValues(canonicalRoot)
    let parentIsArray: Bool
    if case .array = canonicalRoot { parentIsArray = true } else { parentIsArray = false }
    for (parentIndex, parent) in parents.enumerated() {
        for sourceValue in sources {
            guard case .object(let source) = sourceValue else { continue }
            let constantsMatch = source["values"]?.objectValue?.allSatisfy { field, value in
                projected[field] == value
            } ?? true
            let parentMatches = source["parentFields"]?.objectValue?.allSatisfy { field, path in
                projected[field] == selectToolFeedValue(appleFeedString(path), from: parent)
            } ?? true
            if !constantsMatch || !parentMatches { continue }
            let sourcePath = appleFeedString(source["path"])
            let rawValue: AgentlySDK.JSONValue
            if let fields = source["fields"]?.objectValue {
                if fields.count == 1,
                   let first = fields.first,
                   appleFeedString(first.value) == "$" {
                    rawValue = projected[first.key] ?? .null
                } else {
                    var raw: AgentlySDK.JSONValue = .object([:])
                    for (viewField, mapping) in fields where projected[viewField] != nil {
                        raw = setAppleFeedPath(
                            root: raw,
                            tokens: toolFeedSelectorTokens(appleFeedString(mapping)),
                            value: projected[viewField] ?? .null
                        )
                    }
                    rawValue = raw
                }
            } else {
                let constants = Set(source["values"]?.objectValue?.keys.map { $0 } ?? [])
                let parentFields = Set(source["parentFields"]?.objectValue?.keys.map { $0 } ?? [])
                rawValue = .object(projected.filter { !constants.contains($0.key) && !parentFields.contains($0.key) })
            }
            var path: [String] = []
            if parentIsArray { path.append(String(parentIndex)) }
            if !sourcePath.isEmpty, sourcePath != "$" { path.append(contentsOf: toolFeedSelectorTokens(sourcePath)) }
            path.append("-")
            return (path, rawValue)
        }
    }
    throw FeedDraftRuntimeError.missingPath("flattened add parent/source")
}

private func setAppleFeedPath(
    root: AgentlySDK.JSONValue,
    tokens: [String],
    value: AgentlySDK.JSONValue
) -> AgentlySDK.JSONValue {
    guard let token = tokens.first else { return value }
    var object = root.objectValue ?? [:]
    object[token] = setAppleFeedPath(
        root: object[token] ?? .object([:]),
        tokens: Array(tokens.dropFirst()),
        value: value
    )
    return .object(object)
}

private func mapProjectedAppleFeedRelativePath(
    definition: [String: AgentlySDK.JSONValue],
    viewRelative: [String],
    canonicalRoot: AgentlySDK.JSONValue,
    currentRows: [[String: AgentlySDK.JSONValue]]
) throws -> [String] {
    if case .object(let aggregate) = definition["aggregate"], aggregate["countAs"] != nil {
        throw FeedDraftRuntimeError.invalidViewShape("aggregate datasource is read-only")
    }
    if case .object(let flatten) = definition["flatten"] {
        return try mapFlattenedAppleFeedRelativePath(
            definition: definition,
            flatten: flatten,
            viewRelative: viewRelative,
            canonicalRoot: canonicalRoot,
            currentRows: currentRows
        )
    }
    if case .array(let rawValues) = canonicalRoot, !viewRelative.isEmpty {
        if viewRelative[0] == "-", definition["fields"] == nil, definition["exclude"] == nil, definition["derive"] == nil {
            return viewRelative
        }
        guard let viewIndex = Int(viewRelative[0]), currentRows.indices.contains(viewIndex) else {
            throw FeedDraftRuntimeError.invalidArrayIndex(viewRelative.first ?? "")
        }
        let target = currentRows[viewIndex]
        guard let rawIndex = rawValues.firstIndex(where: { raw in
            guard let projected = projectAppleFeedRows(raw, definition: definition).first else { return false }
            return appleFeedRowsMatch(projected, target, uniqueKeys: appleFeedUniqueKeyFields(definition))
        }) else {
            throw FeedDraftRuntimeError.missingPath("canonical collection row")
        }
        return [String(rawIndex)] + (try mapProjectedAppleFeedFieldPath(
            fields: definition["fields"]?.objectValue,
            relative: Array(viewRelative.dropFirst()),
            definition: definition
        ))
    }
    return try mapProjectedAppleFeedFieldPath(
        fields: definition["fields"]?.objectValue,
        relative: viewRelative,
        definition: definition
    )
}

private func mapProjectedAppleFeedFieldPath(
    fields: [String: AgentlySDK.JSONValue]?,
    relative: [String],
    definition: [String: AgentlySDK.JSONValue]
) throws -> [String] {
    guard let field = relative.first else { return relative }
    if definition["derive"]?.objectValue?[field] != nil {
        throw FeedDraftRuntimeError.invalidViewShape("derived field is read-only: \(field)")
    }
    guard let raw = fields?[field] else { return relative }
    let config = raw.objectValue
    let transform = appleFeedString(config?["transform"]).lowercased()
    let remainder = Array(relative.dropFirst())
    let path: String
    var consumed = 0
    if transform == "daterange", remainder.first == "start" {
        path = appleFeedString(config?["startPath"]).isEmpty ? "start" : appleFeedString(config?["startPath"])
        consumed = 1
    } else if transform == "daterange", remainder.first == "end" {
        path = appleFeedString(config?["endPath"]).isEmpty ? "end" : appleFeedString(config?["endPath"])
        consumed = 1
    } else if case .string(let direct) = raw {
        path = direct
    } else {
        let configured = appleFeedString(config?["path"])
        path = configured.isEmpty ? (appleFeedString(config?["selector"]).isEmpty ? field : appleFeedString(config?["selector"])) : configured
    }
    let mapped = path.trimmingCharacters(in: .whitespacesAndNewlines) == "$" ? [] : toolFeedSelectorTokens(path)
    return mapped + remainder.dropFirst(consumed)
}

private func mapFlattenedAppleFeedRelativePath(
    definition: [String: AgentlySDK.JSONValue],
    flatten: [String: AgentlySDK.JSONValue],
    viewRelative: [String],
    canonicalRoot: AgentlySDK.JSONValue,
    currentRows: [[String: AgentlySDK.JSONValue]]
) throws -> [String] {
    guard let viewIndexText = viewRelative.first,
          let viewIndex = Int(viewIndexText),
          currentRows.indices.contains(viewIndex),
          case .array(let sources) = flatten["sources"] else {
        throw FeedDraftRuntimeError.invalidArrayIndex(viewRelative.first ?? "")
    }
    let target = currentRows[viewIndex]
    let parents = appleFeedValues(canonicalRoot)
    let parentIsArray: Bool
    if case .array = canonicalRoot { parentIsArray = true } else { parentIsArray = false }
    for (parentIndex, parent) in parents.enumerated() {
        for sourceValue in sources {
            guard case .object(let source) = sourceValue else { continue }
            let sourcePath = appleFeedString(source["path"])
            let selectedChildren = selectToolFeedValue(sourcePath, from: parent)
            let children = appleFeedValues(selectedChildren)
            let childIsArray: Bool
            if case .array = selectedChildren { childIsArray = true } else { childIsArray = false }
            for (childIndex, child) in children.enumerated() {
                if appleFeedRowExcluded(child, rule: source["exclude"]) { continue }
                var projected = source["fields"]?.objectValue.map { projectAppleFeedFields(child, fields: $0) }
                    ?? child.objectValue
                    ?? ["value": child]
                if case .object(let parentFields) = source["parentFields"] {
                    for (field, path) in parentFields {
                        projected[field] = selectToolFeedValue(appleFeedString(path), from: parent) ?? .null
                    }
                }
                if case .object(let values) = source["values"] {
                    for (field, value) in values { projected[field] = value }
                }
                if !appleFeedRowsMatch(projected, target, uniqueKeys: appleFeedUniqueKeyFields(definition)) { continue }
                let field = viewRelative.dropFirst().first
                if let field, source["parentFields"]?.objectValue?[field] != nil {
                    throw FeedDraftRuntimeError.invalidViewShape("flattened parent field is read-only: \(field)")
                }
                if let field, source["values"]?.objectValue?[field] != nil {
                    throw FeedDraftRuntimeError.invalidViewShape("flattened constant field is read-only: \(field)")
                }
                var prefix: [String] = []
                if parentIsArray { prefix.append(String(parentIndex)) }
                if !sourcePath.isEmpty, sourcePath != "$" { prefix.append(contentsOf: toolFeedSelectorTokens(sourcePath)) }
                if childIsArray { prefix.append(String(childIndex)) }
                return prefix + (try mapProjectedAppleFeedFieldPath(
                    fields: source["fields"]?.objectValue,
                    relative: Array(viewRelative.dropFirst()),
                    definition: definition
                ))
            }
        }
    }
    throw FeedDraftRuntimeError.missingPath("flattened canonical row")
}

private func appleFeedRowsMatch(
    _ candidate: [String: AgentlySDK.JSONValue],
    _ target: [String: AgentlySDK.JSONValue],
    uniqueKeys: [String]
) -> Bool {
    if uniqueKeys.isEmpty { return candidate == target }
    return uniqueKeys.allSatisfy { candidate[$0] == target[$0] }
}

private func appleFeedUniqueKeyFields(_ definition: [String: AgentlySDK.JSONValue]) -> [String] {
    toolFeedArray(definition["uniqueKey"]).compactMap {
        toolFeedString($0.objectValue?["field"]).trimmingCharacters(in: .whitespacesAndNewlines)
    }.filter { !$0.isEmpty }
}

private func normalizedToolFeedID(_ value: String?) -> String? {
    let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    return trimmed.isEmpty ? nil : trimmed
}

func selectToolFeedValue(
    _ selector: String?,
    from root: AgentlySDK.JSONValue
) -> AgentlySDK.JSONValue? {
    let normalizedSelector = selector?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    if normalizedSelector.isEmpty || normalizedSelector == "$" { return root }
    if normalizedSelector == "output" || normalizedSelector == "input" {
        if case .object(let object) = root, let selected = object[normalizedSelector] { return selected }
        return root
    }
    var tokens = toolFeedSelectorTokens(selector)
    if case .object(let object) = root,
       object["output"] == nil,
       object["input"] == nil,
       tokens.first == "output" || tokens.first == "input" {
        tokens.removeFirst()
    }
    var current = root
    for token in tokens {
        switch current {
        case .object(let object):
            guard let next = object[token] else { return nil }
            current = next
        case .array(let values):
            guard let index = Int(token), values.indices.contains(index) else { return nil }
            current = values[index]
        default:
            return nil
        }
    }
    return current
}

func toolFeedSelectorTokens(_ selector: String?) -> [String] {
    let input = selector?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    guard !input.isEmpty else { return [] }
    var tokens: [String] = []
    var current = ""
    var index = input.startIndex
    func flush() {
        let token = current.trimmingCharacters(in: .whitespacesAndNewlines)
        if !token.isEmpty { tokens.append(token) }
        current = ""
    }
    while index < input.endIndex {
        let character = input[index]
        if character == "." {
            flush()
            index = input.index(after: index)
        } else if character == "[" {
            flush()
            guard let close = input[index...].firstIndex(of: "]") else { break }
            let token = input[input.index(after: index)..<close].trimmingCharacters(in: .whitespacesAndNewlines)
            if !token.isEmpty { tokens.append(token) }
            index = input.index(after: close)
        } else {
            current.append(character)
            index = input.index(after: index)
        }
    }
    flush()
    return tokens
}

private func canonicalToolFeedDataSourcePath(
    _ ref: String,
    definitions: [String: AgentlySDK.JSONValue],
    canonical: AgentlySDK.JSONValue,
    visiting: Set<String> = []
) throws -> [String] {
    guard !visiting.contains(ref) else {
        throw FeedDraftRuntimeError.missingPath("cyclic datasource: \(ref)")
    }
    guard let definition = definitions[ref]?.objectValue else {
        throw FeedDraftRuntimeError.unknownDataSource(ref)
    }
    var nextVisiting = visiting
    nextVisiting.insert(ref)
    let parent = toolFeedString(definition["dataSourceRef"]).trimmingCharacters(in: .whitespacesAndNewlines)
    var path: [String]
    if parent.isEmpty {
        path = toolFeedSelectorTokens(toolFeedString(definition["source"]))
    } else {
        let rawSelector = toolFeedString(definition["selectors"]?.objectValue?["data"])
        let selector = rawSelector.isEmpty ? "output" : rawSelector
        let parentPath = try canonicalToolFeedDataSourcePath(
            parent,
            definitions: definitions,
            canonical: canonical,
            visiting: nextVisiting
        )
        let selectorTokens = toolFeedSelectorTokens(selector)
        let parentValue = toolFeedValue(at: parentPath, in: canonical)
        let effectiveSelector: [String]
        if selectorTokens.count == 1,
           selectorTokens[0] == "output" || selectorTokens[0] == "input",
           case .object(let object) = parentValue,
           object[selectorTokens[0]] == nil {
            effectiveSelector = []
        } else {
            effectiveSelector = selectorTokens
        }
        path = parentPath + effectiveSelector
    }
    if case .object(let object) = canonical,
       object["output"] == nil,
       object["input"] == nil,
       path.first == "output" || path.first == "input" {
        path.removeFirst()
    }
    return path
}

private func synchronizeCurrentDataSourceView(
    canonical: AgentlySDK.JSONValue,
    rootPath: [String],
    forgeRuntime: ForgeRuntime,
    windowID: String,
    ref: String,
    definition: [String: AgentlySDK.JSONValue]
) async -> AgentlySDK.JSONValue {
    guard let currentRoot = toolFeedValue(at: rootPath, in: canonical) else { return canonical }
    if case .object = currentRoot, let fields = definition["fields"]?.objectValue {
        var synchronized = canonical
        let form = await forgeRuntime.formJSONValue(windowID: windowID, dataSourceRef: ref).mapValues(\.appValue)
        for (field, value) in form {
            let config = fields[field]?.objectValue
            let transform = appleFeedString(config?["transform"]).lowercased()
            if transform == "daterange", case .object(let range) = value {
                for (viewField, pathField) in [("start", "startPath"), ("end", "endPath")] {
                    let configured = appleFeedString(config?[pathField])
                    let mapped = toolFeedSelectorTokens(configured.isEmpty ? viewField : configured)
                    synchronized = (try? replaceCanonicalToolFeedValue(
                        synchronized,
                        tokens: rootPath + mapped,
                        value: range[viewField] ?? .null
                    )) ?? synchronized
                }
            } else if transform != "daterangelabel", definition["derive"]?.objectValue?[field] == nil {
                let mapped = try? mapProjectedAppleFeedFieldPath(fields: fields, relative: [field], definition: definition)
                if let mapped {
                    synchronized = (try? replaceCanonicalToolFeedValue(
                        synchronized,
                        tokens: rootPath + mapped,
                        value: value
                    )) ?? synchronized
                }
            }
        }
        return synchronized
    }
    if definition["flatten"] != nil || definition["exclude"] != nil || definition["aggregate"] != nil || definition["derive"] != nil {
        return canonical
    }
    let replacement: AgentlySDK.JSONValue?
    switch currentRoot {
    case .array:
        let rows = await forgeRuntime.dataSourceCollection(windowID: windowID, dataSourceRef: ref)
        replacement = .array(rows.map { .object($0.mapValues(\.appValue)) })
    case .object:
        let form = await forgeRuntime.formJSONValue(windowID: windowID, dataSourceRef: ref)
        if !form.isEmpty {
            replacement = .object(form.mapValues(\.appValue))
        } else {
            let rows = await forgeRuntime.dataSourceCollection(windowID: windowID, dataSourceRef: ref)
            replacement = rows.count == 1 ? .object(rows[0].mapValues(\.appValue)) : nil
        }
    default:
        replacement = nil
    }
    guard let replacement else { return canonical }
    return (try? replaceCanonicalToolFeedValue(canonical, tokens: rootPath, value: replacement)) ?? canonical
}

private func selectionCanonicalRelativePath(
    forgeRuntime: ForgeRuntime,
    windowID: String,
    ref: String,
    definition: [String: AgentlySDK.JSONValue],
    relative: [String]
) async -> [String]? {
    let selection = await forgeRuntime.dataSourceSelectionState(windowID: windowID, dataSourceRef: ref)
    let selectedRow: [String: ForgeIOSRuntime.JSONValue]?
    let remainder: [String]
    switch relative.first {
    case "selection":
        guard relative.count >= 2,
              let selectedIndex = Int(relative[1]),
              selection.selection.indices.contains(selectedIndex) else { return nil }
        selectedRow = selection.selection[selectedIndex]
        remainder = Array(relative.dropFirst(2))
    case "selected":
        selectedRow = selection.selected
        remainder = Array(relative.dropFirst())
    default:
        return nil
    }
    guard let selectedRow else { return nil }
    let rows = await forgeRuntime.dataSourceCollection(windowID: windowID, dataSourceRef: ref)
    guard let index = toolFeedRowIndex(
        rows: rows,
        target: selectedRow,
        uniqueKeys: toolFeedUniqueKeyFields(definition)
    ) else { return nil }
    return [String(index)] + remainder
}

private func toolFeedUniqueKeyFields(_ definition: [String: AgentlySDK.JSONValue]) -> [String] {
    toolFeedArray(definition["uniqueKey"]).compactMap {
        toolFeedString($0.objectValue?["field"]).trimmingCharacters(in: .whitespacesAndNewlines)
    }.filter { !$0.isEmpty }
}

private func reconcileToolFeedSelection(
    _ previous: SelectionState,
    rows: [[String: ForgeIOSRuntime.JSONValue]],
    uniqueKeys: [String]
) -> SelectionState {
    func resolve(_ row: [String: ForgeIOSRuntime.JSONValue]?) -> [String: ForgeIOSRuntime.JSONValue]? {
        guard let row,
              let index = toolFeedRowIndex(rows: rows, target: row, uniqueKeys: uniqueKeys) else { return nil }
        return rows[index]
    }
    let selection = previous.selection.compactMap(resolve)
    let selected = resolve(previous.selected) ?? selection.last
    let rowIndex = selected.flatMap { rows.firstIndex(of: $0) } ?? -1
    return SelectionState(selected: selected, selection: selection, rowIndex: rowIndex)
}

private func toolFeedRowIndex(
    rows: [[String: ForgeIOSRuntime.JSONValue]],
    target: [String: ForgeIOSRuntime.JSONValue],
    uniqueKeys: [String]
) -> Int? {
    if uniqueKeys.isEmpty { return rows.firstIndex(of: target) }
    return rows.firstIndex { row in uniqueKeys.allSatisfy { row[$0] == target[$0] } }
}

private func toolFeedPointerTokens(_ path: String) throws -> [String] {
    guard path.hasPrefix("/") else { throw FeedDraftRuntimeError.invalidPointer(path) }
    return path.split(separator: "/", omittingEmptySubsequences: false).dropFirst().map {
        $0.replacingOccurrences(of: "~1", with: "/").replacingOccurrences(of: "~0", with: "~")
    }
}

private func patchCanonicalToolFeedValue(
    _ current: AgentlySDK.JSONValue,
    tokens: [String],
    operation: ForgeIOSRuntime.FeedPatchOperation
) throws -> AgentlySDK.JSONValue {
    guard let token = tokens.first else {
        guard operation.op != "remove" else { throw FeedDraftRuntimeError.missingPath("canonical root") }
        guard operation.op == "add" || operation.op == "replace" else {
            throw FeedDraftRuntimeError.unsupportedOperation(operation.op)
        }
        return operation.value?.appValue ?? .null
    }
    let remaining = Array(tokens.dropFirst())
    switch current {
    case .object(var object):
        if remaining.isEmpty {
            switch operation.op {
            case "add": object[token] = operation.value?.appValue ?? .null
            case "replace":
                guard object[token] != nil else { throw FeedDraftRuntimeError.missingPath(token) }
                object[token] = operation.value?.appValue ?? .null
            case "remove":
                guard object.removeValue(forKey: token) != nil else { throw FeedDraftRuntimeError.missingPath(token) }
            default: throw FeedDraftRuntimeError.unsupportedOperation(operation.op)
            }
        } else {
            guard let child = object[token] else { throw FeedDraftRuntimeError.missingPath(token) }
            object[token] = try patchCanonicalToolFeedValue(child, tokens: remaining, operation: operation)
        }
        return .object(object)
    case .array(var values):
        if remaining.isEmpty {
            switch operation.op {
            case "add":
                values.insert(operation.value?.appValue ?? .null, at: try toolFeedArrayIndex(token, count: values.count, allowEnd: true))
            case "replace":
                values[try toolFeedArrayIndex(token, count: values.count, allowEnd: false)] = operation.value?.appValue ?? .null
            case "remove":
                values.remove(at: try toolFeedArrayIndex(token, count: values.count, allowEnd: false))
            default: throw FeedDraftRuntimeError.unsupportedOperation(operation.op)
            }
        } else {
            let index = try toolFeedArrayIndex(token, count: values.count, allowEnd: false)
            values[index] = try patchCanonicalToolFeedValue(values[index], tokens: remaining, operation: operation)
        }
        return .array(values)
    default:
        throw FeedDraftRuntimeError.missingPath(token)
    }
}

private func replaceCanonicalToolFeedValue(
    _ current: AgentlySDK.JSONValue,
    tokens: [String],
    value: AgentlySDK.JSONValue
) throws -> AgentlySDK.JSONValue {
    guard let token = tokens.first else { return value }
    let remaining = Array(tokens.dropFirst())
    switch current {
    case .object(var object):
        guard let child = object[token] else { throw FeedDraftRuntimeError.missingPath(token) }
        object[token] = try replaceCanonicalToolFeedValue(child, tokens: remaining, value: value)
        return .object(object)
    case .array(var values):
        let index = try toolFeedArrayIndex(token, count: values.count, allowEnd: false)
        values[index] = try replaceCanonicalToolFeedValue(values[index], tokens: remaining, value: value)
        return .array(values)
    default:
        throw FeedDraftRuntimeError.missingPath(token)
    }
}

private func toolFeedValue(
    at tokens: [String],
    in root: AgentlySDK.JSONValue
) -> AgentlySDK.JSONValue? {
    var current = root
    for token in tokens {
        switch current {
        case .object(let object):
            guard let next = object[token] else { return nil }
            current = next
        case .array(let values):
            guard let index = Int(token), values.indices.contains(index) else { return nil }
            current = values[index]
        default:
            return nil
        }
    }
    return current
}

private func toolFeedArrayIndex(_ token: String, count: Int, allowEnd: Bool) throws -> Int {
    if allowEnd && token == "-" { return count }
    guard let index = Int(token), index >= 0, (allowEnd ? index <= count : index < count) else {
        throw FeedDraftRuntimeError.invalidArrayIndex(token)
    }
    return index
}

private func toolFeedString(_ value: AgentlySDK.JSONValue?) -> String {
    guard case .string(let string) = value else { return "" }
    return string
}

private func toolFeedArray(_ value: AgentlySDK.JSONValue?) -> [AgentlySDK.JSONValue] {
    guard case .array(let values) = value else { return [] }
    return values
}
