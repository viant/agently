import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class ForgeHookSupportTests: XCTestCase {
    func testApplyForgeCollectionHookTransformsRows() {
        let metadata = WindowMetadata(
            actions: ActionsDef(
                code: """
                (() => ({
                  prepareCollection: ({ collection = [] }) => collection.map((row) => ({
                    ...row,
                    applyStatus: String(row.apply_status ?? "").trim().toUpperCase()
                  }))
                }))()
                """
            )
        )
        let rows: [[String: AppJSONValue]] = [[
            "apply_status": .string("approved"),
            "id": .number(2)
        ]]

        let result = applyForgeCollectionHook(metadata: metadata, rows: rows)

        XCTAssertEqual(result.first?["applyStatus"], .string("APPROVED"))
        XCTAssertEqual(result.first?["id"], .number(2))
    }

    func testApplyForgeSelectionHookTransformsSelectedRow() {
        let metadata = WindowMetadata(
            actions: ActionsDef(
                code: """
                (() => ({
                  prepareSelection: ({ selected = {}, rowIndex = -1 }) => ({
                    ...selected,
                    selectedIndex: rowIndex,
                    applyStatus: String(selected.apply_status ?? "").trim().toUpperCase()
                  })
                }))()
                """
            )
        )
        let row: [String: AppJSONValue] = [
            "apply_status": .string("approved"),
            "id": .number(2)
        ]

        let result = applyForgeSelectionHook(metadata: metadata, selectedRow: row, rowIndex: 4)

        XCTAssertEqual(result["applyStatus"], .string("APPROVED"))
        XCTAssertEqual(result["selectedIndex"], .number(4))
    }
}
