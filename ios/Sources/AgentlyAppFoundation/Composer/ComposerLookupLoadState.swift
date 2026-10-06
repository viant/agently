import Foundation
import AgentlySDK

/// A search completion belongs only to the generation that started it.
struct ComposerLookupLoadState {
    private(set) var generation: UInt64 = 0
    private(set) var rows: [[String: JSONValue]] = []
    private(set) var errorMessage: String?
    private(set) var isLoading = false

    mutating func begin() -> UInt64 {
        generation &+= 1
        errorMessage = nil
        isLoading = true
        return generation
    }

    mutating func complete(_ result: Result<[[String: JSONValue]], Error>, generation expected: UInt64, taskCancelled: Bool = false) {
        guard expected == generation else { return }
        isLoading = false
        guard !taskCancelled else { return }
        switch result {
        case .success(let rows):
            self.rows = rows
            errorMessage = nil
        case .failure(let error):
            // SwiftUI cancels an old .task when its search identity changes.
            guard !(error is CancellationError),
                  !((error as NSError).domain == NSURLErrorDomain && (error as NSError).code == URLError.cancelled.rawValue) else { return }
            rows = []
            errorMessage = error.localizedDescription
        }
    }
}
