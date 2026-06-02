import AgentlySDK
import ForgeIOSRuntime

func applyForgeCollectionHook(
    metadata: WindowMetadata,
    rows: [[String: AppJSONValue]]
) -> [[String: AppJSONValue]] {
    guard let code = metadata.actions?.code?.trimmingCharacters(in: .whitespacesAndNewlines),
          !code.isEmpty else {
        return rows
    }
    let payload: ForgeJSONValue = .object([
        "collection": .array(rows.map { .object($0.mapValues(\.forgeValue)) })
    ])
    guard let result = try? ActionHookRuntime.invoke(
        code: code,
        functionName: "prepareCollection",
        props: payload
    ) else {
        return rows
    }
    guard case .array(let transformedRows) = result else {
        return rows
    }
    return transformedRows.compactMap { value in
        value.objectValue?.mapValues(\.appValue)
    }
}

func applyForgeSelectionHook(
    metadata: WindowMetadata,
    selectedRow: [String: AppJSONValue],
    rowIndex: Int
) -> [String: AppJSONValue] {
    guard let code = metadata.actions?.code?.trimmingCharacters(in: .whitespacesAndNewlines),
          !code.isEmpty else {
        return selectedRow
    }
    let payload: ForgeJSONValue = .object([
        "selected": .object(selectedRow.mapValues(\.forgeValue)),
        "rowIndex": .number(Double(rowIndex))
    ])
    guard let result = try? ActionHookRuntime.invoke(
        code: code,
        functionName: "prepareSelection",
        props: payload
    ) else {
        return selectedRow
    }
    guard case .object(let transformedRow) = result else {
        return selectedRow
    }
    return transformedRow.mapValues(\.appValue)
}
